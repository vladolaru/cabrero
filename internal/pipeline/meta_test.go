package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// capturingLogger captures logged messages for assertions in tests.
type capturingLogger struct {
	infoMsgs  []string
	errorMsgs []string
}

func (l *capturingLogger) Info(format string, args ...any) {
	l.infoMsgs = append(l.infoMsgs, fmt.Sprintf(format, args...))
}

func (l *capturingLogger) Error(format string, args ...any) {
	l.errorMsgs = append(l.errorMsgs, fmt.Sprintf(format, args...))
}

func TestRunMetaAnalysis_SkipsMissingTranscripts(t *testing.T) {
	var warnings []string
	logger := &capturingLogger{}
	logger.errorMsgs = warnings

	// Non-existent transcript path — should warn and skip.
	transcripts := []string{"/nonexistent/path/fake-uuid.jsonl"}
	valid := filterValidTranscripts(transcripts, logger)
	if len(valid) != 0 {
		t.Errorf("expected 0 valid transcripts, got %d", len(valid))
	}
	if len(logger.errorMsgs) == 0 {
		t.Error("expected a warning for missing transcript")
	}
	if !strings.Contains(logger.errorMsgs[0], "CC storage conventions may have changed") {
		t.Errorf("unexpected warning: %q", logger.errorMsgs[0])
	}
}

func TestRunMetaAnalysis_SkipsTranscriptWithNoToolUse(t *testing.T) {
	tmp := t.TempDir()

	// Write a transcript with no tool_use entries.
	transcriptPath := filepath.Join(tmp, "no-tools.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"hello"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"world"}]}}`
	os.WriteFile(transcriptPath, []byte(content), 0o644)

	logger := &capturingLogger{}

	valid := filterValidTranscripts([]string{transcriptPath}, logger)
	if len(valid) != 0 {
		t.Errorf("expected 0 valid transcripts, got %d", len(valid))
	}
	if len(logger.errorMsgs) == 0 {
		t.Error("expected a warning for no tool_use entries")
	}
	if !strings.Contains(logger.errorMsgs[0], "no tool_use entries") {
		t.Errorf("unexpected warning: %q", logger.errorMsgs[0])
	}
}

func TestProposalCreatedAfter(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-7 * 24 * time.Hour) // 7 days ago

	tests := []struct {
		name     string
		id       string
		wantTrue bool
	}{
		{
			name:     "meta proposal created recently (after cutoff)",
			id:       fmt.Sprintf("prop-meta-%d-1", now.Add(-1*24*time.Hour).Unix()),
			wantTrue: true,
		},
		{
			name:     "meta proposal created long ago (before cutoff)",
			id:       fmt.Sprintf("prop-meta-%d-1", now.Add(-30*24*time.Hour).Unix()),
			wantTrue: false,
		},
		{
			name:     "evaluator proposal with no timestamp in ID",
			id:       "prop-abcd1234-0",
			wantTrue: true, // no timestamp info → treat as recent (fail open)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pw := ProposalWithSession{
				Proposal: Proposal{ID: tt.id},
			}
			got := ProposalCreatedAfter(pw, cutoff)
			if got != tt.wantTrue {
				t.Errorf("ProposalCreatedAfter(%q, cutoff) = %v, want %v", tt.id, got, tt.wantTrue)
			}
		})
	}
}

func TestComputePipelineMetrics_ClassifierFPR(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)
	store.Init()

	// Write history: 4 sessions sent to evaluator, 2 generated 0 proposals (FP).
	histPath := filepath.Join(tmp, "run_history.jsonl")
	now := time.Now()
	writeRec := func(sessionID, triage string, proposals int) {
		r := HistoryRecord{
			SessionID:              sessionID,
			Timestamp:              now,
			Status:                 "processed",
			Triage:                 triage,
			ProposalCount:          proposals,
			EvaluatorPromptVersion: "evaluator-v4",
		}
		if triage == "evaluate" {
			r.EvaluatorModel = DefaultEvaluatorModel
		}
		data, _ := json.Marshal(r)
		f, _ := os.OpenFile(histPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		f.Write(append(data, '\n'))
		f.Close()
	}
	writeRec("sess-1", "evaluate", 2)
	writeRec("sess-2", "evaluate", 0) // FP
	writeRec("sess-3", "evaluate", 1)
	writeRec("sess-4", "evaluate", 0) // FP
	writeRec("sess-5", "clean", 0)    // not sent to evaluator

	cfg := DefaultPipelineConfig()
	metrics, err := ComputePipelineMetrics(cfg)
	if err != nil {
		t.Fatalf("ComputePipelineMetrics: %v", err)
	}
	// 2 FP out of 4 evaluate sessions = 0.50
	if metrics.ClassifierFPR < 0.49 || metrics.ClassifierFPR > 0.51 {
		t.Errorf("ClassifierFPR = %f, want ~0.50", metrics.ClassifierFPR)
	}
	if metrics.ClassifierFPRWindow != 30 {
		t.Errorf("ClassifierFPRWindow = %d, want 30", metrics.ClassifierFPRWindow)
	}
}
