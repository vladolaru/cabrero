# DX: Style Consolidation — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate the style proliferation across view packages — 5 packages each independently declare the same lipgloss styles instead of using the shared ones. Add `shared.AccentBoldStyle` (the missing composition). Unify all `SubHeader()` implementations. Decide on one section header separator convention and apply it everywhere.

**Architecture:** All changes are mechanical substitutions: delete a local var, replace its usages with the shared equivalent. Tests assert on rendered content (using `ansi.Strip`), so they are immune to style changes. The snapshot PNGs will change if section separators are added or removed.

**Dependency:** Safe to execute after the utilities plan (`dx-unified-utilities`). Must run `make snapshots` and commit updated files at the end.

**Snapshot note:** This plan changes rendered output in `detail`, `fitness`, `sources`, and `pipeline`. Run `make snapshots` after Task 3 and commit the regenerated files.

---

### Task 1: Add `shared.AccentBoldStyle`

**Files:**
- Modify: `internal/tui/shared/styles.go`
- Test: (compile-time — subsequent tasks verify it works)

**Step 1: Add to `shared/styles.go`**

```go
// AccentBoldStyle is Bold + Accent foreground. Used for section headers across views.
AccentBoldStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
```

Place it after `AccentStyle` in the var block.

**Step 2: Build**

```bash
go build ./internal/tui/shared/
```

**Step 3: Commit**

```bash
git add internal/tui/shared/styles.go
git commit -m "feat(shared): add AccentBoldStyle for reuse across view section headers"
```

---

### Task 2: Remove local style aliases from `dashboard`, `sources`, and `pipeline`

These three packages declare local vars that are byte-for-byte aliases of shared styles. Delete them and use the shared names directly.

**Files:**
- Modify: `internal/tui/dashboard/view.go`
- Modify: `internal/tui/sources/view.go`
- Modify: `internal/tui/pipeline/view.go`

**Step 1: Delete the alias block from `dashboard/view.go` (lines 15–23)**

```go
// DELETE this entire block:
var (
    headerStyle   = shared.HeaderStyle
    mutedStyle    = shared.MutedStyle
    accentStyle   = shared.AccentStyle
    warningStyle  = shared.WarningStyle
    successStyle  = shared.SuccessStyle
    errorStyle    = shared.ErrorStyle
    selectedStyle = shared.SelectedStyle
)
```

Then replace every usage in the file:
- `headerStyle` → `shared.HeaderStyle`
- `mutedStyle` → `shared.MutedStyle`
- `accentStyle` → `shared.AccentStyle`
- `warningStyle` → `shared.WarningStyle`
- `successStyle` → `shared.SuccessStyle`
- `errorStyle` → `shared.ErrorStyle`
- `selectedStyle` → `shared.SelectedStyle`

**Step 2: Delete the alias block from `sources/view.go` (lines 27–36)**

Same pattern — delete the `var (...)` block, replace all 8 local names with `shared.*` equivalents.

**Step 3: Update `pipeline/view.go`**

`pipeline/view.go` has a mix: some direct aliases and a new composition:

```go
// REMOVE these aliases:
successStyle       = shared.SuccessStyle
warningStyle       = shared.WarningStyle
errorStyle         = shared.ErrorStyle
mutedStyle         = shared.MutedStyle

// KEEP titleStyle (it adds ColorFgBold — a deliberate styling choice specific to pipeline headers):
titleStyle         = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorFgBold)

// REPLACE sectionHeaderStyle with shared.AccentBoldStyle:
// sectionHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorAccent)
// → replace all usages with shared.AccentBoldStyle
```

Replace each usage of `sectionHeaderStyle` in `pipeline/view.go` with `shared.AccentBoldStyle`.
Replace `successStyle`, `warningStyle`, `errorStyle`, `mutedStyle` with `shared.*Style`.

**Step 4: Build all three packages**

```bash
go build ./internal/tui/dashboard/ ./internal/tui/sources/ ./internal/tui/pipeline/
```

**Step 5: Run tests**

```bash
go test ./internal/tui/dashboard/... ./internal/tui/sources/... ./internal/tui/pipeline/...
```

