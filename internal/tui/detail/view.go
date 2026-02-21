package detail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	detailHeader  = lipgloss.NewStyle().Bold(true)
	detailMuted   = lipgloss.NewStyle().Foreground(shared.ColorMuted)
	detailSection = lipgloss.NewStyle().Bold(true)
	detailAccent  = lipgloss.NewStyle().Foreground(shared.ColorAccent)
)

// View renders the proposal detail view.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.proposal == nil {
		return "  No proposal selected."
	}

	var b strings.Builder

	// Header.
	p := &m.proposal.Proposal
	b.WriteString(detailHeader.Render(fmt.Sprintf("  Proposal: %s", p.Type)))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Target: %s\n", p.Target))
	b.WriteString(fmt.Sprintf("  Confidence: %s  │  Session: %s\n",
		detailAccent.Render(p.Confidence),
		detailMuted.Render(shared.TruncateID(m.proposal.SessionID, 12))))
	b.WriteString("\n")

	// Proposed change.
	b.WriteString(detailSection.Render("  PROPOSED CHANGE"))
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("─", 17))
	b.WriteString("\n")
	b.WriteString(shared.IndentBlock(m.diffViewport.View(), 2))
	b.WriteString("\n\n")

	// Rationale.
	b.WriteString(detailSection.Render("  RATIONALE"))
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("─", 17))
	b.WriteString("\n")
	b.WriteString(shared.WrapIndent(p.Rationale, m.width, 2))
	b.WriteString("\n\n")

	// Citation chain.
	if len(m.citations) > 0 {
		b.WriteString(detailSection.Render(fmt.Sprintf("  CITATION CHAIN (%d entries)", len(m.citations))))
		b.WriteString("\n")
		b.WriteString("  " + strings.Repeat("─", 17))
		b.WriteString("\n")
		b.WriteString(renderCitations(m.citations, m.width))
		b.WriteString("\n")
	}

	// Apply state overlay.
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

	// Fill remaining space.
	content := b.String()
	lines := strings.Count(content, "\n")
	remaining := m.height - lines - 1
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Status bar — hide "tab" hint when chat panel isn't visible.
	bindings := m.keys.DetailShortHelp()
	if !m.isWideMode() || !m.config.Detail.ChatPanelOpen {
		var filtered []key.Binding
		for _, b := range bindings {
			if key.Matches(tea.KeyMsg{Type: tea.KeyTab}, b) {
				continue
			}
			filtered = append(filtered, b)
		}
		bindings = filtered
	}
	content += components.RenderStatusBar(bindings, "", m.width)

	return content
}

func renderCitations(citations []shared.CitationEntry, width int) string {
	var b strings.Builder
	for i, c := range citations {
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}
		b.WriteString(fmt.Sprintf("  %s%s\n", prefix, detailMuted.Render(c.Summary)))
		if c.Expanded {
			b.WriteString(shared.IndentBlock(c.RawJSON, 6))
			b.WriteString("\n")
		}
	}
	return b.String()
}

