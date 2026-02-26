// Package store manages the ~/.cabrero/ directory layout and provides access
// to raw session backups, digests, evaluations, proposals, and the blocklist.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// testRootOverride is set by RootOverrideForTest to redirect store access in tests.
var testRootOverride string

// Root returns the absolute path to the Cabrero data directory.
func Root() string {
	if testRootOverride != "" {
		return testRootOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// This would only fail on truly broken systems.
		panic(fmt.Sprintf("cannot determine home directory: %v", err))
	}
	return filepath.Join(home, ".cabrero")
}

// RootOverrideForTest sets a temporary root directory for tests.
// Returns the previous override value for restoration with ResetRootOverrideForTest.
// Not goroutine-safe — use only in single-threaded test setup/teardown.
func RootOverrideForTest(dir string) string {
	old := testRootOverride
	testRootOverride = dir
	return old
}

// ResetRootOverrideForTest restores the root directory to a previous value
// returned by RootOverrideForTest.
func ResetRootOverrideForTest(old string) {
	testRootOverride = old
}

// subdirectories created by Init.
var subdirs = []string{
	"raw",
	"digests",
	"prompts",
	"evaluations",
	"proposals",
	"replays",
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

// ReplayDir returns the path to the replays directory.
func ReplayDir() string {
	return filepath.Join(Root(), "replays")
}

// ArchivedProposalsDir returns the path to the archived proposals directory.
func ArchivedProposalsDir() string {
	return filepath.Join(Root(), "proposals", "archived")
}

// AtomicWrite writes data to path using a temp file + rename to prevent
// partial writes on crash. The temp file is created in the same directory
// as path to ensure the rename is atomic (same filesystem).
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// --- blocklist --------------------------------------------------------

var blocklistMu sync.Mutex

func blocklistPath() string {
	return filepath.Join(Root(), "blocklist.json")
}

// BlocklistEntry records when a session was blocked.
type BlocklistEntry struct {
	BlockedAt time.Time `json:"blockedAt"`
}

// ReadBlocklist returns the set of blocked session IDs.
// Callers can use the returned map to batch-check multiple sessions without
// repeated disk reads.
func ReadBlocklist() (map[string]bool, error) {
	return readBlocklist()
}

func readBlocklist() (map[string]bool, error) {
	data, err := os.ReadFile(blocklistPath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]bool), nil
		}
		return nil, err
	}
	// Migration: if root is a JSON array, it's the old []string format.
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		var ids []string
		if err := json.Unmarshal(data, &ids); err != nil {
			return nil, fmt.Errorf("parsing old-format blocklist: %w", err)
		}
		// Convert to new format with zero time; write back.
		entries := make(map[string]BlocklistEntry, len(ids))
		for _, id := range ids {
			entries[id] = BlocklistEntry{} // zero BlockedAt
		}
		if err := writeBlocklistEntries(entries); err != nil {
			return nil, fmt.Errorf("migrating blocklist: %w", err)
		}
		m := make(map[string]bool, len(ids))
		for _, id := range ids {
			m[id] = true
		}
		return m, nil
	}
	var entries map[string]BlocklistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing blocklist: %w", err)
	}
	m := make(map[string]bool, len(entries))
	for id := range entries {
		m[id] = true
	}
	return m, nil
}

func writeBlocklistEntries(entries map[string]BlocklistEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(blocklistPath(), data, 0o644)
}

// writeBlocklist is kept for Init's nil seed (writes empty map).
func writeBlocklist(m map[string]bool) error {
	entries := make(map[string]BlocklistEntry, len(m))
	for id := range m {
		entries[id] = BlocklistEntry{}
	}
	return writeBlocklistEntries(entries)
}

