package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/daemon"
	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Run launches the interactive review TUI.
func Run() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	components.SetFlavorEnabled(cfg.Personality.FlavorText)

	proposals, err := pipeline.ListProposals()
	if err != nil {
		return fmt.Errorf("loading proposals: %w", err)
	}

	stats := gatherStats()

	// Future: sourceGroups := fitness.ListSourceGroups(sources)
	sourceGroups := []fitness.SourceGroup{}

	m := newReviewModel(proposals, stats, sourceGroups, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

// gatherStats collects dashboard statistics from the store and daemon.
func gatherStats() message.DashboardStats {
	stats := message.DashboardStats{}

	// Proposal counts.
	proposals, _ := pipeline.ListProposals()
	stats.PendingCount = len(proposals)

	// Session counts for last capture time.
	sessions, _ := store.ListSessions()
	if len(sessions) > 0 {
		ts, err := time.Parse(time.RFC3339, sessions[0].Timestamp)
		if err == nil {
			stats.LastCaptureTime = &ts
		}
	}

	// Daemon status.
	pid, alive := daemon.IsDaemonRunning()
	stats.DaemonRunning = alive
	stats.DaemonPID = pid

	// Hook status.
	stats.HookPreCompact, stats.HookSessionEnd = checkHookStatus()

	return stats
}

// checkHookStatus reads Claude Code settings to determine hook installation status.
func checkHookStatus() (preCompact, sessionEnd bool) {
	// Reuse the same logic from cmd/status.go.
	// For now, a simplified check.
	home, err := os.UserHomeDir()
	if err != nil {
		return false, false
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, false
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, false
	}

	hooksRaw, ok := settings["hooks"]
	if !ok {
		return false, false
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		return false, false
	}

	preCompact = containsCabrero(hooks["PreCompact"])
	sessionEnd = containsCabrero(hooks["SessionEnd"])
	return
}

func containsCabrero(raw json.RawMessage) bool {
	if raw == nil {
		return false
	}
	return strings.Contains(string(raw), "cabrero")
}
