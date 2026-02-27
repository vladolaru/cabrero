package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestPromptVersionParsing(t *testing.T) {
	tests := []struct {
		filename string
		name     string
		version  string
	}{
		{"classifier-v3.txt", "classifier", "v3"},
		{"evaluator-v3.txt", "evaluator", "v3"},
		{"apply-v1.txt", "apply", "v1"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			name, ver := parsePromptFilename(tt.filename)
			if name != tt.name {
				t.Errorf("name = %q, want %q", name, tt.name)
			}
			if ver != tt.version {
				t.Errorf("version = %q, want %q", ver, tt.version)
			}
		})
	}
}

func TestComputeStatsFromHistory_Empty(t *testing.T) {
	stats := ComputeStatsFromHistory(nil, time.Time{})
	if stats.TotalRuns != 0 {
		t.Errorf("TotalRuns = %d, want 0", stats.TotalRuns)
	}
	if stats.MedianClassifier != 0 {
		t.Errorf("MedianClassifier = %v, want 0", stats.MedianClassifier)
	}
}

func TestComputeStatsFromHistory_Percentiles(t *testing.T) {
	now := time.Now()
	// Create 10 records with known classifier durations: 100ms, 200ms, ..., 1000ms
	var records []HistoryRecord
	for i := 1; i <= 10; i++ {
		records = append(records, HistoryRecord{
			SessionID:            "sess-" + string(rune('0'+i)),
			Timestamp:            now.Add(-time.Duration(i) * time.Minute),
			Source:               "daemon",
			Status:               "processed",
			Triage:               "evaluate",
			ClassifierDurationNs: int64(time.Duration(i*100) * time.Millisecond),
			EvaluatorDurationNs:  int64(time.Duration(i*500) * time.Millisecond),
			TotalDurationNs:      int64(time.Duration(i*600) * time.Millisecond),
		})
	}

	stats := ComputeStatsFromHistory(records, time.Time{})

	if stats.TotalRuns != 10 {
		t.Errorf("TotalRuns = %d, want 10", stats.TotalRuns)
	}
	if stats.ClassifierRuns != 10 {
		t.Errorf("ClassifierRuns = %d, want 10", stats.ClassifierRuns)
	}
	if stats.EvaluatorRuns != 10 {
		t.Errorf("EvaluatorRuns = %d, want 10", stats.EvaluatorRuns)
	}

	// Median (p50) of [100,200,...,1000] ms → index 5 → 600ms
	expectedMedian := 600 * time.Millisecond
	if stats.MedianClassifier != expectedMedian {
		t.Errorf("MedianClassifier = %v, want %v", stats.MedianClassifier, expectedMedian)
	}

	// P95 of 10 items → index 9 → 1000ms
	expectedP95 := 1000 * time.Millisecond
	if stats.P95Classifier != expectedP95 {
		t.Errorf("P95Classifier = %v, want %v", stats.P95Classifier, expectedP95)
	}
}

func TestComputeStatsFromHistory_SkipRate(t *testing.T) {
	now := time.Now()
	records := []HistoryRecord{
		{SessionID: "s1", Timestamp: now, Triage: "clean", Status: "processed", ClassifierDurationNs: int64(time.Second), TotalDurationNs: int64(time.Second)},
		{SessionID: "s2", Timestamp: now, Triage: "clean", Status: "processed", ClassifierDurationNs: int64(time.Second), TotalDurationNs: int64(time.Second)},
		{SessionID: "s3", Timestamp: now, Triage: "evaluate", Status: "processed", ClassifierDurationNs: int64(time.Second), EvaluatorDurationNs: int64(2 * time.Second), TotalDurationNs: int64(3 * time.Second)},
	}

	stats := ComputeStatsFromHistory(records, time.Time{})
	if stats.EvaluatorSkipped != 2 {
		t.Errorf("EvaluatorSkipped = %d, want 2", stats.EvaluatorSkipped)
	}
	if stats.EvaluatorRuns != 1 {
		t.Errorf("EvaluatorRuns = %d, want 1", stats.EvaluatorRuns)
	}
}

