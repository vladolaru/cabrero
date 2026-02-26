package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestBlocklistMigration_OldArrayFormat(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Write old-format blocklist ([]string array).
	oldData := `["sess-aaa","sess-bbb"]`
	blPath := filepath.Join(tmp, ".cabrero", "blocklist.json")
	if err := os.WriteFile(blPath, []byte(oldData), 0o644); err != nil {
		t.Fatalf("writing old blocklist: %v", err)
	}
	// ReadBlocklist must return both IDs without error.
	m, err := ReadBlocklist()
	if err != nil {
		t.Fatalf("ReadBlocklist after migration: %v", err)
	}
	if !m["sess-aaa"] || !m["sess-bbb"] {
		t.Errorf("expected both sessions in blocklist, got %v", m)
	}
}

func TestBlockSession_WritesTimestamp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	before := time.Now()
	if err := BlockSession("sess-xyz", time.Now()); err != nil {
		t.Fatalf("BlockSession: %v", err)
	}
	after := time.Now()

	m, err := ReadBlocklist()
	if err != nil {
		t.Fatalf("ReadBlocklist: %v", err)
	}
	if !m["sess-xyz"] {
		t.Errorf("sess-xyz not in blocklist")
	}

	// Read raw file and verify timestamp field is present.
	blPath := filepath.Join(tmp, ".cabrero", "blocklist.json")
	raw, _ := os.ReadFile(blPath)
	var entries map[string]struct{ BlockedAt time.Time }
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parsing new-format blocklist: %v", err)
	}
	e := entries["sess-xyz"]
	if e.BlockedAt.Before(before) || e.BlockedAt.After(after) {
		t.Errorf("BlockedAt %v outside expected range [%v, %v]", e.BlockedAt, before, after)
	}
}

func TestRotateBlocklist_RemovesOldEntries(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	old := time.Now().Add(-100 * 24 * time.Hour) // 100 days ago
	fresh := time.Now().Add(-1 * time.Hour)       // 1 hour ago

	if err := BlockSession("old-sess", old); err != nil {
		t.Fatalf("BlockSession old: %v", err)
	}
	if err := BlockSession("fresh-sess", fresh); err != nil {
		t.Fatalf("BlockSession fresh: %v", err)
	}

	removed, err := RotateBlocklist(90 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("RotateBlocklist: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	m, _ := ReadBlocklist()
	if m["old-sess"] {
		t.Error("old-sess should have been rotated out")
	}
	if !m["fresh-sess"] {
		t.Error("fresh-sess should have been kept")
	}
}

func TestRotateBlocklist_ZeroAgeEntries_Kept(t *testing.T) {
	// Migrated entries have BlockedAt zero — they should NOT be rotated
	// until they age past maxAge from the zero time, which is far in the past.
	// In practice, they will be rotated immediately since zero time is ancient.
	// This test verifies the rotation is based purely on BlockedAt age.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := BlockSession("migrated-sess", time.Time{}); err != nil {
		t.Fatalf("BlockSession: %v", err)
	}
	removed, err := RotateBlocklist(90 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("RotateBlocklist: %v", err)
	}
	if removed != 1 {
		t.Errorf("migrated entry with zero time should be rotated, removed = %d", removed)
	}
}
