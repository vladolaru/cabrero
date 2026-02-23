package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/apply"
	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
	"github.com/vladolaru/cabrero/internal/tui/chat"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/dashboard"
	"github.com/vladolaru/cabrero/internal/tui/detail"
	fitness_tui "github.com/vladolaru/cabrero/internal/tui/fitness"
	"github.com/vladolaru/cabrero/internal/tui/logview"
	"github.com/vladolaru/cabrero/internal/tui/message"
	pipeline_tui "github.com/vladolaru/cabrero/internal/tui/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/shared"
	"github.com/vladolaru/cabrero/internal/tui/sources"
)

// reviewModel is the root Bubble Tea model for the review TUI.
type reviewModel struct {
	state     message.ViewState
	viewStack []message.ViewState
	config    *shared.Config

	// Persistent header stats
	stats        message.DashboardStats
	headerHeight int

	// Status bar
	statusMsg    string
	statusExpiry time.Time

	// Child models
	dashboard       dashboard.Model
	detail          detail.Model
	chat            chat.Model
	fitness         fitness_tui.Model
	sources         sources.Model
	pipelineMonitor pipeline_tui.Model
	logViewer       logview.Model

	// Source groups for re-use when pushing ViewSourceManager.
	sourceGroups []fitness.SourceGroup

	// Shared
	helpOpen bool
	keys     shared.KeyMap

	// All proposals for navigation context
	proposals []pipeline.ProposalWithSession

	// Log follow mode: track file size for incremental reads.
	logFileSize int64

	// Pipeline refresh state: track whether a refresh is in flight.
	pipelineRefreshing bool

	width  int
	height int
}

// newReviewModel creates the root model with loaded data.
func newReviewModel(proposals []pipeline.ProposalWithSession, reports []fitness.Report, stats message.DashboardStats, sourceGroups []fitness.SourceGroup, runs []pipeline.PipelineRun, pipelineStats pipeline.PipelineStats, prompts []pipeline.PromptVersion, cfg *shared.Config) reviewModel {
	keys := shared.NewKeyMap(cfg.Navigation)

	m := reviewModel{
		state:           message.ViewDashboard,
		config:          cfg,
		stats:           stats,
		keys:            keys,
		proposals:       proposals,
		sourceGroups:    sourceGroups,
		dashboard:       dashboard.New(proposals, reports, stats, &keys, cfg),
		pipelineMonitor: pipeline_tui.New(runs, pipelineStats, prompts, stats, &keys, cfg),
	}

	return m
}

