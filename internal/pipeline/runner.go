package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
	"github.com/vladolaru/cabrero/internal/store"
)

// Runner orchestrates the analysis pipeline for single or batched sessions.
// Inject a PipelineStages implementation via NewRunnerWithStages for testing
// without LLM calls. When stages is nil, the real package-level functions are used.
type Runner struct {
	Config       PipelineConfig
	MaxBatchSize int    // 0 means DefaultMaxBatchSize
	Source       string // "daemon", "cli-run", "cli-backfill" — set by caller before RunOne/RunGroup
	OnStatus     func(sessionID string, event BatchEvent)

	// stages overrides pipeline execution for testing. nil = use built-in production logic.
	stages PipelineStages
}

// NewRunner creates a Runner with production (nil stages) defaults.
func NewRunner(cfg PipelineConfig) *Runner {
	return &Runner{Config: cfg}
}

// NewRunnerWithStages creates a Runner with a custom PipelineStages implementation.
// Use TestStages{...} to override individual stages in tests.
func NewRunnerWithStages(cfg PipelineConfig, s PipelineStages) *Runner {
	return &Runner{Config: cfg, stages: s}
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

func (r *Runner) classify(sessionID string) (*ClassifierResult, *ClaudeResult, error) {
	if r.stages != nil {
		if cr, cl, err := r.stages.Classify(sessionID, r.Config); cr != nil || cl != nil || err != nil {
			return cr, cl, err
		}
	}

	// Default: pre-parse, aggregate, then classify.
	if !store.SessionExists(sessionID) {
		return nil, nil, fmt.Errorf("session %s not found in store", sessionID)
	}
	if err := EnsurePrompts(); err != nil {
		return nil, nil, fmt.Errorf("ensuring prompt files: %w", err)
	}

	log := r.log()

	parseStart := time.Now()
	pre, err := r.runPreParseAndAggregate(sessionID)
	parseDuration := time.Since(parseStart)

	// Partial result carries parse timing through error paths so the TUI
	// can display it even for failed runs.
	partial := &ClassifierResult{ParseDuration: parseDuration}

	if err != nil {
		return partial, nil, err
	}

	log.Info("  Running Classifier...")
	classifierOutput, cr, err := RunClassifier(sessionID, pre.Digest, pre.AggregatorOutput, r.Config)
	if err != nil {
		return partial, cr, fmt.Errorf("classifier failed: %w", err)
	}
	if err := WriteClassifierOutput(sessionID, classifierOutput); err != nil {
		return partial, cr, fmt.Errorf("writing classifier output: %w", err)
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
		ParseDuration:    parseDuration,
	}, cr, nil
}

func (r *Runner) evalOne(sessionID string, digest *parser.Digest, co *ClassifierOutput) (*EvaluatorOutput, *ClaudeResult, error) {
	if r.stages != nil {
		if out, cl, err := r.stages.EvalOne(sessionID, digest, co, r.Config); out != nil || cl != nil || err != nil {
			return out, cl, err
		}
	}
	return RunEvaluator(sessionID, digest, co, r.Config)
}

func (r *Runner) evalMany(sessions []BatchSession) (*EvaluatorOutput, *ClaudeResult, error) {
	if r.stages != nil {
		if out, cl, err := r.stages.EvalBatch(sessions, r.Config); out != nil || cl != nil || err != nil {
			return out, cl, err
		}
	}
	return RunEvaluatorBatch(sessions, r.Config)
}

func (r *Runner) parseSession(sessionID string) (*parser.Digest, error) {
	if r.stages != nil {
		if d, err := r.stages.ParseSession(sessionID); d != nil || err != nil {
			if d != nil && len(d.RawUnknown) > 0 {
				r.warnFormatDrift(sessionID, d.RawUnknown)
			}
			return d, err
		}
	}
	d, err := parser.ParseSession(sessionID)
	if err != nil {
		return nil, err
	}
	if len(d.RawUnknown) > 0 {
		r.warnFormatDrift(sessionID, d.RawUnknown)
	}
	return d, nil
}

func (r *Runner) warnFormatDrift(sessionID string, unknown []parser.RawUnknown) {
	types := make(map[string]int)
	for _, u := range unknown {
		types[u.Type]++
	}
	typeList := make([]string, 0, len(types))
	for t := range types {
		typeList = append(typeList, t)
	}
	r.Config.logger().Error("WARN session %s: %d unrecognised transcript entries (types: %v) — CC format may have changed",
		sessionID, len(unknown), typeList)
}

func (r *Runner) aggregate(sessionID, project string) (*patterns.AggregatorOutput, error) {
	if r.stages != nil {
		if a, err := r.stages.Aggregate(sessionID, project); a != nil || err != nil {
			return a, err
		}
	}
	return patterns.Aggregate(sessionID, project)
}

// buildBaseRecord creates a HistoryRecord pre-filled with identity, source,
// provenance, config, and model/prompt fields. Caller fills in timing and outcome.
func (r *Runner) buildBaseRecord(sessionID string, runStart time.Time) HistoryRecord {
	meta, _ := store.ReadMetadata(sessionID)
	return HistoryRecord{
		SessionID:      sessionID,
		Timestamp:      runStart,
		Project:        store.ProjectDisplayName(meta.Project),
		Source:         r.Source,
		CaptureTrigger: meta.CaptureTrigger,
		PreviousStatus: meta.Status,

		ClassifierModel:         r.Config.ClassifierModel,
		ClassifierPromptVersion: strings.TrimSuffix(classifierPromptFile, ".txt"),
		EvaluatorModel:          r.Config.EvaluatorModel,
		EvaluatorPromptVersion:  strings.TrimSuffix(evaluatorPromptFile, ".txt"),

		ClassifierMaxTurns:  r.Config.ClassifierMaxTurns,
		EvaluatorMaxTurns:   r.Config.EvaluatorMaxTurns,
		ClassifierTimeoutNs: int64(r.Config.ClassifierTimeout),
		EvaluatorTimeoutNs:  int64(r.Config.EvaluatorTimeout),
		Debug:               r.Config.Debug,
	}
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
	runStart := time.Now()

	// Check context before work begins.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if dryRun {
		// Dry-run only needs pre-parse — no history record for dry runs.
		pre, err := r.runPreParseAndAggregate(sessionID)
		if err != nil {
			return nil, err
		}
		result.Digest = pre.Digest
		result.AggregatorOutput = pre.AggregatorOutput
		log.Info("  Dry run — stopping after pre-parser.")
		return result, nil
	}

	rec := r.buildBaseRecord(sessionID, runStart)

	// Full run: classify (includes pre-parse when no hook is set).
	// Check context before classify.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	classifyStart := time.Now()
	classifierResult, classifierCR, err := r.classify(sessionID)
	classifyDuration := time.Since(classifyStart)

	// Split parse vs classifier timing: classify() measures pre-parse internally.
	var parseDuration time.Duration
	if classifierResult != nil {
		parseDuration = classifierResult.ParseDuration
	}
	classifierOnly := classifyDuration - parseDuration

	rec.ClassifierUsage = usageFromResult(classifierCR)

	if err != nil {
		rec.ParseDurationNs = int64(parseDuration)
		rec.ClassifierDurationNs = int64(classifierOnly)
		if markErr := finalizeSessionOutcome(&rec, HistoryStatusError, err, 0, runStart); markErr != nil {
			log.Error("  marking error for %s: %v", sessionID, markErr)
		}
		return nil, err
	}
	result.Digest = classifierResult.Digest
	result.AggregatorOutput = classifierResult.AggregatorOutput
	result.ClassifierOutput = classifierResult.ClassifierOutput

	rec.ParseDurationNs = int64(parseDuration)
	rec.ClassifierDurationNs = int64(classifierOnly)
	if classifierResult.ClassifierOutput != nil && classifierResult.ClassifierOutput.PromptVersion != "" {
		rec.ClassifierPromptVersion = classifierResult.ClassifierOutput.PromptVersion
	}

	// Triage gate.
	if classifierResult.ClassifierOutput.Triage == TriageClean {
		log.Info("  Classifier triage: clean session — skipping Evaluator")
		rec.Triage = TriageClean
		rec.EvaluatorModel = ""
		rec.EvaluatorPromptVersion = ""
		if markErr := finalizeSessionOutcome(&rec, HistoryStatusProcessed, nil, 0, runStart); markErr != nil {
			log.Error("  marking processed for %s: %v", sessionID, markErr)
		}
		return result, nil
	}
	log.Info("  Classifier triage: session worth evaluating")
	rec.Triage = TriageEvaluate

	// Source policy gate: skip evaluator for unclassified or paused sources.
	gate := CheckSourcePolicy(classifierResult.ClassifierOutput)
	if !gate.Allowed {
		log.Info("  Source policy gate: %s (source %q) — skipping Evaluator", gate.Reason, gate.SourceName)
		rec.GateReason = gate.Reason
		rec.EvaluatorModel = ""
		rec.EvaluatorPromptVersion = ""
		if markErr := finalizeSessionOutcome(&rec, HistoryStatusProcessed, nil, 0, runStart); markErr != nil {
			log.Error("  marking processed for %s: %v", sessionID, markErr)
		}
		return result, nil
	}

	// Check context before evaluator.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	log.Info("  Running Evaluator...")
	evalStart := time.Now()
	evaluatorOutput, evaluatorCR, err := r.evalOne(sessionID, classifierResult.Digest, classifierResult.ClassifierOutput)
	evalDuration := time.Since(evalStart)

	rec.EvaluatorUsage = usageFromResult(evaluatorCR)

	if err != nil {
		rec.EvaluatorDurationNs = int64(evalDuration)
		if markErr := finalizeSessionOutcome(&rec, HistoryStatusError, err, 0, runStart); markErr != nil {
			log.Error("  marking error for %s: %v", sessionID, markErr)
		}
		return nil, fmt.Errorf("evaluator failed: %w", err)
	}
	result.EvaluatorOutput = evaluatorOutput

	if evaluatorOutput.PromptVersion != "" {
		rec.EvaluatorPromptVersion = evaluatorOutput.PromptVersion
	}

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

	rec.EvaluatorDurationNs = int64(evalDuration)
	if markErr := finalizeSessionOutcome(&rec, HistoryStatusProcessed, nil, len(evaluatorOutput.Proposals), runStart); markErr != nil {
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

	// Build batch context for history records.
	allSessionIDs := make([]string, len(sessions))
	for i, s := range sessions {
		allSessionIDs[i] = s.SessionID
	}

	// History records indexed by session ID — filled progressively.
	records := make(map[string]*HistoryRecord, len(sessions))
	runStarts := make(map[string]time.Time, len(sessions))

	for _, s := range sessions {
		now := time.Now()
		runStarts[s.SessionID] = now
		rec := r.buildBaseRecord(s.SessionID, now)
		rec.BatchMode = true
		rec.BatchSize = len(sessions)
		rec.BatchSessionIDs = allSessionIDs
		records[s.SessionID] = &rec
	}

	// Phase 1: Run Classifier individually on each session.
	var toEvaluate []BatchSession
	for i, s := range sessions {
		select {
		case <-ctx.Done():
			// Mark remaining sessions as errors.
			for j := i; j < len(sessions); j++ {
				results[j].Status = HistoryStatusError
				results[j].Error = ctx.Err()
			}
			return results
		default:
		}

		classifyStart := time.Now()
		classifierResult, classifierCR, err := r.classify(s.SessionID)
		classifyDuration := time.Since(classifyStart)

		// Split parse vs classifier timing.
		var parseDuration time.Duration
		if classifierResult != nil {
			parseDuration = classifierResult.ParseDuration
		}
		classifierOnly := classifyDuration - parseDuration

		rec := records[s.SessionID]
		rec.ParseDurationNs = int64(parseDuration)
		rec.ClassifierDurationNs = int64(classifierOnly)
		rec.ClassifierUsage = usageFromResult(classifierCR)

		if err != nil {
			results[i].Status = HistoryStatusError
			results[i].Error = err
			r.emit(s.SessionID, BatchEvent{Type: "error", Error: err})

			if markErr := finalizeSessionOutcome(rec, HistoryStatusError, err, 0, runStarts[s.SessionID]); markErr != nil {
				r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking error: %w", markErr)})
			}
			continue
		}

		triage := classifierResult.ClassifierOutput.Triage
		results[i].Triage = triage

		if classifierResult.ClassifierOutput.PromptVersion != "" {
			rec.ClassifierPromptVersion = classifierResult.ClassifierOutput.PromptVersion
		}

		if triage == TriageClean {
			results[i].Status = HistoryStatusProcessed
			r.emit(s.SessionID, BatchEvent{Type: "classifier_done", Triage: TriageClean})

			rec.Triage = TriageClean
			rec.EvaluatorModel = ""
			rec.EvaluatorPromptVersion = ""
			if markErr := finalizeSessionOutcome(rec, HistoryStatusProcessed, nil, 0, runStarts[s.SessionID]); markErr != nil {
				r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
			}
			continue
		}

		rec.Triage = TriageEvaluate

		// Source policy gate: skip evaluator for unclassified or paused sources.
		gate := CheckSourcePolicy(classifierResult.ClassifierOutput)
		if !gate.Allowed {
			results[i].Status = HistoryStatusProcessed
			r.emit(s.SessionID, BatchEvent{Type: "classifier_done", Triage: TriageEvaluate})

			rec.GateReason = gate.Reason
			rec.EvaluatorModel = ""
			rec.EvaluatorPromptVersion = ""
			if markErr := finalizeSessionOutcome(rec, HistoryStatusProcessed, nil, 0, runStarts[s.SessionID]); markErr != nil {
				r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
			}
			continue
		}

		// Session needs evaluation.
		r.emit(s.SessionID, BatchEvent{Type: "classifier_done", Triage: TriageEvaluate})
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
				results[idx].Status = HistoryStatusError
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
			r.runGroupEvalSingle(chunk[0], results, indexByID, records, runStarts)
		} else {
			r.runGroupEvalBatch(chunk, results, indexByID, records, runStarts)
		}
	}

	return results
}

