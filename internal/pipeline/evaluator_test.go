package pipeline

import (
	"strings"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestEvaluatorAllowedToolsMulti_UnionOfCwds(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	cwd1 := "/projects/alpha"
	cwd2 := "/projects/beta"
	cwds := []*string{&cwd1, &cwd2}

	result := evaluatorAllowedToolsMulti(cwds)

	if !strings.Contains(result, "Read(///projects/alpha/**)") {
		t.Errorf("expected first cwd in allowedTools, got: %s", result)
	}
	if !strings.Contains(result, "Read(///projects/beta/**)") {
		t.Errorf("expected second cwd in allowedTools, got: %s", result)
	}
	if !strings.Contains(result, "Grep(///projects/alpha/**)") {
		t.Errorf("expected first cwd grep in allowedTools, got: %s", result)
	}
	if !strings.Contains(result, "Grep(///projects/beta/**)") {
		t.Errorf("expected second cwd grep in allowedTools, got: %s", result)
	}
}

func TestEvaluatorAllowedToolsMulti_DeduplicatesSameCwd(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	cwd := "/projects/same"
	cwds := []*string{&cwd, &cwd}

	result := evaluatorAllowedToolsMulti(cwds)

	// Count occurrences of the cwd Read path — should be exactly 1.
	readPath := "Read(///projects/same/**)"
	count := strings.Count(result, readPath)
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of %q, got %d in: %s", readPath, count, result)
	}
}

func TestEvaluatorAllowedToolsMulti_SkipsNilAndEmpty(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	cwd := "/projects/valid"
	empty := ""
	cwds := []*string{nil, &empty, &cwd}

	result := evaluatorAllowedToolsMulti(cwds)

	if !strings.Contains(result, "Read(///projects/valid/**)") {
		t.Errorf("expected valid cwd in allowedTools, got: %s", result)
	}
	// Should not contain empty path entries.
	if strings.Contains(result, "Read(///**)") {
		t.Errorf("should not contain empty cwd path, got: %s", result)
	}
}