Expected: all pass — these tests use `ansi.Strip()` so they are color-blind.

**Step 6: Commit**

```bash
git add internal/tui/dashboard/view.go internal/tui/sources/view.go internal/tui/pipeline/view.go
git commit -m "refactor(tui): remove local style aliases — use shared.*Style directly

dashboard, sources, and pipeline each maintained a local copy of the same
7-8 shared styles. Changing a shared style (e.g. adding italic to section
headers) required editing 5 files. Now they reference shared.* directly.
pipeline.sectionHeaderStyle replaced with shared.AccentBoldStyle."
```

---

### Task 3: Replace local styles in `detail` and `fitness`

`detail/view.go` and `fitness/view.go` create *new* style objects with the same properties as shared ones (not aliases — independent objects with the same values). Replace them with shared equivalents.

**Files:**
- Modify: `internal/tui/detail/view.go`
- Modify: `internal/tui/fitness/view.go`

**Step 1: Update `detail/view.go`**

Current (lines 16–20):
```go
var (
    detailHeader  = lipgloss.NewStyle().Bold(true)
    detailMuted   = lipgloss.NewStyle().Foreground(shared.ColorMuted)
    detailSection = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorAccent)
    detailAccent  = lipgloss.NewStyle().Foreground(shared.ColorAccent)
)
```

Delete this block. Replace usages:
- `detailHeader` → `shared.HeaderStyle`
- `detailMuted` → `shared.MutedStyle`
- `detailSection` → `shared.AccentBoldStyle`
- `detailAccent` → `shared.AccentStyle`

**Step 2: Update `fitness/view.go`**

Current (lines 13–18):
```go
var (
    fitnessHeader  = lipgloss.NewStyle().Bold(true)
    fitnessMuted   = lipgloss.NewStyle().Foreground(shared.ColorMuted)
    fitnessSection = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorAccent)
    fitnessAccent  = lipgloss.NewStyle().Foreground(shared.ColorAccent)
)
```

Delete this block. Replace usages:
- `fitnessHeader` → `shared.HeaderStyle`
- `fitnessMuted` → `shared.MutedStyle`
- `fitnessSection` → `shared.AccentBoldStyle`
- `fitnessAccent` → `shared.AccentStyle`

**Step 3: Build and test**

```bash
go build ./internal/tui/detail/ ./internal/tui/fitness/
go test ./internal/tui/detail/... ./internal/tui/fitness/...
```

**Step 4: Commit**

```bash
git add internal/tui/detail/view.go internal/tui/fitness/view.go
git commit -m "refactor(tui): replace detail/fitness local styles with shared.*Style"
```

---

### Task 4: Add `shared.RenderSubHeader` and unify all `SubHeader()` implementations

Every view implements `SubHeader()` returning `title\nstats`. The title style and stats muting differ. Unify behind one helper.

**Files:**
- Modify: `internal/tui/shared/format.go`
- Modify: `internal/tui/dashboard/view.go`
- Modify: `internal/tui/detail/view.go`
- Modify: `internal/tui/fitness/view.go`
- Modify: `internal/tui/sources/view.go`
- Modify: `internal/tui/pipeline/view.go`
- Modify: `internal/tui/logview/view.go`
- Test: `internal/tui/shared/format_test.go`

**Step 1: Write failing test**

```go
func TestRenderSubHeader(t *testing.T) {
    result := RenderSubHeader("  Proposals", "  3 awaiting review")
    stripped := ansi.Strip(result)
    if !strings.Contains(stripped, "Proposals") {
        t.Error("SubHeader should contain title")
    }
    if !strings.Contains(stripped, "3 awaiting review") {
        t.Error("SubHeader should contain stats")
    }
    // Should be exactly two lines.
    if strings.Count(result, "\n") != 1 {
        t.Errorf("SubHeader should have exactly 1 newline, got %d", strings.Count(result, "\n"))
    }
}
```

**Step 2: Add `RenderSubHeader` to `shared/format.go`**

```go
// RenderSubHeader renders the standard two-line view sub-header:
// a bold title on the first line and a muted stats string on the second.
func RenderSubHeader(title, stats string) string {
    return HeaderStyle.Render(title) + "\n" + MutedStyle.Render(stats)
}
```

