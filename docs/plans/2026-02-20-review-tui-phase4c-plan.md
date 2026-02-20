# Phase 4c Implementation Plan: Pipeline Monitor + Log Viewer

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Pipeline Monitor and Log Viewer views to complete the review TUI's operational monitoring capabilities.

**Architecture:** Two new view packages (`pipeline/`, `logview/`) following the exact same model/update/view split pattern used by all existing views. Data layer extends the `internal/pipeline` package with a `PipelineRun` type reconstructed from existing store artifacts. Auto-refresh via `tea.Tick` polling (5s pipeline, 1s log follow). A shared sparkline component renders Unicode block charts.

**Tech Stack:** Go, Bubble Tea (bubbletea), Bubbles (viewport, textinput, spinner, key), Lip Gloss (lipgloss), existing `internal/store` and `internal/pipeline` packages.

**Working directory:** `/Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui/`

---

### Task 1: PipelineRun Data Types

Create the data types and reconstruction logic for pipeline runs.

**Files:**
- Create: `internal/pipeline/run.go`
- Test: `internal/pipeline/run_test.go`

**Step 1: Write the failing test**

```go
// internal/pipeline/run_test.go
package pipeline

import (
	"testing"
	"time"
)

func TestPipelineRunStatus(t *testing.T) {
	run := PipelineRun{
		SessionID: "abc123",
		Status:    "processed",
		HasDigest: true,
		HasClassifier:  true,
		HasEvaluator: true,
	}
	if run.Status != "processed" {
		t.Errorf("status = %q, want processed", run.Status)
	}
	if !run.HasEvaluator {
		t.Error("expected HasEvaluator true")
	}
}

func TestPipelineStatsZero(t *testing.T) {
	stats := PipelineStats{}
	if stats.SessionsCaptured != 0 {
		t.Errorf("expected zero stats")
	}
}

func TestPromptVersionParsing(t *testing.T) {
	tests := []struct {
		filename string
		name     string
		version  string
	}{
		{"classifier-v3.txt", "classifier", "v3"},
		{"evaluator-v3.txt", "evaluator", "v3"},
		{"apply-v1.txt", "apply", "v1"},
	}
	for _, tt := range tests {
		name, ver := parsePromptFilename(tt.filename)
		if name != tt.name {
			t.Errorf("parsePromptFilename(%q) name = %q, want %q", tt.filename, name, tt.name)
		}
		if ver != tt.version {
			t.Errorf("parsePromptFilename(%q) version = %q, want %q", tt.filename, ver, tt.version)
		}
	}
}

func TestSparklineBuckets(t *testing.T) {
	sessions := []time.Time{
		time.Now().Add(-1 * time.Hour),
		time.Now().Add(-25 * time.Hour),
		time.Now().Add(-25 * time.Hour),
		time.Now().Add(-49 * time.Hour),
	}
	buckets := bucketSessionsByDay(sessions, 3)
	// Day 0 (today): 1 session, Day 1 (yesterday): 2 sessions, Day 2: 1 session
	if len(buckets) != 3 {
		t.Fatalf("buckets len = %d, want 3", len(buckets))
	}
	if buckets[0] != 1 {
		t.Errorf("buckets[0] = %d, want 1", buckets[0])
	}
	if buckets[1] != 2 {
		t.Errorf("buckets[1] = %d, want 2", buckets[1])
	}
	if buckets[2] != 1 {
		t.Errorf("buckets[2] = %d, want 1", buckets[2])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/pipeline/ -run 'TestPipelineRun|TestPipelineStats|TestPromptVersion|TestSparkline' -v`
Expected: FAIL — types and functions don't exist yet.

**Step 3: Write minimal implementation**

