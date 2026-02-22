package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testHistoryRecord(sessionID string, ts time.Time) HistoryRecord {
	return HistoryRecord{
		SessionID:            sessionID,
		Timestamp:            ts,
		Project:              "test-project",
		Source:                "cli-run",
		Triage:               "evaluate",
		Status:               "processed",
		ProposalCount:        2,
		ClassifierDurationNs: int64(500 * time.Millisecond),
		EvaluatorDurationNs:  int64(3 * time.Second),
		TotalDurationNs:      int64(4 * time.Second),
		ClassifierModel:      DefaultClassifierModel,
		EvaluatorModel:       DefaultEvaluatorModel,
		ClassifierMaxTurns:   15,
		EvaluatorMaxTurns:    20,
		ClassifierTimeoutNs:  int64(2 * time.Minute),
		EvaluatorTimeoutNs:   int64(5 * time.Minute),
	}
}

// withHistoryFile temporarily overrides historyPath for testing,
// returns a cleanup function.
func withHistoryFile(t *testing.T, path string) func() {
	t.Helper()
	origPath := historyPath
	historyPath = func() string { return path }
	return func() { historyPath = origPath }
}

func TestAppendHistory_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run_history.jsonl")
	cleanup := withHistoryFile(t, path)
	defer cleanup()

	rec := testHistoryRecord("sess-001", time.Now())
	if err := AppendHistory(rec); err != nil {
		t.Fatalf("AppendHistory failed: %v", err)
	}

	// File should exist with one line.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	lines := nonEmptyLines(data)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}

func TestAppendHistory_AppendsMultiple(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run_history.jsonl")
	cleanup := withHistoryFile(t, path)
	defer cleanup()

	for i := 0; i < 3; i++ {
		rec := testHistoryRecord("sess-"+string(rune('a'+i)), time.Now())
		if err := AppendHistory(rec); err != nil {
			t.Fatalf("AppendHistory %d failed: %v", i, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	lines := nonEmptyLines(data)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestReadHistory_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run_history.jsonl")
	// File does not exist.
	records, err := readHistoryFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}

	// Empty file.
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("writing empty file: %v", err)
	}
	records, err = readHistoryFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

func TestReadHistory_SkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run_history.jsonl")
	cleanup := withHistoryFile(t, path)
	defer cleanup()

	// Write a valid record, then a malformed line, then another valid record.
	rec1 := testHistoryRecord("sess-valid-1", time.Now())
	rec2 := testHistoryRecord("sess-valid-2", time.Now())

	line1, _ := json.Marshal(rec1)
	line2, _ := json.Marshal(rec2)
	content := string(line1) + "\n" + "not valid json{{\n" + string(line2) + "\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	records, err := ReadHistory()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(records))
	}
	if records[0].SessionID != "sess-valid-1" {
		t.Errorf("first record session_id = %q, want %q", records[0].SessionID, "sess-valid-1")
	}
	if records[1].SessionID != "sess-valid-2" {
		t.Errorf("second record session_id = %q, want %q", records[1].SessionID, "sess-valid-2")
	}
}

