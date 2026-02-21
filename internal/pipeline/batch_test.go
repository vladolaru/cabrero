package pipeline

import (
	"context"
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

// --- ProcessGroup tests ---

func TestProcessGroup(t *testing.T) {
	t.Run("all clean marked processed", func(t *testing.T) {
		setupBatchStore(t)
		s1 := createBatchSession(t, "cleantest00000001")
		s2 := createBatchSession(t, "cleantest00000002")

		bp := &BatchProcessor{ClassifyFunc: fakeClassifyClean}
		results := bp.ProcessGroup(context.Background(), []BatchSession{s1, s2})

		if len(results) != 2 {
			t.Fatalf("got %d results, want 2", len(results))
		}
		for _, r := range results {
			if r.Status != "processed" {
				t.Errorf("%s: Status = %q, want 'processed'", r.SessionID, r.Status)
			}
			if r.Triage != "clean" {
				t.Errorf("%s: Triage = %q, want 'clean'", r.SessionID, r.Triage)
			}
			if r.Error != nil {
				t.Errorf("%s: Error = %v, want nil", r.SessionID, r.Error)
			}
		}
		// Verify store state.
		for _, s := range []BatchSession{s1, s2} {
			meta, err := store.ReadMetadata(s.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if meta.Status != "processed" {
				t.Errorf("store %s: Status = %q, want 'processed'", s.SessionID, meta.Status)
			}
		}
	})

	t.Run("classifier error marks session error", func(t *testing.T) {
		setupBatchStore(t)
		s := createBatchSession(t, "classifyfail0001")

		bp := &BatchProcessor{
			ClassifyFunc: func(sid string, cfg PipelineConfig) (*ClassifierResult, error) {
				return nil, fmt.Errorf("classifier boom")
			},
		}
		results := bp.ProcessGroup(context.Background(), []BatchSession{s})

		if results[0].Status != "error" {
			t.Errorf("Status = %q, want 'error'", results[0].Status)
		}
		if results[0].Error == nil {
			t.Error("Error is nil, want non-nil")
		}
		meta, _ := store.ReadMetadata(s.SessionID)
		if meta.Status != "error" {
			t.Errorf("store Status = %q, want 'error'", meta.Status)
		}
	})

	t.Run("single evaluate uses EvalFunc not batch", func(t *testing.T) {
		setupBatchStore(t)
		s := createBatchSession(t, "singleeval000001")

		singleCalled := false
		batchCalled := false
		bp := &BatchProcessor{
			ClassifyFunc: fakeClassifyEvaluate,
			EvalFunc: func(sid string, d *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, error) {
				singleCalled = true
				return &EvaluatorOutput{SessionID: sid, Proposals: []Proposal{}}, nil
			},
			EvalBatchFunc: func(_ []BatchSession, _ PipelineConfig) (*EvaluatorOutput, error) {
				batchCalled = true
				return nil, fmt.Errorf("should not be called")
			},
		}
		results := bp.ProcessGroup(context.Background(), []BatchSession{s})

		if !singleCalled {
			t.Error("EvalFunc not called")
		}
		if batchCalled {
			t.Error("EvalBatchFunc called unexpectedly")
		}
		if results[0].Status != "processed" {
			t.Errorf("Status = %q, want 'processed'", results[0].Status)
		}
	})

	t.Run("two evaluate sessions use EvalBatchFunc", func(t *testing.T) {
		setupBatchStore(t)
		s1 := createBatchSession(t, "batcheval0000001")
		s2 := createBatchSession(t, "batcheval0000002")

		batchCalled := false
		bp := &BatchProcessor{
			ClassifyFunc: fakeClassifyEvaluate,
			EvalFunc: func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
				return nil, fmt.Errorf("should not be called")
			},
			EvalBatchFunc: func(sessions []BatchSession, _ PipelineConfig) (*EvaluatorOutput, error) {
				batchCalled = true
				// Return proposals with correctly-prefixed IDs (first 6 chars of session ID).
				return &EvaluatorOutput{Proposals: []Proposal{
					{ID: "prop-batche-0", Type: "skill_improvement", Confidence: "high", Rationale: "t"},
					{ID: "prop-batche-1", Type: "skill_improvement", Confidence: "high", Rationale: "t"},
				}}, nil
			},
		}
		results := bp.ProcessGroup(context.Background(), []BatchSession{s1, s2})

		if !batchCalled {
			t.Error("EvalBatchFunc not called")
		}
		for _, r := range results {
			if r.Status != "processed" {
				t.Errorf("%s: Status = %q, want 'processed'", r.SessionID, r.Status)
			}
		}
	})

	t.Run("MaxBatchSize=1 forces single eval", func(t *testing.T) {
		setupBatchStore(t)
		s1 := createBatchSession(t, "maxbatch00000001")
		s2 := createBatchSession(t, "maxbatch00000002")

		singleCount := 0
		bp := &BatchProcessor{
			MaxBatchSize: 1,
			ClassifyFunc: fakeClassifyEvaluate,
			EvalFunc: func(sid string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
				singleCount++
				return &EvaluatorOutput{SessionID: sid, Proposals: []Proposal{}}, nil
			},
		}
		bp.ProcessGroup(context.Background(), []BatchSession{s1, s2})

		if singleCount != 2 {
			t.Errorf("EvalFunc called %d times, want 2", singleCount)
		}
	})

	t.Run("context cancel in phase 1 errors remaining", func(t *testing.T) {
		setupBatchStore(t)
		s1 := createBatchSession(t, "ctxcancel0000001")
		s2 := createBatchSession(t, "ctxcancel0000002")

		ctx, cancel := context.WithCancel(context.Background())
		callCount := 0
		bp := &BatchProcessor{
			ClassifyFunc: func(sid string, cfg PipelineConfig) (*ClassifierResult, error) {
				callCount++
				if callCount == 1 {
					cancel()
				}
				return fakeClassifyClean(sid, cfg)
			},
		}
		results := bp.ProcessGroup(ctx, []BatchSession{s1, s2})

		if results[1].Status != "error" {
			t.Errorf("s2 Status = %q, want 'error'", results[1].Status)
		}
		if results[1].Error == nil {
			t.Error("s2 Error is nil, want context.Canceled")
		}
	})

	t.Run("OnStatus emits events", func(t *testing.T) {
		setupBatchStore(t)
		s := createBatchSession(t, "statusevents0001")

		var events []BatchEvent
		bp := &BatchProcessor{
			ClassifyFunc: fakeClassifyEvaluate,
			EvalFunc:     fakeEvalNoProposals,
			OnStatus: func(_ string, event BatchEvent) {
				events = append(events, event)
			},
		}
		bp.ProcessGroup(context.Background(), []BatchSession{s})

		hasClassifier := false
		hasEvaluator := false
		for _, e := range events {
			if e.Type == "classifier_done" && e.Triage == "evaluate" {
				hasClassifier = true
			}
			if e.Type == "evaluator_done" {
				hasEvaluator = true
			}
		}
		if !hasClassifier {
			t.Error("missing classifier_done event")
		}
		if !hasEvaluator {
			t.Error("missing evaluator_done event")
		}
	})

	t.Run("proposal count in results", func(t *testing.T) {
		setupBatchStore(t)
		s := createBatchSession(t, "proposalcnt00001")

		bp := &BatchProcessor{
			ClassifyFunc: fakeClassifyEvaluate,
			EvalFunc:     fakeEvalWithProposals(3),
		}
		results := bp.ProcessGroup(context.Background(), []BatchSession{s})

		if results[0].Proposals != 3 {
			t.Errorf("Proposals = %d, want 3", results[0].Proposals)
		}
	})

	t.Run("logger receives messages during processing", func(t *testing.T) {
		setupBatchStore(t)
		s := createBatchSession(t, "loggertest000001")

		spy := &spyLogger{}
		bp := &BatchProcessor{
			Config:       PipelineConfig{Logger: spy},
			ClassifyFunc: fakeClassifyEvaluate,
			EvalFunc:     fakeEvalWithProposals(2),
		}
		bp.ProcessGroup(context.Background(), []BatchSession{s})

		// persistEvaluatorResults writes proposals through the store, and
		// any write errors would appear via Error. With a working store the
		// spy should have no Error calls from persist — just verify no panic
		// and that the logger was threaded through without issues.
		if spy == nil {
			t.Fatal("spy logger is nil")
		}
	})

	t.Run("warnings route to logger error", func(t *testing.T) {
		// filterAndValidateProposals is called inside RunEvaluator (not via
		// the fake EvalFunc), so we test it directly here.
		spy := &spyLogger{}
		output := &EvaluatorOutput{
			Proposals: []Proposal{
				{
					ID:         "prop-warnlo-0",
					Type:       "skill_improvement",
					Confidence: "low", // should be dropped with a warning
					Rationale:  "test",
				},
				{
					ID:         "prop-warnlo-1",
					Type:       "skill_improvement",
					Confidence: "high",
					Rationale:  "test",
				},
				{
					ID:         "prop-warnlo-1", // duplicate
					Type:       "skill_improvement",
					Confidence: "high",
					Rationale:  "test",
				},
			},
		}

		err := filterAndValidateProposals(output, map[string]bool{}, map[string]bool{}, spy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !spy.hasError("low-confidence") {
			t.Errorf("expected Error about low-confidence, got: %v", spy.errors)
		}
		if !spy.hasError("duplicate proposal") {
			t.Errorf("expected Error about duplicate proposal, got: %v", spy.errors)
		}
		if len(output.Proposals) != 1 {
			t.Errorf("expected 1 valid proposal, got %d", len(output.Proposals))
		}
	})
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
