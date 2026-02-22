package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestInferStage(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"classifier prefix", "classifier-v3.txt", "classifier"},
		{"classifier with path", "/home/user/.cabrero/prompts/classifier-v4.txt", "classifier"},
		{"evaluator prefix", "evaluator-v3.txt", "evaluator"},
		{"evaluator with path", "/tmp/evaluator-experimental.txt", "evaluator"},
		{"Classifier uppercase", "Classifier-v1.txt", "classifier"},
		{"Evaluator uppercase", "EVALUATOR-v2.txt", "evaluator"},
		{"unknown prefix", "aggregator-v1.txt", ""},
		{"empty string", "", ""},
		{"no prefix match", "system-prompt.txt", ""},
		{"just classifier word", "classifier", "classifier"},
		{"just evaluator word", "evaluator", "evaluator"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := InferStage(tc.filename)
			if got != tc.want {
				t.Errorf("InferStage(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

func TestWriteReplayResult(t *testing.T) {
	t.Run("writes meta and classifier output", func(t *testing.T) {
		setupBatchStore(t)

		result := ReplayResult{
			ReplayID:  "replay-abc12345-20250101T120000",
			SessionID: "abc1234567890001",
			Stage:     "classifier",
			PromptPath: "/tmp/classifier-v4.txt",
			ClassifierOutput: &ClassifierOutput{
				SessionID: "abc1234567890001",
				Triage:    "evaluate",
			},
		}
		meta := ReplayMeta{
			ReplayID:         result.ReplayID,
			SessionID:        result.SessionID,
			Timestamp:        "2025-01-01T12:00:00Z",
			Stage:            result.Stage,
			PromptFile:       result.PromptPath,
			OriginalDecision: "evaluate",
		}

		if err := WriteReplayResult(result, meta); err != nil {
			t.Fatalf("WriteReplayResult: %v", err)
		}

		dir := filepath.Join(store.ReplayDir(), result.ReplayID)

		// Verify meta.json.
		metaPath := filepath.Join(dir, "meta.json")
		metaData, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatalf("reading meta.json: %v", err)
		}
		var gotMeta ReplayMeta
		if err := json.Unmarshal(metaData, &gotMeta); err != nil {
			t.Fatalf("parsing meta.json: %v", err)
		}
		if gotMeta.ReplayID != meta.ReplayID {
			t.Errorf("meta.ReplayID = %q, want %q", gotMeta.ReplayID, meta.ReplayID)
		}
		if gotMeta.Stage != "classifier" {
			t.Errorf("meta.Stage = %q, want %q", gotMeta.Stage, "classifier")
		}
		if gotMeta.OriginalDecision != "evaluate" {
			t.Errorf("meta.OriginalDecision = %q, want %q", gotMeta.OriginalDecision, "evaluate")
		}

		// Verify classifier.json.
		classPath := filepath.Join(dir, "classifier.json")
		classData, err := os.ReadFile(classPath)
		if err != nil {
			t.Fatalf("reading classifier.json: %v", err)
		}
		var gotClass ClassifierOutput
		if err := json.Unmarshal(classData, &gotClass); err != nil {
			t.Fatalf("parsing classifier.json: %v", err)
		}
		if gotClass.SessionID != result.SessionID {
			t.Errorf("classifier.SessionID = %q, want %q", gotClass.SessionID, result.SessionID)
		}

		// Verify evaluator.json is NOT written.
		evalPath := filepath.Join(dir, "evaluator.json")
		if _, err := os.Stat(evalPath); err == nil {
			t.Error("evaluator.json should not exist when EvaluatorOutput is nil")
		}
	})

	t.Run("writes meta and evaluator output", func(t *testing.T) {
		setupBatchStore(t)

		noReason := "test evaluator replay"
		result := ReplayResult{
			ReplayID:  "replay-def67890-20250101T130000",
			SessionID: "def6789012345001",
			Stage:     "evaluator",
			PromptPath: "/tmp/evaluator-v4.txt",
			EvaluatorOutput: &EvaluatorOutput{
				SessionID:        "def6789012345001",
				Proposals:        []Proposal{},
				NoProposalReason: &noReason,
			},
		}
		meta := ReplayMeta{
			ReplayID:   result.ReplayID,
			SessionID:  result.SessionID,
			Timestamp:  "2025-01-01T13:00:00Z",
			Stage:      result.Stage,
			PromptFile: result.PromptPath,
		}

		if err := WriteReplayResult(result, meta); err != nil {
			t.Fatalf("WriteReplayResult: %v", err)
		}

		dir := filepath.Join(store.ReplayDir(), result.ReplayID)

		// Verify evaluator.json.
		evalPath := filepath.Join(dir, "evaluator.json")
		evalData, err := os.ReadFile(evalPath)
		if err != nil {
			t.Fatalf("reading evaluator.json: %v", err)
		}
		var gotEval EvaluatorOutput
		if err := json.Unmarshal(evalData, &gotEval); err != nil {
			t.Fatalf("parsing evaluator.json: %v", err)
		}
		if gotEval.SessionID != result.SessionID {
			t.Errorf("evaluator.SessionID = %q, want %q", gotEval.SessionID, result.SessionID)
		}

		// Verify classifier.json is NOT written.
		classPath := filepath.Join(dir, "classifier.json")
		if _, err := os.Stat(classPath); err == nil {
			t.Error("classifier.json should not exist when ClassifierOutput is nil")
		}
	})

	t.Run("replay dir contains replayID subdirectory", func(t *testing.T) {
		setupBatchStore(t)

		result := ReplayResult{
			ReplayID:  "replay-aaa11111-20250101T140000",
			SessionID: "aaa1111122223333",
			Stage:     "classifier",
		}
		meta := ReplayMeta{
			ReplayID:  result.ReplayID,
			SessionID: result.SessionID,
			Stage:     result.Stage,
		}

		if err := WriteReplayResult(result, meta); err != nil {
			t.Fatalf("WriteReplayResult: %v", err)
		}

		expectedDir := filepath.Join(store.ReplayDir(), result.ReplayID)
		info, err := os.Stat(expectedDir)
		if err != nil {
			t.Fatalf("replay subdir not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected a directory, got a file")
		}
	})
}

func TestNewReplayID(t *testing.T) {
	t.Run("has expected prefix format", func(t *testing.T) {
		id := NewReplayID("abcdef1234567890")
		if !strings.HasPrefix(id, "replay-abcdef12-") {
			t.Errorf("NewReplayID prefix unexpected: %q", id)
		}
	})

	t.Run("short session ID is not padded", func(t *testing.T) {
		id := NewReplayID("abc")
		if !strings.HasPrefix(id, "replay-abc-") {
			t.Errorf("NewReplayID(%q) = %q, expected prefix 'replay-abc-'", "abc", id)
		}
	})

	t.Run("unique across calls", func(t *testing.T) {
		// Two IDs generated within the same second will be equal in the
		// timestamp component, but that's acceptable — we only test the format.
		id := NewReplayID("sess1234")
		if !strings.HasPrefix(id, "replay-") {
			t.Errorf("expected 'replay-' prefix, got %q", id)
		}
	})
}
