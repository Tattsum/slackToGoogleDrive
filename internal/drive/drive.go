// Package drive uploads files into a Drive folder using Application Default
// Credentials (ADC) — the token written by
// `gcloud auth application-default login`. Auth runs as the logged-in user, so
// uploads count against that user's quota and can target a personal My Drive
// folder; a service account has no My Drive quota and would 403.
// SupportsAllDrives(true) is set so a Shared Drive folder also works.
package drive

import (
	"context"
	"fmt"
	"io"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const uploadChunkSize = 16 * 1024 * 1024 // resumable chunk; retries per-chunk on network drops

const adcHint = "run: gcloud auth application-default login --scopes=openid,https://www.googleapis.com/auth/drive"

type Client struct {
	svc      *drive.Service
	folderID string
}

// New builds a Drive client from ADC. It fails fast with a copy-pasteable
// gcloud command if credentials are missing, rather than surfacing an opaque
// error on the first upload. Note: for gcloud user credentials the drive scope
// must be granted at login time (WithScopes cannot widen a user token), which is
// why the hint pins --scopes.
func New(ctx context.Context, folderID string) (*Client, error) {
	if _, err := google.FindDefaultCredentials(ctx, drive.DriveScope); err != nil {
		return nil, fmt.Errorf("no Application Default Credentials (%s): %w", adcHint, err)
	}

	svc, err := drive.NewService(ctx, option.WithScopes(drive.DriveScope))
	if err != nil {
		return nil, fmt.Errorf("create drive service (%s): %w", adcHint, err)
	}
	return &Client{svc: svc, folderID: folderID}, nil
}

// Upload streams r into the destination folder as a resumable upload (so large
// videos survive network drops) and stamps the Slack file ID into appProperties
// as a recovery key independent of the local manifest. Returns the new Drive
// file ID.
func (c *Client) Upload(ctx context.Context, name, mimetype, slackFileID string, r io.Reader) (string, error) {
	meta := &drive.File{
		Name:          name,
		Parents:       []string{c.folderID},
		MimeType:      mimetype,
		AppProperties: map[string]string{"slackFileId": slackFileID},
	}

	created, err := c.svc.Files.Create(meta).
		SupportsAllDrives(true).
		Media(r, googleapi.ChunkSize(uploadChunkSize)).
		Fields("id").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("upload %s: %w", name, err)
	}
	return created.Id, nil
}
