package cmd

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/vladolaru/cabrero/internal/apply"
	"github.com/vladolaru/cabrero/internal/pipeline"
)

// Reject archives a proposal with an optional reason.
func Reject(args []string) error {
	fs := flag.NewFlagSet("reject", flag.ContinueOnError)
	reason := fs.String("reason", "", "reason for rejection")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: cabrero reject <proposal_id> [--reason \"text\"]")
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: cabrero reject <proposal_id> [--reason \"text\"]")
	}
	proposalID := fs.Arg(0)

	pw, err := pipeline.ReadProposal(proposalID)
	if err != nil {
		return fmt.Errorf("reading proposal %q: %w", proposalID, err)
	}
	p := &pw.Proposal

	// Show summary.
	fmt.Printf("Proposal: %s\n", p.ID)
	fmt.Printf("Type:     %s\n", p.Type)
	fmt.Printf("Target:   %s\n", p.Target)
	fmt.Printf("Confidence: %s\n", p.Confidence)
	fmt.Println()

	// Prompt for reason if not provided via flag.
	if *reason == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Reason for rejection (optional, press Enter to skip): ")
		input, err := reader.ReadString('\n')
		if err == nil {
			*reason = strings.TrimSpace(input)
		}
	}

	if err := apply.Archive(p.ID, apply.OutcomeRejected, *reason); err != nil {
		return fmt.Errorf("archive failed: %w", err)
	}

	fmt.Printf("Proposal %s rejected and archived.\n", p.ID)
	return nil
}
