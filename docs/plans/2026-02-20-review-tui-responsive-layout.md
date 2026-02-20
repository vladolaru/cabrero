# Responsive Pipeline Monitor Layout Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the responsive layout table from the Phase 4c design so the pipeline monitor adapts cleanly to wide (>=120), standard (80-119), and narrow (<80) terminals.

**Architecture:** Add a `layoutMode` helper to classify terminal width into three tiers. Each render function (`renderDaemonHeader`, `renderActivityStats`, `renderRecentRuns`, `renderPrompts`) checks the mode and adjusts content density accordingly. Follows the same pattern used by `sources/view.go:columnLayout()`.

**Tech Stack:** Go, Bubble Tea, Lip Gloss

**Worktree:** `/Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui/`

---

### Task 1: Add layoutMode helper and test the three breakpoints

**Context:** Every render function needs to know the terminal width tier. Rather than scattering `if m.width >= 120` checks everywhere, add a typed constant and a method. The sources view uses a similar `columnLayout()` approach.

**Files:**
- Modify: `internal/tui/pipeline/view.go` (add layoutMode type and method)
- Test: `internal/tui/pipeline/model_test.go` (test breakpoints)

**Step 1: Write the failing test**

In `model_test.go`, add:

```go
func TestLayoutMode(t *testing.T) {
	m := newTestModel()

	m.SetSize(120, 40)
	if m.layoutMode() != layoutWide {
		t.Errorf("width 120 should be wide, got %d", m.layoutMode())
	}

	m.SetSize(100, 40)
	if m.layoutMode() != layoutStandard {
		t.Errorf("width 100 should be standard, got %d", m.layoutMode())
	}

	m.SetSize(79, 40)
	if m.layoutMode() != layoutNarrow {
		t.Errorf("width 79 should be narrow, got %d", m.layoutMode())
	}

	// Boundary: 80 is standard, not narrow.
	m.SetSize(80, 40)
	if m.layoutMode() != layoutStandard {
		t.Errorf("width 80 should be standard, got %d", m.layoutMode())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/pipeline/ -run TestLayoutMode -v`
Expected: FAIL (layoutMode undefined)

**Step 3: Implement layoutMode**

In `internal/tui/pipeline/view.go`, add near the top (after the style vars):

```go
type layout int

const (
	layoutNarrow   layout = iota // < 80
	layoutStandard               // 80-119
	layoutWide                   // >= 120
)

func (m Model) layoutMode() layout {
	switch {
	case m.width >= 120:
		return layoutWide
	case m.width >= 80:
		return layoutStandard
	default:
		return layoutNarrow
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/tui/pipeline/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/pipeline/view.go internal/tui/pipeline/model_test.go
git commit -m "refactor(pipeline): add layoutMode helper for responsive breakpoints"
```

---

### Task 2: Abbreviated narrow daemon header

**Context:** The design says narrow mode shows an "Abbreviated" daemon header. Currently the header always shows all fields (status, uptime, poll/stale/delay, hooks, store). In narrow mode, omit the intervals (poll/stale/delay) and the STORE section to save vertical space. Keep the essentials: status+PID, uptime, and hooks.

Also refactor `renderDaemonHeader()` to use `layoutMode()` instead of the raw `m.width >= 120` check.

**Files:**
- Modify: `internal/tui/pipeline/view.go` (renderDaemonHeader)
- Modify: `internal/tui/pipeline/model_test.go` (update TestModelViewNarrowLayout)

**Step 1: Write the failing test**

In `model_test.go`, replace `TestModelViewNarrowLayout` with a more specific test:

