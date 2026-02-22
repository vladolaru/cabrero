package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit_CreatesReplaysDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	dir := filepath.Join(tmp, ".cabrero", "replays")
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("replays dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("replays exists but is not a directory")
	}
}

func TestReplayDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	want := filepath.Join(tmp, ".cabrero", "replays")
	got := ReplayDir()
	if got != want {
		t.Errorf("ReplayDir() = %q, want %q", got, want)
	}
}

func TestArchivedProposalsDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	want := filepath.Join(tmp, ".cabrero", "proposals", "archived")
	got := ArchivedProposalsDir()
	if got != want {
		t.Errorf("ArchivedProposalsDir() = %q, want %q", got, want)
	}
}
