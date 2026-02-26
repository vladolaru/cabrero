# Charm v2 Upgrade — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Upgrade to BubbleTea v2, Bubbles v2, and Lipgloss v2. All three released on 2026-02-24 with new import paths under `charm.land`.

**Architecture:** This is a prerequisite for all other plans. The upgrade is mostly mechanical (import paths, API renames) with two substantive changes: `View()` now returns `tea.View` instead of `string` (root model only), and `lipgloss.AdaptiveColor` is replaced by `compat.AdaptiveColor` as a drop-in. The recommended path for dark/light detection uses BubbleTea's `BackgroundColorMsg` — this plan uses the `compat` package as an immediate upgrade, and a follow-up task adds proper detection.

**New import paths:**
```
github.com/charmbracelet/bubbletea  →  charm.land/bubbletea/v2
github.com/charmbracelet/bubbles/*  →  charm.land/bubbles/v2/*
github.com/charmbracelet/lipgloss   →  charm.land/lipgloss/v2
```

**Must run before:** every other plan. All subsequent plans are written against v2 APIs.

---

## Impact on Existing Plans

| Plan | Impact |
|------|--------|
| `bug-statusmsg-silent-drop` | `tea.KeyMsg` → `tea.KeyPressMsg` in new Update() handlers |
| `bug-sources-pipeline-viewport` | **Already implemented** — viewport code in sources/pipeline uses v1 API; Task 6 below covers those files |
| `bug-dashboard-filter` | Obsoleted by bubbles-list plan — no impact |
| `dx-unified-utilities` | `categoryColor()` return type: `lipgloss.TerminalColor` → `color.Color` |
| `dx-style-consolidation` | Must run after upgrade; `shared.Color*` now `compat.AdaptiveColor` |
| `dx-layout-patterns` | No direct impact |
| `dx-architecture-cleanup` | Logview `.Dark`/`.Light` field access changes — `compat.AdaptiveColor` has them as `color.Color` not strings; update the hardcoded hex strings in the helpers |
| `dx-help-overlay-scroll` | `viewport.New(WithWidth, WithHeight)` in the new viewport |
| `bubbles-list-dashboard` | `list.DefaultStyles(isDark bool)` — use `compat.HasDarkBackground`; `tea.KeyPressMsg` in Update |

---

## What Does NOT Change

- `key.Matches(msg, binding)` — still works; accepts `tea.KeyPressMsg` since it implements `tea.KeyMsg` (now an interface)
- Child model `View() string` — only the root `appModel` implements `tea.Model`; child views keep `string` return type
- `tea.WindowSizeMsg` — unchanged
- `tea.Batch`, `tea.Tick`, `tea.Quit` — unchanged
- `components.RenderStatusBar`, all lipgloss `Style.Render()` — unchanged
- `key.Binding`, `key.NewBinding` — unchanged

---

### Task 1: Update dependencies in `go.mod`

**Files:**
- Modify: `go.mod`, `go.sum`

**Step 1: Install v2 packages**

```bash
go get charm.land/bubbletea/v2@latest
go get charm.land/bubbles/v2@latest
go get charm.land/lipgloss/v2@latest
```

**Step 2: Verify the new module paths appear in `go.mod`**

```bash
grep "charm.land" go.mod
```

Expected: three entries for `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`.

**Step 3: Remove old v1 entries**

```bash
go mod tidy
```

This removes `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles`, and `github.com/charmbracelet/lipgloss` from `go.mod` and `go.sum`.

**Step 4: Verify**

```bash
grep "charmbracelet/bubbletea\|charmbracelet/bubbles\|charmbracelet/lipgloss" go.mod
```

Expected: zero matches (old paths fully replaced).

**Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): upgrade to BubbleTea v2, Bubbles v2, Lipgloss v2"
```

---

### Task 2: Replace import paths (mechanical)

Every `.go` file under `internal/tui/` and any other file that imports the charm packages needs its imports updated.

**Files:** All `*.go` files in `internal/tui/` (and `internal/apply/`, `cmd/` if they import lipgloss/bubbletea).

**Step 1: Search for all files with old imports**

```bash
grep -rl "github.com/charmbracelet/bubbletea\|github.com/charmbracelet/bubbles\|github.com/charmbracelet/lipgloss" . --include="*.go"
```

**Step 2: Replace import paths**

```bash
# BubbleTea
find . -name "*.go" -exec sed -i '' \
  's|"github.com/charmbracelet/bubbletea"|"charm.land/bubbletea/v2"|g' {} \;

# Bubbles sub-packages
find . -name "*.go" -exec sed -i '' \
  's|"github.com/charmbracelet/bubbles/|"charm.land/bubbles/v2/|g' {} \;

# Bubbles root (for any direct import)
find . -name "*.go" -exec sed -i '' \
  's|"github.com/charmbracelet/bubbles"|"charm.land/bubbles/v2"|g' {} \;

# Lipgloss
find . -name "*.go" -exec sed -i '' \
  's|"github.com/charmbracelet/lipgloss"|"charm.land/lipgloss/v2"|g' {} \;
```

**Step 3: Verify no old paths remain**

```bash
grep -r "github.com/charmbracelet/bubbletea\|github.com/charmbracelet/bubbles\|github.com/charmbracelet/lipgloss" . --include="*.go"
```

Expected: zero matches.

**Step 4: Attempt build (will fail — that's expected at this stage)**

```bash
go build ./... 2>&1 | head -30
```

Note which errors are import-path errors (resolved) vs API change errors (resolved in subsequent tasks).

**Step 5: Commit**

```bash
git add -A
git commit -m "chore(tui): update charm import paths to charm.land v2"
```

---

### Task 3: Fix Lipgloss colors — `AdaptiveColor` → `compat.AdaptiveColor`

`lipgloss.AdaptiveColor` is removed in v2. The `compat` package provides a drop-in replacement that preserves v1 behavior (reads `stdin`/`stdout` at init).

**Files:**
- Modify: `internal/tui/shared/styles.go`
- Modify: `internal/tui/components/assessbar.go`
- Modify: `internal/tui/logview/model.go`

**Step 1: Update `shared/styles.go`**

Add import `"charm.land/lipgloss/v2/compat"`. Replace all `lipgloss.AdaptiveColor{...}` with `compat.AdaptiveColor{...}`. The field names `Light` and `Dark` stay the same but now hold `color.Color` values (via `lipgloss.Color()`):

```go
import (
    "charm.land/lipgloss/v2"
    "charm.land/lipgloss/v2/compat"
)

var (
    ColorSuccess = compat.AdaptiveColor{Light: lipgloss.Color("#2E7D32"), Dark: lipgloss.Color("#66BB6A")}
    ColorError   = compat.AdaptiveColor{Light: lipgloss.Color("#C62828"), Dark: lipgloss.Color("#EF5350")}
    ColorWarning = compat.AdaptiveColor{Light: lipgloss.Color("#E65100"), Dark: lipgloss.Color("#FFA726")}
    ColorAccent  = compat.AdaptiveColor{Light: lipgloss.Color("#6A1B9A"), Dark: lipgloss.Color("#CE93D8")}
    ColorMuted   = compat.AdaptiveColor{Light: lipgloss.Color("#757575"), Dark: lipgloss.Color("#9E9E9E")}
    ColorChat    = compat.AdaptiveColor{Light: lipgloss.Color("#00695C"), Dark: lipgloss.Color("#4DB6AC")}

    ColorFgBold      = compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#FFFFFF")}
    ColorBorder      = compat.AdaptiveColor{Light: lipgloss.Color("#BDBDBD"), Dark: lipgloss.Color("#616161")}
    ColorHighlightFg = compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#FFFFFF")}
    ColorHighlightBg = compat.AdaptiveColor{Light: lipgloss.Color("#6A1B9A"), Dark: lipgloss.Color("#9C27B0")}
)
```

Update `HighlightFg()` and `HighlightBg()` — `lipgloss.HasDarkBackground()` (no-arg) is removed; use `compat.HasDarkBackground` (a bool set at package init):

```go
// HighlightFg returns the foreground color string for search match highlighting.
func HighlightFg() string {
    if compat.HasDarkBackground {
        return "#FFFFFF"
    }
    return "#FFFFFF"
}

