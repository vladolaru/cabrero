package pipeline

import (
	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
)

// Runner orchestrates the analysis pipeline for single or batched sessions.
// Hook fields allow testing without LLM calls — when nil, the real
// package-level functions are used.
type Runner struct {
	Config       PipelineConfig
	MaxBatchSize int // 0 means DefaultMaxBatchSize
	OnStatus     func(sessionID string, event BatchEvent)

	// Testing hooks — when nil, package-level functions are used.
	ParseSessionFunc func(sessionID string) (*parser.Digest, error)
	AggregateFunc    func(sessionID string, project string) (*patterns.AggregatorOutput, error)
	ClassifyFunc     func(sessionID string, cfg PipelineConfig) (*ClassifierResult, error)
	EvalFunc         func(sessionID string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, error)
	EvalBatchFunc    func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, error)
}

// NewRunner creates a Runner with default (nil) hooks.
func NewRunner(cfg PipelineConfig) *Runner {
	return &Runner{Config: cfg}
}

func (r *Runner) log() Logger {
	return r.Config.logger()
}

func (r *Runner) maxBatch() int {
	if r.MaxBatchSize > 0 {
		return r.MaxBatchSize
	}
	return DefaultMaxBatchSize
}

func (r *Runner) emit(sessionID string, event BatchEvent) {
	if r.OnStatus != nil {
		r.OnStatus(sessionID, event)
	}
}
