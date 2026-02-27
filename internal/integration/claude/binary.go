package claude

import (
	"os"
	"path/filepath"

	"github.com/vladolaru/cabrero/internal/store"
)

// DetermineBinaryPath resolves the cabrero binary path.
// Prefers ~/.cabrero/bin/cabrero, falls back to the current executable
// with symlink resolution.
func DetermineBinaryPath() string {
	binaryPath := filepath.Join(store.Root(), "bin", "cabrero")
	if _, err := os.Stat(binaryPath); err == nil {
		return binaryPath
	}

	exe, err := os.Executable()
	if err != nil {
		return binaryPath
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err == nil {
		return resolved
	}
	return exe
}