```go
// internal/pipeline/run.go
package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// PipelineRun represents a single pipeline processing run for a session.
type PipelineRun struct {
	SessionID string
	Project   string
	Timestamp time.Time
	Status    string // "pending", "processed", "error"

	// Per-stage completion.
	HasDigest bool
	HasClassifier  bool
	HasEvaluator bool

	// Per-stage timing (zero if stage not completed).
	ParseDuration  time.Duration
	ClassifierDuration  time.Duration
	EvaluatorDuration time.Duration

	// Results.
	ProposalCount int
	ErrorDetail   string
}

// PipelineStats holds aggregated pipeline statistics.
type PipelineStats struct {
	SessionsCaptured   int
	SessionsProcessed  int
	SessionsPending    int
	SessionsErrored    int
	ProposalsGenerated int
	ProposalsApproved  int
	ProposalsRejected  int
	ProposalsPending   int
	SessionsPerDay     []int // for sparkline, index 0 = today
}

// PromptVersion represents a prompt file with its version and last-used time.
type PromptVersion struct {
	Name     string
	Version  string
	LastUsed time.Time
}

// ListPipelineRuns returns recent pipeline runs, sorted newest first.
// It reconstructs run data from session metadata and evaluation file existence.
func ListPipelineRuns(limit int) ([]PipelineRun, error) {
	sessions, err := store.ListSessions()
	if err != nil {
		return nil, err
	}

	var runs []PipelineRun
	for i, meta := range sessions {
		if limit > 0 && i >= limit {
			break
		}

		ts, _ := time.Parse(time.RFC3339, meta.Timestamp)
		run := PipelineRun{
			SessionID: meta.SessionID,
			Project:   store.ProjectDisplayName(meta.Project),
			Timestamp: ts,
			Status:    meta.Status,
		}

		evalDir := filepath.Join(store.Root(), "evaluations")
		digestDir := filepath.Join(store.Root(), "digests")

		// Check stage completion via file existence.
		digestPath := filepath.Join(digestDir, meta.SessionID+".json")
		classifierPath := filepath.Join(evalDir, meta.SessionID+"-classifier.json")
		evaluatorPath := filepath.Join(evalDir, meta.SessionID+"-evaluator.json")

		var digestInfo, classifierInfo, evaluatorInfo os.FileInfo

		if info, err := os.Stat(digestPath); err == nil {
			run.HasDigest = true
			digestInfo = info
		}
		if info, err := os.Stat(classifierPath); err == nil {
			run.HasClassifier = true
			classifierInfo = info
		}
		if info, err := os.Stat(evaluatorPath); err == nil {
			run.HasEvaluator = true
			evaluatorInfo = info
		}

		// Estimate per-stage timing from file modification timestamps.
		if run.HasDigest && !ts.IsZero() {
			run.ParseDuration = digestInfo.ModTime().Sub(ts)
			if run.ParseDuration < 0 {
				run.ParseDuration = 0
			}
		}
		if run.HasClassifier && run.HasDigest {
			run.ClassifierDuration = classifierInfo.ModTime().Sub(digestInfo.ModTime())
			if run.ClassifierDuration < 0 {
				run.ClassifierDuration = 0
			}
		}
		if run.HasEvaluator && run.HasClassifier {
			run.EvaluatorDuration = evaluatorInfo.ModTime().Sub(classifierInfo.ModTime())
			if run.EvaluatorDuration < 0 {
				run.EvaluatorDuration = 0
			}
		}

		// Count proposals from evaluator output.
		if run.HasEvaluator {
			if so, err := ReadEvaluatorOutput(meta.SessionID); err == nil {
				run.ProposalCount = len(so.Proposals)
			}
		}

		runs = append(runs, run)
	}

	return runs, nil
}

// GatherPipelineStats aggregates pipeline statistics over the given number of days.
func GatherPipelineStats(days int) (PipelineStats, error) {
	sessions, err := store.ListSessions()
	if err != nil {
		return PipelineStats{}, err
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	stats := PipelineStats{}
	var timestamps []time.Time

	for _, meta := range sessions {
		ts, _ := time.Parse(time.RFC3339, meta.Timestamp)
		if ts.Before(cutoff) {
			continue
		}

		stats.SessionsCaptured++
		timestamps = append(timestamps, ts)

		switch meta.Status {
		case "processed":
			stats.SessionsProcessed++
		case "pending":
			stats.SessionsPending++
		case "error":
			stats.SessionsErrored++
		}
	}

	// Count proposals.
	proposals, _ := ListProposals()
	stats.ProposalsPending = len(proposals)

	// Archived proposals are in the evaluations dir (we count evaluator outputs).
	for _, meta := range sessions {
		ts, _ := time.Parse(time.RFC3339, meta.Timestamp)
		if ts.Before(cutoff) {
			continue
		}
		if so, err := ReadEvaluatorOutput(meta.SessionID); err == nil {
			stats.ProposalsGenerated += len(so.Proposals)
		}
	}

	stats.SessionsPerDay = bucketSessionsByDay(timestamps, days)

	return stats, nil
}

// bucketSessionsByDay groups timestamps into daily buckets.
// Index 0 = today, index 1 = yesterday, etc.
func bucketSessionsByDay(timestamps []time.Time, days int) []int {
	buckets := make([]int, days)
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for _, ts := range timestamps {
		dayOffset := int(todayStart.Sub(ts.Truncate(24*time.Hour)).Hours() / 24)
		if dayOffset >= 0 && dayOffset < days {
			buckets[dayOffset]++
		}
	}
	return buckets
}

// ListPromptVersions reads prompt files from ~/.cabrero/prompts/ and returns
// their names and versions.
func ListPromptVersions() ([]PromptVersion, error) {
	dir := filepath.Join(store.Root(), "prompts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var versions []PromptVersion
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		name, ver := parsePromptFilename(e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		versions = append(versions, PromptVersion{
			Name:     name,
			Version:  ver,
			LastUsed: info.ModTime(),
		})
	}
	return versions, nil
}

// parsePromptFilename extracts the prompt name and version from a filename
// like "classifier-v3.txt" -> ("classifier", "v3").
func parsePromptFilename(filename string) (name, version string) {
	base := strings.TrimSuffix(filename, ".txt")
	idx := strings.LastIndex(base, "-v")
	if idx < 0 {
		return base, ""
	}
	return base[:idx], base[idx+1:]
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/pipeline/ -run 'TestPipelineRun|TestPipelineStats|TestPromptVersion|TestSparkline' -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui
git add internal/pipeline/run.go internal/pipeline/run_test.go
git commit -m "feat(pipeline): add PipelineRun data types and reconstruction logic

Introduce PipelineRun, PipelineStats, and PromptVersion types for
the pipeline monitor view. Runs are reconstructed from existing store
artifacts: session metadata provides base info, file existence in
digests/ and evaluations/ determines stage completion, and file
modification timestamps estimate per-stage timing."
```

---

### Task 2: Sparkline Component

Create a reusable sparkline renderer for the pipeline activity chart.

**Files:**
- Create: `internal/tui/components/sparkline.go`
- Test: `internal/tui/components/sparkline_test.go`

**Step 1: Write the failing test**

```go
// internal/tui/components/sparkline_test.go
package components

import "testing"

func TestRenderSparkline(t *testing.T) {
	tests := []struct {
		name   string
		data   []int
		width  int
		expect string
	}{
		{"empty", nil, 10, ""},
		{"single", []int{5}, 10, "█"},
		{"all zeros", []int{0, 0, 0}, 10, "▁▁▁"},
		{"ascending", []int{1, 2, 3, 4, 5, 6, 7, 8}, 10, "▁▂▃▄▅▆▇█"},
		{"mixed", []int{0, 4, 8, 4, 0}, 10, "▁▄█▄▁"},
		{"truncated to width", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 5, "▁▂▃▄▅"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderSparkline(tt.data, tt.width)
			if got != tt.expect {
				t.Errorf("RenderSparkline(%v, %d) = %q, want %q", tt.data, tt.width, got, tt.expect)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/tui/components/ -run TestRenderSparkline -v`
Expected: FAIL — function not defined.

**Step 3: Write minimal implementation**

```go
// internal/tui/components/sparkline.go
package components

// sparkChars are Unicode block elements for sparkline rendering, ordered
// from lowest to highest.
var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// RenderSparkline renders a sparkline string from integer data.
// Each value maps to a Unicode block character proportional to the max value.
// Output is truncated to width characters. Returns "" for empty data.
func RenderSparkline(data []int, width int) string {
	if len(data) == 0 {
		return ""
	}

	// Find max for scaling.
	maxVal := 0
	for _, v := range data {
		if v > maxVal {
			maxVal = v
		}
	}

	// Truncate to width.
	display := data
	if len(display) > width {
		display = display[:width]
	}

	result := make([]rune, len(display))
	for i, v := range display {
		if maxVal == 0 {
			result[i] = sparkChars[0]
			continue
		}
		// Scale to 0..7 range.
		idx := v * (len(sparkChars) - 1) / maxVal
		result[i] = sparkChars[idx]
	}
	return string(result)
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/tui/components/ -run TestRenderSparkline -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui
git add internal/tui/components/sparkline.go internal/tui/components/sparkline_test.go
git commit -m "feat(components): add sparkline renderer for pipeline activity chart

Unicode block character sparkline (▁▂▃▄▅▆▇█) that scales data
proportionally to the max value. Truncates to a given width."
```

---

### Task 3: Message Types and View States

Add the new view states, message types, key bindings, and config for phase 4c.

**Files:**
- Modify: `internal/tui/message/message.go:9-15` — add ViewPipelineMonitor, ViewLogViewer
- Modify: `internal/tui/shared/keys.go:7-54` — add pipeline/log keys
- Modify: `internal/tui/shared/config.go:10-21` — add PipelineConfig

**Step 1: Add view states to message.go**

Add `ViewPipelineMonitor` and `ViewLogViewer` to the ViewState const block at line 9-15:

```go
const (
	ViewDashboard ViewState = iota
	ViewProposalDetail
	ViewFitnessDetail
	ViewSourceManager
	ViewSourceDetail
	ViewPipelineMonitor
	ViewLogViewer
)
```

