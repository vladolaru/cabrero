package pipeline

import (
	"context"
	"fmt"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
	"github.com/vladolaru/cabrero/internal/store"
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

func (r *Runner) classify(sessionID string) (*ClassifierResult, error) {
	if r.ClassifyFunc != nil {
		return r.ClassifyFunc(sessionID, r.Config)
	}
	return RunThroughClassifier(sessionID, r.Config)
}

func (r *Runner) evalOne(sessionID string, digest *parser.Digest, co *ClassifierOutput) (*EvaluatorOutput, error) {
	if r.EvalFunc != nil {
		return r.EvalFunc(sessionID, digest, co, r.Config)
	}
	return RunEvaluator(sessionID, digest, co, r.Config)
}

func (r *Runner) evalMany(sessions []BatchSession) (*EvaluatorOutput, error) {
	if r.EvalBatchFunc != nil {
		return r.EvalBatchFunc(sessions, r.Config)
	}
	return RunEvaluatorBatch(sessions, r.Config)
}

func (r *Runner) parseSession(sessionID string) (*parser.Digest, error) {
	if r.ParseSessionFunc != nil {
		return r.ParseSessionFunc(sessionID)
	}
	return parser.ParseSession(sessionID)
}

func (r *Runner) aggregate(sessionID, project string) (*patterns.AggregatorOutput, error) {
	if r.AggregateFunc != nil {
		return r.AggregateFunc(sessionID, project)
	}
	return patterns.Aggregate(sessionID, project)
}

// RunOne executes the full pipeline for a single session.
// If dryRun is true, only the pre-parser runs (no LLM invocations).
func (r *Runner) RunOne(ctx context.Context, sessionID string, dryRun bool) (*RunResult, error) {
	if !store.SessionExists(sessionID) {
		return nil, fmt.Errorf("session %s not found in store", sessionID)
	}
	if err := EnsurePrompts(); err != nil {
		return nil, fmt.Errorf("ensuring prompt files: %w", err)
	}

	log := r.log()
	result := &RunResult{DryRun: dryRun}

	// Check context before pre-parse.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	pre, err := r.runPreParseAndAggregate(sessionID)
	if err != nil {
		return nil, err
	}
	result.Digest = pre.Digest
	result.AggregatorOutput = pre.AggregatorOutput

	if dryRun {
		log.Info("  Dry run — stopping after pre-parser.")
		return result, nil
	}

	// Check context before classify.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	classifierResult, err := r.classify(sessionID)
	if err != nil {
		if markErr := store.MarkError(sessionID); markErr != nil {
			log.Error("  marking error for %s: %v", sessionID, markErr)
		}
		return nil, err
	}
	result.Digest = classifierResult.Digest
	result.AggregatorOutput = classifierResult.AggregatorOutput
	result.ClassifierOutput = classifierResult.ClassifierOutput

	// Triage gate.
	if classifierResult.ClassifierOutput.Triage == "clean" {
		log.Info("  Classifier triage: clean session — skipping Evaluator")
		if markErr := store.MarkProcessed(sessionID); markErr != nil {
			log.Error("  marking processed for %s: %v", sessionID, markErr)
		}
		return result, nil
	}
	log.Info("  Classifier triage: session worth evaluating")

	// Check context before evaluator.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	log.Info("  Running Evaluator...")
	evaluatorOutput, err := r.evalOne(sessionID, classifierResult.Digest, classifierResult.ClassifierOutput)
	if err != nil {
		if markErr := store.MarkError(sessionID); markErr != nil {
			log.Error("  marking error for %s: %v", sessionID, markErr)
		}
		return nil, fmt.Errorf("evaluator failed: %w", err)
	}
	result.EvaluatorOutput = evaluatorOutput

	// Persist proposals.
	proposals := persistEvaluatorResults(sessionID, evaluatorOutput, log)
	if proposals > 0 {
		log.Info("  Evaluator: %d proposals generated", proposals)
	} else {
		reason := "no improvement signals detected"
		if evaluatorOutput.NoProposalReason != nil {
			reason = *evaluatorOutput.NoProposalReason
		}
		log.Info("  Evaluator: no proposals (%s)", reason)
	}

	if markErr := store.MarkProcessed(sessionID); markErr != nil {
		log.Error("  marking processed for %s: %v", sessionID, markErr)
	}

	return result, nil
}

// runPreParseAndAggregate delegates to hooks or real functions.
func (r *Runner) runPreParseAndAggregate(sessionID string) (*preParseResult, error) {
	log := r.log()

	log.Info("  Parsing session %s...", sessionID)
	digest, err := r.parseSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("pre-parser failed: %w", err)
	}
	if err := parser.WriteDigest(digest); err != nil {
		return nil, fmt.Errorf("writing digest: %w", err)
	}
	log.Info("  Digest written: %d entries, %d turns, %d errors, %d friction signals",
		digest.Shape.EntryCount, digest.Shape.TurnCount, len(digest.Errors), len(digest.ToolCalls.FrictionSignals))

	var aggregatorOutput *patterns.AggregatorOutput
	meta, metaErr := store.ReadMetadata(sessionID)
	if metaErr == nil && meta.Project != "" {
		log.Info("  Aggregating cross-session patterns...")
		aggregatorOutput, err = r.aggregate(sessionID, meta.Project)
		if err != nil {
			log.Info("  Warning: pattern aggregation failed: %v", err)
		} else if aggregatorOutput != nil {
			log.Info("  Found %d recurring pattern(s) across %d sessions",
				len(aggregatorOutput.Patterns), aggregatorOutput.SessionsScanned)
		}
	}

	return &preParseResult{
		Digest:           digest,
		AggregatorOutput: aggregatorOutput,
	}, nil
}
