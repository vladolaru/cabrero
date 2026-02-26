# DX: Help Overlay Scroll — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** The help overlay renders all content as a flat string with no scroll mechanism. On terminals shorter than the total help content, content is silently clipped. Wrap the help overlay in a `viewport.Model` so users can scroll through it.

**Architecture:** `components.RenderHelpOverlay(hc, width, height)` currently returns a plain string. The root replaces `content` with this string when `m.helpOpen`. To add scroll we need the overlay to be stateful — it must hold a viewport that can be scrolled with up/down keys. The simplest approach: add a `helpViewport viewport.Model` to `appModel`, initialize it when help opens, and forward up/down keys to it while help is open.

**Dependency:** None — safe to execute at any point. This is independent of all other plans.

**Snapshot note:** Snapshot views do not capture help overlays. No snapshot updates needed.

---

### Task 1: Add viewport-aware `RenderHelpContent` to components

The existing `RenderHelpOverlay(hc, width, height)` will stay as-is for callers that don't need scroll. We add a separate helper that produces the raw content string (without viewport sizing) for use by the root's viewport.

**Files:**
- Modify: `internal/tui/components/helpoverlay.go`
- Test: `internal/tui/components/helpoverlay_test.go`

**Step 1: Read existing `helpoverlay_test.go`**

```bash
cat internal/tui/components/helpoverlay_test.go
```

Understand what the existing tests cover.

**Step 2: Write a test**

Add to `internal/tui/components/helpoverlay_test.go`:

```go
func TestRenderHelpContent_ContainsAllSections(t *testing.T) {
    hc := shared.HelpContent{
        Description: "This is the description.",
        Sections: []shared.HelpSection{
            {
                Title: "Navigation",
                Entries: []shared.HelpEntry{
                    {Key: "j", Desc: "move down"},
                    {Key: "k", Desc: "move up"},
                },
            },
            {
                Title: "Actions",
                Entries: []shared.HelpEntry{
                    {Key: "a", Desc: "approve"},
                },
            },
        },
    }

    content := RenderHelpContent(hc, 80)
    stripped := ansi.Strip(content)

    if !strings.Contains(stripped, "description") {
        t.Error("content should contain description")
    }
    if !strings.Contains(stripped, "Navigation") {
        t.Error("content should contain Navigation section")
    }
    if !strings.Contains(stripped, "Actions") {
        t.Error("content should contain Actions section")
    }
    if !strings.Contains(stripped, "approve") {
        t.Error("content should contain key descriptions")
    }
}
```

**Step 3: Extract `RenderHelpContent` from `helpoverlay.go`**

Refactor `RenderHelpOverlay` to delegate to a new `RenderHelpContent`:

```go
// RenderHelpContent renders the help content as a scrollable-friendly string
// (no viewport sizing — suitable for use as viewport content).
func RenderHelpContent(hc shared.HelpContent, width int) string {
    var b strings.Builder

    b.WriteString("\n") // top padding

    if hc.Description != "" {
        wrapW := width - 6
        if wrapW < 40 {
            wrapW = 40
        }
        wrapStyle := helpDescStyle.Width(wrapW)
        for _, para := range strings.Split(hc.Description, "\n\n") {
            b.WriteString(wrapStyle.Render(strings.TrimSpace(para)))
            b.WriteString("\n")
        }
        b.WriteString("\n")
    }

    for i, section := range hc.Sections {
        if i > 0 {
            b.WriteString("\n")
        }
        b.WriteString("  ")
        b.WriteString(helpTitleStyle.Render(section.Title))
        b.WriteString("\n")
        for _, entry := range section.Entries {
            line := "  " + helpKeyStyle.Render(entry.Key) + helpEntryDescStyle.Render(entry.Desc)
            b.WriteString(line)
            b.WriteString("\n")
        }
    }

    return b.String()
}

// RenderHelpOverlay is kept for backward compatibility. Use a viewport for
// proper scroll support — see appModel.helpViewport.
func RenderHelpOverlay(hc shared.HelpContent, width, height int) string {
    return RenderHelpContent(hc, width)
}
```

**Step 4: Verify tests pass**

```bash
cd internal/tui/components && go test ./...
```

**Step 5: Commit**

```bash
git add internal/tui/components/helpoverlay.go internal/tui/components/helpoverlay_test.go
git commit -m "refactor(helpoverlay): extract RenderHelpContent for viewport-based scroll"
```

---

