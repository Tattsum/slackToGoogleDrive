// Package config loads runtime settings from environment variables (optionally
// seeded from a .env file) and validates that everything required is present.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	SlackToken    string // Bot token (xoxb-...)
	SlackChannel  string // Channel name ("general") or ID ("C0123...")
	DriveFolderID string // Destination folder ID (personal or Shared Drive)
	DownloadDir   string // Where files are staged before upload
	ManifestPath  string // JSON file tracking already-uploaded Slack file IDs
}

// Load reads .env (if present) into the process environment, then builds a
// Config from environment variables and validates required fields.
func Load(envPath string) (Config, error) {
	if err := loadDotEnv(envPath); err != nil {
		return Config{}, err
	}

	cfg := Config{
		SlackToken:    os.Getenv("SLACK_BOT_TOKEN"),
		SlackChannel:  os.Getenv("SLACK_CHANNEL"),
		DriveFolderID: os.Getenv("DRIVE_FOLDER_ID"),
		DownloadDir:   getenvDefault("DOWNLOAD_DIR", "./downloads"),
		ManifestPath:  getenvDefault("MANIFEST_PATH", "./manifest.json"),
	}

	// Only the token is validated here. The channel and Drive folder can also be
	// supplied via CLI flags (-channel / -folder), so they are validated after
	// flag overrides are applied — see RequireChannel and RequireDrive.
	if err := requirePresent(map[string]string{
		"SLACK_BOT_TOKEN": cfg.SlackToken,
	}); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// RequireChannel validates that a channel was supplied via SLACK_CHANNEL or the
// -channel flag.
func (c Config) RequireChannel() error {
	if c.SlackChannel == "" {
		return fmt.Errorf("no channel: set SLACK_CHANNEL or pass -channel")
	}
	return nil
}

// RequireDrive validates the fields needed to upload. It is called separately
// from Load so that -dry-run (which lists Slack files but never touches Drive)
// can run with only the Slack credentials configured — useful while setting the
// tool up one integration at a time.
func (c Config) RequireDrive() error {
	if c.DriveFolderID == "" {
		return fmt.Errorf("no destination folder: set DRIVE_FOLDER_ID or pass -folder")
	}
	return nil
}

func requirePresent(fields map[string]string) error {
	var missing []string
	for name, value := range fields {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv seeds os.Environ from a KEY=VALUE file. A missing file is not an
// error: real environment variables are the primary source and .env is an
// optional convenience for local runs. Existing env vars win over the file so
// an operator can override a single value without editing .env.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}
