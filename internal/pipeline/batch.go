package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/store"
)

// DefaultMaxBatchSize is the maximum number of sessions per Evaluator invocation.
// Keeps each batch within the Evaluator's 60-turn / 15-minute caps.
const DefaultMaxBatchSize = 10

// BatchEvent carries progress information for a single session during batch processing.
type BatchEvent struct {
	Type   string // "classifier_done", "evaluator_done", "error"
	Triage string // "clean" or "evaluate" (set for classifier_done)
	Error  error  // set for "error" events
}

// BatchResult summarises the outcome of processing one session.
type BatchResult struct {
	SessionID string
	Status    string // "processed" or "error"
	Proposals int
	Triage    string // "clean" or "evaluate"
	Error     error
}

// BatchProcessor runs Classifier individually then Evaluator in batches.
type BatchProcessor struct {
	Config       PipelineConfig
	MaxBatchSize int // 0 means DefaultMaxBatchSize
	OnStatus     func(sessionID string, event BatchEvent)

	// Testing hooks — when nil, package-level functions are used.
	ClassifyFunc  func(sessionID string, cfg PipelineConfig) (*ClassifierResult, error)
	EvalFunc      func(sessionID string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, error)
	EvalBatchFunc func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, error)
}

func (bp *BatchProcessor) log() Logger {
	return bp.Config.logger()
}

func (bp *BatchProcessor) maxBatch() int {
	if bp.MaxBatchSize > 0 {
		return bp.MaxBatchSize
	}
	return DefaultMaxBatchSize
}

func (bp *BatchProcessor) emit(sessionID string, event BatchEvent) {
	if bp.OnStatus != nil {
		bp.OnStatus(sessionID, event)
	}
}

func (bp *BatchProcessor) classify(sessionID string) (*ClassifierResult, error) {
	if bp.ClassifyFunc != nil {
		return bp.ClassifyFunc(sessionID, bp.Config)
	}
	return RunThroughClassifier(sessionID, bp.Config)
}

func (bp *BatchProcessor) evalOne(sessionID string, digest *parser.Digest, co *ClassifierOutput) (*EvaluatorOutput, error) {
	if bp.EvalFunc != nil {
		return bp.EvalFunc(sessionID, digest, co, bp.Config)
	}
	return RunEvaluator(sessionID, digest, co, bp.Config)
}

func (bp *BatchProcessor) evalMany(sessions []BatchSession) (*EvaluatorOutput, error) {
	if bp.EvalBatchFunc != nil {
		return bp.EvalBatchFunc(sessions, bp.Config)
	}
	return RunEvaluatorBatch(sessions, bp.Config)
}

// ProcessGroup runs all sessions through the Classifier, then batches "evaluate"
// sessions through the Evaluator. It returns one BatchResult per input session.
func (bp *BatchProcessor) ProcessGroup(ctx context.Context, sessions []BatchSession) []BatchResult {
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

		classifierResult, err := bp.classify(s.SessionID)
		if err != nil {
			results[i].Status = "error"
			results[i].Error = err
			bp.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
			if markErr := store.MarkError(s.SessionID); markErr != nil {
				bp.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking error: %w", markErr)})
			}
			continue
		}

		triage := classifierResult.ClassifierOutput.Triage
		results[i].Triage = triage

		if triage == "clean" {
			results[i].Status = "processed"
			bp.emit(s.SessionID, BatchEvent{Type: "classifier_done", Triage: "clean"})
			if markErr := store.MarkProcessed(s.SessionID); markErr != nil {
				bp.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
			}
			continue
		}

		// Session needs evaluation.
		bp.emit(s.SessionID, BatchEvent{Type: "classifier_done", Triage: "evaluate"})
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
	maxBatch := bp.maxBatch()
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
			bp.evalSingle(chunk[0], results, indexByID)
		} else {
			bp.evalBatch(chunk, results, indexByID)
		}
	}

	return results
}