func TestComputeStatsFromHistory_TimeWindow(t *testing.T) {
	now := time.Now()
	records := []HistoryRecord{
		{SessionID: "old", Timestamp: now.Add(-48 * time.Hour), Source: "daemon", Status: "processed", ClassifierDurationNs: int64(time.Second), TotalDurationNs: int64(time.Second)},
		{SessionID: "recent", Timestamp: now.Add(-1 * time.Hour), Source: "daemon", Status: "processed", ClassifierDurationNs: int64(time.Second), TotalDurationNs: int64(time.Second)},
	}

	// Only include records from last 24 hours.
	stats := ComputeStatsFromHistory(records, now.Add(-24*time.Hour))
	if stats.TotalRuns != 1 {
		t.Errorf("TotalRuns = %d, want 1", stats.TotalRuns)
	}
}

func TestComputeStatsFromHistory_SourceBreakdown(t *testing.T) {
	now := time.Now()
	records := []HistoryRecord{
		{SessionID: "s1", Timestamp: now, Source: "daemon", Status: "processed", TotalDurationNs: 1},
		{SessionID: "s2", Timestamp: now, Source: "daemon", Status: "processed", TotalDurationNs: 1},
		{SessionID: "s3", Timestamp: now, Source: "cli-run", Status: "processed", TotalDurationNs: 1},
		{SessionID: "s4", Timestamp: now, Source: "cli-backfill", Status: "processed", TotalDurationNs: 1},
	}

	stats := ComputeStatsFromHistory(records, time.Time{})
	if stats.DaemonRuns != 2 {
		t.Errorf("DaemonRuns = %d, want 2", stats.DaemonRuns)
	}
	if stats.CLIRuns != 1 {
		t.Errorf("CLIRuns = %d, want 1", stats.CLIRuns)
	}
	if stats.BackfillRuns != 1 {
		t.Errorf("BackfillRuns = %d, want 1", stats.BackfillRuns)
	}
}

func TestComputeStatsFromHistory_BatchVsSingle(t *testing.T) {
	now := time.Now()
	records := []HistoryRecord{
		{SessionID: "s1", Timestamp: now, BatchMode: true, Status: "processed", TotalDurationNs: 1},
		{SessionID: "s2", Timestamp: now, BatchMode: true, Status: "processed", TotalDurationNs: 1},
		{SessionID: "s3", Timestamp: now, BatchMode: false, Status: "processed", TotalDurationNs: 1},
	}

	stats := ComputeStatsFromHistory(records, time.Time{})
	if stats.BatchRuns != 2 {
		t.Errorf("BatchRuns = %d, want 2", stats.BatchRuns)
	}
	if stats.SingleRuns != 1 {
		t.Errorf("SingleRuns = %d, want 1", stats.SingleRuns)
	}
}

func TestComputeStatsFromHistory_RetryCount(t *testing.T) {
	now := time.Now()
	records := []HistoryRecord{
		{SessionID: "s1", Timestamp: now, PreviousStatus: "error", Status: "processed", TotalDurationNs: 1},
		{SessionID: "s2", Timestamp: now, PreviousStatus: "queued", Status: "processed", TotalDurationNs: 1},
		{SessionID: "s3", Timestamp: now, PreviousStatus: "error", Status: "error", TotalDurationNs: 1},
	}

	stats := ComputeStatsFromHistory(records, time.Time{})
	if stats.RetryRuns != 2 {
		t.Errorf("RetryRuns = %d, want 2", stats.RetryRuns)
	}
	if stats.ErrorRuns != 1 {
		t.Errorf("ErrorRuns = %d, want 1", stats.ErrorRuns)
	}
}

func TestComputeStatsFromHistory_SkippedBusy(t *testing.T) {
	now := time.Now()
	records := []HistoryRecord{
		{SessionID: "s1", Timestamp: now, Source: "daemon", Status: "skipped_busy"},
		{SessionID: "s2", Timestamp: now, Source: "daemon", Status: "skipped_busy"},
		{SessionID: "s3", Timestamp: now, Source: "daemon", Status: "processed", TotalDurationNs: 1},
	}

	stats := ComputeStatsFromHistory(records, time.Time{})
	if stats.SkippedBusy != 2 {
		t.Errorf("SkippedBusy = %d, want 2", stats.SkippedBusy)
	}
	if stats.TotalRuns != 3 {
		t.Errorf("TotalRuns = %d, want 3", stats.TotalRuns)
	}
}

