package cmd

import (
	"flag"
	"fmt"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// Sessions lists captured sessions, most recent first.
func Sessions(args []string) error {
	fs := flag.NewFlagSet("sessions", flag.ExitOnError)
	limit := fs.Int("limit", 20, "maximum number of sessions to show")
	statusFilter := fs.String("status", "all", "filter by status: pending, processed, all")
	if err := fs.Parse(args); err != nil {
		return err
	}

	sessions, err := store.ListSessions()
	if err != nil {
		return fmt.Errorf("reading sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions captured yet.")
		fmt.Println("Run 'cabrero import --from ~/.claude/projects/' to seed the store.")
		return nil
	}

	// Filter by status.
	if *statusFilter != "all" {
		var filtered []store.Metadata
		for _, s := range sessions {
			if s.Status == *statusFilter {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	total := len(sessions)

	// Apply limit.
	if *limit > 0 && len(sessions) > *limit {
		sessions = sessions[:*limit]
	}

	// Print table header.
	fmt.Printf("%-14s  %-20s  %-14s  %s\n", "SESSION ID", "CAPTURED", "TRIGGER", "STATUS")
	fmt.Println("──────────────────────────────────────────────────────────────────")

	for _, s := range sessions {
		shortID := s.SessionID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		captured := s.Timestamp
		if ts, err := time.Parse(time.RFC3339, s.Timestamp); err == nil {
			captured = ts.Local().Format("2006-01-02 15:04")
		}

		trigger := s.CaptureTrigger
		if len(trigger) > 14 {
			trigger = trigger[:14]
		}

		fmt.Printf("%-14s  %-20s  %-14s  %s\n", shortID, captured, trigger, s.Status)
	}

	if *statusFilter != "all" {
		fmt.Printf("\nShowing %d of %d %s sessions.\n", len(sessions), total, *statusFilter)
	} else {
		fmt.Printf("\nShowing %d of %d sessions.\n", len(sessions), total)
	}

	return nil
}
