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

func TestMergeSources_CrossOriginCollision(t *testing.T) {
	now := time.Now().UTC()

	// Two persisted sources with the same name but different origins.
	persisted := []fitness.Source{
		{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate", SessionCount: 3, ClassifiedAt: &now},
		{Name: "git-workflow", Origin: "project:cabrero", Ownership: "not_mine", Approach: "evaluate", SessionCount: 1, ClassifiedAt: &now},
	}

	// Discovery finds updated counts for both.
	discovered := []fitness.Source{
		{Name: "git-workflow", Origin: "user", SessionCount: 10},
		{Name: "git-workflow", Origin: "project:cabrero", SessionCount: 5},
	}

	merged := MergeSources(persisted, discovered)

	// Both sources must survive — no collision.
	if len(merged) != 2 {
		t.Fatalf("got %d sources, want 2", len(merged))
	}

	// Build lookup by (name, origin) to verify independence.
	type key struct{ name, origin string }
	byKey := map[key]fitness.Source{}
	for _, s := range merged {
		byKey[key{s.Name, s.Origin}] = s
	}

	// User-level source: classification preserved, count updated.
	user := byKey[key{"git-workflow", "user"}]
	if user.Ownership != "mine" {
		t.Errorf("user source ownership = %q, want %q", user.Ownership, "mine")
	}
	if user.SessionCount != 10 {
		t.Errorf("user source sessions = %d, want 10", user.SessionCount)
	}

	// Project-level source: classification preserved independently, count updated.
	proj := byKey[key{"git-workflow", "project:cabrero"}]
	if proj.Ownership != "not_mine" {
		t.Errorf("project source ownership = %q, want %q", proj.Ownership, "not_mine")
	}
	if proj.SessionCount != 5 {
		t.Errorf("project source sessions = %d, want 5", proj.SessionCount)
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
