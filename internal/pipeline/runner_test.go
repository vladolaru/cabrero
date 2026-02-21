package pipeline

import (
	"testing"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
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
