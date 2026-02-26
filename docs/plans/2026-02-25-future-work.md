# Future Work Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Address four deferred architectural improvements identified in the full codebase review: consolidate duplicated format helpers into `internal/cli`, fix the O(N²) `RenderStatusBar` trimming loop, introduce a `PipelineStages` interface to replace `Runner`'s 5 exported hook fields, and extract log file I/O from the root TUI model into the `logview` package where it belongs.

**Architecture:** Four sequential, independent tasks ordered from smallest to largest. Each builds on a clean codebase. Tasks 1–2 are pure consolidation/cleanup. Task 3 is a bounded refactor of the pipeline package with mechanical test migration. Task 4 is the most structural: introduces new Bubble Tea message types and moves filesystem I/O to the correct package.

**Tech Stack:** Go 1.22+, Bubble Tea v2 (`charm.land/bubbletea/v2`), standard library

---

## Task 1: Consolidate `RelativeTime` and `ShortenHome` into `internal/cli`

**Files:**
- Create: `internal/cli/format.go`
- Create: `internal/cli/format_test.go`
- Modify: `internal/cmd/format.go` → delete entirely
- Modify: `internal/cmd/prompts_test.go` (update test to call `cli.RelativeTime`)
- Modify: `internal/cmd/calibrate.go` (one `formatAge` caller)
- Modify: `internal/cmd/prompts.go` (one `formatAge` caller)
- Modify: `internal/cmd/status.go` (one `ShortenHome` site)
- Modify: `internal/cmd/doctor.go` (two `ShortenHome` sites)
- Modify: `internal/cmd/uninstall.go` (five `ShortenHome` sites)

**Context:** `internal/cmd` cannot import `internal/tui/shared` (architectural boundary). It already imports `internal/cli`. `formatAge` in `cmd/format.go` duplicates `shared.RelativeTime` identically, plus adds a zero-time guard. `ShortenHome` (`strings.Replace(path, home, "~", 1)`) is inlined at 8 call sites across 3 files in `cmd`. Moving both helpers to `internal/cli` consolidates all duplicated logic.

---

**Step 1: Create `internal/cli/format.go`**

```go
package cli

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// homeDir is resolved once at init so ShortenHome never calls os.UserHomeDir
// on the hot path.
var homeDir string

func init() {
	homeDir, _ = os.UserHomeDir()
}

// RelativeTime formats t as a human-readable relative age string.
// Returns "unknown" for a zero time; "just now" for durations under 1 minute.
func RelativeTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
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

// ShortenHome replaces the current user's home directory prefix with "~".
// Returns path unchanged if home cannot be determined or path is not under home.
func ShortenHome(path string) string {
	if homeDir != "" && strings.HasPrefix(path, homeDir) {
		return "~" + path[len(homeDir):]
	}
	return path
}
```

**Step 2: Create `internal/cli/format_test.go`**

```go
package cli

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero time", time.Time{}, "unknown"},
		{"just now", now.Add(-10 * time.Second), "just now"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-48 * time.Hour), "2d ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RelativeTime(tt.t); got != tt.want {
				t.Errorf("RelativeTime() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortenHome(t *testing.T) {
	if homeDir == "" {
		t.Skip("home dir not available")
	}
	full := homeDir + "/.claude/SKILL.md"
	got := ShortenHome(full)
	want := "~/.claude/SKILL.md"
	if got != want {
		t.Errorf("ShortenHome(%q) = %q, want %q", full, got, want)
	}
	// Paths not under home are returned unchanged.
	other := "/etc/hosts"
	if got := ShortenHome(other); got != other {
		t.Errorf("ShortenHome(%q) = %q, want unchanged", other, got)
	}
}
```

**Step 3: Run the new tests to confirm they pass**

```bash
cd /Users/vladolaru/Work/a8c/cabrero
go test ./internal/cli/... -v
```

Expected: PASS

**Step 4: Update `internal/cmd/prompts_test.go`**

Find the `TestFormatAge` test function (it currently calls the unexported `formatAge`). Update it to use `cli.RelativeTime` instead:

```go
// Before:
got := formatAge(tt.t)

// After:
got := cli.RelativeTime(tt.t)
```

Add `"github.com/vladolaru/cabrero/internal/cli"` to imports if not already present.

