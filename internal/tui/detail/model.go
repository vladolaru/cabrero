package detail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Model is the proposal detail view model.
type Model struct {
	proposal     *pipeline.ProposalWithSession
	bodyViewport viewport.Model
	citations    []shared.CitationEntry
	focus        Focus
	applyState   ApplyState
	revision     *string // chat-produced alternative diff
	blendResult  *string // before/after diff from blend
	spinner      spinner.Model
	confirm      components.ConfirmModel
	revConfirm   components.RevisionConfirmModel
	width        int
	height       int
	contentWidth int // viewport width for text wrapping
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

// SetSize updates the model dimensions and recalculates the body viewport.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Reserve: 5 lines header + 1 status bar + 1 fill = 7 lines of chrome.
	bodyHeight := height - 7
	if bodyHeight < 4 {
		bodyHeight = 4
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

	m.contentWidth = contentWidth
	m.bodyViewport = viewport.New(contentWidth, bodyHeight)
	m.bodyViewport.SetContent(m.renderBodyContent())
}

func (m Model) isWideMode() bool {
	return m.width >= 120
}

// renderBodyContent builds the scrollable content for the body viewport:
// diff, rationale, citations, and any active apply-state overlay.
func (m Model) renderBodyContent() string {
	if m.proposal == nil {
		return "(no proposal)"
	}

	var b strings.Builder
	p := &m.proposal.Proposal

	// PROPOSED CHANGE section.
	b.WriteString(detailSection.Render("  PROPOSED CHANGE"))
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("─", 17))
	b.WriteString("\n")
	b.WriteString(shared.IndentBlock(RenderDiff(p.Change, p.FlaggedEntry, p.Type, m.contentWidth), 2))
	b.WriteString("\n\n")

	// RATIONALE section.
	b.WriteString(detailSection.Render("  RATIONALE"))
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("─", 17))
	b.WriteString("\n")
	b.WriteString(shared.WrapIndent(p.Rationale, m.contentWidth, 2))
	b.WriteString("\n\n")

	// CITATION CHAIN.
	if len(m.citations) > 0 {
		b.WriteString(detailSection.Render(fmt.Sprintf("  CITATION CHAIN (%d entries)", len(m.citations))))
		b.WriteString("\n")
		b.WriteString("  " + strings.Repeat("─", 17))
		b.WriteString("\n")
		b.WriteString(renderCitations(m.citations, m.width))
		b.WriteString("\n")
	}

	// Apply state overlay (inside viewport so it scrolls naturally).
	switch m.applyState {
	case ApplyConfirming:
		b.WriteString("\n")
		if m.HasRevision() {
			b.WriteString("  " + m.revConfirm.View())
		} else {
			b.WriteString("  " + m.confirm.View())
		}
	case ApplyBlending:
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s Blending change...", m.spinner.View()))
	case ApplyReviewing:
		if m.blendResult != nil {
			b.WriteString("\n")
			b.WriteString(detailSection.Render("  BLENDED RESULT"))
			b.WriteString("\n")
			b.WriteString(shared.IndentBlock(*m.blendResult, 2))
			b.WriteString("\n\n")
			b.WriteString("  " + m.confirm.View())
		}
	case ApplyRejectConfirming, ApplyDeferConfirming:
		b.WriteString("\n")
		b.WriteString("  " + m.confirm.View())
	case ApplyDone:
		b.WriteString("\n")
		b.WriteString("  " + components.ConfirmApprove())
	}

	return b.String()
}

// Proposal returns the underlying proposal, or nil.
func (m Model) Proposal() *pipeline.ProposalWithSession {
	return m.proposal
}

// BlendResult returns the blended content from the approve flow, or empty string.
func (m Model) BlendResult() string {
	if m.blendResult != nil {
		return *m.blendResult
	}
	return ""
}

// HasRevision returns true if a chat-produced revision is available.
func (m Model) HasRevision() bool {
	return m.revision != nil
}

// HasActivePrompt returns true when a confirmation prompt is active
// and the view should handle Esc itself (to dismiss the prompt).
func (m Model) HasActivePrompt() bool {
	return m.applyState == ApplyConfirming ||
		m.applyState == ApplyRejectConfirming ||
		m.applyState == ApplyDeferConfirming ||
		m.applyState == ApplyReviewing
}

