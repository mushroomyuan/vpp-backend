package authz

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Snapshot is the on-disk last-success policy cache (§6.3).
type Snapshot struct {
	SyncedAt time.Time    `json:"synced_at"`
	Policies []PolicyRule `json:"policies"`
}

func loadSnapshot(path string) (*Snapshot, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("authz snapshot: %w", err)
	}
	if s.SyncedAt.IsZero() {
		return nil, fmt.Errorf("authz snapshot: missing synced_at")
	}
	return &s, nil
}

func saveSnapshot(path string, s Snapshot) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