// Init implements tea.Model.
func (m reviewModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m reviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	childMsg := msg // message forwarded to child views (may be height-adjusted)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Compute persistent header height.
		header := dashboard.RenderHeader(m.stats, m.width)
		m.headerHeight = strings.Count(header, "\n") + 2 // +1 trailing newline, +1 separator line
		childMsg = tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - m.headerHeight}

	case tea.KeyMsg:
		// Global keys handled first.
		if m2, cmd, handled := m.handleGlobalKey(msg); handled {
			return m2, cmd
		}

	case message.PushView:
		return m.pushView(msg.View, msg.Action)

	case message.PopView:
		return m.popView()

	case message.StatusMessage:
		m.statusMsg = msg.Text
		if msg.Duration > 0 {
			m.statusExpiry = time.Now().Add(msg.Duration)
			cmds = append(cmds, tea.Tick(msg.Duration, func(time.Time) tea.Msg {
				return message.StatusMessageExpired{}
			}))
		}
		return m, tea.Batch(cmds...)

	case message.StatusMessageExpired:
		if !m.statusExpiry.IsZero() && time.Now().After(m.statusExpiry) {
			m.statusMsg = ""
		}
		return m, nil

	case message.ChatPanelToggled:
		m.resizeDetailChat()
		return m, nil

	case message.ApproveStarted:
		// Start blending in a background goroutine.
		p := m.detail.Proposal()
		if p != nil {
			proposalID := msg.ProposalID
			proposal := &p.Proposal
			sessionID := p.SessionID
			return m, func() tea.Msg {
				diff, err := apply.Blend(proposal, sessionID)
				return message.BlendFinished{
					ProposalID:      proposalID,
					BeforeAfterDiff: diff,
					Err:             err,
				}
			}
		}
		return m, nil

	case message.ApplyConfirmed:
		// Apply blended content and archive the proposal.
		p := m.detail.Proposal()
		if p != nil {
			proposalID := msg.ProposalID
			proposal := &p.Proposal
			blended := m.detail.BlendResult()
			return m, func() tea.Msg {
				if blended != "" {
					if err := apply.Commit(proposal, blended); err != nil {
						return message.ApplyFinished{ProposalID: proposalID, Err: err}
					}
				}
				if err := apply.Archive(proposalID, "approved"); err != nil {
					return message.ApplyFinished{ProposalID: proposalID, Err: err}
				}
				return message.ApplyFinished{ProposalID: proposalID}
			}
		}
		return m, nil

	case message.RejectFinished:
		// Archive the proposal and return to dashboard.
		proposalID := msg.ProposalID
		m.statusMsg = actionStatusText(msg)
		m.statusExpiry = time.Now().Add(3 * time.Second)
		if m.state != message.ViewDashboard {
			m2, cmd := m.popView()
			cmds = append(cmds, cmd)
			m = m2.(reviewModel)
		}
		cmds = append(cmds, func() tea.Msg {
			if err := apply.Archive(proposalID, "rejected"); err != nil {
				return message.StatusMessage{Text: "Archive failed: " + err.Error(), Duration: 3 * time.Second}
			}
			return nil
		})
		cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return message.StatusMessageExpired{}
		}))
		return m, tea.Batch(cmds...)

	case message.DeferFinished:
		// Archive the proposal and return to dashboard.
		proposalID := msg.ProposalID
		m.statusMsg = actionStatusText(msg)
		m.statusExpiry = time.Now().Add(3 * time.Second)
		if m.state != message.ViewDashboard {
			m2, cmd := m.popView()
			cmds = append(cmds, cmd)
			m = m2.(reviewModel)
		}
		cmds = append(cmds, func() tea.Msg {
			if err := apply.Archive(proposalID, "deferred"); err != nil {
				return message.StatusMessage{Text: "Archive failed: " + err.Error(), Duration: 3 * time.Second}
			}
			return nil
		})
		cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return message.StatusMessageExpired{}
		}))
		return m, tea.Batch(cmds...)

	case message.ApplyFinished:
		if msg.Err != nil {
			m.statusMsg = "Apply failed: " + msg.Err.Error()
		} else {
			m.statusMsg = components.ConfirmApprove()
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		if m.state != message.ViewDashboard {
			m2, cmd := m.popView()
			cmds = append(cmds, cmd)
			m = m2.(reviewModel)
		}
		cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return message.StatusMessageExpired{}
		}))
		return m, tea.Batch(cmds...)

	case message.DismissFinished:
		if msg.Err != nil {
			m.statusMsg = "Dismiss failed: " + msg.Err.Error()
		} else {
			m.statusMsg = "Report dismissed."
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		if m.state != message.ViewDashboard {
			m2, cmd := m.popView()
			cmds = append(cmds, cmd)
			m = m2.(reviewModel)
		}
		cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return message.StatusMessageExpired{}
		}))
		return m, tea.Batch(cmds...)

	case message.JumpToSources:
		// Push the source manager view with a pre-selected source.
		m.viewStack = append(m.viewStack, m.state)
		m.state = message.ViewSourceManager
		m.sources = sources.New(m.sourceGroups, &m.keys, m.config)
		m.sources.SetSize(m.width, m.childHeight())
		if msg.SourceName != "" {
			m.sources = m.sources.PreSelectSource(msg.SourceName)
		}
		return m, nil

	case message.RollbackFinished:
		if msg.Err != nil {
			m.statusMsg = "Rollback failed: " + msg.Err.Error()
		} else {
			m.statusMsg = "Rollback complete."
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return message.StatusMessageExpired{}
		}))
		return m, tea.Batch(cmds...)

	case message.PipelineTickMsg:
		if m.state == message.ViewPipelineMonitor && !m.pipelineRefreshing {
			m.pipelineRefreshing = true
			recentRunsLimit := m.config.Pipeline.RecentRunsLimit
			sparklineDays := m.config.Pipeline.SparklineDays
			proposals := m.proposals
			return m, func() tea.Msg {
				sessions, _ := store.ListSessions()
				runs, _ := pipeline.ListPipelineRunsFromHistory(sessions, recentRunsLimit)
				stats, _ := pipeline.GatherPipelineStatsFromSessions(sessions, runs, sparklineDays)
				prompts, _ := pipeline.ListPromptVersions()
				dashStats := gatherStatsFromSessions(sessions, proposals)
				return message.PipelineDataRefreshed{
					Runs:      runs,
					Stats:     stats,
					Prompts:   prompts,
					DashStats: dashStats,
				}
			}
		}
		return m, nil

	case message.PipelineDataRefreshed:
		m.pipelineRefreshing = false
		statusCmd := m.pipelineMonitor.Refresh(msg.Runs, msg.Stats, msg.Prompts, msg.DashStats)
		nextTick := tea.Tick(5*time.Second, func(time.Time) tea.Msg {
			return message.PipelineTickMsg{}
		})
		return m, tea.Batch(statusCmd, nextTick)

	case message.LogTickMsg:
		if m.state == message.ViewLogViewer {
			logPath := filepath.Join(store.Root(), "daemon.log")
			info, err := os.Stat(logPath)
			if err == nil {
				newSize := info.Size()
				if newSize > m.logFileSize {
					// Read only new bytes from the end.
					f, err := os.Open(logPath)
					if err == nil {
						buf := make([]byte, newSize-m.logFileSize)
						n, _ := f.ReadAt(buf, m.logFileSize)
						f.Close()
						if n > 0 {
							m.logViewer.AppendContent(string(buf[:n]))
							m.logFileSize += int64(n)
						}
					}
				} else if newSize < m.logFileSize {
					// File was truncated (log rotation) — full reload.
					content, _ := os.ReadFile(logPath)
					m.logViewer.UpdateContent(string(content))
					m.logFileSize = newSize
				}
			}
			return m, tea.Tick(time.Second, func(time.Time) tea.Msg {
				return message.LogTickMsg{}
			})
		}
		return m, nil

	case message.RetryRunStarted:
		sessionID := msg.SessionID
		return m, func() tea.Msg {
			// Placeholder — actual retry via exec.Command in future.
			return message.RetryRunFinished{SessionID: sessionID}
		}

	case message.RetryRunFinished:
		if msg.Err != nil {
			m.statusMsg = "Retry failed: " + msg.Err.Error()
		} else {
			m.statusMsg = "Retry complete."
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return message.StatusMessageExpired{}
		}))
		return m, tea.Batch(cmds...)
	}

	// Route to active child (using childMsg which has reduced height for WindowSizeMsg).
	switch m.state {
	case message.ViewDashboard:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(childMsg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case message.ViewProposalDetail:
		// Handle resize with layout-aware dimensions (don't forward raw WindowSizeMsg).
		if _, isResize := childMsg.(tea.WindowSizeMsg); isResize {
			m.resizeDetailChat()
		} else if _, isToggle := childMsg.(message.ChatPanelToggled); isToggle {
			m.resizeDetailChat()
		} else {
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(childMsg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if m.config.Detail.ChatPanelOpen {
				var chatCmd tea.Cmd
				m.chat, chatCmd = m.chat.Update(childMsg)
				if chatCmd != nil {
					cmds = append(cmds, chatCmd)
				}
			}
		}
	case message.ViewFitnessDetail:
		var cmd tea.Cmd
		m.fitness, cmd = m.fitness.Update(childMsg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case message.ViewSourceManager, message.ViewSourceDetail:
		var cmd tea.Cmd
		m.sources, cmd = m.sources.Update(childMsg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case message.ViewPipelineMonitor:
		var cmd tea.Cmd
		m.pipelineMonitor, cmd = m.pipelineMonitor.Update(childMsg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case message.ViewLogViewer:
		var cmd tea.Cmd
		m.logViewer, cmd = m.logViewer.Update(childMsg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m reviewModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Persistent header + separator.
	header := dashboard.RenderHeader(m.stats, m.width)
	separator := strings.Repeat("─", m.width)

	var content string

	switch m.state {
	case message.ViewDashboard:
		content = m.dashboard.View()
	case message.ViewProposalDetail:
		if m.config.Detail.ChatPanelOpen {
			detailView := m.detail.View()
			chatView := m.chat.View()
			if m.width >= 120 {
				content = lipgloss.JoinHorizontal(lipgloss.Top, detailView, chatView)
			} else {
				content = lipgloss.JoinVertical(lipgloss.Left, detailView, chatView)
			}
		} else {
			content = m.detail.View()
		}
	case message.ViewFitnessDetail:
		content = m.fitness.View()
	case message.ViewSourceManager, message.ViewSourceDetail:
		content = m.sources.View()
	case message.ViewPipelineMonitor:
		content = m.pipelineMonitor.View()
	case message.ViewLogViewer:
		content = m.logViewer.View()
	}

	// Help overlay.
	if m.helpOpen {
		viewState := m.state
		if m.state == message.ViewSourceManager && m.sources.DetailOpen() {
			viewState = message.ViewSourceDetail
		}
		sections := shared.HelpForView(viewState, m.keys)
		content = components.RenderHelpOverlay(sections, m.width, m.height)
	}

	return header + "\n" + separator + "\n" + content
}

func (m reviewModel) handleGlobalKey(msg tea.KeyMsg) (reviewModel, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.ForceQuit):
		return m, tea.Quit, true

	case key.Matches(msg, m.keys.Quit):
		// Only quit from dashboard.
		if m.state == message.ViewDashboard {
			return m, tea.Quit, true
		}
		return m, nil, false

	case key.Matches(msg, m.keys.Help):
		m.helpOpen = !m.helpOpen
		return m, nil, true

	case key.Matches(msg, m.keys.Back):
		if m.helpOpen {
			m.helpOpen = false
			return m, nil, true
		}
		// Let child views handle Esc when they have active prompts or searches.
		if m.state == message.ViewLogViewer && m.logViewer.HasActiveSearch() {
			return m, nil, false
		}
		if m.state == message.ViewSourceManager && m.sources.HasActivePrompt() {
			return m, nil, false
		}
		if m.state == message.ViewProposalDetail && m.detail.HasActivePrompt() {
			return m, nil, false
		}
		if m.state == message.ViewPipelineMonitor && m.pipelineMonitor.HasActivePrompt() {
			return m, nil, false
		}
		if m.state != message.ViewDashboard {
			return m, func() tea.Msg { return message.PopView{} }, true
		}
		return m, nil, false
	}

	return m, nil, false
}

func (m reviewModel) pushView(view message.ViewState, action string) (tea.Model, tea.Cmd) {
	m.viewStack = append(m.viewStack, m.state)
	m.state = view

	var cmds []tea.Cmd

	switch view {
	case message.ViewProposalDetail:
		// Initialize detail from selected proposal.
		p := m.dashboard.SelectedProposal()
		if p != nil {
			citations := buildCitations(p)
			m.detail = detail.New(p, citations, &m.keys, m.config)

			// Initialize chat for this proposal (dimensions set by resizeDetailChat).
			chips := defaultChips()
			m.chat = chat.New(chips, "", m.width, m.childHeight())
			m.resizeDetailChat()

			// Trigger follow-up action if specified.
			switch action {
			case "approve":
				var cmd tea.Cmd
				m.detail, cmd = m.detail.TriggerApprove()
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			case "reject":
				var cmd tea.Cmd
				m.detail, cmd = m.detail.TriggerReject()
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			case "defer":
				var cmd tea.Cmd
				m.detail, cmd = m.detail.TriggerDefer()
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

	case message.ViewFitnessDetail:
		// Initialize fitness detail from the dashboard's selected report.
		report := m.dashboard.SelectedFitnessReport()
		m.fitness = fitness_tui.New(report, &m.keys, m.config)
		m.fitness.SetSize(m.width, m.childHeight())

	case message.ViewSourceManager:
		// Initialize source manager with current source groups.
		m.sources = sources.New(m.sourceGroups, &m.keys, m.config)
		m.sources.SetSize(m.width, m.childHeight())
		if action != "" {
			m.sources = m.sources.PreSelectSource(action)
		}

	case message.ViewPipelineMonitor:
		m.pipelineMonitor.SetSize(m.width, m.childHeight())
		cmds = append(cmds, tea.Tick(5*time.Second, func(time.Time) tea.Msg {
			return message.PipelineTickMsg{}
		}))

	case message.ViewLogViewer:
		logPath := filepath.Join(store.Root(), "daemon.log")
		content, _ := os.ReadFile(logPath)
		m.logFileSize = int64(len(content))
		m.logViewer = logview.New(string(content), &m.keys, m.config)
		m.logViewer.SetSize(m.width, m.childHeight())
		// Always poll for new log content while the viewer is open.
		cmds = append(cmds, tea.Tick(time.Second, func(time.Time) tea.Msg {
			return message.LogTickMsg{}
		}))
	}

	return m, tea.Batch(cmds...)
}

// childHeight returns the height available for child views (total minus persistent header).
func (m reviewModel) childHeight() int {
	return m.height - m.headerHeight
}

// resizeDetailChat sets layout-aware dimensions on detail and chat models.
// Wide (>= 120): horizontal split using ChatPanelWidth percentage for chat width.
// Narrow (< 120): vertical split using ChatPanelWidth percentage for chat height.
func (m *reviewModel) resizeDetailChat() {
	ch := m.childHeight()
	if !m.config.Detail.ChatPanelOpen {
		m.detail.SetSize(m.width, ch)
		return
	}

	chatPct := m.config.Detail.ChatPanelWidth
	if chatPct <= 0 {
		chatPct = 35
	}

	if m.width >= 120 {
		// Horizontal split: detail gets full height, chat gets percentage of width.
		cw := m.width * chatPct / 100
		m.detail.SetSize(m.width, ch)
		m.chat.SetSize(cw, ch)
	} else {
		// Vertical split: both get full width, height is split.
		chatH := ch * chatPct / 100
		if chatH < 6 {
			chatH = 6
		}
		detailH := ch - chatH
		m.detail.SetSize(m.width, detailH)
		m.chat.SetSize(m.width-2, chatH)
	}
}

func (m reviewModel) popView() (tea.Model, tea.Cmd) {
	if len(m.viewStack) == 0 {
		return m, nil
	}

	m.state = m.viewStack[len(m.viewStack)-1]
	m.viewStack = m.viewStack[:len(m.viewStack)-1]
	return m, nil
}

func buildCitations(p *pipeline.ProposalWithSession) []shared.CitationEntry {
	var citations []shared.CitationEntry
	for i, uuid := range p.Proposal.CitedUUIDs {
		citations = append(citations, shared.CitationEntry{
			UUID:    uuid,
			Summary: fmt.Sprintf("[%d] %s", i+1, uuid),
		})
	}
	return citations
}

func defaultChips() []string {
	return []string{
		"Why was this flagged?",
		"Show the raw turns",
		"Conservative version",
		"Risk of approving?",
	}
}

func actionStatusText(msg tea.Msg) string {
	switch msg.(type) {
	case message.RejectFinished:
		return components.ConfirmReject()
	case message.DeferFinished:
		return components.ConfirmDefer()
	default:
		return ""
	}
}
