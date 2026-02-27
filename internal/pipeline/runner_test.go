package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
	"github.com/vladolaru/cabrero/internal/store"
)

func TestNewRunner_DefaultHooks(t *testing.T) {
	r := NewRunner(PipelineConfig{})

	// stages should be nil by default (production logic used directly).
	if r.stages != nil {
		t.Error("stages should be nil by default")
	}
}

func TestNewRunner_CustomLogger(t *testing.T) {
	spy := &spyLogger{}
	cfg := PipelineConfig{Logger: spy}
	r := NewRunner(cfg)

	// Logger should be accessible via the config.
	log := r.Config.logger()
	if log != spy {
		t.Errorf("logger = %T, want *spyLogger", log)
	}
}

func TestRunner_MaxBatch(t *testing.T) {
	r := NewRunner(PipelineConfig{})
	if r.maxBatch() != DefaultMaxBatchSize {
		t.Errorf("maxBatch() = %d, want %d", r.maxBatch(), DefaultMaxBatchSize)
	}

	r.MaxBatchSize = 3
	if r.maxBatch() != 3 {
		t.Errorf("maxBatch() = %d, want 3", r.maxBatch())
	}
}

func TestRunner_Classify_UsesHook(t *testing.T) {
	hookCalled := false
	r := NewRunnerWithStages(PipelineConfig{}, TestStages{
		ClassifyFunc: func(sid string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			hookCalled = true
			return &ClassifierResult{
				Digest:           &parser.Digest{SessionID: sid},
				ClassifierOutput: &ClassifierOutput{SessionID: sid, Triage: "clean"},
			}, nil, nil
		},
	})

	result, _, err := r.classify("test-session")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if !hookCalled {
		t.Error("ClassifyFunc hook not called")
	}
	if result.ClassifierOutput.Triage != "clean" {
		t.Errorf("Triage = %q, want 'clean'", result.ClassifierOutput.Triage)
	}
}

func TestRunner_EvalOne_UsesHook(t *testing.T) {
	hookCalled := false
	r := NewRunnerWithStages(PipelineConfig{}, TestStages{
		EvalOneFunc: func(sid string, d *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			hookCalled = true
			return &EvaluatorOutput{SessionID: sid}, nil, nil
		},
	})

	result, _, err := r.evalOne("test-session", &parser.Digest{}, &ClassifierOutput{})
	if err != nil {
		t.Fatalf("evalOne: %v", err)
	}
	if !hookCalled {
		t.Error("EvalOneFunc hook not called")
	}
	if result.SessionID != "test-session" {
		t.Errorf("SessionID = %q, want 'test-session'", result.SessionID)
	}
}

func TestRunner_EvalMany_UsesHook(t *testing.T) {
	hookCalled := false
	r := NewRunnerWithStages(PipelineConfig{}, TestStages{
		EvalBatchFunc: func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			hookCalled = true
			return &EvaluatorOutput{SessionID: "batch"}, nil, nil
		},
	})

	result, _, err := r.evalMany([]BatchSession{{SessionID: "s1"}})
	if err != nil {
		t.Fatalf("evalMany: %v", err)
	}
	if !hookCalled {
		t.Error("EvalBatchFunc hook not called")
	}
	if result.SessionID != "batch" {
		t.Errorf("SessionID = %q, want 'batch'", result.SessionID)
	}
}

func TestRunner_ParseSession_UsesHook(t *testing.T) {
	hookCalled := false
	r := NewRunnerWithStages(PipelineConfig{}, TestStages{
		ParseSessionFunc: func(sid string) (*parser.Digest, error) {
			hookCalled = true
			return &parser.Digest{SessionID: sid}, nil
		},
	})

	result, err := r.parseSession("test-session")
	if err != nil {
		t.Fatalf("parseSession: %v", err)
	}
	if !hookCalled {
		t.Error("ParseSessionFunc hook not called")
	}
	if result.SessionID != "test-session" {
		t.Errorf("SessionID = %q, want 'test-session'", result.SessionID)
	}
}

func TestRunner_Aggregate_UsesHook(t *testing.T) {
	hookCalled := false
	r := NewRunnerWithStages(PipelineConfig{}, TestStages{
		AggregateFunc: func(sid string, project string) (*patterns.AggregatorOutput, error) {
			hookCalled = true
			return &patterns.AggregatorOutput{}, nil
		},
	})

	_, err := r.aggregate("test-session", "my-project")
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if !hookCalled {
		t.Error("AggregateFunc hook not called")
	}
}

func TestRunOne_DryRun(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-dryrun001"
	createBatchSession(t, sid)
	writeTranscript(t, sid, []string{"uuid-1"})

	parseCalled := false
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			parseCalled = true
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: func(_ string, _ PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			t.Fatal("ClassifyFunc should not be called in dry-run")
			return nil, nil, nil
		},
	})

	result, err := r.RunOne(context.Background(), sid, true)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if !parseCalled {
		t.Error("ParseSessionFunc not called")
	}
	if !result.DryRun {
		t.Error("DryRun should be true")
	}
}

func TestRunOne_CleanTriage_SkipsEvaluator(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-clean00001"
	createBatchSession(t, sid)

	evalCalled := false
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: fakeClassifyClean,
		EvalOneFunc: func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			evalCalled = true
			return nil, nil, nil
		},
	})

	result, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if evalCalled {
		t.Error("EvalOneFunc called for clean session")
	}
	if result.ClassifierOutput.Triage != "clean" {
		t.Errorf("Triage = %q, want 'clean'", result.ClassifierOutput.Triage)
	}
	if result.EvaluatorOutput != nil {
		t.Error("EvaluatorOutput should be nil for clean session")
	}
}

func TestRunOne_Evaluate_ProducesProposals(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-eval000001"
	createBatchSession(t, sid)

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: fakeClassifyEvaluate,
		EvalOneFunc:  fakeEvalWithProposals(2),
	})

	result, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if result.EvaluatorOutput == nil {
		t.Fatal("EvaluatorOutput is nil")
	}
	if len(result.EvaluatorOutput.Proposals) != 2 {
		t.Errorf("got %d proposals, want 2", len(result.EvaluatorOutput.Proposals))
	}

	// Verify status marked as processed.
	meta, err := store.ReadMetadata(sid)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != store.StatusProcessed {
		t.Errorf("Status = %q, want %q", meta.Status, store.StatusProcessed)
	}
}