// HighlightBg returns the background color string for search match highlighting.
func HighlightBg() string {
    if compat.HasDarkBackground {
        return "#9C27B0"
    }
    return "#6A1B9A"
}
```

**Step 2: Update `components/assessbar.go`**

`categoryColor()` returns `lipgloss.TerminalColor` which no longer exists. Change to `color.Color`:

```go
import "image/color"

func categoryColor(category string) color.Color {
    switch category {
    case "followed":
        return shared.ColorSuccess
    case "worked_around":
        return shared.ColorWarning
    case "confused":
        return shared.ColorError
    default:
        return shared.ColorMuted
    }
}
```

`compat.AdaptiveColor` implements `color.Color`, so the return values are unchanged.

**Step 3: Update `logview/model.go` color helpers**

The helpers access `.Dark`/`.Light` fields which are now `color.Color` (not `string`). Simplest fix: hardcode the same hex values (these are the same values as in `shared/styles.go`). The `dx-architecture-cleanup` plan will replace these with lipgloss styles later:

```go
func mutedColor() string {
    if compat.HasDarkBackground {
        return "#9E9E9E" // ColorMuted.Dark
    }
    return "#757575" // ColorMuted.Light
}

func errorColor() string {
    if compat.HasDarkBackground {
        return "#EF5350" // ColorError.Dark
    }
    return "#C62828" // ColorError.Light
}

func accentColor() string {
    if compat.HasDarkBackground {
        return "#CE93D8" // ColorAccent.Dark
    }
    return "#6A1B9A" // ColorAccent.Light
}
```

Add import `"charm.land/lipgloss/v2/compat"`.

**Step 4: Build shared and components packages**

```bash
go build ./internal/tui/shared/ ./internal/tui/components/ ./internal/tui/logview/
```

**Step 5: Commit**

```bash
git add internal/tui/shared/styles.go internal/tui/components/assessbar.go internal/tui/logview/model.go
git commit -m "fix(tui): replace lipgloss.AdaptiveColor with compat.AdaptiveColor for v2"
```

---

### Task 4: Root model — `View()` returns `tea.View`, remove `WithAltScreen`

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/tui.go`

**Step 1: Update `tui.go`**

Remove `tea.WithAltScreen()` — alt screen is now declared in `View()`:

```go
// Before
p := tea.NewProgram(m, tea.WithAltScreen())

// After
p := tea.NewProgram(m)
```

**Step 2: Update `appModel.View()` in `model.go`**

Change signature and wrap content in `tea.NewView()`. Enable alt screen via the view struct:

```go
// Before
func (m appModel) View() string {
    // ... build content string ...
    return header + "\n" + separator + "\n" + subHeader + "\n" + separator + "\n" + content
}

// After
func (m appModel) View() tea.View {
    if m.width == 0 || m.height == 0 {
        return tea.NewView("")
    }
    // ... same content assembly as before ...
    content := header + "\n" + separator + "\n" + subHeader + "\n" + separator + "\n" + content

    v := tea.NewView(content)
    v.AltScreen = true
    return v
}
```

**Step 3: Build**

```bash
go build ./internal/tui/
```

**Step 4: Commit**

```bash
git add internal/tui/model.go internal/tui/tui.go
git commit -m "fix(tui): View() returns tea.View, alt screen moved to view field"
```

---

### Task 5: `tea.KeyMsg` → `tea.KeyPressMsg` across all Update() methods

Every `Update()` that does `case tea.KeyMsg:` needs updating. The handler function signatures change too.

**Files:** `model.go`, `dashboard/update.go`, `detail/update.go`, `fitness/update.go`, `sources/update.go`, `pipeline/update.go`, `logview/update.go`, `chat/update.go`

**Step 1: Global replace in all update files**

```bash
find ./internal/tui -name "*.go" -exec sed -i '' \
  's/case tea\.KeyMsg:/case tea.KeyPressMsg:/g' {} \;
```

