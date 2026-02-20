package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/chat"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/dashboard"
	"github.com/vladolaru/cabrero/internal/tui/detail"
	fitness_tui "github.com/vladolaru/cabrero/internal/tui/fitness"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
	"github.com/vladolaru/cabrero/internal/tui/sources"
)

// reviewModel is the root Bubble Tea model for the review TUI.
type reviewModel struct {
	state     message.ViewState
	viewStack []message.ViewState
	config    *shared.Config
	styles    shared.Styles

	// Status bar
	statusMsg    string
	statusExpiry time.Time

	// Child models
	dashboard dashboard.Model
	detail    detail.Model
	chat      chat.Model
	fitness   fitness_tui.Model
	sources   sources.Model

	// Source groups for re-use when pushing ViewSourceManager.
	sourceGroups []fitness.SourceGroup

	// Shared
	help     help.Model
	helpOpen bool
	keys     shared.KeyMap

	// All proposals for navigation context
	proposals []pipeline.ProposalWithSession

	width  int
	height int
}

// newReviewModel creates the root model with loaded data.
func newReviewModel(proposals []pipeline.ProposalWithSession, reports []fitness.Report, stats message.DashboardStats, sourceGroups []fitness.SourceGroup, cfg *shared.Config) reviewModel {
	keys := shared.NewKeyMap(cfg.Navigation)
	styles := shared.ThemeFromConfig(cfg)

	m := reviewModel{
		state:        message.ViewDashboard,
		config:       cfg,
		styles:       styles,
		keys:         keys,
		proposals:    proposals,
		sourceGroups: sourceGroups,
		help:         help.New(),
		dashboard:    dashboard.New(proposals, reports, stats, &keys, cfg),
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

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width

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

	case message.RejectFinished, message.DeferFinished:
		// Return to dashboard after action.
		m.statusMsg = actionStatusText(msg)
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
		m.sources.SetSize(m.width, m.height)
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
	}

	// Route to active child.
	switch m.state {
	case message.ViewDashboard:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case message.ViewProposalDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Always forward to chat — it handles stream messages and input when focused.
		var chatCmd tea.Cmd
		m.chat, chatCmd = m.chat.Update(msg)
		if chatCmd != nil {
			cmds = append(cmds, chatCmd)
		}
	case message.ViewFitnessDetail:
		var cmd tea.Cmd
		m.fitness, cmd = m.fitness.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case message.ViewSourceManager, message.ViewSourceDetail:
		var cmd tea.Cmd
		m.sources, cmd = m.sources.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m reviewModel) View() string {
	var content string

	switch m.state {
	case message.ViewDashboard:
		content = m.dashboard.View()
	case message.ViewProposalDetail:
		if m.width >= 120 && m.config.Detail.ChatPanelOpen {
			// Wide mode: detail and chat side by side.
			detailView := m.detail.View()
			chatView := m.chat.View()
			content = lipgloss.JoinHorizontal(lipgloss.Top, detailView, chatView)
		} else {
			content = m.detail.View()
		}
	case message.ViewFitnessDetail:
		content = m.fitness.View()
	case message.ViewSourceManager, message.ViewSourceDetail:
		content = m.sources.View()
	}

	// Help overlay.
	if m.helpOpen {
		content = m.help.View(m.keys)
	}

	return content
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
			m.detail.SetSize(m.width, m.height)

			// Initialize chat for this proposal.
			chips := defaultChips()
			m.chat = chat.New(chips, "", chatWidth(m.width, m.config), m.height-6)

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
		m.fitness.SetSize(m.width, m.height)

	case message.ViewSourceManager:
		// Initialize source manager with current source groups.
		m.sources = sources.New(m.sourceGroups, &m.keys, m.config)
		m.sources.SetSize(m.width, m.height)
		if action != "" {
			m.sources = m.sources.PreSelectSource(action)
		}
	}

	return m, tea.Batch(cmds...)
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

func chatWidth(totalWidth int, cfg *shared.Config) int {
	if totalWidth < 120 {
		return totalWidth - 2
	}
	pct := cfg.Detail.ChatPanelWidth
	if pct <= 0 {
		pct = 35
	}
	return totalWidth * pct / 100
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
