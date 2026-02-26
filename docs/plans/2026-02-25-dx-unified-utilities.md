# DX: Unified Utilities — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate duplicate utility functions across packages and remove dead code. Three concrete issues: `timeAgo`/`relativeTime` are the same function in two packages; `checkMark`/`checkmark` are identical; two independent progress bar implementations exist in `components` and `sources`; fitness carries an unused `spinner`.

**Architecture:** Add canonical versions to `shared/format.go` and `components/`. Delete the duplicates. Update all call sites. This plan has zero user-visible impact — it is pure developer DX.

**Dependency:** Safe to execute at any time. No dependency on other plans.

**Snapshot note:** No view changes expected. Run `go test ./...` before and after to confirm no regressions.

---

### Task 1: Remove unused spinner from `fitness.Model`

**Files:**
- Modify: `internal/tui/fitness/model.go`
- Modify: `internal/tui/fitness/update.go` (verify spinner is not forwarded)
- Test: `internal/tui/fitness/model_test.go`

**Step 1: Read `fitness/update.go` and `fitness/view.go` to confirm spinner is never used**

```bash
grep -n "spinner" internal/tui/fitness/update.go internal/tui/fitness/view.go
```

Expected: zero matches — the spinner is declared in the struct but never referenced outside `model.go`.

**Step 2: Write a compile-time confirmation test** (no behavioral change, so just verify it builds)

Before touching anything:

```bash
go test ./internal/tui/fitness/...
```

Expected: all pass (baseline).

**Step 3: Remove spinner field and initialization from `fitness/model.go`**

Remove:
```go
import "github.com/charmbracelet/bubbles/spinner"
```
(only if no other usage in the file)

Remove from `Model` struct:
```go
spinner spinner.Model
```

Remove from `New()`:
```go
s := spinner.New()
s.Spinner = spinner.Dot
// ...
spinner:  s,
```

**Step 4: Build and test**

```bash
go build ./internal/tui/fitness/ && go test ./internal/tui/fitness/...
```

Expected: clean build, all tests pass.

**Step 5: Commit**

```bash
git add internal/tui/fitness/model.go
git commit -m "chore(fitness): remove unused spinner field"
```

---

### Task 2: Unify `timeAgo` and `relativeTime` into `shared.RelativeTime`

**Files:**
- Modify: `internal/tui/shared/format.go`
- Modify: `internal/tui/dashboard/model.go` (remove `timeAgo`, update call site)
- Modify: `internal/tui/pipeline/view.go` (remove `relativeTime`, update call sites)
- Test: `internal/tui/shared/` (new test)

**Step 1: Write a failing test in `internal/tui/shared/format_test.go`**

```go
package shared

import (
    "testing"
    "time"
)

func TestRelativeTime(t *testing.T) {
    now := time.Now()
    cases := []struct {
        t    time.Time
        want string
    }{
        {now.Add(-30 * time.Second), "just now"},
        {now.Add(-5 * time.Minute), "5m ago"},
        {now.Add(-3 * time.Hour), "3h ago"},
        {now.Add(-2 * 24 * time.Hour), "2d ago"},
    }
    for _, c := range cases {
        got := RelativeTime(c.t)
        if got != c.want {
            t.Errorf("RelativeTime(%v) = %q, want %q", time.Since(c.t).Round(time.Second), got, c.want)
        }
    }
}
```

**Step 2: Verify test fails**

```bash
cd internal/tui/shared && go test -run TestRelativeTime -v
```

Expected: FAIL — `RelativeTime` undefined.

**Step 3: Add `RelativeTime` to `shared/format.go`**

```go
// RelativeTime formats t as a human-readable relative age string.
// Returns "just now" for durations under 1 minute.
func RelativeTime(t time.Time) string {
    d := time.Since(t)
    switch {
    case d < time.Minute:
        return "just now"
    case d < time.Hour:
        return fmt.Sprintf("%dm ago", int(d.Minutes()))
    case d < 24*time.Hour:
        return fmt.Sprintf("%dh ago", int(d.Hours()))
    default:
        return fmt.Sprintf("%dd ago", int(d.Hours()/24))
    }
}
```