**Step 3: Verify test passes**

```bash
cd internal/tui/shared && go test -run TestRenderSubHeader -v
```

**Step 4: Update each `SubHeader()` implementation**

**`dashboard/view.go`** — `SubHeader()`:
```go
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
    return shared.RenderSubHeader("  Proposals", statsLine)
}
```

**`detail/view.go`** — `SubHeader()`:
```go
func (m Model) SubHeader() string {
    if m.proposal == nil {
        return shared.RenderSubHeader("  Proposal Detail", "")
    }
    p := &m.proposal.Proposal
    statsLine := fmt.Sprintf("  %s  ·  %s  ·  %s", p.Type, shared.ShortenHome(p.Target), p.Confidence)
    return shared.RenderSubHeader("  Proposal Detail", statsLine)
}
```

**`fitness/view.go`** — `SubHeader()`:
```go
func (m Model) SubHeader() string {
    if m.report == nil {
        return shared.RenderSubHeader("  Fitness Report", "")
    }
    r := m.report
    statsLine := fmt.Sprintf("  %s  ·  ownership: %s  ·  %d sessions",
        r.SourceName, r.Ownership, r.ObservedCount)
    return shared.RenderSubHeader("  Fitness Report", statsLine)
}
```

**`sources/view.go`** — `renderHeader()` (called from `SubHeader()`):
```go
func (m Model) renderHeader() string {
    total, iterate, evaluate, unclassified := m.sourceCounts()
    stats := fmt.Sprintf("  %d sources", total)
    if iterate > 0 {
        stats += fmt.Sprintf("  ·  %d iterate", iterate)
    }
    if evaluate > 0 {
        stats += fmt.Sprintf("  ·  %d evaluate", evaluate)
    }
    if unclassified > 0 {
        stats += fmt.Sprintf("  ·  %d unclassified", unclassified)
    }
    return shared.RenderSubHeader("  Source Manager", stats)
}

func (m Model) detailSubHeader() string {
    if m.detailSource == nil {
        return shared.RenderSubHeader("  Source Detail", "")
    }
    return shared.RenderSubHeader("  Source Detail", "  "+m.detailSource.Name)
}
```

**`pipeline/view.go`** — `SubHeader()`:
```go
func (m Model) SubHeader() string {
    statsLine := fmt.Sprintf("  captured: %d  ·  processed: %d  ·  queued: %d",
        m.stats.SessionsCaptured, m.stats.SessionsProcessed, m.stats.SessionsQueued)
    return shared.RenderSubHeader("  Pipeline Monitor", statsLine)
}
```

Note: pipeline previously used `titleStyle` (Bold + `ColorFgBold`) for the title. `shared.HeaderStyle` is Bold only — functionally identical on dark terminals (foreground is already white), and correct on light terminals where `ColorFgBold` maps to black. Use `shared.HeaderStyle`.

**`logview/view.go`** — `SubHeader()`:
```go
func (m Model) SubHeader() string {
    var followIndicator string
    if m.followMode {
        followIndicator = shared.SuccessStyle.Render("●")
    } else {
        followIndicator = shared.MutedStyle.Render("○")
    }
    statsLine := fmt.Sprintf("  %d entries  ·  follow %s", len(m.entries), followIndicator)
    return shared.RenderSubHeader("  Log Viewer", statsLine)
}
```

Previously the stats line was rendered without muting. Now it's wrapped in `MutedStyle` via `RenderSubHeader`. The follow indicator preserves its own color since it's pre-rendered before being embedded in `statsLine`.

**Step 5: Build and test everything**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

**Step 6: Commit**

```bash
git add internal/tui/shared/format.go internal/tui/dashboard/view.go internal/tui/detail/view.go internal/tui/fitness/view.go internal/tui/sources/view.go internal/tui/pipeline/view.go internal/tui/logview/view.go
git commit -m "refactor(tui): unify SubHeader() via shared.RenderSubHeader

Each view reimplemented the same two-line title+stats pattern with
inconsistent style choices (some titles Bold, some Bold+Foreground;
stats line muted in most views but plain in logview).

shared.RenderSubHeader(title, stats) is the single canonical implementation:
HeaderStyle for title, MutedStyle for stats. All six views now delegate to it."
```

