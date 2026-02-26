# TUI Uniformization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bring consistency to the Cabrero TUI by renaming the `review` command to `dashboard`, centralizing sub-headers across all views, and making the help overlay preserve sub-headers.

**Architecture:** Each view model gains a `SubHeader(width int) string` method. The root model renders `header + separator + subheader + separator + content` for all views. Proposal stats move from the persistent header to the dashboard sub-header. Help overlays replace only the content area below the sub-header.

**Tech Stack:** Go, Bubble Tea (charmbracelet), lipgloss

---

### Task 1: Rename `review` command to `dashboard`

Rename the CLI command, file, function, and root model type. This is a mechanical rename with no behavior change.

**Files:**
- Rename: `internal/cmd/review.go` → `internal/cmd/dashboard.go`
- Modify: `main.go:25,96-98`
- Modify: `internal/tui/model.go` (all `reviewModel` references)
- Modify: `internal/tui/tui.go:63`
- Modify: `internal/tui/integration_test.go` (all `reviewModel` and `newReviewModel` references)
- Modify: package comments in `internal/tui/config.go:1`, `internal/tui/message/message.go:1`, `internal/tui/pipeline/model.go:1`, `internal/tui/fitness/model.go:1`, `internal/tui/sources/model.go:1`

**Step 1: Rename file and function**

Rename `internal/cmd/review.go` to `internal/cmd/dashboard.go`. Change function name `Review` → `Dashboard`:

```go
// Dashboard launches the interactive dashboard TUI.
func Dashboard(args []string, version string) error {
	return tui.Run(version)
}
```

**Step 2: Update main.go**

In `main.go:25`, change command entry:
```go
{"dashboard", "Interactive dashboard", cmdDashboard},
```

At `main.go:96-98`, rename `cmdReview` → `cmdDashboard`:
```go
func cmdDashboard(args []string) error {
	return cmd.Dashboard(args, version)
}
```

**Step 3: Rename reviewModel → appModel in model.go**

In `internal/tui/model.go`, rename all occurrences:
- `reviewModel` → `appModel` (struct type, all method receivers, type assertions)
- `newReviewModel` → `newAppModel`
- Comment on line 30: `"reviewModel is the root Bubble Tea model"` → `"appModel is the root Bubble Tea model for the TUI."`

**Step 4: Update tui.go**

In `internal/tui/tui.go:21`, update comment:
```go
// Run launches the interactive TUI dashboard.
```

At line 63, change `newReviewModel` → `newAppModel`.

**Step 5: Update integration_test.go**

In `internal/tui/integration_test.go`, rename all `reviewModel` → `appModel` and `newReviewModel` → `newAppModel`. Update comment on line 27-28 and line 15.

**Step 6: Update package comments**

Update "review TUI" → "TUI" or "dashboard TUI" in:
- `internal/tui/config.go:1`
- `internal/tui/message/message.go:1`
- `internal/tui/pipeline/model.go:1-3`
- `internal/tui/fitness/model.go:1-3`
- `internal/tui/sources/model.go:1`

**Step 7: Run tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go test ./...`
Expected: All tests pass, no compile errors.

**Step 8: Build and verify**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build -o /dev/null .`
Expected: Clean build.

**Step 9: Commit**

```
refactor(tui): rename review command to dashboard

The TUI has evolved beyond proposal review into a full operational
dashboard. Rename the CLI command from "review" to "dashboard" and
the root model from reviewModel to appModel to reflect this.
```

---

### Task 2: Simplify header — "Cabrero Review" → "Cabrero" and remove proposal stats

Remove the proposal stats line from the persistent header. The header retains only: title, version, daemon status, hooks, debug indicator.

**Files:**
- Modify: `internal/tui/dashboard/view.go:70-127` (`RenderHeader`)
- Modify: `internal/tui/integration_test.go:44` (update title assertion)

**Step 1: Update RenderHeader**

In `internal/tui/dashboard/view.go`, function `RenderHeader` (lines 70-127):

