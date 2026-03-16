package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestScanQueued_SkipsIgnoredProjects(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Create a queued session with transcript for an ignored project.
	dir := filepath.Join(tmp, ".cabrero", "raw", "ignored-sess-1234")
	os.MkdirAll(dir, 0o755)
	meta := `{"session_id":"ignored-sess-1234","timestamp":"2026-03-16T10:00:00Z","project":"-Users-vlad-CodexBar-Probe","status":"queued"}`
	os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(meta), 0o644)
	os.WriteFile(filepath.Join(dir, "transcript.jsonl"), []byte(`{"type":"test"}`+"\n"), 0o644)

	store.AddIgnoredPattern("CodexBar")

	ready, err := ScanQueued()
	if err != nil {
		t.Fatalf("ScanQueued: %v", err)
	}
	for _, s := range ready {
		if s.SessionID == "ignored-sess-1234" {
			t.Error("ignored session should not appear in ScanQueued results")
		}
	}
}