func TestRunOne_ContextCancel(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-cancel0001"
	createBatchSession(t, sid)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
	})

	_, err := r.RunOne(ctx, sid, false)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRunOne_LoggerReceivesMessages(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-logger00001"
	createBatchSession(t, sid)

	spy := &spyLogger{}
	r := NewRunnerWithStages(PipelineConfig{Logger: spy}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: fakeClassifyEvaluate,
		EvalOneFunc:  fakeEvalWithProposals(1),
	})

	r.RunOne(context.Background(), sid, false)

	if len(spy.infos) == 0 {
		t.Error("expected Info calls on spy logger")
	}
}

func TestRunGroup_AllClean(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-clean00000001")
	s2 := createBatchSession(t, "rg-clean00000002")

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyClean,
	})
	results := r.RunGroup(context.Background(), []BatchSession{s1, s2})

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, res := range results {
		if res.Status != "processed" {
			t.Errorf("%s: Status = %q, want 'processed'", res.SessionID, res.Status)
		}
		if res.Triage != "clean" {
			t.Errorf("%s: Triage = %q, want 'clean'", res.SessionID, res.Triage)
		}
	}
}

func TestRunGroup_SingleEvalUsesEvalOneFunc(t *testing.T) {
	setupBatchStore(t)
	s := createBatchSession(t, "rg-single000001")

	singleCalled := false
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
		EvalOneFunc: func(sid string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			singleCalled = true
			return &EvaluatorOutput{SessionID: sid, Proposals: []Proposal{}}, nil, nil
		},
	})

	results := r.RunGroup(context.Background(), []BatchSession{s})

	if !singleCalled {
		t.Error("EvalOneFunc not called")
	}
	if results[0].Status != "processed" {
		t.Errorf("Status = %q, want 'processed'", results[0].Status)
	}
}

func TestRunGroup_BatchEval(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-batch00000001")
	s2 := createBatchSession(t, "rg-batch00000002")

	batchCalled := false
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
		EvalBatchFunc: func(sessions []BatchSession, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			batchCalled = true
			return &EvaluatorOutput{Proposals: []Proposal{
				{ID: "prop-rg-bat-0", Type: "skill_improvement", Confidence: "high", Rationale: "t"},
			}}, nil, nil
		},
	})

	r.RunGroup(context.Background(), []BatchSession{s1, s2})

	if !batchCalled {
		t.Error("EvalBatchFunc not called")
	}
}

func TestRunGroup_ClassifierError(t *testing.T) {
	setupBatchStore(t)
	s := createBatchSession(t, "rg-classerr00001")

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: func(_ string, _ PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			return nil, nil, fmt.Errorf("classifier boom")
		},
	})

	results := r.RunGroup(context.Background(), []BatchSession{s})
	if results[0].Status != "error" {
		t.Errorf("Status = %q, want 'error'", results[0].Status)
	}
	if results[0].Error == nil {
		t.Error("Error is nil, want non-nil")
	}
}

func TestRunGroup_ContextCancel(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-cancel0000001")
	s2 := createBatchSession(t, "rg-cancel0000002")

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: func(sid string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			callCount++
			if callCount == 1 {
				cancel()
			}
			return fakeClassifyClean(sid, cfg)
		},
	})

	results := r.RunGroup(ctx, []BatchSession{s1, s2})
	if results[1].Status != "error" {
		t.Errorf("s2 Status = %q, want 'error'", results[1].Status)
	}
}

func TestRunGroup_OnStatusEmitsEvents(t *testing.T) {
	setupBatchStore(t)
	s := createBatchSession(t, "rg-events0000001")

	var events []BatchEvent
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
		EvalOneFunc:  fakeEvalNoProposals,
	})
	r.OnStatus = func(_ string, event BatchEvent) {
		events = append(events, event)
	}

	r.RunGroup(context.Background(), []BatchSession{s})

	hasClassifier := false
	hasEvaluator := false
	for _, e := range events {
		if e.Type == "classifier_done" && e.Triage == "evaluate" {
			hasClassifier = true
		}
		if e.Type == "evaluator_done" {
			hasEvaluator = true
		}
	}
	if !hasClassifier {
		t.Error("missing classifier_done event")
	}
	if !hasEvaluator {
		t.Error("missing evaluator_done event")
	}
}

func TestRunGroup_MaxBatchSizeForcesSingleEval(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-maxbatch00001")
	s2 := createBatchSession(t, "rg-maxbatch00002")

	singleCount := 0
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
		EvalOneFunc: func(sid string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			singleCount++
			return &EvaluatorOutput{SessionID: sid, Proposals: []Proposal{}}, nil, nil
		},
	})
	r.MaxBatchSize = 1

	r.RunGroup(context.Background(), []BatchSession{s1, s2})

	if singleCount != 2 {
		t.Errorf("EvalOneFunc called %d times, want 2", singleCount)
	}
}

// --- History integration tests ---
// These verify that RunOne and RunGroup actually append HistoryRecords
// with correct fields. The existing setupBatchStore(t) sets HOME to a
// temp dir, so historyPath() points to the right place.

// readHistoryForTest reads the history file from the store root.
func readHistoryForTest(t *testing.T) []HistoryRecord {
	t.Helper()
	records, err := ReadHistory()
	if err != nil {
		t.Fatalf("ReadHistory: %v", err)
	}
	return records
}

func findRecord(records []HistoryRecord, sessionID string) *HistoryRecord {
	for i := range records {
		if records[i].SessionID == sessionID {
			return &records[i]
		}
	}
	return nil
}

