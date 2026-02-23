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

	// Header (5 visual lines).
	p := &m.proposal.Proposal
	b.WriteString(detailHeader.Render(fmt.Sprintf("  Proposal: %s", p.Type)))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Target: %s\n", p.Target))
	b.WriteString(fmt.Sprintf("  Confidence: %s  │  Session: %s\n",
		detailAccent.Render(p.Confidence),
		detailMuted.Render(m.proposal.SessionID)))
	b.WriteString("\n")

	// Scrollable body viewport.
	b.WriteString(m.bodyViewport.View())
	b.WriteString("\n")

	// Fill remaining space.
	content := b.String()
	lines := strings.Count(content, "\n")
	remaining := m.height - lines - 1
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Status bar — hide tab hint when chat panel isn't open (nothing to tab to).
	bindings := m.keys.DetailShortHelp()
	if !m.config.Detail.ChatPanelOpen {
		var filtered []key.Binding
		for _, kb := range bindings {
			if key.Matches(tea.KeyMsg{Type: tea.KeyTab}, kb) {
				continue
			}
			filtered = append(filtered, kb)
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

