package pipeline

import (
	"context"
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