func (r *Runner) runGroupEvalSingle(s BatchSession, results []BatchResult, indexByID map[string]int, records map[string]*HistoryRecord, runStarts map[string]time.Time) {
	idx := indexByID[s.SessionID]
	rec := records[s.SessionID]

	evalStart := time.Now()
	evaluatorOutput, evaluatorCR, err := r.evalOne(s.SessionID, s.Digest, s.ClassifierOutput)
	evalDuration := time.Since(evalStart)

	rec.EvaluatorDurationNs = int64(evalDuration)
	rec.EvaluatorUsage = usageFromResult(evaluatorCR)

	if err != nil {
		results[idx].Status = HistoryStatusError
		results[idx].Error = err
		r.emit(s.SessionID, BatchEvent{Type: "error", Error: err})

		if markErr := finalizeSessionOutcome(rec, HistoryStatusError, err, 0, runStarts[s.SessionID]); markErr != nil {
			r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking error: %w", markErr)})
		}
		return
	}

	if evaluatorOutput.PromptVersion != "" {
		rec.EvaluatorPromptVersion = evaluatorOutput.PromptVersion
	}

	proposals := persistEvaluatorResults(s.SessionID, evaluatorOutput, r.log())
	results[idx].Status = HistoryStatusProcessed
	results[idx].Proposals = proposals
	r.emit(s.SessionID, BatchEvent{Type: "evaluator_done"})

	if markErr := finalizeSessionOutcome(rec, HistoryStatusProcessed, nil, len(evaluatorOutput.Proposals), runStarts[s.SessionID]); markErr != nil {
		r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
	}
}