```go
func TestModelViewNarrowLayout(t *testing.T) {
	m := newTestModel()
	m.SetSize(70, 40) // narrow mode (< 80)
	view := ansi.Strip(m.View())

	// Essentials should be present.
	if !strings.Contains(view, "DAEMON") {
		t.Error("narrow view missing DAEMON section")
	}
	if !strings.Contains(view, "HOOKS") {
		t.Error("narrow view missing HOOKS section")
	}

	// Abbreviated: no intervals, no STORE section.
	if strings.Contains(view, "Poll:") {
		t.Error("narrow view should not show Poll interval")
	}
	if strings.Contains(view, "STORE") {
		t.Error("narrow view should not show STORE section")
	}

	// Prompts should be hidden in narrow mode.
	if strings.Contains(view, "PROMPTS") {
		t.Error("narrow view should not show PROMPTS section")
	}
}

func TestModelViewStandardLayout(t *testing.T) {
	m := newTestModel()
	m.SetSize(100, 40) // standard mode (80-119)
	view := ansi.Strip(m.View())

	// All sections present, stacked (not side-by-side).
	if !strings.Contains(view, "DAEMON") {
		t.Error("standard view missing DAEMON section")
	}
	if !strings.Contains(view, "STORE") {
		t.Error("standard view missing STORE section")
	}
	if !strings.Contains(view, "PROMPTS") {
		t.Error("standard view missing PROMPTS section")
	}

	// Stacked: DAEMON and HOOKS should NOT share a line.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "DAEMON") && strings.Contains(line, "HOOKS") {
			t.Error("standard view should stack DAEMON and HOOKS, not side-by-side")
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/pipeline/ -run "TestModelViewNarrowLayout|TestModelViewStandardLayout" -v`
Expected: FAIL (narrow still shows Poll/STORE/PROMPTS)

**Step 3: Implement abbreviated narrow header**

In `internal/tui/pipeline/view.go`, replace the `renderDaemonHeader()` method:

```go
func (m Model) renderDaemonHeader() string {
	mode := m.layoutMode()

	// Build left column: DAEMON section.
	var left strings.Builder
	left.WriteString(sectionHeaderStyle.Render("DAEMON"))
	left.WriteString("\n")
	if m.dashStats.DaemonRunning {
		left.WriteString(fmt.Sprintf("  Status:  %s (PID %d)\n", successStyle.Render("● running"), m.dashStats.DaemonPID))
		if m.dashStats.DaemonStartTime != nil {
			left.WriteString(fmt.Sprintf("  Uptime:  %s\n", formatUptime(time.Since(*m.dashStats.DaemonStartTime))))
		}
	} else {
		left.WriteString(fmt.Sprintf("  Status:  %s\n", errorStyle.Render("● stopped")))
	}
	// Intervals: shown in standard and wide, omitted in narrow.
	if mode != layoutNarrow && m.dashStats.PollInterval > 0 {
		left.WriteString(fmt.Sprintf("  Poll:    every %s\n", formatInterval(m.dashStats.PollInterval)))
		left.WriteString(fmt.Sprintf("  Stale:   every %s\n", formatInterval(m.dashStats.StaleInterval)))
		left.WriteString(fmt.Sprintf("  Delay:   %s", formatInterval(m.dashStats.InterSessionDelay)))
	}

	// Build right column: HOOKS + STORE (store hidden in narrow).
	var right strings.Builder
	right.WriteString(sectionHeaderStyle.Render("HOOKS"))
	right.WriteString("\n")
	right.WriteString(fmt.Sprintf("  pre-compact:  %s\n", checkmark(m.dashStats.HookPreCompact)))
	right.WriteString(fmt.Sprintf("  session-end:  %s", checkmark(m.dashStats.HookSessionEnd)))

	if mode != layoutNarrow {
		right.WriteString("\n\n")
		right.WriteString(sectionHeaderStyle.Render("STORE"))
		right.WriteString("\n")
		right.WriteString(fmt.Sprintf("  Path: %s\n", m.dashStats.StorePath))
		right.WriteString(fmt.Sprintf("  Raw:  %d sessions\n", m.dashStats.SessionCount))
		right.WriteString(fmt.Sprintf("  Disk: %s", formatBytes(m.dashStats.DiskBytes)))
	}

	// Layout: wide = two-column, standard/narrow = stacked.
	if mode == layoutWide {
		colWidth := m.width / 2
		leftStyle := lipgloss.NewStyle().Width(colWidth)
		return lipgloss.JoinHorizontal(lipgloss.Top, leftStyle.Render(left.String()), right.String())
	}
	return left.String() + "\n\n" + right.String()
}
```

