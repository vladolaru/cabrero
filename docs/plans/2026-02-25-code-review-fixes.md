# Code Review Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Address all confirmed findings from the full-codebase code review: fix a security boundary, a UI rendering bug, and five medium/low-severity issues, then clear a batch of trivial cleanup items.

**Architecture:** Seven sequential tasks ordered by severity. Tasks 1–2 are required fixes (security + UI bug). Tasks 3–6 are medium improvements (correctness, consistency, performance). Task 7 is a single-commit batch of trivially small cleanups.

**Tech Stack:** Go 1.22+, Bubble Tea v2, lipgloss v2, standard library

---

## Task 1: Security — Fix validateTarget path traversal check

**Files:**
- Create: `internal/apply/apply_test.go`
- Modify: `internal/apply/apply.go:182-195`

**Context:** `validateTarget` calls `filepath.Clean(resolved)` which resolves all `..` components, then checks `strings.Contains(cleaned, "..")`. After `filepath.Clean`, an absolute path can never contain `..`, so the check is always false. An adversarial evaluator output with `Target: "~/../../etc/cron.d/evil.md"` would pass validation and write to `/etc/cron.d/evil.md`. The `.md` suffix check limits the blast radius but doesn't prevent writing to arbitrary `.md` paths.

---

**Step 1: Write failing tests for validateTarget**

Create `internal/apply/apply_test.go`:

```go
package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTarget_TraversalEscapingHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	// Craft a path that resolves outside home after filepath.Clean.
	escaped := filepath.Clean(filepath.Join(home, "../../etc/hosts.md"))
	if strings.HasPrefix(escaped, home+string(filepath.Separator)) {
		t.Skipf("path %s still inside home — unusual test environment", escaped)
	}
	if err := validateTarget(escaped); err == nil {
		t.Errorf("validateTarget(%q) = nil, want error for path outside home", escaped)
	}
}

func TestValidateTarget_ValidInsideHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	valid := filepath.Join(home, ".claude", "SKILL.md")
	if err := validateTarget(valid); err != nil {
		t.Errorf("validateTarget(%q) = %v, want nil", valid, err)
	}
}

func TestValidateTarget_NotMarkdown(t *testing.T) {
	home, _ := os.UserHomeDir()
	notMd := filepath.Join(home, ".claude", "script.sh")
	if err := validateTarget(notMd); err == nil {
		t.Errorf("validateTarget(%q) = nil, want error for non-.md", notMd)
	}
}

func TestValidateTarget_AtHomeRoot(t *testing.T) {
	// A path exactly equal to home (without a child) must be rejected.
	home, _ := os.UserHomeDir()
	if err := validateTarget(home); err == nil {
		t.Errorf("validateTarget(%q) = nil, want error for home root", home)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /Users/vladolaru/Work/a8c/cabrero
go test ./internal/apply/... -run TestValidateTarget -v
```

Expected: `TestValidateTarget_TraversalEscapingHome` FAILS — `validateTarget` returns nil for the escaped path.

**Step 3: Fix validateTarget**

Replace the body of `validateTarget` in `internal/apply/apply.go:182-195`:

```go
func validateTarget(resolved string) error {
	cleaned := filepath.Clean(resolved)

	// Must be a markdown file.
	if !strings.HasSuffix(strings.ToLower(cleaned), ".md") {
		return fmt.Errorf("target must be a .md file, got: %s", resolved)
	}

	// Reject path traversal: require the target to be inside the user's home directory.
	// Note: strings.Contains(cleaned, "..") is a no-op after filepath.Clean resolves
	// all traversal components. Use a prefix check instead.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	if !strings.HasPrefix(cleaned, home+string(filepath.Separator)) {
		return fmt.Errorf("target outside home directory: %s", resolved)
	}

	return nil
}
```

`os` is already imported in `apply.go`.

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/apply/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/apply/apply_test.go internal/apply/apply.go
git commit -m "fix(apply): replace no-op traversal check with home-dir prefix guard

