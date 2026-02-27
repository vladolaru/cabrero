package pipeline

import (
	"testing"
	"time"
)

func TestComputeOpsStats_Empty(t *testing.T) {
	stats := ComputeOpsStats(nil, time.Now().Add(-24*time.Hour), 7)

	if stats.GatedRuns != 0 {
		t.Errorf("GatedRuns = %d, want 0", stats.GatedRuns)
	}
	if stats.SkippedBusy != 0 {
		t.Errorf("SkippedBusy = %d, want 0", stats.SkippedBusy)
	}
	if stats.MetaTriggered != 0 {
		t.Errorf("MetaTriggered = %d, want 0", stats.MetaTriggered)
	}
	if len(stats.RecentEvents) != 0 {
		t.Errorf("RecentEvents len = %d, want 0", len(stats.RecentEvents))
	}
}

func TestComputeOpsStats_MixedRecords(t *testing.T) {
	now := time.Now()
	since := now.Add(-48 * time.Hour)

	records := []HistoryRecord{
		// Normal processed run — should count as processed.
		{SessionID: "s1", Timestamp: now.Add(-1 * time.Hour), Source: "daemon", Status: HistoryStatusProcessed},
		// Gated run (unclassified) — should count as both processed and gated.
		{SessionID: "s2", Timestamp: now.Add(-2 * time.Hour), Source: "daemon", Status: HistoryStatusProcessed, GateReason: GateReasonUnclassified},
		// Gated run (paused).
		{SessionID: "s3", Timestamp: now.Add(-3 * time.Hour), Source: "daemon", Status: HistoryStatusProcessed, GateReason: GateReasonPaused},
		// Skipped busy.
		{SessionID: "s4", Timestamp: now.Add(-4 * time.Hour), Source: "daemon", Status: HistoryStatusSkippedBusy},
		{SessionID: "s5", Timestamp: now.Add(-5 * time.Hour), Source: "daemon", Status: HistoryStatusSkippedBusy},
		// Error.
		{SessionID: "s6", Timestamp: now.Add(-6 * time.Hour), Source: "daemon", Status: HistoryStatusError, ErrorDetail: "timeout"},
		// Meta events.
		{Timestamp: now.Add(-7 * time.Hour), Source: "meta", Status: HistoryStatusMetaTriggered, ErrorDetail: "v3"},
		{Timestamp: now.Add(-8 * time.Hour), Source: "meta", Status: HistoryStatusMetaCooldown, ErrorDetail: "v2 in cooldown"},
		{Timestamp: now.Add(-9 * time.Hour), Source: "meta", Status: HistoryStatusMetaNoThreshold, ErrorDetail: "all clear"},
		// Old record — outside window.
		{SessionID: "old", Timestamp: now.Add(-72 * time.Hour), Source: "daemon", Status: HistoryStatusProcessed},
	}

	stats := ComputeOpsStats(records, since, 7)

	if stats.ProcessedRuns != 3 { // s1, s2, s3
		t.Errorf("ProcessedRuns = %d, want 3", stats.ProcessedRuns)
	}
	if stats.GatedRuns != 2 { // s2, s3
		t.Errorf("GatedRuns = %d, want 2", stats.GatedRuns)
	}
	if stats.GatedUnclassified != 1 {
		t.Errorf("GatedUnclassified = %d, want 1", stats.GatedUnclassified)
	}
	if stats.GatedPaused != 1 {
		t.Errorf("GatedPaused = %d, want 1", stats.GatedPaused)
	}
	if stats.SkippedBusy != 2 {
		t.Errorf("SkippedBusy = %d, want 2", stats.SkippedBusy)
	}
	if stats.ErrorRuns != 1 {
		t.Errorf("ErrorRuns = %d, want 1", stats.ErrorRuns)
	}
	if stats.MetaTriggered != 1 {
		t.Errorf("MetaTriggered = %d, want 1", stats.MetaTriggered)
	}
	if stats.MetaCooldowns != 1 {
		t.Errorf("MetaCooldowns = %d, want 1", stats.MetaCooldowns)
	}
	if stats.MetaNoThreshold != 1 {
		t.Errorf("MetaNoThreshold = %d, want 1", stats.MetaNoThreshold)
	}
}

