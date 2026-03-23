package store

import (
	"os"
	"path/filepath"
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

func TestTranscriptExists_True(t *testing.T) {
	setupTestStore(t)

	sid := "transcript-exists-1"
	rawDir := RawDir(sid)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rawDir, "transcript.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !TranscriptExists(sid) {
		t.Error("TranscriptExists = false, want true")
	}
}

func TestTranscriptExists_False_NoFile(t *testing.T) {
	setupTestStore(t)

	sid := "transcript-missing-1"
	rawDir := RawDir(sid)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if TranscriptExists(sid) {
		t.Error("TranscriptExists = true, want false")
	}
}

func TestTranscriptExists_False_NoDir(t *testing.T) {
	setupTestStore(t)

	if TranscriptExists("nonexistent-session") {
		t.Error("TranscriptExists = true, want false")
	}
}

func TestPurgeSessions(t *testing.T) {
	setupTestStore(t)

	for _, tc := range []struct {
		id     string
		status string
	}{
		{"purge-err-1", StatusError},
		{"purge-err-2", StatusError},
		{"purge-cf-1", StatusCaptureFailed},
		{"purge-proc-1", StatusProcessed},
		{"purge-queued-1", StatusQueued},
	} {
		dir := RawDir(tc.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		meta := Metadata{
			SessionID: tc.id,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    tc.status,
		}
		if err := WriteMetadata(dir, meta); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := PurgeSessions([]string{StatusError})
	if err != nil {
		t.Fatalf("PurgeSessions: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}

	if SessionExists("purge-err-1") {
		t.Error("purge-err-1 still exists")
	}
	if SessionExists("purge-err-2") {
		t.Error("purge-err-2 still exists")
	}
	if !SessionExists("purge-cf-1") {
		t.Error("purge-cf-1 should still exist")
	}
	if !SessionExists("purge-proc-1") {
		t.Error("purge-proc-1 should still exist")
	}
}

func TestPurgeSessions_MultipleStatuses(t *testing.T) {
	setupTestStore(t)

	for _, tc := range []struct {
		id     string
		status string
	}{
		{"purge-m-err", StatusError},
		{"purge-m-cf", StatusCaptureFailed},
		{"purge-m-proc", StatusProcessed},
	} {
		dir := RawDir(tc.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := WriteMetadata(dir, Metadata{
			SessionID: tc.id,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    tc.status,
		}); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := PurgeSessions([]string{StatusError, StatusCaptureFailed})
	if err != nil {
		t.Fatalf("PurgeSessions: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
	if !SessionExists("purge-m-proc") {
		t.Error("purge-m-proc should still exist")
	}
}
