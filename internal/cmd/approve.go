package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/vladolaru/cabrero/internal/apply"
	"github.com/vladolaru/cabrero/internal/pipeline"
)

// Approve runs the non-interactive approve flow for a proposal.
func Approve(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero approve <proposal_id>")
	}
	proposalID := args[0]

	pw, err := pipeline.ReadProposal(proposalID)
	if err != nil {
		return fmt.Errorf("reading proposal %q: %w", proposalID, err)
	}
	p := &pw.Proposal

	// Show proposal summary.
	fmt.Printf("Proposal: %s\n", p.ID)
	fmt.Printf("Type:     %s\n", p.Type)
	fmt.Printf("Target:   %s\n", p.Target)
	fmt.Printf("Confidence: %s\n", p.Confidence)
	fmt.Println()
	if p.Rationale != "" {
		fmt.Printf("Rationale: %s\n\n", truncateStr(p.Rationale, 200))
	}

	// Confirm.
	if !promptYesNo("Apply this change?") {
		fmt.Println("Cancelled.")
		return nil
	}

	// Blend.
	fmt.Println("Blending change with Claude...")
	blended, err := apply.Blend(p, pw.SessionID)
	if err != nil {
		return fmt.Errorf("blend failed: %w", err)
	}

	// Show result.
	fmt.Println("\n--- Blended result ---")
	fmt.Println(blended)
	fmt.Println("--- End result ---")

	// Confirm commit.
	if !promptYesNo("Commit this change?") {
		fmt.Println("Cancelled. No changes written.")
		return nil
	}

	// Write file.
	if err := apply.Commit(p, blended); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	// Archive proposal.
	if err := apply.Archive(p.ID, "approved"); err != nil {
		return fmt.Errorf("archive failed: %w", err)
	}

	fmt.Printf("Change applied to %s and proposal archived.\n", p.Target)
	return nil
}

func promptYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N] ", question)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	return strings.TrimSpace(strings.ToLower(input)) == "y"
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
