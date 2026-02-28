package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/daemon"
	claude "github.com/vladolaru/cabrero/internal/integration/claude"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Status shows pipeline health and store overview.
func Status(args []string) error {
	root := store.Root()
	fmt.Println()
	fmt.Println(cli.Bold("Cabrero Status"))
	fmt.Println(cli.Accent(strings.Repeat("─", 30)))

	// Store path and status.
	display := cli.ShortenHome(root)
	if _, err := os.Stat(root); err == nil {
		fmt.Printf("  %s  %s %s\n", cli.Bold("Store:"), display, cli.Success("(initialized)"))
	} else {
		fmt.Printf("  %s  %s %s\n", cli.Bold("Store:"), display, cli.Warn("(not initialized)"))
	}

	// Session counts.
	sessions, err := store.ListSessions()
	if err != nil {
		fmt.Printf("  %s  %s\n", cli.Bold("Sessions:"), cli.Error(fmt.Sprintf("error reading (%v)", err)))
	} else {
		queued := 0
		imported := 0
		processed := 0
		errored := 0
		captureFailed := 0
		for _, s := range sessions {
			switch s.Status {
			case "queued":
				queued++
			case "imported":
				imported++
			case "processed":
				processed++
			case "error":
				errored++
			case "capture_failed":
				captureFailed++
			}
		}
		statusLine := fmt.Sprintf("  %s  %d captured, %d queued, %d imported, %d processed",
			cli.Bold("Sessions:"), len(sessions), queued, imported, processed)
		if errored > 0 {
			statusLine += fmt.Sprintf(", %s", cli.Error(fmt.Sprintf("%d errored", errored)))
		}
		if captureFailed > 0 {
			statusLine += fmt.Sprintf(", %s", cli.Muted(fmt.Sprintf("%d capture_failed", captureFailed)))
		}
		fmt.Println(statusLine)
	}

	// Blocklist count.
	blCount := store.BlocklistLen()
	fmt.Printf("  %s  %d entries\n", cli.Bold("Blocklist:"), blCount)

	// Last capture.
	if len(sessions) > 0 {
		latest := sessions[0]
		ts, err := time.Parse(time.RFC3339, latest.Timestamp)
		capDisplay := latest.Timestamp
		if err == nil {
			capDisplay = ts.Local().Format("2006-01-02 15:04")
		}
		fmt.Printf("  %s  %s %s\n", cli.Bold("Last capture:"), capDisplay, cli.Muted("(session "+store.ShortSessionID(latest.SessionID)+")"))
	} else {
		fmt.Printf("  %s  %s\n", cli.Bold("Last capture:"), cli.Muted("none"))
	}

	// Daemon status.
	if pid, alive := daemon.IsDaemonRunning(); alive {
		fmt.Printf("  %s  %s %s\n", cli.Bold("Daemon:"), cli.Success("running"), cli.Muted(fmt.Sprintf("(PID %d)", pid)))
	} else {
		fmt.Printf("  %s  %s\n", cli.Bold("Daemon:"), cli.Warn("not running"))
	}

	// Hook status.
	settingsPath, _ := claude.SettingsPath()
	preCompact, sessionEnd := claude.HookStatus(settingsPath)
	fmt.Printf("  %s  pre-compact %s   session-end %s\n",
		cli.Bold("Hooks:"), hookStatus(preCompact), hookStatus(sessionEnd))

	// Pipeline models and prompts.
	cfg := pipeline.DefaultPipelineConfig()
	prompts, _ := pipeline.ListPromptVersions()
	classifierPrompt := ""
	evaluatorPrompt := ""
	for _, p := range prompts {
		if p.Name == "classifier" {
			classifierPrompt = p.Version
		}
		if p.Name == "evaluator" {
			evaluatorPrompt = p.Version
		}
	}
	fmt.Printf("  %s\n", cli.Bold("Pipeline:"))
	clsLine := fmt.Sprintf("    Classifier:  %s", cfg.ClassifierModel)
	if classifierPrompt != "" {
		clsLine += cli.Muted(fmt.Sprintf("  (prompt: %s)", classifierPrompt))
	}
	fmt.Println(clsLine)
	evalLine := fmt.Sprintf("    Evaluator:   %s", cfg.EvaluatorModel)
	if evaluatorPrompt != "" {
		evalLine += cli.Muted(fmt.Sprintf("  (prompt: %s)", evaluatorPrompt))
	}
	fmt.Println(evalLine)

	// Recent skip count from history.
	if history, err := pipeline.ReadHistory(); err == nil && len(history) > 0 {
		since := time.Now().AddDate(0, 0, -7)
		stats := pipeline.ComputeStatsFromHistory(history, since)
		if stats.SkippedBusy > 0 {
			fmt.Printf("  %s  %s %s\n", cli.Bold("Contention:"),
				cli.Warn(fmt.Sprintf("%d sessions skipped (busy slots)", stats.SkippedBusy)),
				cli.Muted("(last 7 days)"))
		}
	}

	// Circuit breaker state (only shown when not closed).
	if cbState, err := store.ReadCircuitBreakerState(); err == nil && cbState.State != "closed" {
		switch cbState.State {
		case "open":
			fmt.Printf("  %s  %s — %d consecutive errors, paused since %s\n",
				cli.Bold("Circuit Breaker:"),
				cli.Error("OPEN"),
				cbState.ConsecutiveErrors,
				cbState.LastTripAt.Local().Format("2006-01-02 15:04"))
		case "half-open":
			fmt.Printf("  %s  %s — testing recovery\n",
				cli.Bold("Circuit Breaker:"),
				cli.Warn("PROBING"))
		}
		if cbState.TotalTrips > 0 {
			fmt.Printf("  %s  %d\n", cli.Bold("Total Trips:"), cbState.TotalTrips)
		}
	}

	// Debug mode.
	if store.ReadDebugFlag() {
		fmt.Printf("  %s  %s %s\n", cli.Bold("Debug:"), cli.Warn("enabled"), cli.Muted("(via config)"))
	}
	fmt.Println()

	return nil
}

func hookStatus(installed bool) string {
	if installed {
		return cli.Success("✓")
	}
	return cli.Error("✗")
}