func TestReadHistory_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run_history.jsonl")
	cleanup := withHistoryFile(t, path)
	defer cleanup()

	now := time.Now().Truncate(time.Millisecond) // JSON truncates sub-ms
	recs := []HistoryRecord{
		{
			SessionID:               "sess-rt-1",
			Timestamp:               now,
			Project:                 "proj-a",
			Source:                  "daemon",
			BatchMode:               true,
			BatchSize:               3,
			BatchSessionIDs:         []string{"sess-rt-1", "sess-rt-2", "sess-rt-3"},
			CaptureTrigger:          "session-end",
			PreviousStatus:          "queued",
			Triage:                  "evaluate",
			Status:                  "processed",
			ProposalCount:           1,
			ClassifierDurationNs:    int64(100 * time.Millisecond),
			EvaluatorDurationNs:     int64(2 * time.Second),
			TotalDurationNs:         int64(3 * time.Second),
			ClassifierModel:         DefaultClassifierModel,
			EvaluatorModel:          DefaultEvaluatorModel,
			ClassifierPromptVersion: "classifier-v3",
			EvaluatorPromptVersion:  "evaluator-v3",
			ClassifierMaxTurns:      15,
			EvaluatorMaxTurns:       20,
			ClassifierTimeoutNs:     int64(2 * time.Minute),
			EvaluatorTimeoutNs:      int64(5 * time.Minute),
			Debug:                   true,
		},
		{
			SessionID:               "sess-rt-2",
			Timestamp:               now.Add(-time.Hour),
			Project:                 "proj-b",
			Source:                  "cli-run",
			Triage:                  "clean",
			Status:                  "processed",
			ClassifierDurationNs:    int64(200 * time.Millisecond),
			TotalDurationNs:         int64(300 * time.Millisecond),
			ClassifierModel:         DefaultClassifierModel,
			ClassifierPromptVersion: "classifier-v3",
			ClassifierMaxTurns:      15,
			EvaluatorMaxTurns:       20,
			ClassifierTimeoutNs:     int64(2 * time.Minute),
			EvaluatorTimeoutNs:      int64(5 * time.Minute),
		},
	}

	for _, rec := range recs {
		if err := AppendHistory(rec); err != nil {
			t.Fatalf("AppendHistory failed: %v", err)
		}
	}

	got, err := ReadHistory()
	if err != nil {
		t.Fatalf("ReadHistory failed: %v", err)
	}
	if len(got) != len(recs) {
		t.Fatalf("expected %d records, got %d", len(recs), len(got))
	}

	for i, want := range recs {
		g := got[i]
		if g.SessionID != want.SessionID {
			t.Errorf("[%d] SessionID = %q, want %q", i, g.SessionID, want.SessionID)
		}
		if !g.Timestamp.Equal(want.Timestamp) {
			t.Errorf("[%d] Timestamp = %v, want %v", i, g.Timestamp, want.Timestamp)
		}
		if g.Source != want.Source {
			t.Errorf("[%d] Source = %q, want %q", i, g.Source, want.Source)
		}
		if g.BatchMode != want.BatchMode {
			t.Errorf("[%d] BatchMode = %v, want %v", i, g.BatchMode, want.BatchMode)
		}
		if g.BatchSize != want.BatchSize {
			t.Errorf("[%d] BatchSize = %d, want %d", i, g.BatchSize, want.BatchSize)
		}
		if g.Triage != want.Triage {
			t.Errorf("[%d] Triage = %q, want %q", i, g.Triage, want.Triage)
		}
		if g.Status != want.Status {
			t.Errorf("[%d] Status = %q, want %q", i, g.Status, want.Status)
		}
		if g.ProposalCount != want.ProposalCount {
			t.Errorf("[%d] ProposalCount = %d, want %d", i, g.ProposalCount, want.ProposalCount)
		}
		if g.ClassifierDurationNs != want.ClassifierDurationNs {
			t.Errorf("[%d] ClassifierDurationNs = %d, want %d", i, g.ClassifierDurationNs, want.ClassifierDurationNs)
		}
		if g.EvaluatorDurationNs != want.EvaluatorDurationNs {
			t.Errorf("[%d] EvaluatorDurationNs = %d, want %d", i, g.EvaluatorDurationNs, want.EvaluatorDurationNs)
		}
		if g.Debug != want.Debug {
			t.Errorf("[%d] Debug = %v, want %v", i, g.Debug, want.Debug)
		}
	}
}

func TestRotateHistory_RemovesOld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run_history.jsonl")
	cleanup := withHistoryFile(t, path)
	defer cleanup()

	now := time.Now()
	// Two old records (100 days ago) and one recent (1 day ago).
	old1 := testHistoryRecord("old-1", now.Add(-100*24*time.Hour))
	old2 := testHistoryRecord("old-2", now.Add(-95*24*time.Hour))
	recent := testHistoryRecord("recent-1", now.Add(-24*time.Hour))

	for _, rec := range []HistoryRecord{old1, old2, recent} {
		if err := AppendHistory(rec); err != nil {
			t.Fatalf("AppendHistory failed: %v", err)
		}
	}

	removed, err := RotateHistory(90 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("RotateHistory failed: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}

	records, err := ReadHistory()
	if err != nil {
		t.Fatalf("ReadHistory failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record after rotation, got %d", len(records))
	}
	if records[0].SessionID != "recent-1" {
		t.Errorf("remaining record session_id = %q, want %q", records[0].SessionID, "recent-1")
	}
}

func TestRotateHistory_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run_history.jsonl")
	cleanup := withHistoryFile(t, path)
	defer cleanup()

	// File does not exist — should be a no-op.
	removed, err := RotateHistory(90 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("RotateHistory failed: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}

func TestRotateHistory_AllRecent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run_history.jsonl")
	cleanup := withHistoryFile(t, path)
	defer cleanup()

	now := time.Now()
	for i := 0; i < 3; i++ {
		rec := testHistoryRecord("recent-"+string(rune('a'+i)), now.Add(-time.Duration(i)*24*time.Hour))
		if err := AppendHistory(rec); err != nil {
			t.Fatalf("AppendHistory failed: %v", err)
		}
	}

	removed, err := RotateHistory(90 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("RotateHistory failed: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}

	records, err := ReadHistory()
	if err != nil {
		t.Fatalf("ReadHistory failed: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
}

// nonEmptyLines splits data by newline and filters empty lines.
func nonEmptyLines(data []byte) []string {
	var lines []string
	for _, line := range splitLines(data) {
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
	}
	return lines
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
