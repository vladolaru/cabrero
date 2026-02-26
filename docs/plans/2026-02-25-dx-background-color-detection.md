# DX: Proper Background Color Detection — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace `compat.AdaptiveColor` with BubbleTea v2's async `tea.RequestBackgroundColor` / `tea.BackgroundColorMsg`. Drop the `compat` package from the project entirely.

**Architecture:**

Single-path initialization — no blocking I/O, no sync seed:

1. `tui.go` calls `shared.InitStyles(true)` before creating the model. Dark is assumed as the starting default — statistically correct for CLI/TUI users, and any mismatch corrects within the first frames anyway.
2. `appModel.Init()` returns `tea.RequestBackgroundColor`. The terminal responds asynchronously with `tea.BackgroundColorMsg`.
3. `appModel.Update()` handles `tea.BackgroundColorMsg` → sets `m.isDark`, calls `shared.ReinitStyles(isDark)` to rebuild all style vars with the correct value.

`shared.InitStyles(isDark bool)` and `shared.ReinitStyles(isDark bool)` use `lipgloss.LightDark(isDark)` to create explicit `color.Color` values — no adaptive types, no compat. The package-level `shared.IsDark bool` replaces `compat.HasDarkBackground` everywhere. After this plan, `compat` has zero imports across the project.

BubbleTea's Update() loop is single-threaded, so reinitializing package-level vars from `Update()` is safe.

**Dependencies:**
- `charm-v2-upgrade` complete (`tea.RequestBackgroundColor`, `tea.BackgroundColorMsg` available)
- `dx-style-consolidation` complete (all `Color*` and `*Style` vars consolidated in `shared/styles.go`; all views use `shared.*Style` directly)

**Slot:** Step 7 — after `bubbles-list-dashboard`, last in the DX sequence. Placed last because it forces `shared.InitStyles(true)` into every test setup, including the dashboard tests added by `bubbles-list-dashboard`.

---

### Task 1: Rewrite `shared/styles.go` — `InitStyles`, `ReinitStyles`, no compat

**Files:**
- Modify: `internal/tui/shared/styles.go`
- Create: `internal/tui/shared/styles_test.go`

**Step 1: Write failing tests**

```go
package shared

import (
    "testing"

    "charm.land/lipgloss/v2"
)

func TestInitStyles_SetsIsDark(t *testing.T) {
    InitStyles(true)
    if !IsDark {
        t.Error("IsDark should be true after InitStyles(true)")
    }
    InitStyles(false)
    if IsDark {
        t.Error("IsDark should be false after InitStyles(false)")
    }
}

func TestInitStyles_StylesRenderable(t *testing.T) {
    // Styles must be non-zero after InitStyles — Render must not panic.
    InitStyles(true)
    _ = SuccessStyle.Render("ok")
    _ = ErrorStyle.Render("err")
    _ = AccentBoldStyle.Render("section")
    _ = MutedStyle.Render("muted")
}

func TestReinitStyles_ChangesOutput(t *testing.T) {
    InitStyles(false)
    lightOut := SuccessStyle.Render("x")

    ReinitStyles(true)
    darkOut := SuccessStyle.Render("x")

    if lightOut == darkOut {
        t.Error("ReinitStyles should change ANSI output when isDark changes")
    }
}

func TestHighlightBg_DependsOnIsDark(t *testing.T) {
    InitStyles(false)
    light := HighlightBg()

    InitStyles(true)
    dark := HighlightBg()

    if light == dark {
        t.Error("HighlightBg should differ between light and dark")
    }
}
```

**Step 2: Verify tests fail**

```bash
cd internal/tui/shared && go test -run "TestInitStyles|TestReinitStyles|TestHighlight" -v
```

Expected: FAIL — `InitStyles`, `ReinitStyles`, `IsDark` undefined.

**Step 3: Rewrite `shared/styles.go`**

No `compat` import. No `init()` function. The package vars are zero until `InitStyles` is called explicitly from `tui.go`.