Add `"fmt"` and `"time"` to imports if not already present.

**Step 4: Run the test — expect pass**

```bash
cd internal/tui/shared && go test -run TestRelativeTime -v
```

**Step 5: Replace `timeAgo` in `dashboard/model.go`**

Delete the `timeAgo` function (lines 235–247). Replace the one call site in `dashboard/model.go` (inside `timeAgo` it was only used in view, check where it's called). Search:

```bash
grep -n "timeAgo" internal/tui/dashboard/
```

Update each call site from `timeAgo(t)` to `shared.RelativeTime(t)`. Add `shared` import if needed.

**Step 6: Replace `relativeTime` in `pipeline/view.go`**

Delete the `relativeTime` function (lines 448–460). Replace call sites:

```bash
grep -n "relativeTime" internal/tui/pipeline/view.go
```

Update each from `relativeTime(t)` to `shared.RelativeTime(t)`.

**Step 7: Build and test all affected packages**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

Expected: clean.

**Step 8: Commit**

```bash
git add internal/tui/shared/format.go internal/tui/dashboard/model.go internal/tui/pipeline/view.go
git commit -m "refactor(tui): unify timeAgo/relativeTime into shared.RelativeTime"
```

---

### Task 3: Unify `checkMark` and `checkmark` into `shared.Checkmark`

**Files:**
- Modify: `internal/tui/shared/format.go`
- Modify: `internal/tui/dashboard/view.go` (remove `checkMark`)
- Modify: `internal/tui/pipeline/view.go` (remove `checkmark`)
- Test: `internal/tui/shared/format_test.go`

**Step 1: Add test**

```go
func TestCheckmark(t *testing.T) {
    ok := Checkmark(true)
    if ok == "" {
        t.Error("Checkmark(true) should return non-empty string")
    }
    notOk := Checkmark(false)
    if notOk == "" {
        t.Error("Checkmark(false) should return non-empty string")
    }
    if ok == notOk {
        t.Error("Checkmark(true) and Checkmark(false) should differ")
    }
}
```

**Step 2: Add `Checkmark` to `shared/format.go`**

```go
// Checkmark renders a ✓ (success) or ✗ (error) with appropriate color.
func Checkmark(ok bool) string {
    if ok {
        return SuccessStyle.Render("✓")
    }
    return ErrorStyle.Render("✗")
}
```

**Step 3: Replace in `dashboard/view.go`**

Delete the `checkMark` function. Find its call sites:

```bash
grep -n "checkMark" internal/tui/dashboard/view.go
```

Replace `checkMark(x)` → `shared.Checkmark(x)`.

**Step 4: Replace in `pipeline/view.go`**

Delete the `checkmark` function. Find its call sites:

```bash
grep -n "checkmark" internal/tui/pipeline/view.go
```

Replace `checkmark(x)` → `shared.Checkmark(x)`.

**Step 5: Build and test**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

**Step 6: Commit**

```bash
git add internal/tui/shared/format.go internal/tui/dashboard/view.go internal/tui/pipeline/view.go
git commit -m "refactor(tui): unify checkMark/checkmark into shared.Checkmark"
```

---

### Task 4: Unify progress bar implementations

There are two independent bar implementations:
- `components.RenderAssessBar(percent, width, category)` in `assessbar.go`
- `sources.renderBar(score, approach)` in `sources/view.go`

Both render `█` (filled) and `░` (empty) characters with lipgloss coloring. Additionally, `renderAssessRow()` in `assessbar.go` creates a `mutedStyle` inline on every call — a minor allocation issue.

**Files:**
- Modify: `internal/tui/components/assessbar.go`
- Modify: `internal/tui/sources/view.go`
- Test: `internal/tui/components/assessbar_test.go`

**Step 1: Read existing bar tests**

```bash
cat internal/tui/components/assessbar_test.go
```

**Step 2: Add test for a shared bar primitive**

Add to `internal/tui/components/assessbar_test.go`:

```go
func TestRenderBar_FilledEmpty(t *testing.T) {
    // At 0%: all empty.
    bar := RenderBar(0, 10, shared.ColorSuccess)
    stripped := ansi.Strip(bar)
    if strings.ContainsRune(stripped, barFilled) {
        t.Error("0% bar should have no filled chars")
    }
    if len([]rune(stripped)) != 10 {
        t.Errorf("bar width = %d, want 10", len([]rune(stripped)))
    }

    // At 100%: all filled.
    bar = RenderBar(100, 10, shared.ColorSuccess)
    stripped = ansi.Strip(bar)
    if strings.ContainsRune(stripped, barEmpty) {
        t.Error("100% bar should have no empty chars")
    }

    // At 50%: half filled.
    bar = RenderBar(50, 10, shared.ColorSuccess)
    stripped = ansi.Strip(bar)
    filled := strings.Count(stripped, string(barFilled))
    empty := strings.Count(stripped, string(barEmpty))
    if filled != 5 || empty != 5 {
        t.Errorf("50%% bar: filled=%d empty=%d, want 5 each", filled, empty)
    }
}
```

**Step 3: Add `RenderBar` to `assessbar.go`**

```go
// RenderBar renders a horizontal progress bar of the given width.
// percent is 0–100, color is applied to the filled portion.
func RenderBar(percent float64, width int, color lipgloss.TerminalColor) string {
    if width <= 0 {
        return ""
    }
    if percent < 0 {
        percent = 0
    }
    if percent > 100 {
        percent = 100
    }
    filled := int(math.Round(float64(width) * percent / 100))
    if filled > width {
        filled = width
    }
    empty := width - filled
    filledStyle := lipgloss.NewStyle().Foreground(color)
    return filledStyle.Render(strings.Repeat(string(barFilled), filled)) +
        strings.Repeat(string(barEmpty), empty)
}
```

**Step 4: Update `RenderAssessBar` to delegate to `RenderBar`**

```go
func RenderAssessBar(percent float64, width int, category string) string {
    return RenderBar(percent, width, categoryColor(category))
}
```

**Step 5: Extract package-level `mutedStyle` in `assessbar.go`**

The inline `mutedStyle` created on every call inside `renderAssessRow` should be a package-level var. But since `components` already imports `shared`, use `shared.MutedStyle` directly:

In `renderAssessRow`, replace:
```go
mutedStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
```
with a direct reference to `shared.MutedStyle`.

**Step 6: Replace `sources.renderBar` with `components.RenderBar`**

In `sources/view.go`, delete the `renderBar` function. Update `renderHealth`:

```go
func renderHealth(s fitness.Source) string {
    if s.Ownership == "" {
        return "───"
    }
    if s.HealthScore < 0 {
        return mutedStyle.Render("n/a")
    }
    color := healthColor(s.HealthScore, s.Approach)
    bar := components.RenderBar(s.HealthScore, 10, color)
    return fmt.Sprintf("%s %3.0f%%", bar, s.HealthScore)
}

// healthColor returns the bar color for a source health score.
func healthColor(score float64, approach string) lipgloss.TerminalColor {
    if approach == "iterate" {
        return shared.ColorSuccess
    }
    switch {
    case score >= 80:
        return shared.ColorSuccess
    case score >= 50:
        return shared.ColorWarning
    default:
        return shared.ColorError
    }
}
```

**Step 7: Build and test**

```bash
go build ./internal/tui/... && go test ./internal/tui/...
```

**Step 8: Commit**

```bash
git add internal/tui/components/assessbar.go internal/tui/sources/view.go
git commit -m "refactor(tui): unify progress bar implementations into components.RenderBar

Two independent bar renderers existed (assessbar.go and sources/view.go).
RenderBar is now the single primitive; RenderAssessBar delegates to it.
Also removes the inline lipgloss.NewStyle() allocation per renderAssessRow call."
```
