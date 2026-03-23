package cmd

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestSessionsPurge_DryRun(t *testing.T) {
	dir := t.TempDir()
	old := store.RootOverrideForTest(dir)
	defer store.ResetRootOverrideForTest(old)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		id     string
		status string
	}{
		{"purge-test-err", store.StatusError},
		{"purge-test-cf", store.StatusCaptureFailed},
		{"purge-test-ok", store.StatusProcessed},
	} {
		d := store.RawDir(tc.id)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteMetadata(d, store.Metadata{
			SessionID: tc.id,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    tc.status,
		}); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	err := sessionsPurgeRun([]string{"--status", "error,capture_failed", "--dry-run"}, &buf)
	if err != nil {
		t.Fatalf("sessionsPurge: %v", err)
	}

	if !store.SessionExists("purge-test-err") {
		t.Error("dry-run should not remove sessions")
	}
	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Would purge")) {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSessionsPurge_Execute(t *testing.T) {
	dir := t.TempDir()
	old := store.RootOverrideForTest(dir)
	defer store.ResetRootOverrideForTest(old)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		id     string
		status string
	}{
		{"purge-exec-err", store.StatusError},
		{"purge-exec-cf", store.StatusCaptureFailed},
		{"purge-exec-ok", store.StatusProcessed},
	} {
		d := store.RawDir(tc.id)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteMetadata(d, store.Metadata{
			SessionID: tc.id,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    tc.status,
		}); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	err := sessionsPurgeRun([]string{"--status", "error,capture_failed"}, &buf)
	if err != nil {
		t.Fatalf("sessionsPurge: %v", err)
	}

	if store.SessionExists("purge-exec-err") {
		t.Error("error session should be removed")
	}
	if store.SessionExists("purge-exec-cf") {
		t.Error("capture_failed session should be removed")
	}
	if !store.SessionExists("purge-exec-ok") {
		t.Error("processed session should remain")
	}
}

func TestSessionsPurge_NoStatus(t *testing.T) {
	var buf bytes.Buffer
	err := sessionsPurgeRun([]string{}, &buf)
	if err == nil {
		t.Error("expected error when --status not provided")
	}
}

func TestSessionsPurge_InvalidStatus(t *testing.T) {
	var buf bytes.Buffer
	err := sessionsPurgeRun([]string{"--status", "processed"}, &buf)
	if err == nil {
		t.Error("expected error for non-purgeable status")
	}
}