Also update `View()` to hide prompts in narrow mode:

```go
// Prompts: hidden in narrow mode.
if len(m.prompts) > 0 && m.layoutMode() != layoutNarrow {
    sections = append(sections, m.renderPrompts())
}
```

**Step 4: Run tests**

Run: `go test ./internal/tui/pipeline/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/pipeline/view.go internal/tui/pipeline/model_test.go
git commit -m "feat(pipeline): abbreviated narrow header and hide prompts below 80 cols"
```

---

### Task 3: Responsive activity stats (stacked narrow + hide sparkline)

**Context:** The design specifies:
- Wide/Standard: 2-column layout (sessions left, proposals right) + sparkline
- Narrow: Stacked (one stat per line) and sparkline hidden

Currently `renderActivityStats()` always uses the 2-column format.

**Files:**
- Modify: `internal/tui/pipeline/view.go` (renderActivityStats)
- Modify: `internal/tui/pipeline/model_test.go` (add narrow activity test)

**Step 1: Write the failing test**

In `model_test.go`, add:

```go
func TestModelViewNarrowActivityStats(t *testing.T) {
	m := newTestModel()
	m.SetSize(70, 40) // narrow
	view := ansi.Strip(m.View())

	// Activity section should still be present.
	if !strings.Contains(view, "PIPELINE ACTIVITY") {
		t.Error("narrow view missing PIPELINE ACTIVITY section")
	}

	// Sparkline should be hidden in narrow mode.
	if strings.Contains(view, "sessions/day") {
		t.Error("narrow view should not show sparkline")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/pipeline/ -run TestModelViewNarrowActivityStats -v`
Expected: FAIL (sparkline still shown)

**Step 3: Implement responsive activity stats**

In `internal/tui/pipeline/view.go`, replace `renderActivityStats()`:

```go
func (m Model) renderActivityStats() string {
	mode := m.layoutMode()
	var b strings.Builder
	days := m.config.Pipeline.SparklineDays
	b.WriteString(sectionHeaderStyle.Render(fmt.Sprintf("PIPELINE ACTIVITY (last %d days)", days)))
	b.WriteString("\n")

	if mode == layoutNarrow {
		// Stacked: one stat per line.
		b.WriteString(fmt.Sprintf("  Captured:  %d   Processed: %d\n", m.stats.SessionsCaptured, m.stats.SessionsProcessed))
		b.WriteString(fmt.Sprintf("  Pending:   %d   Errored:   %d\n", m.stats.SessionsPending, m.stats.SessionsErrored))
		b.WriteString(fmt.Sprintf("  Proposals: %d gen  %d ok  %d rej", m.stats.ProposalsGenerated, m.stats.ProposalsApproved, m.stats.ProposalsRejected))
	} else {
		// Wide/standard: 2-column layout.
		b.WriteString(fmt.Sprintf("  Sessions captured:  %-6d Proposals generated:  %d\n",
			m.stats.SessionsCaptured, m.stats.ProposalsGenerated))
		b.WriteString(fmt.Sprintf("  Sessions processed: %-6d Proposals approved:   %d\n",
			m.stats.SessionsProcessed, m.stats.ProposalsApproved))
		b.WriteString(fmt.Sprintf("  Sessions pending:   %-6d Proposals rejected:   %d\n",
			m.stats.SessionsPending, m.stats.ProposalsRejected))
		b.WriteString(fmt.Sprintf("  Sessions errored:   %-6d Proposals pending:    %d",
			m.stats.SessionsErrored, m.stats.ProposalsPending))

		if len(m.stats.SessionsPerDay) > 0 {
			sparkline := components.RenderSparkline(m.stats.SessionsPerDay, m.width-4)
			b.WriteString("\n\n  " + sparkline + "  sessions/day")
		}
	}

	return b.String()
}
```