func TestRunOne_CleanTriage_WritesHistory(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-clean0000001"
	createBatchSession(t, sid)

	cfg := DefaultPipelineConfig()
	cfg.Logger = &discardLogger{}

	r := NewRunnerWithStages(cfg, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: fakeClassifyClean,
	})
	r.Source = "cli-run"

	_, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record found for %s", sid)
	}

	if rec.Source != "cli-run" {
		t.Errorf("Source = %q, want %q", rec.Source, "cli-run")
	}
	if rec.Triage != "clean" {
		t.Errorf("Triage = %q, want %q", rec.Triage, "clean")
	}
	if rec.Status != "processed" {
		t.Errorf("Status = %q, want %q", rec.Status, "processed")
	}
	if rec.BatchMode {
		t.Error("BatchMode = true, want false")
	}
	if rec.ClassifierDurationNs <= 0 {
		t.Errorf("ClassifierDurationNs = %d, want > 0", rec.ClassifierDurationNs)
	}
	if rec.EvaluatorDurationNs != 0 {
		t.Errorf("EvaluatorDurationNs = %d, want 0 (clean session skips evaluator)", rec.EvaluatorDurationNs)
	}
	if rec.TotalDurationNs <= 0 {
		t.Errorf("TotalDurationNs = %d, want > 0", rec.TotalDurationNs)
	}
	if rec.EvaluatorModel != "" {
		t.Errorf("EvaluatorModel = %q, want empty (clean session)", rec.EvaluatorModel)
	}
	if rec.ClassifierModel != DefaultClassifierModel {
		t.Errorf("ClassifierModel = %q, want %q", rec.ClassifierModel, DefaultClassifierModel)
	}
	if rec.PreviousStatus != "queued" {
		t.Errorf("PreviousStatus = %q, want %q", rec.PreviousStatus, "queued")
	}
	if rec.ClassifierMaxTurns != cfg.ClassifierMaxTurns {
		t.Errorf("ClassifierMaxTurns = %d, want %d", rec.ClassifierMaxTurns, cfg.ClassifierMaxTurns)
	}
}

func TestRunOne_Evaluate_WritesHistory(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-eval00000001"
	createBatchSession(t, sid)

	cfg := DefaultPipelineConfig()
	cfg.Logger = &discardLogger{}
	r := NewRunnerWithStages(cfg, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: func(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			return &ClassifierResult{
				Digest:           &parser.Digest{SessionID: sessionID},
				ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: "evaluate", PromptVersion: "classifier-v3"},
			}, nil, nil
		},
		EvalOneFunc: func(sessionID string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			return &EvaluatorOutput{
				SessionID:     sessionID,
				PromptVersion: "evaluator-v3",
				Proposals: []Proposal{
					{ID: fmt.Sprintf("prop-%s-0", store.ShortSessionID(sessionID)), Type: "skill_improvement", Confidence: "high", Rationale: "test"},
					{ID: fmt.Sprintf("prop-%s-1", store.ShortSessionID(sessionID)), Type: "skill_improvement", Confidence: "high", Rationale: "test"},
				},
			}, nil, nil
		},
	})
	r.Source = "daemon"

	_, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record found for %s", sid)
	}

	if rec.Source != "daemon" {
		t.Errorf("Source = %q, want %q", rec.Source, "daemon")
	}
	if rec.Triage != "evaluate" {
		t.Errorf("Triage = %q, want %q", rec.Triage, "evaluate")
	}
	if rec.Status != "processed" {
		t.Errorf("Status = %q, want %q", rec.Status, "processed")
	}
	if rec.ProposalCount != 2 {
		t.Errorf("ProposalCount = %d, want 2", rec.ProposalCount)
	}
	if rec.ClassifierDurationNs <= 0 {
		t.Errorf("ClassifierDurationNs = %d, want > 0", rec.ClassifierDurationNs)
	}
	if rec.EvaluatorDurationNs <= 0 {
		t.Errorf("EvaluatorDurationNs = %d, want > 0", rec.EvaluatorDurationNs)
	}
	if rec.TotalDurationNs <= 0 {
		t.Errorf("TotalDurationNs = %d, want > 0", rec.TotalDurationNs)
	}
	if rec.ClassifierPromptVersion != "classifier-v3" {
		t.Errorf("ClassifierPromptVersion = %q, want %q", rec.ClassifierPromptVersion, "classifier-v3")
	}
	if rec.EvaluatorPromptVersion != "evaluator-v3" {
		t.Errorf("EvaluatorPromptVersion = %q, want %q", rec.EvaluatorPromptVersion, "evaluator-v3")
	}
	if rec.EvaluatorModel != DefaultEvaluatorModel {
		t.Errorf("EvaluatorModel = %q, want %q", rec.EvaluatorModel, DefaultEvaluatorModel)
	}
}

func TestRunOne_ClassifierError_WritesHistory(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-clerr0000001"
	createBatchSession(t, sid)

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: func(_ string, _ PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			return nil, nil, fmt.Errorf("classifier boom")
		},
	})
	r.Source = "cli-run"

	_, err := r.RunOne(context.Background(), sid, false)
	if err == nil {
		t.Fatal("expected error")
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record found for %s after classifier error", sid)
	}

	if rec.Status != "error" {
		t.Errorf("Status = %q, want %q", rec.Status, "error")
	}
	if !strings.Contains(rec.ErrorDetail, "classifier boom") {
		t.Errorf("ErrorDetail = %q, want to contain 'classifier boom'", rec.ErrorDetail)
	}
	if rec.ClassifierDurationNs <= 0 {
		t.Errorf("ClassifierDurationNs = %d, want > 0 (even on error)", rec.ClassifierDurationNs)
	}
	if rec.TotalDurationNs <= 0 {
		t.Errorf("TotalDurationNs = %d, want > 0", rec.TotalDurationNs)
	}
}

func TestRunOne_EvaluatorError_WritesHistory(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-everr0000001"
	createBatchSession(t, sid)

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: fakeClassifyEvaluate,
		EvalOneFunc: func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			return nil, nil, fmt.Errorf("evaluator boom")
		},
	})
	r.Source = "cli-run"

	_, err := r.RunOne(context.Background(), sid, false)
	if err == nil {
		t.Fatal("expected error")
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record found for %s after evaluator error", sid)
	}

	if rec.Status != "error" {
		t.Errorf("Status = %q, want %q", rec.Status, "error")
	}
	if rec.Triage != "evaluate" {
		t.Errorf("Triage = %q, want %q", rec.Triage, "evaluate")
	}
	if !strings.Contains(rec.ErrorDetail, "evaluator boom") {
		t.Errorf("ErrorDetail = %q, want to contain 'evaluator boom'", rec.ErrorDetail)
	}
	if rec.ClassifierDurationNs <= 0 {
		t.Errorf("ClassifierDurationNs = %d, want > 0", rec.ClassifierDurationNs)
	}
	if rec.EvaluatorDurationNs <= 0 {
		t.Errorf("EvaluatorDurationNs = %d, want > 0 (even on error)", rec.EvaluatorDurationNs)
	}
}

