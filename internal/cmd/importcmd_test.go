package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func setupImportStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
}

// writeSessionFile creates a minimal JSONL fixture at srcDir/project/sessionID.jsonl.
func writeSessionFile(t *testing.T, srcDir, project, sessionID string) {
	t.Helper()
	dir := srcDir
	if project != "" {
		dir = filepath.Join(srcDir, project)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"user\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunImport(t *testing.T) {
	t.Run("nonexistent path returns error", func(t *testing.T) {
		setupImportStore(t)
		_, err := RunImport("/nonexistent/path/xyz", false, true)
		if err == nil {
			t.Error("expected error for nonexistent path")
		}
	})

	t.Run("file instead of directory returns error", func(t *testing.T) {
		setupImportStore(t)
		f := filepath.Join(t.TempDir(), "afile.jsonl")
		os.WriteFile(f, []byte("{}"), 0o644)
		_, err := RunImport(f, false, true)
		if err == nil {
			t.Error("expected error for file instead of directory")
		}
	})

	t.Run("dry run counts without writing", func(t *testing.T) {
		setupImportStore(t)
		src := t.TempDir()
		writeSessionFile(t, src, "my-project", "abcdef1234567890")

		result, err := RunImport(src, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if result.Imported != 1 {
			t.Errorf("Imported = %d, want 1", result.Imported)
		}
		if store.SessionExists("abcdef1234567890") {
			t.Error("session written during dry run")
		}
	})

	t.Run("imports valid session", func(t *testing.T) {
		setupImportStore(t)
		src := t.TempDir()
		sessionID := "validimport12345"
		writeSessionFile(t, src, "proj-a", sessionID)

		result, err := RunImport(src, false, true)
		if err != nil {
			t.Fatal(err)
		}
		if result.Imported != 1 {
			t.Errorf("Imported = %d, want 1", result.Imported)
		}
		if !store.SessionExists(sessionID) {
			t.Error("session not in store after import")
		}
		meta, err := store.ReadMetadata(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Status != "imported" {
			t.Errorf("Status = %q, want 'imported'", meta.Status)
		}
		if meta.CaptureTrigger != "imported" {
			t.Errorf("CaptureTrigger = %q, want 'imported'", meta.CaptureTrigger)
		}
	})

	t.Run("skips already-present session", func(t *testing.T) {
		setupImportStore(t)
		src := t.TempDir()
		writeSessionFile(t, src, "proj", "existingsess1234")

		RunImport(src, false, true) // first import
		result, err := RunImport(src, false, true) // second import
		if err != nil {
			t.Fatal(err)
		}
		if result.Skipped != 1 {
			t.Errorf("Skipped = %d, want 1", result.Skipped)
		}
		if result.Imported != 0 {
			t.Errorf("Imported = %d, want 0", result.Imported)
		}
	})

	t.Run("short session IDs are skipped", func(t *testing.T) {
		setupImportStore(t)
		src := t.TempDir()
		writeSessionFile(t, src, "proj", "short")           // 5 chars - skipped
		writeSessionFile(t, src, "proj", "validid123456789") // valid

		result, err := RunImport(src, false, true)
		if err != nil {
			t.Fatal(err)
		}
		if result.Imported != 1 {
			t.Errorf("Imported = %d, want 1", result.Imported)
		}
	})

	t.Run("project slug from parent directory", func(t *testing.T) {
		setupImportStore(t)
		src := t.TempDir()
		sessionID := "projslugtest1234"
		writeSessionFile(t, src, "Work-a8c-myproject", sessionID)

		RunImport(src, false, true)

		meta, err := store.ReadMetadata(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Project != "Work-a8c-myproject" {
			t.Errorf("Project = %q, want 'Work-a8c-myproject'", meta.Project)
		}
	})

	t.Run("file in root has empty project", func(t *testing.T) {
		setupImportStore(t)
		src := t.TempDir()
		sessionID := "noprojectsess123"
		writeSessionFile(t, src, "", sessionID)

		RunImport(src, false, true)

		meta, err := store.ReadMetadata(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Project != "" {
			t.Errorf("Project = %q, want empty", meta.Project)
		}
	})
}