**Step 5: Update `internal/cmd/calibrate.go` and `internal/cmd/prompts.go`**

In each file, replace `formatAge(x)` with `cli.RelativeTime(x)`. Make sure `internal/cli` is imported. Run:

```bash
grep -n "formatAge" internal/cmd/calibrate.go internal/cmd/prompts.go
```

For each occurrence, update the call.

**Step 6: Delete `internal/cmd/format.go`**

```bash
rm internal/cmd/format.go
```

**Step 7: Build to confirm no broken references**

```bash
go build ./internal/cmd/...
```

Fix any "undefined: formatAge" errors by updating remaining callers (the grep in step 5 should catch them all).

**Step 8: Replace 8 `ShortenHome` inline sites in `cmd` files**

Each site follows the pattern:
```go
home, _ := os.UserHomeDir()
display := strings.Replace(path, home, "~", 1)
```

Replace with:
```go
display := cli.ShortenHome(path)
```

**Important:** Some functions declare `home` for a purpose other than the display string (e.g., building a path with `filepath.Join(home, ...)`). Before removing the `home, _ := os.UserHomeDir()` line, scan the rest of the function for other `home` uses. Only remove the declaration if it has no remaining uses.

Sites to update:
- `internal/cmd/status.go:25-26`
- `internal/cmd/doctor.go:341-342`
- `internal/cmd/doctor.go:976-977`
- `internal/cmd/uninstall.go:86-88`
- `internal/cmd/uninstall.go` (4 additional sites — locate with `grep -n "strings.Replace.*home" internal/cmd/uninstall.go`)

After updating all sites, remove unused `"strings"` imports from any file that no longer calls `strings.Replace` (run `go build ./internal/cmd/...` and let the compiler tell you).

**Step 9: Run full test suite**

```bash
go test ./...
```

Expected: PASS

**Step 10: Commit**

```bash
git add internal/cli/format.go internal/cli/format_test.go \
  internal/cmd/format.go internal/cmd/prompts_test.go \
  internal/cmd/calibrate.go internal/cmd/prompts.go \
  internal/cmd/status.go internal/cmd/doctor.go internal/cmd/uninstall.go
git commit -m "$(cat <<'EOF'
refactor(cli): add RelativeTime + ShortenHome helpers; remove cmd duplication

formatAge in cmd/format.go duplicated shared.RelativeTime exactly
(plus a zero-time guard). ShortenHome was inlined at 8 call sites across
cmd/status.go, cmd/doctor.go, and cmd/uninstall.go. Neither could
import tui/shared due to the architectural boundary.

Add both helpers to internal/cli (already imported by all cmd files).
Delete cmd/format.go. Update all 8 inline ShortenHome sites.
EOF
)"
```

---

## Task 2: Fix RenderStatusBar O(N²) trimming loop

**Files:**
- Create: `internal/tui/components/statusbar_test.go`
- Modify: `internal/tui/components/statusbar.go:39-48`

**Context:** The key-binding drop loop in `RenderStatusBar` calls `strings.Join(parts, sep)` and `lipgloss.Width(bar)` (full ANSI scan) on each iteration. Worst case with N bindings is O(N²). N is bounded at ≤10 in practice, so real-world impact is currently negligible — but the correct fix is a single linear pass using pre-computed widths, and it removes the repeated allocations.

---

**Step 1: Create `internal/tui/components/statusbar_test.go`** with a correctness test and a trimming behaviour test

```go
package components

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
)

func makeBinding(k, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc))
}

func TestRenderStatusBar_FitsWidth(t *testing.T) {
	bindings := []key.Binding{
		makeBinding("?", "help"),
		makeBinding("q", "quit"),
		makeBinding("enter", "open"),
		makeBinding("esc", "back"),
		makeBinding("j", "down"),
	}
	width := 40
	result := RenderStatusBar(bindings, "", width)
	w := ansi.StringWidth(result)
	if w > width {
		t.Errorf("RenderStatusBar width = %d, want ≤ %d", w, width)
	}
}

func TestRenderStatusBar_TimedMsgOverride(t *testing.T) {
	bindings := []key.Binding{makeBinding("q", "quit")}
	result := RenderStatusBar(bindings, "1/3 matches", 60)
	stripped := ansi.Strip(result)
	if stripped != "1/3 matches" {
		// timedMsg should replace bindings entirely in the rendered output
		t.Errorf("expected timedMsg in output, got %q", stripped)
	}
}

func TestRenderStatusBar_DropsBindingsToFit(t *testing.T) {
	// With a very narrow width, all bindings should be dropped rather than overflow.
	bindings := []key.Binding{
		makeBinding("ctrl+a", "select all"),
		makeBinding("ctrl+b", "bold"),
		makeBinding("ctrl+c", "copy"),
	}
	width := 10
	result := RenderStatusBar(bindings, "", width)
	w := ansi.StringWidth(result)
	if w > width {
		t.Errorf("RenderStatusBar with narrow width: got %d, want ≤ %d", w, width)
	}
}
```

