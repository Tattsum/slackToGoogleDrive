// Package slackfiles lists the image/video files posted in a channel and
// downloads their bytes. It wraps slack-go with the two things the raw client
// does not give us: channel name→ID resolution, and authenticated downloads of
// url_private (which require the bot token in an Authorization header).
package slackfiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

type Client struct {
	api   *slack.Client
	token string
	http  *http.Client
}

func New(token string) *Client {
	return &Client{
		api:   slack.New(token),
		token: token,
		http:  &http.Client{Timeout: 10 * time.Minute},
	}
}

// ResolveChannelID accepts either a channel ID (returned as-is) or a channel
// name and looks up its ID by scanning public channels. Names are matched
// without a leading '#'.
func (c *Client) ResolveChannelID(ctx context.Context, channel string) (string, error) {
	if looksLikeChannelID(channel) {
		return channel, nil
	}
	want := strings.TrimPrefix(channel, "#")

	params := &slack.GetConversationsParameters{
		Types:           []string{"public_channel"},
		Limit:           1000,
		ExcludeArchived: false,
	}
	for {
		channels, cursor, err := c.api.GetConversationsContext(ctx, params)
		if err != nil {
			return "", fmt.Errorf("list conversations: %w", err)
		}
		for _, ch := range channels {
			if ch.Name == want {
				return ch.ID, nil
			}
		}
		if cursor == "" {
			return "", fmt.Errorf("channel %q not found among public channels", channel)
		}
		params.Cursor = cursor
	}
}

// File is the subset of a Slack file we act on.
type File struct {
	ID          string
	Name        string
	Mimetype    string
	DownloadURL string
	Size        int
}

// ListMedia returns every image, video, and zip file in the channel. files.list
// uses page-based pagination (paging.pages), NOT the response_metadata.next_cursor
// that most Slack endpoints use — its next_cursor is always empty, so we must
// walk pages 1..Pages or we silently stop after the first 100 files.
// types=images,videos,zips filters server-side; the caller still checks the mimetype.
func (c *Client) ListMedia(ctx context.Context, channelID string) ([]File, error) {
	params := slack.GetFilesParameters{
		Channel: channelID,
		Types:   "images,videos,zips",
		Count:   100, // files.list caps count at 100 per page
		Page:    1,
	}

	var out []File
	for {
		files, paging, err := c.getFilesWithRetry(ctx, params)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			url := f.URLPrivateDownload
			if url == "" {
				url = f.URLPrivate
			}
			out = append(out, File{
				ID:          f.ID,
				Name:        f.Name,
				Mimetype:    f.Mimetype,
				DownloadURL: url,
				Size:        f.Size,
			})
		}
		if paging == nil || params.Page >= paging.Pages {
			return out, nil
		}
		params.Page++
	}
}

// getFilesWithRetry honors Slack's Retry-After on 429 rather than failing the
// whole run: even internal apps can hit bursts, and re-listing from scratch is
// wasteful.
func (c *Client) getFilesWithRetry(ctx context.Context, params slack.GetFilesParameters) ([]slack.File, *slack.Paging, error) {
	for {
		files, paging, err := c.api.GetFilesContext(ctx, params)
		var rlErr *slack.RateLimitedError
		if errors.As(err, &rlErr) {
			if waitErr := sleep(ctx, rlErr.RetryAfter); waitErr != nil {
				return nil, nil, waitErr
			}
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("list files: %w", err)
		}
		return files, paging, nil
	}
}

// Download streams a file's bytes to w. Slack file URLs are private: without the
// bot token in the Authorization header Slack returns an HTML login page with a
// 200 status, so a text/html response is treated as an auth failure rather than
// silently writing garbage.
func (c *Client) Download(ctx context.Context, f File, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", f.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %d", f.Name, resp.StatusCode)
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		return fmt.Errorf("download %s: got text/html (token likely lacks files:read or bot not in channel)", f.Name)
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("stream %s: %w", f.Name, err)
	}
	return nil
}

func looksLikeChannelID(s string) bool {
	// Slack channel IDs start with C (public), G (private), or D (DM) and are
	// uppercase alphanumeric. Names are lowercase and may contain '-'/'_'.
	if len(s) < 8 {
		return false
	}
	switch s[0] {
	case 'C', 'G', 'D':
	default:
		return false
	}
	return s == strings.ToUpper(s) && !strings.ContainsAny(s, "-_ #")
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
