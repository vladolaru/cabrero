package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettings_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")

	settings, settingsPath, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings != nil {
		t.Error("expected nil settings for non-existent file")
	}
	if settingsPath != path {
		t.Errorf("path = %q, want %q", settingsPath, path)
	}
}

func TestLoadSettings_ValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	os.WriteFile(path, []byte(`{"hooks": {"PreCompact": []}}`), 0o644)

	settings, _, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings == nil {
		t.Fatal("expected non-nil settings")
	}
	if _, ok := settings["hooks"]; !ok {
		t.Error("expected 'hooks' key in settings")
	}
}

func TestLoadSettings_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	os.WriteFile(path, []byte(`{invalid`), 0o644)

	_, _, err := LoadSettings(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWriteSettings(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")

	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreCompact": []interface{}{},
		},
	}
	err := WriteSettings(settings, path)
	if err != nil {
		t.Fatalf("WriteSettings: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("empty settings file")
	}
}

func TestWriteSettings_CreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "dir", "settings.json")

	err := WriteSettings(map[string]interface{}{"key": "val"}, path)
	if err != nil {
		t.Fatalf("WriteSettings: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestHookStatus_NoSettings(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")

	pre, session := HookStatus(path)
	if pre || session {
		t.Errorf("HookStatus = (%v, %v), want (false, false)", pre, session)
	}
}

func TestHookStatus_NoHooksKey(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	os.WriteFile(path, []byte(`{"other": "stuff"}`), 0o644)

	pre, session := HookStatus(path)
	if pre || session {
		t.Errorf("HookStatus = (%v, %v), want (false, false)", pre, session)
	}
}

func TestHookStatus_WithCabreroHooks(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	content := `{"hooks": {"PreCompact": [{"matcher": "", "hooks": [{"type": "command", "command": "/path/to/cabrero hook pre-compact"}]}], "SessionEnd": [{"matcher": "", "hooks": [{"type": "command", "command": "/path/to/cabrero hook session-end"}]}]}}`
	os.WriteFile(path, []byte(content), 0o644)

	pre, session := HookStatus(path)
	if !pre {
		t.Error("expected PreCompact hook detected")
	}
	if !session {
		t.Error("expected SessionEnd hook detected")
	}
}

func TestHookStatus_OnlyPreCompact(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	content := `{"hooks": {"PreCompact": [{"matcher": "", "hooks": [{"type": "command", "command": "/path/to/cabrero hook"}]}]}}`
	os.WriteFile(path, []byte(content), 0o644)

	pre, session := HookStatus(path)
	if !pre {
		t.Error("expected PreCompact hook detected")
	}
	if session {
		t.Error("expected no SessionEnd hook")
	}
}

func TestHookGroupContainsCabrero(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want bool
	}{
		{"nil", nil, false},
		{"empty slice", []interface{}{}, false},
		{"with cabrero", []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{"command": "/path/to/cabrero hook"},
				},
			},
		}, true},
		{"without cabrero", []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{"command": "/path/to/other hook"},
				},
			},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HookGroupContainsCabrero(tt.v)
			if got != tt.want {
				t.Errorf("HookGroupContainsCabrero = %v, want %v", got, tt.want)
			}
		})
	}
}
