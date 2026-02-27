package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/fitness"
)

func TestChangeStore_AppendAndList(t *testing.T) {
	dir := t.TempDir()
	old := RootOverrideForTest(dir)
	defer ResetRootOverrideForTest(old)

	entry := fitness.ChangeEntry{
		ID:              "change-001",
		SourceName:      "my-skill",
		ProposalID:      "prop-001",
		Description:     "Added validation",
		Timestamp:       time.Now(),
		Status:          "approved",
		PreviousContent: "old content",
		FilePath:        "/path/to/skill.md",
	}

	if err := AppendChange(entry); err != nil {
		t.Fatalf("AppendChange: %v", err)
	}

	changes, err := ChangesBySource("my-skill")
	if err != nil {
		t.Fatalf("ChangesBySource: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}
	if changes[0].ID != "change-001" {
		t.Errorf("ID = %q, want change-001", changes[0].ID)
	}
	if changes[0].PreviousContent != "old content" {
		t.Errorf("PreviousContent = %q, want 'old content'", changes[0].PreviousContent)
	}
}

func TestChangeStore_GetByID(t *testing.T) {
	dir := t.TempDir()
	old := RootOverrideForTest(dir)
	defer ResetRootOverrideForTest(old)

	entry := fitness.ChangeEntry{
		ID:         "change-002",
		SourceName: "my-skill",
	}
	if err := AppendChange(entry); err != nil {
		t.Fatalf("AppendChange: %v", err)
	}

	got, err := GetChange("change-002")
	if err != nil {
		t.Fatalf("GetChange: %v", err)
	}
	if got == nil {
		t.Fatal("GetChange returned nil")
	}
	if got.ID != "change-002" {
		t.Errorf("ID = %q", got.ID)
	}
}

func TestChangeStore_GetByID_NotFound(t *testing.T) {
	dir := t.TempDir()
	old := RootOverrideForTest(dir)
	defer ResetRootOverrideForTest(old)

	got, err := GetChange("nonexistent")
	if err != nil {
		t.Fatalf("GetChange: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent change")
	}
}

func TestChangeStore_RollbackEntry(t *testing.T) {
	dir := t.TempDir()
	old := RootOverrideForTest(dir)
	defer ResetRootOverrideForTest(old)

	// Original change.
	entry := fitness.ChangeEntry{
		ID:              "change-003",
		SourceName:      "my-skill",
		Status:          "approved",
		PreviousContent: "original",
		FilePath:        filepath.Join(dir, "test.md"),
	}
	if err := AppendChange(entry); err != nil {
		t.Fatalf("AppendChange: %v", err)
	}

	// Rollback entry.
	rollback := fitness.ChangeEntry{
		ID:         "rollback-003",
		SourceName: "my-skill",
		Status:     "rollback",
	}
	if err := AppendChange(rollback); err != nil {
		t.Fatalf("AppendChange rollback: %v", err)
	}

	// Both should appear in source history.
	changes, _ := ChangesBySource("my-skill")
	if len(changes) != 2 {
		t.Fatalf("changes = %d, want 2", len(changes))
	}
}

func TestChangeStore_FileCreatedOnDemand(t *testing.T) {
	dir := t.TempDir()
	old := RootOverrideForTest(dir)
	defer ResetRootOverrideForTest(old)

	// File doesn't exist yet — ChangesBySource should return empty, not error.
	changes, err := ChangesBySource("anything")
	if err != nil {
		t.Fatalf("ChangesBySource on missing file: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected empty changes, got %d", len(changes))
	}
}
