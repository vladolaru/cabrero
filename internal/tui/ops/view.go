package ops

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// SubHeader renders the persistent sub-header for the Operations view.
func (m Model) SubHeader() string {
	s := m.stats
	total := s.GatedRuns + s.SkippedBusy + s.ErrorRuns + s.MetaTriggered + s.MetaCooldowns
	statsLine := fmt.Sprintf("  %d operational event(s) in window", total)
	return shared.RenderSubHeader("  Operations", statsLine)
}

// View renders the complete view content.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	if m.isEmpty() {
		empty := "\n" + shared.MutedStyle.Render("  No operational events recorded yet.") + "\n"
		return shared.FillToBottom(empty, m.height, 1) + m.renderStatusBar()
	}

	return m.viewport.View() + "\n" + m.renderStatusBar()
}

func (m Model) renderStatusBar() string {
	return components.RenderStatusBar(m.shortHelp(), m.statusMsg, m.width)
}

func (m Model) shortHelp() []key.Binding {
	return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Back, m.keys.Help}
}

func (m Model) isEmpty() bool {
	s := m.stats
	return s.GatedRuns == 0 && s.SkippedBusy == 0 && s.ErrorRuns == 0 &&
		s.MetaTriggered == 0 && s.MetaCooldowns == 0 && s.ProcessedRuns == 0
}

// renderBody builds the scrollable content.
func (m Model) renderBody() string {
	var b strings.Builder
	s := m.stats

	// Summary cards.
	b.WriteString("\n")
	b.WriteString(shared.HeaderStyle.Render("  Summary") + "\n")
	b.WriteString(renderCardLine("Gated runs", s.GatedRuns, s.DailyGated))
	b.WriteString(renderCardLine("Skipped (busy)", s.SkippedBusy, s.DailySkipped))
	b.WriteString(renderCardLine("Errors", s.ErrorRuns, s.DailyErrors))
	b.WriteString(renderCardLine("Processed", s.ProcessedRuns, s.DailyProcessed))
	b.WriteString("\n")

	// Meta section.
	b.WriteString(shared.HeaderStyle.Render("  Meta Analysis") + "\n")
	b.WriteString(fmt.Sprintf("    Triggered:     %d\n", s.MetaTriggered))
	b.WriteString(fmt.Sprintf("    Cooldown skip: %d\n", s.MetaCooldowns))
	b.WriteString(fmt.Sprintf("    No threshold:  %d\n", s.MetaNoThreshold))
	b.WriteString("\n")

	// Gate breakdown.
	if s.GatedRuns > 0 {
		b.WriteString(shared.HeaderStyle.Render("  Gate Breakdown") + "\n")
		b.WriteString(fmt.Sprintf("    Unclassified source: %d\n", s.GatedUnclassified))
		b.WriteString(fmt.Sprintf("    Paused source:       %d\n", s.GatedPaused))
		b.WriteString("\n")
	}

	// Recent events.
	if len(s.RecentEvents) > 0 {
		b.WriteString(shared.HeaderStyle.Render("  Recent Events") + "\n")
		for i, ev := range s.RecentEvents {
			prefix := "  "
			if i == m.cursor {
				prefix = shared.AccentStyle.Render("> ")
			}
			b.WriteString(prefix)
			b.WriteString(renderEvent(ev))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func renderCardLine(label string, count int, daily []int) string {
	countStr := fmt.Sprintf("%d", count)
	if count > 0 {
		countStr = shared.AccentBoldStyle.Render(countStr)
	} else {
		countStr = shared.MutedStyle.Render(countStr)
	}
	spark := ""
	if len(daily) > 0 {
		spark = "  " + shared.MutedStyle.Render(components.RenderSparkline(daily, 14))
	}
	return fmt.Sprintf("    %-18s %s%s\n", label, countStr, spark)
}

func renderEvent(ev pipeline.OpsEvent) string {
	ts := ev.Timestamp.Local().Format("Jan 02 15:04")
	sid := ""
	if ev.SessionID != "" {
		sid = store.ShortSessionID(ev.SessionID)
	}

	var status string
	switch ev.Status {
	case pipeline.HistoryStatusSkippedBusy:
		status = shared.WarningStyle.Render("skipped_busy")
	case pipeline.HistoryStatusError:
		status = shared.ErrorStyle.Render("error")
	case pipeline.HistoryStatusProcessed:
		status = shared.MutedStyle.Render("gated")
	case pipeline.HistoryStatusMetaTriggered:
		status = shared.SuccessStyle.Render("meta_triggered")
	case pipeline.HistoryStatusMetaCooldown:
		status = shared.WarningStyle.Render("meta_cooldown")
	default:
		status = ev.Status
	}

	reason := ""
	if ev.Reason != "" {
		reason = " " + shared.MutedStyle.Render("("+ev.Reason+")")
	}

	project := ""
	if ev.Project != "" {
		project = " " + shared.MutedStyle.Render(store.ProjectDisplayName(ev.Project))
	}

	if sid != "" {
		return fmt.Sprintf("  %s  %-8s  %s%s%s", ts, sid, status, reason, project)
	}
	return fmt.Sprintf("  %s  %-8s  %s%s", ts, ev.Source, status, reason)
}
