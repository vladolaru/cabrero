package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/fitness"
)

func TestDiscoverSourcesFromEvaluations(t *testing.T) {
	setupTestStore(t)

	evalDir := filepath.Join(Root(), "evaluations")

	// Write two classifier outputs.
	c1 := classifierSignals{
		SkillSignals: []classifierSkillSignal{
			{SkillName: "superpowers:brainstorming"},
			{SkillName: "using-ghe"},
		},
		ClaudeMdSignals: []classifierClaudeMdSignal{
			{Path: "/tmp/test-home/.claude/CLAUDE.md"},
		},
	}
	c2 := classifierSignals{
		SkillSignals: []classifierSkillSignal{
			{SkillName: "superpowers:brainstorming"}, // duplicate skill
		},
		ClaudeMdSignals: []classifierClaudeMdSignal{
			{Path: "/tmp/test-home/Work/myproject/CLAUDE.md"},
			{Path: "/tmp/test-home/.claude/CLAUDE.md"}, // duplicate path
		},
	}

	writeClassifier(t, evalDir, "sess-001", c1)
	writeClassifier(t, evalDir, "sess-002", c2)

	sources, err := DiscoverSourcesFromEvaluations()
	if err != nil {
		t.Fatalf("DiscoverSourcesFromEvaluations: %v", err)
	}

	// Expect 4 unique sources: brainstorming (plugin:superpowers, 2 sessions),
	// using-ghe (user, 1), CLAUDE.md (.claude) (project:.claude, 2),
	// CLAUDE.md (myproject) (project:myproject, 1).
	if len(sources) != 4 {
		t.Fatalf("got %d sources, want 4; sources: %v", len(sources), sourceNames(sources))
	}

	byName := map[string]int{}
	for _, s := range sources {
		byName[s.Name] = s.SessionCount
	}

	if byName["brainstorming"] != 2 {
		t.Errorf("brainstorming sessions = %d, want 2", byName["brainstorming"])
	}
	if byName["using-ghe"] != 1 {
		t.Errorf("using-ghe sessions = %d, want 1", byName["using-ghe"])
	}
}

func TestDiscoverSourcesFromEvaluations_Empty(t *testing.T) {
	setupTestStore(t)

	sources, err := DiscoverSourcesFromEvaluations()
	if err != nil {
		t.Fatalf("DiscoverSourcesFromEvaluations: %v", err)
	}
	if sources == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(sources) != 0 {
		t.Errorf("got %d sources, want 0", len(sources))
	}
}

func writeClassifier(t *testing.T, dir, sessionID string, c classifierSignals) {
	t.Helper()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sessionID+"-classifier.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sourceNames(sources []fitness.Source) []string {
	var names []string
	for _, s := range sources {
		names = append(names, s.Name)
	}
	return names
}

// TestClassifierSignalsLocalSchema documents the JSON contract between store's
// local types and pipeline.ClassifierOutput. If JSON tags in
// classifierSignals/classifierSkillSignal/classifierClaudeMdSignal are
// renamed, this test will fail.
func TestClassifierSignalsLocalSchema(t *testing.T) {
	const fixture = `{
		"skillSignals": [{"skillName": "my-skill"}],
		"claudeMdSignals": [{"path": "/home/user/CLAUDE.md"}]
	}`

	var out classifierSignals
	if err := json.Unmarshal([]byte(fixture), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.SkillSignals) != 1 {
		t.Fatalf("SkillSignals: got %d items, want 1", len(out.SkillSignals))
	}
	if out.SkillSignals[0].SkillName != "my-skill" {
		t.Errorf("SkillName tag mismatch: got %q, want %q", out.SkillSignals[0].SkillName, "my-skill")
	}
	if len(out.ClaudeMdSignals) != 1 {
		t.Fatalf("ClaudeMdSignals: got %d items, want 1", len(out.ClaudeMdSignals))
	}
	if out.ClaudeMdSignals[0].Path != "/home/user/CLAUDE.md" {
		t.Errorf("Path tag mismatch: got %q, want %q", out.ClaudeMdSignals[0].Path, "/home/user/CLAUDE.md")
	}
}
