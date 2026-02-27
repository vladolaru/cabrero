// Package claude provides shared logic for reading, writing, and inspecting
// Claude Code settings. Commands and TUI import this package instead of
// reimplementing settings logic inline.
package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// SettingsPath returns the default Claude Code settings.json path.
func SettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// LoadSettings reads and parses the Claude Code settings JSON file.
// Returns (nil, path, nil) if the file doesn't exist.
func LoadSettings(path string) (map[string]interface{}, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, path, nil
		}
		return nil, path, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, path, err
	}
	return settings, path, nil
}

// WriteSettings serializes settings to the given path with pretty-printing.
// Creates parent directories if needed.
func WriteSettings(settings map[string]interface{}, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// HookStatus checks whether cabrero hooks are registered for PreCompact
// and SessionEnd events at the given settings path.
// Returns (preCompact, sessionEnd).
func HookStatus(settingsPath string) (bool, bool) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, false
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, false
	}

	hooksRaw, ok := settings["hooks"]
	if !ok {
		return false, false
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		return false, false
	}

	return HookGroupContainsCabrero(hooks["PreCompact"]), HookGroupContainsCabrero(hooks["SessionEnd"])
}

// HookGroupContainsCabrero checks if a hook group (any type) contains
// a cabrero hook by marshalling to JSON and string-matching.
// Works with both []interface{} (from map[string]interface{} settings)
// and json.RawMessage.
func HookGroupContainsCabrero(v interface{}) bool {
	if v == nil {
		return false
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), "cabrero")
}
