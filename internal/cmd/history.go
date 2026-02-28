package cmd

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// History lists pipeline run history.
func History(args []string) error {
	return historyRun(args, os.Stdout)
}

func historyRun(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	limit := fs.Int("limit", 30, "maximum number of records to show")
	statusFilter := fs.String("status", "", "filter by status: processed, error, skipped_busy")
	since := fs.String("since", "", "show records since date (YYYY-MM-DD)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	records, err := pipeline.ReadHistory()
	if err != nil {
		return fmt.Errorf("reading history: %w", err)
	}

	if len(records) == 0 {
		fmt.Fprintln(w, "No pipeline runs recorded yet.")
		return nil
	}

	// Filter by --since.
	if *since != "" {
		cutoff, err := time.Parse("2006-01-02", *since)
		if err != nil {
			return fmt.Errorf("invalid --since date %q (use YYYY-MM-DD): %w", *since, err)
		}
		var filtered []pipeline.HistoryRecord
		for _, r := range records {
			if !r.Timestamp.Before(cutoff) {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}

	// Filter by --status.
	if *statusFilter != "" {
		var filtered []pipeline.HistoryRecord
		for _, r := range records {
			if r.Status == *statusFilter {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}

	if len(records) == 0 {
		fmt.Fprintln(w, "No pipeline runs match the given filters.")
		return nil
	}

	// Reverse to show newest first.
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	total := len(records)
	if *limit > 0 && len(records) > *limit {
		records = records[:*limit]
	}

	fmt.Fprintf(w, "%-14s  %-16s  %-20s  %-10s  %-8s  %-8s  %s\n",
		"SESSION", "WHEN", "PROJECT", "STATUS", "TRIAGE", "COST", "DETAIL")
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────────────────────────────────────────────────────")

	for _, r := range records {
		shortID := store.ShortSessionID(r.SessionID)
		when := cli.RelativeTime(r.Timestamp)

		project := r.Project
		if len(project) > 20 {
			project = "..." + project[len(project)-17:]
		}

		cost := ""
		if r.TotalCostUSD > 0 {
			cost = fmt.Sprintf("$%.3f", r.TotalCostUSD)
		}

		detail := r.ErrorDetail
		if r.GateReason != "" {
			detail = "gate:" + r.GateReason
		}
		if len(detail) > 30 {
			detail = detail[:30] + "..."
		}

		fmt.Fprintf(w, "%-14s  %-16s  %-20s  %-10s  %-8s  %-8s  %s\n",
			shortID, when, project, r.Status, r.Triage, cost, detail)
	}

	fmt.Fprintf(w, "\nShowing %d of %d records.\n", len(records), total)
	return nil
}