**Step 2: Run tests to confirm they pass on current code**

```bash
cd /Users/vladolaru/Work/a8c/cabrero
go test ./internal/tui/components/... -run TestRenderStatusBar -v
```

Expected: PASS (the loop is correct; we are not fixing a bug, we are improving the algorithm).

**Step 3: Fix the trimming loop in `internal/tui/components/statusbar.go`**

Replace lines 39–48 (the `// Drop trailing bindings...` loop) with a single linear pass:

```go
	// Drop trailing bindings until the bar fits in one line.
	// Pre-compute each part's rendered width to avoid O(N²) re-joins.
	sep := "  "
	sepWidth := lipgloss.Width(sep)
	contentWidth := width - 2 // statusBarStyle has Padding(0, 1)

	total := 0
	cutoff := 0
	for i, p := range parts {
		w := lipgloss.Width(p)
		if i > 0 {
			w += sepWidth
		}
		if total+w > contentWidth {
			break
		}
		total += w
		cutoff = i + 1
	}

	if cutoff > 0 {
		return statusBarStyle.Width(width).Render(strings.Join(parts[:cutoff], sep))
	}
	return statusBarStyle.Width(width).Render("")
```

**Step 4: Run tests to confirm they still pass**

```bash
go test ./internal/tui/components/... -v
```

Expected: PASS

**Step 5: Run full suite**

```bash
go test ./...
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/tui/components/statusbar.go internal/tui/components/statusbar_test.go
git commit -m "$(cat <<'EOF'
perf(statusbar): replace O(N²) trimming loop with single linear pass

The key-binding drop loop was calling strings.Join + lipgloss.Width
(full ANSI scan) on each iteration. With N bindings, worst case was N
re-joins and N ANSI scans. Pre-computing each part's width and using a
cumulative sum finds the cutoff in O(N) with zero re-allocations.

N is currently ≤10 so the real-world impact was negligible, but the
linear algorithm is cleaner and removes repeated string allocations.
EOF
)"
```

---

## Task 3: PipelineStages interface for Runner

**Files:**
- Modify: `internal/pipeline/pipeline.go` (add interface + TestStages)
- Modify: `internal/pipeline/runner.go` (replace 5 exported fields + update dispatch)
- Modify: `internal/pipeline/runner_test.go` (migrate to TestStages)
- Modify: `internal/pipeline/invoke_test.go` (check for Runner usage)
- Modify: `internal/pipeline/batch_test.go` (check for Runner usage)

**Context:** `Runner` has 5 exported function fields (`ParseSessionFunc`, `AggregateFunc`, `ClassifyFunc`, `EvalFunc`, `EvalBatchFunc`) used only for test injection. Production code nil-checks them and falls back to package-level functions. This pattern works but means adding a new pipeline stage requires modifying the struct definition. Introducing a `PipelineStages` interface replaces the 5 fields with a single injection point. An exported `TestStages` struct (same function fields, implementing the interface) lets tests migrate mechanically without losing per-method override granularity.

---

**Step 1: Add `PipelineStages` interface and `TestStages` to `internal/pipeline/pipeline.go`**

Add after the existing `Logger` interface block:

```go
// PipelineStages is the injection interface for pipeline stage execution.
// Implement this (or use TestStages) to override stages in tests.
// Production code uses the Runner's built-in implementations.
type PipelineStages interface {
	ParseSession(sessionID string) (*parser.Digest, error)
	Aggregate(sessionID, project string) (*patterns.AggregatorOutput, error)
	Classify(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error)
	EvalOne(sessionID string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
	EvalBatch(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
}

// TestStages implements PipelineStages via optional function overrides.
// Unset fields fall back to the Runner's production implementation.
// Use NewRunnerWithStages to inject into a Runner.
type TestStages struct {
	ParseSessionFunc func(sessionID string) (*parser.Digest, error)
	AggregateFunc    func(sessionID, project string) (*patterns.AggregatorOutput, error)
	ClassifyFunc     func(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error)
	EvalOneFunc      func(sessionID string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
	EvalBatchFunc    func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
}

func (s TestStages) ParseSession(sid string) (*parser.Digest, error) {
	if s.ParseSessionFunc != nil {
		return s.ParseSessionFunc(sid)
	}
	return nil, nil // signals "use production"
}

func (s TestStages) Aggregate(sid, project string) (*patterns.AggregatorOutput, error) {
	if s.AggregateFunc != nil {
		return s.AggregateFunc(sid, project)
	}
	return nil, nil
}

func (s TestStages) Classify(sid string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
	if s.ClassifyFunc != nil {
		return s.ClassifyFunc(sid, cfg)
	}
	return nil, nil, nil
}

func (s TestStages) EvalOne(sid string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
	if s.EvalOneFunc != nil {
		return s.EvalOneFunc(sid, digest, co, cfg)
	}
	return nil, nil, nil
}

func (s TestStages) EvalBatch(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
	if s.EvalBatchFunc != nil {
		return s.EvalBatchFunc(sessions, cfg)
	}
	return nil, nil, nil
}
```

Note: `BatchSession`, `ClassifierResult`, `ClaudeResult`, `EvaluatorOutput`, `ClassifierOutput` are already defined in the `pipeline` package. Add any needed imports (`parser`, `patterns`) to `pipeline.go`'s import block.

**Step 2: Update `Runner` struct and add `NewRunnerWithStages`**

In `internal/pipeline/runner.go`, change the struct from:

```go
type Runner struct {
	Config       PipelineConfig
	MaxBatchSize int
	Source       string
	OnStatus     func(sessionID string, event BatchEvent)

	// Testing hooks — when nil, package-level functions are used.
	ParseSessionFunc func(sessionID string) (*parser.Digest, error)
	AggregateFunc    func(sessionID string, project string) (*patterns.AggregatorOutput, error)
	ClassifyFunc     func(sessionID string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error)
	EvalFunc         func(sessionID string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
	EvalBatchFunc    func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error)
}
```

To:

```go
type Runner struct {
	Config       PipelineConfig
	MaxBatchSize int
	Source       string
	OnStatus     func(sessionID string, event BatchEvent)

	// stages overrides pipeline execution for testing. nil = use built-in production logic.
	stages PipelineStages
}
```

Add `NewRunnerWithStages` after `NewRunner`:

```go
// NewRunnerWithStages creates a Runner with a custom PipelineStages implementation.
// Use TestStages{...} to override individual stages in tests.
func NewRunnerWithStages(cfg PipelineConfig, s PipelineStages) *Runner {
	return &Runner{Config: cfg, stages: s}
}
```

**Step 3: Update `Runner`'s private dispatch methods**

The 4 methods that check for hooks (`parseSession`, `aggregate`, `classify`, `evalOne`, `evalMany`) need to check `r.stages` instead of the removed function fields.

Update `parseSession`:
```go
func (r *Runner) parseSession(sessionID string) (*parser.Digest, error) {
	if r.stages != nil {
		if d, err := r.stages.ParseSession(sessionID); d != nil || err != nil {
			return d, err
		}
	}
	return parser.ParseSession(sessionID)
}
```

Update `aggregate`:
```go
func (r *Runner) aggregate(sessionID, project string) (*patterns.AggregatorOutput, error) {
	if r.stages != nil {
		if a, err := r.stages.Aggregate(sessionID, project); a != nil || err != nil {
			return a, err
		}
	}
	return patterns.Aggregate(sessionID, project)
}
```

Update `classify`:
```go
func (r *Runner) classify(sessionID string) (*ClassifierResult, *ClaudeResult, error) {
	if r.stages != nil {
		if cr, cl, err := r.stages.Classify(sessionID, r.Config); cr != nil || cl != nil || err != nil {
			return cr, cl, err
		}
	}
	// Production logic (unchanged) ...
```