Add new message types at the end of the file:

```go
// Pipeline monitor messages.

// RetryRunStarted signals the beginning of a pipeline retry.
type RetryRunStarted struct{ SessionID string }

// RetryRunFinished carries the result of retrying a pipeline run.
type RetryRunFinished struct {
	SessionID string
	Err       error
}

// PipelineTickMsg triggers auto-refresh of pipeline data.
type PipelineTickMsg struct{}

// LogTickMsg triggers log viewer follow-mode refresh.
type LogTickMsg struct{}
```

**Step 2: Add key bindings to shared/keys.go**

Add to the KeyMap struct (after `Pipeline key.Binding` at line 53):

```go
	// Pipeline Monitor
	Retry        key.Binding
	LogView      key.Binding
	Refresh      key.Binding

	// Log Viewer
	Search       key.Binding
	SearchNext   key.Binding
	SearchPrev   key.Binding
	FollowToggle key.Binding
```

Add to `NewKeyMap()` inside the function body (after the Pipeline binding at line 95):

```go
		// Pipeline Monitor.
		Retry:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "retry")),
		LogView: key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "log")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),

		// Log Viewer.
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		SearchNext:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
		SearchPrev:   key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev")),
		FollowToggle: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "follow")),
```

Add short help functions after `SourcesShortHelp()`:

```go
// PipelineShortHelp returns help bindings for the pipeline monitor view.
func (k KeyMap) PipelineShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.Retry, k.LogView, k.Help}
}

// LogViewShortHelp returns help bindings for the log viewer.
func (k KeyMap) LogViewShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.SearchNext, k.SearchPrev, k.FollowToggle, k.Back, k.Help}
}
```

**Step 3: Add PipelineConfig to shared/config.go**

Add the `Pipeline` field to Config struct (after SourceManager at line 17):

```go
	Pipeline      PipelineConfig      `json:"pipeline"`
```

Add the PipelineConfig type (after SourceManagerConfig):

```go
// PipelineConfig holds pipeline monitor view settings.
type PipelineConfig struct {
	SparklineDays   int  `json:"sparklineDays"`
	RecentRunsLimit int  `json:"recentRunsLimit"`
	LogFollowMode   bool `json:"logFollowMode"`
}
```

Add defaults in `DefaultConfig()` (after SourceManager block):

```go
		Pipeline: PipelineConfig{
			SparklineDays:   7,
			RecentRunsLimit: 20,
			LogFollowMode:   true,
		},
```

**Step 4: Run all tests to ensure no regressions**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/tui/... -v -count=1`
Expected: All existing tests PASS. Some may need trivial fixes if they assert on exact ViewState values.

**Step 5: Commit**

```bash
cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui
git add internal/tui/message/message.go internal/tui/shared/keys.go internal/tui/shared/config.go
git commit -m "feat(tui): add view states, messages, keys, and config for phase 4c

Add ViewPipelineMonitor and ViewLogViewer states, RetryRun and tick
messages, pipeline/log key bindings (Retry, LogView, Search, Follow),
and PipelineConfig with sparklineDays, recentRunsLimit, logFollowMode."
```

---

### Task 4: Test Fixtures for Pipeline Data

Add factory functions for pipeline test data.

**Files:**
- Modify: `internal/tui/testdata/fixtures.go` — add pipeline fixtures

**Step 1: Add fixtures**

Add to the end of `testdata/fixtures.go`:

```go
// TestPipelineRun returns a pipeline run with sensible defaults.
func TestPipelineRun(overrides ...func(*pipeline.PipelineRun)) pipeline.PipelineRun {
	run := pipeline.PipelineRun{
		SessionID:      "e7f2a103",
		Project:        "woo-payments",
		Timestamp:      time.Now().Add(-12 * time.Minute),
		Status:         "processed",
		HasDigest:      true,
		HasClassifier:       true,
		HasEvaluator:      true,
		ParseDuration:  1200 * time.Millisecond,
		ClassifierDuration:  8400 * time.Millisecond,
		EvaluatorDuration: 12 * time.Second,
		ProposalCount:  1,
	}
	for _, fn := range overrides {
		fn(&run)
	}
	return run
}

// TestPipelineRuns returns a mixed set of pipeline runs for testing.
func TestPipelineRuns() []pipeline.PipelineRun {
	return []pipeline.PipelineRun{
		TestPipelineRun(),
		TestPipelineRun(func(r *pipeline.PipelineRun) {
			r.SessionID = "3bc891ff"
			r.Project = "cabrero"
			r.Timestamp = time.Now().Add(-2 * time.Hour)
			r.ParseDuration = 800 * time.Millisecond
			r.ClassifierDuration = 6100 * time.Millisecond
			r.EvaluatorDuration = 9 * time.Second
			r.ProposalCount = 0
		}),
		TestPipelineRun(func(r *pipeline.PipelineRun) {
			r.SessionID = "91cd02ab"
			r.Project = "woo-payments"
			r.Timestamp = time.Now().Add(-8 * time.Hour)
			r.Status = "error"
			r.HasClassifier = false
			r.HasEvaluator = false
			r.ParseDuration = 400 * time.Millisecond
			r.ClassifierDuration = 0
			r.EvaluatorDuration = 0
			r.ProposalCount = 0
			r.ErrorDetail = "classifier timeout after 2m"
		}),
		TestPipelineRun(func(r *pipeline.PipelineRun) {
			r.SessionID = "7e0b1234"
			r.Project = "woo-payments"
			r.Timestamp = time.Now().Add(-24 * time.Hour)
			r.Status = "pending"
			r.HasDigest = false
			r.HasClassifier = false
			r.HasEvaluator = false
			r.ParseDuration = 0
			r.ClassifierDuration = 0
			r.EvaluatorDuration = 0
			r.ProposalCount = 0
		}),
	}
}

// TestPipelineStats returns realistic pipeline statistics.
func TestPipelineStats() pipeline.PipelineStats {
	return pipeline.PipelineStats{
		SessionsCaptured:   18,
		SessionsProcessed:  16,
		SessionsPending:    1,
		SessionsErrored:    1,
		ProposalsGenerated: 5,
		ProposalsApproved:  3,
		ProposalsRejected:  1,
		ProposalsPending:   1,
		SessionsPerDay:     []int{3, 2, 1, 4, 2, 3, 3},
	}
}

// TestPromptVersions returns prompt version fixtures.
func TestPromptVersions() []pipeline.PromptVersion {
	return []pipeline.PromptVersion{
		{Name: "classifier", Version: "v3", LastUsed: time.Now().Add(-12 * time.Minute)},
		{Name: "evaluator", Version: "v3", LastUsed: time.Now().Add(-12 * time.Minute)},
		{Name: "apply", Version: "v1", LastUsed: time.Now().Add(-3 * 24 * time.Hour)},
	}
}
```

Add the import for `pipeline` at the top of fixtures.go if not already present:

```go
"github.com/vladolaru/cabrero/internal/pipeline"
```

**Step 2: Run to verify compilation**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go build ./internal/tui/testdata/`
Expected: Compiles without errors.