```go
package shared

import (
    "image/color"

    "charm.land/lipgloss/v2"
)

// IsDark reports whether the terminal has a dark background.
// Set by InitStyles; updated by ReinitStyles when BackgroundColorMsg arrives.
var IsDark bool

// Color palette — concrete color.Color values set by InitStyles/ReinitStyles.
var (
    ColorSuccess     color.Color
    ColorError       color.Color
    ColorWarning     color.Color
    ColorAccent      color.Color
    ColorMuted       color.Color
    ColorChat        color.Color
    ColorFgBold      color.Color
    ColorBorder      color.Color
    ColorHighlightFg color.Color
    ColorHighlightBg color.Color
)

// Reusable lipgloss styles — rebuilt by InitStyles/ReinitStyles.
var (
    HeaderStyle     lipgloss.Style
    MutedStyle      lipgloss.Style
    SuccessStyle    lipgloss.Style
    ErrorStyle      lipgloss.Style
    WarningStyle    lipgloss.Style
    AccentStyle     lipgloss.Style
    SelectedStyle   lipgloss.Style
    AccentBoldStyle lipgloss.Style
)

// InitStyles sets all color and style vars for the given background.
// Call once from tui.go before tea.NewProgram. Dark (true) is the assumed
// default; BackgroundColorMsg will update to the correct value within the
// first frames.
func InitStyles(isDark bool) {
    IsDark = isDark
    ld := lipgloss.LightDark(isDark)

    ColorSuccess     = ld(lipgloss.Color("#2E7D32"), lipgloss.Color("#66BB6A"))
    ColorError       = ld(lipgloss.Color("#C62828"), lipgloss.Color("#EF5350"))
    ColorWarning     = ld(lipgloss.Color("#E65100"), lipgloss.Color("#FFA726"))
    ColorAccent      = ld(lipgloss.Color("#6A1B9A"), lipgloss.Color("#CE93D8"))
    ColorMuted       = ld(lipgloss.Color("#757575"), lipgloss.Color("#9E9E9E"))
    ColorChat        = ld(lipgloss.Color("#00695C"), lipgloss.Color("#4DB6AC"))
    ColorFgBold      = ld(lipgloss.Color("#000000"), lipgloss.Color("#FFFFFF"))
    ColorBorder      = ld(lipgloss.Color("#BDBDBD"), lipgloss.Color("#616161"))
    ColorHighlightFg = ld(lipgloss.Color("#FFFFFF"), lipgloss.Color("#FFFFFF"))
    ColorHighlightBg = ld(lipgloss.Color("#6A1B9A"), lipgloss.Color("#9C27B0"))

    HeaderStyle     = lipgloss.NewStyle().Bold(true)
    MutedStyle      = lipgloss.NewStyle().Foreground(ColorMuted)
    SuccessStyle    = lipgloss.NewStyle().Foreground(ColorSuccess)
    ErrorStyle      = lipgloss.NewStyle().Foreground(ColorError)
    WarningStyle    = lipgloss.NewStyle().Foreground(ColorWarning)
    AccentStyle     = lipgloss.NewStyle().Foreground(ColorAccent)
    SelectedStyle   = lipgloss.NewStyle().Bold(true).Foreground(ColorFgBold)
    AccentBoldStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
}

// ReinitStyles rebuilds all styles for the updated background. Call from
// appModel.Update() when tea.BackgroundColorMsg arrives.
func ReinitStyles(isDark bool) {
    InitStyles(isDark)
}

// HighlightFg returns the foreground hex for search match highlighting.
func HighlightFg() string {
    return "#FFFFFF" // same on both backgrounds
}

// HighlightBg returns the background hex for search match highlighting.
func HighlightBg() string {
    if IsDark {
        return "#9C27B0"
    }
    return "#6A1B9A"
}
```

**Step 4: Run tests**

```bash
cd internal/tui/shared && go test ./... -v
```

Expected: all pass. Tests call `InitStyles` explicitly so zero-value vars are not a problem.

**Step 5: Build TUI packages**

```bash
go build ./internal/tui/...
```

Watch for: any package that used `shared.ColorX` as a `compat.AdaptiveColor` (type assertion or `.Dark`/`.Light` field access). These were already eliminated in `charm-v2-upgrade` (Task 3) and `dx-architecture-cleanup` (logview helpers); if any survive, fix them to use `shared.IsDark` directly.

