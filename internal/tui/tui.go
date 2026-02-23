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
// The version parameter is displayed in the dashboard header.
func Run(version string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	components.SetFlavorEnabled(cfg.Personality.FlavorText)

	proposals, err := pipeline.ListProposals()
	if err != nil {
		return fmt.Errorf("loading proposals: %w", err)
	}

	// Load sessions once and reuse for all startup queries.
	sessions, _ := store.ListSessions()

	stats := gatherStatsFromSessions(sessions, proposals)
	stats.Version = version

	// Future: reports := fitness.ListReports()
	var reports []fitness.Report

	mergedSources, _ := store.LoadAndMergeSources()
	sourceGroups := store.GroupSources(mergedSources)

	runs, err := pipeline.ListPipelineRunsFromHistory(sessions, cfg.Pipeline.RecentRunsLimit)
	if err != nil {
		runs = nil // non-fatal
	}

	pipelineStats, err := pipeline.GatherPipelineStatsFromSessions(sessions, runs, cfg.Pipeline.SparklineDays)
	if err != nil {
		pipelineStats = pipeline.PipelineStats{}
	}

	prompts, err := pipeline.ListPromptVersions()
	if err != nil {
		prompts = nil
	}

	m := newReviewModel(proposals, reports, stats, sourceGroups, runs, pipelineStats, prompts, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

// gatherStatsFromSessions collects dashboard statistics from pre-loaded sessions.
func gatherStatsFromSessions(sessions []store.Metadata, proposals []pipeline.ProposalWithSession) message.DashboardStats {
	stats := message.DashboardStats{}

	// Proposal count from already-loaded data.
	stats.PendingCount = len(proposals)

	// Session counts for last capture time.
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

	// Daemon start time from PID file modtime.
	if alive {
		pidPath := filepath.Join(store.Root(), "daemon.pid")
		if info, err := os.Stat(pidPath); err == nil {
			t := info.ModTime()
			stats.DaemonStartTime = &t
		}
	}

	// Daemon intervals from compile-time defaults.
	defaults := daemon.DefaultConfig()
	stats.PollInterval = defaults.PollInterval
	stats.StaleInterval = defaults.StaleInterval
	stats.InterSessionDelay = defaults.InterSessionDelay

	// Store metrics.
	stats.StorePath = store.Root()
	stats.SessionCount = len(sessions)
	stats.DiskBytes = storeDiskBytes(store.Root())

	// Hook status.
	stats.HookPreCompact, stats.HookSessionEnd = checkHookStatus()

	// Debug mode.
	stats.DebugMode = store.ReadDebugFlag()

	return stats
}

// Disk bytes cache to avoid expensive directory walks on every tick.
var (
	cachedDiskBytes      int64
	cachedDiskBytesTime  time.Time
	diskBytesCacheTTL    = 60 * time.Second
)

// storeDiskBytes returns the total size of all files under the given directory.
// Results are cached for 60 seconds to avoid expensive walks on every tick.
func storeDiskBytes(root string) int64 {
	if time.Since(cachedDiskBytesTime) < diskBytesCacheTTL {
		return cachedDiskBytes
	}
	cachedDiskBytes = walkDiskBytes(root)
	cachedDiskBytesTime = time.Now()
	return cachedDiskBytes
}

func walkDiskBytes(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
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
