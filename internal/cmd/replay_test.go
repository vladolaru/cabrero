package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReplay_MissingSession verifies that Replay returns an error when --session
// is absent from the argument list.
func TestReplay_MissingSession(t *testing.T) {
	setupTestEnv(t)

	err := Replay([]string{"--prompt", "/tmp/classifier-v4.txt"})
	if err == nil {
		t.Fatal("expected error when --session is missing, got nil")
	}
	if !strings.Contains(err.Error(), "--session") {
		t.Errorf("expected error message to mention --session, got: %v", err)
	}
}

// TestReplay_MissingPrompt verifies that Replay returns an error when --prompt
// is absent from the argument list.
func TestReplay_MissingPrompt(t *testing.T) {
	setupTestEnv(t)

	err := Replay([]string{"--session", "abcdef1234567890"})
	if err == nil {
		t.Fatal("expected error when --prompt is missing, got nil")
	}
	if !strings.Contains(err.Error(), "--prompt") {
		t.Errorf("expected error message to mention --prompt, got: %v", err)
	}
}

// TestReplay_InvalidStage verifies that Replay returns an error when --stage
// contains an unrecognised value.
func TestReplay_InvalidStage(t *testing.T) {
	setupTestEnv(t)

	// Create a session so the session-existence check passes.
	createTestSession(t, "abcdef1234567890")

	// Create a temporary prompt file so the file-read check passes.
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "myprompt.txt")
	if err := os.WriteFile(promptFile, []byte("test prompt"), 0o644); err != nil {
		t.Fatalf("writing temp prompt: %v", err)
	}

	err := Replay([]string{
		"--session", "abcdef1234567890",
		"--prompt", promptFile,
		"--stage", "aggregator",
	})
	if err == nil {
		t.Fatal("expected error for invalid --stage, got nil")
	}
	if !strings.Contains(err.Error(), "aggregator") {
		t.Errorf("expected error to mention the invalid stage value, got: %v", err)
	}
}

// TestReplay_SessionNotFound verifies that Replay returns an error when the
// session does not exist in the store.
func TestReplay_SessionNotFound(t *testing.T) {
	setupTestEnv(t)

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "classifier-v4.txt")
	if err := os.WriteFile(promptFile, []byte("test prompt"), 0o644); err != nil {
		t.Fatalf("writing temp prompt: %v", err)
	}

	err := Replay([]string{
		"--session", "nosuchsession12345",
		"--prompt", promptFile,
	})
	if err == nil {
		t.Fatal("expected error for non-existent session, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// TestReplay_UnambiguousStageInference verifies that when the prompt filename
// starts with a known stage name (e.g. "evaluator-..."), Replay infers the
// stage without requiring an explicit --stage flag.
// We only test the inference path here — no LLM is invoked.
func TestReplay_UnambiguousStageInference(t *testing.T) {
	setupTestEnv(t)

	// Create a session.
	createTestSession(t, "abcdef1234567890")

	// A prompt named "aggregator-..." cannot be inferred and should fail.
	dir := t.TempDir()
	ambiguousPrompt := filepath.Join(dir, "aggregator-v1.txt")
	if err := os.WriteFile(ambiguousPrompt, []byte("test"), 0o644); err != nil {
		t.Fatalf("writing temp prompt: %v", err)
	}

	err := Replay([]string{
		"--session", "abcdef1234567890",
		"--prompt", ambiguousPrompt,
	})
	if err == nil {
		t.Fatal("expected error for ambiguous stage inference, got nil")
	}
	// Should mention the filename.
	if !strings.Contains(err.Error(), "aggregator-v1.txt") {
		t.Errorf("expected filename in error, got: %v", err)
	}
}
