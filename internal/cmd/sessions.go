package cmd

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// Sessions lists captured sessions, most recent first.
func Sessions(args []string) error {
	fs := flag.NewFlagSet("sessions", flag.ExitOnError)
	limit := fs.Int("limit", 20, "maximum number of sessions to show")
	statusFilter := fs.String("status", "all", "filter by status: queued, imported, processed, error, capture_failed, all")
	projectFilter := fs.String("project", "", "filter by project (substring match on slug or display name)")
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

	// Filter by project.
	if *projectFilter != "" {
		needle := strings.ToLower(*projectFilter)
		var filtered []store.Metadata
		for _, s := range sessions {
			slug := strings.ToLower(s.Project)
			display := strings.ToLower(store.ProjectDisplayName(s.Project))
			if strings.Contains(slug, needle) || strings.Contains(display, needle) {
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

	if len(sessions) == 0 {
		fmt.Println("No sessions match the given filters.")
		return nil
	}

	// Print table.
	fmt.Printf("%-14s  %-16s  %-30s  %-12s  %s\n", "SESSION ID", "CAPTURED", "PROJECT", "TRIGGER", "STATUS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────────────────────")

	for _, s := range sessions {
		shortID := store.ShortSessionID(s.SessionID)

		captured := s.Timestamp
		if ts, err := time.Parse(time.RFC3339, s.Timestamp); err == nil {
			captured = ts.Local().Format("2006-01-02 15:04")
		}

		project := store.ProjectDisplayName(s.Project)
		if len(project) > 30 {
			project = "…" + project[len(project)-29:]
		}

		trigger := s.CaptureTrigger
		if len(trigger) > 12 {
			trigger = trigger[:12]
		}

		fmt.Printf("%-14s  %-16s  %-30s  %-12s  %s\n", shortID, captured, project, trigger, s.Status)
	}

	// Footer.
	var filters []string
	if *statusFilter != "all" {
		filters = append(filters, "status="+*statusFilter)
	}
	if *projectFilter != "" {
		filters = append(filters, "project="+*projectFilter)
	}
	filterNote := ""
	if len(filters) > 0 {
		filterNote = " (" + strings.Join(filters, ", ") + ")"
	}
	fmt.Printf("\nShowing %d of %d sessions%s.\n", len(sessions), total, filterNote)

	return nil
}