func TestRunOne_DryRun_NoHistory(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-dryrun000001"
	createBatchSession(t, sid)
	writeTranscript(t, sid, []string{"uuid-1"})

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
	})
	r.Source = "cli-run"

	_, err := r.RunOne(context.Background(), sid, true)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}

	records := readHistoryForTest(t)
	if rec := findRecord(records, sid); rec != nil {
		t.Errorf("dry run should not write history, but found record for %s", sid)
	}
}

func TestRunGroup_WritesHistoryWithBatchContext(t *testing.T) {
	setupBatchStore(t)
	// Session IDs must have distinct 6-char prefixes for proposal partitioning.
	s1 := createBatchSession(t, "aaaaaa-hist-b0001")
	s2 := createBatchSession(t, "bbbbbb-hist-b0002")
	s3 := createBatchSession(t, "cccccc-hist-b0003")

	// s1 and s3 evaluate, s2 is clean.
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: func(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			triage := "evaluate"
			if sessionID == s2.SessionID {
				triage = "clean"
			}
			return &ClassifierResult{
				Digest:           &parser.Digest{SessionID: sessionID},
				ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: triage, PromptVersion: "classifier-v3"},
			}, nil, nil
		},
		EvalBatchFunc: func(sessions []BatchSession, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			var proposals []Proposal
			for _, s := range sessions {
				proposals = append(proposals, Proposal{
					ID:         fmt.Sprintf("prop-%s-0", store.ShortSessionID(s.SessionID)),
					Type:       "skill_improvement",
					Confidence: "high",
					Rationale:  "test",
				})
			}
			return &EvaluatorOutput{Proposals: proposals, PromptVersion: "evaluator-v3"}, nil, nil
		},
	})
	r.Source = "cli-backfill"

	r.RunGroup(context.Background(), []BatchSession{s1, s2, s3})

	records := readHistoryForTest(t)
	if len(records) != 3 {
		t.Fatalf("expected 3 history records, got %d", len(records))
	}

	// Check all records have batch context.
	for _, sid := range []string{s1.SessionID, s2.SessionID, s3.SessionID} {
		rec := findRecord(records, sid)
		if rec == nil {
			t.Fatalf("missing history record for %s", sid)
		}

		if rec.Source != "cli-backfill" {
			t.Errorf("[%s] Source = %q, want %q", sid, rec.Source, "cli-backfill")
		}
		if !rec.BatchMode {
			t.Errorf("[%s] BatchMode = false, want true", sid)
		}
		if rec.BatchSize != 3 {
			t.Errorf("[%s] BatchSize = %d, want 3", sid, rec.BatchSize)
		}
		if len(rec.BatchSessionIDs) != 3 {
			t.Errorf("[%s] BatchSessionIDs len = %d, want 3", sid, len(rec.BatchSessionIDs))
		}
	}

	// Check clean session.
	cleanRec := findRecord(records, s2.SessionID)
	if cleanRec.Triage != "clean" {
		t.Errorf("clean session Triage = %q, want %q", cleanRec.Triage, "clean")
	}
	if cleanRec.EvaluatorDurationNs != 0 {
		t.Errorf("clean session EvaluatorDurationNs = %d, want 0", cleanRec.EvaluatorDurationNs)
	}

	// Check evaluated sessions.
	for _, sid := range []string{s1.SessionID, s3.SessionID} {
		rec := findRecord(records, sid)
		if rec.Triage != "evaluate" {
			t.Errorf("[%s] Triage = %q, want %q", sid, rec.Triage, "evaluate")
		}
		if rec.Status != "processed" {
			t.Errorf("[%s] Status = %q, want %q", sid, rec.Status, "processed")
		}
		if rec.ProposalCount != 1 {
			t.Errorf("[%s] ProposalCount = %d, want 1", sid, rec.ProposalCount)
		}
		if rec.EvaluatorDurationNs <= 0 {
			t.Errorf("[%s] EvaluatorDurationNs = %d, want > 0", sid, rec.EvaluatorDurationNs)
		}
		if rec.EvaluatorPromptVersion != "evaluator-v3" {
			t.Errorf("[%s] EvaluatorPromptVersion = %q, want %q", sid, rec.EvaluatorPromptVersion, "evaluator-v3")
		}
	}
}

func TestRunGroup_ClassifierError_WritesHistory(t *testing.T) {
	setupBatchStore(t)
	s := createBatchSession(t, "hist-grperr000001")

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: func(_ string, _ PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			return nil, nil, fmt.Errorf("batch classifier boom")
		},
	})
	r.Source = "daemon"

	r.RunGroup(context.Background(), []BatchSession{s})

	records := readHistoryForTest(t)
	rec := findRecord(records, s.SessionID)
	if rec == nil {
		t.Fatalf("no history record after RunGroup classifier error")
	}

	if rec.Status != "error" {
		t.Errorf("Status = %q, want %q", rec.Status, "error")
	}
	if !strings.Contains(rec.ErrorDetail, "batch classifier boom") {
		t.Errorf("ErrorDetail = %q, want to contain 'batch classifier boom'", rec.ErrorDetail)
	}
	if !rec.BatchMode {
		t.Error("BatchMode = false, want true")
	}
}

func TestRunGroup_EvalSingleError_WritesHistory(t *testing.T) {
	setupBatchStore(t)
	s := createBatchSession(t, "hist-evserr000001")

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
		EvalOneFunc: func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			return nil, nil, fmt.Errorf("eval single boom")
		},
	})
	r.Source = "daemon"

	r.RunGroup(context.Background(), []BatchSession{s})

	records := readHistoryForTest(t)
	rec := findRecord(records, s.SessionID)
	if rec == nil {
		t.Fatalf("no history record after eval single error")
	}

	if rec.Status != "error" {
		t.Errorf("Status = %q, want %q", rec.Status, "error")
	}
	if rec.Triage != "evaluate" {
		t.Errorf("Triage = %q, want %q", rec.Triage, "evaluate")
	}
	if !strings.Contains(rec.ErrorDetail, "eval single boom") {
		t.Errorf("ErrorDetail = %q, want to contain 'eval single boom'", rec.ErrorDetail)
	}
}