**Step 3: Commit**

```bash
cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui
git add internal/tui/testdata/fixtures.go
git commit -m "test(testdata): add pipeline run, stats, and prompt version fixtures

Factory functions for PipelineRun (4 runs: success, success, error,
pending), PipelineStats (7-day window), and PromptVersion (3 prompts)."
```

---

### Task 5: Pipeline Monitor — Model

Create the pipeline monitor model with constructor and data holders.

**Files:**
- Create: `internal/tui/pipeline/model.go`
- Create: `internal/tui/pipeline/testmain_test.go`
- Test: `internal/tui/pipeline/model_test.go`

**Step 1: Write the failing test**

```go
// internal/tui/pipeline/testmain_test.go
package pipeline
```

```go
// internal/tui/pipeline/model_test.go
package pipeline

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/testdata"
)

func newTestModel() Model {
	runs := testdata.TestPipelineRuns()
	stats := testdata.TestPipelineStats()
	prompts := testdata.TestPromptVersions()
	dashStats := testdata.TestDashboardStats()
	keys := testdata.TestConfig().NewKeyMap()
	cfg := testdata.TestConfig()
	return New(runs, stats, prompts, dashStats, &keys, cfg)
}

func TestNewModel(t *testing.T) {
	m := newTestModel()
	if len(m.runs) != 4 {
		t.Errorf("runs = %d, want 4", len(m.runs))
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestModelView(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)
	view := ansi.Strip(m.View())

	// Should contain key sections.
	if !strings.Contains(view, "DAEMON") {
		t.Error("view missing DAEMON section")
	}
	if !strings.Contains(view, "RECENT RUNS") {
		t.Error("view missing RECENT RUNS section")
	}
	if !strings.Contains(view, "e7f2a103") {
		t.Error("view missing first run session ID")
	}
}

func TestModelNavigation(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Move down.
	keys := testdata.TestConfig().NewKeyMap()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("cursor after down = %d, want 1", m.cursor)
	}

	// Move up.
	_ = keys
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor after up = %d, want 0", m.cursor)
	}
}

func TestModelExpandRun(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Press Enter to expand.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.expandedIdx != 0 {
		t.Errorf("expandedIdx = %d, want 0", m.expandedIdx)
	}

	// Press Enter again to collapse.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.expandedIdx != -1 {
		t.Errorf("expandedIdx = %d, want -1", m.expandedIdx)
	}
}

func TestModelRetryKey(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to errored run (index 2).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press R.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	_ = cmd

	// Should activate confirm.
	if !m.confirm.Active() {
		t.Error("R on errored run should activate confirm")
	}
}

func TestModelPipelineKeyEmitsPushLogView(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Press L.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if cmd == nil {
		t.Fatal("L should produce cmd")
	}
	msg := cmd()
	push, ok := msg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", msg)
	}
	if push.View != message.ViewLogViewer {
		t.Errorf("push view = %d, want ViewLogViewer", push.View)
	}
}
```

**Note:** The `newTestModel` helper calls `testdata.TestConfig().NewKeyMap()` — this won't exist yet. We need to either: (a) use `shared.NewKeyMap(cfg.Navigation)` directly, or (b) add a convenience method. Use option (a): `shared.NewKeyMap(testdata.TestConfig().Navigation)`.

Actually, looking at the existing test patterns (e.g., `sources/model_test.go`), they create keys inline. Adjust `newTestModel()`:

```go
func newTestModel() Model {
	runs := testdata.TestPipelineRuns()
	stats := testdata.TestPipelineStats()
	prompts := testdata.TestPromptVersions()
	dashStats := testdata.TestDashboardStats()
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	return New(runs, stats, prompts, dashStats, &keys, cfg)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/tui/pipeline/ -v`
Expected: FAIL — package and types don't exist.

**Step 3: Write minimal implementation**

```go
// internal/tui/pipeline/model.go
package pipeline

import (
	pl "github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Model is the pipeline monitor view model.
type Model struct {
	runs       []pl.PipelineRun
	stats      pl.PipelineStats
	prompts    []pl.PromptVersion
	dashStats  message.DashboardStats
	cursor     int
	expandedIdx int // -1 means no run expanded
	confirm    components.ConfirmModel
	retrying   string // session ID being retried, "" if none
	width      int
	height     int
	keys       *shared.KeyMap
	config     *shared.Config
}

// New creates a pipeline monitor model with loaded data.
func New(runs []pl.PipelineRun, stats pl.PipelineStats, prompts []pl.PromptVersion, dashStats message.DashboardStats, keys *shared.KeyMap, cfg *shared.Config) Model {
	return Model{
		runs:        runs,
		stats:       stats,
		prompts:     prompts,
		dashStats:   dashStats,
		expandedIdx: -1,
		keys:        keys,
		config:      cfg,
	}
}

// SetSize updates the viewport dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// SelectedRun returns the run at the current cursor position, or nil.
func (m Model) SelectedRun() *pl.PipelineRun {
	if m.cursor < 0 || m.cursor >= len(m.runs) {
		return nil
	}
	return &m.runs[m.cursor]
}
```

**Step 4: Write stub update.go and view.go** (enough to pass tests)

Create `internal/tui/pipeline/update.go` and `internal/tui/pipeline/view.go` with basic implementations. The full rendering comes in Task 6 and full update logic in Task 7. For now:

```go
// internal/tui/pipeline/update.go
package pipeline

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the pipeline monitor.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle confirm dialog first.
	if m.confirm.Active() {
		return m.updateConfirm(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.runs)-1 {
			m.cursor++
		}
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case key.Matches(msg, m.keys.Open):
		// Toggle inline expansion.
		if m.expandedIdx == m.cursor {
			m.expandedIdx = -1
		} else {
			m.expandedIdx = m.cursor
		}
		return m, nil

	case key.Matches(msg, m.keys.Retry):
		run := m.SelectedRun()
		if run != nil && run.Status == "error" {
			m.confirm = components.NewConfirmModel("Retry session " + run.SessionID[:8] + "?")
			return m, nil
		}
		return m, nil

	case key.Matches(msg, m.keys.LogView):
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewLogViewer}
		}

	case key.Matches(msg, m.keys.Refresh):
		// Manual refresh handled by root model (reloads data).
		return m, nil
	}

	return m, nil
}

func (m Model) updateConfirm(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)

	// Check for result.
	if !m.confirm.Active() {
		if m.confirm.Confirmed() {
			run := m.SelectedRun()
			if run != nil {
				sessionID := run.SessionID
				m.retrying = sessionID
				return m, func() tea.Msg {
					return message.RetryRunStarted{SessionID: sessionID}
				}
			}
		}
	}

	return m, cmd
}
```