func (r *Runner) runGroupEvalBatch(chunk []BatchSession, results []BatchResult, indexByID map[string]int, records map[string]*HistoryRecord, runStarts map[string]time.Time) {
	evalStart := time.Now()
	evaluatorOutput, evaluatorCR, err := r.evalMany(chunk)
	evalDuration := time.Since(evalStart)

	// Split batch evaluator duration equally among sessions in the chunk.
	perSessionDuration := evalDuration / time.Duration(len(chunk))

	// Split usage equally among sessions in the chunk (same approximation as duration).
	splitUsage := splitUsageForBatch(evaluatorCR, len(chunk))

	if err != nil {
		// Fallback: try per-session evaluation for each session in the chunk.
		// Only mark individual sessions as error if per-session eval also fails.
		r.log().Info("  Batch evaluator failed, falling back to per-session evaluation: %v", err)
		for _, s := range chunk {
			r.runGroupEvalSingle(s, results, indexByID, records, runStarts)
		}
		return
	}

	// Partition proposals by session.
	// Prefer explicit SessionID field (typed contract); fall back to prefix
	// matching on proposal IDs for legacy evaluator output without SessionID.
	type sessionResult struct {
		filtered *EvaluatorOutput
		matched  int
	}
	partitioned := make([]sessionResult, len(chunk))
	totalMatched := 0

	allHaveSessionID := len(evaluatorOutput.Proposals) > 0
	for _, p := range evaluatorOutput.Proposals {
		if p.SessionID == "" {
			allHaveSessionID = false
			break
		}
	}

	for i, s := range chunk {
		var filtered *EvaluatorOutput
		if allHaveSessionID {
			filtered = filterProposalsBySessionID(evaluatorOutput, s.SessionID)
		} else {
			prefix := "prop-" + store.ShortSessionID(s.SessionID) + "-"
			filtered = filterProposals(evaluatorOutput, prefix)
		}
		filtered.SessionID = s.SessionID
		partitioned[i] = sessionResult{filtered: filtered, matched: len(filtered.Proposals)}
		totalMatched += len(filtered.Proposals)
	}

	// Validate: if proposals are unmatched, mark all sessions as error.
	if totalMatched != len(evaluatorOutput.Proposals) {
		partitionErr := fmt.Errorf("batch: %d of %d proposals unmatched after partitioning (possible ID format mismatch)",
			len(evaluatorOutput.Proposals)-totalMatched, len(evaluatorOutput.Proposals))
		for i, s := range chunk {
			idx := indexByID[s.SessionID]
			results[idx].Status = HistoryStatusError
			results[idx].Error = partitionErr
			r.emit(s.SessionID, BatchEvent{Type: "error", Error: partitionErr})

			rec := records[s.SessionID]
			rec.EvaluatorDurationNs = int64(perSessionDuration)
			rec.EvaluatorUsage = splitUsage[i]
			if markErr := finalizeSessionOutcome(rec, HistoryStatusError, partitionErr, 0, runStarts[s.SessionID]); markErr != nil {
				r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking error: %w", markErr)})
			}
		}
		return
	}

	// Partition valid — persist results and mark processed.
	for i, s := range chunk {
		idx := indexByID[s.SessionID]
		rec := records[s.SessionID]

		if evaluatorOutput.PromptVersion != "" {
			rec.EvaluatorPromptVersion = evaluatorOutput.PromptVersion
		}

		proposals := persistEvaluatorResults(s.SessionID, partitioned[i].filtered, r.log())
		results[idx].Status = HistoryStatusProcessed
		results[idx].Proposals = proposals
		r.emit(s.SessionID, BatchEvent{Type: "evaluator_done"})

		rec.EvaluatorDurationNs = int64(perSessionDuration)
		rec.EvaluatorUsage = splitUsage[i]
		if markErr := finalizeSessionOutcome(rec, HistoryStatusProcessed, nil, len(partitioned[i].filtered.Proposals), runStarts[s.SessionID]); markErr != nil {
			r.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
		}
	}
}

