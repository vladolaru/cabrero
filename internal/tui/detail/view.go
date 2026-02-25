package detail

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)


// SubHeader returns the view title and contextual stats for the proposal detail.
func (m Model) SubHeader() string {
	title := shared.HeaderStyle.Render("  Proposal Detail")
	if m.proposal == nil {
		return title
	}
	p := &m.proposal.Proposal
	statsLine := fmt.Sprintf("  %s  ·  %s  ·  %s",
		p.Type,
		shared.ShortenHome(p.Target),
		p.Confidence)
	return title + "\n" + shared.MutedStyle.Render(statsLine)
}

// View renders the proposal detail view.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.proposal == nil {
		return "  No proposal selected."
	}

	var b strings.Builder

	b.WriteString("\n") // spacing before viewport

	// Scrollable body viewport.
	b.WriteString(m.bodyViewport.View())
	b.WriteString("\n")

	content := b.String()
	if m.HideStatusBar {
		// No fill — root handles layout (horizontal or vertical split).
	} else {
		// Fill remaining space.
		lines := strings.Count(content, "\n")
		// Reserve 1 line for the status bar.
		remaining := m.height - lines - 1
		if remaining > 0 {
			content += strings.Repeat("\n", remaining)
		}
		bindings := m.keys.DetailShortHelp()
		if !m.config.Detail.ChatPanelOpen {
			var filtered []key.Binding
			for _, kb := range bindings {
				if key.Matches(tea.KeyPressMsg{Code: tea.KeyTab}, kb) {
					continue
				}
				filtered = append(filtered, kb)
			}
			bindings = filtered
		}
		content += components.RenderStatusBar(bindings, "", m.width)
	}

	return content
}

func renderCitations(citations []shared.CitationEntry, cursor int, width int) string {
	var b strings.Builder
	for i, c := range citations {
		prefix := "    "
		if i == cursor {
			prefix = "  > "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", prefix, shared.MutedStyle.Render(c.Summary)))
		if c.Expanded {
			b.WriteString(shared.WrapIndent(c.RawJSON, width, 6))
			b.WriteString("\n")
		}
	}
	return b.String()
}