1. Change line 73 from `"  Cabrero Review"` to `"  Cabrero"`.
2. Remove the stats line entirely (lines 79-83 — the `statsLine` variable and its rendering).
3. Update the wide layout (line 110) and narrow layout (line 124) to remove `statsLine`.

The resulting function should render:
- Title: `"  Cabrero  v0.13.0"`
- Daemon/hooks info (unchanged)
- Debug indicator (unchanged)

Wide layout (>= 120):
```go
left := title
rightLines := []string{
    mutedStyle.Render("Daemon:") + " " + daemonStatus,
    lastCapture,
    hooks + debugIndicator,
}
return lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", strings.Join(rightLines, "\n"))
```

Narrow layout:
```go
daemonLine := "  " + mutedStyle.Render("Daemon:") + " " + daemonStatus
if lastCapture != "" {
    daemonLine += "  " + mutedStyle.Render("│") + "  " + lastCapture
}
return title + "\n" +
    daemonLine + "\n" +
    "  " + hooks + debugIndicator
```

**Step 2: Update integration test assertion**

In `internal/tui/integration_test.go:44`, change `"Cabrero Review"` to `"Cabrero"`.

**Step 3: Run tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go test ./internal/tui/...`
Expected: All tests pass.

**Step 4: Commit**

```
refactor(tui): simplify header to "Cabrero" and remove proposal stats

The persistent header now shows only the app name, version, daemon
status, and hooks. Proposal stats will move to the dashboard view's
sub-header in the next commit.
```

---

### Task 3: Add SubHeader method to dashboard view

Add a `SubHeader` method to the dashboard model that returns the proposals stats line. This is the content that was removed from the persistent header.

**Files:**
- Modify: `internal/tui/dashboard/view.go` (add `SubHeader` method)
- Modify: `internal/tui/dashboard/model.go` (may need to expose stats)

**Step 1: Add SubHeader method**

Add to `internal/tui/dashboard/view.go`, after the `RenderHeader` function:

```go
// SubHeader returns the view title and stats line for the dashboard.
func (m Model) SubHeader() string {
	title := headerStyle.Render("  Proposals")

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

	return title + "\n" + mutedStyle.Render(statsLine)
}
```

Check that `m.stats` is accessible on the dashboard Model (it's `message.DashboardStats`, stored at creation).

**Step 2: Run tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./...`
Expected: Clean build (no callers yet, just adding the method).

**Step 3: Commit**

```
feat(tui): add SubHeader method to dashboard view

Returns the proposals title and stats line (awaiting review, approved,
rejected, fitness reports) to be rendered as a centralized sub-header.
```

---

### Task 4: Add SubHeader methods to all other views

Add `SubHeader` methods to the remaining 6 view models: detail, fitness, sources, pipeline, logview. Sources also needs to handle the detail sub-view case.

**Files:**
- Modify: `internal/tui/detail/view.go` (add `SubHeader`)
- Modify: `internal/tui/fitness/view.go` (add `SubHeader`)
- Modify: `internal/tui/sources/view.go` (add `SubHeader` + `DetailSubHeader`)
- Modify: `internal/tui/pipeline/view.go` (add `SubHeader`)
- Modify: `internal/tui/logview/view.go` (add `SubHeader`)

**Step 1: Proposal Detail SubHeader**

Add to `internal/tui/detail/view.go`:

```go
// SubHeader returns the view title and contextual stats for the proposal detail.
func (m Model) SubHeader() string {
	title := detailHeader.Render("  Proposal Detail")
	if m.proposal == nil {
		return title
	}
	p := &m.proposal.Proposal
	statsLine := fmt.Sprintf("  %s  ·  %s  ·  %s",
		p.Type,
		shared.ShortenHome(p.Target),
		p.Confidence)
	return title + "\n" + detailMuted.Render(statsLine)
}
```

**Step 2: Fitness Detail SubHeader**

Add to `internal/tui/fitness/view.go`:

```go
// SubHeader returns the view title and contextual stats for the fitness report.
func (m Model) SubHeader() string {
	title := fitnessHeader.Render("  Fitness Report")
	if m.report == nil {
		return title
	}
	r := m.report
	statsLine := fmt.Sprintf("  %s  ·  ownership: %s  ·  %d sessions",
		r.SourceName, r.Ownership, r.ObservedCount)
	return title + "\n" + fitnessMuted.Render(statsLine)
}
```

**Step 3: Source Manager SubHeader (list + detail)**

In `internal/tui/sources/view.go`, rename existing `renderHeader` to align with the new pattern. The existing `renderHeader` (lines 96-113) already does what `SubHeader` needs. Add:

```go
// SubHeader returns the view title and stats line for the source list.
func (m Model) SubHeader() string {
	if m.detailOpen {
		return m.detailSubHeader()
	}
	return m.renderHeader()
}

// detailSubHeader returns the sub-header for the source detail sub-view.
func (m Model) detailSubHeader() string {
	title := headerStyle.Render("  Source Detail")
	if m.detailIdx < 0 || m.detailIdx >= len(m.flatItems) {
		return title
	}
	src := m.flatItems[m.detailIdx].source
	if src == nil {
		return title
	}
	statsLine := fmt.Sprintf("  %s  ·  %s  ·  %s", src.Name, src.Ownership, src.Approach)
	return title + "\n" + mutedStyle.Render(statsLine)
}
```

**Step 4: Pipeline Monitor SubHeader**

Add to `internal/tui/pipeline/view.go`:

```go
// SubHeader returns the view title and stats line for the pipeline monitor.
func (m Model) SubHeader() string {
	title := "  " + titleStyle.Render("Pipeline Monitor")
	statsLine := fmt.Sprintf("  captured: %d  ·  processed: %d  ·  queued: %d",
		m.dashStats.SessionCount, m.stats.ProcessedSessions, m.stats.QueuedSessions)
	return title + "\n" + mutedStyle.Render(statsLine)
}
```

Verify the `PipelineStats` struct has `ProcessedSessions` and `QueuedSessions` fields. If not, use fields that exist (check `internal/pipeline/stats.go`).

**Step 5: Log Viewer SubHeader**

Add to `internal/tui/logview/view.go`:

```go
// SubHeader returns the view title and stats for the log viewer.
func (m Model) SubHeader() string {
	var followIndicator string
	if m.followMode {
		followIndicator = followOnStyle.Render("●")
	} else {
		followIndicator = followOffStyle.Render("○")
	}
	title := "  " + titleStyle.Render("Log Viewer")
	statsLine := fmt.Sprintf("  %d entries  ·  follow %s", len(m.entries), followIndicator)
	return title + "\n" + statsLine
}
```

**Step 6: Run tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./...`
Expected: Clean build.

**Step 7: Commit**

```
feat(tui): add SubHeader methods to all views

