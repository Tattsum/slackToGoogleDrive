package manifest

import (
	"path/filepath"
	"testing"
)

func TestManifest(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, path string)
	}{
		{
			name: "missing file loads as empty and reports nothing recorded",
			run: func(t *testing.T, path string) {
				m, err := Load(path)
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				if m.Count() != 0 {
					t.Fatalf("Count = %d, want 0", m.Count())
				}
				if m.Has("F123") {
					t.Fatal("Has(F123) = true on empty manifest")
				}
			},
		},
		{
			name: "recorded file is reported present and survives reload",
			run: func(t *testing.T, path string) {
				m, _ := Load(path)
				if err := m.Record(Entry{SlackFileID: "F123", DriveFileID: "D1", Name: "a.png"}); err != nil {
					t.Fatalf("Record: %v", err)
				}
				reloaded, err := Load(path)
				if err != nil {
					t.Fatalf("reload: %v", err)
				}
				if !reloaded.Has("F123") {
					t.Fatal("reloaded manifest lost F123")
				}
				if reloaded.Count() != 1 {
					t.Fatalf("Count = %d, want 1", reloaded.Count())
				}
			},
		},
		{
			name: "re-recording the same id does not duplicate",
			run: func(t *testing.T, path string) {
				m, _ := Load(path)
				_ = m.Record(Entry{SlackFileID: "F1", Name: "first"})
				_ = m.Record(Entry{SlackFileID: "F1", Name: "second"})
				if m.Count() != 1 {
					t.Fatalf("Count = %d, want 1", m.Count())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.json")
			tt.run(t, path)
		})
	}
}