func TestRunGroup_EvalBatchError_WritesHistory(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "hist-evberr000001")
	s2 := createBatchSession(t, "hist-evberr000002")

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
		EvalBatchFunc: func(_ []BatchSession, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			return nil, nil, fmt.Errorf("eval batch boom")
		},
		// Batch failure triggers per-session fallback; make that fail too.
		EvalOneFunc: func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			return nil, nil, fmt.Errorf("eval per-session boom")
		},
	})
	r.Source = "daemon"

	r.RunGroup(context.Background(), []BatchSession{s1, s2})

	records := readHistoryForTest(t)
	for _, sid := range []string{s1.SessionID, s2.SessionID} {
		rec := findRecord(records, sid)
		if rec == nil {
			t.Fatalf("no history record for %s after batch eval error", sid)
		}
		if rec.Status != "error" {
			t.Errorf("[%s] Status = %q, want %q", sid, rec.Status, "error")
		}
		if !strings.Contains(rec.ErrorDetail, "eval per-session boom") {
			t.Errorf("[%s] ErrorDetail = %q, want to contain 'eval per-session boom'", sid, rec.ErrorDetail)
		}
	}
}

func TestRunOne_PreviousStatus_CapturedBeforeMarkError(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-prevst000001"
	// Create session with "imported" status to verify PreviousStatus captures it.
	rawDir := store.RawDir(sid)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := store.Metadata{
		SessionID:      sid,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CaptureTrigger: "session-end",
		Status:         "imported",
	}
	if err := store.WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyClean,
	})
	r.Source = "cli-backfill"

	_, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record found for %s", sid)
	}

	// PreviousStatus should be "imported" (what it was BEFORE pipeline ran).
	if rec.PreviousStatus != "imported" {
		t.Errorf("PreviousStatus = %q, want %q", rec.PreviousStatus, "imported")
	}
	if rec.CaptureTrigger != "session-end" {
		t.Errorf("CaptureTrigger = %q, want %q", rec.CaptureTrigger, "session-end")
	}
}

// --- Usage tracking integration tests ---

func TestRunOne_UsageTrackedInHistory(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-usage0000001"
	createBatchSession(t, sid)

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: func(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			return &ClassifierResult{
				Digest:           &parser.Digest{SessionID: sessionID},
				ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: "evaluate"},
			}, &ClaudeResult{
				SessionID:    "cc-classify-sess",
				NumTurns:     3,
				InputTokens:  5000,
				OutputTokens: 1500,
				TotalCostUSD: 0.01,
			}, nil
		},
		EvalOneFunc: func(sessionID string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			return &EvaluatorOutput{
				SessionID: sessionID,
				Proposals: []Proposal{
					{ID: fmt.Sprintf("prop-%s-0", store.ShortSessionID(sessionID)), Type: "skill_improvement", Confidence: "high", Rationale: "test"},
				},
			}, &ClaudeResult{
				SessionID:           "cc-eval-sess",
				NumTurns:            8,
				InputTokens:         10000,
				OutputTokens:        3000,
				TotalCostUSD:        0.03,
				CacheCreationTokens: 200,
				CacheReadTokens:     4000,
			}, nil
		},
	})
	r.Source = "cli-run"

	_, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record found for %s", sid)
	}

	// Verify classifier usage.
	if rec.ClassifierUsage == nil {
		t.Fatal("ClassifierUsage is nil")
	}
	if rec.ClassifierUsage.CCSessionID != "cc-classify-sess" {
		t.Errorf("ClassifierUsage.CCSessionID = %q, want %q", rec.ClassifierUsage.CCSessionID, "cc-classify-sess")
	}
	if rec.ClassifierUsage.NumTurns != 3 {
		t.Errorf("ClassifierUsage.NumTurns = %d, want 3", rec.ClassifierUsage.NumTurns)
	}
	if rec.ClassifierUsage.InputTokens != 5000 {
		t.Errorf("ClassifierUsage.InputTokens = %d, want 5000", rec.ClassifierUsage.InputTokens)
	}
	if rec.ClassifierUsage.CostUSD != 0.01 {
		t.Errorf("ClassifierUsage.CostUSD = %f, want 0.01", rec.ClassifierUsage.CostUSD)
	}

	// Verify evaluator usage.
	if rec.EvaluatorUsage == nil {
		t.Fatal("EvaluatorUsage is nil")
	}
	if rec.EvaluatorUsage.CCSessionID != "cc-eval-sess" {
		t.Errorf("EvaluatorUsage.CCSessionID = %q, want %q", rec.EvaluatorUsage.CCSessionID, "cc-eval-sess")
	}
	if rec.EvaluatorUsage.NumTurns != 8 {
		t.Errorf("EvaluatorUsage.NumTurns = %d, want 8", rec.EvaluatorUsage.NumTurns)
	}
	if rec.EvaluatorUsage.InputTokens != 10000 {
		t.Errorf("EvaluatorUsage.InputTokens = %d, want 10000", rec.EvaluatorUsage.InputTokens)
	}
	if rec.EvaluatorUsage.CostUSD != 0.03 {
		t.Errorf("EvaluatorUsage.CostUSD = %f, want 0.03", rec.EvaluatorUsage.CostUSD)
	}
	if rec.EvaluatorUsage.CacheCreationTokens != 200 {
		t.Errorf("EvaluatorUsage.CacheCreationTokens = %d, want 200", rec.EvaluatorUsage.CacheCreationTokens)
	}
	if rec.EvaluatorUsage.CacheReadTokens != 4000 {
		t.Errorf("EvaluatorUsage.CacheReadTokens = %d, want 4000", rec.EvaluatorUsage.CacheReadTokens)
	}

	// Verify totals.
	if rec.TotalCostUSD != 0.04 {
		t.Errorf("TotalCostUSD = %f, want 0.04", rec.TotalCostUSD)
	}
	if rec.TotalInputTokens != 15000 {
		t.Errorf("TotalInputTokens = %d, want 15000", rec.TotalInputTokens)
	}
	if rec.TotalOutputTokens != 4500 {
		t.Errorf("TotalOutputTokens = %d, want 4500", rec.TotalOutputTokens)
	}
}

