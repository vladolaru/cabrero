// Package testdata provides factory functions for test data used across TUI tests.
package testdata

import (
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

func ptr(s string) *string { return &s }

// TestProposal returns a skill_improvement proposal with sensible defaults.
// Use overrides to customize specific fields.
func TestProposal(overrides ...func(*pipeline.Proposal)) pipeline.ProposalWithSession {
	p := pipeline.Proposal{
		ID:         "prop-abc123",
		Type:       "skill_improvement",
		Confidence: "high",
		Target:     "~/.claude/skills/docx-helper/SKILL.md",
		Change: ptr(`@@ -12,3 +12,5 @@
 ## Workflow
-Read the template before writing
+Read SKILL.md before any write tool call
+Verify template structure matches expected
+format before generating content`),
		Rationale:            "Skill was read after 3 write attempts. The writes failed because the template structure was not understood before generation.",
		CitedUUIDs:           []string{"uuid-turn-9", "uuid-turn-12", "uuid-turn-15", "uuid-turn-18", "uuid-turn-19"},
		CitedSkillSignals:    []string{"docx-helper: read at turn 18, first write at turn 9"},
		CitedClaudeMdSignals: nil,
	}
	for _, fn := range overrides {
		fn(&p)
	}
	return pipeline.ProposalWithSession{
		SessionID: "abc123def456",
		Proposal:  p,
	}
}

// TestProposalSkillImprovement is an alias for TestProposal with no overrides.
func TestProposalSkillImprovement() pipeline.ProposalWithSession {
	return TestProposal()
}

// TestProposalClaudeReview returns a claude_review proposal.
func TestProposalClaudeReview() pipeline.ProposalWithSession {
	return pipeline.ProposalWithSession{
		SessionID: "review-session-789",
		Proposal: pipeline.Proposal{
			ID:                "prop-review-001",
			Type:              "claude_review",
			Confidence:        "medium",
			Target:            "CLAUDE.md (woo-payments)",
			FlaggedEntry:      ptr("Always use snake_case for variable names in PHP hooks"),
			AssessmentSummary: ptr("Entry contradicts WordPress coding standards which use camelCase for function names"),
			Rationale:         "The flagged entry was worked around in 3 of 5 sessions. Agents consistently used camelCase despite this instruction.",
			CitedUUIDs:        []string{"uuid-cr-1", "uuid-cr-2", "uuid-cr-3"},
		},
	}
}

// TestProposalSkillScaffold returns a skill_scaffold proposal.
func TestProposalSkillScaffold() pipeline.ProposalWithSession {
	return pipeline.ProposalWithSession{
		SessionID: "scaffold-session-456",
		Proposal: pipeline.Proposal{
			ID:         "prop-scaffold-001",
			Type:       "skill_scaffold",
			Confidence: "high",
			Target:     "~/.claude/skills/git-workflow/SKILL.md",
			Change: ptr(`name: git-workflow
trigger: when working with git branches and commits

## Steps
1. Check status before any operation
2. Stage specific files, never use git add -A
3. Write descriptive commit messages
4. Push only the current branch explicitly`),
			Rationale:         "Pattern observed: user corrected git workflow in 4 sessions. A dedicated skill would prevent repeated corrections.",
			ScaffoldSkillName: ptr("git-workflow"),
			ScaffoldTrigger:   ptr("when working with git branches and commits"),
			CitedUUIDs:        []string{"uuid-sc-1", "uuid-sc-2", "uuid-sc-3", "uuid-sc-4"},
		},
	}
}

// TestDashboardStats returns realistic dashboard statistics.
func TestDashboardStats() message.DashboardStats {
	t := time.Now().Add(-12 * time.Minute)
	return message.DashboardStats{
		PendingCount:  3,
		ApprovedCount: 7,
		RejectedCount: 2,

		DaemonRunning: true,
		DaemonPID:     4821,

		HookPreCompact: true,
		HookSessionEnd: true,

		LastCaptureTime: &t,
	}
}

// TestDashboardStatsEmpty returns stats with no activity.
func TestDashboardStatsEmpty() message.DashboardStats {
	return message.DashboardStats{}
}

// TestConfig returns a default config with optional overrides.
func TestConfig(overrides ...func(*shared.Config)) *shared.Config {
	cfg := shared.DefaultConfig()
	for _, fn := range overrides {
		fn(cfg)
	}
	return cfg
}

// TestCitations returns a set of citation entries for testing.
func TestCitations() []shared.CitationEntry {
	return []shared.CitationEntry{
		{
			UUID:    "uuid-turn-9",
			Summary: "[1] Turn 9:  tool_use write -> report.docx",
			RawJSON: `{"type":"tool_use","tool":"Write","target":"report.docx","turn":9}`,
		},
		{
			UUID:    "uuid-turn-12",
			Summary: "[2] Turn 12: tool_use write -> report.docx",
			RawJSON: `{"type":"tool_use","tool":"Write","target":"report.docx","turn":12}`,
		},
		{
			UUID:    "uuid-turn-15",
			Summary: "[3] Turn 15: tool_use write -> report.docx",
			RawJSON: `{"type":"tool_use","tool":"Write","target":"report.docx","turn":15}`,
		},
		{
			UUID:    "uuid-turn-18",
			Summary: "[4] Turn 18: tool_use view -> SKILL.md",
			RawJSON: `{"type":"tool_use","tool":"Read","target":"SKILL.md","turn":18}`,
		},
		{
			UUID:    "uuid-turn-19",
			Summary: "[5] Turn 19: tool_use write -> report.docx OK",
			RawJSON: `{"type":"tool_use","tool":"Write","target":"report.docx","turn":19,"success":true}`,
		},
	}
}

// TestProposals returns a mixed set of proposals for dashboard testing.
func TestProposals() []pipeline.ProposalWithSession {
	return []pipeline.ProposalWithSession{
		TestProposalSkillImprovement(),
		TestProposalSkillScaffold(),
		TestProposalClaudeReview(),
	}
}
