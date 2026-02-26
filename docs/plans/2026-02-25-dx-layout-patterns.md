# DX: Layout Patterns — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Three related layout-level improvements: (1) extract the repeated fill+statusbar pattern into a shared utility; (2) remove the `HideStatusBar` anti-pattern — detail should never render its own status bar, the root always renders it; (3) add a proper confirm overlay renderer for views where the confirm dialog is currently a raw single-line string.

**Architecture:**
- `shared.FillToBottom(content, height, reserved)` replaces 3 remaining copy-pasted fill blocks (sources detail, detail view, fitness view). The sources list view and pipeline view instances were eliminated when `bug-sources-pipeline-viewport` added viewports to those views.
- `detail.Model.HideStatusBar` is deleted; the root always subtracts 1 line from detail's height for the status bar and always renders it.
- `components.RenderConfirmOverlay(text, width, height)` gives pipeline and sources a properly-sized confirm view.

**Dependency:** Run after `dx-unified-utilities` and `dx-style-consolidation`. `bug-sources-pipeline-viewport` is already done — the two instances it would have removed are gone.

**Snapshot note:** Removing `HideStatusBar` and the confirm overlay change visual output. Run `make snapshots` after Task 2 and Task 3.

---

### Task 1: Add `shared.FillToBottom` and replace fill instances

**Files:**
- Modify: `internal/tui/shared/format.go`
- Modify: `internal/tui/sources/view.go` (`renderDetail` only — the list view already uses viewport)
- Modify: `internal/tui/detail/view.go` (when HideStatusBar=false path; simplified further in Task 2)
- Modify: `internal/tui/fitness/view.go`
- Test: `internal/tui/shared/format_test.go`

**Step 1: Write failing test**

```go
func TestFillToBottom_AddsNewlines(t *testing.T) {
    content := "line1\nline2\nline3"
    // content has 2 newlines → 3 lines rendered. Height=10, reserved=1 (status bar).
    // remaining = 10 - 2 - 1 = 7 → append 7 newlines.
    result := FillToBottom(content, 10, 1)
    newlines := strings.Count(result, "\n")
    if newlines != 9 { // 2 existing + 7 added
        t.Errorf("FillToBottom newlines = %d, want 9", newlines)
    }
}

func TestFillToBottom_NoOpWhenAlreadyFull(t *testing.T) {
    content := strings.Repeat("line\n", 10)
    result := FillToBottom(content, 10, 0)
    if result != content {
        t.Error("FillToBottom should not modify content that already fills height")
    }
}
```

**Step 2: Add `FillToBottom` to `shared/format.go`**

```go
// FillToBottom pads content with newlines so the total height is
// (totalHeight - reservedLines). Use reservedLines=1 for a status bar.
// Returns content unchanged if it already meets or exceeds the target.
func FillToBottom(content string, totalHeight, reservedLines int) string {
    lines := strings.Count(content, "\n")
    remaining := totalHeight - lines - reservedLines
    if remaining > 0 {
        return content + strings.Repeat("\n", remaining)
    }
    return content
}
```

**Step 3: Verify test passes**

```bash
cd internal/tui/shared && go test -run TestFillToBottom -v
```

**Step 4: Replace fill instances**

Verify which instances still exist after Bug 2 (viewport for sources/pipeline removes their list view instances):

```bash
grep -rn "strings.Count.*\\\\n" internal/tui/sources/ internal/tui/detail/ internal/tui/fitness/ internal/tui/logview/
```

For each remaining instance, replace the pattern:

```go
// Before (example from sources/view.go renderDetail):
content := b.String()
lines := strings.Count(content, "\n")
statusBarHeight := 1
remaining := m.height - lines - statusBarHeight
if remaining > 0 {
    content += strings.Repeat("\n", remaining)
}
content += m.renderDetailStatusBar()

// After:
content := shared.FillToBottom(b.String(), m.height, 1)
content += m.renderDetailStatusBar()
```

Apply to: `sources/view.go` (renderDetail), `fitness/view.go`, and `detail/view.go` (the non-HideStatusBar path, which will be cleaned up further in Task 2).

**Step 5: Build and test**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

**Step 6: Commit**

```bash
git add internal/tui/shared/format.go internal/tui/sources/view.go internal/tui/fitness/view.go internal/tui/detail/view.go
git commit -m "refactor(tui): extract shared.FillToBottom, replace 5 copy-pasted fill blocks"
```

