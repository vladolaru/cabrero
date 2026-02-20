package pipeline

import (
	"testing"
	"time"
)

func TestPipelineRunStatus(t *testing.T) {
	run := PipelineRun{
		SessionID:     "abc123",
		Status:        "processed",
		HasDigest:     true,
		HasClassifier: true,
		HasEvaluator:  true,
	}
	if run.Status != "processed" {
		t.Errorf("status = %q, want processed", run.Status)
	}
	if !run.HasEvaluator {
		t.Error("expected HasEvaluator true")
	}
}

func TestPipelineStatsZero(t *testing.T) {
	stats := PipelineStats{}
	if stats.SessionsCaptured != 0 {
		t.Errorf("expected zero stats")
	}
}

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
		name, ver := parsePromptFilename(tt.filename)
		if name != tt.name {
			t.Errorf("parsePromptFilename(%q) name = %q, want %q", tt.filename, name, tt.name)
		}
		if ver != tt.version {
			t.Errorf("parsePromptFilename(%q) version = %q, want %q", tt.filename, ver, tt.version)
		}
	}
}

func TestSparklineBuckets(t *testing.T) {
	sessions := []time.Time{
		time.Now().Add(-1 * time.Hour),
		time.Now().Add(-25 * time.Hour),
		time.Now().Add(-25 * time.Hour),
		time.Now().Add(-49 * time.Hour),
	}
	buckets := bucketSessionsByDay(sessions, 3)
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
