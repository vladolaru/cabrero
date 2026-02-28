package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigKeys_AllValid(t *testing.T) {
	for _, k := range ConfigKeys() {
		if k.Key == "" || k.JSONField == "" || k.Type == "" {
			t.Errorf("invalid config key entry: %+v", k)
		}
	}
}

func TestConfigGet_MissingFile_ReturnsDefault(t *testing.T) {
	tmp := t.TempDir()
	old := RootOverrideForTest(filepath.Join(tmp, ".cabrero"))
	defer ResetRootOverrideForTest(old)
	os.MkdirAll(filepath.Join(tmp, ".cabrero"), 0o755)

	val, isDefault, err := ConfigGet("evaluator.model")
	if err != nil {
		t.Fatalf("ConfigGet: %v", err)
	}
	if val != "claude-sonnet-4-6" {
		t.Errorf("got %q, want claude-sonnet-4-6", val)
	}
	if !isDefault {
		t.Error("expected isDefault=true")
	}
}

func TestConfigSet_CreatesFileIfMissing(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	old := RootOverrideForTest(root)
	defer ResetRootOverrideForTest(old)
	os.MkdirAll(root, 0o755)

	if err := ConfigSet("debug", "true"); err != nil {
		t.Fatalf("ConfigSet: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		t.Fatalf("reading config.json: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing config.json: %v", err)
	}
	if string(m["debug"]) != "true" {
		t.Errorf("debug = %s, want true", m["debug"])
	}
}

func TestConfigSet_PreservesOtherFields(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	old := RootOverrideForTest(root)
	defer ResetRootOverrideForTest(old)
	os.MkdirAll(root, 0o755)

	// Write initial config with TUI fields.
	initial := `{"navigation":"vim","theme":"dark"}`
	os.WriteFile(filepath.Join(root, "config.json"), []byte(initial), 0o644)

	if err := ConfigSet("debug", "true"); err != nil {
		t.Fatalf("ConfigSet: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "config.json"))
	var m map[string]json.RawMessage
	json.Unmarshal(data, &m)

	if string(m["navigation"]) != `"vim"` {
		t.Errorf("navigation lost: got %s", m["navigation"])
	}
	if string(m["theme"]) != `"dark"` {
		t.Errorf("theme lost: got %s", m["theme"])
	}
}

func TestConfigGet_ReadsSetValue(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	old := RootOverrideForTest(root)
	defer ResetRootOverrideForTest(old)
	os.MkdirAll(root, 0o755)

	ConfigSet("evaluator.model", "claude-opus-4-6")
	val, isDefault, err := ConfigGet("evaluator.model")
	if err != nil {
		t.Fatalf("ConfigGet: %v", err)
	}
	if val != "claude-opus-4-6" || isDefault {
		t.Errorf("got %q (default=%v), want claude-opus-4-6 (default=false)", val, isDefault)
	}
}

func TestConfigUnset_RemovesField(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	old := RootOverrideForTest(root)
	defer ResetRootOverrideForTest(old)
	os.MkdirAll(root, 0o755)

	ConfigSet("evaluator.model", "claude-opus-4-6")
	ConfigUnset("evaluator.model")
	val, isDefault, _ := ConfigGet("evaluator.model")
	if val != "claude-sonnet-4-6" || !isDefault {
		t.Errorf("after unset: got %q (default=%v), want default", val, isDefault)
	}
}

func TestConfigUnset_MissingKey_NoError(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	old := RootOverrideForTest(root)
	defer ResetRootOverrideForTest(old)
	os.MkdirAll(root, 0o755)

	if err := ConfigUnset("evaluator.model"); err != nil {
		t.Errorf("unset on absent key should not error: %v", err)
	}
}

func TestConfigSet_ValidatesDuration(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	old := RootOverrideForTest(root)
	defer ResetRootOverrideForTest(old)
	os.MkdirAll(root, 0o755)

	err := ConfigSet("evaluator.timeout", "not-a-duration")
	if err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestConfigSet_ValidatesInt(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	old := RootOverrideForTest(root)
	defer ResetRootOverrideForTest(old)
	os.MkdirAll(root, 0o755)

	err := ConfigSet("circuit-breaker.threshold", "abc")
	if err == nil {
		t.Error("expected error for non-integer")
	}
}

func TestConfigSet_ValidatesBool(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	old := RootOverrideForTest(root)
	defer ResetRootOverrideForTest(old)
	os.MkdirAll(root, 0o755)

	err := ConfigSet("debug", "maybe")
	if err == nil {
		t.Error("expected error for non-bool")
	}
}

func TestConfigListAll_ReturnsAllKeys(t *testing.T) {
	entries, err := ConfigList()
	if err != nil {
		t.Fatalf("ConfigList: %v", err)
	}
	if len(entries) != len(ConfigKeys()) {
		t.Errorf("got %d entries, want %d", len(entries), len(ConfigKeys()))
	}
}