**Step 2: Update handler function signatures**

Any function that accepts `tea.KeyMsg` as parameter needs to accept `tea.KeyPressMsg`:

```bash
find ./internal/tui -name "*.go" -exec sed -i '' \
  's/func (m \(.*\)) handleKey(msg tea\.KeyMsg)/func (m \1) handleKey(msg tea.KeyPressMsg)/g' {} \;
```

Also find any `updateFilter(msg tea.KeyMsg)`, `updateSearch(msg tea.KeyMsg)`, etc.:

```bash
grep -rn "tea\.KeyMsg" ./internal/tui/ --include="*.go"
```

Fix each remaining occurrence manually.

**Step 3: Fix `detail/view.go:68` — inline key construction**

```go
// Before
if key.Matches(tea.KeyMsg{Type: tea.KeyTab}, kb) {

// After
if key.Matches(tea.KeyPressMsg{Code: tea.KeyTab}, kb) {
```

**Step 4: Build**

```bash
go build ./internal/tui/...
```

**Step 5: Commit**

```bash
git add ./internal/tui/
git commit -m "fix(tui): tea.KeyMsg → tea.KeyPressMsg across all Update() methods"
```

---

### Task 6: Viewport API changes

`viewport.New(w, h)` → `viewport.New(viewport.WithWidth(w), viewport.WithHeight(h))`

Width/Height/YOffset are now getter/setter methods, not fields.

**Files:**
- `internal/tui/detail/model.go`
- `internal/tui/fitness/model.go`
- `internal/tui/chat/model.go`
- `internal/tui/logview/model.go`
- `internal/tui/sources/model.go` (viewport added by completed bug-sources-pipeline-viewport — uses v1 API)
- `internal/tui/pipeline/model.go` (viewport added by completed bug-sources-pipeline-viewport — uses v1 API)
- `internal/tui/dashboard/model.go` (construction not yet present — handled in bubbles-list plan)
- `internal/tui/dashboard/update.go`

**Step 1: Fix `viewport.New()` calls**

For each file, update construction:

```go
// Before
m.bodyViewport = viewport.New(contentWidth, bodyHeight)

// After
m.bodyViewport = viewport.New(viewport.WithWidth(contentWidth), viewport.WithHeight(bodyHeight))
```

Files with `viewport.New(w, h)`:
- `detail/model.go:87`
- `fitness/model.go:88` (uses `viewport.New(contentWidth, contentHeight)`)
- `chat/model.go:96` (uses `viewport.New(width-2, vpH)`)
- `logview/model.go:127` (uses `viewport.New(width, viewHeight)`)

**Step 2: Fix field writes → setter calls**

```bash
grep -n "viewport\.Width =\|viewport\.Height =\|viewport\.YOffset =" ./internal/tui/ -r --include="*.go"
```

For each:

```go
// Before
m.viewport.Width = width - 2
m.viewport.Height = m.viewportHeight()
m.viewport.YOffset = cursorLine

// After
m.viewport.SetWidth(width - 2)
m.viewport.SetHeight(m.viewportHeight())
m.viewport.SetYOffset(cursorLine)
```

Specific locations:
- `chat/model.go:104-105`: `m.viewport.Width = width - 2` and `m.viewport.Height = m.viewportHeight()`
- `dashboard/update.go:39-40`: `m.viewport.Width = msg.Width` and `m.viewport.Height = m.viewportHeight()`
- `dashboard/model.go:210`: `m.viewport.Height = m.viewportHeight()`
- `dashboard/model.go:227-233`: `yOff := m.viewport.YOffset` (read) and `m.viewport.YOffset = cursorLine` (write)

For the dashboard `ensureCursorVisible()`:
```go
// Before
yOff := m.viewport.YOffset
h := m.viewport.Height
if cursorLine < yOff {
    m.viewport.YOffset = cursorLine
} else if cursorLine >= yOff+h {
    m.viewport.YOffset = cursorLine - h + 1
}

// After
yOff := m.viewport.YOffset()
h := m.viewport.Height()
if cursorLine < yOff {
    m.viewport.SetYOffset(cursorLine)
} else if cursorLine >= yOff+h {
    m.viewport.SetYOffset(cursorLine - h + 1)
}
```

