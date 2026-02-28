package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// notifySpy records notifications sent by the daemon.
type notifySpy struct {
	mu       sync.Mutex
	messages []string
}

func (n *notifySpy) notify(title, message string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.messages = append(n.messages, message)
	return nil
}

func (n *notifySpy) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.messages)
}

func (n *notifySpy) has(substr string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, m := range n.messages {
		if contains(m, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// setupTestDaemon creates a Daemon with a temp store, spy notifications,
// and a pipeline runner that classifies all sessions as clean (no LLM calls).
func setupTestDaemon(t *testing.T) (*Daemon, *notifySpy) {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	logPath := filepath.Join(dir, "daemon.log")
	cfg := Config{
		LogPath:                 logPath,
		InterSessionDelay:       0,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldown:  30 * time.Minute,
		Pipeline:                pipeline.DefaultPipelineConfig(),
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { d.log.Close() })

	// Inject fake classifier that marks everything as clean.
	d.runner = pipeline.NewRunnerWithStages(cfg.Pipeline, pipeline.TestStages{
		ClassifyFunc: func(sessionID string, cfg pipeline.PipelineConfig) (*pipeline.ClassifierResult, *pipeline.ClaudeResult, error) {
			return &pipeline.ClassifierResult{
				Digest:           &parser.Digest{SessionID: sessionID},
				ClassifierOutput: &pipeline.ClassifierOutput{SessionID: sessionID, Triage: "clean"},
			}, nil, nil
		},
	})
	d.runner.Source = "daemon"

	spy := &notifySpy{}
	d.NotifyFunc = spy.notify

	return d, spy
}

// createQueuedSession writes a minimal queued session into the store.
func createQueuedSession(t *testing.T, sessionID string) {
	t.Helper()
	rawDir := store.RawDir(sessionID)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := store.Metadata{
		SessionID:      sessionID,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CaptureTrigger: "test",
		Status:         store.StatusQueued,
	}
	if err := store.WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}
	// Write a minimal transcript so the session is valid.
	transcript := filepath.Join(rawDir, "transcript.jsonl")
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProcessOne_SkipsWhenSemaphoreFull(t *testing.T) {
	d, spy := setupTestDaemon(t)

	// Initialize semaphore with 1 slot and fill it.
	pipeline.ResetInvokeSemaphoreForTest()
	pipeline.InitInvokeSemaphore(1)
	if !pipeline.TryAcquireInvokeSemaphore() {
		t.Fatal("failed to acquire semaphore slot")
	}
	defer pipeline.ReleaseInvokeSemaphore()

	createQueuedSession(t, "session-blocked")

	// processOne should skip — semaphore is full.
	d.processOne(context.Background(), "session-blocked")

	// Session should NOT have been processed (no classifier call).
	if spy.count() != 0 {
		t.Errorf("expected no notifications (session skipped), got: %v", spy.messages)
	}

	// Session should still be queued (not marked as error or processed).
	meta, err := store.ReadMetadata("session-blocked")
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if meta.Status != store.StatusQueued {
		t.Errorf("status = %q, want %q (session should remain queued)", meta.Status, store.StatusQueued)
	}

	// A skipped_busy history record should have been written.
	records, err := pipeline.ReadHistory()
	if err != nil {
		t.Fatalf("ReadHistory: %v", err)
	}
	var found bool
	for _, rec := range records {
		if rec.SessionID == "session-blocked" && rec.Status == "skipped_busy" {
			found = true
			if rec.Source != "daemon" {
				t.Errorf("skip record Source = %q, want %q", rec.Source, "daemon")
			}
			break
		}
	}
	if !found {
		t.Error("expected a skipped_busy history record for session-blocked")
	}
}

func TestScanQueued_SkipsSessionsWithoutTranscript(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	// Session with transcript — should be included.
	createQueuedSession(t, "has-transcript-001")

	// Session without transcript — should be skipped.
	rawDir := store.RawDir("no-transcript-001")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := store.Metadata{
		SessionID: "no-transcript-001",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    store.StatusQueued,
	}
	if err := store.WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}

	queued, err := ScanQueued()
	if err != nil {
		t.Fatalf("ScanQueued: %v", err)
	}

	if len(queued) != 1 {
		t.Fatalf("got %d queued sessions, want 1", len(queued))
	}
	if queued[0].SessionID != "has-transcript-001" {
		t.Errorf("SessionID = %q, want %q", queued[0].SessionID, "has-transcript-001")
	}
}

func TestCleanDanglingQueued(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	log, err := NewLogger(filepath.Join(dir, "test.log"), 0)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer log.Close()

	// Old queued session WITHOUT transcript — should be cleaned up.
	oldRaw := store.RawDir("dangling-old-001")
	if err := os.MkdirAll(oldRaw, 0o755); err != nil {
		t.Fatal(err)
	}
	oldMeta := store.Metadata{
		SessionID: "dangling-old-001",
		Timestamp: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		Status:    store.StatusQueued,
	}
	if err := store.WriteMetadata(oldRaw, oldMeta); err != nil {
		t.Fatal(err)
	}

	// Recent queued session WITHOUT transcript — too fresh, should NOT be cleaned up.
	freshRaw := store.RawDir("dangling-fresh-001")
	if err := os.MkdirAll(freshRaw, 0o755); err != nil {
		t.Fatal(err)
	}
	freshMeta := store.Metadata{
		SessionID: "dangling-fresh-001",
		Timestamp: time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
		Status:    store.StatusQueued,
	}
	if err := store.WriteMetadata(freshRaw, freshMeta); err != nil {
		t.Fatal(err)
	}

	// Queued session WITH transcript — should NOT be cleaned up.
	createQueuedSession(t, "has-transcript-002")

	cleaned := CleanDanglingQueued(log)
	if cleaned != 1 {
		t.Fatalf("cleaned %d sessions, want 1", cleaned)
	}

	// Verify the old one was marked as error.
	sessions, _ := store.ListSessions()
	for _, s := range sessions {
		switch s.SessionID {
		case "dangling-old-001":
			if s.Status != store.StatusError {
				t.Errorf("dangling-old-001 status = %q, want %q", s.Status, store.StatusError)
			}
		case "dangling-fresh-001":
			if s.Status != store.StatusQueued {
				t.Errorf("dangling-fresh-001 status = %q, want %q", s.Status, store.StatusQueued)
			}
		case "has-transcript-002":
			if s.Status != store.StatusQueued {
				t.Errorf("has-transcript-002 status = %q, want %q", s.Status, store.StatusQueued)
			}
		}
	}
}

func TestProcessQueued_CircuitBreakerTripsAfterConsecutiveErrors(t *testing.T) {
	d, spy := setupTestDaemon(t)

	// Override breaker with a low threshold.
	d.breaker = NewCircuitBreaker(2, 30*time.Minute)

	// Inject classifier that always errors.
	callCount := 0
	d.runner = pipeline.NewRunnerWithStages(d.config.Pipeline, pipeline.TestStages{
		ClassifyFunc: func(sessionID string, cfg pipeline.PipelineConfig) (*pipeline.ClassifierResult, *pipeline.ClaudeResult, error) {
			callCount++
			return nil, nil, fmt.Errorf("simulated LLM failure")
		},
	})
	d.runner.Source = "daemon"

	createQueuedSession(t, "cb-session-1")
	createQueuedSession(t, "cb-session-2")
	createQueuedSession(t, "cb-session-3")

	d.processQueued(context.Background())

	// Breaker should trip after 2 errors. The 3rd session should NOT be attempted.
	if callCount > 2 {
		t.Errorf("expected at most 2 classifier calls, got %d", callCount)
	}

	if d.breaker.State() != CircuitOpen {
		t.Errorf("breaker state = %q, want %q", d.breaker.State(), CircuitOpen)
	}

	if !spy.has("Circuit breaker tripped") {
		t.Errorf("expected circuit breaker trip notification, got: %v", spy.messages)
	}
}

func TestProcessQueued_CircuitBreakerPausesProcessing(t *testing.T) {
	d, _ := setupTestDaemon(t)

	// Override breaker with a low threshold and pre-trip it.
	d.breaker = NewCircuitBreaker(2, 30*time.Minute)
	d.breaker.RecordFailure()
	d.breaker.RecordFailure()

	if d.breaker.State() != CircuitOpen {
		t.Fatalf("breaker should be open, got %q", d.breaker.State())
	}

	// Persist the open state so the external-reset check in processQueued
	// sees "open" and does not reset the in-memory breaker.
	if err := store.WriteCircuitBreakerState(store.CircuitBreakerState{
		State:             CircuitOpen,
		ConsecutiveErrors: d.breaker.ConsecutiveErrors(),
	}); err != nil {
		t.Fatalf("WriteCircuitBreakerState: %v", err)
	}

	// Inject classifier that counts calls.
	callCount := 0
	d.runner = pipeline.NewRunnerWithStages(d.config.Pipeline, pipeline.TestStages{
		ClassifyFunc: func(sessionID string, cfg pipeline.PipelineConfig) (*pipeline.ClassifierResult, *pipeline.ClaudeResult, error) {
			callCount++
			return &pipeline.ClassifierResult{
				Digest:           &parser.Digest{SessionID: sessionID},
				ClassifierOutput: &pipeline.ClassifierOutput{SessionID: sessionID, Triage: "clean"},
			}, nil, nil
		},
	})
	d.runner.Source = "daemon"

	createQueuedSession(t, "cb-paused-session")

	d.processQueued(context.Background())

	// No sessions should be processed — breaker is open.
	if callCount != 0 {
		t.Errorf("expected 0 classifier calls (breaker open), got %d", callCount)
	}
}

func TestProcessQueued_CircuitBreakerProbesInHalfOpen(t *testing.T) {
	d, _ := setupTestDaemon(t)

	// Override breaker with a low threshold and very short cooldown.
	d.breaker = NewCircuitBreaker(2, 1*time.Millisecond)
	d.breaker.RecordFailure()
	d.breaker.RecordFailure()

	// Wait for cooldown to elapse so breaker transitions to half-open.
	time.Sleep(5 * time.Millisecond)

	if d.breaker.State() != CircuitHalfOpen {
		t.Fatalf("breaker should be half-open, got %q", d.breaker.State())
	}

	// Persist the open state so the external-reset check in processQueued
	// sees "open" (not "closed") and does not reset the in-memory breaker.
	// The breaker will naturally transition to half-open via State() once the
	// cooldown has elapsed, which processQueued observes on the next State() call.
	if err := store.WriteCircuitBreakerState(store.CircuitBreakerState{
		State:             CircuitOpen,
		ConsecutiveErrors: d.breaker.ConsecutiveErrors(),
	}); err != nil {
		t.Fatalf("WriteCircuitBreakerState: %v", err)
	}

	// Inject classifier that counts calls and succeeds.
	callCount := 0
	d.runner = pipeline.NewRunnerWithStages(d.config.Pipeline, pipeline.TestStages{
		ClassifyFunc: func(sessionID string, cfg pipeline.PipelineConfig) (*pipeline.ClassifierResult, *pipeline.ClaudeResult, error) {
			callCount++
			return &pipeline.ClassifierResult{
				Digest:           &parser.Digest{SessionID: sessionID},
				ClassifierOutput: &pipeline.ClassifierOutput{SessionID: sessionID, Triage: "clean"},
			}, nil, nil
		},
	})
	d.runner.Source = "daemon"

	createQueuedSession(t, "cb-probe-session-1")
	createQueuedSession(t, "cb-probe-session-2")

	d.processQueued(context.Background())

	// Only 1 session should be processed (the probe).
	if callCount != 1 {
		t.Errorf("expected 1 classifier call (half-open probe), got %d", callCount)
	}

	// Successful probe should close the breaker.
	if d.breaker.State() != CircuitClosed {
		t.Errorf("breaker state after successful probe = %q, want %q", d.breaker.State(), CircuitClosed)
	}
}
