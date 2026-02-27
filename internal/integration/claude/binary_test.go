package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestDetermineBinaryPath_PrefersInstalledBinary(t *testing.T) {
	tmpDir := t.TempDir()
	old := store.RootOverrideForTest(tmpDir)
	t.Cleanup(func() { store.ResetRootOverrideForTest(old) })

	binDir := filepath.Join(tmpDir, "bin")
	os.MkdirAll(binDir, 0o755)
	binPath := filepath.Join(binDir, "cabrero")
	os.WriteFile(binPath, []byte("fake"), 0o755)

	got := DetermineBinaryPath()
	if got != binPath {
		t.Errorf("DetermineBinaryPath = %q, want %q", got, binPath)
	}
}

func TestDetermineBinaryPath_FallsBackToExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	old := store.RootOverrideForTest(tmpDir)
	t.Cleanup(func() { store.ResetRootOverrideForTest(old) })

	// No bin/cabrero exists — should fall back.
	got := DetermineBinaryPath()
	if got == "" {
		t.Error("DetermineBinaryPath returned empty string")
	}
	// We can't assert the exact path since it depends on test runner,
	// but it should not be the non-existent preferred path.
	preferred := filepath.Join(tmpDir, "bin", "cabrero")
	if got == preferred {
		t.Error("should not return preferred path when it doesn't exist")
	}
}
