package cmd

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Proposals lists proposals filtered by status.
func Proposals(args []string) error {
	return proposalsRun(args, os.Stdout)
}

func proposalsRun(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("proposals", flag.ContinueOnError)
	statusFilter := fs.String("status", "pending", "filter: pending, approved, rejected, deferred, culled, all")
	limit := fs.Int("limit", 50, "maximum number of proposals to show")
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch *statusFilter {
	case "pending":
		return listPendingProposals(w, *limit)
	case "all":
		return listAllProposals(w, *limit)
	case "approved", "rejected", "deferred", "culled", "auto-rejected":
		return listArchivedByOutcome(w, *statusFilter, *limit)
	default:
		return fmt.Errorf("unknown status %q (valid: pending, approved, rejected, deferred, culled, all)", *statusFilter)
	}
}

func listPendingProposals(w io.Writer, limit int) error {
	proposals, err := pipeline.ListProposals()
	if err != nil {
		return fmt.Errorf("reading proposals: %w", err)
	}

	if len(proposals) == 0 {
		fmt.Fprintln(w, "No pending proposals.")
		return nil
	}

	if limit > 0 && len(proposals) > limit {
		proposals = proposals[:limit]
	}

	fmt.Fprintf(w, "%-28s  %-22s  %-10s  %-12s  %s\n", "PROPOSAL ID", "TYPE", "CONFIDENCE", "SESSION", "TARGET")
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────────────────────────────────────────────────")

	for _, pw := range proposals {
		p := pw.Proposal
		shortSession := store.ShortSessionID(pw.SessionID)
		target := p.Target
		if len(target) > 40 {
			target = "…" + target[len(target)-39:]
		}
		fmt.Fprintf(w, "%-28s  %-22s  %-10s  %-12s  %s\n",
			p.ID, p.Type, p.Confidence, shortSession, target)
	}

	fmt.Fprintf(w, "\n%d pending proposals.\n", len(proposals))
	return nil
}

func listArchivedByOutcome(w io.Writer, outcome string, limit int) error {
	archived, err := pipeline.ListArchivedProposals()
	if err != nil {
		return fmt.Errorf("reading archived proposals: %w", err)
	}

	var filtered []pipeline.ArchivedProposal
	for _, ap := range archived {
		if ap.Outcome == outcome {
			filtered = append(filtered, ap)
		}
	}

	if len(filtered) == 0 {
		fmt.Fprintf(w, "No %s proposals.\n", outcome)
		return nil
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	fmt.Fprintf(w, "%-28s  %-22s  %-10s  %-12s  %-16s  %s\n",
		"PROPOSAL ID", "TYPE", "CONFIDENCE", "SESSION", "ARCHIVED", "TARGET")
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────────────────────────────────────────────────────────────")

	for _, ap := range filtered {
		p := ap.Proposal
		shortSession := store.ShortSessionID(ap.SessionID)
		target := p.Target
		if len(target) > 35 {
			target = "…" + target[len(target)-34:]
		}
		archived := cli.RelativeTime(ap.ArchivedAt)
		fmt.Fprintf(w, "%-28s  %-22s  %-10s  %-12s  %-16s  %s\n",
			p.ID, p.Type, p.Confidence, shortSession, archived, target)
	}

	fmt.Fprintf(w, "\n%d %s proposals.\n", len(filtered), outcome)
	return nil
}

func listAllProposals(w io.Writer, limit int) error {
	pending, err := pipeline.ListProposals()
	if err != nil {
		return fmt.Errorf("reading proposals: %w", err)
	}
	archived, err := pipeline.ListArchivedProposals()
	if err != nil {
		return fmt.Errorf("reading archived proposals: %w", err)
	}

	total := len(pending) + len(archived)
	if total == 0 {
		fmt.Fprintln(w, "No proposals yet.")
		return nil
	}

	fmt.Fprintf(w, "%-28s  %-22s  %-10s  %-14s  %-12s  %s\n",
		"PROPOSAL ID", "TYPE", "CONFIDENCE", "STATUS", "SESSION", "TARGET")
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────")

	shown := 0
	for _, pw := range pending {
		if limit > 0 && shown >= limit {
			break
		}
		p := pw.Proposal
		shortSession := store.ShortSessionID(pw.SessionID)
		target := p.Target
		if len(target) > 35 {
			target = "…" + target[len(target)-34:]
		}
		fmt.Fprintf(w, "%-28s  %-22s  %-10s  %-14s  %-12s  %s\n",
			p.ID, p.Type, p.Confidence, "pending", shortSession, target)
		shown++
	}
	for _, ap := range archived {
		if limit > 0 && shown >= limit {
			break
		}
		p := ap.Proposal
		shortSession := store.ShortSessionID(ap.SessionID)
		target := p.Target
		if len(target) > 35 {
			target = "…" + target[len(target)-34:]
		}
		fmt.Fprintf(w, "%-28s  %-22s  %-10s  %-14s  %-12s  %s\n",
			p.ID, p.Type, p.Confidence, ap.Outcome, shortSession, target)
		shown++
	}

	fmt.Fprintf(w, "\n%d proposals (%d pending, %d archived).\n", total, len(pending), len(archived))
	return nil
}
