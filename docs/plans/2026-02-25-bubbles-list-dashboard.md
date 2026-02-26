# Adopt `bubbles/list` for Dashboard — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the dashboard's custom list/viewport/filter infrastructure with `charmbracelet/bubbles/list`. Net result: ~70 fewer lines of code, filter-as-you-type with live results, and match highlighting — making `bug-dashboard-filter.md` redundant (do not execute that plan).

**Architecture:**
- `DashboardItem` gains `FilterValue()` to satisfy `list.Item`.
- A `dashboardDelegate` implements `list.ItemDelegate` to produce the existing single-line column-aligned rendering.
- `dashboard.Model` replaces `viewport`, `cursor`, `filterInput`, `filterActive`, `filterText`, `filtered` with a single `list.Model`.
- A custom `FilterFunc` handles `type:`/`target:` prefix syntax. Match highlighting marks the matched column cell.
- All list chrome (title, status bar, pagination, help) is disabled; we render column headers and the sort/status bar ourselves, exactly as today.
- Sort: items are sorted before `list.SetItems()` is called; cycling sort re-sorts and calls `SetItems()` again.

**Replaces:** `bug-dashboard-filter.md` — do not execute that plan.

**Dependency:** Run after `dx-architecture-cleanup`. This ensures the style infrastructure is stable before replacing the dashboard's interaction model, making any regression easier to catch. `shared.Checkmark` and `shared.RelativeTime` from `dx-unified-utilities` are available.

**Snapshot note:** Run `make snapshot VIEW=dashboard` and `make snapshot VIEW=dashboard-narrow` and `make snapshot VIEW=dashboard-empty` after Task 4. Commit updated files.

---

### Background: what is deleted vs what stays

| Deleted | Kept / moved |
|---------|-------------|
| `Model.filtered []DashboardItem` | `Model.items []DashboardItem` (source of truth for sort) |
| `Model.cursor int` | `Model.list.Index()` |
| `Model.viewport viewport.Model` | `list.Model` internal viewport |
| `Model.filterInput textinput.Model` | `list.Model.FilterInput` |
| `Model.filterActive bool` | `list.Model.SettingFilter()` |
| `Model.filterText string` | `list.Model.FilterValue()` |
| `applyFilter()` | `dashboardFilter()` FilterFunc |
| `updateFilter()` | handled by `list.Update()` |
| `ensureCursorVisible()` | handled by list |
| `updateContent()` | handled by list |
| `viewportHeight()` | `viewportHeight()` simplified |
| `renderItemRows()` | `dashboardDelegate.Render()` |

---

### Task 1: Add `FilterValue()` to `DashboardItem` and write `dashboardDelegate`

**Files:**
- Modify: `internal/tui/dashboard/model.go`
- Create: `internal/tui/dashboard/delegate.go`
- Test: `internal/tui/dashboard/model_test.go`

**Step 1: Write a test for `FilterValue()`**

Add to `internal/tui/dashboard/model_test.go`:

```go
func TestDashboardItem_FilterValue(t *testing.T) {
	p := testdata.TestProposal()
	item := DashboardItem{Proposal: &p}

	fv := item.FilterValue()
	if !strings.Contains(fv, item.TypeName()) {
		t.Errorf("FilterValue %q should contain TypeName %q", fv, item.TypeName())
	}
	if !strings.Contains(fv, "target:") {
		t.Error("FilterValue should contain 'target:' tag")
	}
	if !strings.Contains(fv, "type:") {
		t.Error("FilterValue should contain 'type:' tag")
	}
}
```

**Step 2: Add `FilterValue()` to `DashboardItem` in `model.go`**

```go
// FilterValue implements list.Item. Returns a tagged string used by dashboardFilter.
// Format: "type:<TypeName> target:<Target> confidence:<Confidence>"
func (d DashboardItem) FilterValue() string {
	return "type:" + d.TypeName() + " target:" + d.Target() + " confidence:" + d.Confidence()
}
```

