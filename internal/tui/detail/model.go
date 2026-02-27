package detail

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Model is the proposal detail view model.
type Model struct {
	proposal       *pipeline.ProposalWithSession
	bodyViewport   viewport.Model
	citations      []shared.CitationEntry
	citationCursor int // -1 = no selection; 0..len-1 = selected citation
	focus          Focus
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

	// inlineChat holds rendered chat content for narrow mode (appended to body viewport).
	inlineChat string
	// inlineChatInput holds the chat input line for fixed rendering below the viewport.
	inlineChatInput string
}

// New creates a detail model for the given proposal.
func New(p *pipeline.ProposalWithSession, citations []shared.CitationEntry, keys *shared.KeyMap, cfg *shared.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	citationCursor := -1
	if len(citations) > 0 {
		citationCursor = 0
	}

	m := Model{
		proposal:       p,
		citations:      citations,
		citationCursor: citationCursor,
		focus:          FocusProposal,
		applyState:     ApplyIdle,
		spinner:        s,
		keys:           keys,
		config:         cfg,
	}

	return m
}

// SetSize updates the model dimensions and recalculates the body viewport.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Chrome: newline before viewport (1) + newline after (1).
	// The root always renders the status bar externally and accounts for it
	// by passing (childHeight - 1) as height. We reserve only viewport spacing.
	chrome := 2
	bodyHeight := height - chrome
	if bodyHeight < 4 {
		bodyHeight = 4
	}

	contentWidth := width - 2

	m.contentWidth = contentWidth
	m.bodyViewport = viewport.New(viewport.WithWidth(contentWidth), viewport.WithHeight(bodyHeight))
	m.bodyViewport.SetContent(m.renderBodyContent())
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
	b.WriteString(shared.RenderSectionHeader("PROPOSED CHANGE"))
	b.WriteString("\n")
	b.WriteString(shared.IndentBlock(RenderDiff(p.Change, p.FlaggedEntry, p.Type, m.contentWidth), 2))
	b.WriteString("\n\n")

	// RATIONALE section.
	b.WriteString(shared.RenderSectionHeader("RATIONALE"))
	b.WriteString("\n")
	b.WriteString(shared.WrapIndent(p.Rationale, m.contentWidth, 2))
	b.WriteString("\n\n")

	// CITATION CHAIN.
	if len(m.citations) > 0 {
		b.WriteString(shared.RenderSectionHeader(fmt.Sprintf("CITATION CHAIN (%d entries)", len(m.citations))))
		b.WriteString("\n")
		b.WriteString(renderCitations(m.citations, m.citationCursor, m.width))
		b.WriteString("\n")
	}

	// Inline chat content (narrow mode — chat appears within the scrollable body).
	if m.inlineChat != "" {
		b.WriteString("\n")
		b.WriteString(m.inlineChat)
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
			b.WriteString(shared.RenderSectionHeader("BLENDED RESULT"))
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

	content := b.String()
	if m.focus == FocusChat {
		content = shared.MuteANSI(content)
	}
	return content
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

// SetRevision sets the chat-produced revision for the approve flow.
func (m *Model) SetRevision(rev *string) {
	m.revision = rev
}

// SetInlineChat sets rendered chat content to be appended inside the body viewport.
// The input line is rendered as fixed chrome below the viewport (not scrollable).
func (m *Model) SetInlineChat(content, input string) {
	wasAtBottom := m.bodyViewport.AtBottom()
	m.inlineChat = content
	m.inlineChatInput = input
	m.bodyViewport.SetContent(m.renderBodyContent())
	if wasAtBottom {
		m.bodyViewport.GotoBottom()
	}
}

// ClearInlineChat removes any inline chat content from the viewport.
func (m *Model) ClearInlineChat() {
	if m.inlineChat != "" || m.inlineChatInput != "" {
		m.inlineChat = ""
		m.inlineChatInput = ""
		m.bodyViewport.SetContent(m.renderBodyContent())
	}
}

// CurrentFocus returns which pane currently has focus.
func (m Model) CurrentFocus() Focus { return m.focus }

// SetFocus updates which pane has focus and refreshes the body viewport so that
// muting is applied or removed immediately.
func (m *Model) SetFocus(f Focus) {
	if m.focus == f {
		return
	}
	m.focus = f
	m.bodyViewport.SetContent(m.renderBodyContent())
}

// HasActivePrompt returns true when a confirmation prompt is active
// and the view should handle Esc itself (to dismiss the prompt).
func (m Model) HasActivePrompt() bool {
	return m.applyState == ApplyConfirming ||
		m.applyState == ApplyRejectConfirming ||
		m.applyState == ApplyDeferConfirming ||
		m.applyState == ApplyReviewing
}

