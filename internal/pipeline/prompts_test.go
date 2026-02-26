package pipeline

import (
	"os"
	"path/filepath"
	"strings"
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

func TestEnsureMetaPrompts_CreatesMetaV1(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	if err := EnsureMetaPrompts(); err != nil {
		t.Fatalf("EnsureMetaPrompts: %v", err)
	}
	path := filepath.Join(tmp, "prompts", "meta-v1.txt")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("meta-v1.txt not created at %s", path)
	}
	data, _ := os.ReadFile(path)
	// Verify it contains key instruction phrases.
	if !strings.Contains(string(data), "prompt_improvement") {
		t.Error("meta prompt should mention prompt_improvement")
	}
}
