package pipeline

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/store"
)

func TestShortID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"longer than 6 truncates", "abcdef1234567890", "abcdef"},
		{"exactly 6 unchanged", "abcdef", "abcdef"},
		{"shorter than 6 unchanged", "abc", "abc"},
		{"empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shortID(tc.input)
			if got != tc.want {
				t.Errorf("shortID(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFilterProposals(t *testing.T) {
	t.Run("returns shallow copy not same pointer", func(t *testing.T) {
		out := &EvaluatorOutput{SessionID: "sess-abc", Proposals: []Proposal{}}
		got := filterProposals(out, "prop-")
		if got == out {
			t.Error("filterProposals returned same pointer, want a copy")
		}
		if got.SessionID != out.SessionID {
			t.Errorf("SessionID = %q, want %q", got.SessionID, out.SessionID)
		}
	})

	t.Run("nil proposals returns empty slice", func(t *testing.T) {
		out := &EvaluatorOutput{Proposals: nil}
		got := filterProposals(out, "prop-abcd1234-")
		if got.Proposals == nil {
			t.Error("Proposals is nil, want empty slice")
		}
		if len(got.Proposals) != 0 {
			t.Errorf("len(Proposals) = %d, want 0", len(got.Proposals))
		}
	})

	t.Run("keeps only matching prefix", func(t *testing.T) {
		out := &EvaluatorOutput{
			Proposals: []Proposal{
				{ID: "prop-abcd1234-0"},
				{ID: "prop-abcd1234-1"},
				{ID: "prop-efgh5678-0"},
			},
		}
		got := filterProposals(out, "prop-abcd1234-")
		if len(got.Proposals) != 2 {
			t.Fatalf("got %d proposals, want 2", len(got.Proposals))
		}
		for _, p := range got.Proposals {
			if p.ID != "prop-abcd1234-0" && p.ID != "prop-abcd1234-1" {
				t.Errorf("unexpected proposal ID %q", p.ID)
			}
		}
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		out := &EvaluatorOutput{
			Proposals: []Proposal{{ID: "prop-efgh5678-0"}},
		}
		got := filterProposals(out, "prop-abcd1234-")
		if len(got.Proposals) != 0 {
			t.Errorf("got %d proposals, want 0", len(got.Proposals))
		}
	})

	t.Run("does not modify original", func(t *testing.T) {
		original := &EvaluatorOutput{
			Proposals: []Proposal{
				{ID: "prop-abcd1234-0"},
				{ID: "prop-efgh5678-0"},
			},
		}
		filterProposals(original, "prop-abcd1234-")
		if len(original.Proposals) != 2 {
			t.Errorf("original modified: got %d proposals, want 2", len(original.Proposals))
		}
	})

	t.Run("empty prefix matches all", func(t *testing.T) {
		out := &EvaluatorOutput{
			Proposals: []Proposal{
				{ID: "prop-abcd1234-0"},
				{ID: "prop-efgh5678-0"},
			},
		}
		got := filterProposals(out, "")
		if len(got.Proposals) != 2 {
			t.Errorf("got %d proposals, want 2", len(got.Proposals))
		}
	})
}

// --- Store and session helpers ---

func setupBatchStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
}

func createBatchSession(t *testing.T, sessionID string) BatchSession {
	t.Helper()
	rawDir := store.RawDir(sessionID)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := store.Metadata{
		SessionID:      sessionID,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CaptureTrigger: "imported",
		Status:         "queued",
	}
	if err := store.WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}
	return BatchSession{SessionID: sessionID}
}

// --- Fake classify/eval functions ---

func fakeClassifyClean(sessionID string, cfg PipelineConfig) (*ClassifierResult, error) {
	return &ClassifierResult{
		Digest:           &parser.Digest{SessionID: sessionID},
		ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: "clean"},
	}, nil
}

func fakeClassifyEvaluate(sessionID string, cfg PipelineConfig) (*ClassifierResult, error) {
	return &ClassifierResult{
		Digest:           &parser.Digest{SessionID: sessionID},
		ClassifierOutput: &ClassifierOutput{SessionID: sessionID, Triage: "evaluate"},
	}, nil
}

func fakeEvalNoProposals(sessionID string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
	return &EvaluatorOutput{SessionID: sessionID, Proposals: []Proposal{}}, nil
}

