package pipeline

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupHistoryAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	origPath := cleanupHistoryPath
	cleanupHistoryPath = func() string { return filepath.Join(dir, "cleanup_history.jsonl") }
	defer func() { cleanupHistoryPath = origPath }()

	rec := CleanupRecord{
		Timestamp:       time.Now().Truncate(time.Second),
		DurationNs:      int64(5 * time.Second),
		ProposalsBefore: 10,
		ProposalsAfter:  3,
		Decisions: []CuratorDecision{
			{ProposalID: "prop-abc-1", Action: "cull", Reason: "already applied"},
		},
	}

	if err := AppendCleanupHistory(rec); err != nil {
		t.Fatal(err)
	}

	records, err := ReadCleanupHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].ProposalsBefore != 10 {
		t.Errorf("ProposalsBefore: got %d, want 10", records[0].ProposalsBefore)
	}
	if len(records[0].Decisions) != 1 {
		t.Errorf("Decisions: got %d, want 1", len(records[0].Decisions))
	}
}

func TestCleanupHistoryMissingFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	origPath := cleanupHistoryPath
	cleanupHistoryPath = func() string { return filepath.Join(dir, "no_such_file.jsonl") }
	defer func() { cleanupHistoryPath = origPath }()

	records, err := ReadCleanupHistory()
	if err != nil {
		t.Fatal(err)
	}
	if records != nil {
		t.Errorf("got %v, want nil", records)
	}
}

func TestRotateCleanupHistory(t *testing.T) {
	dir := t.TempDir()
	origPath := cleanupHistoryPath
	cleanupHistoryPath = func() string { return filepath.Join(dir, "cleanup_history.jsonl") }
	defer func() { cleanupHistoryPath = origPath }()

	old := CleanupRecord{Timestamp: time.Now().Add(-100 * 24 * time.Hour)}
	newRec := CleanupRecord{Timestamp: time.Now()}
	_ = AppendCleanupHistory(old)
	_ = AppendCleanupHistory(newRec)

	removed, err := RotateCleanupHistory(90 * 24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("removed: got %d, want 1", removed)
	}
	records, _ := ReadCleanupHistory()
	if len(records) != 1 {
		t.Errorf("remaining: got %d, want 1", len(records))
	}
}
