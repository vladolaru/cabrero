package daemon

import (
	"context"
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
		LogPath:           logPath,
		InterSessionDelay: 0,
		Pipeline:          pipeline.DefaultPipelineConfig(),
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { d.log.Close() })

	// Inject fake classifier that marks everything as clean.
	d.runner.ClassifyFunc = func(sessionID string, cfg pipeline.PipelineConfig) (*pipeline.ClassifierResult, error) {
		return &pipeline.ClassifierResult{
			Digest:           &parser.Digest{SessionID: sessionID},
			ClassifierOutput: &pipeline.ClassifierOutput{SessionID: sessionID, Triage: "clean"},
		}, nil
	}

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

func TestProcessQueued_NotifiesWhenQueueDrained(t *testing.T) {
	d, spy := setupTestDaemon(t)

	createQueuedSession(t, "session-aaa")

	d.processQueued(context.Background())

	if !spy.has("Queue processing complete") {
		t.Errorf("expected queue-drain notification, got %d notifications: %v", spy.count(), spy.messages)
	}
}

func TestProcessQueued_NoNotifyWhenQueueStillHasSessions(t *testing.T) {
	d, spy := setupTestDaemon(t)

	createQueuedSession(t, "session-bbb")
	createQueuedSession(t, "session-ccc")

	// Inject a classifier that processes the first session but sneaks a new
	// queued session into the store before the second one finishes —
	// simulating a hook capture arriving during processing.
	callCount := 0
	d.runner.ClassifyFunc = func(sessionID string, cfg pipeline.PipelineConfig) (*pipeline.ClassifierResult, error) {
		callCount++
		if callCount == 2 {
			// A new session arrives while we're still processing.
			createQueuedSession(t, "session-ddd")
		}
		return &pipeline.ClassifierResult{
			Digest:           &parser.Digest{SessionID: sessionID},
			ClassifierOutput: &pipeline.ClassifierOutput{SessionID: sessionID, Triage: "clean"},
		}, nil
	}

	d.processQueued(context.Background())

	if spy.has("Queue processing complete") {
		t.Errorf("should NOT notify when queue still has sessions, got: %v", spy.messages)
	}
}

func TestProcessQueued_NoNotifyOnEmptyQueue(t *testing.T) {
	d, spy := setupTestDaemon(t)

	// No sessions created — queue is empty.
	d.processQueued(context.Background())

	if spy.count() != 0 {
		t.Errorf("expected no notifications for empty queue, got: %v", spy.messages)
	}
}

func TestProcessQueued_MultipleSessions_NotifiesOnceDrained(t *testing.T) {
	d, spy := setupTestDaemon(t)

	createQueuedSession(t, "session-eee")
	createQueuedSession(t, "session-fff")
	createQueuedSession(t, "session-ggg")

	d.processQueued(context.Background())

	drainCount := 0
	for _, msg := range spy.messages {
		if contains(msg, "Queue processing complete") {
			drainCount++
		}
	}
	if drainCount != 1 {
		t.Errorf("expected exactly 1 queue-drain notification, got %d; messages: %v", drainCount, spy.messages)
	}
}