**Step 4: Run tests**

Run: `go test ./internal/tui/pipeline/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/pipeline/view.go internal/tui/pipeline/model_test.go
git commit -m "feat(pipeline): responsive activity stats with stacked narrow and hidden sparkline"
```

---

### Task 4: Responsive recent runs (ID, project, timing)

**Context:** The most complex responsive element. The design specifies three levels of detail:

| Element          | Wide (>=120)  | Standard (80-119) | Narrow (<80)   |
|------------------|---------------|---------------------|----------------|
| Session ID       | 8 chars       | 8 chars             | 6 chars        |
| Project name     | 20 chars      | 15 chars            | 10 chars       |
| Per-stage timing | All 3 stages  | 2 stages            | Total only     |

Currently `renderRecentRuns()` uses hardcoded 8-char ID, 16-char project, and all 3 stages. The `truncateID` function lives in `update.go` and always returns 8 chars. Need to add a width-aware version and make `formatTiming` width-aware.

**Files:**
- Modify: `internal/tui/pipeline/view.go` (renderRecentRuns, formatTiming, add helpers)
- Modify: `internal/tui/pipeline/model_test.go` (test run row layout at each width)

**Step 1: Write the failing test**

In `model_test.go`, add:

```go
func TestModelViewRunRowWide(t *testing.T) {
	m := newTestModel()
	m.SetSize(140, 40)
	view := ansi.Strip(m.View())

	// Wide: 8-char ID, full project name (up to 20), all 3 timing stages.
	if !strings.Contains(view, "e7f2a103") {
		t.Error("wide view should show 8-char session ID")
	}
	// All 3 timing columns.
	if !strings.Contains(view, "parse") || !strings.Contains(view, "cls") || !strings.Contains(view, "eval") {
		t.Error("wide view should show all 3 timing stages")
	}
}

func TestModelViewRunRowStandard(t *testing.T) {
	m := newTestModel()
	m.SetSize(100, 40)
	view := ansi.Strip(m.View())

	// Standard: 8-char ID still shown.
	if !strings.Contains(view, "e7f2a103") {
		t.Error("standard view should show 8-char session ID")
	}
	// Standard: 2 stages (parse + eval) — classifier omitted.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "e7f2a103") {
			if strings.Contains(line, "cls") {
				t.Error("standard view should omit classifier timing (show 2 stages)")
			}
			break
		}
	}
}

func TestModelViewRunRowNarrow(t *testing.T) {
	m := newTestModel()
	m.SetSize(70, 40)
	view := ansi.Strip(m.View())

	// Narrow: 6-char ID.
	if !strings.Contains(view, "e7f2a1") {
		t.Error("narrow view should show 6-char session ID")
	}
	// Ensure the full 8-char ID is NOT shown (would mean no truncation).
	// Check specific run line does not have the full 8-char + space pattern.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "e7f2a103") {
			t.Error("narrow view should truncate session ID to 6 chars, not 8")
			break
		}
	}
	// Narrow: total-only timing (no stage names).
	for _, line := range lines {
		if strings.Contains(line, "e7f2a1") {
			if strings.Contains(line, "parse") || strings.Contains(line, "cls") || strings.Contains(line, "eval") {
				t.Error("narrow view should show total timing only, not per-stage")
			}
			break
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/pipeline/ -run "TestModelViewRunRow" -v`
Expected: FAIL (standard still shows cls, narrow still shows 8-char ID)

**Step 3: Implement responsive recent runs**

In `internal/tui/pipeline/view.go`, add width-aware helpers and update `renderRecentRuns`:

