package pipeline

import (
	"strings"
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
