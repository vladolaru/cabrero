package cmd

import (
	"os"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func setupTestEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

func TestCalibrateTag_MissingLabel(t *testing.T) {
	setupTestEnv(t)

	err := Calibrate([]string{"tag", "some-session"})
	if err == nil {
		t.Error("expected error for missing --label")
	}
}

func TestCalibrateTag_InvalidLabel(t *testing.T) {
	setupTestEnv(t)

	// Create a session so the "session not found" check passes.
	createTestSession(t, "some-session")

	err := Calibrate([]string{"tag", "some-session", "--label", "maybe"})
	if err == nil {
		t.Error("expected error for invalid label")
	}
}

func TestCalibrate_UnknownSubcommand(t *testing.T) {
	setupTestEnv(t)

	err := Calibrate([]string{"frobnicate"})
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}
}

func TestCalibrateList_Empty(t *testing.T) {
	setupTestEnv(t)

	err := Calibrate([]string{"list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCalibrate_NoSubcommand(t *testing.T) {
	setupTestEnv(t)

	err := Calibrate(nil)
	if err == nil {
		t.Error("expected error for no subcommand")
	}
}

// createTestSession creates a minimal session in the test store so that
// SessionExists returns true.
func createTestSession(t *testing.T, sessionID string) {
	t.Helper()
	rawDir := store.RawDir(sessionID)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatalf("creating raw dir: %v", err)
	}
	meta := store.Metadata{
		SessionID:      sessionID,
		Timestamp:      "2025-01-01T00:00:00Z",
		CaptureTrigger: "test",
		Status:         store.StatusImported,
	}
	if err := store.WriteMetadata(rawDir, meta); err != nil {
		t.Fatalf("creating test session: %v", err)
	}
}