Each view model now exposes a SubHeader() method returning a title
and contextual stats line. These will be rendered by the root model
as a centralized sub-header between the persistent header and content.
```

---

### Task 5: Centralize sub-header rendering in the root model

Wire the sub-headers into the root model's `View()` method. Update height calculation to account for sub-header + extra separator.

**Files:**
- Modify: `internal/tui/model.go:419-467` (`View` method)
- Modify: `internal/tui/model.go:106-109` (`WindowSizeMsg` handling — height calculation)
- Modify: `internal/tui/model.go:589-592` (`childHeight` method)

**Step 1: Add subHeader helper to root model**

Add a helper method that calls the correct child's `SubHeader()`:

```go
// subHeader returns the sub-header for the currently active view.
func (m appModel) subHeader() string {
	switch m.state {
	case message.ViewDashboard:
		return m.dashboard.SubHeader()
	case message.ViewProposalDetail:
		return m.detail.SubHeader()
	case message.ViewFitnessDetail:
		return m.fitness.SubHeader()
	case message.ViewSourceManager, message.ViewSourceDetail:
		return m.sources.SubHeader()
	case message.ViewPipelineMonitor:
		return m.pipelineMonitor.SubHeader()
	case message.ViewLogViewer:
		return m.logViewer.SubHeader()
	default:
		return ""
	}
}
```

**Step 2: Update View() to render sub-header**

Replace the `View()` method's assembly (currently lines ~425-466) to render:

```go
func (m appModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Persistent header.
	header := dashboard.RenderHeader(m.stats, m.width)
	separator := strings.Repeat("─", m.width)

	// Sub-header (view title + stats).
	subHeader := m.subHeader()

	// View content.
	var content string
	switch m.state {
	// ... (same switch as before)
	}

	// Help overlay replaces only the content area.
	if m.helpOpen {
		viewState := m.state
		if m.state == message.ViewSourceManager && m.sources.DetailOpen() {
			viewState = message.ViewSourceDetail
		}
		hc := shared.HelpForView(viewState, m.keys)
		content = components.RenderHelpOverlay(hc, m.width, m.height)
	}

	return header + "\n" + separator + "\n" + subHeader + "\n" + separator + "\n" + content
}
```

**Step 3: Update height calculation**

The sub-header is always 2 lines (title + stats) + 1 separator line = 3 extra lines compared to before.

In the `WindowSizeMsg` handler (around line 106-109), update:

```go
case tea.WindowSizeMsg:
	m.width = msg.Width
	m.height = msg.Height

	// Compute persistent header height (header + separator).
	header := dashboard.RenderHeader(m.stats, m.width)
	m.headerHeight = strings.Count(header, "\n") + 2 // +1 trailing newline, +1 separator

	// Sub-header: 2 lines (title + stats) + 1 separator = 3 lines.
	subHeaderHeight := 3
	childMsg = tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - m.headerHeight - subHeaderHeight}
```

Update `childHeight()`:

```go
func (m appModel) childHeight() int {
	subHeaderHeight := 3 // title + stats + separator
	return m.height - m.headerHeight - subHeaderHeight
}
```

**Step 4: Remove source manager's own sub-header rendering**

The source manager currently renders its own header + separator in `View()` (lines 60-66 in `sources/view.go`). Remove this since the root model now handles it:

In `sources/view.go`, remove from `View()`:
```go
// Remove these lines:
// b.WriteString(m.renderHeader())
// b.WriteString("\n")
// b.WriteString(strings.Repeat("\u2500", m.width))
// b.WriteString("\n")
```

Keep the column headers and content.

**Step 5: Remove ad-hoc titles from other views**

- **Pipeline Monitor** (`pipeline/view.go:52-54`): Remove the title line `"  " + titleStyle.Render("Pipeline Monitor")` from the sections. The sub-header now has the title.
- **Log Viewer** (`logview/view.go:24-31`): Remove the title + follow indicator line. The sub-header now has both.
- **Detail** (`detail/view.go:33-41`): Remove the header block (lines 33-41: `Proposal: <type>`, `Target:`, `Confidence: | Session:`). The sub-header replaces this. Keep the body viewport.
- **Fitness** (`fitness/view.go:31-49`): Remove the header block (lines 31-49: `Fitness Report: <name>`, `Ownership: | Origin: | Observed:`). The sub-header replaces this. Keep the viewport.

**Step 6: Run tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go test ./internal/tui/...`
Expected: Tests pass (may need to update assertions that check for removed content).

**Step 7: Fix any test assertions**

Common assertions that may break:
- `integration_test.go:76`: `"Proposal:"` → may need updating (sub-header says "Proposal Detail")
- `integration_test.go:286`: `"SOURCE"` → should still exist (column headers remain)

Fix each broken assertion to match the new layout.

**Step 8: Commit**

```
feat(tui): centralize sub-header rendering in root model

The root model now renders header + separator + sub-header + separator +
content for all views. Each view's ad-hoc title rendering is removed.
The source manager's self-rendered sub-header moves to the centralized
pattern. Height calculations account for the new sub-header.
```

---

### Task 6: Make help overlay preserve sub-header

Update the help overlay to render only key binding sections (no title/description), since the sub-header already provides view context.

