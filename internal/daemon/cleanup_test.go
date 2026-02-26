package daemon

import (
	"testing"

	"github.com/vladolaru/cabrero/internal/pipeline"
)

func TestGroupProposalsByTarget(t *testing.T) {
	proposals := []pipeline.ProposalWithSession{
		{SessionID: "s1", Proposal: pipeline.Proposal{ID: "p1", Type: "claude_addition", Target: "/a/CLAUDE.md"}},
		{SessionID: "s2", Proposal: pipeline.Proposal{ID: "p2", Type: "claude_addition", Target: "/a/CLAUDE.md"}},
		{SessionID: "s3", Proposal: pipeline.Proposal{ID: "p3", Type: "skill_scaffold", Target: "/b/SKILL.md"}},
		{SessionID: "s4", Proposal: pipeline.Proposal{ID: "p4", Type: "skill_improvement", Target: "/c/skill.md"}},
	}

	multi, single := groupProposalsByTarget(proposals)

	if len(multi) != 1 {
		t.Errorf("multi: got %d targets, want 1", len(multi))
	}
	if len(multi["/a/CLAUDE.md"]) != 2 {
		t.Errorf("multi[/a/CLAUDE.md]: got %d, want 2", len(multi["/a/CLAUDE.md"]))
	}
	// Scaffolds always skip cleanup — should appear in single.
	// /b/SKILL.md has 1 proposal (scaffold) — kept in single.
	// /c/skill.md has 1 proposal (skill_improvement) — kept in single.
	if len(single) != 2 {
		t.Errorf("single: got %d, want 2", len(single))
	}
}

func TestSkipNonFileTargets(t *testing.T) {
	proposals := []pipeline.ProposalWithSession{
		{Proposal: pipeline.Proposal{ID: "p1", Type: "skill_improvement", Target: "local-environment"}},
		{Proposal: pipeline.Proposal{ID: "p2", Type: "claude_addition", Target: "/a/CLAUDE.md"}},
	}
	_, single := groupProposalsByTarget(proposals)
	// "local-environment" is not a file target — should be excluded from single-check list.
	for _, pw := range single {
		if !pipeline.IsFileTarget(pw.Proposal.Target) {
			t.Errorf("non-file target %q should not appear in single list", pw.Proposal.Target)
		}
	}
}
