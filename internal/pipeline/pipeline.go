package pipeline

import (
	"fmt"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/store"
)

// RunResult holds the outcome of a pipeline run.
type RunResult struct {
	Digest       *parser.Digest
	HaikuOutput  *HaikuOutput
	SonnetOutput *SonnetOutput
	DryRun       bool
}

// Run executes the full analysis pipeline for a session.
// If dryRun is true, only the pre-parser runs (no LLM invocations).
func Run(sessionID string, dryRun bool) (*RunResult, error) {
	// Verify session exists.
	if !store.SessionExists(sessionID) {
		return nil, fmt.Errorf("session %s not found in store", sessionID)
	}

	// Ensure prompt files exist (cheap file write, always safe to run).
	if err := EnsurePrompts(); err != nil {
		return nil, fmt.Errorf("ensuring prompt files: %w", err)
	}

	result := &RunResult{DryRun: dryRun}

	// Step 1: Pre-parse.
	fmt.Printf("  Parsing session %s...\n", sessionID)
	digest, err := parser.ParseSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("pre-parser failed: %w", err)
	}
	result.Digest = digest

	// Write digest to disk.
	if err := parser.WriteDigest(digest); err != nil {
		return nil, fmt.Errorf("writing digest: %w", err)
	}
	fmt.Printf("  Digest written: %d entries, %d turns, %d errors\n",
		digest.Shape.EntryCount, digest.Shape.TurnCount, len(digest.Errors))

	if dryRun {
		fmt.Println("  Dry run — stopping after pre-parser.")
		return result, nil
	}

	// Step 2: Haiku classifier.
	fmt.Println("  Running Haiku classifier...")
	haikuOutput, err := RunHaiku(sessionID, digest)
	if err != nil {
		return nil, fmt.Errorf("haiku classifier failed: %w", err)
	}
	result.HaikuOutput = haikuOutput

	if err := WriteHaikuOutput(sessionID, haikuOutput); err != nil {
		return nil, fmt.Errorf("writing haiku output: %w", err)
	}
	fmt.Printf("  Haiku: goal=%q, %d errors, %d key turns, %d skill signals\n",
		haikuOutput.Goal.Summary,
		len(haikuOutput.ErrorClassification),
		len(haikuOutput.KeyTurns),
		len(haikuOutput.SkillSignals))

	// Step 3: Sonnet evaluator.
	fmt.Println("  Running Sonnet evaluator...")
	sonnetOutput, err := RunSonnet(sessionID, digest, haikuOutput)
	if err != nil {
		return nil, fmt.Errorf("sonnet evaluator failed: %w", err)
	}
	result.SonnetOutput = sonnetOutput

	if err := WriteSonnetOutput(sessionID, sonnetOutput); err != nil {
		return nil, fmt.Errorf("writing sonnet output: %w", err)
	}

	// Step 4: Persist proposals.
	for i := range sonnetOutput.Proposals {
		p := &sonnetOutput.Proposals[i]
		if err := WriteProposal(p, sessionID); err != nil {
			fmt.Printf("  Warning: failed to write proposal %s: %v\n", p.ID, err)
		}
	}

	if len(sonnetOutput.Proposals) > 0 {
		fmt.Printf("  Sonnet: %d proposals generated\n", len(sonnetOutput.Proposals))
	} else {
		reason := "no improvement signals detected"
		if sonnetOutput.NoProposalReason != nil {
			reason = *sonnetOutput.NoProposalReason
		}
		fmt.Printf("  Sonnet: no proposals (%s)\n", reason)
	}

	// Mark session as processed.
	meta, err := store.ReadMetadata(sessionID)
	if err == nil {
		meta.Status = "processed"
		if err := store.WriteMetadata(store.RawDir(sessionID), meta); err != nil {
			fmt.Printf("  Warning: failed to update session status: %v\n", err)
		}
	}

	return result, nil
}
