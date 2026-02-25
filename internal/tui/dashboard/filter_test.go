package dashboard

import (
	"testing"
)

func TestDashboardFilter_FreeText(t *testing.T) {
	targets := []string{
		"type:skill_improvement target:~/.claude/SKILL.md confidence:high",
		"type:skill_scaffold target:~/Work/project confidence:medium",
		"type:fitness_report target:docx-helper confidence:85% health",
	}

	ranks := dashboardFilter("skill", targets)
	if len(ranks) != 2 {
		t.Errorf("'skill' should match 2 items, got %d", len(ranks))
	}
}

func TestDashboardFilter_TypePrefix(t *testing.T) {
	targets := []string{
		"type:skill_improvement target:path1 confidence:high",
		"type:skill_scaffold target:path2 confidence:medium",
		"type:fitness_report target:docx-helper confidence:low",
	}

	ranks := dashboardFilter("type:skill", targets)
	if len(ranks) != 2 {
		t.Errorf("type:skill should match 2 items, got %d", len(ranks))
	}
	for _, r := range ranks {
		if r.Index == 2 {
			t.Error("fitness_report should not match type:skill")
		}
	}
}

func TestDashboardFilter_TargetPrefix(t *testing.T) {
	targets := []string{
		"type:skill_improvement target:~/.claude/docx-helper confidence:high",
		"type:skill_scaffold target:~/Work/project confidence:medium",
	}

	ranks := dashboardFilter("target:docx", targets)
	if len(ranks) != 1 || ranks[0].Index != 0 {
		t.Errorf("target:docx should match item 0 only, got %v", ranks)
	}
}

func TestDashboardFilter_CaseInsensitive(t *testing.T) {
	targets := []string{
		"type:SKILL_IMPROVEMENT target:path confidence:HIGH",
	}
	if len(dashboardFilter("skill_improvement", targets)) != 1 {
		t.Error("filter should be case-insensitive")
	}
	if len(dashboardFilter("SKILL_IMPROVEMENT", targets)) != 1 {
		t.Error("filter should be case-insensitive for uppercase query")
	}
}

func TestDashboardFilter_EmptyTerm_ReturnsAll(t *testing.T) {
	targets := []string{"a", "b", "c"}
	ranks := dashboardFilter("", targets)
	if len(ranks) != 3 {
		t.Errorf("empty filter should return all %d items, got %d", len(targets), len(ranks))
	}
}

func TestDashboardFilter_NoMatch_ReturnsEmpty(t *testing.T) {
	targets := []string{
		"type:skill_improvement target:path confidence:high",
	}
	if len(dashboardFilter("xyzzy_no_match_12345", targets)) != 0 {
		t.Error("unmatched filter should return empty")
	}
}
