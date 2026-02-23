package store

import "testing"

func TestInferOrigin(t *testing.T) {
	tests := []struct {
		input      string
		wantName   string
		wantOrigin string
	}{
		// Colon-namespaced -> plugin.
		{"superpowers:brainstorming", "brainstorming", "plugin:superpowers"},
		{"pirategoat-tools:full-code-review", "full-code-review", "plugin:pirategoat-tools"},

		// Bare name -> user.
		{"brainstorming", "brainstorming", "user"},
		{"using-ghe", "using-ghe", "user"},
		{"write-like-a-pirategoat", "write-like-a-pirategoat", "user"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, origin := InferOrigin(tt.input)
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if origin != tt.wantOrigin {
				t.Errorf("origin = %q, want %q", origin, tt.wantOrigin)
			}
		})
	}
}

func TestInferOriginFromPath(t *testing.T) {
	home := "/Users/vlad"

	tests := []struct {
		path       string
		wantName   string
		wantOrigin string
	}{
		// User-level CLAUDE.md.
		{home + "/.claude/CLAUDE.md", "CLAUDE.md", "user"},

		// Project-level CLAUDE.md.
		{home + "/Work/a8c/cabrero/CLAUDE.md", "CLAUDE.md (cabrero)", "project:cabrero"},
		{home + "/Work/a8c/woo-payments/CLAUDE.md", "CLAUDE.md (woo-payments)", "project:woo-payments"},

		// User-level skill (flat file).
		{home + "/.claude/skills/using-ghe.md", "using-ghe", "user"},

		// User-level skill (directory with SKILL.md).
		{home + "/.claude/skills/git-workflow/SKILL.md", "git-workflow", "user"},

		// Plugin skill.
		{home + "/.claude/plugins/cache/superpowers-marketplace/superpowers/4.3.1/skills/writing-plans/SKILL.md", "writing-plans", "plugin:superpowers"},
		{home + "/.claude/plugins/cache/pirategoat-marketplace/pirategoat-tools/1.0.0/skills/code-review/SKILL.md", "code-review", "plugin:pirategoat-tools"},

		// Worktree project CLAUDE.md.
		{home + "/Work/a8c/cabrero/.worktrees/review-tui/CLAUDE.md", "CLAUDE.md (review-tui)", "project:review-tui"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			name, origin := InferOriginFromPath(tt.path, home)
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if origin != tt.wantOrigin {
				t.Errorf("origin = %q, want %q", origin, tt.wantOrigin)
			}
		})
	}
}
