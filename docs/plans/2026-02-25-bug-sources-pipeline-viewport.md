# Add Viewport to Sources and Pipeline Views — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Prevent sources and pipeline views from overflowing the terminal by wrapping their content in a `viewport.Model`.

**Architecture:** Both views currently render all content as a flat string and fill the remaining height with `\n` characters. Sources renders the flat item list directly; pipeline renders all stat sections joined with `\n\n`. At the default config (`recentRunsLimit: 20`), the pipeline view produces ~42+ lines on a standard 80×24 terminal — well past what fits. The fix puts scrollable content into a `viewport.Model` sized to `height - chrome`. For sources, the chrome is the column header line (1) + status bar (1). For pipeline, all content is scrollable — no fixed chrome above the viewport since the persistent app header and sub-header are rendered by the root.

**Tech Stack:** `charmbracelet/bubbles/viewport`, existing `SetSize()` patterns from `detail`, `fitness`, `logview`.

**Important:** After each view change, run `make snapshots` and commit the updated PNG/SVG files alongside the code.

---

### Task 1: Add viewport to sources list view

**Files:**
- Modify: `internal/tui/sources/model.go`
- Modify: `internal/tui/sources/update.go`
- Modify: `internal/tui/sources/view.go`
- Test: `internal/tui/sources/model_test.go`

**Step 1: Write a failing test**

Add to `internal/tui/sources/model_test.go`:

```go
func TestSources_Viewport_ScrollsOnOverflow(t *testing.T) {
	// Build a model with many sources to force overflow.
	keys := shared.NewKeyMap("arrows")
	cfg := shared.DefaultConfig()

	groups := make([]fitness.SourceGroup, 1)
	groups[0].Label = "User-level"
	groups[0].Origin = "user"
	groups[0].Sources = make([]fitness.Source, 30)
	for i := range groups[0].Sources {
		groups[0].Sources[i] = fitness.Source{
			Name:         fmt.Sprintf("source-%02d", i),
			Origin:       "user",
			Ownership:    "mine",
			Approach:     "iterate",
			SessionCount: i,
			HealthScore:  float64(i) * 3,
		}
	}

	m := New(groups, &keys, cfg)
	m.SetSize(120, 20) // 20 lines total; 31 items won't fit

	// Viewport height = 20 - 2 (column header + status bar) = 18.
	if m.viewport.Height != 18 {
		t.Fatalf("viewport.Height = %d, want 18", m.viewport.Height)
	}

	// Navigate down past the viewport height — cursor should remain visible.
	for i := 0; i < 25; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	cursorLine := m.cursor
	if cursorLine < m.viewport.YOffset || cursorLine >= m.viewport.YOffset+m.viewport.Height {
		t.Errorf("cursor line %d not visible: YOffset=%d Height=%d",
			cursorLine, m.viewport.YOffset, m.viewport.Height)
	}
}
```

**Step 2: Run to verify it fails**

```bash
cd internal/tui/sources && go test -run "TestSources_Viewport_ScrollsOnOverflow" -v
```

Expected: FAIL — `m.viewport` field does not exist.

**Step 3: Add viewport to `sources/model.go`**

Add import:

```go
"github.com/charmbracelet/bubbles/viewport"
```

Add field to `Model` struct (after `height int`):

```go
viewport viewport.Model
```

Replace `SetSize()`:

```go
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	vpH := height - 2 // column header (1) + status bar (1)
	if vpH < 1 {
		vpH = 1
	}
	m.viewport = viewport.New(width, vpH)
	m.refreshViewport()
}
```

Add `refreshViewport()` and `ensureCursorVisible()`:

```go
// refreshViewport rebuilds the viewport content from the current flat list.
// Call whenever flatItems, cursor, or group state changes.
func (m *Model) refreshViewport() {
	if m.width == 0 {
		return
	}
	m.viewport.SetContent(m.renderFlatList())
	m.ensureCursorVisible()
}

// ensureCursorVisible adjusts the viewport offset so the cursor row is visible.
func (m *Model) ensureCursorVisible() {
	if len(m.flatItems) == 0 {
		return
	}
	yOff := m.viewport.YOffset
	h := m.viewport.Height
	if m.cursor < yOff {
		m.viewport.YOffset = m.cursor
	} else if m.cursor >= yOff+h {
		m.viewport.YOffset = m.cursor - h + 1
	}
}
```

Update `rebuildFlatItems()` to call `refreshViewport()` at the end:

```go
func (m *Model) rebuildFlatItems() {
	// ... existing code ...
	m.refreshViewport() // add this line at the end
}
```

**Step 4: Update `sources/update.go`**

After the final `switch msg := msg.(type)` block, add viewport forwarding for all unhandled messages (mouse events, resize-triggered redraws):

In `Update()`, before the final `return m, nil`, add:

```go
// Forward to viewport for scroll events.
var cmd tea.Cmd
m.viewport, cmd = m.viewport.Update(msg)
return m, cmd
```

In `handleKey()`, after each cursor mutation (`m.cursor++`, `m.cursor--`), call `m.ensureCursorVisible()`:

```go
case key.Matches(msg, m.keys.Down):
    if m.cursor < len(m.flatItems)-1 {
        m.cursor++
        m.ensureCursorVisible()
    }
    return m, nil

case key.Matches(msg, m.keys.Up):
    if m.cursor > 0 {
        m.cursor--
        m.ensureCursorVisible()
    }
    return m, nil
```

For Left/Right (collapse/expand), `rebuildFlatItems()` already calls `refreshViewport()` — no change needed.

**Step 5: Update `sources/view.go`**

In `View()`, replace the inline `b.WriteString(m.renderFlatList())` section with `m.viewport.View()`:

```go
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Ownership choice prompt overlay.
	if m.confirmState == ConfirmSetOwnership && m.ownershipPrompt != "" {
		return m.ownershipPrompt
	}

	// Confirmation prompt overlay.
	if m.confirm.Active {
		return m.confirm.View()
	}

	// Detail sub-view.
	if m.detailOpen {
		return m.renderDetail()
	}

	var b strings.Builder

	// Fixed chrome: column headers (only when items exist).
	if len(m.groups) > 0 {
		b.WriteString(m.renderColumnHeaders())
		b.WriteString("\n")
	}

	// Scrollable item list.
	if len(m.groups) == 0 {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("  No sources tracked."))
		b.WriteString("\n")
		// Fill remaining space.
		content := b.String()
		lines := strings.Count(content, "\n")
		remaining := m.height - lines - 1
		if remaining > 0 {
			content += strings.Repeat("\n", remaining)
		}
		return content + m.renderStatusBar()
	}

	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Status bar.
	return b.String() + m.renderStatusBar()
}
```

**Step 6: Run the new test and all sources tests**

```bash
cd internal/tui/sources && go test ./...
```

Expected: all pass.

**Step 7: Update snapshots**

```bash
make snapshot VIEW=source-manager
```

Verify the snapshot still looks correct. If the rendering changed, commit the updated file.

**Step 8: Commit**

```bash
git add internal/tui/sources/
git commit -m "fix(sources): add viewport to source list for overflow-safe scrolling

Sources rendered all items as a flat string with no scrolling. With many
source groups, content overflowed the terminal. Now the flat item list lives
in a viewport.Model sized to available height minus column header and status
bar chrome. Cursor navigation keeps the selected row visible."
```

---

### Task 2: Add viewport to pipeline monitor

**Files:**
- Modify: `internal/tui/pipeline/model.go`
- Modify: `internal/tui/pipeline/update.go`
- Modify: `internal/tui/pipeline/view.go`
- Test: `internal/tui/pipeline/model_test.go`

**Step 1: Write a failing test**

Read `internal/tui/pipeline/model_test.go` first to understand existing helpers.

Add to `internal/tui/pipeline/model_test.go`:

```go
func TestPipeline_Viewport_ExistsAfterSetSize(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 24)

	// Viewport height = 24 - 1 (status bar).
	if m.viewport.Height != 23 {
		t.Fatalf("viewport.Height = %d, want 23", m.viewport.Height)
	}
	if m.viewport.Width != 120 {
		t.Fatalf("viewport.Width = %d, want 120", m.viewport.Width)
	}
}

func TestPipeline_Viewport_ContentRendered(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "DAEMON") {
		t.Error("View should contain DAEMON section")
	}
	if !strings.Contains(view, "PIPELINE ACTIVITY") {
		t.Error("View should contain PIPELINE ACTIVITY section")
	}
	if !strings.Contains(view, "RECENT RUNS") {
		t.Error("View should contain RECENT RUNS section")
	}
}
```

**Step 2: Verify tests fail**

```bash
cd internal/tui/pipeline && go test -run "TestPipeline_Viewport" -v
```

Expected: FAIL — `m.viewport` field missing.

