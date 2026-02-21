// Command snapshot renders TUI views to stdout as ANSI output.
// Pipe the output through freeze to generate PNG/SVG snapshots.
//
// Usage:
//
//	go run ./cmd/snapshot dashboard
//	go run ./cmd/snapshot dashboard -w 80 -h 24
//	go run ./cmd/snapshot dashboard | freeze --config freeze.json --language ansi -o snapshots/dashboard.png
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/chat"
	"github.com/vladolaru/cabrero/internal/tui/dashboard"
	"github.com/vladolaru/cabrero/internal/tui/detail"
	fitness_tui "github.com/vladolaru/cabrero/internal/tui/fitness"
	"github.com/vladolaru/cabrero/internal/tui/message"
	pipeline_tui "github.com/vladolaru/cabrero/internal/tui/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/shared"
	"github.com/vladolaru/cabrero/internal/tui/sources"
	"github.com/vladolaru/cabrero/internal/tui/testdata"

	"github.com/charmbracelet/lipgloss"
)

var views = []string{
	"dashboard",
	"dashboard-narrow",
	"dashboard-empty",
	"proposal-detail",
	"proposal-detail-chat",
	"fitness-report",
	"source-manager",
	"pipeline-monitor",
	"help-overlay",
	"help-overlay-vim",
}

func main() {
	width := flag.Int("w", 0, "terminal width (0 = view default)")
	height := flag.Int("h", 0, "terminal height (0 = view default)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: snapshot [flags] <view>\n\nAvailable views:\n")
		for _, v := range views {
			fmt.Fprintf(os.Stderr, "  %s\n", v)
		}
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	view := flag.Arg(0)
	output, err := render(view, *width, *height)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Trailing newline required — freeze drops the last line without it.
	fmt.Println(output)
}

func render(view string, w, h int) (string, error) {
	switch view {
	case "dashboard":
		return renderDashboard(w, h, false)
	case "dashboard-narrow":
		if w == 0 {
			w = 80
		}
		if h == 0 {
			h = 24
		}
		return renderDashboard(w, h, false)
	case "dashboard-empty":
		return renderDashboard(w, h, true)
	case "proposal-detail":
		return renderProposalDetail(w, h)
	case "proposal-detail-chat":
		return renderProposalDetailChat(w, h)
	case "fitness-report":
		return renderFitnessReport(w, h)
	case "source-manager":
		return renderSourceManager(w, h)
	case "pipeline-monitor":
		return renderPipelineMonitor(w, h)
	case "help-overlay":
		return renderHelpOverlay(w, h, "arrows")
	case "help-overlay-vim":
		return renderHelpOverlay(w, h, "vim")
	default:
		return "", fmt.Errorf("unknown view %q, available: %s", view, strings.Join(views, ", "))
	}
}

func defaults(w, h int) (int, int) {
	if w == 0 {
		w = 120
	}
	if h == 0 {
		h = 40
	}
	return w, h
}

func renderDashboard(w, h int, empty bool) (string, error) {
	w, h = defaults(w, h)
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)

	var proposals []pipeline.ProposalWithSession
	var reports []fitness.Report
	var stats message.DashboardStats

	if empty {
		stats = testdata.TestDashboardStatsEmpty()
	} else {
		proposals = testdata.TestProposals()
		reports = testdata.TestFitnessReports()
		stats = testdata.TestDashboardStats()
	}

	m := dashboard.New(proposals, reports, stats, &keys, cfg)
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m.View(), nil
}

func renderProposalDetail(w, h int) (string, error) {
	w, h = defaults(w, h)
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	p := testdata.TestProposal()
	citations := testdata.TestCitations()

	m := detail.New(&p, citations, &keys, cfg)
	m.SetSize(w, h)
	return m.View(), nil
}

func renderProposalDetailChat(w, h int) (string, error) {
	if w == 0 {
		w = 160
	}
	if h == 0 {
		h = 40
	}
	cfg := testdata.TestConfig()
	cfg.Detail.ChatPanelOpen = true
	keys := shared.NewKeyMap(cfg.Navigation)
	p := testdata.TestProposal()
	citations := testdata.TestCitations()

	m := detail.New(&p, citations, &keys, cfg)
	m.SetSize(w, h)

	chatPct := cfg.Detail.ChatPanelWidth
	if chatPct <= 0 {
		chatPct = 35
	}
	chatW := w * chatPct / 100
	c := chat.New(
		[]string{"Why was this flagged?", "Show the raw turns", "Conservative version", "Risk of approving?"},
		"", chatW, h-6,
	)

	detailView := m.View()
	chatView := c.View()
	return lipgloss.JoinHorizontal(lipgloss.Top, detailView, chatView), nil
}

func renderFitnessReport(w, h int) (string, error) {
	w, h = defaults(w, h)
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	report := testdata.TestFitnessReport()

	m := fitness_tui.New(report, &keys, cfg)
	m.SetSize(w, h)
	return m.View(), nil
}

func renderSourceManager(w, h int) (string, error) {
	w, h = defaults(w, h)
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	groups := testdata.TestSourceGroups()

	m := sources.New(groups, &keys, cfg)
	m.SetSize(w, h)
	return m.View(), nil
}

func renderPipelineMonitor(w, h int) (string, error) {
	w, h = defaults(w, h)
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	runs := testdata.TestPipelineRuns()
	stats := testdata.TestPipelineStats()
	prompts := testdata.TestPromptVersions()
	dashStats := testdata.TestDashboardStats()

	m := pipeline_tui.New(runs, stats, prompts, dashStats, &keys, cfg)
	m.SetSize(w, h)
	return m.View(), nil
}

func renderHelpOverlay(w, h int, nav string) (string, error) {
	w, h = defaults(w, h)
	keys := shared.NewKeyMap(nav)
	hm := help.New()
	hm.Width = w
	hm.ShowAll = true
	return hm.View(keys), nil
}
