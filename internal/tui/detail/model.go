package detail

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Model is the proposal detail view model.
type Model struct {
	proposal     *pipeline.ProposalWithSession
	diffViewport viewport.Model
	citations    []shared.CitationEntry
	citationVP   viewport.Model
	focus        Focus
	applyState   ApplyState
	revision     *string // chat-produced alternative diff
	blendResult  *string // before/after diff from blend
	spinner      spinner.Model
	confirm      components.ConfirmModel
	revConfirm   components.RevisionConfirmModel
	width        int
	height       int
	keys         *shared.KeyMap
	config       *shared.Config
}

// New creates a detail model for the given proposal.
func New(p *pipeline.ProposalWithSession, citations []shared.CitationEntry, keys *shared.KeyMap, cfg *shared.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := Model{
		proposal:   p,
		citations:  citations,
		focus:      FocusProposal,
		applyState: ApplyIdle,
		spinner:    s,
		keys:       keys,
		config:     cfg,
	}

	return m
}

// SetSize updates the model dimensions and recalculates viewports.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Reserve space for header (4 lines), status bar (1 line), borders.
	contentHeight := height - 6
	if contentHeight < 4 {
		contentHeight = 4
	}

	contentWidth := width - 2
	if m.isWideMode() {
		// Split: proposal pane gets (100 - chatPanelWidth)%.
		chatPct := m.config.Detail.ChatPanelWidth
		if chatPct <= 0 {
			chatPct = 35
		}
		contentWidth = width*(100-chatPct)/100 - 2
	}

	m.diffViewport = viewport.New(contentWidth, contentHeight)
	m.diffViewport.SetContent(m.renderDiffContent())

	m.citationVP = viewport.New(contentWidth, contentHeight)
	m.citationVP.SetContent(m.renderCitationContent())
}

func (m Model) isWideMode() bool {
	return m.width >= 120
}

func (m Model) renderDiffContent() string {
	if m.proposal == nil {
		return "(no proposal)"
	}
	p := &m.proposal.Proposal
	return RenderDiff(p.Change, p.FlaggedEntry, p.Type, m.width)
}

func (m Model) renderCitationContent() string {
	return renderCitations(m.citations, m.width)
}

// SelectedCitationIndex returns the index of the citation at cursor, or -1.
func (m Model) SelectedCitationIndex() int {
	// For now, citations aren't individually selectable via cursor.
	// Expansion is toggled via Enter on the viewport.
	return -1
}

// HasRevision returns true if a chat-produced revision is available.
func (m Model) HasRevision() bool {
	return m.revision != nil
}

// HasChatFocus returns true if the chat pane has focus.
func (m Model) HasChatFocus() bool {
	return m.focus == FocusChat
}