func TestSparklineBuckets(t *testing.T) {
	// Pin to a fixed reference time to avoid midnight flakiness.
	ref := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	sessions := []time.Time{
		ref.Add(-1 * time.Hour),  // today
		ref.Add(-25 * time.Hour), // yesterday
		ref.Add(-25 * time.Hour), // yesterday
		ref.Add(-49 * time.Hour), // 2 days ago
	}
	buckets := bucketSessionsByDay(sessions, 3, ref)
	// Day 0 (today): 1 session, Day 1 (yesterday): 2 sessions, Day 2: 1 session
	if len(buckets) != 3 {
		t.Fatalf("buckets len = %d, want 3", len(buckets))
	}
	if buckets[0] != 1 {
		t.Errorf("buckets[0] = %d, want 1", buckets[0])
	}
	if buckets[1] != 2 {
		t.Errorf("buckets[1] = %d, want 2", buckets[1])
	}
	if buckets[2] != 1 {
		t.Errorf("buckets[2] = %d, want 1", buckets[2])
	}
}

func TestListCleanupRunsFromHistory(t *testing.T) {
	dir := t.TempDir()
	origPath := cleanupHistoryPath
	cleanupHistoryPath = func() string { return filepath.Join(dir, "cleanup_history.jsonl") }
	defer func() { cleanupHistoryPath = origPath }()

	_ = AppendCleanupHistory(CleanupRecord{
		Timestamp:       time.Now().Add(-1 * time.Hour),
		DurationNs:      int64(47 * time.Second),
		ProposalsBefore: 64,
		ProposalsAfter:  12,
		Decisions: []CuratorDecision{
			{ProposalID: "p1", Action: "cull"},
			{ProposalID: "p2", Action: "auto-reject"},
		},
	})

	runs, err := ListCleanupRunsFromHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	run := runs[0]
	if run.Source != "cleanup" {
		t.Errorf("Source: got %q, want cleanup", run.Source)
	}
	if run.ProposalCount != 52 { // 64 - 12 = 52 archived
		t.Errorf("ProposalCount: got %d, want 52", run.ProposalCount)
	}
}

func TestGatherPipelineStatsFromSessions_CountsArchivedOutcomes(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)
	store.Init()

	archivedDir := filepath.Join(tmp, "proposals", "archived")
	os.MkdirAll(archivedDir, 0o755)

	now := time.Now()
	// Write two archived proposals: one approved, one rejected.
	writeArchivedProp := func(id, outcome string) {
		data := fmt.Sprintf(`{"sessionId":"sess-1","outcome":%q,"archivedAt":%q}`,
			outcome, now.Format(time.RFC3339))
		os.WriteFile(filepath.Join(archivedDir, id+".json"), []byte(data), 0o644)
	}
	writeArchivedProp("prop-aaa-1", "approved")
	writeArchivedProp("prop-bbb-1", "rejected")

	stats, err := GatherPipelineStatsFromSessions(nil, nil, 30)
	if err != nil {
		t.Fatalf("GatherPipelineStatsFromSessions: %v", err)
	}
	if stats.ProposalsApproved != 1 {
		t.Errorf("ProposalsApproved = %d, want 1", stats.ProposalsApproved)
	}
	if stats.ProposalsRejected != 1 {
		t.Errorf("ProposalsRejected = %d, want 1", stats.ProposalsRejected)
	}
}

