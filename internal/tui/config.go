// Package tui implements the interactive review TUI for Cabrero.
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vladolaru/cabrero/internal/store"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Config is an alias for shared.Config, used by the root tui package.
type Config = shared.Config

func configPath() string {
	return filepath.Join(store.Root(), "config.json")
}

// LoadConfig reads ~/.cabrero/config.json and merges with defaults.
// Returns defaults if the file does not exist.
func LoadConfig() (*Config, error) {
	return LoadConfigFrom(configPath())
}

// LoadConfigFrom reads config from a specific path (for testing).
func LoadConfigFrom(path string) (*Config, error) {
	cfg := shared.DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// First pass: unmarshal into a raw map to capture unknown fields.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Second pass: unmarshal into the typed config (merges over defaults).
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Preserve unknown top-level fields.
	known := map[string]bool{
		"navigation": true, "theme": true,
		"dashboard": true, "detail": true,
		"personality": true, "confirmations": true,
		"sourceManager": true, "pipeline": true,
	}
	cfg.Extra = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !known[k] {
			cfg.Extra[k] = v
		}
	}

	return cfg, nil
}

// SaveConfigTo writes config to a specific path atomically (for testing).
func SaveConfigTo(cfg *Config, path string) error {
	// Build a map that includes known and unknown fields.
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(data, &merged); err != nil {
		return fmt.Errorf("merging config: %w", err)
	}

	// Re-add unknown fields.
	for k, v := range cfg.Extra {
		merged[k] = v
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("formatting config: %w", err)
	}
	out = append(out, '\n')

	// Atomic write: temp file + rename.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "config-*.json")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming config: %w", err)
	}

	return nil
}
