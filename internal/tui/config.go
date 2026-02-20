// Package tui implements the interactive review TUI for Cabrero.
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vladolaru/cabrero/internal/store"
)

// Config holds all TUI configuration. Stored at ~/.cabrero/config.json.
// Missing fields get defaults. Unknown fields are preserved on roundtrip.
type Config struct {
	Navigation    string            `json:"navigation"`
	Theme         string            `json:"theme"`
	Dashboard     DashboardConfig   `json:"dashboard"`
	Detail        DetailConfig      `json:"detail"`
	Personality   PersonalityConfig `json:"personality"`
	Confirmations ConfirmConfig     `json:"confirmations"`

	// extra preserves unknown JSON fields for forward compatibility.
	extra map[string]json.RawMessage
}

// DashboardConfig holds dashboard-specific settings.
type DashboardConfig struct {
	SortOrder           string `json:"sortOrder"`
	ShowRecentlyDecided bool   `json:"showRecentlyDecided"`
	RecentlyDecidedLimit int   `json:"recentlyDecidedLimit"`
}

// DetailConfig holds proposal detail view settings.
type DetailConfig struct {
	ChatPanelOpen          bool `json:"chatPanelOpen"`
	ChatPanelWidth         int  `json:"chatPanelWidth"`
	ExpandCitationsDefault bool `json:"expandCitationsDefault"`
}

// PersonalityConfig controls pirategoat personality features.
type PersonalityConfig struct {
	FlavorText bool `json:"flavorText"`
	EasterEggs bool `json:"easterEggs"`
}

// ConfirmConfig controls which actions require confirmation.
type ConfirmConfig struct {
	ApproveRequiresConfirm  bool `json:"approveRequiresConfirm"`
	RejectRequiresConfirm   bool `json:"rejectRequiresConfirm"`
	DeferRequiresConfirm    bool `json:"deferRequiresConfirm"`
	RetryRequiresConfirm    bool `json:"retryRequiresConfirm"`
	RollbackRequiresConfirm bool `json:"rollbackRequiresConfirm"`
}

// DefaultConfig returns a Config with all design-doc default values.
func DefaultConfig() *Config {
	return &Config{
		Navigation: "arrows",
		Theme:      "auto",
		Dashboard: DashboardConfig{
			SortOrder:            "newest",
			ShowRecentlyDecided:  true,
			RecentlyDecidedLimit: 10,
		},
		Detail: DetailConfig{
			ChatPanelOpen:          true,
			ChatPanelWidth:         35,
			ExpandCitationsDefault: false,
		},
		Personality: PersonalityConfig{
			FlavorText: true,
			EasterEggs: true,
		},
		Confirmations: ConfirmConfig{
			ApproveRequiresConfirm:  true,
			RejectRequiresConfirm:   false,
			DeferRequiresConfirm:    false,
			RetryRequiresConfirm:    true,
			RollbackRequiresConfirm: true,
		},
	}
}

func configPath() string {
	return filepath.Join(store.Root(), "config.json")
}

// LoadConfig reads ~/.cabrero/config.json and merges with defaults.
// Returns defaults if the file does not exist.
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(configPath())
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
	}
	cfg.extra = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !known[k] {
			cfg.extra[k] = v
		}
	}

	return cfg, nil
}

// LoadConfigFrom reads config from a specific path (for testing).
func LoadConfigFrom(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	known := map[string]bool{
		"navigation": true, "theme": true,
		"dashboard": true, "detail": true,
		"personality": true, "confirmations": true,
	}
	cfg.extra = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !known[k] {
			cfg.extra[k] = v
		}
	}

	return cfg, nil
}

// SaveConfig writes config to ~/.cabrero/config.json atomically.
func SaveConfig(cfg *Config) error {
	return SaveConfigTo(cfg, configPath())
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
	for k, v := range cfg.extra {
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