**Step 3: Add viewport to `pipeline/model.go`**

Add import:

```go
"github.com/charmbracelet/bubbles/viewport"
```

Add field to `Model` struct:

```go
viewport viewport.Model
```

Replace `SetSize()`:

```go
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	vpH := height - 1 // reserve 1 line for status bar
	if vpH < 1 {
		vpH = 1
	}
	m.viewport = viewport.New(width, vpH)
	m.refreshViewport()
}
```

Add `refreshViewport()`:

```go
// refreshViewport rebuilds the viewport content from all pipeline sections.
// Call after data refresh, resize, or cursor/expand state changes.
func (m *Model) refreshViewport() {
	if m.width == 0 {
		return
	}
	// Don't render into viewport when confirm is active (View() returns early).
	if m.confirm.Active {
		return
	}
	var sections []string
	sections = append(sections, m.renderDaemonHeader())
	sections = append(sections, m.renderActivityStats())
	sections = append(sections, m.renderRecentRuns())
	if m.layoutMode() != layoutNarrow {
		sections = append(sections, m.renderModels())
	}
	if len(m.prompts) > 0 && m.layoutMode() != layoutNarrow {
		sections = append(sections, m.renderPrompts())
	}
	m.viewport.SetContent(strings.Join(sections, "\n\n"))
}
```

Add import `"strings"` if not present.

Update `Refresh()` to call `refreshViewport()` at the end:

```go
func (m *Model) Refresh(runs []pl.PipelineRun, stats pl.PipelineStats, prompts []pl.PromptVersion, dashStats message.DashboardStats) tea.Cmd {
	m.runs = runs
	// ... existing clamping code ...
	m.refreshViewport() // add this line
	// ... existing status msg logic ...
}
```

**Step 4: Update `pipeline/update.go`**

In `Update()`, add viewport forwarding for unhandled messages, and call `m.refreshViewport()` after cursor/expand changes in `handleKey()`:

```go
// In handleKey(), after cursor changes:
case key.Matches(msg, m.keys.Down):
    if m.cursor < len(m.runs)-1 {
        m.cursor++
        m.refreshViewport() // cursor affects renderRecentRuns output
    }
    return m, nil

case key.Matches(msg, m.keys.Up):
    if m.cursor > 0 {
        m.cursor--
        m.refreshViewport()
    }
    return m, nil

case key.Matches(msg, m.keys.Open):
    if m.expandedIdx == m.cursor {
        m.expandedIdx = -1
    } else {
        m.expandedIdx = m.cursor
    }
    m.refreshViewport()
    return m, nil
```

In `Update()`, before the final `return m, nil`, forward to viewport:

```go
var cmd tea.Cmd
m.viewport, cmd = m.viewport.Update(msg)
return m, cmd
```

**Step 5: Update `pipeline/view.go`**

Replace `View()`:

```go
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Confirmation prompt overlay.
	if m.confirm.Active {
		return m.confirm.View()
	}

	statusBar := components.RenderStatusBar(m.keys.PipelineShortHelp(), m.statusMsg, m.width)
	return m.viewport.View() + "\n" + statusBar
}
```

Remove the old fill-to-bottom logic (the `strings.Count` / `strings.Repeat("\n", remaining)` block) — the viewport now handles height.

**Step 6: Run all pipeline tests**

```bash
cd internal/tui/pipeline && go test ./...
```

Expected: all pass. If any test asserted on exact line counts in the raw View() output, update it to use the viewport-aware output.

**Step 7: Update snapshots**

```bash
make snapshot VIEW=pipeline-monitor
```

Verify the snapshot. Commit updated snapshot files.

**Step 8: Commit**

```bash
git add internal/tui/pipeline/
git commit -m "fix(pipeline): add viewport to pipeline monitor for overflow-safe scrolling

At default config (recentRunsLimit=20), the pipeline monitor produced ~42+
lines on an 80x24 terminal with no way to scroll. All sections now render
into a viewport.Model sized to available height minus the status bar.
Cursor and expand state changes refresh the viewport content."
```

---

### Task 3: Verify end-to-end on a small terminal

```bash
make install && cabrero
```

1. Resize terminal to 80×24.
2. Open Pipeline Monitor — confirm all sections fit and the viewport scrolls with j/↓.
3. Open Source Manager with many sources — confirm the list scrolls.
4. Resize to 120×40 — confirm everything still renders correctly.
5. Run the full test suite:

```bash
go test ./internal/tui/...
```

Expected: all pass.
