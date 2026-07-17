// Package manifest tracks which Slack files have already been uploaded so the
// tool is idempotent and safe to re-run after an interruption. The Slack file
// ID is the dedup key: it is globally unique and stable, unlike filenames which
// collide constantly.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type Entry struct {
	SlackFileID string `json:"slack_file_id"`
	DriveFileID string `json:"drive_file_id"`
	Name        string `json:"name"`
	UploadedAt  string `json:"uploaded_at"`
}

type Manifest struct {
	path    string
	mu      sync.Mutex
	entries map[string]Entry
}

// Load reads the manifest at path. A missing file yields an empty manifest so
// the first run starts clean rather than failing.
func Load(path string) (*Manifest, error) {
	m := &Manifest{path: path, entries: make(map[string]Entry)}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	if len(data) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(data, &m.entries); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return m, nil
}

func (m *Manifest) Has(slackFileID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.entries[slackFileID]
	return ok
}

// Record adds an entry and persists the whole manifest to disk immediately, so
// a crash mid-run cannot lose the record of an upload that already succeeded.
func (m *Manifest) Record(e Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[e.SlackFileID] = e
	return m.flushLocked()
}

func (m *Manifest) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

// flushLocked writes atomically via a temp file + rename so a crash during the
// write cannot corrupt an existing manifest into unparseable JSON.
func (m *Manifest) flushLocked() error {
	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write manifest tmp: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return fmt.Errorf("rename manifest: %w", err)
	}
	return nil
}