Update `evalOne`:
```go
func (r *Runner) evalOne(sessionID string, digest *parser.Digest, co *ClassifierOutput) (*EvaluatorOutput, *ClaudeResult, error) {
	if r.stages != nil {
		if out, cl, err := r.stages.EvalOne(sessionID, digest, co, r.Config); out != nil || cl != nil || err != nil {
			return out, cl, err
		}
	}
	return RunEvaluator(sessionID, digest, co, r.Config)
}
```

Update `evalMany`:
```go
func (r *Runner) evalMany(sessions []BatchSession) (*EvaluatorOutput, *ClaudeResult, error) {
	if r.stages != nil {
		if out, cl, err := r.stages.EvalBatch(sessions, r.Config); out != nil || cl != nil || err != nil {
			return out, cl, err
		}
	}
	// Production batch eval logic (unchanged) ...
```

**Step 4: Build to confirm struct/interface changes compile**

```bash
go build ./internal/pipeline/...
```

Fix any missing-field or type errors.

**Step 5: Migrate `runner_test.go` call sites**

Search for all direct field assignments on `Runner` test instances:

```bash
grep -n "\.ParseSessionFunc\|\.AggregateFunc\|\.ClassifyFunc\|\.EvalFunc\b\|\.EvalBatchFunc" \
  internal/pipeline/runner_test.go internal/pipeline/invoke_test.go internal/pipeline/batch_test.go
```

For each test, replace the pattern:

```go
// Before:
r := NewRunner(cfg)
r.ClassifyFunc = func(sid string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
    ...
}
r.EvalBatchFunc = func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
    ...
}

// After:
r := NewRunnerWithStages(cfg, TestStages{
    ClassifyFunc: func(sid string, cfg PipelineConfig) (*ClassifierResult, *ClaudeResult, error) {
        ...
    },
    EvalBatchFunc: func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
        ...
    },
})
```

Note: If a test assigns hooks after construction (e.g., `r.EvalFunc = ...`) rather than at construction time, you must restructure to pass a `TestStages` at construction. Read each test to understand the setup before rewriting.

Also note: the old `EvalFunc` field is now `EvalOneFunc` in `TestStages` (renamed for clarity since the interface method is `EvalOne`). Update accordingly.

**Step 6: Run pipeline tests**

```bash
go test ./internal/pipeline/... -v
```

Expected: PASS

**Step 7: Run full suite**

```bash
go test ./...
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/pipeline/pipeline.go internal/pipeline/runner.go \
  internal/pipeline/runner_test.go internal/pipeline/invoke_test.go \
  internal/pipeline/batch_test.go
git commit -m "$(cat <<'EOF'
refactor(pipeline): introduce PipelineStages interface; replace exported hook fields

Runner had 5 exported function fields (ParseSessionFunc, AggregateFunc,
ClassifyFunc, EvalFunc, EvalBatchFunc) used only for test injection,
making the struct's public API bleed testing concerns. Adding a new
pipeline stage required adding a 6th exported field.

Replace with a single unexported `stages PipelineStages` field.
Add NewRunnerWithStages(cfg, TestStages{...}) for tests. TestStages
implements PipelineStages with the same optional-override pattern,
so individual stage overrides remain granular without modifying Runner.
EOF
)"
```

---

## Task 4: Extract log I/O from appModel into logview

**Files:**
- Modify: `internal/tui/logview/model.go` (add `fileSize` field + `SetFileSize` setter)
- Create: `internal/tui/logview/follow.go` (new `FollowTick` command + message types)
- Modify: `internal/tui/logview/update.go` (handle new messages)
- Modify: `internal/tui/logview/model_test.go` (tests for FollowTick)
- Modify: `internal/tui/model.go` (update `LogTickMsg` handler; remove `logFileSize`; route new messages; update `switchView`)

**Context:** The `LogTickMsg` handler in `appModel.Update()` at `model.go:304-332` contains 30 lines of filesystem I/O: stat, incremental read, rotation detection, full reload. This logic belongs in `logview` — the root model should fire a command but not implement the I/O. Moving it to `logview.FollowTick()` gives logview ownership of its data source, removes `logFileSize` from the root model, and makes the root model a thin coordinator.

---

**Step 1: Add `fileSize int64` field and `SetFileSize` to `logview.Model`**

