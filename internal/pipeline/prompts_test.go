package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestEnsureCuratorPrompts(t *testing.T) {
	dir := t.TempDir()
	origRoot := store.RootOverrideForTest(dir)
	defer store.ResetRootOverrideForTest(origRoot)

	if err := EnsureCuratorPrompts(); err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{curatorPromptFile, curatorCheckPromptFile} {
		path := filepath.Join(dir, "prompts", f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected prompt file %s to exist: %v", f, err)
		}
	}
}