func TestComputeOpsStats_RecentEvents(t *testing.T) {
	now := time.Now()
	since := now.Add(-24 * time.Hour)

	records := []HistoryRecord{
		// Normal processed — NOT included in events (not interesting).
		{SessionID: "normal", Timestamp: now.Add(-1 * time.Hour), Source: "daemon", Status: HistoryStatusProcessed},
		// Gated — included.
		{SessionID: "gated", Timestamp: now.Add(-2 * time.Hour), Source: "daemon", Status: HistoryStatusProcessed, GateReason: GateReasonUnclassified},
		// Skipped — included.
		{SessionID: "skipped", Timestamp: now.Add(-3 * time.Hour), Source: "daemon", Status: HistoryStatusSkippedBusy},
		// Error — included.
		{SessionID: "errored", Timestamp: now.Add(-4 * time.Hour), Source: "daemon", Status: HistoryStatusError, ErrorDetail: "fail"},
		// Meta triggered — included.
		{Timestamp: now.Add(-5 * time.Hour), Source: "meta", Status: HistoryStatusMetaTriggered, ErrorDetail: "v3"},
		// Meta cooldown — included.
		{Timestamp: now.Add(-6 * time.Hour), Source: "meta", Status: HistoryStatusMetaCooldown, ErrorDetail: "v2"},
		// Meta no threshold — NOT included (not interesting).
		{Timestamp: now.Add(-7 * time.Hour), Source: "meta", Status: HistoryStatusMetaNoThreshold},
	}

	stats := ComputeOpsStats(records, since, 7)

	if len(stats.RecentEvents) != 5 { // gated, skipped, error, meta triggered, meta cooldown
		t.Errorf("RecentEvents len = %d, want 5", len(stats.RecentEvents))
	}

	// Should be sorted newest first.
	if len(stats.RecentEvents) >= 2 {
		if stats.RecentEvents[0].Timestamp.Before(stats.RecentEvents[1].Timestamp) {
			t.Error("RecentEvents not sorted newest first")
		}
	}

	// First event should be the gated run (most recent interesting event).
	if stats.RecentEvents[0].SessionID != "gated" {
		t.Errorf("first event SessionID = %q, want %q", stats.RecentEvents[0].SessionID, "gated")
	}
}

func TestComputeOpsStats_DailyBuckets(t *testing.T) {
	now := time.Now()
	since := now.Add(-7 * 24 * time.Hour)

	records := []HistoryRecord{
		// Today.
		{SessionID: "s1", Timestamp: now.Add(-1 * time.Hour), Source: "daemon", Status: HistoryStatusProcessed, GateReason: GateReasonUnclassified},
		{SessionID: "s2", Timestamp: now.Add(-2 * time.Hour), Source: "daemon", Status: HistoryStatusSkippedBusy},
		// Yesterday.
		{SessionID: "s3", Timestamp: now.Add(-25 * time.Hour), Source: "daemon", Status: HistoryStatusProcessed, GateReason: GateReasonPaused},
		{SessionID: "s4", Timestamp: now.Add(-26 * time.Hour), Source: "daemon", Status: HistoryStatusError},
	}

	stats := ComputeOpsStats(records, since, 7)

	// Daily gated: [today=1, yesterday=1, 0, 0, 0, 0, 0]
	if stats.DailyGated[0] != 1 {
		t.Errorf("DailyGated[0] = %d, want 1", stats.DailyGated[0])
	}
	if stats.DailyGated[1] != 1 {
		t.Errorf("DailyGated[1] = %d, want 1", stats.DailyGated[1])
	}

	// Daily skipped: [today=1, 0, ...]
	if stats.DailySkipped[0] != 1 {
		t.Errorf("DailySkipped[0] = %d, want 1", stats.DailySkipped[0])
	}

	// Daily errors: [today=0, yesterday=1, ...]
	if stats.DailyErrors[0] != 0 {
		t.Errorf("DailyErrors[0] = %d, want 0", stats.DailyErrors[0])
	}
	if stats.DailyErrors[1] != 1 {
		t.Errorf("DailyErrors[1] = %d, want 1", stats.DailyErrors[1])
	}
}

func TestComputeOpsStats_RecentEventsLimit(t *testing.T) {
	now := time.Now()
	since := now.Add(-24 * time.Hour)

	var records []HistoryRecord
	for i := 0; i < 100; i++ {
		records = append(records, HistoryRecord{
			SessionID: "s",
			Timestamp: now.Add(-time.Duration(i) * time.Minute),
			Source:    "daemon",
			Status:    HistoryStatusSkippedBusy,
		})
	}

	stats := ComputeOpsStats(records, since, 7)

	if len(stats.RecentEvents) != 50 {
		t.Errorf("RecentEvents len = %d, want 50 (limit)", len(stats.RecentEvents))
	}
}

func TestComputeOpsStats_WindowFiltering(t *testing.T) {
	now := time.Now()

	records := []HistoryRecord{
		{SessionID: "recent", Timestamp: now.Add(-1 * time.Hour), Source: "daemon", Status: HistoryStatusSkippedBusy},
		{SessionID: "old", Timestamp: now.Add(-48 * time.Hour), Source: "daemon", Status: HistoryStatusSkippedBusy},
	}

	// 24h window should only include the recent record.
	stats := ComputeOpsStats(records, now.Add(-24*time.Hour), 7)
	if stats.SkippedBusy != 1 {
		t.Errorf("SkippedBusy = %d, want 1 (only recent)", stats.SkippedBusy)
	}

	// 72h window should include both.
	stats = ComputeOpsStats(records, now.Add(-72*time.Hour), 7)
	if stats.SkippedBusy != 2 {
		t.Errorf("SkippedBusy = %d, want 2 (both)", stats.SkippedBusy)
	}
}
