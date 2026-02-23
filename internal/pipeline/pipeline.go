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
}

// logger returns the configured Logger, falling back to stdLogger.
func (c PipelineConfig) logger() Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return stdLogger{}
}

// DefaultPipelineConfig returns production defaults.
// Model names are resolved: config.json override → compile-time default.
func DefaultPipelineConfig() PipelineConfig {
	models := store.ReadModelConfig()
	classifierModel := DefaultClassifierModel
	if models.ClassifierModel != "" {
		classifierModel = models.ClassifierModel
	}
	evaluatorModel := DefaultEvaluatorModel
	if models.EvaluatorModel != "" {
		evaluatorModel = models.EvaluatorModel
	}
	return PipelineConfig{
		ClassifierModel:         classifierModel,
		EvaluatorModel:          evaluatorModel,
		ClassifierMaxTurns:      15,
		EvaluatorMaxTurns:       20,
		ClassifierTimeout:       2 * time.Minute,
		EvaluatorTimeout:        5 * time.Minute,
		MaxConcurrentInvocations: 3,
		MaxLLMRetries:           1,
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

