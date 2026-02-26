package detail

import (
	"fmt"
	"strings"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)


// SubHeader returns the view title and contextual stats for the proposal detail.
func (m Model) SubHeader() string {
	if m.proposal == nil {
		return shared.RenderSubHeader("  Proposal Detail", "")
	}
	p := &m.proposal.Proposal
	badge := ""
	if p.Type == pipeline.TypePromptImprovement || p.Type == pipeline.TypePipelineInsight {
		badge = "  [META]"
	}
	statsLine := fmt.Sprintf("  %s%s  ·  %s  ·  %s", p.Type, badge, cli.ShortenHome(p.Target), p.Confidence)
	return shared.RenderSubHeader("  Proposal Detail", statsLine)
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

	return b.String()
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

