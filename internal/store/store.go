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
	return AtomicWrite(blocklistPath(), data, 0o644)
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
