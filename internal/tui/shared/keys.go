package shared

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all key bindings for the TUI.
type KeyMap struct {
	// Navigation
	Up           key.Binding
	Down         key.Binding
	Left         key.Binding
	Right        key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	GotoTop      key.Binding
	GotoBottom   key.Binding

	// Global
	Quit        key.Binding
	ForceQuit   key.Binding
	Help        key.Binding
	Back        key.Binding
	TabForward  key.Binding
	TabBackward key.Binding

	// Dashboard
	Open   key.Binding
	Filter key.Binding
	Sort   key.Binding

	// Actions (shared between dashboard and detail)
	Approve key.Binding
	Reject  key.Binding
	Defer   key.Binding

	// Detail
	Chat        key.Binding
	UseRevision key.Binding
	Chip1       key.Binding
	Chip2       key.Binding
	Chip3       key.Binding
	Chip4       key.Binding

	// Fitness Report
	Dismiss key.Binding

	// Source Manager
	ToggleApproach key.Binding
	SetOwnership   key.Binding
	Rollback       key.Binding

	// Pipeline Monitor
	Retry        key.Binding
	LogView      key.Binding
	Refresh      key.Binding

	// Log Viewer
	Search       key.Binding
	SearchNext   key.Binding
	SearchPrev   key.Binding
	FollowToggle key.Binding
	ExpandAll    key.Binding
	CollapseAll  key.Binding

	// Future views
	Sources  key.Binding
	Pipeline key.Binding
}

// NewKeyMap creates a KeyMap for the given navigation mode ("arrows" or "vim").
func NewKeyMap(nav string) KeyMap {
	km := KeyMap{
		// Global keys — same in both modes.
		Quit:        key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		ForceQuit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "force quit")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Back:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		TabForward:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next pane")),
		TabBackward: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev pane")),

		// Dashboard keys.
		Open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Filter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Sort:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "sort")),

		// Action keys — same in both modes.
		Approve: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approve")),
		Reject:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reject")),
		Defer:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "defer")),

		// Detail keys.
		Chat:        key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "chat")),
		UseRevision: key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "use revision")),
		Chip1:       key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "chip 1")),
		Chip2:       key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "chip 2")),
		Chip3:       key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "chip 3")),
		Chip4:       key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "chip 4")),

		// Fitness Report.
		Dismiss: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "dismiss")),

		// Source Manager.
		ToggleApproach: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle approach")),
		SetOwnership:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "ownership")),
		Rollback:       key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "rollback")),

		// Pipeline Monitor.
		Retry:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "retry")),
		LogView: key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "log")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),

		// Log Viewer.
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		SearchNext:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
		SearchPrev:   key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev")),
		FollowToggle: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "follow")),
		ExpandAll:    key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "expand all")),
		CollapseAll:  key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "collapse all")),

		// Future views.
		Sources:  key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sources")),
		Pipeline: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pipeline")),
	}

	if nav == "vim" {
		km.Up = key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up"))
		km.Down = key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down"))
		km.Left = key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "left"))
		km.Right = key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "right"))
		km.HalfPageUp = key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up"))
		km.HalfPageDown = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down"))
		km.GotoTop = key.NewBinding(key.WithKeys("g"), key.WithHelp("gg", "top"))
		km.GotoBottom = key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom"))
	} else {
		km.Up = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up"))
		km.Down = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down"))
		km.Left = key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "left"))
		km.Right = key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "right"))
		km.HalfPageUp = key.NewBinding(key.WithKeys("pgup"), key.WithHelp("PgUp", "half page up"))
		km.HalfPageDown = key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("PgDn", "half page down"))
		km.GotoTop = key.NewBinding(key.WithKeys("home"), key.WithHelp("Home", "top"))
		km.GotoBottom = key.NewBinding(key.WithKeys("end"), key.WithHelp("End", "bottom"))
	}

	return km
}

// ShortHelp returns bindings for the short help view (dashboard context).
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Up, k.Down, k.Open, k.Approve, k.Reject, k.Defer, k.Sources, k.Pipeline, k.Help,
	}
}

// DetailShortHelp returns bindings for the detail view status bar.
func (k KeyMap) DetailShortHelp() []key.Binding {
	return []key.Binding{
		k.Up, k.Down, k.Back, k.Approve, k.Reject, k.Defer, k.Chat, k.TabForward, k.Help,
	}
}

// FitnessShortHelp returns help bindings for the fitness detail view.
func (k KeyMap) FitnessShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.Back, k.Dismiss, k.Sources, k.Chat, k.Help}
}

// SourcesShortHelp returns help bindings for the source manager view.
// Left/Right (collapse/expand groups) are omitted from the status bar
// since the generic arrow labels are misleading; the help overlay shows them.
func (k KeyMap) SourcesShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.ToggleApproach, k.SetOwnership, k.Back, k.Help}
}

// PipelineShortHelp returns help bindings for the pipeline monitor view.
func (k KeyMap) PipelineShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.Retry, k.Refresh, k.LogView, k.Back, k.Help}
}

// LogViewShortHelp returns help bindings for the log viewer.
func (k KeyMap) LogViewShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.ExpandAll, k.CollapseAll, k.Search, k.FollowToggle, k.Back, k.Help}
}