validateTarget was calling strings.Contains(cleaned, \"..\") after
filepath.Clean had already resolved all traversal components. For any
absolute path, filepath.Clean never leaves \"..\" in the result, so the
check was permanently false — any target path, however constructed,
would pass.

Replace with strings.HasPrefix(cleaned, home+sep) to enforce that
proposals can only modify markdown files inside the user's home
directory."
```

---

## Task 2: UI Bug — Fix log viewer search bar width overflow

**Files:**
- Modify: `internal/tui/logview/view.go:37-40`
- Modify: `internal/tui/logview/model_test.go` (add test)

**Context:** `RenderStatusBar` returns a string already padded/clamped to `m.width` via `statusBarStyle.Width(m.width).Render(...)`. Prepending `[N/M matches] ` (12–16 chars) to it produces a string wider than `m.width`, causing the bottom bar to wrap. `RenderStatusBar` already has a `timedMsg` parameter designed for transient overlays — use it.

---

**Step 1: Write failing test**

Add to `internal/tui/logview/model_test.go`:

```go
func TestLogModelView_SearchMatchBarFitsTerminalWidth(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(80, 20)

	// Simulate a completed search with matches.
	m.searchActive = false
	m.searchTerm = "daemon"
	m.matches = []int{0, 2, 4}
	m.matchIdx = 0

	view := m.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("View() returned empty string")
	}
	last := lines[len(lines)-1]

	// Status bar must fit within terminal width.
	width := ansi.StringWidth(last)
	if width > 80 {
		t.Errorf("status bar width = %d, want ≤ 80\ngot: %q", width, last)
	}

	// Match count must be visible in the bar.
	stripped := ansi.Strip(last)
	if !strings.Contains(stripped, "1/3 matches") {
		t.Errorf("status bar missing match count\ngot: %q", stripped)
	}
}
```

**Step 2: Run to verify it fails**

```bash
go test ./internal/tui/logview/... -run TestLogModelView_SearchMatchBarFitsTerminalWidth -v
```

Expected: FAIL — status bar width exceeds 80 characters.

**Step 3: Fix view.go**

Replace lines 36–40 in `internal/tui/logview/view.go`:

```go
	} else {
		timedMsg := m.statusMsg
		if m.searchTerm != "" && len(m.matches) > 0 {
			timedMsg = fmt.Sprintf("[%d/%d matches]", m.matchIdx+1, len(m.matches))
		}
		bottom = components.RenderStatusBar(m.keys.LogViewShortHelp(), timedMsg, m.width)
	}
```

**Step 4: Run to verify it passes**

```bash
go test ./internal/tui/logview/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/logview/view.go internal/tui/logview/model_test.go
git commit -m "fix(logview): route search match count through RenderStatusBar timedMsg

[N/M matches] was prepended to the already-width-bounded output of
RenderStatusBar, making the composite string 12-16 chars wider than the
terminal on every active search. RenderStatusBar already has a timedMsg
parameter for transient overlays — use it so the width constraint is
applied to the full output."
```

---

## Task 3: Medium — Fix escapeAppleScript newline escaping

**Files:**
- Create: `internal/daemon/notify_test.go`
- Modify: `internal/daemon/notify.go:14-27`

**Context:** `escapeAppleScript` escapes `"` and `\` but not `\n` or `\r`. A newline in a notification title or message breaks the AppleScript string literal, causing `osascript` to fail silently. Also switching to `strings.Builder` is cleaner. (The UTF-8 byte-loop concern from the security agent is a false positive: `"` (0x22) and `\` (0x5C) are ASCII-range bytes that can't appear as UTF-8 continuation bytes.)

---

**Step 1: Write failing tests**

Create `internal/daemon/notify_test.go`:

```go
package daemon

import "testing"