**Step 3: Fix field reads in tests**

```bash
grep -rn "viewport\.YOffset\|viewport\.Height\b\|viewport\.Width\b" ./internal/tui/ --include="*_test.go"
```

For each test that reads these fields:
```go
// Before
if m.viewport.YOffset != 0 {
if m.viewport.Height != 5 {

// After
if m.viewport.YOffset() != 0 {
if m.viewport.Height() != 5 {
```

**Step 4: Build and test**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

**Step 5: Commit**

```bash
git add ./internal/tui/
git commit -m "fix(tui): update viewport.New and Width/Height/YOffset to v2 getter/setter API"
```

---

### Task 7: Spinner and textarea style changes

**Files:**
- `internal/tui/detail/update.go` (spinner)
- `internal/tui/chat/update.go` (spinner)
- `internal/tui/chat/model.go` (textarea styles)

**Step 1: Fix spinner — method value → method call**

```bash
grep -rn "spinner\.Tick\b" ./internal/tui/ --include="*.go"
```

Locations:
- `detail/update.go:198`: `m.spinner.Tick`
- `detail/update.go:241`: `m.spinner.Tick`
- `chat/update.go:148`: `m.spinner.Tick`

```go
// Before
return m, tea.Batch(m.spinner.Tick, StartChat(text, m.config, firstMessage))

// After
return m, tea.Batch(m.spinner.Tick(), StartChat(text, m.config, firstMessage))
```

**Step 2: Fix textarea styles in `chat/model.go`**

```go
// Before
ta.FocusedStyle.Base = lipgloss.NewStyle()
ta.BlurredStyle.Base = lipgloss.NewStyle()
ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
ta.BlurredStyle.CursorLine = lipgloss.NewStyle()

// After
ta.Styles.Focused.Base = lipgloss.NewStyle()
ta.Styles.Blurred.Base = lipgloss.NewStyle()
ta.Styles.Focused.CursorLine = lipgloss.NewStyle()
ta.Styles.Blurred.CursorLine = lipgloss.NewStyle()
```

**Step 3: Build**

```bash
go build ./internal/tui/chat/ ./internal/tui/detail/
```

**Step 4: Commit**

```bash
git add internal/tui/chat/model.go internal/tui/chat/update.go internal/tui/detail/update.go
git commit -m "fix(tui): spinner.Tick() method call and textarea v2 Styles field names"
```

---

### Task 8: Update tests — key message construction

All test files that construct `tea.KeyMsg{...}` need to use `tea.KeyPressMsg{...}`.

**Files:** All `*_test.go` under `internal/tui/`

**Step 1: Find all test key constructions**

```bash
grep -rn "tea\.KeyMsg{" ./internal/tui/ --include="*_test.go"
```

**Step 2: Replace constructions**

Pattern mapping:

| v1 | v2 |
|---|---|
| `tea.KeyMsg{Type: tea.KeyDown}` | `tea.KeyPressMsg{Code: tea.KeyDown}` |
| `tea.KeyMsg{Type: tea.KeyUp}` | `tea.KeyPressMsg{Code: tea.KeyUp}` |
| `tea.KeyMsg{Type: tea.KeyEnter}` | `tea.KeyPressMsg{Code: tea.KeyEnter}` |
| `tea.KeyMsg{Type: tea.KeyEscape}` | `tea.KeyPressMsg{Code: tea.KeyEsc}` |
| `tea.KeyMsg{Type: tea.KeyHome}` | `tea.KeyPressMsg{Code: tea.KeyHome}` |
| `tea.KeyMsg{Type: tea.KeyEnd}` | `tea.KeyPressMsg{Code: tea.KeyEnd}` |
| `tea.KeyMsg{Type: tea.KeyPgUp}` | `tea.KeyPressMsg{Code: tea.KeyPgUp}` |
| `tea.KeyMsg{Type: tea.KeyPgDown}` | `tea.KeyPressMsg{Code: tea.KeyPgDown}` |
| `tea.KeyMsg{Type: tea.KeyLeft}` | `tea.KeyPressMsg{Code: tea.KeyLeft}` |
| `tea.KeyMsg{Type: tea.KeyRight}` | `tea.KeyPressMsg{Code: tea.KeyRight}` |
| `tea.KeyMsg{Type: tea.KeyTab}` | `tea.KeyPressMsg{Code: tea.KeyTab}` |
| `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}` | `tea.KeyPressMsg{Code: 'q', Text: "q"}` |
| `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}` | `tea.KeyPressMsg{Code: 'o', Text: "o"}` |