func TestRunOne_CleanTriage_OnlyClassifierUsage(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-cleanu000001"
	createBatchSession(t, sid)

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: func(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			return &ClassifierResult{
				Digest:           &parser.Digest{SessionID: sessionID},
				ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: "clean"},
			}, &ClaudeResult{
				SessionID:    "cc-clean-sess",
				NumTurns:     2,
				InputTokens:  3000,
				OutputTokens: 500,
				TotalCostUSD: 0.005,
			}, nil
		},
	})
	r.Source = "cli-run"

	_, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record found for %s", sid)
	}

	// Classifier usage should be present.
	if rec.ClassifierUsage == nil {
		t.Fatal("ClassifierUsage is nil")
	}
	if rec.ClassifierUsage.InputTokens != 3000 {
		t.Errorf("ClassifierUsage.InputTokens = %d, want 3000", rec.ClassifierUsage.InputTokens)
	}

	// Evaluator usage should be nil (skipped).
	if rec.EvaluatorUsage != nil {
		t.Errorf("EvaluatorUsage = %v, want nil (clean session)", rec.EvaluatorUsage)
	}

	// Totals should only reflect classifier.
	if rec.TotalCostUSD != 0.005 {
		t.Errorf("TotalCostUSD = %f, want 0.005", rec.TotalCostUSD)
	}
	if rec.TotalInputTokens != 3000 {
		t.Errorf("TotalInputTokens = %d, want 3000", rec.TotalInputTokens)
	}
}

func TestRunOne_NilClaudeResult_NoUsage(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-nilcr0000001"
	createBatchSession(t, sid)

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		// ClassifyFunc returns nil ClaudeResult (backward compatible).
		ClassifyFunc: fakeClassifyClean,
	})
	r.Source = "cli-run"

	_, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record found for %s", sid)
	}

	// No usage data when hooks return nil.
	if rec.ClassifierUsage != nil {
		t.Errorf("ClassifierUsage = %v, want nil", rec.ClassifierUsage)
	}
	if rec.EvaluatorUsage != nil {
		t.Errorf("EvaluatorUsage = %v, want nil", rec.EvaluatorUsage)
	}
	if rec.TotalCostUSD != 0 {
		t.Errorf("TotalCostUSD = %f, want 0", rec.TotalCostUSD)
	}
}

func TestRunGroup_BatchEval_UsageSplitAcrossSessions(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "aausge-batch00001")
	s2 := createBatchSession(t, "bbusge-batch00002")

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: func(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			return &ClassifierResult{
				Digest:           &parser.Digest{SessionID: sessionID},
				ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: "evaluate"},
			}, &ClaudeResult{
				SessionID:    "cc-class-" + sessionID[:6],
				InputTokens:  2000,
				OutputTokens: 500,
				TotalCostUSD: 0.005,
			}, nil
		},
		EvalBatchFunc: func(sessions []BatchSession, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			var proposals []Proposal
			for _, s := range sessions {
				proposals = append(proposals, Proposal{
					ID:         fmt.Sprintf("prop-%s-0", store.ShortSessionID(s.SessionID)),
					Type:       "skill_improvement",
					Confidence: "high",
					Rationale:  "test",
				})
			}
			return &EvaluatorOutput{Proposals: proposals}, &ClaudeResult{
				SessionID:    "cc-batch-eval",
				NumTurns:     12,
				InputTokens:  20000,
				OutputTokens: 6000,
				TotalCostUSD: 0.06,
			}, nil
		},
	})
	r.Source = "daemon"

	r.RunGroup(context.Background(), []BatchSession{s1, s2})

	records := readHistoryForTest(t)

	for _, sid := range []string{s1.SessionID, s2.SessionID} {
		rec := findRecord(records, sid)
		if rec == nil {
			t.Fatalf("no history record found for %s", sid)
		}

		// Classifier usage — unique per session.
		if rec.ClassifierUsage == nil {
			t.Fatalf("[%s] ClassifierUsage is nil", sid)
		}
		if rec.ClassifierUsage.InputTokens != 2000 {
			t.Errorf("[%s] ClassifierUsage.InputTokens = %d, want 2000", sid, rec.ClassifierUsage.InputTokens)
		}

		// Evaluator usage — split from batch.
		if rec.EvaluatorUsage == nil {
			t.Fatalf("[%s] EvaluatorUsage is nil", sid)
		}
		if rec.EvaluatorUsage.CCSessionID != "cc-batch-eval" {
			t.Errorf("[%s] EvaluatorUsage.CCSessionID = %q, want %q", sid, rec.EvaluatorUsage.CCSessionID, "cc-batch-eval")
		}
		// 20000 / 2 = 10000 per session.
		if rec.EvaluatorUsage.InputTokens != 10000 {
			t.Errorf("[%s] EvaluatorUsage.InputTokens = %d, want 10000", sid, rec.EvaluatorUsage.InputTokens)
		}
		// 6000 / 2 = 3000 per session.
		if rec.EvaluatorUsage.OutputTokens != 3000 {
			t.Errorf("[%s] EvaluatorUsage.OutputTokens = %d, want 3000", sid, rec.EvaluatorUsage.OutputTokens)
		}
		// 0.06 / 2 = 0.03 per session.
		if rec.EvaluatorUsage.CostUSD != 0.03 {
			t.Errorf("[%s] EvaluatorUsage.CostUSD = %f, want 0.03", sid, rec.EvaluatorUsage.CostUSD)
		}

		// Totals: classifier(0.005) + evaluator(0.03) = 0.035.
		if diff := rec.TotalCostUSD - 0.035; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("[%s] TotalCostUSD = %f, want ~0.035", sid, rec.TotalCostUSD)
		}
		// Totals: classifier(2000) + evaluator(10000) = 12000.
		if rec.TotalInputTokens != 12000 {
			t.Errorf("[%s] TotalInputTokens = %d, want 12000", sid, rec.TotalInputTokens)
		}
	}
}

func TestListPipelineRunsFromHistory_UsesHistoryTiming(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-listfh000001"
	createBatchSession(t, sid)

	// Write a history record with known timing.
	rec := HistoryRecord{
		SessionID:            sid,
		Timestamp:            time.Now(),
		Source:               "cli-run",
		Triage:               "evaluate",
		Status:               "processed",
		ProposalCount:        3,
		ClassifierDurationNs: int64(500 * time.Millisecond),
		EvaluatorDurationNs:  int64(2 * time.Second),
		TotalDurationNs:      int64(3 * time.Second),
	}
	if err := AppendHistory(rec); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}

	sessions, _ := store.ListSessions()
	runs, err := ListPipelineRunsFromHistory(sessions, 0)
	if err != nil {
		t.Fatalf("ListPipelineRunsFromHistory: %v", err)
	}

	// Find the run for our session.
	var found *PipelineRun
	for i := range runs {
		if runs[i].SessionID == sid {
			found = &runs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no PipelineRun found for %s", sid)
	}

	// Timing should come from history, not mtime estimation.
	if found.ClassifierDuration != 500*time.Millisecond {
		t.Errorf("ClassifierDuration = %v, want 500ms", found.ClassifierDuration)
	}
	if found.EvaluatorDuration != 2*time.Second {
		t.Errorf("EvaluatorDuration = %v, want 2s", found.EvaluatorDuration)
	}
	if found.ProposalCount != 3 {
		t.Errorf("ProposalCount = %d, want 3", found.ProposalCount)
	}
	if !found.HasClassifier {
		t.Error("HasClassifier = false, want true")
	}
	if !found.HasEvaluator {
		t.Error("HasEvaluator = false, want true")
	}
}

