package store

import (
	"os"
	"testing"
	"time"
)

func createTestSession(t *testing.T, id, status, trigger, project string, ts time.Time) {
	t.Helper()
	rawDir := RawDir(id)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rawDir+"/transcript.jsonl", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := Metadata{
		SessionID:      id,
		Timestamp:      ts.UTC().Format(time.RFC3339),
		CaptureTrigger: trigger,
		Status:         status,
		Project:        project,
	}
	if err := WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}
}

func TestQuerySessions_StatusFilter(t *testing.T) {
	setupTestStore(t)
	now := time.Now()
	createTestSession(t, "s1", "pending", "imported", "proj-a", now.Add(-1*time.Hour))
	createTestSession(t, "s2", "processed", "session-end", "proj-a", now.Add(-2*time.Hour))
	createTestSession(t, "s3", "error", "imported", "proj-a", now.Add(-3*time.Hour))

	results, err := QuerySessions(SessionFilter{Statuses: []string{"pending"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].SessionID != "s1" {
		t.Errorf("got %d results, want 1 (s1)", len(results))
	}

	results, err = QuerySessions(SessionFilter{Statuses: []string{"pending", "error"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
	// Should be oldest-first: s3 then s1.
	if results[0].SessionID != "s3" || results[1].SessionID != "s1" {
		t.Errorf("order: got [%s, %s], want [s3, s1]", results[0].SessionID, results[1].SessionID)
	}
}

func TestQuerySessions_DateRange(t *testing.T) {
	setupTestStore(t)
	now := time.Now()
	createTestSession(t, "old", "pending", "imported", "proj", now.Add(-72*time.Hour))
	createTestSession(t, "mid", "pending", "imported", "proj", now.Add(-24*time.Hour))
	createTestSession(t, "new", "pending", "imported", "proj", now.Add(-1*time.Hour))

	since := now.Add(-48 * time.Hour)
	results, err := QuerySessions(SessionFilter{
		Statuses: []string{"pending"},
		Since:    since,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2 (mid, new)", len(results))
	}
}

func TestQuerySessions_ProjectFilter(t *testing.T) {
	setupTestStore(t)
	now := time.Now()
	createTestSession(t, "p1", "pending", "imported", "Work-a8c-woocommerce-payments", now)
	createTestSession(t, "p2", "pending", "imported", "Work-a8c-cabrero", now)

	results, err := QuerySessions(SessionFilter{
		Statuses: []string{"pending"},
		Project:  "woocommerce",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].SessionID != "p1" {
		t.Errorf("got %d results, want 1 (p1)", len(results))
	}
}

func TestQuerySessions_NoStatuses_ReturnsAll(t *testing.T) {
	setupTestStore(t)
	now := time.Now()
	createTestSession(t, "x1", "pending", "imported", "proj", now)
	createTestSession(t, "x2", "processed", "session-end", "proj", now)

	results, err := QuerySessions(SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2 (all)", len(results))
	}
}
