package cmd

import (
	"fmt"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Proposals lists all pending proposals.
func Proposals(args []string) error {
	proposals, err := pipeline.ListProposals()
	if err != nil {
		return fmt.Errorf("reading proposals: %w", err)
	}

	if len(proposals) == 0 {
		fmt.Println("No proposals yet.")
		fmt.Println("Run 'cabrero run <session_id>' to analyse a session.")
		return nil
	}

	fmt.Printf("%-28s  %-22s  %-10s  %-12s  %s\n", "PROPOSAL ID", "TYPE", "CONFIDENCE", "SESSION", "TARGET")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────────────────────────────")

	for _, pw := range proposals {
		p := pw.Proposal
		shortSession := store.ShortSessionID(pw.SessionID)

		target := p.Target
		if len(target) > 40 {
			target = "…" + target[len(target)-39:]
		}

		fmt.Printf("%-28s  %-22s  %-10s  %-12s  %s\n",
			p.ID, p.Type, p.Confidence, shortSession, target)
	}

	fmt.Printf("\n%d proposals. Run 'cabrero inspect <proposal_id>' for details.\n", len(proposals))
	return nil
}
