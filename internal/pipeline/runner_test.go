package pipeline

import (
	"context"
	"fmt"
	"testing"

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
