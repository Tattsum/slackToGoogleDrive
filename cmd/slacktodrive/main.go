// Command slacktodrive downloads every photo and video from a Slack channel and
// uploads them to a Shared Drive folder. It is idempotent: a local manifest
// keyed by Slack file ID lets it be re-run to pick up new files or resume after
// an interruption.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Tattsum/slackToGoogleDrive/internal/config"
	"github.com/Tattsum/slackToGoogleDrive/internal/drive"
	"github.com/Tattsum/slackToGoogleDrive/internal/manifest"
	"github.com/Tattsum/slackToGoogleDrive/internal/slackfiles"
)

func main() {
	envPath := flag.String("env", ".env", "path to a .env file (optional)")
	channel := flag.String("channel", "", "Slack channel name or ID (overrides SLACK_CHANNEL)")
	folder := flag.String("folder", "", "destination Drive folder ID (overrides DRIVE_FOLDER_ID)")
	dryRun := flag.Bool("dry-run", false, "list files that would be uploaded without downloading or uploading")
	limit := flag.Int("limit", 0, "upload at most N new files this run (0 = no limit); useful to verify the setup on one file first")
	flag.Parse()

	if err := run(*envPath, *channel, *folder, *dryRun, *limit); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run(envPath, channel, folder string, dryRun bool, limit int) error {
	cfg, err := config.Load(envPath)
	if err != nil {
		return err
	}
	if channel != "" {
		cfg.SlackChannel = channel
	}
	if folder != "" {
		cfg.DriveFolderID = folder
	}
	cfg.DriveFolderID = folderID(cfg.DriveFolderID)
	if err := cfg.RequireChannel(); err != nil {
		return err
	}

	if !dryRun {
		if err := cfg.RequireDrive(); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slackClient := slackfiles.New(cfg.SlackToken)

	channelID, err := slackClient.ResolveChannelID(ctx, cfg.SlackChannel)
	if err != nil {
		return err
	}
	log.Printf("resolved channel %q -> %s", cfg.SlackChannel, channelID)

	files, err := slackClient.ListMedia(ctx, channelID)
	if err != nil {
		return err
	}
	log.Printf("found %d image/video/zip files in channel", len(files))

	book, err := manifest.Load(cfg.ManifestPath)
	if err != nil {
		return err
	}
	log.Printf("manifest has %d previously-uploaded files", book.Count())

	if err := os.MkdirAll(cfg.DownloadDir, 0o755); err != nil {
		return fmt.Errorf("create download dir: %w", err)
	}

	var driveClient *drive.Client
	if !dryRun {
		driveClient, err = drive.New(ctx, cfg.DriveFolderID)
		if err != nil {
			return err
		}
	}

	var uploaded, skipped, failed int
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !isWanted(f.Mimetype) {
			continue
		}
		if book.Has(f.ID) {
			skipped++
			continue
		}
		if limit > 0 && uploaded >= limit {
			log.Printf("reached -limit=%d; stopping (re-run without -limit to continue)", limit)
			break
		}
		if dryRun {
			log.Printf("[dry-run] would upload %s (%s, %d bytes)", f.Name, f.ID, f.Size)
			uploaded++
			continue
		}

		if err := process(ctx, slackClient, driveClient, book, cfg.DownloadDir, f); err != nil {
			log.Printf("FAILED %s (%s): %v", f.Name, f.ID, err)
			failed++
			continue
		}
		uploaded++
		log.Printf("uploaded %s (%d/%d)", f.Name, uploaded, len(files))
	}

	log.Printf("done: %d uploaded, %d skipped (already done), %d failed", uploaded, skipped, failed)
	if failed > 0 {
		return fmt.Errorf("%d file(s) failed; re-run to retry (successful ones are recorded and will be skipped)", failed)
	}
	return nil
}

// process downloads one file to disk, uploads it, records it, then removes the
// local copy. Staging to disk (rather than streaming Slack→Drive directly)
// keeps a complete local artifact if the upload half fails and makes the
// download and upload phases independently retryable.
func process(ctx context.Context, sc *slackfiles.Client, dc *drive.Client, book *manifest.Manifest, dir string, f slackfiles.File) error {
	localPath := filepath.Join(dir, sanitizeFilename(f.ID, f.Name))

	if err := downloadToFile(ctx, sc, f, localPath); err != nil {
		return err
	}
	defer os.Remove(localPath)

	driveID, err := uploadWithRetry(ctx, dc, localPath, f)
	if err != nil {
		return err
	}

	return book.Record(manifest.Entry{
		SlackFileID: f.ID,
		DriveFileID: driveID,
		Name:        f.Name,
		UploadedAt:  time.Now().UTC().Format(time.RFC3339),
	})
}

// uploadWithRetry retries the upload on transient failures (large zips/videos
// regularly hit "connection lost" during the minutes-long resumable transfer).
// A failed resumable session leaves no visible Drive file, so re-creating is
// safe and cannot duplicate. The file is reopened each attempt to rewind the
// reader.
func uploadWithRetry(ctx context.Context, dc *drive.Client, path string, f slackfiles.File) (string, error) {
	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		local, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("reopen for upload: %w", err)
		}
		driveID, err := dc.Upload(ctx, f.Name, f.Mimetype, f.ID, local)
		local.Close()
		if err == nil {
			return driveID, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if attempt < maxAttempts {
			backoff := time.Duration(attempt) * 5 * time.Second
			log.Printf("retry %s in %s (attempt %d/%d): %v", f.Name, backoff, attempt, maxAttempts, err)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return "", fmt.Errorf("upload failed after %d attempts: %w", maxAttempts, lastErr)
}

func downloadToFile(ctx context.Context, sc *slackfiles.Client, f slackfiles.File, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer out.Close()

	if err := sc.Download(ctx, f, out); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

// folderID accepts either a raw Drive folder ID or a folder URL pasted from the
// browser (e.g. https://drive.google.com/drive/u/0/folders/<ID>?usp=...) and
// returns just the ID, so callers don't have to hand-extract it.
func folderID(s string) string {
	s = strings.TrimSpace(s)
	const marker = "folders/"
	if i := strings.Index(s, marker); i >= 0 {
		s = s[i+len(marker):]
	}
	if i := strings.IndexAny(s, "?/#"); i >= 0 {
		s = s[:i]
	}
	return s
}

func isWanted(mimetype string) bool {
	switch {
	case strings.HasPrefix(mimetype, "image/"), strings.HasPrefix(mimetype, "video/"):
		return true
	case mimetype == "application/zip", mimetype == "application/x-zip-compressed":
		return true
	default:
		return false
	}
}

// sanitizeFilename prefixes the unique Slack file ID so two files that share a
// display name ("image.png") never collide on disk, and strips path separators
// so a crafted name cannot escape the download directory.
func sanitizeFilename(id, name string) string {
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, `\`, "_")
	if name == "" {
		name = "file"
	}
	return id + "_" + name
}