**Step 6: Commit**

```bash
git add internal/tui/shared/styles.go internal/tui/shared/styles_test.go
git commit -m "feat(shared): InitStyles/ReinitStyles with lipgloss.LightDark — drops compat"
```

---

### Task 2: Wire `appModel` — `Init()`, `isDark`, `BackgroundColorMsg`

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/tui.go`
- Test: `internal/tui/` (new test file or add to integration_test.go)

**Step 1: Write failing tests**

```go
package tui

import (
    "image/color"
    "testing"

    tea "charm.land/bubbletea/v2"

    "github.com/vladolaru/cabrero/internal/tui/shared"
)

func TestAppModel_Init_RequestsBackgroundColor(t *testing.T) {
    shared.InitStyles(true)
    m := buildTestAppModel(t)
    cmd := m.Init()
    if cmd == nil {
        t.Fatal("Init() must return tea.RequestBackgroundColor cmd, got nil")
    }
}

func TestAppModel_BackgroundColorMsg_UpdatesStyles(t *testing.T) {
    shared.InitStyles(true) // start dark
    m := buildTestAppModel(t)
    m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

    // Simulate a light terminal responding.
    lightColor := color.RGBA{R: 240, G: 240, B: 240, A: 255}
    m2, _ := m.Update(tea.BackgroundColorMsg{Color: lightColor})
    appM, ok := m2.(appModel)
    if !ok {
        t.Fatal("Update returned wrong type")
    }
    if appM.isDark {
        t.Error("isDark should be false after light BackgroundColorMsg")
    }
    if shared.IsDark {
        t.Error("shared.IsDark should be false after light BackgroundColorMsg")
    }
}
```

**Step 2: Verify tests fail**

```bash
cd internal/tui && go test -run "TestAppModel_Init|TestAppModel_Background" -v
```

Expected: FAIL — `Init()` returns `nil`, `isDark` field missing.

**Step 3: Add `isDark bool` to `appModel` in `model.go`**

```go
type appModel struct {
    // ... existing fields ...
    isDark bool
}
```

**Step 4: Update `Init()` in `model.go`**

```go
func (m appModel) Init() tea.Cmd {
    return tea.RequestBackgroundColor
}
```

**Step 5: Handle `tea.BackgroundColorMsg` in `appModel.Update()`**

Add before the `tea.WindowSizeMsg` case:

```go
case tea.BackgroundColorMsg:
    m.isDark = msg.IsDark()
    shared.ReinitStyles(m.isDark)
    return m, nil
```

**Step 6: Update `tui.go` — call `InitStyles(true)`, remove compat**

```go
func Run(version string) error {
    cfg, err := LoadConfig()
    // ...

    // Assume dark background as starting default.
    // BackgroundColorMsg will correct this within the first frames.
    shared.InitStyles(true)

    // ... rest of setup ...
    m := newAppModel(proposals, reports, stats, sourceGroups, runs, pipelineStats, prompts, cfg)
    p := tea.NewProgram(m)
    // ...
}
```

`newAppModel` does not need an `isDark` parameter — it sets `isDark: true` inline to match the `InitStyles(true)` call above:

```go
func newAppModel(...) appModel {
    m := appModel{
        // ...
        isDark: true, // matches shared.InitStyles(true) in tui.go; BackgroundColorMsg will update
    }
    return m
}
```

Remove any `compat` import from `tui.go`.

**Step 7: Run tests**

```bash
go test ./internal/tui/...
```

Expected: all pass.

**Step 8: Commit**

```bash
git add internal/tui/model.go internal/tui/tui.go
git commit -m "feat(tui): wire BackgroundColorMsg for async dark/light detection

appModel.Init() returns tea.RequestBackgroundColor. BackgroundColorMsg in
Update() calls shared.ReinitStyles() to rebuild styles with the correctly
detected value. tui.go seeds with InitStyles(true) — dark assumed as
default. No blocking I/O. No compat dependency."
```

---

### Task 3: Update logview color helpers to use `shared.IsDark`

The logview `mutedColor()`, `errorColor()`, `accentColor()` helpers were patched in `charm-v2-upgrade` to check `compat.HasDarkBackground`. Replace with `shared.IsDark` so they update when `BackgroundColorMsg` arrives.

**Files:**
- Modify: `internal/tui/logview/model.go`

**Step 1: Update the three helpers**

```go
func mutedColor() string {
    if shared.IsDark {
        return "#9E9E9E"
    }
    return "#757575"
}