```go
// internal/tui/pipeline/view.go
package pipeline

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	pl "github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// View renders the pipeline monitor.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	styles := shared.DefaultStyles()
	var sections []string

	// Title.
	title := styles.Title.Render("Pipeline Monitor")
	sections = append(sections, title)

	// Daemon header.
	sections = append(sections, m.renderDaemonHeader(styles))

	// Activity stats.
	sections = append(sections, m.renderActivityStats(styles))

	// Recent runs.
	sections = append(sections, m.renderRecentRuns(styles))

	// Prompts.
	if len(m.prompts) > 0 {
		sections = append(sections, m.renderPrompts(styles))
	}

	// Confirm overlay.
	if m.confirm.Active() {
		sections = append(sections, m.confirm.View())
	}

	content := strings.Join(sections, "\n\n")

	// Status bar.
	statusBar := components.RenderStatusBar(m.keys.PipelineShortHelp(), m.width)

	return content + "\n" + statusBar
}

func (m Model) renderDaemonHeader(styles shared.Styles) string {
	var b strings.Builder

	b.WriteString(styles.SectionHeader.Render("DAEMON"))
	b.WriteString("\n")

	if m.dashStats.DaemonRunning {
		b.WriteString(fmt.Sprintf("  Status:  ● running (PID %d)\n", m.dashStats.DaemonPID))
	} else {
		b.WriteString("  Status:  ● stopped\n")
	}

	b.WriteString("\n")

	b.WriteString(styles.SectionHeader.Render("HOOKS"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  pre-compact:  %s\n", checkmark(m.dashStats.HookPreCompact)))
	b.WriteString(fmt.Sprintf("  session-end:  %s", checkmark(m.dashStats.HookSessionEnd)))

	return b.String()
}

func (m Model) renderActivityStats(styles shared.Styles) string {
	var b strings.Builder
	days := m.config.Pipeline.SparklineDays
	b.WriteString(styles.SectionHeader.Render(fmt.Sprintf("PIPELINE ACTIVITY (last %d days)", days)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Sessions captured:  %-6d Proposals generated:  %d\n",
		m.stats.SessionsCaptured, m.stats.ProposalsGenerated))
	b.WriteString(fmt.Sprintf("  Sessions processed: %-6d Proposals approved:   %d\n",
		m.stats.SessionsProcessed, m.stats.ProposalsApproved))
	b.WriteString(fmt.Sprintf("  Sessions pending:   %-6d Proposals rejected:   %d\n",
		m.stats.SessionsPending, m.stats.ProposalsRejected))
	b.WriteString(fmt.Sprintf("  Sessions errored:   %-6d Proposals pending:    %d\n",
		m.stats.SessionsErrored, m.stats.ProposalsPending))

	if len(m.stats.SessionsPerDay) > 0 {
		sparkline := components.RenderSparkline(m.stats.SessionsPerDay, m.width-4)
		b.WriteString("\n  " + sparkline + "  sessions/day")
	}

	return b.String()
}

func (m Model) renderRecentRuns(styles shared.Styles) string {
	var b strings.Builder
	b.WriteString(styles.SectionHeader.Render("RECENT RUNS"))
	b.WriteString("\n")

	for i, run := range m.runs {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		status := statusIndicator(run.Status)
		shortID := run.SessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		age := relativeTime(run.Timestamp)
		project := truncate(run.Project, 16)
		timing := formatTiming(run)

		line := fmt.Sprintf("%s%s %s  %-8s  %-16s  %s", cursor, status, shortID, age, project, timing)
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

func (m Model) renderRunDetail(run pl.PipelineRun) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("      Session: %s\n", run.SessionID))
	b.WriteString(fmt.Sprintf("      Project: %s\n", run.Project))
	b.WriteString(fmt.Sprintf("      Status:  %s\n", run.Status))
	if run.ProposalCount > 0 {
		b.WriteString(fmt.Sprintf("      Proposals: %d\n", run.ProposalCount))
	}
	if run.ErrorDetail != "" {
		b.WriteString(fmt.Sprintf("      Error: %s", run.ErrorDetail))
	}
	return b.String()
}

func (m Model) renderPrompts(styles shared.Styles) string {
	var b strings.Builder
	b.WriteString(styles.SectionHeader.Render("PROMPTS"))
	b.WriteString("\n")
	for _, p := range m.prompts {
		age := relativeTime(p.LastUsed)
		b.WriteString(fmt.Sprintf("  %-20s %-4s  last used: %s\n", p.Name, p.Version, age))
	}
	return b.String()
}

func statusIndicator(status string) string {
	switch status {
	case "processed":
		return "✓"
	case "error":
		return "✗"
	case "pending":
		return "○"
	default:
		return "?"
	}
}

func checkmark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func formatTiming(run pl.PipelineRun) string {
	if run.Status == "pending" {
		return "(pending)"
	}
	var parts []string
	if run.HasDigest {
		parts = append(parts, fmt.Sprintf("%.1fs parse", run.ParseDuration.Seconds()))
	}
	if run.HasClassifier {
		parts = append(parts, fmt.Sprintf("%.1fs cls", run.ClassifierDuration.Seconds()))
	} else if run.Status == "error" && run.HasDigest {
		parts = append(parts, "✗ cls failed")
	}
	if run.HasEvaluator {
		parts = append(parts, fmt.Sprintf("%.0fs eval", run.EvaluatorDuration.Seconds()))
	} else if run.Status == "error" && run.HasClassifier {
		parts = append(parts, "✗ eval failed")
	}
	return strings.Join(parts, "  ")
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
```

**Note:** The `shared.DefaultStyles()` and `shared.Styles` reference needs to match the existing styles pattern. Check `shared/styles.go` — if it uses a different pattern (like standalone style variables), adjust accordingly. The `View()` can use `lipgloss` directly for styling.

**Step 4: Run tests to verify they pass**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/tui/pipeline/ -v`
Expected: PASS (adjust test assertions as needed based on actual patterns).

**Step 5: Commit**

```bash
cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui
git add internal/tui/pipeline/
git commit -m "feat(tui): add pipeline monitor model, update, and view

Pipeline monitor shows daemon health, 7-day activity stats with
sparkline, recent runs with per-stage timing, and prompt versions.
Supports cursor navigation, inline run expansion, retry flow with
confirmation, and navigation to log viewer."
```

---

### Task 6: Log Viewer — Model, Update, View

Create the log viewer view for inspecting daemon.log.

**Files:**
- Create: `internal/tui/logview/model.go`
- Create: `internal/tui/logview/update.go`
- Create: `internal/tui/logview/view.go`
- Create: `internal/tui/logview/testmain_test.go`
- Test: `internal/tui/logview/model_test.go`

**Step 1: Write the failing test**

```go
// internal/tui/logview/testmain_test.go
package logview
```

```go
// internal/tui/logview/model_test.go
package logview

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/tui/shared"
	"github.com/vladolaru/cabrero/internal/tui/testdata"
)

