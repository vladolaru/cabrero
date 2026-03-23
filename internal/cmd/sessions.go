package cmd

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// Sessions lists captured sessions, most recent first.
// If the first positional arg is "purge", routes to the purge subcommand.
func Sessions(args []string) error {
	if len(args) > 0 && args[0] == "purge" {
		return sessionsPurge(args[1:])
	}

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

// purgeableStatuses defines which session statuses can be purged.
// Only terminal failure states are allowed — this prevents accidental
// deletion of queued, imported, or processed sessions.
var purgeableStatuses = map[string]bool{
	store.StatusError:         true,
	store.StatusCaptureFailed: true,
}

func sessionsPurge(args []string) error {
	return sessionsPurgeRun(args, os.Stdout)
}

func sessionsPurgeRun(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("sessions purge", flag.ContinueOnError)
	fs.SetOutput(w)
	statusFlag := fs.String("status", "", "comma-separated statuses to purge (required; allowed: error, capture_failed)")
	dryRun := fs.Bool("dry-run", false, "list sessions that would be purged without removing them")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Require --status.
	if *statusFlag == "" {
		return fmt.Errorf("--status is required (allowed: error, capture_failed)")
	}

	// Parse and validate statuses.
	parts := strings.Split(*statusFlag, ",")
	var statuses []string
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		if !purgeableStatuses[s] {
			return fmt.Errorf("status %q is not purgeable (allowed: error, capture_failed)", s)
		}
		statuses = append(statuses, s)
	}
	if len(statuses) == 0 {
		return fmt.Errorf("--status is required (allowed: error, capture_failed)")
	}

	if *dryRun {
		// List matching sessions without removing them.
		sessions, err := store.ListSessions()
		if err != nil {
			return fmt.Errorf("listing sessions: %w", err)
		}

		statusSet := make(map[string]bool, len(statuses))
		for _, s := range statuses {
			statusSet[s] = true
		}

		var matching []store.Metadata
		for _, s := range sessions {
			if statusSet[s.Status] {
				matching = append(matching, s)
			}
		}

		if len(matching) == 0 {
			fmt.Fprintln(w, "No sessions match the given statuses.")
			return nil
		}

		fmt.Fprintf(w, "Would purge %d session(s):\n", len(matching))
		for _, s := range matching {
			fmt.Fprintf(w, "  %s  %-30s  %s\n",
				store.ShortSessionID(s.SessionID),
				store.ProjectDisplayName(s.Project),
				s.Status,
			)
		}
		return nil
	}

	// Execute purge.
	removed, err := store.PurgeSessions(statuses)
	if err != nil {
		return fmt.Errorf("purging sessions: %w", err)
	}
	fmt.Fprintf(w, "Purged %d session(s).\n", removed)
	return nil
}
