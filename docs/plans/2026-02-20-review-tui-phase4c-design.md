# Phase 4c Design: Pipeline Monitor + Log Viewer

**Date:** 2026-02-20
**Phase:** 4c — Operational Monitoring
**Status:** Approved

## Overview

Phase 4c adds two views that complete the review TUI: a Pipeline Monitor for
daemon health and run history, and a Log Viewer for inspecting daemon logs.
These are operational monitoring views — no approval workflows, no AI chat.

## Data Layer

### PipelineRun Type

No `PipelineRun` data type exists yet. The daemon processes sessions and writes
outputs to `evaluations/`, but there's no structured way to query run history
with per-stage timing. Phase 4c creates this.

**Location:** `internal/pipeline/run.go` (alongside existing `proposal.go`)

```go
type PipelineRun struct {
    SessionID string
    Project   string
    Timestamp time.Time
    Status    string // "pending", "processed", "error"

    // Per-stage completion
    HasDigest bool
    HasClassifier  bool
    HasEvaluator bool

    // Per-stage timing (zero if stage not completed)
    ParseDuration  time.Duration
    ClassifierDuration  time.Duration
    EvaluatorDuration time.Duration

    // Results
    ProposalCount int
    ErrorDetail   string // populated for errored runs
}

type PipelineStats struct {
    SessionsCaptured  int
    SessionsProcessed int
    SessionsPending   int
    SessionsErrored   int
    ProposalsGenerated int
    ProposalsApproved  int
    ProposalsRejected  int
    ProposalsPending   int
    SessionsPerDay    []int // last N days, for sparkline
}

type PromptVersion struct {
    Name     string    // e.g., "classifier"
    Version  string    // e.g., "v3"
    LastUsed time.Time
}
```

**Reconstruction strategy:** Build `PipelineRun` from existing store artifacts:

- Session metadata provides SessionID, Project, Timestamp, Status
- File existence in `digests/` and `evaluations/` determines stage completion
- File modification timestamps provide per-stage timing (modtime of each output
  minus modtime of previous stage's output)
- Evaluator output JSON provides proposal count

**Functions:**

- `ListPipelineRuns(limit int) ([]PipelineRun, error)` — recent runs, sorted
  newest first
- `GatherPipelineStats(days int) (PipelineStats, error)` — aggregated counts
  within a time window
- `ListPromptVersions() ([]PromptVersion, error)` — from `~/.cabrero/prompts/`

### Auto-Refresh

Polling-based via `tea.Tick`:

- Pipeline monitor: 5-second tick when view is active, triggers data reload
- Log viewer follow mode: 1-second tick, re-reads file tail and auto-scrolls

No fsnotify dependency. Manual refresh also available via `r` key.

## Pipeline Monitor View

### Layout (design doc section 6)

```
┌─ Pipeline Monitor ────────────────────────────────────────────────────────┐
│                                                                           │
│  DAEMON                                              HOOKS                │
│  ──────                                              ─────                │
│  Status:  ● running (PID 4821)                       pre-compact:  ✓      │
│  Uptime:  3d 14h 22m                                 session-end:  ✓      │
│  Poll:    every 2m (next in 48s)                                          │
│  Stale:   every 30m (next in 12m)                    STORE                │
│  Delay:   30s between sessions                       ─────                │
│                                                      Path: ~/.cabrero/    │
│                                                      Raw:  142 sessions   │
│                                                      Disk: 284 MB        │
│                                                                           │
│  PIPELINE ACTIVITY (last 7 days)                                          │
│  ────────────────────────────────                                         │
│  Sessions captured:  18          Proposals generated:  5                  │
│  Sessions processed: 16          Proposals approved:   3                  │
│  Sessions pending:    1          Proposals rejected:   1                  │
│  Sessions errored:    1          Proposals pending:    1                  │
│                                                                           │
│  ▁▂▃▅▂▁▃▄▂▁▅▇▃▂   sessions/day                                          │
│                                                                           │
│  RECENT RUNS                                                              │
│  ───────────                                                              │
│  > ✓ e7f2a103  12m ago   woo-payments    1.2s parse  8.4s cls  12s eval  │
│    ✓ 3bc891ff  2h ago    cabrero         0.8s parse  6.1s cls  9s eval   │
│    ✗ 91cd02ab  8h ago    woo-payments    0.4s parse  ✗ cls failed        │
│    ○ 7e0b1234  1d ago    woo-payments    (pending)                       │
│                                                                           │
│  PROMPTS                                                                  │
│  ───────                                                                  │
│  classifier          v3   last used: 12m ago                              │
│  evaluator           v3   last used: 12m ago                              │
│  apply               v1   last used: 3d ago                               │
│                                                                           │
├───────────────────────────────────────────────────────────────────────────┤
│  ↑↓ navigate  enter run details  R retry errored  L view log  ? help      │
└───────────────────────────────────────────────────────────────────────────┘
```

### Sections

**Header (daemon + hooks + store):** Two-column layout in wide mode (>=120),
stacked in standard mode. Daemon status uses green/red indicator. Hook status
shows checkmark/cross. Store shows path, session count, disk usage.

**Activity summary:** 7-day window (configurable via `pipeline.sparklineDays`).
Session and proposal counts in two columns. Sparkline below using Unicode block
characters `▁▂▃▄▅▆▇█`.

**Recent runs:** Scrollable list (limit from `pipeline.recentRunsLimit`,
default 20). Each run shows: status indicator (`✓`/`✗`/`○`), short session ID
(8 chars), relative time, project display name, per-stage timing. Failed stages
show `✗ cls failed` or `✗ eval failed`.

**Prompts:** Static list of prompt files with version parsed from filename
and last-used time from most recent evaluation that references it.

### Keys

| Key     | Action                          |
|---------|---------------------------------|
| `↑`/`↓` | Navigate recent runs list      |
| `Enter` | Expand run details inline      |
| `R`     | Retry errored run (confirm)    |
| `L`     | Open log viewer                |
| `r`     | Manual refresh                 |
| `Esc`   | Back to dashboard              |
| `?`     | Help overlay                   |

### Run Detail (inline expansion)

Pressing `Enter` on a run expands it inline below the run entry (not a
separate view). Shows: full session ID, full pipeline output, proposals
generated, error details for failed runs. Press `Enter` again to collapse.

### Retry Flow

`R` on an errored run:
1. Confirm: "Retry session e7f2a103? [y/N]"
2. On confirm: emit `RetryRunStarted`, show spinner on that run entry
3. Background: invoke `cabrero run <session_id>` as subprocess
4. On completion: emit `RetryRunFinished`, refresh run list

## Log Viewer

### Layout

Full-screen viewport showing `~/.cabrero/daemon.log`. The log file is capped
at 5MB with rotation (3 files), so reading the whole file into memory is safe.

```
┌─ Log Viewer ── follow ● ─────────────────────────────────────────────────┐
│                                                                           │
│  2026-02-20T10:15:03Z INFO  daemon started (PID 4821)                    │
│  2026-02-20T10:15:03Z INFO  poll=2m0s stale=30m0s delay=30s              │
│  2026-02-20T10:15:03Z INFO  processing session e7f2a103                  │
│  2026-02-20T10:15:04Z INFO  pre-parser: 142 entries, 0.8s               │
│  2026-02-20T10:15:12Z INFO  classifier: classified, triage=evaluate      │
│  2026-02-20T10:15:24Z INFO  evaluator: 1 proposal generated             │
│  2026-02-20T10:17:05Z INFO  poll: 0 pending sessions                    │
│  ...                                                                     │
│                                                                           │
├───────────────────────────────────────────────────────────────────────────┤
│  / search  n/N next/prev  f follow  esc back                             │
└───────────────────────────────────────────────────────────────────────────┘
```

### Follow Mode

- Default on (configurable via `pipeline.logFollowMode`)
- 1-second tick re-reads file, appends new lines, auto-scrolls to bottom
- Header shows `follow ●` when active (green dot), `follow ○` when paused
- Any manual scroll up pauses follow mode automatically
- `f` toggles follow on/off

### Search

- `/` activates search bar (textinput at bottom, replaces status bar)
- Matches highlighted with accent color
- `n` goes to next match, `N` to previous
- `Esc` from search returns to normal mode (matches remain highlighted)
- Second `Esc` clears highlights and returns to pipeline monitor

### Keys

| Key     | Action                       |
|---------|------------------------------|
| `/`     | Open search bar              |
| `n`     | Next match                   |
| `N`     | Previous match               |
| `f`     | Toggle follow mode           |
| `Esc`   | Close search / back          |
| `?`     | Help overlay                 |

## Package Structure

```
internal/tui/
├── pipeline/
│   ├── model.go            # Model, New(), SetSize(), data loading
│   ├── update.go           # Update(), handleKey(), retry flow
│   ├── view.go             # View(), renderHeader(), renderRuns()
│   ├── model_test.go
│   └── testmain_test.go
├── logview/
│   ├── model.go            # Model, New(), SetSize()
│   ├── update.go           # Update(), handleKey(), search, follow
│   ├── view.go             # View()
│   ├── model_test.go
│   └── testmain_test.go
└── components/
    └── sparkline.go        # RenderSparkline() — Unicode block chars
```

## Messages

```go
// New view states
ViewPipelineMonitor ViewState
ViewLogViewer       ViewState

// Pipeline monitor messages
RetryRunStarted  { SessionID string }
RetryRunFinished { SessionID string; Err error }
PipelineTickMsg  {}   // 5s auto-refresh
LogTickMsg       {}   // 1s follow-mode refresh
```

## Key Bindings

```go
// Add to KeyMap
Retry        key.Binding  // "R"
LogView      key.Binding  // "L"
Refresh      key.Binding  // "r"
Search       key.Binding  // "/"
SearchNext   key.Binding  // "n"
SearchPrev   key.Binding  // "N"
FollowToggle key.Binding  // "f"
```

Add `PipelineShortHelp()` and `LogViewShortHelp()` to KeyMap.

## Wiring into Root Model

1. Add `pipeline pipeline_tui.Model` and `logview logview.Model` to
   `reviewModel` struct
2. Add `ViewPipelineMonitor` and `ViewLogViewer` cases to `Update()` routing
3. Add corresponding cases to `View()`
4. Handle `'p'` key in dashboard to push `ViewPipelineMonitor`
5. Handle `PipelineTickMsg` and `LogTickMsg` for auto-refresh
6. Handle `RetryRunStarted`/`RetryRunFinished` in root Update

## Test Fixtures

```go
// Add to testdata/fixtures.go
TestPipelineRun(overrides...)    PipelineRun
TestPipelineRuns()               []PipelineRun   // 4 runs: success, success, error, pending
TestPipelineStats()              PipelineStats
TestPromptVersions()             []PromptVersion  // 3 prompts
```

## Integration Tests

Add to `integration_test.go`:

- Dashboard `p` → Pipeline Monitor → `Esc` back
- Pipeline Monitor `L` → Log Viewer → `Esc` back
- Full stack: Dashboard → Pipeline → Log → back → back
- Retry flow: `R` on errored run with confirmation

## Responsive Layout

| Element              | Wide (>=120)   | Standard (80-119) | Narrow (<80)   |
|----------------------|----------------|--------------------|----------------|
| Daemon/Hooks/Store   | Side by side   | Stacked            | Abbreviated    |
| Activity columns     | 2 columns      | 2 columns          | Stacked        |
| Sparkline            | Full width     | Full width         | Hidden         |
| Session ID           | 8 chars        | 8 chars            | 6 chars        |
| Project name         | 20 chars       | 15 chars           | 10 chars       |
| Per-stage timing     | All 3 stages   | 2 stages           | Total only     |
| Prompt versions      | Shown          | Shown              | Hidden         |

## Known Simplifications

The initial implementation intentionally defers the following items from the
design. These are not bugs — they are scope cuts for the first pass.

### Key binding overlaps

`r` maps to both Reject (dashboard/detail) and Refresh (pipeline monitor).
`/` maps to both Filter (dashboard) and Search (log viewer). Both are safe
because each view only checks its own bindings in `handleKey`, but any future
shared key handler or binding iterator would need to account for the overlap.

### Responsive layout

The design (table above) specifies different behavior for wide (>=120),
standard (80-119), and narrow (<80) terminals. The daemon/hooks/store header
now uses two-column layout at width >= 120, but the remaining responsive
behaviors (abbreviated narrow mode, hidden sparklines, truncated IDs) are
not yet implemented.
