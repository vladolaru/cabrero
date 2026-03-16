package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoredPatterns_EmptyByDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	patterns, err := ReadIgnoredPatterns()
	if err != nil {
		t.Fatalf("ReadIgnoredPatterns: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(patterns))
	}
}

func TestAddIgnoredPattern(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := AddIgnoredPattern("CodexBar"); err != nil {
		t.Fatalf("AddIgnoredPattern: %v", err)
	}

	patterns, err := ReadIgnoredPatterns()
	if err != nil {
		t.Fatalf("ReadIgnoredPatterns: %v", err)
	}
	if len(patterns) != 1 || patterns[0].Pattern != "CodexBar" {
		t.Errorf("unexpected patterns: %+v", patterns)
	}
}

func TestAddIgnoredPattern_EmptyRejected(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, pattern := range []string{"", "   ", "\t\n"} {
		if err := AddIgnoredPattern(pattern); err == nil {
			t.Errorf("AddIgnoredPattern(%q) should have returned an error", pattern)
		}
	}

	// Verify nothing was written.
	patterns, err := ReadIgnoredPatterns()
	if err != nil {
		t.Fatalf("ReadIgnoredPatterns: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns after rejected adds, got %d", len(patterns))
	}
}

func TestAddIgnoredPattern_Duplicate(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	AddIgnoredPattern("CodexBar")
	if err := AddIgnoredPattern("CodexBar"); err != nil {
		t.Fatalf("AddIgnoredPattern duplicate: %v", err)
	}

	patterns, _ := ReadIgnoredPatterns()
	if len(patterns) != 1 {
		t.Errorf("expected 1 pattern after duplicate add, got %d", len(patterns))
	}
}

func TestRemoveIgnoredPattern(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	AddIgnoredPattern("CodexBar")
	removed, err := RemoveIgnoredPattern("CodexBar")
	if err != nil {
		t.Fatalf("RemoveIgnoredPattern: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}

	patterns, _ := ReadIgnoredPatterns()
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns after remove, got %d", len(patterns))
	}
}

func TestRemoveIgnoredPattern_NotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	removed, err := RemoveIgnoredPattern("nonexistent")
	if err != nil {
		t.Fatalf("RemoveIgnoredPattern: %v", err)
	}
	if removed {
		t.Error("expected removed=false for nonexistent pattern")
	}
}

func TestIsProjectIgnored(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	AddIgnoredPattern("CodexBar")

	tests := []struct {
		slug string
		want bool
	}{
		{"-Users-vlad-Library-Application-Support-CodexBar-ClaudeProbe", true},
		{"-Users-vlad-Work-a8c-cabrero", false},
		{"-Users-vlad-codexbar-stuff", true}, // case-insensitive
		{"", false},
	}
	for _, tt := range tests {
		got := IsProjectIgnored(tt.slug)
		if got != tt.want {
			t.Errorf("IsProjectIgnored(%q) = %v, want %v", tt.slug, got, tt.want)
		}
	}
}

func TestIsProjectIgnored_NoFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Don't Init — no file exists. Should return false, not error.
	if IsProjectIgnored("anything") {
		t.Error("expected false when no ignore file exists")
	}
}

func TestCountIgnoredSessions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Create two sessions: one matching, one not.
	for _, tc := range []struct {
		id, project string
	}{
		{"sess-aaa", "-Users-vlad-CodexBar-Probe"},
		{"sess-bbb", "-Users-vlad-Work-cabrero"},
	} {
		dir := filepath.Join(tmp, ".cabrero", "raw", tc.id)
		os.MkdirAll(dir, 0o755)
		meta := `{"session_id":"` + tc.id + `","project":"` + tc.project + `","status":"capture_failed"}`
		os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(meta), 0o644)
	}

	AddIgnoredPattern("CodexBar")
	count := CountIgnoredSessions()
	if count != 1 {
		t.Errorf("CountIgnoredSessions = %d, want 1", count)
	}
}