In `internal/tui/logview/model.go`, add `fileSize int64` to the `Model` struct (after `config`):

```go
type Model struct {
	// ... existing fields ...
	config   *shared.Config
	fileSize int64 // byte offset of last-read position for follow mode
}
```

Add after the `SetSize` method:

```go
// SetFileSize records the byte length of the log content at last load.
// Call this after creating the model from file content so FollowTick can
// detect new bytes correctly.
func (m *Model) SetFileSize(n int64) {
	m.fileSize = n
}
```

**Step 2: Create `internal/tui/logview/follow.go`** with message types and the `FollowTick` command

```go
package logview

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

// LogAppended is returned by FollowTick when new bytes were appended to the log file.
type LogAppended struct {
	NewContent  string
	NewFileSize int64
}

// LogReplaced is returned by FollowTick when the log file shrank (log rotation).
// The full new content is included.
type LogReplaced struct {
	Content     string
	NewFileSize int64
}

// FollowTick returns a Bubble Tea command that checks logPath for new content
// since the last read position recorded in fileSize.
//
// Returns:
//   - LogAppended if new bytes were written since last read
//   - LogReplaced if the file shrank (log rotation detected)
//   - nil if the file is unchanged or cannot be read
func (m Model) FollowTick(logPath string) tea.Cmd {
	size := m.fileSize
	return func() tea.Msg {
		info, err := os.Stat(logPath)
		if err != nil {
			return nil
		}
		newSize := info.Size()

		if newSize > size {
			// New bytes appended — read only the delta.
			f, err := os.Open(logPath)
			if err != nil {
				return nil
			}
			buf := make([]byte, newSize-size)
			n, _ := f.ReadAt(buf, size)
			f.Close()
			if n > 0 {
				return LogAppended{NewContent: string(buf[:n]), NewFileSize: size + int64(n)}
			}
			return nil
		}

		if newSize < size {
			// File shrank — full reload (log rotation).
			content, _ := os.ReadFile(logPath)
			return LogReplaced{Content: string(content), NewFileSize: int64(len(content))}
		}

		return nil // unchanged
	}
}
```

**Step 3: Write tests for `FollowTick` in `internal/tui/logview/model_test.go`**

Add at the end of the file:

```go
func TestFollowTick_DetectsNewBytes(t *testing.T) {
	// Write initial content to a temp file.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "daemon.log")
	initial := "line1\nline2\n"
	if err := os.WriteFile(logPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestLogModel()
	m.SetFileSize(int64(len(initial)))

	// Append new content.
	appended := "line3\n"
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(appended)
	f.Close()

	cmd := m.FollowTick(logPath)
	msg := cmd()

	result, ok := msg.(LogAppended)
	if !ok {
		t.Fatalf("expected LogAppended, got %T", msg)
	}
	if result.NewContent != appended {
		t.Errorf("NewContent = %q, want %q", result.NewContent, appended)
	}
	if result.NewFileSize != int64(len(initial)+len(appended)) {
		t.Errorf("NewFileSize = %d, want %d", result.NewFileSize, len(initial)+len(appended))
	}
}

func TestFollowTick_DetectsRotation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "daemon.log")
	initial := "old line1\nold line2\n"
	if err := os.WriteFile(logPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestLogModel()
	m.SetFileSize(int64(len(initial)))

	// Overwrite with shorter content (simulates rotation).
	rotated := "new\n"
	if err := os.WriteFile(logPath, []byte(rotated), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := m.FollowTick(logPath)
	msg := cmd()

	result, ok := msg.(LogReplaced)
	if !ok {
		t.Fatalf("expected LogReplaced, got %T", msg)
	}
	if result.Content != rotated {
		t.Errorf("Content = %q, want %q", result.Content, rotated)
	}
}

func TestFollowTick_NoChangeReturnsNil(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "daemon.log")
	content := "line1\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestLogModel()
	m.SetFileSize(int64(len(content)))

	cmd := m.FollowTick(logPath)
	msg := cmd()

	if msg != nil {
		t.Errorf("expected nil msg for unchanged file, got %T: %v", msg, msg)
	}
}
```

Add the needed import to the test file:
```go
import (
    "os"
    "path/filepath"
    // ... existing imports ...
)
```

**Step 4: Run the new tests to verify they fail (FollowTick not yet wired)**