func fakeEvalWithProposals(n int) func(string, *parser.Digest, *ClassifierOutput, PipelineConfig) (*EvaluatorOutput, error) {
	return func(sessionID string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
		proposals := make([]Proposal, n)
		for i := range proposals {
			proposals[i] = Proposal{
				ID:         fmt.Sprintf("prop-%s-%d", shortID(sessionID), i),
				Type:       "skill_improvement",
				Confidence: "high",
				Rationale:  "test",
			}
		}
		return &EvaluatorOutput{SessionID: sessionID, Proposals: proposals}, nil
	}
}

func TestFilterAndValidateProposals(t *testing.T) {
	t.Run("skill_scaffold without name logs error", func(t *testing.T) {
		spy := &spyLogger{}
		output := &EvaluatorOutput{
			Proposals: []Proposal{
				{
					ID:         "prop-scaffold-0",
					Type:       "skill_scaffold",
					Confidence: "high",
					Rationale:  "test",
					// ScaffoldSkillName intentionally nil
				},
			},
		}

		filterAndValidateProposals(output, map[string]bool{}, map[string]bool{}, spy)

		if !spy.hasError("skill_scaffold proposal without scaffoldSkillName") {
			t.Errorf("expected Error about missing scaffoldSkillName, got: %v", spy.errors)
		}
		if len(output.Proposals) != 0 {
			t.Errorf("expected 0 proposals after filtering, got %d", len(output.Proposals))
		}
	})

	t.Run("skill_scaffold with empty name logs error", func(t *testing.T) {
		spy := &spyLogger{}
		empty := ""
		output := &EvaluatorOutput{
			Proposals: []Proposal{
				{
					ID:                "prop-scaffold-1",
					Type:              "skill_scaffold",
					Confidence:        "high",
					Rationale:         "test",
					ScaffoldSkillName: &empty,
				},
			},
		}

		filterAndValidateProposals(output, map[string]bool{}, map[string]bool{}, spy)

		if !spy.hasError("skill_scaffold proposal without scaffoldSkillName") {
			t.Errorf("expected Error about missing scaffoldSkillName, got: %v", spy.errors)
		}
		if len(output.Proposals) != 0 {
			t.Errorf("expected 0 proposals, got %d", len(output.Proposals))
		}
	})

	t.Run("unknown skill signal logs warning", func(t *testing.T) {
		spy := &spyLogger{}
		output := &EvaluatorOutput{
			Proposals: []Proposal{
				{
					ID:                "prop-skill-0",
					Type:              "skill_improvement",
					Confidence:        "high",
					Rationale:         "test",
					CitedSkillSignals: []string{"nonexistent-skill"},
				},
			},
		}
		knownSkills := map[string]bool{"real-skill": true}

		filterAndValidateProposals(output, map[string]bool{}, knownSkills, spy)

		if !spy.hasError("nonexistent-skill") {
			t.Errorf("expected Error about unknown skill, got: %v", spy.errors)
		}
		// Proposal is kept (warning only, not dropped).
		if len(output.Proposals) != 1 {
			t.Errorf("expected 1 proposal (warning only), got %d", len(output.Proposals))
		}
	})

	t.Run("invalid UUID citations are pruned", func(t *testing.T) {
		spy := &spyLogger{}
		output := &EvaluatorOutput{
			Proposals: []Proposal{
				{
					ID:         "prop-uuid-0",
					Type:       "skill_improvement",
					Confidence: "high",
					Rationale:  "test",
					CitedUUIDs: []string{"valid-uuid", "invalid-uuid"},
				},
			},
		}
		validUUIDs := map[string]bool{"valid-uuid": true}

		filterAndValidateProposals(output, validUUIDs, map[string]bool{}, spy)

		if len(output.Proposals) != 1 {
			t.Fatalf("expected 1 proposal, got %d", len(output.Proposals))
		}
		cited := output.Proposals[0].CitedUUIDs
		if len(cited) != 1 || cited[0] != "valid-uuid" {
			t.Errorf("expected [valid-uuid], got %v", cited)
		}
	})
}

