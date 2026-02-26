package dashboard

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// dashboardDelegate renders each DashboardItem as a single column-aligned line.
type dashboardDelegate struct{}

func (d dashboardDelegate) Height() int  { return 1 }
func (d dashboardDelegate) Spacing() int { return 0 }

func (d dashboardDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d dashboardDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	di, ok := item.(DashboardItem)
	if !ok {
		return
	}

	// Column layout from current list width.
	cols := columnLayoutForWidth(m.Width())

	prefix := "  "
	if index == m.Index() {
		prefix = "> "
	}

	var indicator string
	if di.IsProposal() {
		indicator = shared.AccentStyle.Render(indicatorProposal)
	} else {
		indicator = shared.WarningStyle.Render(indicatorFitness)
	}

	typeName := shared.PadRight(di.TypeName(), cols.typeWidth)
	target := shared.TruncatePad(cli.ShortenHome(di.Target()), cols.targetWidth)

	// When this item matches the active filter, highlight the type field.
	confidence := shared.MutedStyle.Render(di.Confidence())
	if m.IsFiltered() && len(m.MatchesForItem(index)) > 0 {
		typeName = shared.AccentStyle.Render(typeName)
	}

	line := fmt.Sprintf("%s %s %s  %s  %s", prefix, indicator, typeName, target, confidence)
	if index == m.Index() {
		line = shared.SelectedStyle.Render(line)
	}

	fmt.Fprint(w, line)
}
