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

	// All hook fields should be nil (defaults to package-level functions).
	if r.ParseSessionFunc != nil {
		t.Error("ParseSessionFunc should be nil by default")
	}
	if r.AggregateFunc != nil {
		t.Error("AggregateFunc should be nil by default")
	}
	if r.ClassifyFunc != nil {
		t.Error("ClassifyFunc should be nil by default")
	}
	if r.EvalFunc != nil {
		t.Error("EvalFunc should be nil by default")
	}
	if r.EvalBatchFunc != nil {
		t.Error("EvalBatchFunc should be nil by default")
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
	r := NewRunner(PipelineConfig{})
	r.ClassifyFunc = func(sid string, cfg PipelineConfig) (*ClassifierResult, error) {
		hookCalled = true
		return &ClassifierResult{
			Digest:           &parser.Digest{SessionID: sid},
			ClassifierOutput: &ClassifierOutput{SessionID: sid, Triage: "clean"},
		}, nil
	}

	result, err := r.classify("test-session")
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
	r := NewRunner(PipelineConfig{})
	r.EvalFunc = func(sid string, d *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, error) {
		hookCalled = true
		return &EvaluatorOutput{SessionID: sid}, nil
	}

	result, err := r.evalOne("test-session", &parser.Digest{}, &ClassifierOutput{})
	if err != nil {
		t.Fatalf("evalOne: %v", err)
	}
	if !hookCalled {
		t.Error("EvalFunc hook not called")
	}
	if result.SessionID != "test-session" {
		t.Errorf("SessionID = %q, want 'test-session'", result.SessionID)
	}
}

func TestRunner_EvalMany_UsesHook(t *testing.T) {
	hookCalled := false
	r := NewRunner(PipelineConfig{})
	r.EvalBatchFunc = func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, error) {
		hookCalled = true
		return &EvaluatorOutput{SessionID: "batch"}, nil
	}

	result, err := r.evalMany([]BatchSession{{SessionID: "s1"}})
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
	r := NewRunner(PipelineConfig{})
	r.ParseSessionFunc = func(sid string) (*parser.Digest, error) {
		hookCalled = true
		return &parser.Digest{SessionID: sid}, nil
	}

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
	r := NewRunner(PipelineConfig{})
	r.AggregateFunc = func(sid string, project string) (*patterns.AggregatorOutput, error) {
		hookCalled = true
		return &patterns.AggregatorOutput{}, nil
	}

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
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ParseSessionFunc = func(sessionID string) (*parser.Digest, error) {
		parseCalled = true
		return &parser.Digest{SessionID: sessionID}, nil
	}
	r.ClassifyFunc = func(_ string, _ PipelineConfig) (*ClassifierResult, error) {
		t.Fatal("ClassifyFunc should not be called in dry-run")
		return nil, nil
	}

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
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ParseSessionFunc = func(sessionID string) (*parser.Digest, error) {
		return &parser.Digest{SessionID: sessionID}, nil
	}
	r.ClassifyFunc = fakeClassifyClean
	r.EvalFunc = func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
		evalCalled = true
		return nil, nil
	}

	result, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if evalCalled {
		t.Error("EvalFunc called for clean session")
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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ParseSessionFunc = func(sessionID string) (*parser.Digest, error) {
		return &parser.Digest{SessionID: sessionID}, nil
	}
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalFunc = fakeEvalWithProposals(2)

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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyEvaluate

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
	r := NewRunner(PipelineConfig{Logger: spy})
	r.ParseSessionFunc = func(sessionID string) (*parser.Digest, error) {
		return &parser.Digest{SessionID: sessionID}, nil
	}
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalFunc = fakeEvalWithProposals(1)

	r.RunOne(context.Background(), sid, false)

	if len(spy.infos) == 0 {
		t.Error("expected Info calls on spy logger")
	}
}

