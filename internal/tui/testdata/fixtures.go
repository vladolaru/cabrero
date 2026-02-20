// Package testdata provides factory functions for test data used across TUI tests.
package testdata

import (
	"time"

	"github.com/vladolaru/cabrero/internal/fitness"
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

// TestFitnessReport returns a fitness report with sensible defaults.
func TestFitnessReport(overrides ...func(*fitness.Report)) *fitness.Report {
	t := time.Now().Add(-2 * time.Hour)
	r := &fitness.Report{
		ID:            "fit-001",
		SourceName:    "docx-helper",
		SourceOrigin:  "plugin:some-third-party",
		Ownership:     "not_mine",
		ObservedCount: 14,
		WindowDays:    30,
		Assessment: fitness.Assessment{
			Followed:     fitness.BucketStat{Count: 5, Percent: 36},
			WorkedAround: fitness.BucketStat{Count: 6, Percent: 43},
			Confused:     fitness.BucketStat{Count: 3, Percent: 21},
		},
		Verdict: "This skill is frequently worked around. Agents override its template logic in 43% of sessions, suggesting the skill's approach conflicts with actual usage patterns.",
		Evidence: []fitness.EvidenceGroup{
			{
				Category: "followed",
				Entries: []fitness.EvidenceEntry{
					{SessionID: "sess-f1", Timestamp: t.Add(-24 * time.Hour), Summary: "Used template correctly for report generation", Detail: "Agent read SKILL.md first, then generated report matching template structure."},
					{SessionID: "sess-f2", Timestamp: t.Add(-48 * time.Hour), Summary: "Template applied without modification", Detail: "Straightforward document creation following skill workflow."},
				},
			},
			{
				Category: "worked_around",
				Entries: []fitness.EvidenceEntry{
					{SessionID: "sess-w1", Timestamp: t.Add(-12 * time.Hour), Summary: "Skipped template, used direct formatting", Detail: "Agent wrote content first, then attempted to retrofit template structure."},
					{SessionID: "sess-w2", Timestamp: t.Add(-36 * time.Hour), Summary: "Modified template before use", Detail: "Agent altered template headers to match project conventions."},
					{SessionID: "sess-w3", Timestamp: t.Add(-72 * time.Hour), Summary: "Ignored template for small documents", Detail: "Agent deemed template overhead unnecessary for short files."},
				},
			},
			{
				Category: "confused",
				Entries: []fitness.EvidenceEntry{
					{SessionID: "sess-c1", Timestamp: t.Add(-6 * time.Hour), Summary: "Three failed write attempts before reading skill", Detail: "Agent tried to write report.docx three times without reading the skill first, each time producing malformed output."},
				},
			},
		},
		GeneratedAt: t,
	}
	for _, fn := range overrides {
		fn(r)
	}
	return r
}

// TestSource returns a single source with sensible defaults.
func TestSource(overrides ...func(*fitness.Source)) fitness.Source {
	t := time.Now().Add(-7 * 24 * time.Hour)
	s := fitness.Source{
		Name:         "docx-helper",
		Origin:       "plugin:some-third-party",
		Ownership:    "not_mine",
		Approach:     "evaluate",
		SessionCount: 14,
		HealthScore:  36,
		ClassifiedAt: &t,
	}
	for _, fn := range overrides {
		fn(&s)
	}
	return s
}

// TestSourceGroups returns source groups for testing.
func TestSourceGroups() []fitness.SourceGroup {
	t1 := time.Now().Add(-30 * 24 * time.Hour)
	t2 := time.Now().Add(-14 * 24 * time.Hour)
	t3 := time.Now().Add(-7 * 24 * time.Hour)

	return []fitness.SourceGroup{
		{
			Label:  "User-level",
			Origin: "user",
			Sources: []fitness.Source{
				{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate", SessionCount: 22, HealthScore: 85, ClassifiedAt: &t1},
				{Name: "code-review-checklist", Origin: "user", Ownership: "mine", Approach: "iterate", SessionCount: 15, HealthScore: 72, ClassifiedAt: &t1},
				{Name: "debugging-steps", Origin: "user", Ownership: "mine", Approach: "paused", SessionCount: 3, HealthScore: 50, ClassifiedAt: &t2},
			},
		},
		{
			Label:  "Project: woo-payments",
			Origin: "project:woo-payments",
			Sources: []fitness.Source{
				{Name: "payment-gateway-testing", Origin: "project:woo-payments", Ownership: "mine", Approach: "iterate", SessionCount: 18, HealthScore: 90, ClassifiedAt: &t2},
				{Name: "stripe-api-patterns", Origin: "project:woo-payments", Ownership: "not_mine", Approach: "evaluate", SessionCount: 8, HealthScore: 62, ClassifiedAt: &t3},
			},
		},
		{
			Label:  "Plugin: some-third-party",
			Origin: "plugin:some-third-party",
			Sources: []fitness.Source{
				{Name: "docx-helper", Origin: "plugin:some-third-party", Ownership: "not_mine", Approach: "evaluate", SessionCount: 14, HealthScore: 36, ClassifiedAt: &t3},
				{Name: "csv-importer", Origin: "plugin:some-third-party", Ownership: "not_mine", Approach: "evaluate", SessionCount: 6, HealthScore: 78, ClassifiedAt: &t3},
			},
		},
		{
			Label:  "\u26a0 Unclassified",
			Origin: "",
			Sources: []fitness.Source{
				{Name: "mystery-skill", Origin: "user", Ownership: "", Approach: "", SessionCount: 2, HealthScore: -1},
				{Name: "new-plugin-thing", Origin: "plugin:unknown", Ownership: "", Approach: "", SessionCount: 1, HealthScore: -1},
			},
		},
	}
}

// TestChangeEntries returns change history entries for testing.
func TestChangeEntries() []fitness.ChangeEntry {
	now := time.Now()
	return []fitness.ChangeEntry{
		{
			ID:              "chg-001",
			SourceName:      "docx-helper",
			ProposalID:      "prop-abc123",
			Description:     "Updated template read-before-write workflow",
			Timestamp:       now.Add(-24 * time.Hour),
			Status:          "approved",
			PreviousContent: "Read the template before writing",
			FilePath:        "~/.claude/skills/docx-helper/SKILL.md",
		},
		{
			ID:          "chg-002",
			SourceName:  "docx-helper",
			ProposalID:  "prop-def456",
			Description: "Suggested removing template validation step",
			Timestamp:   now.Add(-72 * time.Hour),
			Status:      "approved",
			FilePath:    "~/.claude/skills/docx-helper/SKILL.md",
		},
		{
			ID:          "chg-003",
			SourceName:  "docx-helper",
			ProposalID:  "prop-ghi789",
			Description: "Proposed switching to JSON templates",
			Timestamp:   now.Add(-120 * time.Hour),
			Status:      "rejected",
			FilePath:    "~/.claude/skills/docx-helper/SKILL.md",
		},
	}
}