---

### Task 5: Unify section header separator convention

The section header pattern (`"  SECTION NAME"` + optional `"  " + strings.Repeat("─", 17)`) is used in `detail` and `fitness` but absent in `sources` and `pipeline`. The magic number `17` is unexplained. Standardize: always include the separator, always derive its length from a named constant.

**Files:**
- Modify: `internal/tui/shared/format.go`
- Modify: `internal/tui/detail/model.go`
- Modify: `internal/tui/fitness/view.go`
- Modify: `internal/tui/sources/view.go`
- Modify: `internal/tui/pipeline/view.go`

**Step 1: Add `RenderSectionHeader` to `shared/format.go`**

```go
// sectionSeparatorLen is the character width of the separator under section titles.
// Matches the visual width of the longest section title ("PROPOSED CHANGE" = 15) plus indent.
const sectionSeparatorLen = 17

// RenderSectionHeader renders a bold accent section title with a separator line below it.
// Both title and separator are indented by two spaces.
func RenderSectionHeader(title string) string {
    return AccentBoldStyle.Render("  "+title) + "\n" + "  " + strings.Repeat("─", sectionSeparatorLen)
}
```

Add `"strings"` import if not present.

**Step 2: Write a test**

```go
func TestRenderSectionHeader(t *testing.T) {
    result := RenderSectionHeader("ASSESSMENT")
    stripped := ansi.Strip(result)
    if !strings.Contains(stripped, "ASSESSMENT") {
        t.Error("section header should contain title")
    }
    if !strings.Contains(stripped, "─") {
        t.Error("section header should contain separator")
    }
    if strings.Count(result, "\n") != 1 {
        t.Errorf("section header should have 1 newline, got %d", strings.Count(result, "\n"))
    }
}
```

**Step 3: Replace section headers in `detail/model.go`**

Find all occurrences of the pattern:
```bash
grep -n 'Render("  [A-Z]' internal/tui/detail/model.go
```

Replace each block like:
```go
// Before:
b.WriteString(detailSection.Render("  PROPOSED CHANGE"))
b.WriteString("\n")
b.WriteString("  " + strings.Repeat("─", 17))
b.WriteString("\n")

// After:
b.WriteString(shared.RenderSectionHeader("PROPOSED CHANGE"))
b.WriteString("\n")
```

**Step 4: Replace section headers in `fitness/view.go`**

Same pattern — find and replace all three occurrences (ASSESSMENT, VERDICT, SESSION EVIDENCE).

**Step 5: Add section headers to `sources/view.go` (renderDetail)**

Currently `sources/view.go renderDetail()` uses:
```go
b.WriteString("  " + sectionHeaderStyle.Render("INFO") + "\n\n")
```

Replace with:
```go
b.WriteString(shared.RenderSectionHeader("INFO"))
b.WriteString("\n")
```

Do the same for the `"RECENT CHANGES"` section.

**Step 6: Add section headers to `pipeline/view.go`**

Replace each `"  " + sectionHeaderStyle.Render("DAEMON")` + `"\n"` pattern with `shared.RenderSectionHeader("DAEMON") + "\n"` (no separator was there before — we're adding it). Do this for: DAEMON, HOOKS, STORE, PIPELINE ACTIVITY, RECENT RUNS, MODELS, PROMPTS.

**Step 7: Build and test**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

**Step 8: Update snapshots**

```bash
make snapshots
```

Review all changed snapshots visually. Commit the regenerated files.

**Step 9: Commit**

```bash
git add internal/tui/shared/format.go internal/tui/detail/model.go internal/tui/fitness/view.go internal/tui/sources/view.go internal/tui/pipeline/view.go snapshots/
git commit -m "refactor(tui): unify section header rendering via shared.RenderSectionHeader

The '  SECTION TITLE\n  ────────────────' pattern existed in detail and
fitness but was absent in sources and pipeline. The magic literal 17 is
replaced by sectionSeparatorLen constant. All views now use the same helper,
giving consistent section headers throughout."
```
