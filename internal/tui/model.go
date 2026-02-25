package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bytes"
	"encoding/json"

	"github.com/vladolaru/cabrero/internal/apply"
	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/retrieval"
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

// appModel is the root Bubble Tea model for the TUI.
type appModel struct {
	state     message.ViewState
	viewStack []message.ViewState
	config    *shared.Config

	// Persistent header stats
	stats        message.DashboardStats
	headerHeight int

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

// newAppModel creates the root model with loaded data.
func newAppModel(proposals []pipeline.ProposalWithSession, reports []fitness.Report, stats message.DashboardStats, sourceGroups []fitness.SourceGroup, runs []pipeline.PipelineRun, pipelineStats pipeline.PipelineStats, prompts []pipeline.PromptVersion, cfg *shared.Config) appModel {
	keys := shared.NewKeyMap(cfg.Navigation)

	m := appModel{
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
func (m appModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	childMsg := msg // message forwarded to child views (may be height-adjusted)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Compute persistent header height.
		header := dashboard.RenderHeader(m.stats, m.width)
		m.headerHeight = strings.Count(header, "\n") + 2 // +1 trailing newline, +1 separator line
		subHeaderHeight := 3                              // title + stats + separator
		childMsg = tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - m.headerHeight - subHeaderHeight}

	case tea.KeyMsg:
		// Global keys handled first.
		if m2, cmd, handled := m.handleGlobalKey(msg); handled {
			return m2, cmd
		}

	case message.PushView:
		return m.pushView(msg.View, msg.Action)

	case message.PopView:
		return m.popView()

	case message.SwitchView:
		return m.switchView(msg.View)

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
		if m.state != message.ViewDashboard {
			m2, cmd := m.popView()
			cmds = append(cmds, cmd)
			m = m2.(appModel)
		}
		cmds = append(cmds, func() tea.Msg {
			return message.StatusMessage{Text: actionStatusText(msg), Duration: 3 * time.Second}
		})
		cmds = append(cmds, func() tea.Msg {
			if err := apply.Archive(proposalID, "rejected"); err != nil {
				return message.StatusMessage{Text: "Archive failed: " + err.Error(), Duration: 3 * time.Second}
			}
			return nil
		})
		return m, tea.Batch(cmds...)

	case message.DeferFinished:
		// Archive the proposal and return to dashboard.
		proposalID := msg.ProposalID
		if m.state != message.ViewDashboard {
			m2, cmd := m.popView()
			cmds = append(cmds, cmd)
			m = m2.(appModel)
		}
		cmds = append(cmds, func() tea.Msg {
			return message.StatusMessage{Text: actionStatusText(msg), Duration: 3 * time.Second}
		})
		cmds = append(cmds, func() tea.Msg {
			if err := apply.Archive(proposalID, "deferred"); err != nil {
				return message.StatusMessage{Text: "Archive failed: " + err.Error(), Duration: 3 * time.Second}
			}
			return nil
		})
		return m, tea.Batch(cmds...)

	case message.ApplyFinished:
		var statusText string
		if msg.Err != nil {
			statusText = "Apply failed: " + msg.Err.Error()
		} else {
			statusText = components.ConfirmApprove()
		}
		if m.state != message.ViewDashboard {
			m2, cmd := m.popView()
			cmds = append(cmds, cmd)
			m = m2.(appModel)
		}
		cmds = append(cmds, func() tea.Msg {
			return message.StatusMessage{Text: statusText, Duration: 3 * time.Second}
		})
		return m, tea.Batch(cmds...)

	case message.DismissFinished:
		var statusText string
		if msg.Err != nil {
			statusText = "Dismiss failed: " + msg.Err.Error()
		} else {
			statusText = "Report dismissed."
		}
		if m.state != message.ViewDashboard {
			m2, cmd := m.popView()
			cmds = append(cmds, cmd)
			m = m2.(appModel)
		}
		cmds = append(cmds, func() tea.Msg {
			return message.StatusMessage{Text: statusText, Duration: 3 * time.Second}
		})
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
		var statusText string
		if msg.Err != nil {
			statusText = "Retry failed: " + msg.Err.Error()
		} else {
			statusText = "Retry complete."
		}
		cmds = append(cmds, func() tea.Msg {
			return message.StatusMessage{Text: statusText, Duration: 3 * time.Second}
		})
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
			m.resizeDetailChat() // calls syncInlineChat() for narrow mode
		} else if keyMsg, isKey := childMsg.(tea.KeyMsg); isKey {
			cmds = append(cmds, m.routeDetailKey(keyMsg)...)
			m.syncInlineChat()
		} else {
			// Non-key messages (spinner ticks, etc.) go to both models.
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
				m.syncInlineChat()
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
func (m appModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Persistent header + separator.
	header := dashboard.RenderHeader(m.stats, m.width)
	separator := strings.Repeat("─", m.width)

	// Sub-header (view title + stats).
	subHeader := m.subHeader()

	var content string

	switch m.state {
	case message.ViewDashboard:
		content = m.dashboard.View()
	case message.ViewProposalDetail:
		if m.config.Detail.ChatPanelOpen && m.width >= 160 {
			// Wide: horizontal split — detail | separator | chat, status bar below.
			detailView := m.detail.View()
			sep := m.renderVerticalSeparator(m.childHeight() - 1)
			chatView := m.chat.View()
			content = lipgloss.JoinHorizontal(lipgloss.Top, detailView, sep, chatView)
			content += "\n" + components.RenderStatusBar(m.keys.DetailShortHelp(), "", m.width)
		} else if m.config.Detail.ChatPanelOpen {
			// Narrow: vertical split — detail on top, separator, chat below, shared status bar.
			sep := shared.MutedStyle.Render(strings.Repeat("─", m.width))
			chatView := shared.IndentBlock(m.chat.View(), 2)
			content = m.detail.View() + sep + "\n" + chatView
			content += "\n" + components.RenderStatusBar(m.keys.DetailShortHelp(), "", m.width)
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
		hc := shared.HelpForView(viewState, m.keys)
		content = components.RenderHelpOverlay(hc, m.width, m.height)
	}

	return header + "\n" + separator + "\n" + subHeader + "\n" + separator + "\n" + content
}

func (m appModel) handleGlobalKey(msg tea.KeyMsg) (appModel, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.ForceQuit):
		return m, tea.Quit, true

	case key.Matches(msg, m.keys.Quit):
		// Don't quit when a text input is active (filter, search, chat).
		if m.hasActiveTextInput() {
			return m, nil, false
		}
		return m, tea.Quit, true

	case key.Matches(msg, m.keys.Help):
		if m.hasActiveTextInput() {
			return m, nil, false
		}
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
		// Let routeDetailKey handle Esc when chat panel has focus (switch focus or blur input).
		if m.state == message.ViewProposalDetail && m.config.Detail.ChatPanelOpen &&
			m.detail.CurrentFocus() == detail.FocusChat {
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

// hasActiveTextInput returns true when any child view has a focused text input
// (filter, search, chat), meaning 'q' should be typed rather than quit.
func (m appModel) hasActiveTextInput() bool {
	switch m.state {
	case message.ViewDashboard:
		return m.dashboard.HasActiveInput()
	case message.ViewProposalDetail:
		return m.config.Detail.ChatPanelOpen && m.chat.IsInputFocused()
	case message.ViewLogViewer:
		return m.logViewer.IsSearchInputActive()
	}
	return false
}

func (m appModel) pushView(view message.ViewState, action string) (tea.Model, tea.Cmd) {
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
			chatCfg := buildChatConfig(p, m.config.Debug)
			m.chat = chat.New(chips, chatCfg, m.width, m.childHeight())
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

// subHeader returns the sub-header for the currently active view.
func (m appModel) subHeader() string {
	switch m.state {
	case message.ViewDashboard:
		return m.dashboard.SubHeader()
	case message.ViewProposalDetail:
		return m.detail.SubHeader()
	case message.ViewFitnessDetail:
		return m.fitness.SubHeader()
	case message.ViewSourceManager, message.ViewSourceDetail:
		return m.sources.SubHeader()
	case message.ViewPipelineMonitor:
		return m.pipelineMonitor.SubHeader()
	case message.ViewLogViewer:
		return m.logViewer.SubHeader()
	default:
		return ""
	}
}

// renderVerticalSeparator returns a padded column of │ characters for the given height.
func (m appModel) renderVerticalSeparator(height int) string {
	sepStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = " " + sepStyle.Render("│") + " "
	}
	return strings.Join(lines, "\n")
}

// syncInlineChat clears any stale inline chat content from the detail viewport.
// Narrow mode now uses a vertical split instead of inline embedding.
func (m *appModel) syncInlineChat() {
	m.detail.ClearInlineChat()
}

// childHeight returns the height available for child views (total minus persistent header and sub-header).
func (m appModel) childHeight() int {
	subHeaderHeight := 3 // title + stats + separator
	return m.height - m.headerHeight - subHeaderHeight
}

// resizeDetailChat sets layout-aware dimensions on detail and chat models.
// Wide (>= 160): horizontal 50/50 split.
// Narrow (< 160): chat is inline within the detail's scrollable viewport.
func (m *appModel) resizeDetailChat() {
	ch := m.childHeight()
	if !m.config.Detail.ChatPanelOpen {
		m.detail.HideStatusBar = false
		m.detail.SetSize(m.width, ch)
		return
	}

	chatPct := m.config.Detail.ChatPanelWidth
	if chatPct <= 0 {
		chatPct = 35
	}

	if m.width >= 160 {
		// Horizontal split: detail and chat each get 50% width, status bar rendered by root.
		cw := m.width * 50 / 100
		dw := m.width - cw - 3 // -3 for padded vertical separator ( │ )
		panelH := ch - 1        // -1 for root-rendered status bar
		m.detail.HideStatusBar = true
		m.detail.SetSize(dw, panelH)
		m.chat.SetSize(cw, panelH)
	} else {
		// Narrow mode: vertical split — detail on top, chat panel below, shared status bar.
		panelH := ch - 1 // -1 for root-rendered status bar
		chatH := panelH * chatPct / 100
		if chatH < 8 {
			chatH = 8
		}
		detailH := panelH - chatH - 1 // -1 for horizontal separator
		m.detail.HideStatusBar = true
		m.detail.SetSize(m.width, detailH)
		m.chat.SetSize(m.width-2, chatH) // -2 for left indent
		m.detail.ClearInlineChat()
	}
}

// routeDetailKey routes a key event to the correct child model based on focus.
func (m *appModel) routeDetailKey(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	// Tab always goes to detail for focus toggling.
	if key.Matches(msg, m.keys.TabForward) || key.Matches(msg, m.keys.TabBackward) {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.chat.SetFocused(m.detail.CurrentFocus() == detail.FocusChat)
		return cmds
	}

	chatFocused := m.detail.CurrentFocus() == detail.FocusChat && m.config.Detail.ChatPanelOpen

	if chatFocused {
		if m.chat.IsInputFocused() {
			// Chat input active — all keys to chat (typing, Esc to blur, Enter to send).
			var cmd tea.Cmd
			m.chat, cmd = m.chat.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else {
			// Chat panel focused but not typing — global actions go to detail.
			switch {
			case key.Matches(msg, m.keys.Back):
				// Esc switches focus back to proposal.
				m.detail.SetFocus(detail.FocusProposal)
				m.chat.SetFocused(false)
			case key.Matches(msg, m.keys.Approve),
				key.Matches(msg, m.keys.Reject),
				key.Matches(msg, m.keys.Defer),
				key.Matches(msg, m.keys.Chat):
				var cmd tea.Cmd
				m.detail, cmd = m.detail.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			default:
				// Everything else (Enter, chips 1-4, viewport scroll) to chat.
				var cmd tea.Cmd
				m.chat, cmd = m.chat.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
	} else {
		// Proposal panel has focus — all keys to detail.
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return cmds
}

func (m appModel) popView() (tea.Model, tea.Cmd) {
	if len(m.viewStack) == 0 {
		return m, nil
	}

	m.state = m.viewStack[len(m.viewStack)-1]
	m.viewStack = m.viewStack[:len(m.viewStack)-1]
	return m, nil
}

// switchView replaces the current top-level view without touching the navigation stack.
// This keeps esc always returning to Proposals regardless of cross-navigation jumps.
func (m appModel) switchView(view message.ViewState) (tea.Model, tea.Cmd) {
	m.state = view
	switch view {
	case message.ViewSourceManager:
		m.sources = sources.New(m.sourceGroups, &m.keys, m.config)
		m.sources.SetSize(m.width, m.childHeight())
		return m, nil
	case message.ViewPipelineMonitor:
		m.pipelineMonitor.SetSize(m.width, m.childHeight())
		return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg {
			return message.PipelineTickMsg{}
		})
	case message.ViewDashboard:
		return m, nil
	}
	return m, nil
}

func buildCitations(p *pipeline.ProposalWithSession) []shared.CitationEntry {
	uuids := p.Proposal.CitedUUIDs
	if len(uuids) == 0 {
		return nil
	}

	// Fetch raw turns in one pass.
	rawTurns, _ := retrieval.GetTurns(p.SessionID, uuids)

	var citations []shared.CitationEntry
	for i, uuid := range uuids {
		entry := shared.CitationEntry{
			UUID:    uuid,
			Summary: fmt.Sprintf("[%d] %s", i+1, uuid),
		}
		if rawTurns != nil && i < len(rawTurns) && rawTurns[i] != nil {
			entry.RawJSON = formatTurnJSON(rawTurns[i])
		}
		citations = append(citations, entry)
	}
	return citations
}

// formatTurnJSON pretty-prints a raw JSONL turn for the citation detail view.
func formatTurnJSON(raw []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

// buildChatConfig creates a ChatConfig for a proposal's AI chat session.
// Generates a UUID, blocklists it, and builds the system prompt and tool access.
func buildChatConfig(p *pipeline.ProposalWithSession, debug bool) chat.ChatConfig {
	sessionID, err := pipeline.GenerateUUID()
	if err != nil {
		// Fallback: chat will work but won't have a persistent session.
		return chat.ChatConfig{Debug: debug}
	}

	// Blocklist immediately so the pipeline never processes this session.
	_ = store.BlockSession(sessionID)

	return chat.ChatConfig{
		SessionID:    sessionID,
		SystemPrompt: buildChatSystemPrompt(p),
		AllowedTools: buildChatAllowedTools(p),
		Debug:        debug,
	}
}

func buildChatSystemPrompt(p *pipeline.ProposalWithSession) string {
	var b strings.Builder

	b.WriteString("You are an AI assistant helping a human review a Cabrero improvement proposal. ")
	b.WriteString("Cabrero observes Claude Code sessions and proposes improvements to CLAUDE.md and SKILL.md files. ")
	b.WriteString("Your role is to help the reviewer understand, interrogate, and optionally refine this proposal.\n\n")

	b.WriteString("## Proposal Details\n\n")
	b.WriteString(fmt.Sprintf("- **Type:** %s\n", p.Proposal.Type))
	b.WriteString(fmt.Sprintf("- **Target file:** %s\n", p.Proposal.Target))
	b.WriteString(fmt.Sprintf("- **Confidence:** %s\n", p.Proposal.Confidence))

	if p.Proposal.Change != nil {
		b.WriteString(fmt.Sprintf("\n### Proposed Change\n\n%s\n", *p.Proposal.Change))
	}
	if p.Proposal.FlaggedEntry != nil {
		b.WriteString(fmt.Sprintf("\n### Flagged Entry\n\n%s\n", *p.Proposal.FlaggedEntry))
	}
	if p.Proposal.AssessmentSummary != nil {
		b.WriteString(fmt.Sprintf("\n### Assessment\n\n%s\n", *p.Proposal.AssessmentSummary))
	}

	b.WriteString(fmt.Sprintf("\n### Rationale\n\n%s\n", p.Proposal.Rationale))
	b.WriteString(fmt.Sprintf("\n### Source Session\n\n%s\n", p.SessionID))

	if len(p.Proposal.CitedUUIDs) > 0 {
		b.WriteString("\n### Cited Turn UUIDs\n\n")
		for i, uuid := range p.Proposal.CitedUUIDs {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, uuid))
		}
		b.WriteString("\nYou have Read and Grep access to the raw session transcript. Use it to examine cited turns.\n")
	}

	b.WriteString("\n## Guidelines\n\n")
	b.WriteString("- Format your responses in Markdown (headings, bold, lists, code blocks). Your output is rendered in a terminal with Markdown support. Do not manually wrap lines or insert hard line breaks — the terminal handles wrapping automatically.\n")
	b.WriteString("- Be concise and direct.\n")
	b.WriteString("- When asked to show raw turns, use the Read tool on the transcript file.\n")
	b.WriteString("- If asked for a revised version of the proposed change, wrap it in a ```revision``` code fence.\n")
	b.WriteString("- Do not invent turn content — always read it from the transcript.\n")
	b.WriteString("- You can also read the target file to see its current content.\n")

	return b.String()
}

func buildChatAllowedTools(p *pipeline.ProposalWithSession) string {
	rawDir := store.RawDir(p.SessionID)

	paths := []string{
		fmt.Sprintf("Read(//%s/**)", rawDir),
		fmt.Sprintf("Grep(//%s/**)", rawDir),
	}

	// Allow reading the target file and its parent directory.
	if p.Proposal.Target != "" {
		expanded := expandHomePath(p.Proposal.Target)
		dir := filepath.Dir(expanded)
		paths = append(paths,
			fmt.Sprintf("Read(//%s/**)", dir),
			fmt.Sprintf("Grep(//%s/**)", dir),
		)
	}

	return strings.Join(paths, ",")
}

func expandHomePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
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