**Step 3: Run test**

```bash
cd internal/tui/dashboard && go test -run TestDashboardItem_FilterValue -v
```

Expected: PASS.

**Step 4: Create `internal/tui/dashboard/delegate.go`**

```go
package dashboard

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

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
	target := shared.TruncatePad(shared.ShortenHome(di.Target()), cols.targetWidth)

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
```

Also move `columnLayoutForWidth` out of the `columnLayout()` method into a standalone function (needed by the delegate which doesn't have a model receiver):

In `view.go`, rename `columnLayout()` method to a package-level function:

```go
// columnLayoutForWidth computes column widths for the given terminal width.
func columnLayoutForWidth(width int) columnSpec {
	overhead := 5 + colType + 2 + 2 + colConfidence
	targetWidth := width - overhead
	if targetWidth < 15 {
		targetWidth = 15
	}
	return columnSpec{typeWidth: colType, targetWidth: targetWidth}
}
```

Update the one call site in `renderColumnHeaders()` from `m.columnLayout()` to `columnLayoutForWidth(m.width)` (keep this for the transition — `m.width` will become `m.list.Width()` in Task 2).

**Step 5: Build**

```bash
go build ./internal/tui/dashboard/
```

**Step 6: Commit**

```bash
git add internal/tui/dashboard/model.go internal/tui/dashboard/delegate.go internal/tui/dashboard/view.go
git commit -m "feat(dashboard): add FilterValue to DashboardItem and dashboardDelegate for bubbles/list"
```

---

### Task 2: Replace the Model struct, New(), and accessors

**Files:**
- Modify: `internal/tui/dashboard/model.go`
- Test: `internal/tui/dashboard/model_test.go`

**Step 1: Write failing tests for the new Model API**

The existing tests use `m.cursor`, `m.viewport.YOffset`, `m.viewport.Height`, `m.filtered` directly. Update them to use the list API. Add new tests first:

```go
func TestDashboard_ListIndex_Navigation(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if m.list.Index() != 0 {
		t.Fatalf("initial index = %d, want 0", m.list.Index())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.list.Index() != 1 {
		t.Errorf("index after down = %d, want 1", m.list.Index())
	}
}

func TestDashboard_HasActiveInput_WhenFiltering(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if m.HasActiveInput() {
		t.Error("HasActiveInput should be false initially")
	}

	// Open filter.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.HasActiveInput() {
		t.Error("HasActiveInput should be true while filtering")
	}
}
```

**Step 2: Rewrite `Model` struct and `New()` in `model.go`**

Replace the current struct and `New()`:

```go
import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	// ... other imports
)

// Model is the dashboard view model.
type Model struct {
	items     []DashboardItem    // source of truth for sorting; also passed to list
	list      list.Model
	stats     message.DashboardStats
	sortOrder string
	keys      *shared.KeyMap
	config    *shared.Config
}

func New(proposals []pipeline.ProposalWithSession, reports []fitness.Report,
	stats message.DashboardStats, keys *shared.KeyMap, cfg *shared.Config) Model {

	sortOrder := cfg.Dashboard.SortOrder
	if sortOrder == "" {
		sortOrder = SortNewest
	}

	// Build and sort the unified item list.
	items := buildItems(proposals, reports)
	sorted := sortItems(items, sortOrder)

	l := newList(sorted, keys)

	m := Model{
		items:     items,  // unsorted original, needed for re-sort on cycle
		list:      l,
		stats:     stats,
		sortOrder: sortOrder,
		keys:      keys,
		config:    cfg,
	}
	return m
}

// buildItems constructs the unified item slice: proposals first, then fitness reports.
func buildItems(proposals []pipeline.ProposalWithSession, reports []fitness.Report) []DashboardItem {
	items := make([]DashboardItem, 0, len(proposals)+len(reports))
	for i := range proposals {
		items = append(items, DashboardItem{Proposal: &proposals[i]})
	}
	for i := range reports {
		items = append(items, DashboardItem{FitnessReport: &reports[i]})
	}
	return items
}

// newList constructs a configured list.Model for the dashboard.
func newList(items []DashboardItem, keys *shared.KeyMap) list.Model {
	listItems := toListItems(items)
	l := list.New(listItems, dashboardDelegate{}, 0, 0)

	// Disable all list chrome — we render our own.
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false) // we render filter input ourselves in the status bar area
	l.DisableQuitKeybindings()

	// Custom filter for type:/target: prefix syntax.
	l.Filter = dashboardFilter

	// Remap list navigation to our KeyMap (respects arrows vs vim setting).
	l.KeyMap.CursorUp = keys.Up
	l.KeyMap.CursorDown = keys.Down
	l.KeyMap.NextPage = keys.HalfPageDown
	l.KeyMap.PrevPage = keys.HalfPageUp
	l.KeyMap.GoToStart = keys.GotoTop
	l.KeyMap.GoToEnd = keys.GotoBottom
	l.KeyMap.Filter = keys.Filter
	l.KeyMap.ClearFilter = keys.Back
	l.KeyMap.CancelWhileFiltering = keys.Back
	l.KeyMap.AcceptWhileFiltering = keys.Open

	// Disable list's own quit/help bindings (root handles these).
	off := key.NewBinding(key.WithDisabled())
	l.KeyMap.Quit = off
	l.KeyMap.ForceQuit = off
	l.KeyMap.ShowFullHelp = off
	l.KeyMap.CloseFullHelp = off

	return l
}

func toListItems(items []DashboardItem) []list.Item {
	out := make([]list.Item, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}
```

**Step 3: Update accessor methods**

```go
// SelectedItem returns the DashboardItem at the current cursor position.
func (m Model) SelectedItem() *DashboardItem {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	di := item.(DashboardItem)
	return &di
}

// SelectedProposal returns the proposal at the cursor, or nil.
func (m Model) SelectedProposal() *pipeline.ProposalWithSession {
	item := m.SelectedItem()
	if item == nil || !item.IsProposal() {
		return nil
	}
	return item.Proposal
}

// SelectedFitnessReport returns the fitness report at the cursor, or nil.
func (m Model) SelectedFitnessReport() *fitness.Report {
	item := m.SelectedItem()
	if item == nil || !item.IsFitnessReport() {
		return nil
	}
	return item.FitnessReport
}

// HasActiveInput returns true when the filter input is active.
func (m Model) HasActiveInput() bool {
	return m.list.SettingFilter()
}

// CycleSortOrder advances to the next sort order and refreshes the list.
func (m *Model) CycleSortOrder() tea.Cmd {
	for i, s := range sortOrders {
		if s == m.sortOrder {
			m.sortOrder = sortOrders[(i+1)%len(sortOrders)]
			return m.applySort()
		}
	}
	m.sortOrder = SortNewest
	return m.applySort()
}

// applySort re-sorts m.items and updates the list.
func (m *Model) applySort() tea.Cmd {
	sorted := sortItems(m.items, m.sortOrder)
	return m.list.SetItems(toListItems(sorted))
}
```

Note: `CycleSortOrder` now returns a `tea.Cmd` (the list's `SetItems` returns one for an internal status message). The caller in `handleKey` must propagate it.

**Step 4: Remove the old `applyFilter()` and its comment**

Delete `applyFilter()` (the dead no-op implementation). It is replaced by `dashboardFilter` in Task 3.

**Step 5: Run tests**

```bash
cd internal/tui/dashboard && go test ./...
```

Many existing tests will fail because they access `m.cursor`, `m.viewport`, `m.filtered` directly. Make a note of each failing test — they are updated in Task 4 (test cleanup). For now, confirm the new tests pass:

```bash
go test -run "TestDashboard_ListIndex|TestDashboard_HasActiveInput" -v
```

**Step 6: Commit**

```bash
git add internal/tui/dashboard/model.go
git commit -m "refactor(dashboard): replace custom list/viewport/filter with bubbles/list Model"
```

---

### Task 3: Replace Update() and View()

**Files:**
- Modify: `internal/tui/dashboard/update.go`
- Modify: `internal/tui/dashboard/view.go`

**Step 1: Rewrite `update.go`**

```go
package dashboard

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the dashboard.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, m.viewportHeight(msg.Width, msg.Height))
		return m, nil

	case message.StatusMessage:
		m.statusMsg = msg.Text
		if msg.Duration > 0 {
			m.statusExpiry = time.Now().Add(msg.Duration)
			return m, tea.Tick(msg.Duration, func(time.Time) tea.Msg {
				return message.StatusMessageExpired{}
			})
		}
		return m, nil

	case message.StatusMessageExpired:
		if !m.statusExpiry.IsZero() && time.Now().After(m.statusExpiry) {
			m.statusMsg = ""
		}
		return m, nil

	case tea.KeyMsg:
		// While filter is active, route all keys to the list.
		if m.list.SettingFilter() {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		return m.handleKey(msg)
	}

	// Forward all other messages (mouse, spinner ticks, etc.) to list.
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Open):
		item := m.SelectedItem()
		if item == nil {
			return m, nil
		}
		if item.IsFitnessReport() {
			return m, func() tea.Msg {
				return message.PushView{View: message.ViewFitnessDetail}
			}
		}
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewProposalDetail}
		}

	case key.Matches(msg, m.keys.Sort):
		cmd := m.CycleSortOrder()
		return m, cmd

	case key.Matches(msg, m.keys.Approve):
		if m.SelectedProposal() != nil {
			return m, func() tea.Msg {
				return message.PushView{View: message.ViewProposalDetail, Action: "approve"}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Reject):
		if m.SelectedProposal() != nil {
			return m, func() tea.Msg {
				return message.PushView{View: message.ViewProposalDetail, Action: "reject"}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Defer):
		if m.SelectedProposal() != nil {
			return m, func() tea.Msg {
				return message.PushView{View: message.ViewProposalDetail, Action: "defer"}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Sources):
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewSourceManager}
		}

	case key.Matches(msg, m.keys.Pipeline):
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewPipelineMonitor}
		}
	}

	// Navigation (up/down/half-page/goto) handled by list.
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
```

Note: `statusMsg`/`statusExpiry` fields come from the statusmsg bug plan. If that plan hasn't run yet, add them to the struct and handle them here as part of this plan.

**Step 2: Rewrite `view.go`**

```go
package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// View renders the dashboard.
func (m Model) View() string {
	if m.list.Width() == 0 || m.list.Height() == 0 {
		return ""
	}

	var b strings.Builder

	// Fixed chrome above list: column headers (only when items exist).
	if len(m.list.Items()) > 0 {
		b.WriteString(m.renderColumnHeaders())
		b.WriteString("\n")
	}

	// Scrollable list (handles empty state internally via custom rendering).
	if len(m.list.VisibleItems()) == 0 && !m.list.SettingFilter() {
		// No items at all — show flavor text.
		b.WriteString("\n")
		b.WriteString(shared.MutedStyle.Render("  " + components.EmptyProposals()))
		b.WriteString("\n")
	} else {
		b.WriteString(m.list.View())
		b.WriteString("\n")
	}

	// Sort indicator + filter status (only when items exist and not in filter input mode).
	if len(m.list.Items()) > 0 && !m.list.SettingFilter() {
		sortLine := fmt.Sprintf("  Sort: %s", m.sortOrder)
		if m.list.IsFiltered() {
			sortLine += fmt.Sprintf("  ·  filter: %q  (%d/%d)",
				m.list.FilterValue(),
				len(m.list.VisibleItems()),
				len(m.list.Items()))
		}
		b.WriteString(shared.MutedStyle.Render(sortLine))
		b.WriteString("\n")
	}

	// Filter input or status bar.
	if m.list.SettingFilter() {
		b.WriteString("/ " + m.list.FilterInput.View())
	} else {
		b.WriteString(m.renderStatusBar())
	}

	return b.String()
}

// RenderHeader — moved to components.RenderHeader (see dx-architecture-cleanup plan).
// For now kept here for backward compatibility.
func RenderHeader(stats message.DashboardStats, width int) string {
	return components.RenderHeader(stats, width)
}

// SubHeader returns the view title and stats line for the dashboard.
func (m Model) SubHeader() string {
	statsLine := fmt.Sprintf("  %d awaiting review", m.stats.PendingCount)
	if m.stats.ApprovedCount > 0 {
		statsLine += fmt.Sprintf("  ·  %d approved", m.stats.ApprovedCount)
	}
	if m.stats.RejectedCount > 0 {
		statsLine += fmt.Sprintf("  ·  %d rejected", m.stats.RejectedCount)
	}
	if m.stats.FitnessReportCount > 0 {
		statsLine += fmt.Sprintf("  ·  %d fitness reports", m.stats.FitnessReportCount)
	}
	return shared.HeaderStyle.Render("  Proposals") + "\n" + shared.MutedStyle.Render(statsLine)
}

func (m Model) renderColumnHeaders() string {
	cols := columnLayoutForWidth(m.list.Width())
	header := shared.PadRight("   TYPE", cols.typeWidth+3) +
		"  " + shared.PadRight("TARGET", cols.targetWidth) +
		"  " + "CONFIDENCE"
	return shared.MutedStyle.Render(header)
}

func (m Model) renderStatusBar() string {
	if len(m.list.VisibleItems()) == 0 {
		bindings := []key.Binding{m.keys.Sources, m.keys.Pipeline, m.keys.Help}
		return components.RenderStatusBar(bindings, m.statusMsg, m.list.Width())
	}
	item := m.SelectedItem()
	if item != nil && item.IsFitnessReport() {
		bindings := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Open, m.keys.Sources, m.keys.Help}
		return components.RenderStatusBar(bindings, m.statusMsg, m.list.Width())
	}
	return components.RenderStatusBar(m.keys.ShortHelp(), m.statusMsg, m.list.Width())
}

// viewportHeight returns the list height accounting for chrome.
func (m Model) viewportHeight(width, height int) int {
	chrome := 1 // status/filter bar
	if len(m.list.Items()) > 0 {
		chrome += 2 // column headers + sort indicator
	}
	h := height - chrome
	if h < 1 {
		h = 1
	}
	return h
}
```

Note: `m.width` is no longer stored on `Model` directly — use `m.list.Width()`. The `statusMsg` field is added from the statusmsg bug plan; if that plan hasn't run, add `statusMsg string` and `statusExpiry time.Time` here.

**Step 3: Build**

```bash
go build ./internal/tui/dashboard/
```

**Step 4: Build entire TUI (catches callers of changed API)**

```bash
go build ./internal/tui/...
```

Fix any compilation errors (e.g., callers of `CycleSortOrder()` now receive a `tea.Cmd`).

**Step 5: Commit**

```bash
git add internal/tui/dashboard/update.go internal/tui/dashboard/view.go
git commit -m "refactor(dashboard): rewrite Update/View for bubbles/list"
```

---

### Task 4: Implement `dashboardFilter` (custom FilterFunc)

**Files:**
- Create: `internal/tui/dashboard/filter.go`
- Test: `internal/tui/dashboard/filter_test.go`

**Step 1: Write failing tests**

```go
package dashboard

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
)

func TestDashboardFilter_FreeText(t *testing.T) {
	targets := []string{
		"type:skill_improvement target:~/.claude/SKILL.md confidence:high",
		"type:skill_scaffold target:~/Work/project confidence:medium",
		"type:fitness_report target:docx-helper confidence:85% health",
	}

	ranks := dashboardFilter("skill", targets)
	if len(ranks) != 2 {
		t.Errorf("'skill' should match 2 items, got %d", len(ranks))
	}
}

func TestDashboardFilter_TypePrefix(t *testing.T) {
	targets := []string{
		"type:skill_improvement target:path1 confidence:high",
		"type:skill_scaffold target:path2 confidence:medium",
		"type:fitness_report target:docx-helper confidence:low",
	}

	ranks := dashboardFilter("type:skill", targets)
	if len(ranks) != 2 {
		t.Errorf("type:skill should match 2 items, got %d", len(ranks))
	}
	for _, r := range ranks {
		if r.Index == 2 {
			t.Error("fitness_report should not match type:skill")
		}
	}
}

func TestDashboardFilter_TargetPrefix(t *testing.T) {
	targets := []string{
		"type:skill_improvement target:~/.claude/docx-helper confidence:high",
		"type:skill_scaffold target:~/Work/project confidence:medium",
	}

	ranks := dashboardFilter("target:docx", targets)
	if len(ranks) != 1 || ranks[0].Index != 0 {
		t.Errorf("target:docx should match item 0 only, got %v", ranks)
	}
}

func TestDashboardFilter_CaseInsensitive(t *testing.T) {
	targets := []string{
		"type:SKILL_IMPROVEMENT target:path confidence:HIGH",
	}
	if len(dashboardFilter("skill_improvement", targets)) != 1 {
		t.Error("filter should be case-insensitive")
	}
	if len(dashboardFilter("SKILL_IMPROVEMENT", targets)) != 1 {
		t.Error("filter should be case-insensitive for uppercase query")
	}
}

func TestDashboardFilter_EmptyTerm_ReturnsAll(t *testing.T) {
	targets := []string{"a", "b", "c"}
	ranks := dashboardFilter("", targets)
	if len(ranks) != 3 {
		t.Errorf("empty filter should return all %d items, got %d", len(targets), len(ranks))
	}
}

func TestDashboardFilter_NoMatch_ReturnsEmpty(t *testing.T) {
	targets := []string{
		"type:skill_improvement target:path confidence:high",
	}
	if len(dashboardFilter("xyzzy_no_match_12345", targets)) != 0 {
		t.Error("unmatched filter should return empty")
	}
}
```

**Step 2: Verify tests fail**

```bash
cd internal/tui/dashboard && go test -run TestDashboardFilter -v
```

Expected: FAIL — `dashboardFilter` undefined.

**Step 3: Create `filter.go`**

```go
package dashboard

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// dashboardFilter implements list.FilterFunc with support for:
//   - "type:<val>"   — match items whose TypeName contains val
//   - "target:<val>" — match items whose Target contains val
//   - free text      — match anywhere in the FilterValue string
//
// All matching is case-insensitive. FilterValue() must produce the tagged
// format "type:<T> target:<U> confidence:<C>" for prefix matching to work.
func dashboardFilter(term string, targets []string) []list.Rank {
	lower := strings.ToLower(strings.TrimSpace(term))

	if lower == "" {
		ranks := make([]list.Rank, len(targets))
		for i := range ranks {
			ranks[i] = list.Rank{Index: i}
		}
		return ranks
	}

	var matchFn func(target string) bool

	if val, ok := strings.CutPrefix(lower, "type:"); ok {
		matchFn = func(target string) bool {
			return strings.Contains(extractTag(target, "type:"), val)
		}
	} else if val, ok := strings.CutPrefix(lower, "target:"); ok {
		matchFn = func(target string) bool {
			return strings.Contains(extractTag(target, "target:"), val)
		}
	} else {
		matchFn = func(target string) bool {
			return strings.Contains(strings.ToLower(target), lower)
		}
	}

	var ranks []list.Rank
	for i, t := range targets {
		if matchFn(strings.ToLower(t)) {
			ranks = append(ranks, list.Rank{Index: i})
		}
	}
	return ranks
}

// extractTag returns the value of a tagged field in a FilterValue string.
// e.g. extractTag("type:foo target:bar", "target:") → "bar"
func extractTag(s, tag string) string {
	idx := strings.Index(s, tag)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(tag):]
	if space := strings.Index(rest, " "); space >= 0 {
		return rest[:space]
	}
	return rest
}
```

**Step 4: Run filter tests**

```bash
cd internal/tui/dashboard && go test -run TestDashboardFilter -v
```

Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/tui/dashboard/filter.go internal/tui/dashboard/filter_test.go
git commit -m "feat(dashboard): implement dashboardFilter — type:/target: prefix and free text"
```

---

### Task 5: Update tests and verify full suite

**Files:**
- Modify: `internal/tui/dashboard/model_test.go`

**Step 1: Update tests that accessed deleted fields directly**

`TestDashboard_Navigation` — replace `m.cursor` checks with `m.list.Index()`:

```go
// Before: if m.cursor != 0 {
if m.list.Index() != 0 {
    t.Fatalf("initial index = %d, want 0", m.list.Index())
}
// Before: if m.cursor != i {
if m.list.Index() != i {
    t.Errorf("index after down %d = %d, want %d", i, m.list.Index(), i)
}
```

`TestDashboard_ViewportScrolls` — replace `m.viewport.Height` and `m.viewport.YOffset`:

```go
// List height after setting size = viewportHeight(80, 8).
// chrome = 3 (column header + sort indicator + status bar) → list height = 5.
if m.list.Height() != 5 {
    t.Fatalf("list height = %d, want 5", m.list.Height())
}

// After navigating cursor to item 10:
if m.list.Index() != 10 {
    t.Fatalf("index = %d, want 10", m.list.Index())
}
// Cursor is visible if list.Index() is within the paginated view.
// bubbles/list always keeps the selected item visible — just verify index is correct.
```

`TestDashboard_HalfPageScroll` — replace `m.viewport.Height / 2` with expected half-page from list height:

```go
// List height 18 (21 - 3 chrome) → half page = 9.
// After PgDn, list.Index() should be ~9.
if m.list.Index() != 9 {
    t.Errorf("index after PgDn = %d, want 9", m.list.Index())
}
```

`TestDashboard_EmptyState` — test View() output for flavor text (unchanged, still works):

```go
// This test remains valid — just check view contains flavor text.
```

**Step 2: Run all dashboard tests**

```bash
cd internal/tui/dashboard && go test ./... -v
```

Fix any remaining failures. Common issues:
- `m.filtered` → `m.list.VisibleItems()` (if any test references it)
- `m.filterActive` → `m.list.SettingFilter()`
- `m.sortOrder` is still a field on `Model` — this reference is fine

**Step 3: Run all TUI tests**

```bash
go test ./internal/tui/...
```

**Step 4: Update snapshots**

```bash
make snapshot VIEW=dashboard
make snapshot VIEW=dashboard-narrow
make snapshot VIEW=dashboard-empty
```

Review the snapshots. The visual output should be identical or near-identical to before (same column alignment, same cursor `> ` prefix, same indicator characters). Commit updated snapshots.

**Step 5: Final commit**

```bash
git add internal/tui/dashboard/model_test.go snapshots/
git commit -m "test(dashboard): update tests for bubbles/list migration"
```

---

### Task 6: Manual smoke test

```bash
make install && cabrero
```

1. Dashboard loads — items visible with correct column alignment.
2. Press `/` — filter input appears at bottom.
3. Type `skill` — list updates live to show only matching items.
4. Type `type:skill` — type-prefix filter works.
5. Type `target:docx` — target-prefix filter works.
6. Press Esc — filter clears, all items return.
7. Press `o` — sort cycles: newest → oldest → confidence → type → newest.
8. Press `↓`/`j` — cursor moves down.
9. Press End/`G` — jumps to last item.
10. Press `?` for help — help overlay opens, Esc closes.
11. Resize terminal while list is open — layout adjusts correctly.
12. Verify `q` quits (list's built-in quit binding is disabled; root handles it).
