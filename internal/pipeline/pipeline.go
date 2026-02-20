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
	ClassifierMaxTurns int
	EvaluatorMaxTurns  int
	ClassifierTimeout  time.Duration
	EvaluatorTimeout   time.Duration
}

// DefaultPipelineConfig returns production defaults.
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		ClassifierMaxTurns: 15,
		EvaluatorMaxTurns:  20,
		ClassifierTimeout:  2 * time.Minute,
		EvaluatorTimeout:   5 * time.Minute,
	}
}

// RunResult holds the outcome of a pipeline run.
type RunResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
	ClassifierOutput *ClassifierOutput
	// EvaluatorOutput is nil when DryRun is true, when the Classifier triages
	// the session as "clean", or when the Evaluator stage was not reached.
	EvaluatorOutput *EvaluatorOutput
	DryRun          bool
}

// ClassifierResult holds the output of the pre-parser through Classifier stages.
// Used by RunThroughClassifier to return enough data for batch processing.
type ClassifierResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
	ClassifierOutput *ClassifierOutput
}

// preParseResult holds the output of the pre-parse and aggregation stages.
type preParseResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
}

// runPreParseAndAggregate runs the pre-parser and pattern aggregator.
// Shared by RunThroughClassifier and Run (dry-run path).
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

// RunThroughClassifier runs pre-parser, aggregator, and Classifier.
// Returns enough data for batching. Does not invoke the Evaluator.
func RunThroughClassifier(sessionID string, cfg PipelineConfig) (*ClassifierResult, error) {
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

	// Classifier.
	fmt.Println("  Running Classifier...")
	classifierOutput, err := RunClassifier(sessionID, pre.Digest, pre.AggregatorOutput, cfg)
	if err != nil {
		return nil, fmt.Errorf("classifier failed: %w", err)
	}
	if err := WriteClassifierOutput(sessionID, classifierOutput); err != nil {
		return nil, fmt.Errorf("writing classifier output: %w", err)
	}
	fmt.Printf("  Classifier: goal=%q, %d errors, %d key turns, %d skill signals, triage=%s\n",
		classifierOutput.Goal.Summary,
		len(classifierOutput.ErrorClassification),
		len(classifierOutput.KeyTurns),
		len(classifierOutput.SkillSignals),
		classifierOutput.Triage)

	return &ClassifierResult{
		Digest:           pre.Digest,
		AggregatorOutput: pre.AggregatorOutput,
		ClassifierOutput: classifierOutput,
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

	// Full run: delegate to RunThroughClassifier then continue to Evaluator.
	classifierResult, err := RunThroughClassifier(sessionID, cfg)
	if err != nil {
		return nil, err
	}
	result.Digest = classifierResult.Digest
	result.AggregatorOutput = classifierResult.AggregatorOutput
	result.ClassifierOutput = classifierResult.ClassifierOutput

	// Triage gate: skip Evaluator for clean sessions.
	meta, metaErr := store.ReadMetadata(sessionID)
	if classifierResult.ClassifierOutput.Triage == "clean" {
		fmt.Println("  Classifier triage: clean session — skipping Evaluator")
		if metaErr == nil {
			meta.Status = "processed"
			if err := store.WriteMetadata(store.RawDir(sessionID), meta); err != nil {
				fmt.Printf("  Warning: failed to update session status: %v\n", err)
			}
		}
		return result, nil
	}
	fmt.Println("  Classifier triage: session worth evaluating")

	// Evaluator.
	fmt.Println("  Running Evaluator...")
	evaluatorOutput, err := RunEvaluator(sessionID, classifierResult.Digest, classifierResult.ClassifierOutput, cfg)
	if err != nil {
		return nil, fmt.Errorf("evaluator failed: %w", err)
	}
	result.EvaluatorOutput = evaluatorOutput

	if err := WriteEvaluatorOutput(sessionID, evaluatorOutput); err != nil {
		return nil, fmt.Errorf("writing evaluator output: %w", err)
	}

	// Persist proposals.
	for i := range evaluatorOutput.Proposals {
		p := &evaluatorOutput.Proposals[i]
		if err := WriteProposal(p, sessionID); err != nil {
			fmt.Printf("  Warning: failed to write proposal %s: %v\n", p.ID, err)
		}
	}

	if len(evaluatorOutput.Proposals) > 0 {
		fmt.Printf("  Evaluator: %d proposals generated\n", len(evaluatorOutput.Proposals))
	} else {
		reason := "no improvement signals detected"
		if evaluatorOutput.NoProposalReason != nil {
			reason = *evaluatorOutput.NoProposalReason
		}
		fmt.Printf("  Evaluator: no proposals (%s)\n", reason)
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
