package pipeline

import (
	"testing"
	"time"
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
