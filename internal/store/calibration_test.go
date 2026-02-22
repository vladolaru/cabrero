package store

import (
	"testing"
	"time"
)

func TestCalibrationSet_AddAndList(t *testing.T) {
	setupTestStore(t)

	entry := CalibrationEntry{
		SessionID: "sess-001",
		Label:     "approve",
		Note:      "clear improvement signal",
	}
	if err := AddCalibrationEntry(entry); err != nil {
		t.Fatalf("AddCalibrationEntry: %v", err)
	}

	entries, err := ListCalibrationEntries()
	if err != nil {
		t.Fatalf("ListCalibrationEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want %q", entries[0].SessionID, "sess-001")
	}
	if entries[0].Label != "approve" {
		t.Errorf("Label = %q, want %q", entries[0].Label, "approve")
	}
	if entries[0].Note != "clear improvement signal" {
		t.Errorf("Note = %q, want %q", entries[0].Note, "clear improvement signal")
	}
	if entries[0].TaggedAt.IsZero() {
		t.Error("TaggedAt should be auto-set, got zero")
	}
}

func TestCalibrationSet_Remove(t *testing.T) {
	setupTestStore(t)

	// Add two entries.
	for _, id := range []string{"sess-001", "sess-002"} {
		if err := AddCalibrationEntry(CalibrationEntry{
			SessionID: id,
			Label:     "reject",
		}); err != nil {
			t.Fatalf("AddCalibrationEntry(%s): %v", id, err)
		}
	}

	// Remove one.
	if err := RemoveCalibrationEntry("sess-001"); err != nil {
		t.Fatalf("RemoveCalibrationEntry: %v", err)
	}

	entries, err := ListCalibrationEntries()
	if err != nil {
		t.Fatalf("ListCalibrationEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].SessionID != "sess-002" {
		t.Errorf("remaining SessionID = %q, want %q", entries[0].SessionID, "sess-002")
	}
}

func TestCalibrationSet_RemoveNotFound(t *testing.T) {
	setupTestStore(t)

	err := RemoveCalibrationEntry("nonexistent")
	if err == nil {
		t.Error("expected error when removing nonexistent entry")
	}
}

func TestCalibrationSet_DuplicatePrevented(t *testing.T) {
	setupTestStore(t)

	entry := CalibrationEntry{
		SessionID: "sess-dup",
		Label:     "approve",
	}
	if err := AddCalibrationEntry(entry); err != nil {
		t.Fatalf("first add: %v", err)
	}

	err := AddCalibrationEntry(entry)
	if err == nil {
		t.Error("expected error when adding duplicate session ID")
	}
}

func TestCalibrationSet_InvalidLabel(t *testing.T) {
	setupTestStore(t)

	entry := CalibrationEntry{
		SessionID: "sess-bad-label",
		Label:     "maybe",
	}
	err := AddCalibrationEntry(entry)
	if err == nil {
		t.Error("expected error for invalid label")
	}
}

func TestCalibrationSet_PreservesTaggedAt(t *testing.T) {
	setupTestStore(t)

	explicit := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	entry := CalibrationEntry{
		SessionID: "sess-explicit-time",
		Label:     "approve",
		TaggedAt:  explicit,
	}
	if err := AddCalibrationEntry(entry); err != nil {
		t.Fatalf("AddCalibrationEntry: %v", err)
	}

	entries, err := ListCalibrationEntries()
	if err != nil {
		t.Fatalf("ListCalibrationEntries: %v", err)
	}
	if !entries[0].TaggedAt.Equal(explicit) {
		t.Errorf("TaggedAt = %v, want %v", entries[0].TaggedAt, explicit)
	}
}

func TestCalibrationSet_ListEmpty(t *testing.T) {
	setupTestStore(t)

	entries, err := ListCalibrationEntries()
	if err != nil {
		t.Fatalf("ListCalibrationEntries: %v", err)
	}
	if entries == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}