// BlockSession adds a session ID to the blocklist with a timestamp.
func BlockSession(sessionID string, blockedAt time.Time) error {
	blocklistMu.Lock()
	defer blocklistMu.Unlock()

	data, err := os.ReadFile(blocklistPath())
	var entries map[string]BlocklistEntry
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		entries = make(map[string]BlocklistEntry)
	} else {
		trimmed := strings.TrimSpace(string(data))
		if strings.HasPrefix(trimmed, "[") {
			// old format — read via migration path
			entries = make(map[string]BlocklistEntry)
		} else {
			if err2 := json.Unmarshal(data, &entries); err2 != nil {
				entries = make(map[string]BlocklistEntry)
			}
		}
	}
	entries[sessionID] = BlocklistEntry{BlockedAt: blockedAt}
	return writeBlocklistEntries(entries)
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

// RotateBlocklist removes blocklist entries older than maxAge.
// Returns the number of entries removed and rewrites the file atomically.
// Entries with a zero BlockedAt (migrated from old format) are treated as
// maximally old and will always be removed.
func RotateBlocklist(maxAge time.Duration) (int, error) {
	blocklistMu.Lock()
	defer blocklistMu.Unlock()

	data, err := os.ReadFile(blocklistPath())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		// Old format: all entries have zero time — all will be rotated.
		var ids []string
		json.Unmarshal(data, &ids) //nolint:errcheck
		if err := writeBlocklistEntries(make(map[string]BlocklistEntry)); err != nil {
			return 0, err
		}
		return len(ids), nil
	}

	var entries map[string]BlocklistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return 0, fmt.Errorf("parsing blocklist for rotation: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	kept := make(map[string]BlocklistEntry, len(entries))
	removed := 0
	for id, entry := range entries {
		// Zero time is treated as epoch — always older than any cutoff.
		if !entry.BlockedAt.IsZero() && entry.BlockedAt.After(cutoff) {
			kept[id] = entry
		} else {
			removed++
		}
	}
	if removed == 0 {
		return 0, nil
	}
	if err := writeBlocklistEntries(kept); err != nil {
		return 0, err
	}
	return removed, nil
}

// --- config helpers ---------------------------------------------------

// storeConfig is the minimal subset of config.json needed by store-level helpers.
type storeConfig struct {
	Debug             bool   `json:"debug"`
	ClassifierModel   string `json:"classifierModel"`
	EvaluatorModel    string `json:"evaluatorModel"`
	ClassifierTimeout string `json:"classifierTimeout"`
	EvaluatorTimeout  string `json:"evaluatorTimeout"`
}

// readConfig reads and parses config.json once.
// Returns zero-value storeConfig if the file is missing or malformed.
func readConfig() storeConfig {
	data, err := os.ReadFile(filepath.Join(Root(), "config.json"))
	if err != nil {
		return storeConfig{}
	}
	var cfg storeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return storeConfig{}
	}
	return cfg
}

// ReadDebugFlag reads the "debug" field from ~/.cabrero/config.json.
// Returns false if the file is missing, malformed, or the field is absent.
func ReadDebugFlag() bool {
	return readConfig().Debug
}

// PipelineOverrides holds optional pipeline overrides from config.json.
type PipelineOverrides struct {
	ClassifierModel   string `json:"classifierModel"`
	EvaluatorModel    string `json:"evaluatorModel"`
	ClassifierTimeout string `json:"classifierTimeout"` // e.g. "3m", "90s"
	EvaluatorTimeout  string `json:"evaluatorTimeout"`  // e.g. "7m", "5m30s"
}

// ReadPipelineOverrides reads pipeline overrides from ~/.cabrero/config.json.
// Returns zero-value fields for missing file, malformed JSON, or absent keys.
func ReadPipelineOverrides() PipelineOverrides {
	cfg := readConfig()
	return PipelineOverrides{
		ClassifierModel:   cfg.ClassifierModel,
		EvaluatorModel:    cfg.EvaluatorModel,
		ClassifierTimeout: cfg.ClassifierTimeout,
		EvaluatorTimeout:  cfg.EvaluatorTimeout,
	}
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
