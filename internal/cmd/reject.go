package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/vladolaru/cabrero/internal/apply"
	"github.com/vladolaru/cabrero/internal/pipeline"
)

// Reject archives a proposal with an optional reason.
func Reject(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero reject <proposal_id> [--reason \"text\"]")
	}
	proposalID := args[0]

	// Parse --reason flag.
	var reason string
	for i := 1; i < len(args); i++ {
		if args[i] == "--reason" && i+1 < len(args) {
			reason = args[i+1]
			break
		}
	}

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
	if reason == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Reason for rejection (optional, press Enter to skip): ")
		input, err := reader.ReadString('\n')
		if err == nil {
			reason = strings.TrimSpace(input)
		}
	}

	archiveReason := "rejected"
	if reason != "" {
		archiveReason = "rejected: " + reason
	}

	if err := apply.Archive(p.ID, archiveReason); err != nil {
		return fmt.Errorf("archive failed: %w", err)
	}

	fmt.Printf("Proposal %s rejected and archived.\n", p.ID)
	return nil
}
