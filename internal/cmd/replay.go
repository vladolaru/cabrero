package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Replay re-runs the classifier or evaluator stage with an alternate prompt
// and persists the result under ~/.cabrero/replays/<replayID>/.
//
// Usage:
//
//	cabrero replay --session ID --prompt PATH [--stage classifier|evaluator] [options]
func Replay(args []string) error {
	defaults := pipeline.DefaultPipelineConfig()
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)

	sessionID := fs.String("session", "", "session ID to replay (required)")
	promptPath := fs.String("prompt", "", "path to alternate prompt file (required)")
	stage := fs.String("stage", "", "pipeline stage: classifier or evaluator (inferred from prompt filename when absent)")
	compare := fs.Bool("compare", false, "print a diff of the new output against the original")
	debug := fs.Bool("debug", false, "persist CC sessions for inspection")
	classifierMaxTurns := fs.Int("classifier-max-turns", defaults.ClassifierMaxTurns, "max agentic turns for Classifier")
	evaluatorMaxTurns := fs.Int("evaluator-max-turns", defaults.EvaluatorMaxTurns, "max agentic turns for Evaluator")
	classifierTimeout := fs.Duration("classifier-timeout", defaults.ClassifierTimeout, "timeout for Classifier")
	evaluatorTimeout := fs.Duration("evaluator-timeout", defaults.EvaluatorTimeout, "timeout for Evaluator")
	classifierModel := fs.String("classifier-model", defaults.ClassifierModel, "Claude model for Classifier")
	evaluatorModel := fs.String("evaluator-model", defaults.EvaluatorModel, "Claude model for Evaluator")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Validate required flags.
	if *sessionID == "" {
		return fmt.Errorf("--session is required\nusage: cabrero replay --session ID --prompt PATH [--stage classifier|evaluator]")
	}
	if *promptPath == "" {
		return fmt.Errorf("--prompt is required\nusage: cabrero replay --session ID --prompt PATH [--stage classifier|evaluator]")
	}

	// Validate session exists.
	if !store.SessionExists(*sessionID) {
		return fmt.Errorf("session %q not found in store", *sessionID)
	}

	// Infer stage from filename if not explicitly set.
	resolvedStage := *stage
	if resolvedStage == "" {
		resolvedStage = pipeline.InferStage(*promptPath)
		if resolvedStage == "" {
			return fmt.Errorf("cannot infer stage from prompt filename %q: use --stage classifier|evaluator", filepath.Base(*promptPath))
		}
		fmt.Printf("  Inferred stage: %s\n", resolvedStage)
	}

	// Validate stage value.
	if resolvedStage != "classifier" && resolvedStage != "evaluator" {
		return fmt.Errorf("invalid --stage %q: must be classifier or evaluator", resolvedStage)
	}

	// Read the alternate prompt.
	promptData, err := os.ReadFile(*promptPath)
	if err != nil {
		return fmt.Errorf("reading prompt file %q: %w", *promptPath, err)
	}
	systemPrompt := string(promptData)

	// Build pipeline config.
	cfg := defaults
	cfg.ClassifierMaxTurns = *classifierMaxTurns
	cfg.EvaluatorMaxTurns = *evaluatorMaxTurns
	cfg.ClassifierTimeout = *classifierTimeout
	cfg.EvaluatorTimeout = *evaluatorTimeout
	cfg.ClassifierModel = *classifierModel
	cfg.EvaluatorModel = *evaluatorModel
	cfg.Debug = *debug

	fmt.Printf("Replaying %s stage for session %s\n", resolvedStage, *sessionID)
	fmt.Printf("  Prompt: %s\n", *promptPath)
	fmt.Printf("  Models: classifier=%s, evaluator=%s\n", cfg.ClassifierModel, cfg.EvaluatorModel)

	// Parse the session to get a digest.
	fmt.Println("  Parsing session...")
	digest, err := pipeline.ParseSessionForReplay(*sessionID)
	if err != nil {
		return fmt.Errorf("parsing session: %w", err)
	}

	replayID := pipeline.NewReplayID(*sessionID)
	result := pipeline.ReplayResult{
		ReplayID:   replayID,
		SessionID:  *sessionID,
		Stage:      resolvedStage,
		PromptPath: *promptPath,
	}

	// Look up the original decision for comparison / metadata.
	originalDecision := lookupOriginalDecision(*sessionID)

	switch resolvedStage {
	case "classifier":
		fmt.Println("  Running Classifier with alternate prompt...")
		classOut, _, err := pipeline.RunClassifierWithPrompt(*sessionID, digest, nil, cfg, systemPrompt)
		if err != nil {
			return fmt.Errorf("classifier replay failed: %w", err)
		}
		result.ClassifierOutput = classOut

		fmt.Printf("  Classifier complete: triage=%s, %d errors, %d key turns, %d skill signals\n",
			classOut.Triage,
			len(classOut.ErrorClassification),
			len(classOut.KeyTurns),
			len(classOut.SkillSignals),
		)

		if *compare && originalDecision != "" {
			printClassifierComparison(originalDecision, classOut.Triage)
		}

	case "evaluator":
		// Evaluator needs the classifier output. Read from disk if available,
		// otherwise fail with a helpful message.
		classOut, err := pipeline.ReadClassifierOutput(*sessionID)
		if err != nil {
			return fmt.Errorf("reading classifier output for session %q: %w\n"+
				"Hint: run 'cabrero run %s' first to generate the classifier output", *sessionID, err, *sessionID)
		}

		fmt.Println("  Running Evaluator with alternate prompt...")
		evalOut, _, err := pipeline.RunEvaluatorWithPrompt(*sessionID, digest, classOut, cfg, systemPrompt)
		if err != nil {
			return fmt.Errorf("evaluator replay failed: %w", err)
		}
		result.EvaluatorOutput = evalOut

		fmt.Printf("  Evaluator complete: %d proposals\n", len(evalOut.Proposals))
		if len(evalOut.Proposals) > 0 {
			for _, p := range evalOut.Proposals {
				fmt.Printf("    - %s (%s) → %s\n", p.ID, p.Type, p.Target)
			}
		} else if evalOut.NoProposalReason != nil {
			fmt.Printf("    No proposals: %s\n", *evalOut.NoProposalReason)
		}

		if *compare {
			printEvaluatorComparison(*sessionID, evalOut)
		}
	}

	// Persist the replay result.
	meta := pipeline.ReplayMeta{
		ReplayID:         replayID,
		SessionID:        *sessionID,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Stage:            resolvedStage,
		PromptFile:       *promptPath,
		OriginalDecision: originalDecision,
	}
	if err := pipeline.WriteReplayResult(result, meta); err != nil {
		return fmt.Errorf("persisting replay result: %w", err)
	}

	fmt.Printf("\nReplay complete. Results saved to ~/.cabrero/replays/%s/\n", replayID)
	return nil
}