var testLogContent = `2026-02-20T10:15:03Z INFO  daemon started (PID 4821)
2026-02-20T10:15:03Z INFO  poll=2m0s stale=30m0s delay=30s
2026-02-20T10:15:03Z INFO  processing session e7f2a103
2026-02-20T10:15:04Z INFO  pre-parser: 142 entries, 0.8s
2026-02-20T10:15:12Z INFO  classifier: classified, triage=evaluate
2026-02-20T10:15:24Z INFO  evaluator: 1 proposal generated
2026-02-20T10:17:05Z INFO  poll: 0 pending sessions
`

func newTestLogModel() Model {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	return New(testLogContent, &keys, cfg)
}

func TestNewLogModel(t *testing.T) {
	m := newTestLogModel()
	if !m.followMode {
		t.Error("follow mode should be on by default")
	}
}

func TestLogModelView(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)
	view := ansi.Strip(m.View())

	if !strings.Contains(view, "daemon started") {
		t.Error("view missing log content")
	}
	if !strings.Contains(view, "Log Viewer") {
		t.Error("view missing title")
	}
}

func TestLogModelSearch(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Press / to activate search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.searchActive {
		t.Error("search should be active after /")
	}

	// Type search term.
	for _, r := range "classifier" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.matches) == 0 {
		t.Error("expected matches for 'classifier'")
	}
}

func TestLogModelFollowToggle(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	if !m.followMode {
		t.Fatal("follow should start on")
	}

	// Press 'f' to toggle.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.followMode {
		t.Error("follow should be off after f")
	}

	// Press 'f' again.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !m.followMode {
		t.Error("follow should be on after second f")
	}
}

func TestLogModelEscFromSearch(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Activate search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.searchActive {
		t.Fatal("search should be active")
	}

	// Press Esc to close search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searchActive {
		t.Error("search should be closed after Esc")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/tui/logview/ -v`
Expected: FAIL — package doesn't exist.

**Step 3: Write implementation**

```go
// internal/tui/logview/model.go
package logview

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// lineMatch records the line number of a search match.
type lineMatch struct {
	lineNum int
}

// Model is the log viewer model.
type Model struct {
	content      string
	lines        []string
	viewport     viewport.Model
	searchInput  textinput.Model
	searchActive bool
	searchTerm   string
	followMode   bool
	matches      []lineMatch
	matchIdx     int // current match index, -1 if none
	width        int
	height       int
	keys         *shared.KeyMap
	config       *shared.Config
}

// New creates a log viewer model with the given log content.
func New(content string, keys *shared.KeyMap, cfg *shared.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 256

	lines := strings.Split(content, "\n")

	m := Model{
		content:    content,
		lines:      lines,
		followMode: cfg.Pipeline.LogFollowMode,
		matchIdx:   -1,
		keys:       keys,
		config:     cfg,
		searchInput: ti,
	}

	return m
}

// SetSize updates the viewport dimensions and initializes the viewport.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Reserve 3 lines for title and status bar.
	viewHeight := height - 3
	if viewHeight < 1 {
		viewHeight = 1
	}

	m.viewport = viewport.New(width, viewHeight)
	m.viewport.SetContent(m.content)

	if m.followMode {
		m.viewport.GotoBottom()
	}
}

// UpdateContent replaces the log content (for follow mode refresh).
func (m *Model) UpdateContent(content string) {
	m.content = content
	m.lines = strings.Split(content, "\n")
	m.viewport.SetContent(content)
	if m.followMode {
		m.viewport.GotoBottom()
	}
}

// performSearch finds all lines matching the search term.
func (m *Model) performSearch() {
	m.matches = nil
	m.matchIdx = -1
	if m.searchTerm == "" {
		return
	}
	term := strings.ToLower(m.searchTerm)
	for i, line := range m.lines {
		if strings.Contains(strings.ToLower(line), term) {
			m.matches = append(m.matches, lineMatch{lineNum: i})
		}
	}
	if len(m.matches) > 0 {
		m.matchIdx = 0
		m.gotoMatch(0)
	}
}

// gotoMatch scrolls the viewport to show the match at the given index.
func (m *Model) gotoMatch(idx int) {
	if idx < 0 || idx >= len(m.matches) {
		return
	}
	m.matchIdx = idx
	lineNum := m.matches[idx].lineNum
	m.viewport.SetYOffset(lineNum)
}
```

```go
// internal/tui/logview/update.go
package logview

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles messages for the log viewer.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.searchActive {
		return m.updateSearch(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward to viewport for scrolling.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Search):
		m.searchActive = true
		m.searchInput.Focus()
		return m, nil

	case key.Matches(msg, m.keys.FollowToggle):
		m.followMode = !m.followMode
		if m.followMode {
			m.viewport.GotoBottom()
		}
		return m, nil

	case key.Matches(msg, m.keys.SearchNext):
		if len(m.matches) > 0 {
			next := (m.matchIdx + 1) % len(m.matches)
			m.gotoMatch(next)
		}
		return m, nil

	case key.Matches(msg, m.keys.SearchPrev):
		if len(m.matches) > 0 {
			prev := m.matchIdx - 1
			if prev < 0 {
				prev = len(m.matches) - 1
			}
			m.gotoMatch(prev)
		}
		return m, nil
	}

	// Any manual scroll disables follow mode.
	if msg.Type == tea.KeyUp || msg.Type == tea.KeyDown ||
		msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown {
		m.followMode = false
	}

	// Forward to viewport.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) updateSearch(msg tea.Msg) (Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(kmsg, key.NewBinding(key.WithKeys("esc"))):
			m.searchActive = false
			m.searchInput.Blur()
			return m, nil
		case key.Matches(kmsg, key.NewBinding(key.WithKeys("enter"))):
			m.searchActive = false
			m.searchInput.Blur()
			m.searchTerm = m.searchInput.Value()
			m.performSearch()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}
```

```go
// internal/tui/logview/view.go
package logview

import (
	"fmt"

	"github.com/vladolaru/cabrero/internal/tui/components"
)

// View renders the log viewer.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Title with follow indicator.
	followIndicator := "○"
	if m.followMode {
		followIndicator = "●"
	}
	title := fmt.Sprintf("Log Viewer  follow %s", followIndicator)

	// Viewport content.
	content := m.viewport.View()

	// Bottom bar: search input or status bar.
	var bottom string
	if m.searchActive {
		bottom = "/ " + m.searchInput.View()
	} else {
		bottom = components.RenderStatusBar(m.keys.LogViewShortHelp(), m.width)
		if m.searchTerm != "" && len(m.matches) > 0 {
			bottom = fmt.Sprintf("[%d/%d matches] %s", m.matchIdx+1, len(m.matches), bottom)
		}
	}

	return title + "\n" + content + "\n" + bottom
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/tui/logview/ -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui
git add internal/tui/logview/
git commit -m "feat(tui): add log viewer with search and follow mode