---

### Task 2: Remove `HideStatusBar` — root always renders detail's status bar

**Files:**
- Modify: `internal/tui/detail/model.go`
- Modify: `internal/tui/detail/view.go`
- Modify: `internal/tui/model.go` (root)
- Test: `internal/tui/detail/model_test.go`

**Step 1: Write failing test**

Add to `internal/tui/detail/model_test.go`:

```go
func TestDetail_NeverRendersOwnStatusBar(t *testing.T) {
    // detail.View() should never include status bar bindings in its output.
    // The root is responsible for rendering the status bar.
    keys := shared.NewKeyMap("arrows")
    cfg := testdata.TestConfig()
    p := testdata.TestProposalSkillImprovement()
    m := New(&p, nil, &keys, cfg)
    m.SetSize(80, 24)

    view := ansi.Strip(m.View())

    // The status bar renders key hints like "approve" — these should not appear
    // in the detail view's own output.
    if strings.Contains(view, "approve") {
        t.Error("detail.View() should not render its own status bar")
    }
}
```

**Step 2: Verify it fails** (current code renders status bar when HideStatusBar=false)

```bash
cd internal/tui/detail && go test -run TestDetail_NeverRendersOwnStatusBar -v
```

Expected: FAIL.

**Step 3: Remove `HideStatusBar` from `detail/model.go`**

Delete the field from `Model` struct:
```go
// DELETE:
HideStatusBar bool
```

Update `SetSize()` — always use chrome = 2 (spacing before and after viewport, no status bar):

```go
func (m *Model) SetSize(width, height int) {
    m.width = width
    m.height = height
    // Root always renders the status bar externally and accounts for it
    // by passing (childHeight - 1) as height. We reserve only viewport spacing.
    chrome := 2
    bodyHeight := height - chrome
    if bodyHeight < 4 {
        bodyHeight = 4
    }
    contentWidth := width - 2
    m.contentWidth = contentWidth
    m.bodyViewport = viewport.New(contentWidth, bodyHeight)
    m.bodyViewport.SetContent(m.renderBodyContent())
}
```

**Step 4: Remove status bar rendering from `detail/view.go`**

Simplify `View()`:

```go
func (m Model) View() string {
    if m.width == 0 || m.height == 0 {
        return ""
    }
    if m.proposal == nil {
        return "  No proposal selected."
    }

    var b strings.Builder
    b.WriteString("\n") // spacing before viewport
    b.WriteString(m.bodyViewport.View())
    b.WriteString("\n")
    return b.String()
}
```

The key filtering block (removing TabForward when chat is closed) moves to the root's view composition.

**Step 5: Update `internal/tui/model.go`**

Remove all `m.detail.HideStatusBar = ...` assignments from `resizeDetailChat()`.

Change `resizeDetailChat()` to always subtract 1 for the status bar and always pass that reduced height:

```go
func (m *appModel) resizeDetailChat() {
    ch := m.childHeight()
    if !m.config.Detail.ChatPanelOpen {
        m.detail.SetSize(m.width, ch-1) // -1 for root-rendered status bar
        return
    }
    // ... wide and narrow split cases: already subtract 1 via panelH := ch - 1
    // No HideStatusBar assignment needed anywhere.
}
```

In `View()`, always render the status bar for `ViewProposalDetail`:

```go
case message.ViewProposalDetail:
    if m.config.Detail.ChatPanelOpen && m.width >= 160 {
        // Wide: horizontal split
        detailView := m.detail.View()
        sep := m.renderVerticalSeparator(m.childHeight() - 1)
        chatView := m.chat.View()
        content = lipgloss.JoinHorizontal(lipgloss.Top, detailView, sep, chatView)
    } else if m.config.Detail.ChatPanelOpen {
        // Narrow: vertical split
        sep := shared.MutedStyle.Render(strings.Repeat("─", m.width))
        chatView := shared.IndentBlock(m.chat.View(), 2)
        content = m.detail.View() + sep + "\n" + chatView
    } else {
        content = m.detail.View()
    }
    // Root ALWAYS renders the status bar for the detail view.
    bindings := m.keys.DetailShortHelp()
    if !m.config.Detail.ChatPanelOpen {
        // Filter out Tab binding when chat panel is closed.
        var filtered []key.Binding
        for _, kb := range bindings {
            if key.Matches(tea.KeyMsg{Type: tea.KeyTab}, kb) {
                continue
            }
            filtered = append(filtered, kb)
        }
        bindings = filtered
    }
    content += "\n" + components.RenderStatusBar(bindings, "", m.width)
```

