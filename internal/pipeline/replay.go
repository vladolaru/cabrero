package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/store"
)

// ReplayResult holds the outputs of a single replay run.
// ClassifierOutput and EvaluatorOutput are nil when the stage was not executed.
type ReplayResult struct {
	ReplayID        string
	SessionID       string
	Stage           string // "classifier" or "evaluator"
	PromptPath      string
	ClassifierOutput *ClassifierOutput
	EvaluatorOutput  *EvaluatorOutput
}

// ReplayMeta is the JSON metadata written alongside replay outputs.
type ReplayMeta struct {
	ReplayID         string `json:"replayId"`
	SessionID        string `json:"sessionId"`
	Timestamp        string `json:"timestamp"`
	Stage            string `json:"stage"`
	PromptFile       string `json:"promptFile"`
	OriginalDecision string `json:"originalDecision,omitempty"`
}

// InferStage infers the pipeline stage from a prompt filename.
// It returns "classifier" when the base name starts with "classifier",
// "evaluator" when it starts with "evaluator", and "" otherwise.
func InferStage(filename string) string {
	base := filepath.Base(filename)
	base = strings.ToLower(base)
	switch {
	case strings.HasPrefix(base, "classifier"):
		return "classifier"
	case strings.HasPrefix(base, "evaluator"):
		return "evaluator"
	default:
		return ""
	}
}

// WriteReplayResult persists a replay result under store.ReplayDir()/<replayID>/.
// It writes:
//   - meta.json      — ReplayMeta
//   - classifier.json — ClassifierOutput (when non-nil)
//   - evaluator.json  — EvaluatorOutput  (when non-nil)
func WriteReplayResult(result ReplayResult, meta ReplayMeta) error {
	dir := filepath.Join(store.ReplayDir(), result.ReplayID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating replay dir: %w", err)
	}

	// Write meta.
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling replay meta: %w", err)
	}
	if err := store.AtomicWrite(filepath.Join(dir, "meta.json"), metaData, 0o644); err != nil {
		return fmt.Errorf("writing replay meta: %w", err)
	}

	// Write classifier output when present.
	if result.ClassifierOutput != nil {
		data, err := json.MarshalIndent(result.ClassifierOutput, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling classifier output: %w", err)
		}
		if err := store.AtomicWrite(filepath.Join(dir, "classifier.json"), data, 0o644); err != nil {
			return fmt.Errorf("writing classifier output: %w", err)
		}
	}

	// Write evaluator output when present.
	if result.EvaluatorOutput != nil {
		data, err := json.MarshalIndent(result.EvaluatorOutput, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling evaluator output: %w", err)
		}
		if err := store.AtomicWrite(filepath.Join(dir, "evaluator.json"), data, 0o644); err != nil {
			return fmt.Errorf("writing evaluator output: %w", err)
		}
	}

	return nil
}

// ParseSessionForReplay parses a session transcript and returns the Digest.
// It is a thin wrapper around parser.ParseSession provided so callers in the
// replay command do not need to import the parser package directly.
func ParseSessionForReplay(sessionID string) (*parser.Digest, error) {
	return parser.ParseSession(sessionID)
}

// NewReplayID generates a replay ID of the form "replay-<sessionShort>-<ts>".
// sessionShort is the first 8 characters of the session ID.
// ts is the current UTC time formatted as "20060102T150405".
func NewReplayID(sessionID string) string {
	short := sessionID
	if len(short) > 8 {
		short = short[:8]
	}
	ts := time.Now().UTC().Format("20060102T150405")
	return fmt.Sprintf("replay-%s-%s", short, ts)
}
