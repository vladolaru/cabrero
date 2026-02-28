package cmd

import (
	"flag"
	"fmt"

	"github.com/vladolaru/cabrero/internal/apply"
	"github.com/vladolaru/cabrero/internal/pipeline"
)

// Defer archives a proposal as deferred (revisit later).
func Defer(args []string) error {
	fs := flag.NewFlagSet("defer", flag.ContinueOnError)
	reason := fs.String("reason", "", "reason for deferring")
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: cabrero defer <proposal_id> [--reason \"text\"] [--yes]")
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: cabrero defer <proposal_id> [--reason \"text\"] [--yes]")
	}
	proposalID := fs.Arg(0)

	pw, err := pipeline.ReadProposal(proposalID)
	if err != nil {
		return fmt.Errorf("reading proposal %q: %w", proposalID, err)
	}
	p := &pw.Proposal

	fmt.Printf("Proposal: %s\n", p.ID)
	fmt.Printf("Type:     %s\n", p.Type)
	fmt.Printf("Target:   %s\n", p.Target)
	fmt.Println()

	if !*yes {
		if !promptYesNo("Defer this proposal?") {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if err := apply.Archive(p.ID, apply.OutcomeDeferred, *reason); err != nil {
		return fmt.Errorf("archive failed: %w", err)
	}

	fmt.Printf("Proposal %s deferred and archived.\n", p.ID)
	return nil
}
