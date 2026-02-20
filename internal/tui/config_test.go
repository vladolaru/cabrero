package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

func TestLoadConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}

	// Should return defaults when file doesn't exist.
	want := shared.DefaultConfig()
	if cfg.Navigation != want.Navigation {
		t.Errorf("Navigation = %q, want %q", cfg.Navigation, want.Navigation)
	}
	if cfg.Theme != want.Theme {
		t.Errorf("Theme = %q, want %q", cfg.Theme, want.Theme)
	}
	if cfg.Dashboard.SortOrder != want.Dashboard.SortOrder {
		t.Errorf("SortOrder = %q, want %q", cfg.Dashboard.SortOrder, want.Dashboard.SortOrder)
	}
}

func TestLoadConfig_Partial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write partial config: only navigation and one nested field.
	partial := `{"navigation": "vim", "dashboard": {"sortOrder": "oldest"}}`
	if err := os.WriteFile(path, []byte(partial), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}

	// Specified fields should be overridden.
	if cfg.Navigation != "vim" {
		t.Errorf("Navigation = %q, want %q", cfg.Navigation, "vim")
	}
	if cfg.Dashboard.SortOrder != "oldest" {
		t.Errorf("SortOrder = %q, want %q", cfg.Dashboard.SortOrder, "oldest")
	}

	// Unspecified fields should keep defaults.
	if cfg.Theme != "auto" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "auto")
	}
	if !cfg.Detail.ChatPanelOpen {
		t.Error("Detail.ChatPanelOpen = false, want true (default)")
	}
	if cfg.Detail.ChatPanelWidth != 35 {
		t.Errorf("Detail.ChatPanelWidth = %d, want 35 (default)", cfg.Detail.ChatPanelWidth)
	}
	if !cfg.Personality.FlavorText {
		t.Error("Personality.FlavorText = false, want true (default)")
	}
}

func TestLoadConfig_PreservesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Config with an unknown field from a future version.
	withExtra := `{"navigation": "vim", "futureFeature": {"enabled": true}}`
	if err := os.WriteFile(path, []byte(withExtra), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}

	if cfg.Navigation != "vim" {
		t.Errorf("Navigation = %q, want %q", cfg.Navigation, "vim")
	}

	// Save and reload — unknown field should survive.
	if err := SaveConfigTo(cfg, path); err != nil {
		t.Fatalf("SaveConfigTo: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	if _, ok := raw["futureFeature"]; !ok {
		t.Error("unknown field 'futureFeature' was lost during roundtrip")
	}
}

func TestSaveConfig_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := shared.DefaultConfig()
	original.Navigation = "vim"
	original.Dashboard.SortOrder = "confidence"
	original.Detail.ChatPanelWidth = 40
	original.Personality.FlavorText = false
	original.Pipeline.SparklineDays = 14

	if err := SaveConfigTo(original, path); err != nil {
		t.Fatalf("SaveConfigTo: %v", err)
	}

	loaded, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}

	if loaded.Navigation != original.Navigation {
		t.Errorf("Navigation = %q, want %q", loaded.Navigation, original.Navigation)
	}
	if loaded.Dashboard.SortOrder != original.Dashboard.SortOrder {
		t.Errorf("SortOrder = %q, want %q", loaded.Dashboard.SortOrder, original.Dashboard.SortOrder)
	}
	if loaded.Detail.ChatPanelWidth != original.Detail.ChatPanelWidth {
		t.Errorf("ChatPanelWidth = %d, want %d", loaded.Detail.ChatPanelWidth, original.Detail.ChatPanelWidth)
	}
	if loaded.Personality.FlavorText != original.Personality.FlavorText {
		t.Errorf("FlavorText = %v, want %v", loaded.Personality.FlavorText, original.Personality.FlavorText)
	}
	if loaded.Pipeline.SparklineDays != original.Pipeline.SparklineDays {
		t.Errorf("Pipeline.SparklineDays = %d, want %d", loaded.Pipeline.SparklineDays, original.Pipeline.SparklineDays)
	}
	// Defaults that weren't changed should survive.
	if loaded.Theme != "auto" {
		t.Errorf("Theme = %q, want %q", loaded.Theme, "auto")
	}
	if !loaded.Confirmations.ApproveRequiresConfirm {
		t.Error("ApproveRequiresConfirm = false, want true")
	}
}