func TestListAcceptanceRateByPromptVersion(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)
	store.Init()

	// Write history: session sess-001 used evaluator-v3, generated 2 proposals.
	histPath := filepath.Join(tmp, "run_history.jsonl")
	now := time.Now()
	rec := HistoryRecord{
		SessionID:              "sess-001",
		Timestamp:              now,
		Status:                 "processed",
		EvaluatorPromptVersion: "evaluator-v3",
		ProposalCount:          2,
	}
	data, _ := json.Marshal(rec)
	os.WriteFile(histPath, append(data, '\n'), 0o644)

	// Write archived proposals for sess-001: 1 approved, 1 rejected.
	archivedDir := filepath.Join(tmp, "proposals", "archived")
	os.MkdirAll(archivedDir, 0o755)
	writeArchived := func(id, outcome string) {
		d := fmt.Sprintf(`{"sessionId":"sess-001","outcome":%q,"archivedAt":%q}`,
			outcome, now.Format(time.RFC3339))
		os.WriteFile(filepath.Join(archivedDir, id+".json"), []byte(d), 0o644)
	}
	writeArchived("prop-001-1", "approved")
	writeArchived("prop-001-2", "rejected")

	stats, err := ListAcceptanceRateByPromptVersion()
	if err != nil {
		t.Fatalf("ListAcceptanceRateByPromptVersion: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("len(stats) = %d, want 1", len(stats))
	}
	s := stats[0]
	if s.PromptVersion != "evaluator-v3" {
		t.Errorf("PromptVersion = %q", s.PromptVersion)
	}
	if s.Approved != 1 || s.Rejected != 1 {
		t.Errorf("Approved=%d Rejected=%d, want 1/1", s.Approved, s.Rejected)
	}
	if s.SampleSize != 2 {
		t.Errorf("SampleSize = %d, want 2", s.SampleSize)
	}
	// AcceptanceRate = 1/2 = 0.5
	if s.AcceptanceRate < 0.49 || s.AcceptanceRate > 0.51 {
		t.Errorf("AcceptanceRate = %f, want ~0.5", s.AcceptanceRate)
	}
}

func TestEstimateStageDurations(t *testing.T) {
	tmpDir := t.TempDir()
	base := time.Now().Add(-10 * time.Second)

	// Create files with known mtimes.
	digestPath := filepath.Join(tmpDir, "digest.json")
	classifierPath := filepath.Join(tmpDir, "classifier.json")
	evaluatorPath := filepath.Join(tmpDir, "evaluator.json")

	os.WriteFile(digestPath, []byte("{}"), 0o644)
	os.WriteFile(classifierPath, []byte("{}"), 0o644)
	os.WriteFile(evaluatorPath, []byte("{}"), 0o644)

	os.Chtimes(digestPath, base.Add(2*time.Second), base.Add(2*time.Second))
	os.Chtimes(classifierPath, base.Add(5*time.Second), base.Add(5*time.Second))
	os.Chtimes(evaluatorPath, base.Add(8*time.Second), base.Add(8*time.Second))

	dInfo, _ := os.Stat(digestPath)
	cInfo, _ := os.Stat(classifierPath)
	eInfo, _ := os.Stat(evaluatorPath)

	run := &PipelineRun{HasDigest: true, HasClassifier: true, HasEvaluator: true}
	estimateStageDurations(run, base, dInfo, cInfo, eInfo)

	if run.ParseDuration < time.Second {
		t.Errorf("ParseDuration = %v, want >= 1s", run.ParseDuration)
	}
	if run.ClassifierDuration < 2*time.Second {
		t.Errorf("ClassifierDuration = %v, want >= 2s", run.ClassifierDuration)
	}
	if run.EvaluatorDuration < 2*time.Second {
		t.Errorf("EvaluatorDuration = %v, want >= 2s", run.EvaluatorDuration)
	}
}

func TestEstimateStageDurations_PartialStages(t *testing.T) {
	tmpDir := t.TempDir()
	base := time.Now().Add(-5 * time.Second)

	digestPath := filepath.Join(tmpDir, "digest.json")
	os.WriteFile(digestPath, []byte("{}"), 0o644)
	os.Chtimes(digestPath, base.Add(2*time.Second), base.Add(2*time.Second))
	dInfo, _ := os.Stat(digestPath)

	// Only digest, no classifier or evaluator.
	run := &PipelineRun{HasDigest: true, HasClassifier: false, HasEvaluator: false}
	estimateStageDurations(run, base, dInfo, nil, nil)

	if run.ParseDuration < time.Second {
		t.Errorf("ParseDuration = %v, want >= 1s", run.ParseDuration)
	}
	if run.ClassifierDuration != 0 {
		t.Errorf("ClassifierDuration = %v, want 0", run.ClassifierDuration)
	}
	if run.EvaluatorDuration != 0 {
		t.Errorf("EvaluatorDuration = %v, want 0", run.EvaluatorDuration)
	}
}