func TestRunGroup_AllClean(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-clean00000001")
	s2 := createBatchSession(t, "rg-clean00000002")

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyClean
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

func TestRunGroup_SingleEvalUsesEvalFunc(t *testing.T) {
	setupBatchStore(t)
	s := createBatchSession(t, "rg-single000001")

	singleCalled := false
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalFunc = func(sid string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
		singleCalled = true
		return &EvaluatorOutput{SessionID: sid, Proposals: []Proposal{}}, nil
	}

	results := r.RunGroup(context.Background(), []BatchSession{s})

	if !singleCalled {
		t.Error("EvalFunc not called")
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
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalBatchFunc = func(sessions []BatchSession, _ PipelineConfig) (*EvaluatorOutput, error) {
		batchCalled = true
		return &EvaluatorOutput{Proposals: []Proposal{
			{ID: "prop-rg-bat-0", Type: "skill_improvement", Confidence: "high", Rationale: "t"},
		}}, nil
	}

	r.RunGroup(context.Background(), []BatchSession{s1, s2})

	if !batchCalled {
		t.Error("EvalBatchFunc not called")
	}
}

func TestRunGroup_ClassifierError(t *testing.T) {
	setupBatchStore(t)
	s := createBatchSession(t, "rg-classerr00001")

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = func(_ string, _ PipelineConfig) (*ClassifierResult, error) {
		return nil, fmt.Errorf("classifier boom")
	}

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
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = func(sid string, cfg PipelineConfig) (*ClassifierResult, error) {
		callCount++
		if callCount == 1 {
			cancel()
		}
		return fakeClassifyClean(sid, cfg)
	}

	results := r.RunGroup(ctx, []BatchSession{s1, s2})
	if results[1].Status != "error" {
		t.Errorf("s2 Status = %q, want 'error'", results[1].Status)
	}
}

func TestRunGroup_OnStatusEmitsEvents(t *testing.T) {
	setupBatchStore(t)
	s := createBatchSession(t, "rg-events0000001")

	var events []BatchEvent
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalFunc = fakeEvalNoProposals
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
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.MaxBatchSize = 1
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalFunc = func(sid string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
		singleCount++
		return &EvaluatorOutput{SessionID: sid, Proposals: []Proposal{}}, nil
	}

	r.RunGroup(context.Background(), []BatchSession{s1, s2})

	if singleCount != 2 {
		t.Errorf("EvalFunc called %d times, want 2", singleCount)
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

	r := NewRunner(cfg)
	r.Source = "cli-run"
	r.ParseSessionFunc = func(sessionID string) (*parser.Digest, error) {
		return &parser.Digest{SessionID: sessionID}, nil
	}
	r.ClassifyFunc = fakeClassifyClean

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
	if rec.ClassifierModel != ClassifierModel {
		t.Errorf("ClassifierModel = %q, want %q", rec.ClassifierModel, ClassifierModel)
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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.Source = "daemon"
	r.ParseSessionFunc = func(sessionID string) (*parser.Digest, error) {
		return &parser.Digest{SessionID: sessionID}, nil
	}
	r.ClassifyFunc = func(sessionID string, cfg PipelineConfig) (*ClassifierResult, error) {
		return &ClassifierResult{
			Digest:           &parser.Digest{SessionID: sessionID},
			ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: "evaluate", PromptVersion: "classifier-v3"},
		}, nil
	}
	r.EvalFunc = func(sessionID string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
		return &EvaluatorOutput{
			SessionID:     sessionID,
			PromptVersion: "evaluator-v3",
			Proposals: []Proposal{
				{ID: fmt.Sprintf("prop-%s-0", shortID(sessionID)), Type: "skill_improvement", Confidence: "high", Rationale: "test"},
				{ID: fmt.Sprintf("prop-%s-1", shortID(sessionID)), Type: "skill_improvement", Confidence: "high", Rationale: "test"},
			},
		}, nil
	}

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
	if rec.EvaluatorModel != EvaluatorModel {
		t.Errorf("EvaluatorModel = %q, want %q", rec.EvaluatorModel, EvaluatorModel)
	}
}

func TestRunOne_ClassifierError_WritesHistory(t *testing.T) {
	setupBatchStore(t)
	sid := "hist-clerr0000001"
	createBatchSession(t, sid)

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.Source = "cli-run"
	r.ClassifyFunc = func(_ string, _ PipelineConfig) (*ClassifierResult, error) {
		return nil, fmt.Errorf("classifier boom")
	}

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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.Source = "cli-run"
	r.ParseSessionFunc = func(sessionID string) (*parser.Digest, error) {
		return &parser.Digest{SessionID: sessionID}, nil
	}
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalFunc = func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
		return nil, fmt.Errorf("evaluator boom")
	}

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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.Source = "cli-run"
	r.ParseSessionFunc = func(sessionID string) (*parser.Digest, error) {
		return &parser.Digest{SessionID: sessionID}, nil
	}

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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.Source = "cli-backfill"
	// s1 and s3 evaluate, s2 is clean.
	r.ClassifyFunc = func(sessionID string, cfg PipelineConfig) (*ClassifierResult, error) {
		triage := "evaluate"
		if sessionID == s2.SessionID {
			triage = "clean"
		}
		return &ClassifierResult{
			Digest:           &parser.Digest{SessionID: sessionID},
			ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: triage, PromptVersion: "classifier-v3"},
		}, nil
	}
	r.EvalBatchFunc = func(sessions []BatchSession, _ PipelineConfig) (*EvaluatorOutput, error) {
		var proposals []Proposal
		for _, s := range sessions {
			proposals = append(proposals, Proposal{
				ID: fmt.Sprintf("prop-%s-0", shortID(s.SessionID)),
				Type: "skill_improvement", Confidence: "high", Rationale: "test",
			})
		}
		return &EvaluatorOutput{Proposals: proposals, PromptVersion: "evaluator-v3"}, nil
	}

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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.Source = "daemon"
	r.ClassifyFunc = func(_ string, _ PipelineConfig) (*ClassifierResult, error) {
		return nil, fmt.Errorf("batch classifier boom")
	}

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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.Source = "daemon"
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalFunc = func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
		return nil, fmt.Errorf("eval single boom")
	}

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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.Source = "daemon"
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalBatchFunc = func(_ []BatchSession, _ PipelineConfig) (*EvaluatorOutput, error) {
		return nil, fmt.Errorf("eval batch boom")
	}

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
		if !strings.Contains(rec.ErrorDetail, "eval batch boom") {
			t.Errorf("[%s] ErrorDetail = %q, want to contain 'eval batch boom'", sid, rec.ErrorDetail)
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

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.Source = "cli-backfill"
	r.ClassifyFunc = fakeClassifyClean

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
