# DX: Architecture Cleanup — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Three self-contained structural fixes: (1) move `dashboard.RenderHeader` to `components` where it belongs since it's called by the root; (2) remove the `ViewSourceDetail` enum value which has no dedicated model; (3) fix `logview`'s manual adaptive color helpers that bypass `lipgloss.AdaptiveColor`.

**Architecture:** Each task is an independent refactor with no behavioral change. All three can be done in any order.

**Dependency:** Run after `dx-unified-utilities` (Task 1 needs `shared.Checkmark` and `shared.RelativeTime`) and after `dx-style-consolidation` (Task 3 replaces logview termenv helpers with `shared.*Style` — those styles must be consolidated first). Tasks 2 and 3 do not need `shared.IsDark`; that is added later by `dx-background-color-detection`.

---

### Task 1: Move `RenderHeader` from `dashboard` to `components`

`dashboard.RenderHeader` is called by `internal/tui/model.go` (the root), not by the dashboard itself. The dashboard package is imported by the root purely for this function. It belongs in `components` alongside the other stateless render helpers.

**Files:**
- Modify: `internal/tui/dashboard/view.go` (remove function)
- Create or modify: `internal/tui/components/header.go` (new file for the function)
- Modify: `internal/tui/model.go` (update import and call site)
- Test: `internal/tui/components/` (new test)

**Step 1: Write a test for the moved function**

Create `internal/tui/components/header_test.go`:

```go
package components

import (
    "strings"
    "testing"

    "github.com/charmbracelet/x/ansi"

    "github.com/vladolaru/cabrero/internal/tui/message"
    "github.com/vladolaru/cabrero/internal/tui/shared"
)

func TestRenderHeader_ContainsTitleAndDaemonStatus(t *testing.T) {
    stats := message.DashboardStats{
        Version:       "v0.19.0",
        DaemonRunning: true,
        DaemonPID:     12345,
        HookPreCompact: true,
        HookSessionEnd: false,
    }

    header := RenderHeader(stats, 120)
    stripped := ansi.Strip(header)

    if !strings.Contains(stripped, "Cabrero") {
        t.Error("header should contain application name")
    }
    if !strings.Contains(stripped, "v0.19.0") {
        t.Error("header should contain version")
    }
    if !strings.Contains(stripped, "running") {
        t.Error("header should show daemon running status")
    }
    if !strings.Contains(stripped, "12345") {
        t.Error("header should show daemon PID")
    }
    _ = shared.MutedStyle // ensure shared is used
}

func TestRenderHeader_StoppedDaemon(t *testing.T) {
    stats := message.DashboardStats{DaemonRunning: false}
    header := RenderHeader(stats, 80)
    stripped := ansi.Strip(header)
    if !strings.Contains(stripped, "stopped") {
        t.Error("header should show daemon stopped status")
    }
}

func TestRenderHeader_NarrowLayout(t *testing.T) {
    stats := message.DashboardStats{DaemonRunning: true, DaemonPID: 1}
    // Width < 120 should use stacked layout (no horizontal join).
    header := RenderHeader(stats, 80)
    if header == "" {
        t.Error("RenderHeader should return non-empty output")
    }
}
```

**Step 2: Verify test fails**

```bash
cd internal/tui/components && go test -run TestRenderHeader -v
```

Expected: FAIL — `RenderHeader` not yet defined in `components`.

**Step 3: Create `internal/tui/components/header.go`**

Copy the `RenderHeader` function body verbatim from `internal/tui/dashboard/view.go`, updating the package and any local style references:

```go
package components

import (
    "fmt"
    "strings"

    "github.com/charmbracelet/lipgloss"

    "github.com/vladolaru/cabrero/internal/tui/message"
    "github.com/vladolaru/cabrero/internal/tui/shared"
)

// RenderHeader renders the persistent application header bar shown above every view.
// It is called by the root model, not by the dashboard view.
func RenderHeader(stats message.DashboardStats, width int) string {
    titleText := "  Cabrero"
    if stats.Version != "" {
        titleText += "  " + shared.MutedStyle.Render(stats.Version)
    }
    tagline := shared.MutedStyle.Render("  Shepherding AI pirate goats, one skill at a time")
    title := shared.HeaderStyle.Render(titleText) + "\n" + tagline

    var daemonStatus string
    if stats.DaemonRunning {
        daemonStatus = shared.SuccessStyle.Render("●") + fmt.Sprintf(" running (PID %d)", stats.DaemonPID)
    } else {
        daemonStatus = shared.ErrorStyle.Render("●") + " stopped"
    }

    var lastCapture string
    if stats.LastCaptureTime != nil {
        lastCapture = shared.MutedStyle.Render("Last capture:") + " " + shared.RelativeTime(*stats.LastCaptureTime)
    }

    hookPre := shared.Checkmark(stats.HookPreCompact)
    hookEnd := shared.Checkmark(stats.HookSessionEnd)
    hooks := shared.MutedStyle.Render("Hooks:") + fmt.Sprintf(" pre-compact %s  session-end %s", hookPre, hookEnd)

    debugIndicator := ""
    if stats.DebugMode {
        debugIndicator = "  " + shared.MutedStyle.Render("│  Debug:") + " " + shared.WarningStyle.Render("enabled")
    }

    if width >= 120 {
        left := title
        rightLines := []string{
            shared.MutedStyle.Render("Daemon:") + " " + daemonStatus,
            lastCapture,
            hooks + debugIndicator,
        }
        return lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", strings.Join(rightLines, "\n"))
    }

    daemonLine := "  " + shared.MutedStyle.Render("Daemon:") + " " + daemonStatus
    if lastCapture != "" {
        daemonLine += "  " + shared.MutedStyle.Render("│") + "  " + lastCapture
    }
    return title + "\n" +
        daemonLine + "\n" +
        "  " + hooks + debugIndicator
}
```