func TestListPipelineRunsFromHistory_FallbackToMtime(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-listmt000001"
	createBatchSession(t, sid)

	// Don't write any history record — should fall back to mtime.
	sessions, _ := store.ListSessions()
	runs, err := ListPipelineRunsFromHistory(sessions, 0)
	if err != nil {
		t.Fatalf("ListPipelineRunsFromHistory: %v", err)
	}

	var found *PipelineRun
	for i := range runs {
		if runs[i].SessionID == sid {
			found = &runs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no PipelineRun found for %s", sid)
	}

	// Without digest/classifier/evaluator files, timing should be zero.
	if found.ClassifierDuration != 0 {
		t.Errorf("ClassifierDuration = %v, want 0 (no files)", found.ClassifierDuration)
	}
	if found.HasDigest {
		t.Error("HasDigest = true, want false (no digest file)")
	}
}

func TestRunGroupEvalBatch_MismatchedProposalsMarkError(t *testing.T) {
	setupBatchStore(t)

	s1 := createBatchSession(t, "aaaaaaaa-1111-1111-1111-111111111111")
	s2 := createBatchSession(t, "bbbbbbbb-2222-2222-2222-222222222222")

	runner := NewRunnerWithStages(DefaultPipelineConfig(), TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
		EvalBatchFunc: func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			// Return proposals with IDs that don't match any session prefix.
			return &EvaluatorOutput{
				Proposals: []Proposal{
					{ID: "prop-ZZZZZZZZ-0", Type: "skill_improvement", Confidence: "high", Rationale: "orphan"},
				},
			}, nil, nil
		},
	})
	runner.Source = "test"

	results := runner.RunGroup(context.Background(), []BatchSession{s1, s2})

	// Both sessions should be marked as error because proposals didn't partition cleanly.
	for i, r := range results {
		if r.Status != "error" {
			t.Errorf("results[%d] status = %q, want %q", i, r.Status, "error")
		}
	}

	// Verify history records also show error status.
	records := readHistoryForTest(t)
	for _, sid := range []string{s1.SessionID, s2.SessionID} {
		rec := findRecord(records, sid)
		if rec == nil {
			t.Errorf("no history record for %s", sid)
			continue
		}
		if rec.Status != "error" {
			t.Errorf("history record for %s: status = %q, want %q", sid, rec.Status, "error")
		}
	}
}

func TestSplitUsageForBatch_DividesWebCounters(t *testing.T) {
	cr := &ClaudeResult{
		InputTokens:       1000,
		OutputTokens:      500,
		WebSearchRequests: 6,
		WebFetchRequests:  3,
		TotalCostUSD:      0.06,
	}

	splits := splitUsageForBatch(cr, 3)

	for i, s := range splits {
		if s.WebSearchRequests != 2 {
			t.Errorf("splits[%d].WebSearchRequests = %d, want 2", i, s.WebSearchRequests)
		}
		if s.WebFetchRequests != 1 {
			t.Errorf("splits[%d].WebFetchRequests = %d, want 1", i, s.WebFetchRequests)
		}
	}
}

// --- Source policy gate tests ---

// fakeClassifyWithSignals returns a classify function whose output includes
// skill signals. This lets tests exercise the source policy gate.
func fakeClassifyWithSignals(signals []ClassifierSkillSignal) func(string, PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
	return func(sessionID string, _ PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
		return &ClassifierResult{
			Digest: &parser.Digest{SessionID: sessionID},
			ClassifierOutput: &ClassifierOutput{
				SessionID:    sessionID,
				Triage:       "evaluate",
				SkillSignals: signals,
			},
		}, nil, nil
	}
}

func TestRunOne_SourceGate_UnclassifiedSkipsEvaluator(t *testing.T) {
	setupBatchStore(t)
	sid := "gate-unclass00001"
	createBatchSession(t, sid)

	// Register a source as unclassified (ownership == "").
	writeSources(t, []sourceEntry{
		{Name: "brainstorming", Origin: "plugin:superpowers", Ownership: "", Approach: ""},
	})

	evalCalled := false
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: fakeClassifyWithSignals([]ClassifierSkillSignal{
			{SkillName: "superpowers:brainstorming"},
		}),
		EvalOneFunc: func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			evalCalled = true
			return &EvaluatorOutput{}, nil, nil
		},
	})

	result, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if evalCalled {
		t.Error("evaluator should not be called for unclassified source")
	}
	if result.EvaluatorOutput != nil {
		t.Error("EvaluatorOutput should be nil when gated")
	}

	// Verify history records the gate reason.
	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record for %s", sid)
	}
	if rec.GateReason != "unclassified_source" {
		t.Errorf("GateReason = %q, want %q", rec.GateReason, "unclassified_source")
	}
	if rec.Status != "processed" {
		t.Errorf("Status = %q, want %q", rec.Status, "processed")
	}
	if rec.Triage != "evaluate" {
		t.Errorf("Triage = %q, want %q", rec.Triage, "evaluate")
	}
}

func TestRunOne_SourceGate_PausedSkipsEvaluator(t *testing.T) {
	setupBatchStore(t)
	sid := "gate-paused00001"
	createBatchSession(t, sid)

	// Register a source as classified but paused.
	writeSources(t, []sourceEntry{
		{Name: "brainstorming", Origin: "plugin:superpowers", Ownership: "mine", Approach: "paused"},
	})

	evalCalled := false
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: fakeClassifyWithSignals([]ClassifierSkillSignal{
			{SkillName: "superpowers:brainstorming"},
		}),
		EvalOneFunc: func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			evalCalled = true
			return &EvaluatorOutput{}, nil, nil
		},
	})

	result, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if evalCalled {
		t.Error("evaluator should not be called for paused source")
	}
	if result.EvaluatorOutput != nil {
		t.Error("EvaluatorOutput should be nil when gated")
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record for %s", sid)
	}
	if rec.GateReason != "paused_source" {
		t.Errorf("GateReason = %q, want %q", rec.GateReason, "paused_source")
	}
}