**Step 6: Build and test**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

**Step 7: Update snapshots**

```bash
make snapshots
```

Verify `proposal-detail` and `proposal-detail-chat` snapshots look correct.

**Step 8: Commit**

```bash
git add internal/tui/detail/model.go internal/tui/detail/view.go internal/tui/model.go snapshots/
git commit -m "refactor(detail): remove HideStatusBar — root always renders detail status bar

detail.Model.HideStatusBar was set by the root (parent mutating child state)
to signal which code path should render the status bar. This bidirectional
coupling is eliminated: detail.View() never renders a status bar; the root
always appends it after composing the detail/chat layout. SetSize() now
always uses chrome=2 since the status bar height is accounted for by the
caller."
```

---

### Task 3: Add `components.RenderConfirmOverlay` for modal confirm views

**Files:**
- Modify: `internal/tui/components/confirm.go`
- Modify: `internal/tui/pipeline/view.go`
- Modify: `internal/tui/sources/view.go`
- Test: `internal/tui/components/confirm_test.go`

**Step 1: Write failing test**

Add to `internal/tui/components/confirm_test.go`:

```go
func TestRenderConfirmOverlay_FillsHeight(t *testing.T) {
    overlay := RenderConfirmOverlay("Apply this change? [y/N]", 80, 20)
    lines := strings.Count(overlay, "\n") + 1
    // Should be close to 20 lines.
    if lines < 18 || lines > 22 {
        t.Errorf("overlay lines = %d, want ~20", lines)
    }
    if !strings.Contains(overlay, "Apply this change?") {
        t.Error("overlay should contain prompt text")
    }
}
```

**Step 2: Add `RenderConfirmOverlay` to `components/confirm.go`**

```go
// RenderConfirmOverlay renders a confirmation prompt centered vertically
// within the available terminal space. Use this for modal confirmations
// in views that return early (pipeline, sources) rather than embedding
// the prompt in a viewport.
func RenderConfirmOverlay(text string, width, height int) string {
    var b strings.Builder
    topPad := height / 2
    if topPad > 0 {
        b.WriteString(strings.Repeat("\n", topPad))
    }
    b.WriteString(statusBarStyle.Width(width).Render(text))
    return b.String()
}
```

Add `"strings"` import if not present.

**Step 3: Update `pipeline/view.go`**

Replace:
```go
if m.confirm.Active {
    return m.confirm.View()
}
```

With:
```go
if m.confirm.Active {
    return components.RenderConfirmOverlay(m.confirm.View(), m.width, m.height)
}
```

**Step 4: Update `sources/view.go`**

Replace:
```go
if m.confirmState == ConfirmSetOwnership && m.ownershipPrompt != "" {
    return m.ownershipPrompt
}
if m.confirm.Active {
    return m.confirm.View()
}
```

With:
```go
if m.confirmState == ConfirmSetOwnership && m.ownershipPrompt != "" {
    return components.RenderConfirmOverlay(m.ownershipPrompt, m.width, m.height)
}
if m.confirm.Active {
    return components.RenderConfirmOverlay(m.confirm.View(), m.width, m.height)
}
```

**Step 5: Verify existing tests still pass** (existing confirm tests in sources check for prompt text in View() output — they should still match since the text is preserved):

```bash
go test ./internal/tui/components/... ./internal/tui/pipeline/... ./internal/tui/sources/...
```

**Step 6: Update snapshots if needed**

```bash
make snapshots
```

Check `pipeline-monitor` and `source-manager` snapshots.

**Step 7: Commit**

```bash
git add internal/tui/components/confirm.go internal/tui/pipeline/view.go internal/tui/sources/view.go snapshots/
git commit -m "refactor(tui): add RenderConfirmOverlay for pipeline and sources modal confirms

Pipeline and sources returned confirm.View() as a raw single-line string
with no chrome — a single prompt line on an otherwise blank screen.
RenderConfirmOverlay centers the prompt vertically within the available
height, consistent with how the detail view handles confirmation state."
```
