package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func setupConfigTest(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	os.MkdirAll(root, 0o755)
	old := store.RootOverrideForTest(root)
	t.Cleanup(func() { store.ResetRootOverrideForTest(old) })
}

func captureConfigOutput(t *testing.T, args []string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := configRun(args, &buf)
	return buf.String(), err
}

func TestConfig_Get_Default(t *testing.T) {
	setupConfigTest(t)
	out, err := captureConfigOutput(t, []string{"get", "evaluator.model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "claude-sonnet-4-6" {
		t.Errorf("got %q, want claude-sonnet-4-6", strings.TrimSpace(out))
	}
}

func TestConfig_Set_And_Get(t *testing.T) {
	setupConfigTest(t)
	if _, err := captureConfigOutput(t, []string{"set", "evaluator.model", "claude-opus-4-6"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	out, err := captureConfigOutput(t, []string{"get", "evaluator.model"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(out) != "claude-opus-4-6" {
		t.Errorf("got %q, want claude-opus-4-6", strings.TrimSpace(out))
	}
}

func TestConfig_Unset(t *testing.T) {
	setupConfigTest(t)
	captureConfigOutput(t, []string{"set", "debug", "true"})
	captureConfigOutput(t, []string{"unset", "debug"})
	out, _ := captureConfigOutput(t, []string{"get", "debug"})
	if strings.TrimSpace(out) != "false" {
		t.Errorf("after unset: got %q, want false", strings.TrimSpace(out))
	}
}

func TestConfig_List(t *testing.T) {
	setupConfigTest(t)
	out, err := captureConfigOutput(t, []string{"list"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, key := range []string{"debug", "classifier.model", "evaluator.model",
		"classifier.timeout", "evaluator.timeout",
		"circuit-breaker.threshold", "circuit-breaker.cooldown"} {
		if !strings.Contains(out, key) {
			t.Errorf("list output missing key %q", key)
		}
	}
}

func TestConfig_List_ShowsDefaults(t *testing.T) {
	setupConfigTest(t)
	out, err := captureConfigOutput(t, []string{"list", "--defaults"})
	if err != nil {
		t.Fatalf("list --defaults: %v", err)
	}
	if !strings.Contains(out, "(default)") {
		t.Error("expected (default) annotation in output")
	}
}

func TestConfig_UnknownKey(t *testing.T) {
	setupConfigTest(t)
	_, err := captureConfigOutput(t, []string{"get", "nonexistent.key"})
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestConfig_NoSubcommand(t *testing.T) {
	setupConfigTest(t)
	out, err := captureConfigOutput(t, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Usage") {
		t.Error("expected usage help in output")
	}
}

func TestConfig_Get_MissingArg(t *testing.T) {
	setupConfigTest(t)
	_, err := captureConfigOutput(t, []string{"get"})
	if err == nil {
		t.Error("expected error for missing key argument")
	}
}
