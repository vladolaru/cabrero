package pipeline

import (
	"fmt"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
	"github.com/vladolaru/cabrero/internal/store"
)

// PipelineConfig controls LLM invocation parameters.
type PipelineConfig struct {
	HaikuMaxTurns  int
	SonnetMaxTurns int
	HaikuTimeout   time.Duration
	SonnetTimeout  time.Duration
}

// DefaultPipelineConfig returns production defaults.
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		HaikuMaxTurns:  15,
		SonnetMaxTurns: 20,
		HaikuTimeout:   2 * time.Minute,
		SonnetTimeout:  5 * time.Minute,
	}
}

// RunResult holds the outcome of a pipeline run.
type RunResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
	HaikuOutput      *HaikuOutput
	// SonnetOutput is nil when DryRun is true, when Haiku triages the session
	// as "clean", or when the Sonnet stage was not reached.
	SonnetOutput *SonnetOutput
	DryRun       bool
}

// HaikuResult holds the output of the pre-parser through Haiku stages.
// Used by RunThroughHaiku to return enough data for batch processing.
type HaikuResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
	HaikuOutput      *HaikuOutput
}

// preParseResult holds the output of the pre-parse and aggregation stages.
type preParseResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
}

// runPreParseAndAggregate runs the pre-parser and pattern aggregator.
// Shared by RunThroughHaiku and Run (dry-run path).
func runPreParseAndAggregate(sessionID string) (*preParseResult, error) {
	fmt.Printf("  Parsing session %s...\n", sessionID)
	digest, err := parser.ParseSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("pre-parser failed: %w", err)
	}
	if err := parser.WriteDigest(digest); err != nil {
		return nil, fmt.Errorf("writing digest: %w", err)
	}
	fmt.Printf("  Digest written: %d entries, %d turns, %d errors, %d friction signals\n",
		digest.Shape.EntryCount, digest.Shape.TurnCount, len(digest.Errors), len(digest.ToolCalls.FrictionSignals))

	var aggregatorOutput *patterns.AggregatorOutput
	meta, metaErr := store.ReadMetadata(sessionID)
	if metaErr == nil && meta.Project != "" {
		fmt.Println("  Aggregating cross-session patterns...")
		aggregatorOutput, err = patterns.Aggregate(sessionID, meta.Project)
		if err != nil {
			fmt.Printf("  Warning: pattern aggregation failed: %v\n", err)
		} else if aggregatorOutput != nil {
			fmt.Printf("  Found %d recurring pattern(s) across %d sessions\n",
				len(aggregatorOutput.Patterns), aggregatorOutput.SessionsScanned)
		}
	}

	return &preParseResult{
		Digest:           digest,
		AggregatorOutput: aggregatorOutput,
	}, nil
}

// RunThroughHaiku runs pre-parser, aggregator, and Haiku classifier.
// Returns enough data for batching. Does not invoke Sonnet.
func RunThroughHaiku(sessionID string, cfg PipelineConfig) (*HaikuResult, error) {
	if !store.SessionExists(sessionID) {
		return nil, fmt.Errorf("session %s not found in store", sessionID)
	}
	if err := EnsurePrompts(); err != nil {
		return nil, fmt.Errorf("ensuring prompt files: %w", err)
	}

	pre, err := runPreParseAndAggregate(sessionID)
	if err != nil {
		return nil, err
	}

	// Haiku classifier.
	fmt.Println("  Running Haiku classifier...")
	haikuOutput, err := RunHaiku(sessionID, pre.Digest, pre.AggregatorOutput, cfg)
	if err != nil {
		return nil, fmt.Errorf("haiku classifier failed: %w", err)
	}
	if err := WriteHaikuOutput(sessionID, haikuOutput); err != nil {
		return nil, fmt.Errorf("writing haiku output: %w", err)
	}
	fmt.Printf("  Haiku: goal=%q, %d errors, %d key turns, %d skill signals, triage=%s\n",
		haikuOutput.Goal.Summary,
		len(haikuOutput.ErrorClassification),
		len(haikuOutput.KeyTurns),
		len(haikuOutput.SkillSignals),
		haikuOutput.Triage)

	return &HaikuResult{
		Digest:           pre.Digest,
		AggregatorOutput: pre.AggregatorOutput,
		HaikuOutput:      haikuOutput,
	}, nil
}

// Run executes the full analysis pipeline for a session.
// If dryRun is true, only the pre-parser runs (no LLM invocations).
func Run(sessionID string, dryRun bool, cfg PipelineConfig) (*RunResult, error) {
	if !store.SessionExists(sessionID) {
		return nil, fmt.Errorf("session %s not found in store", sessionID)
	}
	if err := EnsurePrompts(); err != nil {
		return nil, fmt.Errorf("ensuring prompt files: %w", err)
	}

	result := &RunResult{DryRun: dryRun}

	if dryRun {
		pre, err := runPreParseAndAggregate(sessionID)
		if err != nil {
			return nil, err
		}
		result.Digest = pre.Digest
		result.AggregatorOutput = pre.AggregatorOutput
		fmt.Println("  Dry run — stopping after pre-parser.")
		return result, nil
	}

	// Full run: delegate to RunThroughHaiku then continue to Sonnet.
	haikuResult, err := RunThroughHaiku(sessionID, cfg)
	if err != nil {
		return nil, err
	}
	result.Digest = haikuResult.Digest
	result.AggregatorOutput = haikuResult.AggregatorOutput
	result.HaikuOutput = haikuResult.HaikuOutput

	// Triage gate: skip Sonnet for clean sessions.
	meta, metaErr := store.ReadMetadata(sessionID)
	if haikuResult.HaikuOutput.Triage == "clean" {
		fmt.Println("  Haiku triage: clean session — skipping Sonnet evaluator")
		if metaErr == nil {
			meta.Status = "processed"
			if err := store.WriteMetadata(store.RawDir(sessionID), meta); err != nil {
				fmt.Printf("  Warning: failed to update session status: %v\n", err)
			}
		}
		return result, nil
	}
	fmt.Println("  Haiku triage: session worth evaluating")

	// Sonnet evaluator.
	fmt.Println("  Running Sonnet evaluator...")
	sonnetOutput, err := RunSonnet(sessionID, haikuResult.Digest, haikuResult.HaikuOutput, cfg)
	if err != nil {
		return nil, fmt.Errorf("sonnet evaluator failed: %w", err)
	}
	result.SonnetOutput = sonnetOutput

	if err := WriteSonnetOutput(sessionID, sonnetOutput); err != nil {
		return nil, fmt.Errorf("writing sonnet output: %w", err)
	}

	// Persist proposals.
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
	if metaErr == nil {
		meta.Status = "processed"
		if err := store.WriteMetadata(store.RawDir(sessionID), meta); err != nil {
			fmt.Printf("  Warning: failed to update session status: %v\n", err)
		}
	}

	return result, nil
}
