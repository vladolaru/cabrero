package store

import (
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/fitness"
)

func TestMergeSources(t *testing.T) {
	now := time.Now().UTC()

	persisted := []fitness.Source{
		{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate", SessionCount: 3, ClassifiedAt: &now},
		{Name: "old-removed", Origin: "user", Ownership: "not_mine", Approach: "paused", SessionCount: 1, ClassifiedAt: &now},
	}

	discovered := []fitness.Source{
		{Name: "git-workflow", Origin: "user", SessionCount: 7},                // existing: update count, keep classification
		{Name: "brainstorming", Origin: "plugin:superpowers", SessionCount: 2}, // new: add unclassified
	}

	merged := MergeSources(persisted, discovered)

	if len(merged) != 3 {
		t.Fatalf("got %d sources, want 3", len(merged))
	}

	byName := map[string]fitness.Source{}
	for _, s := range merged {
		byName[s.Name] = s
	}

	// Existing source: classification preserved, session count updated.
	gw := byName["git-workflow"]
	if gw.Ownership != "mine" {
		t.Errorf("git-workflow ownership = %q, want %q", gw.Ownership, "mine")
	}
	if gw.SessionCount != 7 {
		t.Errorf("git-workflow sessions = %d, want 7", gw.SessionCount)
	}

	// Persisted-only source: retained.
	old := byName["old-removed"]
	if old.Ownership != "not_mine" {
		t.Errorf("old-removed ownership = %q, want %q", old.Ownership, "not_mine")
	}

	// New source: added unclassified.
	bs := byName["brainstorming"]
	if bs.Ownership != "" {
		t.Errorf("brainstorming ownership = %q, want empty", bs.Ownership)
	}
	if bs.SessionCount != 2 {
		t.Errorf("brainstorming sessions = %d, want 2", bs.SessionCount)
	}
}

func TestGroupSources(t *testing.T) {
	now := time.Now().UTC()
	sources := []fitness.Source{
		{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate", ClassifiedAt: &now},
		{Name: "CLAUDE.md (cabrero)", Origin: "project:cabrero", Ownership: "mine", Approach: "iterate", ClassifiedAt: &now},
		{Name: "brainstorming", Origin: "plugin:superpowers", Ownership: "not_mine", Approach: "evaluate", ClassifiedAt: &now},
		{Name: "new-thing", Origin: "user", Ownership: "", Approach: ""},                  // unclassified
		{Name: "writing-plans", Origin: "plugin:superpowers", Ownership: "", Approach: ""}, // unclassified
	}

	groups := GroupSources(sources)

	// Expect: Unclassified (first), then classified groups.
	if len(groups) < 2 {
		t.Fatalf("got %d groups, want at least 2", len(groups))
	}

	// First group should be unclassified.
	if groups[0].Label != "Unclassified" {
		t.Errorf("first group label = %q, want %q", groups[0].Label, "Unclassified")
	}
	if len(groups[0].Sources) != 2 {
		t.Errorf("unclassified count = %d, want 2", len(groups[0].Sources))
	}

	// Verify classified sources are in their origin groups.
	found := map[string]bool{}
	for _, g := range groups[1:] {
		found[g.Label] = true
	}
	for _, want := range []string{"User-level", "Project: cabrero", "Plugin: superpowers"} {
		if !found[want] {
			t.Errorf("missing group %q", want)
		}
	}
}

func TestGroupSources_NoUnclassified(t *testing.T) {
	now := time.Now().UTC()
	sources := []fitness.Source{
		{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate", ClassifiedAt: &now},
	}

	groups := GroupSources(sources)

	for _, g := range groups {
		if g.Label == "Unclassified" {
			t.Error("unexpected Unclassified group when no unclassified sources exist")
		}
	}
}
