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

	// Default: pre-parse, aggregate, then classify.
	if !store.SessionExists(sessionID) {
		return nil, fmt.Errorf("session %s not found in store", sessionID)
	}
	if err := EnsurePrompts(); err != nil {
		return nil, fmt.Errorf("ensuring prompt files: %w", err)
	}

	log := r.log()

	pre, err := r.runPreParseAndAggregate(sessionID)
	if err != nil {
		return nil, err
	}

	log.Info("  Running Classifier...")
	classifierOutput, err := RunClassifier(sessionID, pre.Digest, pre.AggregatorOutput, r.Config)
	if err != nil {
		return nil, fmt.Errorf("classifier failed: %w", err)
	}
	if err := WriteClassifierOutput(sessionID, classifierOutput); err != nil {
		return nil, fmt.Errorf("writing classifier output: %w", err)
	}
	log.Info("  Classifier: goal=%q, %d errors, %d key turns, %d skill signals, triage=%s",
		classifierOutput.Goal.Summary,
		len(classifierOutput.ErrorClassification),
		len(classifierOutput.KeyTurns),
		len(classifierOutput.SkillSignals),
		classifierOutput.Triage)

	return &ClassifierResult{
		Digest:           pre.Digest,
		AggregatorOutput: pre.AggregatorOutput,
		ClassifierOutput: classifierOutput,
	}, nil
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

	// Check context before work begins.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if dryRun {
		// Dry-run only needs pre-parse.
		pre, err := r.runPreParseAndAggregate(sessionID)
		if err != nil {
			return nil, err
		}
		result.Digest = pre.Digest
		result.AggregatorOutput = pre.AggregatorOutput
		log.Info("  Dry run — stopping after pre-parser.")
		return result, nil
	}

	// Full run: classify (includes pre-parse when no hook is set).
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

// RunGroup runs all sessions through the Classifier, then batches "evaluate"
// sessions through the Evaluator. It returns one BatchResult per input session.
func (r *Runner) RunGroup(ctx context.Context, sessions []BatchSession) []BatchResult {
	results := make([]BatchResult, len(sessions))
	indexByID := make(map[string]int, len(sessions))
	for i, s := range sessions {
		results[i] = BatchResult{SessionID: s.SessionID}
		indexByID[s.SessionID] = i
	}

	// Phase 1: Run Classifier individually on each session.
	var toEvaluate []BatchSession
	for i, s := range sessions {
		select {
		case <-ctx.Done():
			// Mark remaining sessions as errors.
			for j := i; j < len(sessions); j++ {
				results[j].Status = "error"
				results[j].Error = ctx.Err()
			}
			return results
		default:
		}

		classifierResult, err := r.classify(s.SessionID)
		if err != nil {
			results[i].Status = "error"
			results[i].Error = err
			r.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
			if markErr := store.MarkError(s.SessionID); markErr != nil {
				r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking error: %w", markErr)})
			}
			continue
		}

		triage := classifierResult.ClassifierOutput.Triage
		results[i].Triage = triage

		if triage == "clean" {
			results[i].Status = "processed"
			r.emit(s.SessionID, BatchEvent{Type: "classifier_done", Triage: "clean"})
			if markErr := store.MarkProcessed(s.SessionID); markErr != nil {
				r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
			}
			continue
		}

		// Session needs evaluation.
		r.emit(s.SessionID, BatchEvent{Type: "classifier_done", Triage: "evaluate"})
		toEvaluate = append(toEvaluate, BatchSession{
			SessionID:        s.SessionID,
			Digest:           classifierResult.Digest,
			ClassifierOutput: classifierResult.ClassifierOutput,
		})
	}

	if len(toEvaluate) == 0 {
		return results
	}

	// Phase 2: Chunk and run Evaluator.
	maxBatch := r.maxBatch()
	for chunkStart := 0; chunkStart < len(toEvaluate); chunkStart += maxBatch {
		select {
		case <-ctx.Done():
			// Mark remaining evaluate-sessions as errors.
			for j := chunkStart; j < len(toEvaluate); j++ {
				idx := indexByID[toEvaluate[j].SessionID]
				results[idx].Status = "error"
				results[idx].Error = ctx.Err()
			}
			return results
		default:
		}

		chunkEnd := chunkStart + maxBatch
		if chunkEnd > len(toEvaluate) {
			chunkEnd = len(toEvaluate)
		}
		chunk := toEvaluate[chunkStart:chunkEnd]

		if len(chunk) == 1 {
			r.runGroupEvalSingle(chunk[0], results, indexByID)
		} else {
			r.runGroupEvalBatch(chunk, results, indexByID)
		}
	}

	return results
}

func (r *Runner) runGroupEvalSingle(s BatchSession, results []BatchResult, indexByID map[string]int) {
	idx := indexByID[s.SessionID]

	evaluatorOutput, err := r.evalOne(s.SessionID, s.Digest, s.ClassifierOutput)
	if err != nil {
		results[idx].Status = "error"
		results[idx].Error = err
		r.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
		if markErr := store.MarkError(s.SessionID); markErr != nil {
			r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking error: %w", markErr)})
		}
		return
	}

	proposals := persistEvaluatorResults(s.SessionID, evaluatorOutput, r.log())
	results[idx].Status = "processed"
	results[idx].Proposals = proposals
	r.emit(s.SessionID, BatchEvent{Type: "evaluator_done"})

	if markErr := store.MarkProcessed(s.SessionID); markErr != nil {
		r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
	}
}

func (r *Runner) runGroupEvalBatch(chunk []BatchSession, results []BatchResult, indexByID map[string]int) {
	evaluatorOutput, err := r.evalMany(chunk)
	if err != nil {
		// Mark all sessions in the chunk as errors.
		for _, s := range chunk {
			idx := indexByID[s.SessionID]
			results[idx].Status = "error"
			results[idx].Error = err
			r.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
			if markErr := store.MarkError(s.SessionID); markErr != nil {
				r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking error: %w", markErr)})
			}
		}
		return
	}

	// Partition proposals by session: proposal IDs encode their session
	// via the format "prop-{first 6 chars of sessionId}-{index}".
	totalMatched := 0
	for _, s := range chunk {
		idx := indexByID[s.SessionID]
		prefix := "prop-" + shortID(s.SessionID) + "-"
		filtered := filterProposals(evaluatorOutput, prefix)
		filtered.SessionID = s.SessionID
		totalMatched += len(filtered.Proposals)

		proposals := persistEvaluatorResults(s.SessionID, filtered, r.log())
		results[idx].Status = "processed"
		results[idx].Proposals = proposals
		r.emit(s.SessionID, BatchEvent{Type: "evaluator_done"})

		if markErr := store.MarkProcessed(s.SessionID); markErr != nil {
			r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
		}
	}

	if totalMatched == 0 && len(evaluatorOutput.Proposals) > 0 {
		err := fmt.Errorf("batch: all %d proposals dropped during partitioning (possible ID format mismatch)",
			len(evaluatorOutput.Proposals))
		for _, s := range chunk {
			r.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
		}
	} else if totalMatched != len(evaluatorOutput.Proposals) {
		err := fmt.Errorf("batch: %d of %d proposals unmatched after partitioning",
			len(evaluatorOutput.Proposals)-totalMatched, len(evaluatorOutput.Proposals))
		r.emit(chunk[0].SessionID, BatchEvent{Type: "error", Error: err})
	}
}
