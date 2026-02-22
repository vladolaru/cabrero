package cmd

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Backfill runs the full pipeline on stored sessions matching the given filters.
func Backfill(args []string) error {
	fs := flag.NewFlagSet("backfill", flag.ExitOnError)

	since := fs.String("since", "", "process sessions from this date (YYYY-MM-DD, default: 30 days ago)")
	until := fs.String("until", "", "process sessions up to this date (YYYY-MM-DD, default: now)")
	project := fs.String("project", "", "filter by project slug substring")
	dryRun := fs.Bool("dry-run", false, "show preview only, don't process")
	autoYes := fs.Bool("yes", false, "skip confirmation prompt")
	retryErrors := fs.Bool("retry-errors", false, "also re-process sessions with status 'error'")
	enqueue := fs.Bool("enqueue", false, "mark matching sessions as 'queued' for background daemon processing (non-blocking)")

	classifierMaxTurns := fs.Int("classifier-max-turns", 0, "override Classifier max turns")
	evaluatorMaxTurns := fs.Int("evaluator-max-turns", 0, "override Evaluator max turns")
	classifierTimeout := fs.Duration("classifier-timeout", 0, "override Classifier timeout")
	evaluatorTimeout := fs.Duration("evaluator-timeout", 0, "override Evaluator timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Parse date range.
	filter, err := buildBackfillFilter(*since, *until, *project, *retryErrors)
	if err != nil {
		return err
	}

	// Query matching sessions.
	sessions, err := store.QuerySessions(filter)
	if err != nil {
		return fmt.Errorf("querying sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found matching filters.")
		return nil
	}

	// Build pipeline config.
	cfg := pipeline.DefaultPipelineConfig()
	if *classifierMaxTurns > 0 {
		cfg.ClassifierMaxTurns = *classifierMaxTurns
	}
	if *evaluatorMaxTurns > 0 {
		cfg.EvaluatorMaxTurns = *evaluatorMaxTurns
	}
	if *classifierTimeout > 0 {
		cfg.ClassifierTimeout = *classifierTimeout
	}
	if *evaluatorTimeout > 0 {
		cfg.EvaluatorTimeout = *evaluatorTimeout
	}

	// Show preview.
	showBackfillPreview(sessions, filter)

	if *dryRun {
		return nil
	}

	// Confirm.
	if !*autoYes {
		if !confirmBackfill() {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if *enqueue {
		return enqueueSessions(sessions)
	}

	// Process.
	return runBackfill(sessions, cfg)
}

func buildBackfillFilter(sinceStr, untilStr, project string, retryErrors bool) (store.SessionFilter, error) {
	filter := store.SessionFilter{
		Project:  project,
		Statuses: []string{"imported"},
	}

	if retryErrors {
		filter.Statuses = append(filter.Statuses, "error")
	}

	if sinceStr != "" {
		t, err := time.Parse("2006-01-02", sinceStr)
		if err != nil {
			return filter, fmt.Errorf("invalid --since date %q (use YYYY-MM-DD): %w", sinceStr, err)
		}
		filter.Since = t
	} else {
		// Default: 30 days ago.
		filter.Since = time.Now().AddDate(0, 0, -30)
	}

	if untilStr != "" {
		t, err := time.Parse("2006-01-02", untilStr)
		if err != nil {
			return filter, fmt.Errorf("invalid --until date %q (use YYYY-MM-DD): %w", untilStr, err)
		}
		// End of day.
		filter.Until = t.Add(24*time.Hour - time.Second)
	}

	return filter, nil
}

// enqueueSessions marks sessions as "queued" so the daemon processes them.
func enqueueSessions(sessions []store.Metadata) error {
	enqueued := 0
	for _, s := range sessions {
		if err := store.MarkQueued(s.SessionID); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to enqueue %s: %v\n", s.SessionID, err)
			continue
		}
		enqueued++
	}
	fmt.Printf("Enqueued %d session(s) for background processing.\n", enqueued)
	fmt.Println("The daemon will process them automatically.")
	return nil
}

func showBackfillPreview(sessions []store.Metadata, filter store.SessionFilter) {
	fmt.Println()
	fmt.Println("Backfill Preview")
	fmt.Println(strings.Repeat("═", 40))
	fmt.Println()

	// Count by status.
	importedCount := 0
	errorCount := 0
	for _, s := range sessions {
		switch s.Status {
		case "imported":
			importedCount++
		case "error":
			errorCount++
		}
	}

	statusDesc := fmt.Sprintf("imported (%d)", importedCount)
	if errorCount > 0 {
		statusDesc += fmt.Sprintf(", error (%d)", errorCount)
	}
	fmt.Printf("  Status: %s\n", statusDesc)

	// Date range.
	sinceStr := filter.Since.Format("2006-01-02")
	untilStr := "now"
	if !filter.Until.IsZero() {
		untilStr = filter.Until.Format("2006-01-02")
	}
	fmt.Printf("  Date range: %s → %s\n", sinceStr, untilStr)

	if filter.Project != "" {
		fmt.Printf("  Project filter: *%s*\n", filter.Project)
	}

	// Group by project.
	byProject := make(map[string]int)
	var projectOrder []string
	for _, s := range sessions {
		key := s.Project
		if key == "" {
			key = "(no project)"
		}
		if _, seen := byProject[key]; !seen {
			projectOrder = append(projectOrder, key)
		}
		byProject[key]++
	}

	fmt.Printf("\n  %d session(s) to process across %d project(s):\n", len(sessions), len(projectOrder))
	for _, p := range projectOrder {
		display := store.ProjectDisplayName(p)
		if display == "" {
			display = p
		}
		fmt.Printf("    %-40s (%d sessions)\n", display, byProject[p])
	}

	// Estimate.
	fmt.Printf("\n  Estimated pipeline calls:\n")
	fmt.Printf("    Classifier: %d invocations (Haiku — low cost)\n", len(sessions))
	fmt.Printf("    Evaluator:  up to %d batch invocations (Sonnet — one per project)\n", len(projectOrder))
	fmt.Println()
}

func confirmBackfill() bool {
	fmt.Print("  Proceed? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

func runBackfill(sessions []store.Metadata, cfg pipeline.PipelineConfig) error {
	// Group by project.
	type projectGroup struct {
		project  string
		sessions []store.Metadata
	}
	var groups []projectGroup
	groupMap := make(map[string]int)

	for _, s := range sessions {
		key := s.Project
		if idx, ok := groupMap[key]; ok {
			groups[idx].sessions = append(groups[idx].sessions, s)
		} else {
			groupMap[key] = len(groups)
			groups = append(groups, projectGroup{project: key, sessions: []store.Metadata{s}})
		}
	}

	// Set up cancellation.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Process each project group.
	var totalProcessed, totalClean, totalEvaluated, totalProposals, totalErrors int

	fmt.Printf("Processing %d sessions...\n\n", len(sessions))

	for _, g := range groups {
		select {
		case <-ctx.Done():
			fmt.Println("\nInterrupted. Sessions already processed retain their results.")
			goto summary
		default:
		}

		display := store.ProjectDisplayName(g.project)
		if display == "" {
			display = "(no project)"
		}
		fmt.Printf("  Project: %s (%d sessions)\n", display, len(g.sessions))

		batchSessions := make([]pipeline.BatchSession, len(g.sessions))
		for i, s := range g.sessions {
			batchSessions[i] = pipeline.BatchSession{SessionID: s.SessionID}
		}

		sessionCount := len(g.sessions)
		runner := pipeline.NewRunner(cfg)
		runner.Source = "cli-backfill"
		runner.OnStatus = func(sessionID string, event pipeline.BatchEvent) {
			sid := sessionID
			if len(sid) > 8 {
				sid = sid[:8]
			}
			switch event.Type {
			case "classifier_done":
				idx := 0
				for i, s := range g.sessions {
					if s.SessionID == sessionID {
						idx = i + 1
						break
					}
				}
				fmt.Printf("    [%d/%d] %s — Classifier: %s", idx, sessionCount, sid, event.Triage)
				if event.Triage == "clean" {
					fmt.Print(" ✓")
				}
				fmt.Println()
			case "error":
				fmt.Printf("    %s -- Error: %v\n", sid, event.Error)
			}
		}

		results := runner.RunGroup(ctx, batchSessions)

		// Tally results for this group.
		groupProposals := 0
		groupEvaluated := 0
		for _, r := range results {
			if r.Error != nil {
				totalErrors++
			} else {
				totalProcessed++
			}
			if r.Triage == "clean" {
				totalClean++
			}
			if r.Triage == "evaluate" {
				groupEvaluated++
				totalEvaluated++
			}
			groupProposals += r.Proposals
			totalProposals += r.Proposals
		}

		if groupEvaluated > 0 {
			fmt.Printf("    Evaluator batch: %d sessions → %d proposals generated\n", groupEvaluated, groupProposals)
		}
		fmt.Println()
	}

summary:
	fmt.Println("Summary")
	fmt.Println(strings.Repeat("═", 40))
	fmt.Printf("  Processed: %d sessions\n", totalProcessed)
	fmt.Printf("  Clean (skipped Evaluator): %d\n", totalClean)
	fmt.Printf("  Evaluated: %d\n", totalEvaluated)
	fmt.Printf("  Proposals generated: %d\n", totalProposals)
	fmt.Printf("  Errors: %d\n", totalErrors)

	if totalProposals > 0 {
		fmt.Println("\n  Review proposals with: cabrero proposals")
	}

	return nil
}