Note: space bar is `tea.KeyPressMsg{Code: ' ', Text: " "}` but `msg.String()` returns `"space"` — check any tests that compare against `" "` and update to `"space"`.

**Step 3: Run all tests**

```bash
go test ./internal/tui/... -v 2>&1 | grep -E "FAIL|PASS|error"
```

Fix any remaining failures.

**Step 4: Commit**

```bash
git add ./internal/tui/
git commit -m "fix(tui): update test key constructions to tea.KeyPressMsg for v2"
```

---

### Task 9: Full build and verification

**Step 1: Clean build**

```bash
go build ./...
```

Expected: zero errors.

**Step 2: Full test suite**

```bash
go test ./...
```

Expected: all pass.

**Step 3: Snapshot regeneration**

The upgrade may change snapshot output if any styled strings render differently with v2. Regenerate and review:

```bash
make snapshots
```

Visually inspect all snapshots for unexpected changes. Commit if they look correct.

**Step 4: Smoke test**

```bash
make install && cabrero
```

Confirm:
1. Dashboard renders correctly in full-screen
2. Navigation (up/down/page) works
3. Keys respond (q quits, ? opens help, s opens sources)
4. Colors look right on both dark and light terminal backgrounds

**Step 5: Commit snapshots if changed**

```bash
git add snapshots/
git commit -m "chore(snapshots): regenerate for charm v2 upgrade"
```

---

### Task 10: (Follow-up) Proper dark/light mode via `BackgroundColorMsg`

> **Optional but recommended.** The `compat` package in Task 3 reads `stdin`/`stdout` at init time and works correctly for a local terminal. For completeness and future SSH/Wish support, the proper approach is BubbleTea's `BackgroundColorMsg`.

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/shared/styles.go`

**Approach:**

1. Add `isDark bool` to `appModel`.
2. In `appModel.Init()`, return `tea.RequestBackgroundColor`.
3. Handle `tea.BackgroundColorMsg` in `appModel.Update()`:
   ```go
   case tea.BackgroundColorMsg:
       m.isDark = msg.IsDark()
       // Re-initialize styles that depend on isDark.
       shared.InitStyles(m.isDark)
       return m, nil
   ```
4. Change `shared.Color*` vars to functions that take `isDark bool`, or use `lipgloss.LightDark(isDark)`:
   ```go
   func Colors(isDark bool) struct{ Success, Error, ... color.Color } {
       ld := lipgloss.LightDark(isDark)
       return struct{...}{
           Success: ld(lipgloss.Color("#2E7D32"), lipgloss.Color("#66BB6A")),
           // ...
       }
   }
   ```

This is a more involved refactor and can be done as a separate commit after the rest of the upgrade is stable.

---

## Updated Execution Order

After the v2 upgrade, the plan order from the main sequence is unchanged but the upgrade **prepends** it all:

```
1.  charm-v2-upgrade          ← this plan (prerequisite for all)
    (all bug plans already done)
2.  dx-unified-utilities      (zero risk — no user-visible change)
3.  dx-style-consolidation    (snapshot risk, well-contained)
4.  dx-layout-patterns        (moderate — HideStatusBar, FillToBottom, ConfirmOverlay)
5.  dx-architecture-cleanup   (low — RenderHeader, ViewSourceDetail, logview)
6.  bubbles-list-dashboard    (higher risk — run on stable style foundation)
7.  dx-background-color-detection  (last — most sweeping test infra impact)
8.  dx-help-overlay-scroll    (independent — anytime after step 1)
```
