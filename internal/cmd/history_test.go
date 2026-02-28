package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
)

func TestHistory_EmptyOutput(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := historyRun(nil, &buf)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if !strings.Contains(buf.String(), "No pipeline runs") {
		t.Error("expected empty history message")
	}
}

func TestHistory_ShowsRecords(t *testing.T) {
	setupConfigTest(t)

	pipeline.AppendHistory(pipeline.HistoryRecord{
		SessionID: "sess-hist-1234",
		Timestamp: time.Now(),
		Project:   "cabrero",
		Source:    "cli-run",
		Status:    pipeline.HistoryStatusProcessed,
		Triage:    pipeline.TriageEvaluate,
	})

	var buf bytes.Buffer
	err := historyRun(nil, &buf)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if !strings.Contains(buf.String(), "sess-his") {
		t.Error("expected session ID in history output")
	}
}

func TestHistory_StatusFilter(t *testing.T) {
	setupConfigTest(t)

	pipeline.AppendHistory(pipeline.HistoryRecord{
		SessionID: "sess-ok-0001",
		Timestamp: time.Now(),
		Source:    "daemon",
		Status:    pipeline.HistoryStatusProcessed,
	})
	pipeline.AppendHistory(pipeline.HistoryRecord{
		SessionID:   "sess-err-0002",
		Timestamp:   time.Now(),
		Source:      "daemon",
		Status:      pipeline.HistoryStatusError,
		ErrorDetail: "timeout",
	})

	var buf bytes.Buffer
	err := historyRun([]string{"--status", "error"}, &buf)
	if err != nil {
		t.Fatalf("history --status error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "sess-err") {
		t.Error("expected error session in filtered output")
	}
	if strings.Contains(out, "sess-ok-") {
		t.Error("processed session should not appear in error filter")
	}
}
