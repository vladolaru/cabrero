package store

import (
	"os"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

func TestMarkProcessed(t *testing.T) {
	setupTestStore(t)

	sid := "test-session-mark-processed"
	rawDir := RawDir(sid)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := Metadata{
		SessionID:      sid,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CaptureTrigger: "imported",
		Status:         "pending",
	}
	if err := WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}

	if err := MarkProcessed(sid); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}

	got, err := ReadMetadata(sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "processed" {
		t.Errorf("status = %q, want %q", got.Status, "processed")
	}
}

func TestMarkError(t *testing.T) {
	setupTestStore(t)

	sid := "test-session-mark-error"
	rawDir := RawDir(sid)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := Metadata{
		SessionID:      sid,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CaptureTrigger: "imported",
		Status:         "pending",
	}
	if err := WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}

	if err := MarkError(sid); err != nil {
		t.Fatalf("MarkError: %v", err)
	}

	got, err := ReadMetadata(sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "error" {
		t.Errorf("status = %q, want %q", got.Status, "error")
	}
}

func TestWorkDirRoundtrip(t *testing.T) {
	setupTestStore(t)

	sid := "test-session-workdir"
	rawDir := RawDir(sid)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := Metadata{
		SessionID:      sid,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CaptureTrigger: "session-end",
		Status:         StatusQueued,
		WorkDir:        "/Users/vlad/Work/a8c/cabrero",
	}
	if err := WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}

	got, err := ReadMetadata(sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkDir != "/Users/vlad/Work/a8c/cabrero" {
		t.Errorf("WorkDir = %q, want %q", got.WorkDir, "/Users/vlad/Work/a8c/cabrero")
	}

	// Verify omitempty: WorkDir absent when empty.
	meta.WorkDir = ""
	if err := WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}
	got, err = ReadMetadata(sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkDir != "" {
		t.Errorf("WorkDir = %q, want empty", got.WorkDir)
	}
}

func TestMarkProcessed_NotFound(t *testing.T) {
	setupTestStore(t)
	err := MarkProcessed("nonexistent-session")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}
