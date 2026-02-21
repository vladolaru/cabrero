package detail

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func ptr(s string) *string { return &s }

func TestRenderDiff_UnifiedDiff(t *testing.T) {
	diff := `@@ -12,3 +12,5 @@
 ## Workflow
-Read the template before writing
+Read SKILL.md before any write tool call
+Verify template structure matches expected
+format before generating content`

	result := RenderDiff(ptr(diff), nil, "skill_improvement", 80)
	stripped := ansi.Strip(result)

	// Should contain hunk header.
	if !strings.Contains(stripped, "@@ -12,3 +12,5 @@") {
		t.Error("missing hunk header")
	}

	// Should contain additions.
	if !strings.Contains(stripped, "+Read SKILL.md") {
		t.Error("missing addition line")
	}

	// Should contain deletions.
	if !strings.Contains(stripped, "-Read the template") {
		t.Error("missing deletion line")
	}

	// Context lines should appear.
	if !strings.Contains(stripped, "## Workflow") {
		t.Error("missing context line")
	}
}

func TestRenderDiff_FlaggedEntry(t *testing.T) {
	flagged := "Always use snake_case for variable names"
	result := RenderDiff(nil, ptr(flagged), "claude_review", 80)
	stripped := ansi.Strip(result)

	if !strings.Contains(stripped, flagged) {
		t.Errorf("flagged entry not found in output: %q", stripped)
	}
}

func TestRenderDiff_Scaffold(t *testing.T) {
	content := `name: git-workflow
trigger: when working with git

## Steps
1. Check status first
2. Stage specific files`

	result := RenderDiff(ptr(content), nil, "skill_scaffold", 80)
	stripped := ansi.Strip(result)

	// All non-empty lines should contain the addition marker "+".
	// Scaffold lines are rendered as "  N + content" where N is the line number.
	lines := strings.Split(stripped, "\n")
	nonEmpty := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nonEmpty++
		// Each line should have a numeric prefix followed by "+"
		if !strings.Contains(line, "+") {
			t.Errorf("scaffold line missing '+' marker: %q", line)
		}
	}
	if nonEmpty == 0 {
		t.Error("scaffold rendered no lines")
	}
}

func TestRenderDiff_EmptyChange(t *testing.T) {
	// Nil change.
	result := RenderDiff(nil, nil, "skill_improvement", 80)
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "(no changes)") {
		t.Errorf("expected '(no changes)', got %q", stripped)
	}

	// Empty string change.
	empty := ""
	result = RenderDiff(&empty, nil, "skill_improvement", 80)
	stripped = ansi.Strip(result)
	if !strings.Contains(stripped, "(no changes)") {
		t.Errorf("expected '(no changes)', got %q", stripped)
	}
}

func TestRenderDiff_FlaggedEntryNoChange(t *testing.T) {
	// claude_review with flagged entry but no diff.
	flagged := "Use consistent naming"
	change := "some diff content"
	result := RenderDiff(ptr(change), ptr(flagged), "claude_review", 80)
	stripped := ansi.Strip(result)

	// Should show flagged entry box, not the diff.
	if !strings.Contains(stripped, flagged) {
		t.Error("claude_review should show flagged entry")
	}
}
