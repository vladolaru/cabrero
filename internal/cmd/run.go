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
	haikuMaxTurns := fs.Int("haiku-max-turns", defaults.HaikuMaxTurns, "max agentic turns for Haiku classifier")
	sonnetMaxTurns := fs.Int("sonnet-max-turns", defaults.SonnetMaxTurns, "max agentic turns for Sonnet evaluator")
	haikuTimeout := fs.Duration("haiku-timeout", defaults.HaikuTimeout, "timeout for Haiku classifier")
	sonnetTimeout := fs.Duration("sonnet-timeout", defaults.SonnetTimeout, "timeout for Sonnet evaluator")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: cabrero run [--dry-run] <session_id>")
	}

	sessionID := fs.Arg(0)
	fmt.Printf("Running pipeline on session %s\n", sessionID)

	cfg := defaults
	cfg.HaikuMaxTurns = *haikuMaxTurns
	cfg.SonnetMaxTurns = *sonnetMaxTurns
	cfg.HaikuTimeout = *haikuTimeout
	cfg.SonnetTimeout = *sonnetTimeout

	result, err := pipeline.Run(sessionID, *dryRun, cfg)
	if err != nil {
		return err
	}

	fmt.Println()
	if result.DryRun {
		fmt.Println("Dry run complete. Digest written to ~/.cabrero/digests/")
	} else {
		fmt.Println("Pipeline complete.")
		if result.SonnetOutput != nil && len(result.SonnetOutput.Proposals) > 0 {
			fmt.Printf("Run 'cabrero proposals' to see %d new proposals.\n", len(result.SonnetOutput.Proposals))
		}
	}

	return nil
}