### Task 2: Add `helpViewport` to root model and wire scroll

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/integration_test.go` (or new test)

**Step 1: Write a test**

Add to `internal/tui/integration_test.go` (or a new file in the `tui` package):

```go
func TestHelpOverlay_CanScrollDown(t *testing.T) {
    m := buildTestAppModel(t)
    m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

    // Open help overlay.
    m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
    appM, ok := m2.(appModel)
    if !ok {
        t.Fatal("wrong type")
    }
    if !appM.helpOpen {
        t.Fatal("help should be open after '?'")
    }

    initialOffset := appM.helpViewport.YOffset

    // Press Down — viewport should scroll.
    m3, _ := appM.Update(tea.KeyMsg{Type: tea.KeyDown})
    appM2, _ := m3.(appModel)
    if appM2.helpViewport.YOffset == initialOffset && appM2.helpViewport.TotalLineCount() > appM2.helpViewport.Height {
        t.Error("help viewport should scroll down when Down is pressed and content overflows")
    }
}
```

**Step 2: Add `helpViewport` to `appModel`**

In `internal/tui/model.go`, add to `appModel` struct:

```go
helpViewport viewport.Model
```

Add import `"github.com/charmbracelet/bubbles/viewport"` if not already present.

**Step 3: Initialize viewport when help opens**

In `handleGlobalKey`, when `m.keys.Help` is matched:

```go
case key.Matches(msg, m.keys.Help):
    if m.hasActiveTextInput() {
        return m, nil, false
    }
    m.helpOpen = !m.helpOpen
    if m.helpOpen {
        // Build content and initialize viewport.
        viewState := m.state
        if m.state == message.ViewSourceManager && m.sources.DetailOpen() {
            viewState = message.ViewSourceDetail  // or use the new variadic approach from arch-cleanup plan
        }
        hc := shared.HelpForView(viewState, m.keys)
        content := components.RenderHelpContent(hc, m.width)
        m.helpViewport = viewport.New(m.width, m.height-m.headerHeight-3) // -3 for subHeader
        m.helpViewport.SetContent(content)
    }
    return m, nil, true
```

**Step 4: Forward keys to help viewport when help is open**

In `handleGlobalKey`, add before the existing key checks:

```go
// When help is open, Up/Down scroll the viewport; other keys close help or pass through.
if m.helpOpen {
    switch {
    case key.Matches(msg, m.keys.Up):
        m.helpViewport.LineUp(1)
        return m, nil, true
    case key.Matches(msg, m.keys.Down):
        m.helpViewport.LineDown(1)
        return m, nil, true
    case key.Matches(msg, m.keys.HalfPageUp):
        m.helpViewport.HalfViewUp()
        return m, nil, true
    case key.Matches(msg, m.keys.HalfPageDown):
        m.helpViewport.HalfViewDown()
        return m, nil, true
    case key.Matches(msg, m.keys.GotoTop):
        m.helpViewport.GotoTop()
        return m, nil, true
    case key.Matches(msg, m.keys.GotoBottom):
        m.helpViewport.GotoBottom()
        return m, nil, true
    }
}
```

**Step 5: Update `View()` to use viewport**

Replace the current help overlay rendering in `View()`:

```go
// Before:
if m.helpOpen {
    viewState := m.state
    if m.state == message.ViewSourceManager && m.sources.DetailOpen() {
        viewState = message.ViewSourceDetail
    }
    hc := shared.HelpForView(viewState, m.keys)
    content = components.RenderHelpOverlay(hc, m.width, m.height)
}

// After:
if m.helpOpen {
    content = m.helpViewport.View()
}
```

**Step 6: Handle window resize while help is open**

In the `tea.WindowSizeMsg` handler in `Update()`, re-initialize the viewport if help is open:

```go
case tea.WindowSizeMsg:
    m.width = msg.Width
    m.height = msg.Height
    // ... existing header/subheader height recalculation ...
    if m.helpOpen {
        m.helpViewport.Width = m.width
        m.helpViewport.Height = m.height - m.headerHeight - 3
    }
```

**Step 7: Build and test**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

**Step 8: Manual smoke test**

```bash
make install && cabrero
```

1. Press `?` to open help overlay.
2. Confirm content is visible.
3. Press `↓` / `j` — confirm viewport scrolls.
4. Press `↑` / `k` — confirm it scrolls back.
5. Press `?` again — confirm help closes.
6. Resize terminal while help is open — confirm no crash and content reflows.

**Step 9: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): add scrollable viewport to help overlay

The help overlay rendered all content as a flat string. On terminals
shorter than the content, entries were silently clipped. A viewport.Model
is initialized when help opens, sized to the available content area.
Up/Down/HalfPage/GotoTop/GotoBottom keys scroll the viewport while help
is open."
```
