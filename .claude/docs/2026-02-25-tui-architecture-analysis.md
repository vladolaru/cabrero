# 2026-02-25 TUI Architecture Analysis

*Cabrero — deep architectural audit of the dashboard TUI*

---

## Executive Summary

The TUI is structurally sound: the Bubble Tea model/update/view pattern is applied consistently, the message bus is clean, and the package boundaries make sense. The problems fall into two distinct classes that warrant different urgency:

**User-visible bugs** (affect the running TUI today):
- The root `appModel.statusMsg` field is set by multiple message handlers but never rendered — status messages like "Archive failed: …" silently disappear (#17).
- Sources and pipeline views have no scrolling viewport — content overflows on standard 80×24 terminals (#6).
- The dashboard filter accepts input but `applyFilter()` ignores the text — the feature is a no-op (#13, #14).

**Developer DX debt** (affects future development, not current users):
- The same styles are declared independently in 5+ packages instead of using the shared equivalents (#1, #2, #3).
- The fill+statusbar pattern is copy-pasted six times with small variations (#4).
- Utility functions are duplicated across packages (#5, #11).
- Architectural smells: parent setting child rendering flags, orphaned ViewState enum values (#8, #18).

This document catalogs all 18 issues, distinguishes their impact, and provides a sequenced plan that fixes user-visible bugs before tackling developer debt.

> **Note:** This analysis was produced via a read-only code audit followed by a structured decision-critic review. Test coverage depth was not verified; any refactoring phase that touches view files should run `make snapshots` and update golden files as an explicit step.

---

## 1. Style Proliferation

**Severity: Medium — Developer DX**

Every view package declares its own local style variables instead of using `shared.HeaderStyle`, `shared.MutedStyle`, etc. The result is the same color/font specified in six different places.

### What `shared/styles.go` exports

```go
HeaderStyle   = lipgloss.NewStyle().Bold(true)
MutedStyle    = lipgloss.NewStyle().Foreground(ColorMuted)
SuccessStyle  = lipgloss.NewStyle().Foreground(ColorSuccess)
ErrorStyle    = lipgloss.NewStyle().Foreground(ColorError)
WarningStyle  = lipgloss.NewStyle().Foreground(ColorWarning)
AccentStyle   = lipgloss.NewStyle().Foreground(ColorAccent)
SelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorFgBold)
```

### What each package re-creates

**`dashboard/view.go:15-23`** — seven aliases, all identical to shared:
```go
headerStyle   = shared.HeaderStyle
mutedStyle    = shared.MutedStyle
accentStyle   = shared.AccentStyle
// ... four more
```
Using the shared names would eliminate these entirely.

**`sources/view.go:27-36`** — eight aliases, all identical to shared.

**`pipeline/view.go:17-22`** — four aliases + `sectionHeaderStyle` which is a new composition:
```go
sectionHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorAccent)
```
This combination (`Bold + Accent`) is independently re-created in several other places.

**`detail/view.go:16-20`** — new names for the same ideas:
```go
detailHeader  = lipgloss.NewStyle().Bold(true)            // = shared.HeaderStyle
detailMuted   = lipgloss.NewStyle().Foreground(shared.ColorMuted)   // = shared.MutedStyle
detailSection = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorAccent) // = AccentBold (not in shared)
detailAccent  = lipgloss.NewStyle().Foreground(shared.ColorAccent)  // = shared.AccentStyle
```

**`fitness/view.go:13-18`** — same pattern, different name prefix (`fitness*`).

**`components/assessbar.go:153`** — creates a new `lipgloss.Style` inline *inside a function call*:
```go
mutedStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
```
This allocates a new style object on every call to `renderAssessRow`.

### Fix

Add `AccentBoldStyle` (Bold + Accent) to `shared/styles.go` — it's used in 4+ places. Remove all local aliases. The packages that currently shadow `shared.MutedStyle` as `mutedStyle` should just use `shared.MutedStyle` directly.

---

## 2. Section Header Pattern Inconsistency

**Severity: Medium — Developer DX**

Content sections should look the same everywhere: an accented bold title followed by a fixed-width separator line. This pattern exists in some views and is absent in others.

### Where it IS used

`detail/model.go` (renderBodyContent):
```go
b.WriteString(detailSection.Render("  PROPOSED CHANGE"))
b.WriteString("\n")
b.WriteString("  " + strings.Repeat("─", 17))   // magic 17
b.WriteString("\n")
```

`fitness/view.go` (renderViewportContent):
```go
b.WriteString(fitnessSection.Render("  ASSESSMENT"))
b.WriteString("\n")
b.WriteString("  " + strings.Repeat("\u2500", 17))  // same magic 17, different char representation
```

### Where it is NOT used consistently

`sources/view.go` (renderDetail):
```go
b.WriteString("  " + sectionHeaderStyle.Render("INFO") + "\n\n")
// No separator line
```

`pipeline/view.go` (all sections):
```go
b.WriteString("  " + sectionHeaderStyle.Render("DAEMON"))
b.WriteString("\n")
// No separator line
```

### The magic 17

`strings.Repeat("─", 17)` appears at:
- `detail/model.go:104, 111, 120, 142, 147`
- `fitness/view.go:75, 88, 97`

The value 17 is never explained. "PROPOSED CHANGE" is 15 chars. This should be either derived from a constant or dropped entirely in favor of a separator that spans something meaningful (e.g., content width).

### Fix

Add a `shared.RenderSectionHeader(title string)` function that renders the accented bold title. Separator lines are optional decoration — decide one way or the other and apply it everywhere. Remove the bare `17` magic number.

---

## 3. SubHeader Title Styling Inconsistency

**Severity: Medium**

`SubHeader()` is the two-line string (title + stats) rendered beneath the persistent header. Every view implements it, but the title style differs:

| View | Title style | Stats line style |
|------|-------------|-----------------|
| Dashboard | `headerStyle` (Bold) | `mutedStyle` |
| Detail | `detailHeader` (Bold) — same value | `detailMuted` — same value |
| Fitness | `fitnessHeader` (Bold) — same value | `fitnessMuted` — same value |
| Sources | `headerStyle` (Bold) | `mutedStyle` |
| Pipeline | `titleStyle` = Bold + `ColorFgBold` foreground | `mutedStyle` |
| LogView | `titleStyle` = Bold + `ColorFgBold` foreground | **plain, not muted** |

Pipeline and LogView add `ColorFgBold` foreground to the title. On dark terminals this makes no visible difference (foreground is already white), but it is semantically different from the other views' `Bold(true)` only style.

The LogView stats line (`"  %d entries  ·  follow %s"`, line 27) is rendered without any style — not muted, not bold. All other views wrap their stats in `mutedStyle.Render(...)`.

### Fix

- Use `shared.HeaderStyle` for all SubHeader titles.
- Wrap all stats lines in `shared.MutedStyle.Render(...)`.
- Define `SubHeader()` once at the shared level as a `func(title, stats string) string` helper, and call it from each view with the view-specific strings.

---

## 4. Fill + Status Bar Pattern (Repeated 6+ Times)

**Severity: Medium — Developer DX**

Anchoring the status bar to the bottom of the terminal requires counting newlines and filling the remaining space. This same pattern appears with slight variations everywhere:

**`sources/view.go:92-101`** (list view) and **`sources/view.go:368-376`** (detail view):
```go
content := b.String()
lines := strings.Count(content, "\n")
statusBarHeight := 1
remaining := m.height - lines - statusBarHeight
if remaining > 0 {
    content += strings.Repeat("\n", remaining)
}
content += m.renderStatusBar()
```

**`pipeline/view.go:86-97`**:
```go
content := strings.Join(sections, "\n\n")
lines := strings.Count(content, "\n")
statusBarHeight := 1
remaining := m.height - lines - statusBarHeight
if remaining > 0 {
    content += strings.Repeat("\n", remaining)
}
statusBar := components.RenderStatusBar(...)
return content + statusBar
```

**`detail/view.go:57-76`** — same logic, different variable name:
```go
remaining := m.height - lines - 1  // "1" instead of "statusBarHeight"
```

**`fitness/view.go:50-58`** — same but recomputes `lines` after modifying `content`:
```go
content := b.String()
lines := strings.Count(content, "\n")
remaining := m.height - lines - 1
```

**`chat/view.go:112-118`** — same but the filled content pushes input down (not status bar).

This 5-line block is copy-pasted across 6 locations. Any change (e.g., adding a second status bar line) requires updating all 6.

### Fix

```go
// shared/format.go
func FillToBottom(content string, totalHeight, reservedLines int) string {
    lines := strings.Count(content, "\n")
    remaining := totalHeight - lines - reservedLines
    if remaining > 0 {
        return content + strings.Repeat("\n", remaining)
    }
    return content
}
```

Then each view becomes:
```go
content = shared.FillToBottom(content, m.height, 1)
content += components.RenderStatusBar(...)
```

---

## 5. Duplicate Utility Functions

**Severity: Medium**

### `timeAgo` vs `relativeTime`

`dashboard/model.go:235-247` and `pipeline/view.go:448-460` both format a `time.Time` as a relative age string. Different names, slightly different "fresh" labels ("just now" vs "now"), otherwise identical logic.

These should be one function in `shared/format.go`.

### `checkMark` vs `checkmark`

`dashboard/view.go:228-232`:
```go
func checkMark(ok bool) string { ... }
```

`pipeline/view.go:372-377`:
```go
func checkmark(ok bool) string { ... }
```

Identical logic. Should be `shared.Checkmark(ok bool) string` or `components.Checkmark(ok bool) string`.

### `formatBytes`, `formatTokenCount`, `formatCost`, `formatUptime`, `formatInterval`

All defined in `pipeline/view.go`. While currently only used in pipeline, `formatBytes` and `formatUptime` are general utilities. If anything else ever needs them, they'd be duplicated again. They belong in `shared/format.go`.

---

## 6. Missing Scrolling in Sources and Pipeline Views

**Severity: High**

### Pipeline Monitor

`pipeline/model.go` stores `width` and `height` but has no `viewport.Model`. The `View()` method renders all sections as a joined string and then fills to the bottom. With the default `recentRunsLimit: 20`, the RECENT RUNS section alone can produce 20+ lines. The DAEMON + ACTIVITY sections add ~15 more lines. On a standard 80×24 terminal this content overflows.

### Source Manager

Same situation in `sources/model.go` — no viewport, flat render of all items. With many sources the content overflows without scroll.

The dashboard, detail, fitness, logview, and chat views all correctly use `viewport.Model`. Sources and pipeline are the outliers.

### Fix

Both views should adopt the viewport pattern:
1. Fixed sections (headers, summary stats) render as "chrome" above the viewport.
2. The scrollable section (runs list, flat source list) goes into a `viewport.Model`.
3. `SetSize()` computes viewport height = total height − chrome lines.

---

## 7. Confirmation Dialog Inconsistency (3 Patterns for the Same Thing)

**Severity: Medium**

There are three different strategies for displaying the `ConfirmModel`:

**Pattern 1 — Embedded in viewport content** (`detail`):
The confirm prompt is appended inside `renderBodyContent()` and rendered within the scrollable body viewport. The viewport scrolls to show it. This is contextual and smooth.

**Pattern 2 — Full-screen early return** (`sources`):
```go
// sources/view.go:62-69
if m.confirmState == ConfirmSetOwnership && m.ownershipPrompt != "" {
    return m.ownershipPrompt
}
if m.confirm.Active {
    return m.confirm.View()
}
```
Returns just the raw prompt string — no status bar, no fill, no chrome. The result is a single line of text on an otherwise blank screen, anchored to the top.

**Pattern 3 — Full-screen early return, same raw string** (`pipeline`):
```go
// pipeline/view.go:79-82
if m.confirm.Active {
    return m.confirm.View()
}
```
Identical problem to Pattern 2.

### Fix

For views where confirm is a "modal" (sources, pipeline), the confirm should render with proper chrome: fill the remaining space and show a status bar with the available options. Create a `components.RenderConfirmModal(confirm ConfirmModel, width, height int) string` that wraps the prompt in a centered block with fill.

For the detail view, the embedded-in-viewport approach is already good — leave it.

---

## 8. `HideStatusBar` Anti-Pattern

**Severity: Medium**

`detail.Model` has a public `HideStatusBar bool` field that the root `appModel` sets directly:

```go
// model.go:681-682
m.detail.HideStatusBar = true  // or false
m.detail.SetSize(dw, panelH)
```

This is a flag the child model uses to alter its own `View()` output. It creates bidirectional coupling: the parent sets a rendering property on the child. The child's behavior depends on whether it was told to be in a special mode.

The underlying need is real: when the root renders a shared status bar for the detail+chat split layout, the detail model should not render its own. But the mechanism is wrong.

### Fix

The cleanest solution is to have `detail.Model` never render its own status bar. The status bar is always rendered by the root. This removes the need for the flag entirely. The root already calls `components.RenderStatusBar(m.keys.DetailShortHelp(), "", m.width)` in the wide and narrow split cases — just always do it and have detail always leave that line empty in its `View()` output. The `chrome` calculation in `SetSize()` would always use the higher number.

---

## 9. `bubbles/list` Opportunity for Dashboard

**Severity: Low-to-Medium (optional improvement)**

The dashboard implements a full custom list:
- `Model.items`, `Model.filtered`, `Model.cursor`
- `Model.viewport viewport.Model`
- `Model.applyFilter()`, `Model.updateContent()`, `Model.ensureCursorVisible()`
- `Model.filterInput textinput.Model`, `Model.filterActive bool`

The `charmbracelet/bubbles` library (v1.0.0) ships a `list` component that handles most of this — cursor management, viewport scrolling, built-in filter/search with a textinput, empty state, and item delegation.

Adopting `bubbles/list` would:
- Delete ~150 lines of custom list/viewport/filter code
- Get proper filter-as-you-type (the current filter only applies on Enter)
- Get standard keyboard navigation out of the box

The cost: `bubbles/list` has opinionated default styles and delegates rendering through `list.Item` and `list.ItemDelegate` interfaces. Adapting the `DashboardItem` type to this interface is ~40 lines. The visual result would be slightly different from the current custom rendering, so the decision depends on whether the current visual design is a hard constraint.

---

## 10. `bubbles/help` Opportunity for Help Overlay

**Severity: Low (optional improvement)**

`components/helpoverlay.go` implements a custom help renderer. The `charmbracelet/bubbles` help component (`bubbles/help`) provides a standard two-column key binding layout driven directly from `key.Binding` help strings.

Current `RenderHelpOverlay` does more than bubbles/help: it renders description paragraphs before the key bindings. The bubbles help component doesn't support this. However, adopting bubbles/help for just the key bindings section (below the description) would make the key binding columns use standard spacing and automatic width fitting.

The help overlay also has no scrolling — long help content overflows. A `viewport.Model` wrapper would fix this independently of whether bubbles/help is used.

---

## 11. `bubbles/progress` Opportunity for Assessment Bars

**Severity: Low (cosmetic only)**

`components/assessbar.go` and `sources/view.go:renderBar()` both implement custom horizontal progress bars using `█` and `░` characters. The `charmbracelet/bubbles` progress component provides animated progress bars. The animation may not be desirable (bars update rarely), but the progress component's `Full` and `Empty` characters, width calculation, and color theming are equivalent to what's been built manually.

Both custom bar implementations differ from each other:
- `assessbar.go` uses `barFilled = '█'` and `barEmpty = '░'` with lipgloss coloring
- `sources/view.go:renderBar()` uses `strings.Repeat("\u2588", filled)` (same char) with lipgloss coloring

They are functionally identical but duplicated. At minimum, unify them into one function in `components`.

---

## 12. Unused Spinner in Fitness Model

**Severity: Low**

`fitness/model.go` declares and initializes a `spinner.Model`:
```go
spinner spinner.Model
// ...
s := spinner.New()
s.Spinner = spinner.Dot
m.spinner = s
```

The spinner is never referenced in `fitness/view.go` or `fitness/update.go`. It appears to be a placeholder for a future async operation (perhaps loading fitness data). It should be removed until needed.

---

## 13. Filter Not Implemented

**Severity: Medium**

The dashboard filter shows a text input and accepts typed input, but `applyFilter()` ignores `m.filterText`:

```go
// dashboard/model.go:176-184
func (m *Model) applyFilter() {
    m.filtered = m.items
    // Future: implement actual filtering by type/target/text.
    // For now, show all items.
    ...
}
```

Users who type into the filter input see no effect. The UI affordance (the `/ ` prompt, the placeholder text "type:skill target:docx or free text...") implies the feature exists. This should either be implemented or the filter input removed.

---

## 14. Dashboard Filter Uses Hard-coded Keys

**Severity: Low**

In `dashboard/update.go:143-155`, the filter input's exit keys are created inline as new bindings instead of referencing the shared keymap:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
    // dismiss filter
case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
    // commit filter
```

If a user configures vim navigation (`"navigation": "vim"`), Esc still dismisses the filter (fine), but more importantly, if the keymap's `Back` binding ever changes from `"esc"`, the filter won't update. This should use `m.keys.Back` and `m.keys.Open` (Enter).

---

## 15. termenv vs lipgloss Rendering in LogView

**Severity: Low**

`logview/model.go` uses `termenv` for coloring:
```go
var highlightOutput = termenv.NewOutput(os.Stderr, termenv.WithProfile(termenv.TrueColor))
// ...
return highlightOutput.String(level).Foreground(highlightOutput.Color(color)).String()
```

And then manually resolves adaptive colors:
```go
func mutedColor() string {
    if lipgloss.HasDarkBackground() {
        return shared.ColorMuted.Dark
    }
    return shared.ColorMuted.Light
}
```

The rest of the TUI uses `lipgloss` for all styling. `lipgloss.AdaptiveColor` handles the dark/light selection automatically — there's no need to manually access `.Dark` / `.Light` fields. The `termenv` dependency in logview is unique to that package and introduces a second styling system.

The reason termenv is used in logview is that search highlighting needs to apply color to an already-rendered ANSI string (the entry lines). This is a legitimate use case — `ansi.Strip` + re-color requires termenv. However, the non-highlighting rendering (timestamp, level badge) could use lipgloss consistently, and `mutedColor()`, `errorColor()`, `accentColor()` helpers that peek inside `AdaptiveColor` structs should not be necessary.

---

## 16. RenderHeader Coupled to Dashboard Package

**Severity: Low**

`dashboard.RenderHeader(stats, width)` is a free function in the `dashboard` package that the root `appModel` calls to render the persistent application header. The header contains no dashboard-specific state — it only uses `message.DashboardStats` and the current width.

```go
// model.go:111, 439
header := dashboard.RenderHeader(m.stats, m.width)
```

This creates an import dependency from the root TUI package on the `dashboard` child package just for the header. If the header needs to be rendered in a context that doesn't involve the dashboard (e.g., a future configuration view), it requires importing `dashboard`.

The header function should live in either `shared` or `components` since it's used by the root, not the dashboard.

---

## 17. Root `statusMsg` Is Never Rendered — Silent Message Loss

**Severity: High — User-visible bug**

Multiple message handlers in the root model set `m.statusMsg`:

```go
// model.go — RejectFinished, DeferFinished, ApplyFinished, DismissFinished, etc.
m.statusMsg = actionStatusText(msg)
m.statusExpiry = time.Now().Add(3 * time.Second)
```

But the root `View()` never includes `m.statusMsg` in its output:

```go
// model.go:487
return header + "\n" + separator + "\n" + subHeader + "\n" + separator + "\n" + content
```

The `m.statusMsg` and `m.statusExpiry` fields are dead in the render path. Messages like "Archive failed: …" (from a failed `RejectFinished`), "Rollback failed: …", or "Retry failed: …" are set and then silently dropped — the user never sees them.

This is compounded by there being two independent status message systems:

1. **Root system** (broken): `appModel.statusMsg` + `appModel.statusExpiry` — set but never rendered.
2. **Pipeline-local system** (working): `pipeline.Model.statusMsg` + private `statusClearMsg` type — used only by the pipeline view's own status bar.

All other views rely on `components.RenderStatusBar(bindings, timedMsg, width)` but always pass `""` for `timedMsg`, meaning the root's status messages have no delivery mechanism at all for those views either.

### Fix

Pick one canonical path. The simplest fix is to make the root `View()` pass `m.statusMsg` down to the active child's status bar call, or render a dedicated global status line. Alternatively, route all status messages as `message.StatusMessage` to the active child view, have each child accept and display it in its own status bar, and remove the root-level `m.statusMsg` fields entirely. The pipeline's private `statusClearMsg` mechanism can serve as the pattern.

---

## 18. `ViewSourceDetail` Is a Distinct ViewState But Not a Distinct Model

**Severity: Low**

`message.ViewState` includes `ViewSourceDetail`, and `shared/helpdata.go` has separate help content for it. But in `appModel`:

```go
case message.ViewSourceManager, message.ViewSourceDetail:
    content = m.sources.View()
```

Both states render the same `sources.Model`. The `ViewSourceDetail` state is only used to select different help content. The sources model internally tracks `detailOpen bool` and renders differently based on that flag. The `ViewSourceDetail` ViewState is purely cosmetic at the root level — it doesn't control a separate model.

This creates potential confusion: someone reading the root model sees `ViewSourceDetail` and expects there's a dedicated `sourceDetail` model, but there isn't. The approach of the sources model managing its own sub-view state is good, but the separate ViewState is unnecessary. The help overlay could check `m.sources.DetailOpen()` to select the right help content without the extra ViewState.

---

## Summary Table

**Impact key:** 🔴 User-visible bug · 🟡 Developer DX / code quality · ⚪ Architecture / low-risk cleanup

| # | Issue | Severity | Impact | Category |
|---|-------|----------|--------|----------|
| 17 | Root `m.statusMsg` set but never rendered — status messages lost | **High** | 🔴 | Correctness |
| 6 | Sources and pipeline have no viewport (overflow on small terminals) | **High** | 🔴 | Correctness |
| 13 | Dashboard filter UI exists but filter logic is a no-op | **Medium** | 🔴 | Correctness |
| 14 | Dashboard filter uses hard-coded keys instead of keymap | Low | 🔴 | Correctness |
| 1 | Style proliferation — 5 packages re-declare shared styles | Medium | 🟡 | Consistency |
| 2 | Section header pattern inconsistent (separator/no separator) | Medium | 🟡 | Consistency |
| 3 | SubHeader title/stats styling inconsistent across views | Medium | 🟡 | Consistency |
| 4 | Fill+statusbar pattern copy-pasted 6+ times | Medium | 🟡 | Reuse |
| 5 | `timeAgo`/`relativeTime` and `checkMark`/`checkmark` duplicated | Low | 🟡 | Reuse |
| 7 | Confirmation dialog: 3 different rendering patterns | Medium | 🟡 | Consistency |
| 8 | `HideStatusBar` field: parent sets child rendering flag | Medium | 🟡 | Architecture |
| 11 | Two independent bar implementations (assess vs source) | Low | 🟡 | Reuse |
| 12 | Unused spinner in `fitness.Model` | Low | 🟡 | Dead code |
| 15 | `logview` uses `termenv` + manual adaptive color; rest uses lipgloss | Low | 🟡 | Consistency |
| 16 | `dashboard.RenderHeader` should live in `shared` or `components` | Low | ⚪ | Architecture |
| 18 | `ViewSourceDetail` ViewState unnecessary (no dedicated model) | Low | ⚪ | Architecture |
| 9 | Dashboard could use `bubbles/list` (optional) | Low-Med | ⚪ | Bubbles |
| 10 | Help overlay has no scroll; `bubbles/help` not evaluated | Low | ⚪ | Bubbles |

---

## Recommended Refactoring Priorities

The plan is split into two tracks. Fix user-visible bugs first; tackle developer DX debt second. Do not interleave them — a broken feature is more urgent than a messy codebase.

> **Snapshot warning (applies to all phases):** Any phase that modifies `view.go` files must run `make snapshots` and commit the updated PNG/SVG files. Snapshot tests are the primary guard against visual regressions.

---

### Bug Track — Fix what users can see

#### Bug 1 — Fix silent root `statusMsg` (#17) `[High]`

The root `appModel` sets `m.statusMsg` on failure paths but never renders it. Users never see "Archive failed: …" or "Rollback failed: …". Fix: remove `m.statusMsg` / `m.statusExpiry` from the root; route all status messages via `message.StatusMessage` to the active child view, which delivers them through `components.RenderStatusBar(bindings, timedMsg, width)`. The pipeline's existing `statusClearMsg` pattern is the reference implementation.

#### Bug 2 — Add viewport to sources and pipeline (#6) `[High]`

Both views render all content as a flat string with no scrolling. At default config (`recentRunsLimit: 20`) the pipeline overflows a standard 80×24 terminal. Fix:
- **Pipeline**: split into fixed chrome (DAEMON + HOOKS + STORE + ACTIVITY sections) above a `viewport.Model` that contains only the RECENT RUNS list.
- **Sources**: the flat item list goes into a `viewport.Model`; column headers stay as fixed chrome above it.
`SetSize()` in both models computes `viewportHeight = totalHeight − chromeLines`.

#### Bug 3 — Implement or remove the dashboard filter (#13, #14) `[Medium]`

`applyFilter()` sets `m.filtered = m.items` regardless of `m.filterText`. The filter UI (textinput, `/ ` prompt, placeholder) implies the feature works. Either implement it or remove the UI. If implementing: replace the hard-coded `key.NewBinding(key.WithKeys("esc"/"enter"))` inline keys with `m.keys.Back` and `m.keys.Open` from the shared keymap.

> **Dependency note:** If `bubbles/list` adoption (Architecture item 2 below) is being considered, implement the filter *after* deciding on that — building filter logic into the custom list and then migrating to `bubbles/list` is double work.

---

### DX Track — Reduce developer friction

#### DX 1 — Unify duplicate utilities (#5, #11, #12) `[Low — do first, smallest diffs]`

- Remove unused `spinner` from `fitness.Model`.
- Add `shared.RelativeTime(t time.Time) string` (returning "just now" for < 1 min) and `shared.Checkmark(ok bool) string`; delete `timeAgo`, `relativeTime`, `checkMark`, `checkmark`.
- Unify `components.RenderAssessBar` and `sources.renderBar` into a single `components.RenderBar(percent float64, width int, color lipgloss.TerminalColor) string`.

#### DX 2 — Consolidate styles (#1, #2, #3) `[Medium — touches every view file]`

- Add `shared.AccentBoldStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)` to `shared/styles.go`.
- In `dashboard`, `sources`, `pipeline`: delete local aliases (headerStyle, mutedStyle, etc.) and reference `shared.*Style` directly.
- In `detail`, `fitness`: replace `detailHeader`/`fitnessHeader` with `shared.HeaderStyle`; `detailSection`/`fitnessSection` with `shared.AccentBoldStyle`; `detailMuted`/`fitnessMuted` with `shared.MutedStyle`.
- Add `shared.RenderSubHeader(title, stats string) string` → `HeaderStyle.Render(title) + "\n" + MutedStyle.Render(stats)`. Replace all six `SubHeader()` implementations with calls to it.
- Pick one separator convention for section headers: either always use `"  " + strings.Repeat("─", 17)` or drop it from the views that don't have it. Remove the bare `17` literal.

**Run `make snapshots` and commit updated files after this phase.**

#### DX 3 — Consolidate fill + statusbar pattern (#4) `[Medium]`

Add `shared.FillToBottom(content string, totalHeight, reservedLines int) string`. Replace the 5 view instances (sources list, sources detail, pipeline, detail, fitness) that copy-paste the count-newlines-and-pad logic. The chat view's fill-to-push-input variant has different semantics — leave it as-is.

#### DX 4 — Fix `HideStatusBar` anti-pattern (#8) `[Medium]`

Have `detail.Model` never render its own status bar. Remove the `HideStatusBar` public field. The root already renders the status bar in the split layout; extend this to always render it, and compute `chrome` in `SetSize()` uniformly. Remove the parent's direct field mutation.

#### DX 5 — Unify confirmation overlay rendering (#7) `[Medium]`

Create `components.RenderConfirmModal(prompt, opts string, width, height int) string` that fills the terminal and centers the prompt. Replace the two "full-screen early return" cases (sources, pipeline) that currently return a raw single-line string. Leave the detail view's embedded-in-viewport approach unchanged.

#### DX 6 — Architecture cleanup (#16, #18, #15) `[Low]`

- Move `dashboard.RenderHeader` to `shared` or `components` (it uses only `message.DashboardStats` + width).
- Remove `ViewSourceDetail` from `message.ViewState`; replace the help-overlay lookup with `m.sources.DetailOpen()`.
- In `logview`, replace manual `mutedColor()` / `errorColor()` helpers (which access `AdaptiveColor.Dark`/`.Light` directly) with standard lipgloss style rendering for the non-highlight paths.

---

### Architecture Track — Optional, larger scope

#### Architecture 1 — Add viewport scroll to help overlay (#10) `[Low]`

Wrap `components.RenderHelpOverlay` output in a `viewport.Model` so long help content doesn't overflow. This is independent of `bubbles/help`.

#### Architecture 2 — Evaluate `bubbles/list` for dashboard (#9) `[Low-Med — decide deliberately]`

The dashboard has a custom cursor-tracked scrollable list with filter plumbing. `bubbles/list` provides all of this out of the box, including filter-as-you-type. Adopting it would delete ~150 lines of custom code and give the filter feature for free, but requires adapting `DashboardItem` to the `list.Item` interface and accepting `bubbles/list`'s default visual conventions.

**Recommendation:** Make this decision *before* implementing the dashboard filter (Bug 3). If `bubbles/list` is adopted, the filter comes for free. If not, implement the custom filter logic as described in Bug 3.

---

## Bubbles Components: Current Usage vs Opportunities

| Component | Currently Used | Opportunity |
|-----------|---------------|-------------|
| `bubbles/viewport` | dashboard, detail, fitness, chat, logview | Add to sources, pipeline |
| `bubbles/textinput` | dashboard (filter), logview (search) | — |
| `bubbles/textarea` | chat | — |
| `bubbles/spinner` | detail, chat | Remove from fitness (unused) |
| `bubbles/key` | everywhere | — |
| `bubbles/list` | **nowhere** | Dashboard list (#9) |
| `bubbles/progress` | **nowhere** | Unify bar components (#11) |
| `bubbles/help` | **nowhere** | Help overlay bindings (#10) |
| `bubbles/paginator` | **nowhere** | Pipeline runs pagination (if list grows large) |
| `bubbles/table` | **nowhere** | Dashboard/sources column layout (complex tradeoff) |