func (bp *BatchProcessor) evalSingle(s BatchSession, results []BatchResult, indexByID map[string]int) {
	idx := indexByID[s.SessionID]

	evaluatorOutput, err := bp.evalOne(s.SessionID, s.Digest, s.ClassifierOutput)
	if err != nil {
		results[idx].Status = "error"
		results[idx].Error = err
		bp.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
		if markErr := store.MarkError(s.SessionID); markErr != nil {
			bp.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking error: %w", markErr)})
		}
		return
	}

	proposals := persistEvaluatorResults(s.SessionID, evaluatorOutput, bp.log())
	results[idx].Status = "processed"
	results[idx].Proposals = proposals
	bp.emit(s.SessionID, BatchEvent{Type: "evaluator_done"})

	if markErr := store.MarkProcessed(s.SessionID); markErr != nil {
		bp.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
	}
}

func (bp *BatchProcessor) evalBatch(chunk []BatchSession, results []BatchResult, indexByID map[string]int) {
	evaluatorOutput, err := bp.evalMany(chunk)
	if err != nil {
		// Mark all sessions in the chunk as errors.
		for _, s := range chunk {
			idx := indexByID[s.SessionID]
			results[idx].Status = "error"
			results[idx].Error = err
			bp.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
			if markErr := store.MarkError(s.SessionID); markErr != nil {
				bp.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking error: %w", markErr)})
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

		proposals := persistEvaluatorResults(s.SessionID, filtered, bp.log())
		results[idx].Status = "processed"
		results[idx].Proposals = proposals
		bp.emit(s.SessionID, BatchEvent{Type: "evaluator_done"})

		if markErr := store.MarkProcessed(s.SessionID); markErr != nil {
			bp.emit(s.SessionID, BatchEvent{Type: "error", Error: fmt.Errorf("marking processed: %w", markErr)})
		}
	}

	if totalMatched == 0 && len(evaluatorOutput.Proposals) > 0 {
		err := fmt.Errorf("batch: all %d proposals dropped during partitioning (possible ID format mismatch)",
			len(evaluatorOutput.Proposals))
		for _, s := range chunk {
			bp.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
		}
	} else if totalMatched != len(evaluatorOutput.Proposals) {
		err := fmt.Errorf("batch: %d of %d proposals unmatched after partitioning",
			len(evaluatorOutput.Proposals)-totalMatched, len(evaluatorOutput.Proposals))
		bp.emit(chunk[0].SessionID, BatchEvent{Type: "error", Error: err})
	}
}

// persistEvaluatorResults writes the evaluator output and proposals to the store.
// Returns the number of proposals successfully written.
func persistEvaluatorResults(sessionID string, output *EvaluatorOutput, log Logger) int {
	if err := WriteEvaluatorOutput(sessionID, output); err != nil {
		log.Error("writing evaluator output for %s: %v", sessionID, err)
		return 0
	}

	count := 0
	for i := range output.Proposals {
		p := &output.Proposals[i]
		if err := WriteProposal(p, sessionID); err != nil {
			log.Error("writing proposal %s: %v", p.ID, err)
			continue
		}
		count++
	}
	return count
}

// filterProposals returns a shallow copy of the EvaluatorOutput with only
// the proposals whose ID starts with the given prefix.
func filterProposals(output *EvaluatorOutput, prefix string) *EvaluatorOutput {
	filtered := *output // shallow copy
	filtered.Proposals = []Proposal{}
	for _, p := range output.Proposals {
		if strings.HasPrefix(p.ID, prefix) {
			filtered.Proposals = append(filtered.Proposals, p)
		}
	}
	return &filtered
}

// shortID truncates a session ID to 6 characters.
// Must match the evaluator prompt format: "prop-{first 6 chars of sessionId}-{index}".
func shortID(id string) string {
	if len(id) > 6 {
		return id[:6]
	}
	return id
}