```go
// runLayout returns the display parameters for a run row based on terminal width.
func (m Model) runLayout() (idLen int, projectMax int) {
	switch m.layoutMode() {
	case layoutWide:
		return 8, 20
	case layoutStandard:
		return 8, 15
	default: // narrow
		return 6, 10
	}
}

func (m Model) renderRecentRuns() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("RECENT RUNS"))
	b.WriteString("\n")

	idLen, projectMax := m.runLayout()
	mode := m.layoutMode()

	for i, run := range m.runs {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		status := statusIndicator(run.Status)
		shortID := truncate(run.SessionID, idLen)
		age := relativeTime(run.Timestamp)
		project := truncate(run.Project, projectMax)
		timing := formatTimingForMode(run, mode)

		line := fmt.Sprintf("%s%s %s  %-8s  %-*s  %s", cursor, status, shortID, age, projectMax, project, timing)
		b.WriteString(line)

		// Inline expansion.
		if i == m.expandedIdx {
			b.WriteString("\n")
			b.WriteString(m.renderRunDetail(run))
		}

		if i < len(m.runs)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
```

Add a new `formatTimingForMode` function (keeping the old `formatTiming` for backward compat is not needed — replace it):

```go
// formatTimingForMode formats per-stage timing based on layout mode.
//   - Wide: all 3 stages (parse, cls, eval)
//   - Standard: 2 stages (parse, eval) — classifier omitted
//   - Narrow: total duration only
func formatTimingForMode(run pl.PipelineRun, mode layout) string {
	if run.Status == "pending" {
		return mutedStyle.Render("(pending)")
	}

	if mode == layoutNarrow {
		total := run.ParseDuration + run.ClassifierDuration + run.EvaluatorDuration
		if total == 0 {
			return ""
		}
		return fmt.Sprintf("%.0fs", total.Seconds())
	}

	var parts []string
	if run.HasDigest {
		parts = append(parts, fmt.Sprintf("%.1fs parse", run.ParseDuration.Seconds()))
	}
	if mode == layoutWide {
		if run.HasClassifier {
			parts = append(parts, fmt.Sprintf("%.1fs cls", run.ClassifierDuration.Seconds()))
		} else if run.Status == "error" && run.HasDigest {
			parts = append(parts, errorStyle.Render("✗ cls failed"))
		}
	}
	if run.HasEvaluator {
		parts = append(parts, fmt.Sprintf("%.0fs eval", run.EvaluatorDuration.Seconds()))
	} else if run.Status == "error" && run.HasClassifier {
		parts = append(parts, errorStyle.Render("✗ eval failed"))
	}
	return strings.Join(parts, "  ")
}
```

Delete the old `formatTiming` function (it's fully replaced by `formatTimingForMode`).

**Step 4: Run tests**

Run: `go test ./internal/tui/pipeline/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/pipeline/view.go internal/tui/pipeline/model_test.go
git commit -m "feat(pipeline): responsive recent runs with adaptive ID, project, and timing"
```

---

### Task 5: Update documentation

**Files:**
- Modify: `docs/plans/2026-02-20-review-tui-phase4c-design.md` (update Known Simplifications)
- Modify: `CHANGELOG.md` (add entry)

**Step 1: Update design doc**

In `docs/plans/2026-02-20-review-tui-phase4c-design.md`, replace the "Responsive layout" Known Simplification with:

```markdown
### Responsive layout

The responsive layout table is fully implemented for the pipeline monitor.
Wide (>=120), standard (80-119), and narrow (<80) modes adjust daemon header
content, activity stats format, sparkline visibility, run row detail level
(ID length, project width, timing stages), and prompt visibility. Full
responsive behavior for the dashboard and other views is out of scope for
Phase 4c.
```

**Step 2: Update CHANGELOG.md**

Add under `[Unreleased]` → `### Added`:

```markdown
- **Pipeline monitor responsive layout** — three-tier layout (wide/standard/narrow)
  adapts daemon header density, activity stats format, sparkline visibility,
  run row detail level, and prompt section visibility to terminal width.
```

**Step 3: Commit**

```bash
git add docs/plans/2026-02-20-review-tui-phase4c-design.md CHANGELOG.md
git commit -m "docs: document responsive layout completion for pipeline monitor"
```

---

## Verification

After all tasks, run:

```bash
go test ./... -v
go build ./...
```

All tests should pass and the binary should compile cleanly.
