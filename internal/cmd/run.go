package cmd

import (
	"flag"
	"fmt"

	"github.com/vladolaru/cabrero/internal/pipeline"
)

// Run executes the full analysis pipeline on a session.
func Run(args []string) error {
	defaults := pipeline.DefaultPipelineConfig()
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "run only the pre-parser, skip LLM invocations")
	classifierMaxTurns := fs.Int("classifier-max-turns", defaults.ClassifierMaxTurns, "max agentic turns for Classifier")
	evaluatorMaxTurns := fs.Int("evaluator-max-turns", defaults.EvaluatorMaxTurns, "max agentic turns for Evaluator")
	classifierTimeout := fs.Duration("classifier-timeout", defaults.ClassifierTimeout, "timeout for Classifier")
	evaluatorTimeout := fs.Duration("evaluator-timeout", defaults.EvaluatorTimeout, "timeout for Evaluator")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: cabrero run [--dry-run] <session_id>")
	}

	sessionID := fs.Arg(0)
	fmt.Printf("Running pipeline on session %s\n", sessionID)

	cfg := defaults
	cfg.ClassifierMaxTurns = *classifierMaxTurns
	cfg.EvaluatorMaxTurns = *evaluatorMaxTurns
	cfg.ClassifierTimeout = *classifierTimeout
	cfg.EvaluatorTimeout = *evaluatorTimeout

	result, err := pipeline.Run(sessionID, *dryRun, cfg)
	if err != nil {
		return err
	}

	fmt.Println()
	if result.DryRun {
		fmt.Println("Dry run complete. Digest written to ~/.cabrero/digests/")
	} else {
		fmt.Println("Pipeline complete.")
		if result.EvaluatorOutput != nil && len(result.EvaluatorOutput.Proposals) > 0 {
			fmt.Printf("Run 'cabrero proposals' to see %d new proposals.\n", len(result.EvaluatorOutput.Proposals))
		}
	}

	return nil
}