func TestEscapeAppleScript_Quotes(t *testing.T) {
	got := escapeAppleScript(`say "hello"`)
	want := `say \"hello\"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_Backslash(t *testing.T) {
	got := escapeAppleScript(`path\to\file`)
	want := `path\\to\\file`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_Newline(t *testing.T) {
	got := escapeAppleScript("line1\nline2")
	want := `line1\nline2`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_CarriageReturn(t *testing.T) {
	got := escapeAppleScript("line1\rline2")
	want := `line1\rline2`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_Unicode(t *testing.T) {
	// Multi-byte UTF-8 must pass through unchanged.
	got := escapeAppleScript("héllo wörld 🎉")
	want := "héllo wörld 🎉"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_Empty(t *testing.T) {
	if got := escapeAppleScript(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
```

**Step 2: Run to verify failure**

```bash
go test ./internal/daemon/... -run TestEscapeAppleScript -v
```

Expected: `TestEscapeAppleScript_Newline` and `TestEscapeAppleScript_CarriageReturn` FAIL.

**Step 3: Fix escapeAppleScript**

Replace the function and add `"strings"` import in `internal/daemon/notify.go`:

```go
import (
	"os/exec"
	"strings"
)

func escapeAppleScript(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
```

**Step 4: Run to verify pass**

```bash
go test ./internal/daemon/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/daemon/notify.go internal/daemon/notify_test.go
git commit -m "fix(daemon): add newline/CR escaping to escapeAppleScript; use strings.Builder

Unescaped newlines in notification text would produce syntactically
invalid AppleScript and cause osascript to fail silently. Added \\n and
\\r escape cases. Switched from nil []byte + byte-append to
strings.Builder with Grow for cleaner allocation."
```

---

## Task 4: Consistency — Use store.AtomicWrite in SaveConfigTo

**Files:**
- Modify: `internal/tui/config.go:68-115`

**Context:** `store.AtomicWrite` was introduced in this codebase and is used by every other write site (7 callers). `SaveConfigTo` manually replicates the same `CreateTemp + chmod + Rename` pattern. Replace with the shared helper to eliminate the only divergent write site.

---

**Step 1: Run existing config tests to confirm baseline**

```bash
go test ./internal/tui/... -run TestSaveConfig -v
```

Expected: PASS (existing tests already cover this function)

**Step 2: Simplify SaveConfigTo**

In `internal/tui/config.go`, replace lines 91–114 (the atomic write block and its return) with a single call:

```go
func SaveConfigTo(cfg *Config, path string) error {
	// Build a map that includes known and unknown fields.
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(data, &merged); err != nil {
		return fmt.Errorf("merging config: %w", err)
	}

	// Re-add unknown fields.
	for k, v := range cfg.Extra {
		merged[k] = v
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("formatting config: %w", err)
	}
	out = append(out, '\n')

	return store.AtomicWrite(path, out, 0o644)
}
```

Also remove unused imports: after this change, `"path/filepath"` and `"os"` may no longer be needed in `config.go`. Check: `os.IsNotExist` is still used in `LoadConfigFrom`, so keep `"os"`. `filepath` is used in `configPath()`, so keep it too.

**Step 3: Build and test**

```bash
go build ./internal/tui/...
go test ./internal/tui/... -v
```

Expected: PASS

**Step 4: Commit**

```bash
git add internal/tui/config.go
git commit -m "refactor(config): replace inlined atomic-write with store.AtomicWrite

SaveConfigTo was manually implementing CreateTemp + chmod + Rename, the
same pattern encapsulated by store.AtomicWrite. All other write sites
in the codebase use store.AtomicWrite — this was the only divergent
one."
```

---

## Task 5: Dead code — Remove ColorHighlightFg/Bg

**Files:**
- Modify: `internal/tui/shared/styles.go:23-24,55-56`

**Context:** `ColorHighlightFg` and `ColorHighlightBg` are exported vars declared and assigned in `InitStyles` but have zero callers anywhere in the codebase. The actual highlight rendering in `logview/model.go` uses `HighlightFg()` and `HighlightBg()` function helpers that return hardcoded strings. The vars are an orphaned fragment of an incomplete refactor.

---

**Step 1: Verify zero callers**

```bash
grep -rn "ColorHighlightFg\|ColorHighlightBg" /Users/vladolaru/Work/a8c/cabrero --include="*.go"
```

Expected: Only 4 lines, all in `internal/tui/shared/styles.go`.

**Step 2: Remove the var declarations (lines 23–24)**

In `internal/tui/shared/styles.go`, remove from the `var (` block:
```go
ColorHighlightFg color.Color
ColorHighlightBg color.Color
```

**Step 3: Remove the assignments in InitStyles (lines 55–56)**

Remove from `InitStyles`:
```go
ColorHighlightFg = ld(lipgloss.Color("#FFFFFF"), lipgloss.Color("#FFFFFF"))
ColorHighlightBg = ld(lipgloss.Color("#6A1B9A"), lipgloss.Color("#9C27B0"))
```

**Step 4: Build and test**

```bash
go build ./...
go test ./internal/tui/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/shared/styles.go
git commit -m "chore(styles): remove unused ColorHighlightFg/Bg vars

Declared and assigned in InitStyles but never read. Highlight rendering
in logview uses HighlightFg()/HighlightBg() function helpers instead."
```

---

## Task 6: Performance — Cache pipeline config to stop re-reading config.json on every 5s tick

**Files:**
- Modify: `internal/tui/model.go` (struct field + `newAppModel` signature)
- Modify: `internal/tui/tui.go` (`gatherStatsFromSessions` signature + `Run`)
- Modify: `internal/tui/integration_test.go` (`newTestRoot`)

**Context:** `gatherStatsFromSessions` calls `pipeline.DefaultPipelineConfig()` which calls `store.ReadPipelineOverrides()` — a disk read of `config.json`. The function is invoked on every `PipelineTickMsg` (every 5 seconds when the pipeline monitor view is open), causing ~720 unnecessary disk reads per hour. The config values (`ClassifierTimeout`, `EvaluatorTimeout`) are user-set constants that don't change at runtime. Compute them once at startup.

---

**Step 1: Add pipelineCfg field to appModel**

In `internal/tui/model.go`, add to the `appModel` struct after `pipelineRefreshing bool`:

```go
// pipelineCfg caches the pipeline config resolved at startup from config.json.
// Used by gatherStatsFromSessions to avoid re-reading the file on every tick.
pipelineCfg pipeline.PipelineConfig
```

**Step 2: Update newAppModel signature to accept pipelineCfg**

Change the function signature in `internal/tui/model.go:79`:

```go
func newAppModel(
	proposals []pipeline.ProposalWithSession,
	reports []fitness.Report,
	stats message.DashboardStats,
	sourceGroups []fitness.SourceGroup,
	runs []pipeline.PipelineRun,
	pipelineStats pipeline.PipelineStats,
	prompts []pipeline.PromptVersion,
	cfg *shared.Config,
	pipelineCfg pipeline.PipelineConfig,
) appModel {
```

In the function body, add after the model is initialized (find where `m` is constructed and add):
```go
m.pipelineCfg = pipelineCfg
```

**Step 3: Update gatherStatsFromSessions signature**

In `internal/tui/tui.go`, change:

```go
func gatherStatsFromSessions(sessions []store.Metadata, proposals []pipeline.ProposalWithSession) message.DashboardStats {
```
To:
```go
func gatherStatsFromSessions(sessions []store.Metadata, proposals []pipeline.ProposalWithSession, pipelineCfg pipeline.PipelineConfig) message.DashboardStats {
```

Replace lines 114–116 in the function body:
```go
// Before:
pipelineDefaults := pipeline.DefaultPipelineConfig()
stats.ClassifierTimeout = pipelineDefaults.ClassifierTimeout
stats.EvaluatorTimeout = pipelineDefaults.EvaluatorTimeout

// After:
stats.ClassifierTimeout = pipelineCfg.ClassifierTimeout
stats.EvaluatorTimeout = pipelineCfg.EvaluatorTimeout
```

**Step 4: Update Run() in tui.go**

In `internal/tui/tui.go`, add before calling `gatherStatsFromSessions` (around line 38):

```go
pipelineCfg := pipeline.DefaultPipelineConfig()
```

Update the call at line 40:
```go
stats := gatherStatsFromSessions(sessions, proposals, pipelineCfg)
```

Update `newAppModel` call at line 68:
```go
m := newAppModel(proposals, reports, stats, sourceGroups, runs, pipelineStats, prompts, cfg, pipelineCfg)
```

**Step 5: Update PipelineTickMsg handler in model.go**

In `internal/tui/model.go:280`, update:
```go
dashStats := gatherStatsFromSessions(sessions, proposals, m.pipelineCfg)
```

**Step 6: Update integration_test.go**

In `internal/tui/integration_test.go`, update `newTestRoot`:

```go
func newTestRoot() appModel {
	proposals := testdata.TestProposals()
	reports := testdata.TestFitnessReports()
	stats := testdata.TestDashboardStats()
	sourceGroups := testdata.TestSourceGroups()
	runs := testdata.TestPipelineRuns()
	pipelineStats := testdata.TestPipelineStats()
	prompts := testdata.TestPromptVersions()
	cfg := testdata.TestConfig()
	return newAppModel(proposals, reports, stats, sourceGroups, runs, pipelineStats, prompts, cfg, pipeline.PipelineConfig{})
}
```

If `pipeline` is not yet imported in `integration_test.go`, add `"github.com/vladolaru/cabrero/internal/pipeline"` to the import block.

**Step 7: Build and test**

```bash
go build ./...
go test ./...
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/tui/model.go internal/tui/tui.go internal/tui/integration_test.go
git commit -m "perf(tui): cache pipeline config at startup; stop re-reading config.json on every 5s tick

gatherStatsFromSessions was calling pipeline.DefaultPipelineConfig()
which reads and parses config.json on every invocation. With a 5-second
PipelineTickMsg, this caused ~720 unnecessary disk reads per hour while
the pipeline monitor was open.

The two values extracted (ClassifierTimeout, EvaluatorTimeout) are
user-set constants that don't change at runtime. Compute them once in
Run() and thread the result through newAppModel → appModel.pipelineCfg
→ gatherStatsFromSessions parameter."
```

---

## Task 7: Batch cleanup — trivial consistency fixes

**Files:**
- Modify: `internal/store/query.go:52-55`
- Modify: `internal/pipeline/batch.go:62-66`
- Modify: `internal/pipeline/runner.go:583`
- Modify: `internal/pipeline/batch_test.go` (update shortID → store.ShortSessionID)
- Modify: `internal/pipeline/runner_test.go` (update shortID → store.ShortSessionID)
- Modify: `internal/tui/components/helpoverlay.go:73`
- Modify: `internal/tui/components/helpoverlay_test.go` (remove height arg)
- Modify: `cmd/snapshot/main.go:317`
- Modify: `internal/cmd/dashboard.go:8`
- Modify: `main.go:100-102`
- Modify: `hooks/pre-compact-backup.sh`
- Modify: `hooks/session-end.sh`

---

**Step 1: Fix query.go — replace manual swap with slices.Reverse**

In `internal/store/query.go`:

1. Add `"slices"` to imports.
2. Replace lines 52–55:

```go
// Before:
for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
    matched[i], matched[j] = matched[j], matched[i]
}

// After:
slices.Reverse(matched)
```

**Step 2: Remove shortID passthrough in pipeline**

In `internal/pipeline/batch.go`, delete the entire `shortID` function (lines 62–66).

Update `internal/pipeline/runner.go:583`:
```go
// Before:
prefix := "prop-" + shortID(s.SessionID) + "-"
// After:
prefix := "prop-" + store.ShortSessionID(s.SessionID) + "-"
```

Update `internal/pipeline/batch_test.go` — replace all `shortID(` with `store.ShortSessionID(`:
```bash
grep -n "shortID(" internal/pipeline/batch_test.go
```
Then update each occurrence.

Update `internal/pipeline/runner_test.go` — same substitution.

**Step 3: Drop unused height param from RenderHelpOverlay**

In `internal/tui/components/helpoverlay.go:73`, change:

```go
// Before:
func RenderHelpOverlay(hc shared.HelpContent, width, height int) string {
    return RenderHelpContent(hc, width)
}

// After:
func RenderHelpOverlay(hc shared.HelpContent, width int) string {
    return RenderHelpContent(hc, width)
}
```

Update test calls in `internal/tui/components/helpoverlay_test.go` — remove the third argument from all `RenderHelpOverlay(hc, 120, 40)` calls → `RenderHelpOverlay(hc, 120)`.

Update `cmd/snapshot/main.go:317`:
```go
// Before:
helpContent := components.RenderHelpOverlay(hc, w, h-th)
// After:
helpContent := components.RenderHelpOverlay(hc, w)
```

**Step 4: Drop unused args param from Dashboard**

In `internal/cmd/dashboard.go`:
```go
// Before:
func Dashboard(args []string, version string) error {
    return tui.Run(version)
}

// After:
func Dashboard(version string) error {
    return tui.Run(version)
}
```

In `main.go:100-102`, `cmdDashboard` must keep its `args []string` signature (it's used as a dispatch function). Update to discard the arg:
```go
func cmdDashboard(_ []string) error {
    return cmd.Dashboard(version)
}
```

**Step 5: Validate SESSION_ID in hook scripts**

In `hooks/pre-compact-backup.sh`, after the empty-check block (after line 27), add:

```sh
# Reject SESSION_ID values that contain path components (traversal guard).
case "$SESSION_ID" in
  */* | *..*) echo "cabrero: invalid session_id, skipping" >&2; exit 0 ;;
esac
```

Apply the identical block to `hooks/session-end.sh` in the same position (after its empty-check at line 23–25).

**Step 6: Build and test everything**

```bash
go build ./...
go test ./...
```

Expected: PASS

**Step 7: Commit**

```bash
git add \
  internal/store/query.go \
  internal/pipeline/batch.go internal/pipeline/runner.go \
  internal/pipeline/batch_test.go internal/pipeline/runner_test.go \
  internal/tui/components/helpoverlay.go internal/tui/components/helpoverlay_test.go \
  cmd/snapshot/main.go \
  internal/cmd/dashboard.go main.go \
  hooks/pre-compact-backup.sh hooks/session-end.sh
git commit -m "chore: batch cleanup — slices.Reverse, remove shortID, drop unused params, validate hook SESSION_ID

- store/query.go: replace manual swap loop with slices.Reverse
- pipeline/batch.go: remove shortID passthrough; callers use store.ShortSessionID directly
- helpoverlay.go: drop silently-unused height parameter; update all callers
- cmd/dashboard.go: drop unused args []string parameter; update main.go caller
- hooks/*.sh: reject SESSION_ID values containing path separators"
```