Note: `checkMark` calls replaced with `shared.Checkmark` (from the unified utilities plan). If that plan hasn't run yet, inline the equivalent or use the local `checkMark` function temporarily.

**Step 4: Verify test passes**

```bash
cd internal/tui/components && go test -run TestRenderHeader -v
```

**Step 5: Update `internal/tui/model.go`**

Replace the import:
```go
"github.com/vladolaru/cabrero/internal/tui/dashboard"
```
with (if dashboard is no longer needed):
```go
// remove dashboard import
```
or keep it if dashboard is still imported for other reasons.

Update both call sites in `model.go`:
```go
// Before:
header := dashboard.RenderHeader(m.stats, m.width)

// After:
header := components.RenderHeader(m.stats, m.width)
```

**Step 6: Remove `RenderHeader` from `dashboard/view.go`**

Delete the function. Also remove the `timeAgo` call inside it (now handled by `shared.RelativeTime` in the new `components/header.go`) and the old local `checkMark` call.

**Step 7: Build and test**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

**Step 8: Commit**

```bash
git add internal/tui/components/header.go internal/tui/components/header_test.go internal/tui/dashboard/view.go internal/tui/model.go
git commit -m "refactor(tui): move RenderHeader from dashboard to components

RenderHeader is called by the root model for every view, not by the
dashboard. Having it in the dashboard package created an import of
dashboard just for one function. It now lives in components alongside
other stateless render helpers."
```

---

### Task 2: Remove `ViewSourceDetail` from `message.ViewState`

`ViewSourceDetail` is a ViewState enum value with no dedicated model. It is used only to select different help overlay content. The sources model already tracks `detailOpen bool` internally.

**Files:**
- Modify: `internal/tui/message/message.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/shared/helpdata.go`
- Test: (build verification — no behavioral change)

**Step 1: Check all usages**

```bash
grep -rn "ViewSourceDetail" internal/tui/
```

Expected locations: `message.go` (declaration), `model.go` (render switch + pushView/switchView), `helpdata.go` (help content), possibly tests.

**Step 2: Remove from `message/message.go`**

Delete:
```go
ViewSourceDetail
```