func TestRunOne_SourceGate_ClassifiedAllowsEvaluator(t *testing.T) {
	setupBatchStore(t)
	sid := "gate-allow000001"
	createBatchSession(t, sid)

	// Register a source as fully classified with iterate approach.
	writeSources(t, []sourceEntry{
		{Name: "brainstorming", Origin: "plugin:superpowers", Ownership: "mine", Approach: "iterate"},
	})

	evalCalled := false
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ParseSessionFunc: func(sessionID string) (*parser.Digest, error) {
			return &parser.Digest{SessionID: sessionID}, nil
		},
		ClassifyFunc: fakeClassifyWithSignals([]ClassifierSkillSignal{
			{SkillName: "superpowers:brainstorming"},
		}),
		EvalOneFunc: func(sessionID string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			evalCalled = true
			return &EvaluatorOutput{SessionID: sessionID, Proposals: []Proposal{}}, nil, nil
		},
	})

	_, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if !evalCalled {
		t.Error("evaluator should be called for classified source")
	}

	records := readHistoryForTest(t)
	rec := findRecord(records, sid)
	if rec == nil {
		t.Fatalf("no history record for %s", sid)
	}
	if rec.GateReason != "" {
		t.Errorf("GateReason = %q, want empty", rec.GateReason)
	}
}

func TestRunGroup_SourceGate_UnclassifiedSkipsEvaluator(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-gate-unc00001")
	s2 := createBatchSession(t, "rg-gate-cls00001")

	// s1 touches an unclassified source; s2 touches a classified source.
	writeSources(t, []sourceEntry{
		{Name: "brainstorming", Origin: "plugin:superpowers", Ownership: "", Approach: ""},
		{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate"},
	})

	evalCalled := map[string]bool{}
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: func(sessionID string, _ PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
			signals := []ClassifierSkillSignal{{SkillName: "superpowers:brainstorming"}}
			if sessionID == "rg-gate-cls00001" {
				signals = []ClassifierSkillSignal{{SkillName: "git-workflow"}}
			}
			return &ClassifierResult{
				Digest:           &parser.Digest{SessionID: sessionID},
				ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: "evaluate", SkillSignals: signals},
			}, nil, nil
		},
		EvalOneFunc: func(sessionID string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			evalCalled[sessionID] = true
			return &EvaluatorOutput{SessionID: sessionID, Proposals: []Proposal{}}, nil, nil
		},
	})

	results := r.RunGroup(context.Background(), []BatchSession{s1, s2})

	// s1 should be gated (unclassified), s2 should be evaluated.
	if results[0].Status != "processed" {
		t.Errorf("s1 status = %q, want %q", results[0].Status, "processed")
	}
	if results[1].Status != "processed" {
		t.Errorf("s2 status = %q, want %q", results[1].Status, "processed")
	}

	if evalCalled["rg-gate-unc00001"] {
		t.Error("evaluator should not be called for s1 (unclassified)")
	}
	if !evalCalled["rg-gate-cls00001"] {
		t.Error("evaluator should be called for s2 (classified)")
	}
}

// --- Batch evaluator fallback tests ---

func TestRunGroup_BatchFallback_PerSessionSuccess(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-fall-ok000001")
	s2 := createBatchSession(t, "rg-fall-ok000002")

	evalOneCalls := 0
	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
		EvalBatchFunc: func(_ []BatchSession, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			return nil, nil, fmt.Errorf("parsing evaluator batch output: invalid JSON: malformed")
		},
		EvalOneFunc: func(sessionID string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			evalOneCalls++
			return &EvaluatorOutput{SessionID: sessionID, Proposals: []Proposal{}}, nil, nil
		},
	})

	results := r.RunGroup(context.Background(), []BatchSession{s1, s2})

	// Both sessions should succeed via per-session fallback.
	if results[0].Status != "processed" {
		t.Errorf("s1 status = %q, want %q", results[0].Status, "processed")
	}
	if results[1].Status != "processed" {
		t.Errorf("s2 status = %q, want %q", results[1].Status, "processed")
	}
	if evalOneCalls != 2 {
		t.Errorf("evalOne calls = %d, want 2", evalOneCalls)
	}
}

func TestRunGroup_BatchFallback_MixedOutcomes(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-fall-mx000001")
	s2 := createBatchSession(t, "rg-fall-mx000002")

	r := NewRunnerWithStages(PipelineConfig{Logger: &discardLogger{}}, TestStages{
		ClassifyFunc: fakeClassifyEvaluate,
		EvalBatchFunc: func(_ []BatchSession, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			return nil, nil, fmt.Errorf("parsing evaluator batch output: invalid JSON: malformed")
		},
		EvalOneFunc: func(sessionID string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
			if sessionID == "rg-fall-mx000001" {
				return &EvaluatorOutput{SessionID: sessionID, Proposals: []Proposal{}}, nil, nil
			}
			return nil, nil, fmt.Errorf("evaluator failed for session 2")
		},
	})

	results := r.RunGroup(context.Background(), []BatchSession{s1, s2})

	// s1 succeeds, s2 fails — independent outcomes.
	if results[0].Status != "processed" {
		t.Errorf("s1 status = %q, want %q", results[0].Status, "processed")
	}
	if results[1].Status != "error" {
		t.Errorf("s2 status = %q, want %q", results[1].Status, "error")
	}
}

// sourceEntry is a minimal struct for writing test sources.
type sourceEntry struct {
	Name      string `json:"name"`
	Origin    string `json:"origin"`
	Ownership string `json:"ownership"`
	Approach  string `json:"approach"`
}

// writeSources writes a sources.json to the test store.
func writeSources(t *testing.T, entries []sourceEntry) {
	t.Helper()
	if err := os.MkdirAll(store.Root(), 0o755); err != nil {
		t.Fatal(err)
	}

	enc := `{"sources":[`
	for i, e := range entries {
		if i > 0 {
			enc += ","
		}
		enc += fmt.Sprintf(`{"name":%q,"origin":%q,"ownership":%q,"approach":%q,"sessionCount":0,"healthScore":-1}`,
			e.Name, e.Origin, e.Ownership, e.Approach)
	}
	enc += "]}"

	if err := os.WriteFile(store.Root()+"/sources.json", []byte(enc), 0o644); err != nil {
		t.Fatal(err)
	}
}