// finalizeSessionOutcome performs the common bookkeeping sequence for
// completing a session's pipeline run: set status, compute totals,
// append history, and mark the session in the store.
//
// For error outcomes: pass status=HistoryStatusError, err=the error, proposalCount=0.
// For success outcomes: pass status=HistoryStatusProcessed, err=nil, proposalCount=N.
func finalizeSessionOutcome(rec *HistoryRecord, status string, err error, proposalCount int, runStart time.Time) error {
	rec.Status = status
	if err != nil {
		rec.ErrorDetail = err.Error()
	}
	rec.ProposalCount = proposalCount
	rec.computeUsageTotals()
	rec.TotalDurationNs = int64(time.Since(runStart))
	_ = AppendHistory(*rec)

	switch status {
	case HistoryStatusError:
		return store.MarkError(rec.SessionID)
	case HistoryStatusProcessed:
		return store.MarkProcessed(rec.SessionID)
	}
	return nil
}

// splitUsageForBatch divides a single ClaudeResult's usage equally among n sessions.
// Returns a slice of n *InvocationUsage entries, all sharing the same CCSessionID.
// Returns a slice of nils if cr is nil.
func splitUsageForBatch(cr *ClaudeResult, n int) []*InvocationUsage {
	result := make([]*InvocationUsage, n)
	if cr == nil || n == 0 {
		return result
	}

	for i := range result {
		result[i] = &InvocationUsage{
			CCSessionID:         cr.SessionID,
			NumTurns:            cr.NumTurns, // shared — not divisible
			InputTokens:         cr.InputTokens / n,
			OutputTokens:        cr.OutputTokens / n,
			CacheCreationTokens: cr.CacheCreationTokens / n,
			CacheReadTokens:     cr.CacheReadTokens / n,
			CostUSD:             cr.TotalCostUSD / float64(n),
			WebSearchRequests:   cr.WebSearchRequests / n,
			WebFetchRequests:    cr.WebFetchRequests / n,
		}
	}

	return result
}
