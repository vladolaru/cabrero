package pipeline

import (
	"fmt"
	"os"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
	"github.com/vladolaru/cabrero/internal/store"
)

// Logger receives pipeline progress and diagnostic messages.
type Logger interface {
	Info(format string, args ...any)
	Error(format string, args ...any)
}

// PipelineStages is the injection interface for pipeline stage execution.
// Implement this (or embed TestStages) to override stages in tests.
// Production code uses the Runner's built-in implementations.
type PipelineStages interface {
	ParseSession(sessionID string) (*parser.Digest, error)
	Aggregate(sessionID, project string) (*patterns.AggregatorOutput, error)
	Classify(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error)
	EvalOne(sessionID string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
	EvalBatch(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
}

// TestStages implements PipelineStages via optional function overrides.
// Unset fields cause the Runner to fall back to its built-in production logic.
// Use NewRunnerWithStages to inject into a Runner.
type TestStages struct {
	ParseSessionFunc func(sessionID string) (*parser.Digest, error)
	AggregateFunc    func(sessionID, project string) (*patterns.AggregatorOutput, error)
	ClassifyFunc     func(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error)
	EvalOneFunc      func(sessionID string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
	EvalBatchFunc    func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
}

func (s TestStages) ParseSession(sid string) (*parser.Digest, error) {
	if s.ParseSessionFunc != nil {
		return s.ParseSessionFunc(sid)
	}
	return nil, nil // nil signals "use Runner's built-in"
}

func (s TestStages) Aggregate(sid, project string) (*patterns.AggregatorOutput, error) {
	if s.AggregateFunc != nil {
		return s.AggregateFunc(sid, project)
	}
	return nil, nil
}

func (s TestStages) Classify(sid string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
	if s.ClassifyFunc != nil {
		return s.ClassifyFunc(sid, cfg)
	}
	return nil, nil, nil
}

func (s TestStages) EvalOne(sid string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
	if s.EvalOneFunc != nil {
		return s.EvalOneFunc(sid, digest, co, cfg)
	}
	return nil, nil, nil
}

func (s TestStages) EvalBatch(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
	if s.EvalBatchFunc != nil {
		return s.EvalBatchFunc(sessions, cfg)
	}
	return nil, nil, nil
}

// stdLogger writes Info to stdout and Error to stderr.
// Format strings carry their own indentation (e.g. "  Parsing session %s...").
type stdLogger struct{}

func (stdLogger) Info(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

func (stdLogger) Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// discardLogger silently drops all messages.
type discardLogger struct{}

func (discardLogger) Info(string, ...any)  {}
func (discardLogger) Error(string, ...any) {}

// Model name constants for each pipeline entity.
const (
	DefaultCuratorCheckModel = "claude-haiku-4-5"
	DefaultApplyModel        = "claude-sonnet-4-6"
	DefaultChatModel         = "claude-sonnet-4-6"
	DefaultMetaModel         = "claude-opus-4-6"
	DefaultCuratorModel      = "claude-sonnet-4-6"
)

// PipelineConfig controls LLM invocation parameters.
type PipelineConfig struct {
	ClassifierModel    string
	EvaluatorModel     string
	ClassifierMaxTurns int
	EvaluatorMaxTurns  int
	ClassifierTimeout  time.Duration
	EvaluatorTimeout   time.Duration
	MaxConcurrentInvocations int    // 0 means unlimited; daemon default is 3
	MaxLLMRetries           int    // max retries for retriable LLM failures (JSON parse errors); 0 = no retry
	Logger                  Logger // nil defaults to stdLogger (stdout/stderr)
	Debug                   bool   // persist CC sessions for classifier/evaluator

	// Curator stage (daily cleanup).
	CuratorModel        string
	CuratorMaxTurns     int
	CuratorTimeout      time.Duration
	CuratorCheckTimeout time.Duration // for the Haiku batch check

	// Per-entity model config.
	CuratorCheckModel string // default: DefaultCuratorCheckModel (Haiku)
	ApplyModel        string // default: DefaultApplyModel (Sonnet)
	ChatModel         string // default: DefaultChatModel (Sonnet)
	MetaModel         string // default: DefaultMetaModel (Opus)

	// Meta-pipeline thresholds.
	MetaRejectionRateThreshold float64       // default 0.30
	MetaClassifierFPRThreshold float64       // default 0.25
	MetaMinSamples             int           // default 5
	MetaCooldownDays           int           // default 14
	MetaMaxTurns               int           // default 20
	MetaTimeout                time.Duration // default 5 * time.Minute
}

// logger returns the configured Logger, falling back to stdLogger.
func (c PipelineConfig) logger() Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return stdLogger{}
}

// DefaultPipelineConfig returns production defaults.
// Fields are resolved: config.json override → compile-time default.
func DefaultPipelineConfig() PipelineConfig {
	overrides := store.ReadPipelineOverrides()
	classifierModel := DefaultClassifierModel
	if overrides.ClassifierModel != "" {
		classifierModel = overrides.ClassifierModel
	}
	evaluatorModel := DefaultEvaluatorModel
	if overrides.EvaluatorModel != "" {
		evaluatorModel = overrides.EvaluatorModel
	}
	classifierTimeout := 3 * time.Minute
	if d, err := time.ParseDuration(overrides.ClassifierTimeout); err == nil && d > 0 {
		classifierTimeout = d
	}
	evaluatorTimeout := 7 * time.Minute
	if d, err := time.ParseDuration(overrides.EvaluatorTimeout); err == nil && d > 0 {
		evaluatorTimeout = d
	}
	return PipelineConfig{
		ClassifierModel:          classifierModel,
		EvaluatorModel:           evaluatorModel,
		ClassifierMaxTurns:       15,
		EvaluatorMaxTurns:        20,
		ClassifierTimeout:        classifierTimeout,
		EvaluatorTimeout:         evaluatorTimeout,
		MaxConcurrentInvocations: 3,
		MaxLLMRetries:            1,
		CuratorModel:             DefaultCuratorModel,
		CuratorMaxTurns:          15,
		CuratorTimeout:           5 * time.Minute,
		CuratorCheckTimeout:      2 * time.Minute,
		CuratorCheckModel:          DefaultCuratorCheckModel,
		ApplyModel:                 DefaultApplyModel,
		ChatModel:                  DefaultChatModel,
		MetaModel:                  DefaultMetaModel,
		MetaRejectionRateThreshold: 0.30,
		MetaClassifierFPRThreshold: 0.25,
		MetaMinSamples:             5,
		MetaCooldownDays:           14,
		MetaMaxTurns:               20,
		MetaTimeout:                5 * time.Minute,
	}
}

// RunResult holds the outcome of a pipeline run.
type RunResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
	ClassifierOutput *ClassifierOutput
	// EvaluatorOutput is nil when DryRun is true, when the Classifier triages
	// the session as "clean", or when the Evaluator stage was not reached.
	EvaluatorOutput *EvaluatorOutput
	DryRun          bool
}

// ClassifierResult holds the output of the pre-parser through Classifier stages.
type ClassifierResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
	ClassifierOutput *ClassifierOutput
	ParseDuration    time.Duration // wall-clock time for pre-parse + aggregation
}

// preParseResult holds the output of the pre-parse and aggregation stages.
type preParseResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
}

