package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/store"
)

func setupPromptsStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	return dir
}

func TestPrompts_ListsFiles(t *testing.T) {
	home := setupPromptsStore(t)

	promptsDir := filepath.Join(home, ".cabrero", "prompts")

	// Write two prompt files.
	if err := os.WriteFile(filepath.Join(promptsDir, "classifier-v3.txt"), []byte("prompt content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "proposer-v2.txt"), []byte("another prompt"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Prompts(nil)
	if err != nil {
		t.Fatalf("Prompts() returned error: %v", err)
	}
}

func TestPrompts_EmptyDir(t *testing.T) {
	setupPromptsStore(t)

	// prompts/ dir exists but is empty (store.Init creates it).
	err := Prompts(nil)
	if err != nil {
		t.Fatalf("Prompts() returned error: %v", err)
	}
}

func TestFormatAge(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, "unknown"},
		{"just now", now.Add(-10 * time.Second), "just now"},
		{"minutes ago", now.Add(-5 * time.Minute), "5m ago"},
		{"hours ago", now.Add(-3 * time.Hour), "3h ago"},
		{"days ago", now.Add(-48 * time.Hour), "2d ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cli.RelativeTime(tt.t)
			if got != tt.want {
				t.Errorf("cli.RelativeTime() = %q, want %q", got, tt.want)
			}
		})
	}
}
