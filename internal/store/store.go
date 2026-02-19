// Package store manages the ~/.cabrero/ directory layout and provides access
// to raw session backups, digests, evaluations, proposals, and the blocklist.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Root returns the absolute path to the Cabrero data directory.
func Root() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// This would only fail on truly broken systems.
		panic(fmt.Sprintf("cannot determine home directory: %v", err))
	}
	return filepath.Join(home, ".cabrero")
}

// subdirectories created by Init.
var subdirs = []string{
	"raw",
	"digests",
	"prompts",
	"evaluations",
	"proposals",
}

// Init creates the ~/.cabrero/ directory tree if it doesn't exist.
// Safe to call on every invocation.
func Init() error {
	root := Root()
	for _, sub := range subdirs {
		dir := filepath.Join(root, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	// Ensure blocklist.json exists.
	blPath := blocklistPath()
	if _, err := os.Stat(blPath); os.IsNotExist(err) {
		if err := writeBlocklist(nil); err != nil {
			return fmt.Errorf("creating blocklist: %w", err)
		}
	}
	return nil
}

// RawDir returns the path to a session's raw backup directory.
func RawDir(sessionID string) string {
	return filepath.Join(Root(), "raw", sessionID)
}

// --- blocklist --------------------------------------------------------

var blocklistMu sync.Mutex

func blocklistPath() string {
	return filepath.Join(Root(), "blocklist.json")
}

func readBlocklist() (map[string]bool, error) {
	data, err := os.ReadFile(blocklistPath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]bool), nil
		}
		return nil, err
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("parsing blocklist: %w", err)
	}
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m, nil
}

func writeBlocklist(m map[string]bool) error {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(blocklistPath(), data, 0o644)
}

// BlockSession adds a session ID to the blocklist so Cabrero never
// processes its own CLI sessions.
func BlockSession(sessionID string) error {
	blocklistMu.Lock()
	defer blocklistMu.Unlock()

	m, err := readBlocklist()
	if err != nil {
		return err
	}
	m[sessionID] = true
	return writeBlocklist(m)
}

// IsBlocked returns true if the given session ID is in the blocklist.
func IsBlocked(sessionID string) bool {
	blocklistMu.Lock()
	defer blocklistMu.Unlock()

	m, err := readBlocklist()
	if err != nil {
		return false
	}
	return m[sessionID]
}

// BlocklistLen returns the number of entries in the blocklist.
func BlocklistLen() int {
	blocklistMu.Lock()
	defer blocklistMu.Unlock()

	m, err := readBlocklist()
	if err != nil {
		return 0
	}
	return len(m)
}