// lookupOriginalDecision returns the original triage decision for a session
// by checking archived proposals, pending proposals, and evaluator output.
// Returns an empty string when the original decision cannot be determined.
func lookupOriginalDecision(sessionID string) string {
	// Check pending proposals — session has been evaluated and produced proposals.
	proposals, err := pipeline.ListProposals()
	if err == nil {
		for _, p := range proposals {
			if p.SessionID == sessionID {
				return "evaluate"
			}
		}
	}

	// Check archived proposals directory.
	archivedDir := store.ArchivedProposalsDir()
	entries, err := os.ReadDir(archivedDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(archivedDir, e.Name()))
			if err != nil {
				continue
			}
			var wrapper struct {
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(data, &wrapper); err != nil {
				continue
			}
			if wrapper.SessionID == sessionID {
				return "evaluate"
			}
		}
	}

	// Check evaluator output file — produced for sessions that were evaluated.
	evalOut, err := pipeline.ReadEvaluatorOutput(sessionID)
	if err == nil && evalOut != nil {
		if len(evalOut.Proposals) > 0 {
			return "evaluate"
		}
		return "evaluate" // evaluated but produced no proposals
	}

	// Check classifier output for triage decision.
	classOut, err := pipeline.ReadClassifierOutput(sessionID)
	if err == nil && classOut != nil {
		return classOut.Triage
	}

	return ""
}

// printClassifierComparison prints a simple before/after for the triage decision.
func printClassifierComparison(original, replayed string) {
	fmt.Println("\n--- Comparison (classifier triage) ---")
	fmt.Printf("  Original: %s\n", original)
	fmt.Printf("  Replayed: %s\n", replayed)
	if original == replayed {
		fmt.Println("  Result:   same decision")
	} else {
		fmt.Println("  Result:   DECISION CHANGED")
	}
	fmt.Println("--------------------------------------")
}

// printEvaluatorComparison prints a diff of proposals vs. the persisted evaluator output.
func printEvaluatorComparison(sessionID string, newOutput *pipeline.EvaluatorOutput) {
	orig, err := pipeline.ReadEvaluatorOutput(sessionID)
	if err != nil {
		fmt.Printf("\n--- Comparison ---\n  (no original evaluator output found: %v)\n---\n", err)
		return
	}

	fmt.Println("\n--- Comparison (evaluator proposals) ---")
	fmt.Printf("  Original: %d proposals\n", len(orig.Proposals))
	fmt.Printf("  Replayed: %d proposals\n", len(newOutput.Proposals))

	origIDs := make(map[string]bool, len(orig.Proposals))
	for _, p := range orig.Proposals {
		origIDs[p.ID] = true
	}
	newIDs := make(map[string]bool, len(newOutput.Proposals))
	for _, p := range newOutput.Proposals {
		newIDs[p.ID] = true
	}

	var added, removed []string
	for _, p := range newOutput.Proposals {
		if !origIDs[p.ID] {
			added = append(added, p.ID)
		}
	}
	for _, p := range orig.Proposals {
		if !newIDs[p.ID] {
			removed = append(removed, p.ID)
		}
	}

	if len(added) > 0 {
		fmt.Printf("  Added proposals:   %s\n", strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		fmt.Printf("  Removed proposals: %s\n", strings.Join(removed, ", "))
	}
	if len(added) == 0 && len(removed) == 0 {
		fmt.Println("  No proposal-set differences.")
	}
	fmt.Println("-----------------------------------------")
}