```bash
go test ./internal/tui/logview/... -run TestFollowTick -v
```

Since `FollowTick` exists now (we created it in step 2), these should actually pass already. Run them and confirm.

**Step 5: Handle `LogAppended` and `LogReplaced` in `logview.Update()`**

In `internal/tui/logview/update.go`, add cases to the top-level switch:

```go
case LogAppended:
	m.fileSize = msg.NewFileSize
	m.AppendContent(msg.NewContent)
	return m, nil

case LogReplaced:
	m.fileSize = msg.NewFileSize
	m.UpdateContent(msg.Content)
	return m, nil
```

**Step 6: Update `LogTickMsg` handler in `internal/tui/model.go`**

Replace the entire `case message.LogTickMsg:` block (lines ~304–333) with:

```go
case message.LogTickMsg:
	if m.state == message.ViewLogViewer {
		logPath := filepath.Join(store.Root(), "daemon.log")
		nextTick := tea.Tick(time.Second, func(time.Time) tea.Msg {
			return message.LogTickMsg{}
		})
		return m, tea.Batch(m.logViewer.FollowTick(logPath), nextTick)
	}
	return m, nil
```

**Step 7: Add routing for `logview.LogAppended` and `logview.LogReplaced` in `appModel.Update()`**

The root model now receives these messages and must forward them to `m.logViewer`. Add a new case block in the Update switch (place it near the other logview-related cases):

```go
case logview.LogAppended, logview.LogReplaced:
	var cmd tea.Cmd
	m.logViewer, cmd = m.logViewer.Update(msg)
	return m, cmd
```

Import `logview` in `model.go` if not already present:
```go
"github.com/vladolaru/cabrero/internal/tui/logview"
```

**Step 8: Update `switchView` case for `ViewLogViewer` to use `SetFileSize`**

In `model.go`, find the `ViewLogViewer` case in `switchView` (around line 643):

```go
// Before:
case message.ViewLogViewer:
	logPath := filepath.Join(store.Root(), "daemon.log")
	content, _ := os.ReadFile(logPath)
	m.logFileSize = int64(len(content))
	m.logViewer = logview.New(string(content), &m.keys, m.config)
	m.logViewer.SetSize(m.width, m.childHeight())
	cmds = append(cmds, tea.Tick(time.Second, func(time.Time) tea.Msg {
		return message.LogTickMsg{}
	}))

// After:
case message.ViewLogViewer:
	logPath := filepath.Join(store.Root(), "daemon.log")
	content, _ := os.ReadFile(logPath)
	m.logViewer = logview.New(string(content), &m.keys, m.config)
	m.logViewer.SetFileSize(int64(len(content)))
	m.logViewer.SetSize(m.width, m.childHeight())
	cmds = append(cmds, tea.Tick(time.Second, func(time.Time) tea.Msg {
		return message.LogTickMsg{}
	}))
```

**Step 9: Remove `logFileSize` from `appModel`**

In `internal/tui/model.go`:
1. Remove `logFileSize int64` from the `appModel` struct definition
2. Search for any remaining references to `logFileSize` and remove them: `grep -n "logFileSize" internal/tui/model.go`

**Step 10: Build**

```bash
go build ./...
```

Fix any remaining compilation errors (unused imports, missing fields, etc.).

**Step 11: Run all tests**

```bash
go test ./...
```

Expected: PASS

**Step 12: Commit**

```bash
git add \
  internal/tui/logview/model.go \
  internal/tui/logview/follow.go \
  internal/tui/logview/update.go \
  internal/tui/logview/model_test.go \
  internal/tui/model.go
git commit -m "$(cat <<'EOF'
refactor(logview): extract log file I/O from appModel into logview.FollowTick

The LogTickMsg handler in appModel.Update() contained ~30 lines of
filesystem logic (stat, incremental read, rotation detection). This
belongs in the logview package which owns the log content.

Add FollowTick(logPath) tea.Cmd to logview.Model — it stats the file,
reads only new bytes if grown, or reloads on rotation, returning
LogAppended or LogReplaced messages. logview.Update() handles them.
appModel.Update() now just fires FollowTick + next tick in a Batch,
and routes the logview messages back to m.logViewer.Update().

Remove logFileSize from appModel; fileSize is now owned by logview.Model.
EOF
)"
```