func errorColor() string {
    if shared.IsDark {
        return "#EF5350"
    }
    return "#C62828"
}

func accentColor() string {
    if shared.IsDark {
        return "#CE93D8"
    }
    return "#6A1B9A"
}
```

Remove `"charm.land/lipgloss/v2/compat"` from the imports.

**Step 2: Build and test**

```bash
go build ./internal/tui/logview/ && go test ./internal/tui/logview/...
```

**Step 3: Commit**

```bash
git add internal/tui/logview/model.go
git commit -m "refactor(logview): use shared.IsDark instead of compat"
```

---

### Task 4: Remove `compat` from `go.mod`

`compat` should now have zero imports across the entire project. Remove it from the module.

**Step 1: Verify zero compat imports**

```bash
grep -rn "compat" . --include="*.go"
```

Expected: zero matches. If any remain, fix them before proceeding.

**Step 2: Remove from `go.mod`**

```bash
go mod tidy
```

This removes `charm.land/lipgloss/v2/compat` (and any other now-unused transitive deps) from `go.mod` and `go.sum`.

**Step 3: Verify it's gone**

```bash
grep "compat" go.mod go.sum
```

Expected: zero matches.

**Step 4: Full build and test**

```bash
go build ./... && go test ./...
```

**Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): remove compat package — fully replaced by shared.InitStyles"
```

---

### Task 5: Fix tests that need explicit `InitStyles` setup

Now that `shared/styles.go` has no `init()` seed, any test that uses shared styles without going through `tui.Run` → `newAppModel` will hit zero-value styles.

**Step 1: Find affected tests**

```bash
go test ./internal/tui/... 2>&1 | grep -E "FAIL|panic"
```

Common patterns that need fixing: component tests, view render tests, anything calling `View()` directly. **Specifically check `internal/tui/dashboard/` — the bubbles-list-dashboard plan added new tests that use shared styles and will need `InitStyles(true)`.**

**Step 2: Add `shared.InitStyles(true)` to each affected test's setup**

In packages that have a `testmain_test.go`:

```go
// testmain_test.go
func TestMain(m *testing.M) {
    shared.InitStyles(true) // seed styles for all tests in this package
    os.Exit(m.Run())
}
```

In packages without `TestMain`, add to the relevant test helper or each failing test:

```go
func TestFoo(t *testing.T) {
    shared.InitStyles(true)
    // ...
}
```

**Step 3: Full test suite — expect clean pass**

```bash
go test ./...
```

**Step 4: Commit**

```bash
git add ./internal/tui/
git commit -m "test(tui): add shared.InitStyles(true) to test setup — required after removing compat init seed"
```

---

### Task 6: Smoke test

```bash
make install && cabrero
```

**On a dark terminal:**
1. Launch — colors should be dark palette immediately (muted gray `#9E9E9E`, accent `#CE93D8`)
2. No visible flicker (dark assumed → dark confirmed)

**On a light terminal:**
1. Launch — first frame uses dark palette (imperceptible, sub-50ms)
2. Immediately corrects to light palette (muted gray `#757575`, accent `#6A1B9A`)
3. Help overlay, status bar, column headers all render with light colors

**Regenerate snapshots if output changed:**

```bash
make snapshots
git add snapshots/
git commit -m "chore(snapshots): update for compat-free background detection"
```

---

## Updated execution order

```
1.  charm-v2-upgrade
    (all bug plans done)
2.  dx-unified-utilities
3.  dx-style-consolidation
4.  dx-layout-patterns
5.  dx-architecture-cleanup
6.  bubbles-list-dashboard
7.  dx-background-color-detection   ← this plan (fully drops compat; placed last
                                        so it covers all test files including
                                        dashboard tests from step 6)
8.  dx-help-overlay-scroll
```