func TestValidateClassifierUUIDs(t *testing.T) {
	t.Run("invalid UUID logs error", func(t *testing.T) {
		setupBatchStore(t)
		sid := "classifieruuid01"
		createBatchSession(t, sid)
		// Write a transcript with one known UUID.
		writeTranscript(t, sid, []string{"uuid-known-1"})

		spy := &spyLogger{}
		output := &ClassifierOutput{
			KeyTurns: []ClassifierKeyTurn{
				{UUID: "uuid-known-1", Reason: "test", Category: "test"},
				{UUID: "uuid-missing", Reason: "test", Category: "test"},
			},
		}

		err := validateClassifierUUIDs(sid, output, spy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !spy.hasError("non-existent UUID") {
			t.Errorf("expected Error about non-existent UUID, got: %v", spy.errors)
		}
		// uuid-missing should be pruned from KeyTurns.
		if len(output.KeyTurns) != 1 {
			t.Errorf("expected 1 key turn after pruning, got %d", len(output.KeyTurns))
		}
		if output.KeyTurns[0].UUID != "uuid-known-1" {
			t.Errorf("expected uuid-known-1, got %s", output.KeyTurns[0].UUID)
		}
	})

	t.Run("all valid UUIDs no errors", func(t *testing.T) {
		setupBatchStore(t)
		sid := "classifieruuid02"
		createBatchSession(t, sid)
		writeTranscript(t, sid, []string{"uuid-a", "uuid-b"})

		spy := &spyLogger{}
		output := &ClassifierOutput{
			KeyTurns: []ClassifierKeyTurn{
				{UUID: "uuid-a", Reason: "test", Category: "test"},
				{UUID: "uuid-b", Reason: "test2", Category: "test"},
			},
		}

		err := validateClassifierUUIDs(sid, output, spy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(spy.errors) != 0 {
			t.Errorf("expected no errors, got: %v", spy.errors)
		}
		if len(output.KeyTurns) != 2 {
			t.Errorf("expected 2 key turns, got %d", len(output.KeyTurns))
		}
	})

	t.Run("majority invalid UUIDs returns error", func(t *testing.T) {
		setupBatchStore(t)
		sid := "classifieruuid03"
		createBatchSession(t, sid)
		writeTranscript(t, sid, []string{"uuid-only-valid"})

		spy := &spyLogger{}
		output := &ClassifierOutput{
			KeyTurns: []ClassifierKeyTurn{
				{UUID: "uuid-only-valid", Reason: "test", Category: "test"},
				{UUID: "uuid-bad-1", Reason: "test", Category: "test"},
				{UUID: "uuid-bad-2", Reason: "test", Category: "test"},
			},
		}

		err := validateClassifierUUIDs(sid, output, spy)
		if err == nil {
			t.Fatal("expected error for >50% invalid UUIDs, got nil")
		}
		if !strings.Contains(err.Error(), "critical") {
			t.Errorf("expected 'critical' in error, got: %v", err)
		}
	})
}

func TestValidateEvaluatorOutput(t *testing.T) {
	t.Run("invalid UUID logs error and prunes", func(t *testing.T) {
		setupBatchStore(t)
		sid := "evaluatoruuid001"
		createBatchSession(t, sid)
		writeTranscript(t, sid, []string{"uuid-valid"})

		spy := &spyLogger{}
		output := &EvaluatorOutput{
			SessionID: sid,
			Proposals: []Proposal{
				{
					ID:         "prop-eval-0",
					Type:       "skill_improvement",
					Confidence: "high",
					Rationale:  "test",
					CitedUUIDs: []string{"uuid-valid", "uuid-gone"},
				},
			},
		}
		classifierOutput := &ClassifierOutput{}

		err := validateEvaluatorOutput(sid, output, classifierOutput, spy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !spy.hasError("non-existent UUID") {
			t.Errorf("expected Error about non-existent UUID, got: %v", spy.errors)
		}
		// uuid-gone should be pruned from CitedUUIDs.
		if len(output.Proposals) != 1 {
			t.Fatalf("expected 1 proposal, got %d", len(output.Proposals))
		}
		cited := output.Proposals[0].CitedUUIDs
		if len(cited) != 1 || cited[0] != "uuid-valid" {
			t.Errorf("expected [uuid-valid], got %v", cited)
		}
	})
}

// writeTranscript writes a minimal JSONL transcript with the given UUIDs.
func writeTranscript(t *testing.T, sessionID string, uuids []string) {
	t.Helper()
	var lines string
	for _, uuid := range uuids {
		lines += fmt.Sprintf(`{"uuid":"%s","type":"assistant","message":{"content":"test"}}`, uuid) + "\n"
	}
	path := store.RawDir(sessionID) + "/transcript.jsonl"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
}
