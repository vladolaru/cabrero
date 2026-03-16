package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestIgnore_List_Empty(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := ignoreRun([]string{"list"}, &buf)
	if err != nil {
		t.Fatalf("ignore list: %v", err)
	}
	if !strings.Contains(buf.String(), "No ignored") {
		t.Errorf("expected 'No ignored' message, got: %s", buf.String())
	}
}

func TestIgnore_AddAndList(t *testing.T) {
	setupConfigTest(t)

	var buf bytes.Buffer
	err := ignoreRun([]string{"add", "CodexBar"}, &buf)
	if err != nil {
		t.Fatalf("ignore add: %v", err)
	}
	if !strings.Contains(buf.String(), "CodexBar") {
		t.Error("expected pattern in add output")
	}

	buf.Reset()
	err = ignoreRun([]string{"list"}, &buf)
	if err != nil {
		t.Fatalf("ignore list: %v", err)
	}
	if !strings.Contains(buf.String(), "CodexBar") {
		t.Error("expected pattern in list output")
	}
}

func TestIgnore_Add_Duplicate(t *testing.T) {
	setupConfigTest(t)

	var buf bytes.Buffer
	ignoreRun([]string{"add", "CodexBar"}, &buf)

	buf.Reset()
	err := ignoreRun([]string{"add", "CodexBar"}, &buf)
	if err != nil {
		t.Fatalf("ignore add duplicate: %v", err)
	}
	if !strings.Contains(buf.String(), "already ignored") {
		t.Errorf("expected 'already ignored' message, got: %s", buf.String())
	}
}

func TestIgnore_Remove(t *testing.T) {
	setupConfigTest(t)

	var buf bytes.Buffer
	ignoreRun([]string{"add", "CodexBar"}, &buf)

	buf.Reset()
	err := ignoreRun([]string{"remove", "CodexBar"}, &buf)
	if err != nil {
		t.Fatalf("ignore remove: %v", err)
	}
	if !strings.Contains(buf.String(), "Removed") {
		t.Errorf("expected 'Removed' message, got: %s", buf.String())
	}
}

func TestIgnore_Remove_NotFound(t *testing.T) {
	setupConfigTest(t)

	var buf bytes.Buffer
	err := ignoreRun([]string{"remove", "nonexistent"}, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "not in the ignore list") {
		t.Errorf("expected 'not in the ignore list' message, got: %s", buf.String())
	}
}

func TestIgnore_Clean_DryRun(t *testing.T) {
	setupConfigTest(t)

	// Create a session matching the pattern.
	root := store.Root()
	dir := filepath.Join(root, "raw", "sess-codex-1234")
	os.MkdirAll(dir, 0o755)
	meta := `{"session_id":"sess-codex-1234","project":"-Users-vlad-CodexBar-Probe","status":"capture_failed"}`
	os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(meta), 0o644)

	store.AddIgnoredPattern("CodexBar")

	var buf bytes.Buffer
	err := ignoreRun([]string{"clean", "--dry-run"}, &buf)
	if err != nil {
		t.Fatalf("ignore clean --dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "Would remove") {
		t.Errorf("expected dry-run output, got: %s", buf.String())
	}
	// Session should still exist.
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("session should not have been deleted in dry-run mode")
	}
}

func TestIgnore_Clean(t *testing.T) {
	setupConfigTest(t)

	root := store.Root()
	dir := filepath.Join(root, "raw", "sess-codex-5678")
	os.MkdirAll(dir, 0o755)
	meta := `{"session_id":"sess-codex-5678","project":"-Users-vlad-CodexBar-Probe","status":"capture_failed"}`
	os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(meta), 0o644)

	store.AddIgnoredPattern("CodexBar")

	var buf bytes.Buffer
	err := ignoreRun([]string{"clean"}, &buf)
	if err != nil {
		t.Fatalf("ignore clean: %v", err)
	}
	if !strings.Contains(buf.String(), "Removed 1") {
		t.Errorf("expected 'Removed 1' message, got: %s", buf.String())
	}
	// Session should be gone.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("session should have been deleted")
	}
}

func TestIgnore_NoSubcommand(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := ignoreRun(nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Usage") {
		t.Error("expected usage help")
	}
}

func TestIgnore_Add_MissingArg(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := ignoreRun([]string{"add"}, &buf)
	if err == nil {
		t.Error("expected error for missing pattern argument")
	}
}

func TestIgnore_Remove_MissingArg(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := ignoreRun([]string{"remove"}, &buf)
	if err == nil {
		t.Error("expected error for missing pattern argument")
	}
}

func TestIgnore_UnknownSubcommand(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := ignoreRun([]string{"bogus"}, &buf)
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}
}