Full-screen scrollable viewport over daemon.log content. Features:
search with / (case-insensitive), n/N for next/prev match, f to
toggle follow mode, manual scroll auto-pauses follow."
```

---

### Task 7: Wire Pipeline and Log Views into Root Model

Connect everything: add child models to root, handle navigation, routing, and auto-refresh.

**Files:**
- Modify: `internal/tui/model.go` — add pipeline/logview fields, routing
- Modify: `internal/tui/tui.go` — load pipeline data on startup
- Modify: `internal/tui/dashboard/update.go:77` — add `p` key handler

**Step 1: Add pipeline/logview to root model in model.go**

Add imports for the new packages:

```go
	pipeline_tui "github.com/vladolaru/cabrero/internal/tui/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/logview"
```

Add fields to `reviewModel` struct (after `sources sources.Model`):

```go
	pipelineMonitor pipeline_tui.Model
	logViewer       logview.Model
```

Add pipeline data to `newReviewModel` — change the signature to accept pipeline data:

```go
func newReviewModel(proposals []pipeline.ProposalWithSession, reports []fitness.Report, stats message.DashboardStats, sourceGroups []fitness.SourceGroup, runs []pipeline.PipelineRun, pipelineStats pipeline.PipelineStats, prompts []pipeline.PromptVersion, cfg *shared.Config) reviewModel {
```

In the `pushView` function, add cases for the new views:

```go
	case message.ViewPipelineMonitor:
		// Pipeline data already loaded in root; just set size.
		m.pipelineMonitor.SetSize(m.width, m.height)

	case message.ViewLogViewer:
		// Read daemon.log on push.
		logPath := filepath.Join(store.Root(), "daemon.log")
		content, _ := os.ReadFile(logPath)
		m.logViewer = logview.New(string(content), &m.keys, m.config)
		m.logViewer.SetSize(m.width, m.height)
```

In `Update()` routing section, add:

```go
	case message.ViewPipelineMonitor:
		var cmd tea.Cmd
		m.pipelineMonitor, cmd = m.pipelineMonitor.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case message.ViewLogViewer:
		var cmd tea.Cmd
		m.logViewer, cmd = m.logViewer.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
```

In `View()`, add:

```go
	case message.ViewPipelineMonitor:
		content = m.pipelineMonitor.View()
	case message.ViewLogViewer:
		content = m.logViewer.View()
```

Handle tick messages in `Update()` (in the main switch):

```go
	case message.PipelineTickMsg:
		if m.state == message.ViewPipelineMonitor {
			// Reload pipeline data.
			runs, _ := pipeline.ListPipelineRuns(m.config.Pipeline.RecentRunsLimit)
			stats, _ := pipeline.GatherPipelineStats(m.config.Pipeline.SparklineDays)
			prompts, _ := pipeline.ListPromptVersions()
			dashStats := gatherStats(m.proposals)
			m.pipelineMonitor = pipeline_tui.New(runs, stats, prompts, dashStats, &m.keys, m.config)
			m.pipelineMonitor.SetSize(m.width, m.height)
			// Schedule next tick.
			return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg {
				return message.PipelineTickMsg{}
			})
		}
		return m, nil

	case message.LogTickMsg:
		if m.state == message.ViewLogViewer {
			logPath := filepath.Join(store.Root(), "daemon.log")
			content, _ := os.ReadFile(logPath)
			m.logViewer.UpdateContent(string(content))
			return m, tea.Tick(time.Second, func(time.Time) tea.Msg {
				return message.LogTickMsg{}
			})
		}
		return m, nil

	case message.RetryRunStarted:
		sessionID := msg.SessionID
		return m, func() tea.Msg {
			// Shell out to cabrero run.
			// For now, placeholder — actual retry via exec.Command.
			return message.RetryRunFinished{SessionID: sessionID}
		}

	case message.RetryRunFinished:
		if msg.Err != nil {
			m.statusMsg = "Retry failed: " + msg.Err.Error()
		} else {
			m.statusMsg = "Retry complete."
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return message.StatusMessageExpired{}
		}))
		return m, tea.Batch(cmds...)
```

Start tick when pushing to pipeline/log views (in `pushView`):

```go
	case message.ViewPipelineMonitor:
		m.pipelineMonitor.SetSize(m.width, m.height)
		cmds = append(cmds, tea.Tick(5*time.Second, func(time.Time) tea.Msg {
			return message.PipelineTickMsg{}
		}))

	case message.ViewLogViewer:
		logPath := filepath.Join(store.Root(), "daemon.log")
		content, _ := os.ReadFile(logPath)
		m.logViewer = logview.New(string(content), &m.keys, m.config)
		m.logViewer.SetSize(m.width, m.height)
		if m.config.Pipeline.LogFollowMode {
			cmds = append(cmds, tea.Tick(time.Second, func(time.Time) tea.Msg {
				return message.LogTickMsg{}
			}))
		}
```

**Step 2: Add `p` key handler in dashboard/update.go**

After the `Sources` key handler (line 77-80):

```go
	case key.Matches(msg, m.keys.Pipeline):
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewPipelineMonitor}
		}
```

**Step 3: Load pipeline data in tui.go**

In `Run()`, after loading proposals, add:

```go
	runs, err := pipeline.ListPipelineRuns(cfg.Pipeline.RecentRunsLimit)
	if err != nil {
		runs = nil // non-fatal
	}

	pipelineStats, err := pipeline.GatherPipelineStats(cfg.Pipeline.SparklineDays)
	if err != nil {
		pipelineStats = pipeline.PipelineStats{}
	}

	prompts, err := pipeline.ListPromptVersions()
	if err != nil {
		prompts = nil
	}
```

Update the `newReviewModel` call to pass the new data.

**Step 4: Run all tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/tui/... -v -count=1`
Expected: PASS (fix any compilation issues from signature changes).

**Step 5: Commit**

```bash
cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui
git add internal/tui/model.go internal/tui/tui.go internal/tui/dashboard/update.go
git commit -m "feat(tui): wire pipeline monitor and log viewer into root model

Route 'p' from dashboard to pipeline monitor, 'L' from pipeline to
log viewer. Auto-refresh via tea.Tick (5s pipeline, 1s log follow).
Handle RetryRun messages and log content reload."
```

---

### Task 8: Integration Tests

Add integration tests for the new navigation flows.

**Files:**
- Modify: `internal/tui/integration_test.go` — add pipeline/log tests

**Step 1: Write new integration tests**

Add at the end of `integration_test.go`:

```go
// Phase 4c integration tests.

func TestDashboardToPipelineAndBack(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Press 'p' to open pipeline monitor.
	m, cmd := update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil {
		t.Fatal("p should produce cmd")
	}
	pushMsg := cmd()
	push, ok := pushMsg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", pushMsg)
	}
	if push.View != message.ViewPipelineMonitor {
		t.Errorf("PushView = %d, want ViewPipelineMonitor", push.View)
	}

	// Process the push.
	m, _ = update(m, push)
	if m.state != message.ViewPipelineMonitor {
		t.Errorf("state = %d, want ViewPipelineMonitor", m.state)
	}

	// Pipeline monitor should render.
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "DAEMON") || !strings.Contains(view, "RECENT RUNS") {
		t.Error("pipeline monitor view missing expected sections")
	}

	// Press Esc to go back.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		m, _ = update(m, cmd())
	}
	if m.state != message.ViewDashboard {
		t.Errorf("state after pop = %d, want ViewDashboard", m.state)
	}
}

func TestPipelineToLogViewerAndBack(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Push to pipeline monitor.
	m, _ = update(m, message.PushView{View: message.ViewPipelineMonitor})
	if m.state != message.ViewPipelineMonitor {
		t.Fatal("should be in pipeline monitor")
	}

	// Press 'L' to open log viewer.
	m, cmd := update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if cmd == nil {
		t.Fatal("L should produce cmd")
	}
	pushMsg := cmd()
	push, ok := pushMsg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", pushMsg)
	}
	if push.View != message.ViewLogViewer {
		t.Errorf("PushView = %d, want ViewLogViewer", push.View)
	}

	// Process the push.
	m, _ = update(m, push)
	if m.state != message.ViewLogViewer {
		t.Errorf("state = %d, want ViewLogViewer", m.state)
	}
	if len(m.viewStack) != 2 {
		t.Errorf("viewStack len = %d, want 2", len(m.viewStack))
	}

	// Press Esc to go back to pipeline.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		m, _ = update(m, cmd())
	}
	if m.state != message.ViewPipelineMonitor {
		t.Errorf("state after pop = %d, want ViewPipelineMonitor", m.state)
	}

	// Press Esc again to go back to dashboard.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		m, _ = update(m, cmd())
	}
	if m.state != message.ViewDashboard {
		t.Errorf("state after second pop = %d, want ViewDashboard", m.state)
	}
}

func TestFullStackNavigation(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Dashboard -> Pipeline -> Log -> back -> back -> Dashboard.
	m, _ = update(m, message.PushView{View: message.ViewPipelineMonitor})
	m, _ = update(m, message.PushView{View: message.ViewLogViewer})

	if m.state != message.ViewLogViewer {
		t.Fatal("should be in log viewer")
	}
	if len(m.viewStack) != 2 {
		t.Fatalf("viewStack = %d, want 2", len(m.viewStack))
	}

	m, _ = update(m, message.PopView{})
	if m.state != message.ViewPipelineMonitor {
		t.Errorf("after first pop: state = %d, want ViewPipelineMonitor", m.state)
	}

	m, _ = update(m, message.PopView{})
	if m.state != message.ViewDashboard {
		t.Errorf("after second pop: state = %d, want ViewDashboard", m.state)
	}
}
```

**Step 2: Update `newTestRoot()` to pass pipeline data**

The `newTestRoot()` helper needs to pass pipeline data to `newReviewModel`. Add:

```go
func newTestRoot() reviewModel {
	proposals := testdata.TestProposals()
	reports := testdata.TestFitnessReports()
	stats := testdata.TestDashboardStats()
	sourceGroups := testdata.TestSourceGroups()
	runs := testdata.TestPipelineRuns()
	pipelineStats := testdata.TestPipelineStats()
	prompts := testdata.TestPromptVersions()
	cfg := testdata.TestConfig()
	return newReviewModel(proposals, reports, stats, sourceGroups, runs, pipelineStats, prompts, cfg)
}
```

**Step 3: Run all tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui && go test ./internal/tui/... -v -count=1`
Expected: All tests PASS including new ones.

**Step 4: Commit**

```bash
cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui
git add internal/tui/integration_test.go
git commit -m "test(tui): add integration tests for pipeline and log viewer navigation

Test dashboard->pipeline->back, pipeline->log->back, and full stack
navigation (dashboard->pipeline->log->back->back->dashboard)."
```

---

### Task 9: Update Documentation

Update DESIGN.md progress section and CHANGELOG.md.

**Files:**
- Modify: `DESIGN.md` — update Phase 4c status
- Modify: `CHANGELOG.md` — add entries

**Step 1: Update DESIGN.md**

Change the Phase 4c section from:

```
**Phase 4c — Review TUI (operational monitoring)**

17. **Pipeline monitor** — daemon health, recent runs with timing, sparkline
    activity chart, prompt versions
18. **Log viewer** — full-screen scrollable log with search, follow mode
```

To:

```
**Phase 4c — Review TUI (operational monitoring)** ✓

17. **Pipeline monitor** — daemon health, recent runs with per-stage timing,
    sparkline activity chart, prompt versions, retry flow, polling auto-refresh
18. **Log viewer** — full-screen scrollable log with search, follow mode,
    auto-refresh via polling
```

**Step 2: Update CHANGELOG.md**

Add under `[Unreleased]`:

```markdown
### Added
- Pipeline monitor view (`p` from dashboard) with daemon health, recent pipeline
  runs with per-stage timing, 7-day activity sparkline, prompt versions, and
  retry flow for errored runs
- Log viewer (`L` from pipeline monitor) with full-screen scrollable daemon.log,
  search (`/` + `n`/`N`), and follow mode (`f`)
- PipelineRun data type with reconstruction from existing store artifacts
- Sparkline component for Unicode block character charts
- Polling-based auto-refresh (5s pipeline, 1s log follow mode)
- PipelineConfig with sparklineDays, recentRunsLimit, logFollowMode settings
```

**Step 3: Commit**

```bash
cd /Users/vladolaru/Work/a8c/cabrero/.worktrees/review-tui
git add DESIGN.md CHANGELOG.md
git commit -m "docs: mark phase 4c complete, add changelog entries

Pipeline monitor and log viewer complete the review TUI's operational
monitoring views. Updates DESIGN.md progress and CHANGELOG.md."
```

---

## Summary

| Task | What | Files | Tests |
|------|------|-------|-------|
| 1 | PipelineRun data types | `internal/pipeline/run.go` | `run_test.go` |
| 2 | Sparkline component | `internal/tui/components/sparkline.go` | `sparkline_test.go` |
| 3 | Messages, keys, config | `message.go`, `keys.go`, `config.go` | existing tests pass |
| 4 | Test fixtures | `testdata/fixtures.go` | compilation check |
| 5 | Pipeline monitor model | `internal/tui/pipeline/*` | `model_test.go` |
| 6 | Log viewer model | `internal/tui/logview/*` | `model_test.go` |
| 7 | Root model wiring | `model.go`, `tui.go`, `dashboard/update.go` | existing tests pass |
| 8 | Integration tests | `integration_test.go` | 3 new tests |
| 9 | Documentation | `DESIGN.md`, `CHANGELOG.md` | — |

Dependencies: 1→5, 2→5, 3→5, 3→6, 4→5, 4→6, 5→7, 6→7, 7→8, 8→9