from the `const` block. Verify remaining constants are renumbered if iota-based — since Go iota assigns values in order, removing a middle value shifts everything after it. Check if any code compares ViewState values as integers (it shouldn't — they should only be compared via `==`). Since `ViewSourceDetail` follows `ViewSourceManager`, the remaining values after removal are: `ViewDashboard=0, ViewProposalDetail=1, ViewFitnessDetail=2, ViewSourceManager=3, ViewPipelineMonitor=4, ViewLogViewer=5`. Any persisted state (e.g. in config) that stores integers would break, but ViewState is not persisted — it is ephemeral.

**Step 3: Update `model.go`**

Remove `ViewSourceDetail` from the render switch:
```go
// Before:
case message.ViewSourceManager, message.ViewSourceDetail:
    content = m.sources.View()

// After:
case message.ViewSourceManager:
    content = m.sources.View()
```

Remove from the child routing switch (same change).

Remove from `subHeader()`:
```go
// Before:
case message.ViewSourceManager, message.ViewSourceDetail:
    return m.sources.SubHeader()

// After:
case message.ViewSourceManager:
    return m.sources.SubHeader()
```

**Step 4: Update `shared/helpdata.go`**

In `HelpForView()`, replace the `ViewSourceDetail` case — use `m.sources.DetailOpen()` logic in the caller instead. Since `HelpForView` doesn't have access to the sources model, the caller must pass the right viewState.

In `model.go`, update the help overlay logic:
```go
// Before:
viewState := m.state
if m.state == message.ViewSourceManager && m.sources.DetailOpen() {
    viewState = message.ViewSourceDetail
}
hc := shared.HelpForView(viewState, m.keys)

// After:
viewState := m.state
hc := shared.HelpForView(viewState, m.keys, m.sources.DetailOpen())
```

Update `shared.HelpForView` signature:
```go
// Before:
func HelpForView(view message.ViewState, km KeyMap) HelpContent

// After:
func HelpForView(view message.ViewState, km KeyMap, sourceDetailOpen ...bool) HelpContent
```

In the function body, for the `ViewSourceManager` case:
```go
case message.ViewSourceManager:
    if len(sourceDetailOpen) > 0 && sourceDetailOpen[0] {
        return HelpContent{ /* source detail help content */ }
    }
    return HelpContent{ /* source manager help content */ }
```

Move the existing `ViewSourceDetail` case content into this conditional.

Remove the now-unused `ViewSourceDetail` case from `HelpForView`.

**Step 5: Build**

```bash
go build ./internal/tui/...
```

If any test references `message.ViewSourceDetail` directly, update it to use `message.ViewSourceManager` and simulate detail-open state via the sources model.

**Step 6: Commit**

```bash
git add internal/tui/message/message.go internal/tui/model.go internal/tui/shared/helpdata.go
git commit -m "refactor(tui): remove ViewSourceDetail ViewState — use sources.DetailOpen() instead

ViewSourceDetail had no dedicated model; it was used only to select help
overlay content. The sources model already tracks detailOpen internally.
HelpForView now takes a variadic sourceDetailOpen bool to select the
correct content when ViewSourceManager is active."
```

---

### Task 3: Fix `logview` adaptive color helpers

`logview/model.go` has three private functions (`mutedColor()`, `errorColor()`, `accentColor()`) that manually inspect `shared.ColorMuted.Dark` / `.Light` fields. This bypasses `lipgloss.AdaptiveColor` resolution and is fragile. The non-highlighting rendering (timestamp, level badge) should use standard lipgloss styles.

**Files:**
- Modify: `internal/tui/logview/model.go`
- Test: (build + existing tests)

**Background:** The reason `termenv` is used in logview is that search highlighting needs to apply color to already-rendered ANSI strings (`ansi.Strip` + re-color). This is legitimate and stays. But the level badge and timestamp coloring in `renderEntries` can use lipgloss instead of termenv.

> **Note:** Use `shared.MutedStyle`, `shared.ErrorStyle`, `shared.AccentStyle` directly — do NOT use `shared.IsDark`. That field is added by `dx-background-color-detection` which runs later. The lipgloss styles themselves already encode the correct color and will update when `ReinitStyles` is called.

**Step 1: Read `renderEntries` to understand what uses termenv**

```bash
grep -n "highlightOutput" internal/tui/logview/model.go
```

Identify which uses are for highlighting (legitimate) vs basic coloring (fixable).

**Step 2: Update `renderLevel` to use shared styles directly**

Replace the `termenv`-based `renderLevel`. Use `shared.*Style` inline rather than declaring local package-level style vars — local vars would capture the color at init time and miss `ReinitStyles` updates:

```go
func renderLevel(level string) string {
    switch level {
    case "ERROR":
        return shared.ErrorStyle.Render(level)
    default:
        return shared.AccentStyle.Render(level)
    }
}
```

**Step 3: Update `renderEntries` — timestamp and expand indicator**

Replace:
```go
ts = highlightOutput.String(entry.Timestamp).
    Foreground(highlightOutput.Color(mutedColor())).String() + "  "
```
With:
```go
ts = shared.MutedStyle.Render(entry.Timestamp) + "  "
```

Replace the expand/collapse indicator rendering:
```go
// Before:
b.WriteString(highlightOutput.String("[-]").
    Foreground(highlightOutput.Color(mutedColor())).String())
// ...
b.WriteString(highlightOutput.String(indicator).
    Foreground(highlightOutput.Color(mutedColor())).String())

// After:
b.WriteString(shared.MutedStyle.Render("[-]"))
// ...
b.WriteString(shared.MutedStyle.Render(indicator))
```

**Step 5: Remove the now-unused color helper functions**

Delete `mutedColor()`, `errorColor()`, `accentColor()` from `logview/model.go`.

The `highlightOutput` variable stays — it is still needed for `applySearchHighlights` and `highlightedContent`.

**Step 6: Build and test**

```bash
go build ./internal/tui/logview/ && go test ./internal/tui/logview/...
```

**Step 7: Commit**

```bash
git add internal/tui/logview/model.go
git commit -m "refactor(logview): use shared.*Style for timestamp/level coloring

The mutedColor()/errorColor()/accentColor() helpers hardcoded hex values
and duplicated color knowledge already in shared/styles.go. Non-highlighting
rendering (timestamps, level badges, expand indicators) now delegates to
shared.MutedStyle, shared.ErrorStyle, shared.AccentStyle — automatically
updated by ReinitStyles when BackgroundColorMsg arrives. The termenv
highlightOutput variable remains for search match highlighting."
```