**Files:**
- Modify: `internal/tui/components/helpoverlay.go:34-71`
- Modify: `internal/tui/model.go` (View method — already partially done in Task 5)

**Step 1: Update RenderHelpOverlay to render sections only**

In `internal/tui/components/helpoverlay.go`, simplify `RenderHelpOverlay` to only render key binding sections:

```go
func RenderHelpOverlay(hc shared.HelpContent, width, height int) string {
	var b strings.Builder

	b.WriteString("\n") // top padding

	for i, section := range hc.Sections {
		if i > 0 {
			b.WriteString("\n")
		}
		// Section title.
		b.WriteString("  ")
		b.WriteString(helpTitleStyle.Render(section.Title))
		b.WriteString("\n")

		// Entries.
		for _, entry := range section.Entries {
			line := "  " + helpKeyStyle.Render(entry.Key) + helpEntryDescStyle.Render(entry.Desc)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}
```

This removes the view title (lines 39-42) and view description (lines 44-49).

**Step 2: Verify sub-header is visible during help**

The root model's `View()` (updated in Task 5) already renders `header + separator + subHeader + separator + content`. When help is open, only `content` is replaced with the help overlay. The sub-header remains visible.

**Step 3: Run tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go test ./internal/tui/...`
Expected: All tests pass.

**Step 4: Visual verification**

Run: `go run ./cmd/snapshot help-overlay`

Verify the help overlay shows:
- Persistent header at top
- Sub-header (e.g. "Proposals / 3 awaiting review ...")
- Separator
- Key binding sections (without redundant title/description)

**Step 5: Commit**

```
feat(tui): help overlay preserves sub-header

The help overlay now renders only key binding sections, omitting
the title and description which are already visible in the sub-header.
This keeps the header and sub-header stable when toggling help.
```

---

### Task 7: Update snapshots and visual verification

Regenerate all TUI snapshots and do a final visual check.

**Files:**
- Run: `make snapshots`
- Verify: `snapshots/` directory

**Step 1: Regenerate snapshots**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && make snapshots`

Review each generated snapshot visually to confirm:
1. Header says "Cabrero" (not "Cabrero Review")
2. No proposal stats in the header
3. Each view has a sub-header with title + stats between two horizontal separators
4. Help overlay shows sub-header + key binding sections (no redundant title)

**Step 2: Fix any rendering issues**

If any view looks off (e.g., extra blank lines, misaligned text, missing separators), fix the corresponding view file and re-run snapshots.

**Step 3: Commit snapshots**

```
chore: regenerate TUI snapshots after uniformization
```

---

### Task 8: Update DESIGN.md and CHANGELOG.md

Update documentation to reflect the renamed command, new layout structure, and add changelog entries.

**Files:**
- Modify: `DESIGN.md` (references to "review" command, "Cabrero Review")
- Modify: `CHANGELOG.md` (add entries under `[Unreleased]`)
- Modify: `CLAUDE.md` (if any "review" references exist)

**Step 1: Update DESIGN.md**

Search for "review" in DESIGN.md and update contextually:
- "Cabrero Review App" → "Cabrero Dashboard" (or just "Cabrero")
- "`cabrero review`" → "`cabrero dashboard`"
- "Review TUI" → "Dashboard TUI" or just "TUI"
- "Review Window" → "Main Window"
- Keep "review" where it refers to the action of reviewing proposals (not the command name)

**Step 2: Update CHANGELOG.md**

Add under `[Unreleased]`:

```markdown
### Changed
- Renamed `review` CLI command to `dashboard`
- Simplified persistent header from "Cabrero Review" to "Cabrero"
- All views now have a consistent sub-header with title and contextual stats
- Help overlay preserves the sub-header, only replacing the content area

### Removed
- Proposal stats from the persistent header (moved to dashboard sub-header)
```

**Step 3: Commit**

```
docs: update DESIGN.md and CHANGELOG.md for TUI uniformization

Rename "review" references to "dashboard" in DESIGN.md. Add changelog
entries for the command rename, header simplification, centralized
sub-headers, and help overlay changes.
```
