package pipeline

import (
	"fmt"
	"os"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
	"github.com/vladolaru/cabrero/internal/store"
)

// Logger receives pipeline progress and diagnostic messages.
type Logger interface {
	Info(format string, args ...any)
	Error(format string, args ...any)
}

// stdLogger writes Info to stdout and Error to stderr.
// Format strings carry their own indentation (e.g. "  Parsing session %s...").
type stdLogger struct{}

func (stdLogger) Info(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

func (stdLogger) Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// discardLogger silently drops all messages.
type discardLogger struct{}

func (discardLogger) Info(string, ...any)  {}
func (discardLogger) Error(string, ...any) {}

// PipelineConfig controls LLM invocation parameters.
type PipelineConfig struct {
	ClassifierMaxTurns int
	EvaluatorMaxTurns  int
	ClassifierTimeout  time.Duration
	EvaluatorTimeout   time.Duration
	Logger             Logger // nil defaults to stdLogger (stdout/stderr)
}

// logger returns the configured Logger, falling back to stdLogger.
func (c PipelineConfig) logger() Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return stdLogger{}
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
func runPreParseAndAggregate(sessionID string, log Logger) (*preParseResult, error) {
	log.Info("  Parsing session %s...", sessionID)
	digest, err := parser.ParseSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("pre-parser failed: %w", err)
	}
	if err := parser.WriteDigest(digest); err != nil {
		return nil, fmt.Errorf("writing digest: %w", err)
	}
	log.Info("  Digest written: %d entries, %d turns, %d errors, %d friction signals",
		digest.Shape.EntryCount, digest.Shape.TurnCount, len(digest.Errors), len(digest.ToolCalls.FrictionSignals))

	var aggregatorOutput *patterns.AggregatorOutput
	meta, metaErr := store.ReadMetadata(sessionID)
	if metaErr == nil && meta.Project != "" {
		log.Info("  Aggregating cross-session patterns...")
		aggregatorOutput, err = patterns.Aggregate(sessionID, meta.Project)
		if err != nil {
			log.Info("  Warning: pattern aggregation failed: %v", err)
		} else if aggregatorOutput != nil {
			log.Info("  Found %d recurring pattern(s) across %d sessions",
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

	log := cfg.logger()

	pre, err := runPreParseAndAggregate(sessionID, log)
	if err != nil {
		return nil, err
	}

	// Classifier.
	log.Info("  Running Classifier...")
	classifierOutput, err := RunClassifier(sessionID, pre.Digest, pre.AggregatorOutput, cfg)
	if err != nil {
		return nil, fmt.Errorf("classifier failed: %w", err)
	}
	if err := WriteClassifierOutput(sessionID, classifierOutput); err != nil {
		return nil, fmt.Errorf("writing classifier output: %w", err)
	}
	log.Info("  Classifier: goal=%q, %d errors, %d key turns, %d skill signals, triage=%s",
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

	log := cfg.logger()
	result := &RunResult{DryRun: dryRun}

	if dryRun {
		pre, err := runPreParseAndAggregate(sessionID, log)
		if err != nil {
			return nil, err
		}
		result.Digest = pre.Digest
		result.AggregatorOutput = pre.AggregatorOutput
		log.Info("  Dry run — stopping after pre-parser.")
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
		log.Info("  Classifier triage: clean session — skipping Evaluator")
		if metaErr == nil {
			meta.Status = "processed"
			if err := store.WriteMetadata(store.RawDir(sessionID), meta); err != nil {
				log.Error("  Warning: failed to update session status: %v", err)
			}
		}
		return result, nil
	}
	log.Info("  Classifier triage: session worth evaluating")

	// Evaluator.
	log.Info("  Running Evaluator...")
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
			log.Error("  Warning: failed to write proposal %s: %v", p.ID, err)
		}
	}

	if len(evaluatorOutput.Proposals) > 0 {
		log.Info("  Evaluator: %d proposals generated", len(evaluatorOutput.Proposals))
	} else {
		reason := "no improvement signals detected"
		if evaluatorOutput.NoProposalReason != nil {
			reason = *evaluatorOutput.NoProposalReason
		}
		log.Info("  Evaluator: no proposals (%s)", reason)
	}

	// Mark session as processed.
	if metaErr == nil {
		meta.Status = "processed"
		if err := store.WriteMetadata(store.RawDir(sessionID), meta); err != nil {
			log.Error("  Warning: failed to update session status: %v", err)
		}
	}

	return result, nil
}
