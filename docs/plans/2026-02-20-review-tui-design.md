# Cabrero Review TUI — Design Document

**Date:** 2026-02-20
**Phase:** 4a/4b/4c — Review TUI
**Status:** Approved design, pending implementation

### Implementation Phasing

The TUI ships incrementally to validate the core value proposition early:

- **Phase 4a** — Dashboard + Proposal Detail + AI Chat. The core review loop:
  see proposals, inspect evidence, interrogate via chat, approve/reject/defer.
  Ships first and validates whether TUI-based review improves decision quality
  over bare `cabrero approve` commands.
- **Phase 4b** — Fitness Report Detail + Source Manager. Assessment and
  management views for EVALUATE-mode output and source classification.
- **Phase 4c** — Pipeline Monitor + Log Viewer. Operational monitoring views
  for daemon health, run history, and log inspection.

The package structure (Section 14) supports this: each view is an independent
package with no cross-view dependencies. The root model, message types, config,
and shared components are built in 4a and extended in later phases.

## Overview

`cabrero review` is the primary interactive interface for Cabrero. It is a
Bubble Tea TUI that provides the complete operational dashboard: proposal
review with AI chat, fitness report assessment, source management, and
pipeline monitoring. Built with the Charm library ecosystem (Bubble Tea,
Bubbles, Lip Gloss, Huh, Glamour).

The TUI is self-sufficient. A native macOS SwiftUI app may come later as a
stretch goal, but the TUI handles everything. The individual CLI commands
(`cabrero approve`, `cabrero reject`) become non-interactive shortcuts for
scripting — the TUI is the primary review experience.

### Design Principles

- **Clarity over density.** Show what matters, hide what doesn't. Progressive
  disclosure through expandable sections, not walls of text.
- **Fast, predictable navigation.** Stack-based views. Enter pushes, Esc pops.
  Arrow-key navigation by default, vim-style as a config toggle.
- **Low cognitive load.** Actions and status are always visible. The status bar
  shows context-sensitive shortcuts. Help is one keypress away.
- **Generous context.** Full citation chains are always accessible. AI chat
  is always available to interrogate proposals before deciding.
- **Pirategoat spirit.** Mild personality in loading messages, empty states,
  and confirmation text. Never obtrusive, always warm.

### Scope

The TUI handles all item types across the full pipeline:

- **Proposals** (4 types): `skill_improvement`, `claude_review`,
  `claude_addition`, `skill_scaffold`
- **Fitness reports**: EVALUATE mode assessments of third-party artifacts
- **Source management**: Ownership classification, iterate/evaluate toggles,
  rollback history
- **Pipeline monitoring**: Daemon health, recent runs, prompt versions, log viewer

---

## 1. Architecture

### Entry Point

```
cabrero review    Launch the interactive TUI
```

Single command, full experience. No flags needed for normal use. The TUI reads
from and writes to `~/.cabrero/` — the same store the CLI and daemon use.

### View Hierarchy

```
reviewModel (root)
  ├── Dashboard (home screen)
  │   ├── Proposal List (filterable, sortable)
  │   ├── Pipeline Status (sidebar/header)
  │   └── Quick Stats
  ├── Proposal Detail
  │   ├── Diff View (colored, scrollable)
  │   ├── Citation Chain (expandable)
  │   ├── AI Chat Panel (split pane, streaming)
  │   └── Action Bar (approve/reject/defer)
  ├── Fitness Report Detail
  │   ├── Assessment summary with visual bars
  │   ├── Session evidence (grouped, expandable)
  │   └── Action recommendations
  ├── Source Manager
  │   ├── Source list with ownership/approach
  │   ├── Toggle iterate/evaluate
  │   └── New source classification
  └── Pipeline Monitor
      ├── Daemon status
      ├── Recent pipeline runs with timing
      ├── Prompt versions
      └── Log Viewer (full-screen scrollable)
```

### Navigation Model

Stack-based. `Enter` pushes detail views onto the stack. `Esc` pops back.
The dashboard is always the root — you can never pop past it (Esc on the
dashboard does nothing; `q` quits).

The root model holds a `viewStack []viewState` and a `state viewState` enum.
Each child view is an embedded struct field, initialized when pushed and
preserved when another view is pushed on top (so returning to a view keeps
scroll position and selection).

### Bubble Tea Composition Pattern

Thin root model delegates to embedded child models:

```go
// Root model
type reviewModel struct {
    state     viewState
    viewStack []viewState
    config    *Config
    // ... embedded child models
    dashboard       dashboardModel
    proposalDetail  proposalDetailModel
    fitnessDetail   fitnessDetailModel
    sourceManager   sourceManagerModel
    pipelineMonitor pipelineMonitorModel
    logViewer       logViewerModel
    // ... shared state
    help     help.Model
    helpOpen bool
    width    int
    height   int
}
```

Root `Update()` routes messages: global keys are handled first (quit, help,
Esc for back-navigation), then the active child's Update is called. Root
`View()` switches on `m.state` to render the active child plus the status bar.

### Message Architecture

All messages live in `internal/tui/message/message.go`. Messages are grouped
by domain:

**Navigation:** `PushView`, `PopView`

**Data loading:** `ProposalsLoaded`, `FitnessReportsLoaded`, `SessionsLoaded`,
`SourcesLoaded`, `PipelineRunsLoaded`, `StatsLoaded`

**Store changes:** `StoreChanged` (from file watcher — proposals, evaluations,
or sessions directory changed)

**Review actions:** `ApproveStarted`, `BlendFinished`, `ApplyConfirmed`,
`ApplyFinished`, `RejectFinished`, `DeferFinished`

**Pipeline actions:** `RetryRunStarted`, `RetryRunFinished`

**AI chat:** `ChatStreamToken`, `ChatStreamDone`, `ChatStreamError`,
`ChatRevisionParsed`

**Config:** `ConfigReloaded`

**Status bar:** `StatusMessage`, `StatusMessageExpired`

---

## 2. Dashboard

The home screen. Answers: "What needs my attention?"

### Layout

```
┌─ Cabrero Review ──────────────────────────────────────────────────────────┐
│  3 proposals awaiting review     │  Daemon: running (PID 4821)           │
│  1 fitness report                │  Last capture: 12m ago                │
│  7 approved  · 2 rejected        │  Hooks: pre-compact ✓  session-end ✓  │
├───────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  PENDING REVIEW                                                           │
│                                                                           │
│  > ● skill_improvement  docx-helper          2h ago   retry anomaly (3x) │
│    ● skill_scaffold     git-workflow          5h ago   correction pattern │
│    ● claude_review      CLAUDE.md (woo...)    1d ago   friction (5 sess.) │
│    ◎ fitness_report     some-third-party      3d ago   low fitness        │
│                                                                           │
│  RECENTLY DECIDED                                                         │
│                                                                           │
│    ✓ skill_improvement  debugging             3d ago   approved           │
│    ✗ claude_addition    CLAUDE.md (cabr...)   4d ago   rejected           │
│                                                                           │
├───────────────────────────────────────────────────────────────────────────┤
│  ↑↓ navigate  enter open  a approve  r reject  d defer  s sources  ? help│
└───────────────────────────────────────────────────────────────────────────┘
```

### Design Decisions

- **Two sections:** "Pending Review" (actionable) and "Recently Decided"
  (context). Pending always above.
- **Item type indicators:** `●` for proposals, `◎` for fitness reports.
- **Signal summary inline:** Each item shows why it was flagged (retry anomaly,
  correction pattern, friction) for triage without opening.
- **Quick actions from list:** `a` to approve (with confirmation), `r` to
  reject (prompts for reason), `d` to defer. Work on the selected item
  without entering the detail view.
- **Status header:** Pipeline health at a glance. Muted colors, not competing
  with the item list.
- **Filtering:** `/` opens a filter bar. Filter by type (`type:skill`),
  target (`target:docx`), confidence (`conf:high`), or free text.
- **Sorting:** `o` cycles sort order: newest first (default), oldest first,
  by confidence, by type. Persisted to config.

---

## 3. Proposal Detail View

Entered by pressing `Enter` on a proposal. This is where review happens.

### Wide Mode (>=120 cols) — Split Pane

```
┌─ Proposal: skill_improvement ─────────────────────┬─ AI Chat ────────────────────┐
│                                                    │                              │
│  Target: ~/.claude/skills/docx-helper/SKILL.md     │  Ask me about this proposal  │
│  Confidence: high  │  Session: abc123  │  2h ago   │                              │
│                                                    │  ┌─────────────────────────┐  │
│  PROPOSED CHANGE                                   │  │ 1  Why was this flagged?│  │
│  ─────────────────                                 │  │ 2  Show the raw turns   │  │
│  @@ -12,3 +12,5 @@                                │  │ 3  Conservative version │  │
│    ## Workflow                                      │  │ 4  Risk of approving?   │  │
│  - Read the template before writing                │  └─────────────────────────┘  │
│  + Read SKILL.md before any write tool call        │                              │
│  + Verify template structure matches expected       │  ┌──────────────────────────┐│
│  + format before generating content                 │  │ Type a question...       ││
│                                                    │  └──────────────────────────┘│
│  RATIONALE                                         │                              │
│  ─────────────────                                 │                              │
│  Skill was read after 3 write attempts in session  │                              │
│  abc123. The writes failed because the template    │                              │
│  structure was not understood before generation.    │                              │
│  Pattern observed in 2 sessions over 7 days.       │                              │
│                                                    │                              │
│  EVALUATOR REASONING                               │                              │
│  ─────────────────                                 │                              │
│  "Skill loaded at turn 18, first write at turn 9.  │                              │
│  Three consecutive write failures before skill      │                              │
│  read. Post-read write succeeded on first try."    │                              │
│                                                    │                              │
│  HAIKU CLASSIFICATION                              │                              │
│  ─────────────────                                 │                              │
│  Goal: "Create a formatted Word report"            │                              │
│  Signal: skill read at turn 18, first write at 9   │                              │
│  Confidence: high                                  │                              │
│                                                    │                              │
│  CITATION CHAIN (5 entries)                        │                              │
│  ─────────────────                                 │                              │
│  > [1] Turn 9:  tool_use write -> report.docx     │                              │
│    [2] Turn 12: tool_use write -> report.docx     │                              │
│    [3] Turn 15: tool_use write -> report.docx     │                              │
│    [4] Turn 18: tool_use view -> SKILL.md          │                              │
│    [5] Turn 19: tool_use write -> report.docx OK  │                              │
│                                                    │                              │
├────────────────────────────────────────────────────┴──────────────────────────────┤
│  esc back  a approve  r reject  d defer  tab focus chat  1-4 chip  ? help         │
└───────────────────────────────────────────────────────────────────────────────────┘
```

### Narrow Mode (<120 cols) — Stacked

Single-column layout. Same content, full width. Chat opens as a full-screen
overlay when pressing `c`. `Esc` from chat returns to the proposal.

### Diff Rendering

- Red/green colored unified diff with `@@` hunk headers
- Line numbers in muted color
- `claude_review` proposals (flagged entries, no diff): flagged entry rendered
  in a highlighted box
- `skill_scaffold` proposals: all-green additions showing the proposed new
  skill content

### Citation Chain

- Each entry is a collapsed one-liner: turn number, tool name, target
- `Enter` on a citation expands it inline showing the full raw JSONL entry
  formatted for readability (type, timestamp, role, content preview)
- Citations are scrollable within the left pane's viewport

### AI Chat Panel

- Starts with 3-4 question chips generated by Haiku (from evaluation output)
- Number keys `1`-`4` send a chip immediately
- `Tab` toggles focus between proposal pane and chat pane
- When chat has focus: text input active, `Enter` sends, `Esc` returns focus
- Streaming responses render incrementally via `claude` CLI stdout
- Markdown rendered via Glamour (syntax-highlighted code blocks, formatting)

### Revised Proposals

The chat model detects revised proposals using a dedicated fence marker:
` ```revision `. Ordinary diff blocks (` ```diff `, ` ```skill `) are rendered
as syntax-highlighted code but are **not** treated as actionable revisions.
The chat system prompt instructs the model: "When producing a revised proposal,
use a ` ```revision ` fenced block. Use ` ```diff ` for illustrative diffs."

Detection rules:

- Only ` ```revision ` blocks are parsed as revised proposals
- Only the **last** ` ```revision ` block in a response is used (earlier ones
  are superseded)
- Parsing fires only after `ChatStreamDone` — partial blocks during streaming
  are never parsed as revisions
- Malformed blocks (invalid diff syntax) are rendered as code but not offered
  as revisions; a muted warning appears: "Could not parse revision"

When a valid revision is detected:

- Renders inline with red/green coloring
- `[u] Use this revision` prompt appears
- Pressing `u` stores the revision; the action bar updates to show
  "revision available"
- Approve now offers the choice: `[o]riginal / [r]evision / [c]ancel`

### Approve Flow

```
Press 'a'
  -> "Apply this change? [y/N]" (or original/revision choice if revision exists)

Press 'y'
  -> Spinner: "Blending change into SKILL.md..."
  -> claude CLI invoked with writing skill, current file, proposed change

Blend finishes
  -> Before/after diff shown: "Commit this change? [y/N]"

Press 'y'
  -> File written, proposal archived, rollback entry stored
  -> Return to dashboard with status message

Press 'n' or Esc at any step
  -> Reset to idle, no side effects
```

---

## 4. Fitness Report Detail View

For EVALUATE mode output — assessments of third-party artifacts. No diff to
approve, just an assessment to act on.

### Layout

```
┌─ Fitness Report: some-third-party/docx-helper ────────────────────────────┐
│                                                                            │
│  Source: plugin: some-third-party    Ownership: not mine    Mode: Evaluate │
│  Observed in: 14 sessions (past 30 days)                                   │
│                                                                            │
│  ASSESSMENT                                                                │
│  ───────────                                                               │
│                                                                            │
│  Followed correctly ..... 5 sessions  ████████░░░░░░░░░░░░░░░░  36%       │
│  Worked around .......... 6 sessions  ████████████░░░░░░░░░░░░  43%       │
│  Caused confusion ....... 3 sessions  ██████░░░░░░░░░░░░░░░░░░  21%       │
│                                                                            │
│  VERDICT                                                                   │
│  ───────                                                                   │
│  Low fitness for your workflow. The skill is followed less than half the    │
│  time and actively caused confusion in 3 sessions. Consider replacing      │
│  with a user-level skill or supplementing with project-level overrides.    │
│                                                                            │
│  SESSION EVIDENCE                                                          │
│  ────────────────                                                          │
│  Worked around:                                                            │
│  > Session e7f2a1 (2d ago)  "Skill read, then manually overridden at t15" │
│    Session 3bc891 (5d ago)  "Instructions ignored; grep-based approach"    │
│    + 4 more                                                                │
│                                                                            │
│  Caused confusion:                                                         │
│  > Session 91cd02 (3d ago)  "3 retries following skill before switching"   │
│    Session f8a234 (12d ago) "Error loop attributed to outdated guidance"   │
│    Session 7e0b12 (18d ago) "Sub-agent spawned to work around skill"      │
│                                                                            │
├────────────────────────────────────────────────────────────────────────────┤
│  esc back  x dismiss  s go to sources  c chat  ? help                      │
└────────────────────────────────────────────────────────────────────────────┘
```

### Design Decisions

- **Visual fitness bars:** Horizontal bar per category with percentage. Instant
  visual signal of health.
- **Three-bucket breakdown:** Followed / Worked around / Confusion — the three
  categories from the evaluator.
- **Session evidence grouped by category:** Collapsed one-liners. `Enter`
  expands to show evaluator notes and cited turns. `+ N more` when a category
  has many sessions.
- **No approve/reject:** Fitness reports use different actions:
  - `x` — Dismiss (acknowledge, archive)
  - `s` — Jump to Source Manager with this source pre-selected
  - `c` — Open AI chat ("What would a replacement skill look like?",
    "Which sessions were worst?", etc.)

---

## 5. Source Manager

Accessible from the dashboard via `s` or from a fitness report. Manages which
artifacts Cabrero tracks and how.

### Layout

```
┌─ Source Manager ──────────────────────────────────────────────────────────┐
│                                                                           │
│  Sources discovered: 12    Iterate: 7    Evaluate: 3    Unclassified: 2   │
│                                                                           │
│  SOURCE                          OWNERSHIP   APPROACH     SESSIONS  HEALTH│
│  ─────────────────────────────────────────────────────────────────────── │
│                                                                           │
│  User-level                                                               │
│  > brainstorming                 mine        ● Iterate       42    ████   │
│    debugging                     mine        ● Iterate       38    ████   │
│    writing-plans                 mine        ● Iterate       31    ███    │
│                                                                           │
│  Project: woocommerce-payments                                            │
│    wordpress-backend-dev         mine        ● Iterate       15    ███    │
│    php-testing-patterns          mine        ● Iterate       12    ████   │
│                                                                           │
│  Plugin: some-third-party                                                 │
│    docx-helper                   not mine    ◎ Evaluate      14    ▓░     │
│    csv-parser                    not mine    ◎ Evaluate       6    ▓▓░    │
│                                                                           │
│  ⚠ Unclassified                                                          │
│    Plugin: new-plugin-x          ???         ─ Paused         2    ───    │
│    Plugin: another-new           ???         ─ Paused         0    ───    │
│                                                                           │
├───────────────────────────────────────────────────────────────────────────┤
│  ↑↓ navigate  enter details  t toggle mode  o set ownership  ? help       │
└───────────────────────────────────────────────────────────────────────────┘
```

### Design Decisions

- **Grouping by origin:** User-level, Project (by name), Plugin (by name),
  Unclassified at bottom with `⚠`.
- **Collapsible groups:** `←`/`→` collapses/expands. Collapsed shows summary.
- **Columns:** Source name, ownership (`mine`/`not mine`), approach
  (`● Iterate`/`◎ Evaluate`/`─ Paused`), session count, health bar.
- **Health bar:** For Iterate sources: ratio of proposals approved vs generated.
  For Evaluate: fitness percentage. `───` for unclassified.

### Actions

- `t` — Toggle approach (Iterate/Evaluate) with confirmation
- `o` — Set ownership (`[m]ine / [n]ot mine`)
- `Enter` on unclassified source — Classification flow via huh form
  (ownership + approach selection)
- `Enter` on classified source — Detail panel: recent proposals/reports,
  change history, per-source timeline

### Rollback

Within a source's detail panel, recent approved changes are listed with
`[z] rollback` on each. Rollback shows the before/after diff and confirms
before reverting.

```
  Recent changes for: brainstorming
  ──────────────────────────────────
  ✓ 2025-02-18  "Added pre-write checklist step"        [z] rollback
  ✓ 2025-02-15  "Removed redundant re-read pattern"     [z] rollback
  ✗ 2025-02-14  "Aggressive rewrite of intro"           (rejected)
```

---

## 6. Pipeline Monitor

Accessible from the dashboard via `p`. Operational health view.

### Layout

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
│  > ✓ e7f2a103  12m ago   woo-payments    1.2s parse  8.4s haiku  12s son │
│    ✓ 3bc891ff  2h ago    cabrero         0.8s parse  6.1s haiku  9s son  │
│    ✗ 91cd02ab  8h ago    woo-payments    0.4s parse  ✗ haiku failed      │
│    ○ 7e0b1234  1d ago    woo-payments    (pending)                       │
│                                                                           │
│  PROMPTS                                                                  │
│  ───────                                                                  │
│  haiku-classifier    v2   last used: 12m ago                              │
│  sonnet-evaluator    v2   last used: 12m ago                              │
│  sonnet-apply        v1   last used: 3d ago                               │
│                                                                           │
├───────────────────────────────────────────────────────────────────────────┤
│  ↑↓ navigate  enter run details  R retry errored  L view log  ? help      │
└───────────────────────────────────────────────────────────────────────────┘
```

### Design Decisions

- **System health at a glance:** Daemon status (green/red indicator), hook
  status, store stats. Practical for knowing when cleanup might be needed.
- **7-day activity summary:** Session/proposal counts with a sparkline bar
  (Unicode block characters `▁▂▃▄▅▆▇█`). Configurable window via
  `pipeline.sparklineDays`.
- **Recent runs with timing:** Per-stage timing breakdown. Failed stages show
  `✗ stage failed`. Status: `✓` success, `✗` error, `○` pending,
  `◐` in-progress (spinner).
- **Live updates:** If the daemon processes a session while you watch, the
  list updates in real-time via file watcher.

### Actions

- `Enter` on a run — Detail panel: full pipeline output, proposals generated,
  error details
- `R` on an errored run — Retry (equivalent to `cabrero run <id>`) with
  confirmation and live spinner
- `L` — Open log viewer

### Log Viewer

Full-screen scrollable viewport showing `~/.cabrero/daemon.log`.

- Tail/follow mode by default (new lines appear live)
- `/` to search, `n`/`N` for matches
- `f` to toggle follow mode
- `Esc` returns to pipeline monitor

---

## 7. Keybindings

### Philosophy

Arrow-first by default — accessible out of the box. Vim-style available via
config toggle. Action keys (`a`, `r`, `d`, `c`, etc.) are the same in both
modes.

### Navigation Scheme

Controlled by `config.navigation`:

| Action             | `"arrows"` (default) | `"vim"`   |
|--------------------|----------------------|-----------|
| Move down          | `↓`                  | `j`       |
| Move up            | `↑`                  | `k`       |
| Move left/collapse | `←`                  | `h`       |
| Move right/expand  | `→`                  | `l`       |
| Half-page down     | `Page Down`          | `Ctrl+d`  |
| Half-page up       | `Page Up`            | `Ctrl+u`  |
| Jump to top        | `Home`               | `g` `g`   |
| Jump to bottom     | `End`                | `G`       |

In vim mode, arrow keys still work as fallback — they are never disabled.

### Global Keys (both modes)

| Key        | Action                                  |
|------------|-----------------------------------------|
| `Ctrl+C`   | Quit immediately                        |
| `q`        | Quit (confirm if pending actions)       |
| `?`        | Toggle help overlay                     |
| `Esc`      | Back / close overlay / cancel           |
| `Tab`      | Cycle focus forward between panes       |
| `Shift+Tab`| Cycle focus backward                    |

### Dashboard Keys

| Key     | Action                          |
|---------|---------------------------------|
| `Enter` | Open selected item              |
| `a`     | Quick approve (with confirm)    |
| `r`     | Quick reject (prompts reason)   |
| `d`     | Defer selected item             |
| `/`     | Open filter bar                 |
| `o`     | Cycle sort order                |
| `s`     | Open Source Manager             |
| `p`     | Open Pipeline Monitor           |

### Proposal Detail Keys

| Key     | Action                                  |
|---------|-----------------------------------------|
| `a`     | Approve (starts apply flow)             |
| `r`     | Reject (prompts for reason)             |
| `d`     | Defer                                   |
| `c`     | Focus chat / open chat overlay (narrow) |
| `Tab`   | Toggle focus: proposal <-> chat         |
| `Enter` | Expand/collapse citation                |
| `1`-`4` | Send question chip                      |
| `u`     | Use chat-revised proposal               |

### Chat Pane Keys (when focused)

| Key     | Action                        |
|---------|-------------------------------|
| `Enter` | Send message                  |
| `Esc`   | Return focus to proposal pane |
| `↑`     | Previous prompt from history  |

### Fitness Report Keys

| Key     | Action                              |
|---------|-------------------------------------|
| `x`     | Dismiss/acknowledge report          |
| `s`     | Jump to Source Manager               |
| `c`     | Open chat                           |
| `Enter` | Expand/collapse evidence            |

### Source Manager Keys

| Key        | Action                                  |
|------------|-----------------------------------------|
| `t`        | Toggle iterate/evaluate                 |
| `o`        | Set ownership                           |
| `Enter`    | Open detail / classify unclassified     |
| `z`        | Rollback (in detail view)               |
| `←`/`→`   | Collapse/expand group                   |

### Pipeline Monitor Keys

| Key     | Action              |
|---------|---------------------|
| `Enter` | Open run detail     |
| `R`     | Retry errored run   |
| `L`     | Open log viewer     |

### Log Viewer Keys

| Key     | Action                       |
|---------|------------------------------|
| `/`     | Search                       |
| `n`/`N` | Next/previous match          |
| `f`     | Toggle follow/tail mode      |
| `Esc`   | Back to pipeline monitor     |

### Help Overlay

Pressing `?` shows a full-screen overlay listing all keybindings for the
current view, grouped by category. Renders correct keys for the active
navigation mode. `?` or `Esc` closes it.

### Status Bar

Bottom bar shows 4-6 context-sensitive shortcuts for the current view and
focus state. Adapts when focus changes. Full set always via `?`.

---

## 8. Color System

### Adaptive Palette

Works on both light and dark terminals via `lipgloss.AdaptiveColor`. The TUI
detects terminal background and picks the right variant.

### 60-30-10 Rule

| Layer       | Weight | Purpose                                    |
|-------------|--------|--------------------------------------------|
| **Base**    | 60%    | Background, default text, borders          |
| **Secondary**| 30%   | Headers, muted text, inactive items        |
| **Accent**  | 10%    | Focus indicator, active items, highlights  |

### Semantic Colors

| Semantic         | Usage                                             |
|------------------|---------------------------------------------------|
| **Success/Green**| Approved status, diff additions, healthy states   |
| **Error/Red**    | Rejected status, diff deletions, error states     |
| **Warning/Amber**| Deferred items, unclassified sources              |
| **Accent/Purple**| Selected item, active pane border, focus indicator|
| **Muted/Gray**   | Timestamps, secondary metadata, faint text        |
| **Chat/Teal**    | AI response text, question chips                  |

### Element Styling

```
Active pane border:     accent, double-line (╔══╗)
Inactive pane border:   muted, single-line (┌──┐)
Selected list item:     accent background, bold
Section headers:        bold, secondary, UPPERCASE
Diff additions:         green foreground
Diff deletions:         red foreground
Diff hunk headers:      cyan, faint
Spinner:                accent, cycling
Status bar:             inverted (light on secondary bg)
Fitness bar filled:     green (high) -> yellow -> red (low)
Fitness bar empty:      muted
```

### Focus Management

Active element is always unambiguous:

- Active pane: bright accent double-line border
- Inactive pane: muted single-line border
- Selected item: accent background highlight
- Cursor `>` appears only in the focused list

### Theme Override

`config.theme` accepts `"auto"` (default), `"dark"`, or `"light"` to
force a specific palette when auto-detection fails.

### Pirategoat Personality

Controlled by `config.personality.flavorText` and
`config.personality.easterEggs`. Mild, delightful, never obtrusive. All in
muted/secondary color.

**Loading messages** (random, shown next to spinners):

```
Herding the goats...
Tending the flock...
The goatherd ponders...
Gathering scattered insights...
Sharpening the horns...
Consulting the elders...
One goat at a time...
```

**Empty states:**

```
No proposals:   "The flock is calm. No proposals pending."
No errors:      "All goats accounted for. No errors."
All classified: "Every goat has a name."
```

**Status bar confirmations** (disappear after 3s):

```
After approve: "Change landed. The flock grows stronger."
After reject:  "Noted. The goatherd remembers."
After defer:   "Back of the line, little goat."
```

**Startup:** Single line, then dashboard.

```
🐐 cabrero v0.1.0
```

---

## 9. Configuration

Stateful JSON at `~/.cabrero/config.json`. Created with defaults on first
`cabrero review` launch if absent.

### Schema

```json
{
  "navigation": "arrows",
  "theme": "auto",

  "dashboard": {
    "sortOrder": "newest",
    "showRecentlyDecided": true,
    "recentlyDecidedLimit": 10
  },

  "detail": {
    "chatPanelOpen": true,
    "chatPanelWidth": 35,
    "expandCitationsDefault": false
  },

  "pipeline": {
    "sparklineDays": 7,
    "recentRunsLimit": 20,
    "logFollowMode": true
  },

  "sourceManager": {
    "groupCollapsedDefault": false
  },

  "personality": {
    "flavorText": true,
    "easterEggs": true
  },

  "confirmations": {
    "approveRequiresConfirm": true,
    "rejectRequiresConfirm": false,
    "deferRequiresConfirm": false,
    "retryRequiresConfirm": true,
    "rollbackRequiresConfirm": true
  }
}
```

### Field Reference

| Field                               | Type     | Default     | Purpose                                   |
|-------------------------------------|----------|-------------|-------------------------------------------|
| `navigation`                        | string   | `"arrows"`  | `"arrows"` or `"vim"`                     |
| `theme`                             | string   | `"auto"`    | `"auto"`, `"dark"`, or `"light"`          |
| `dashboard.sortOrder`               | string   | `"newest"`  | `"newest"`, `"oldest"`, `"confidence"`, `"type"` |
| `dashboard.showRecentlyDecided`     | bool     | `true`      | Show the Recently Decided section         |
| `dashboard.recentlyDecidedLimit`    | int      | `10`        | Max items in Recently Decided             |
| `detail.chatPanelOpen`              | bool     | `true`      | Chat panel starts open in wide mode       |
| `detail.chatPanelWidth`             | int      | `35`        | Chat panel width (percentage)             |
| `detail.expandCitationsDefault`     | bool     | `false`     | Citations start expanded                  |
| `pipeline.sparklineDays`            | int      | `7`         | Days in activity sparkline                |
| `pipeline.recentRunsLimit`          | int      | `20`        | Max runs shown                            |
| `pipeline.logFollowMode`           | bool     | `true`      | Log viewer starts in tail mode            |
| `sourceManager.groupCollapsedDefault`| bool    | `false`     | Source groups start collapsed             |
| `personality.flavorText`            | bool     | `true`      | Loading messages, empty states            |
| `personality.easterEggs`            | bool     | `true`      | Hidden goat and similar                   |
| `confirmations.approveRequiresConfirm` | bool | `true`      | Confirm before approve                    |
| `confirmations.rejectRequiresConfirm`  | bool | `false`     | Confirm before reject                     |
| `confirmations.deferRequiresConfirm`   | bool | `false`     | Confirm before defer                      |
| `confirmations.retryRequiresConfirm`   | bool | `true`      | Confirm before pipeline retry             |
| `confirmations.rollbackRequiresConfirm`| bool | `true`      | Confirm before rollback                   |

### Behavior

- **Read on startup**, held in memory.
- **Hot-reload:** File watcher detects external edits, sends `ConfigReloaded`
  message. No restart needed.
- **Persisted on state change:** Sort order, chat panel visibility, etc.
  written immediately when changed in-TUI.
- **Merge semantics:** Missing fields get defaults. Unknown fields are
  preserved (forward compatibility).
- **No in-TUI editor:** Edit the JSON directly. Help overlay shows the
  config path as a hint.

### What Gets Persisted vs. Session-Only

| Persisted to config           | Session-only (not persisted)    |
|-------------------------------|---------------------------------|
| Navigation mode               | Scroll position                 |
| Sort order                    | Filter bar contents             |
| Chat panel open/width         | Expanded citations              |
| Confirmation toggles          | Selected item                   |
| Theme preference              | Chat history                    |
| Personality toggles           | In-progress approval flows      |

---

## 10. AI Chat Integration

### Invocation

The chat spawns `claude` CLI as a subprocess:

```bash
CABRERO_SESSION=1 claude --model claude-sonnet-4-6 --print --output-format stream-json
```

Stdin receives the user question plus system context (the full citation chain
for the current proposal as structured JSON). Environment variable prevents
loop capture. Spawned session ID added to blocklist.

### Streaming

`--output-format stream-json` produces one JSON object per line as tokens
arrive. A background goroutine reads stdout line by line and sends
`ChatStreamToken` messages to the Bubble Tea program.

The viewport auto-scrolls to bottom while streaming. A blinking cursor `▊`
appears at the end, removed on `ChatStreamDone`.

Markdown in responses is rendered via Glamour (syntax-highlighted code blocks,
formatting, inline styling).

**Known dependency:** The `--output-format stream-json` flag and its JSON
schema are not covered by a stability contract from the `claude` CLI. A CC
update could change the output format. Mitigation: the stream parser validates
the JSON schema on the first message and falls back gracefully — if the format
is unrecognized, the chat panel shows "Chat unavailable: unsupported claude CLI
output format" and the rest of the TUI continues to work. Defensive parsing
ignores unknown JSON fields rather than failing on them.

### Revised Proposal Detection

The chat uses a dedicated ` ```revision ` fence marker to distinguish
actionable revised proposals from illustrative diffs. The chat system prompt
instructs the model to use ` ```revision ` for proposals and ` ```diff ` for
illustrations. Detection rules:

- Only ` ```revision ` blocks are parsed as revised proposals
- Only the **last** ` ```revision ` block per response is used
- Parsing fires only after `ChatStreamDone` — never during streaming
- Malformed blocks render as code with a muted warning, not as revisions

When a valid revision is detected:

- Diff block renders inline with red/green coloring
- `[u] Use this revision` prompt appears
- Pressing `u` stores revision; approve flow gains original/revision choice

### Question Chips

3-4 chips from Haiku evaluation output, rendered as bordered elements in
chat accent color. Keys `1`-`4` send immediately. Chips hide after the first
manually-typed message.

### Loop Prevention

- `CABRERO_SESSION=1` environment variable on all CLI invocations
- Spawned session IDs added to the blocklist
- Consistent with pipeline's own loop prevention layers

---

## 11. Responsive Layout

### Width Tiers

| Width      | Name         | Behavior                                           |
|------------|--------------|----------------------------------------------------|
| >=120 cols | **Wide**     | Split panes. Dashboard stats sidebar. Chat panel.  |
| 80-119     | **Standard** | Single-column. Chat as overlay. Stats in header.   |
| <80        | **Narrow**   | Compact rendering. Truncated columns. Abbreviated. |

### Height Handling

| Height   | Behavior                                           |
|----------|----------------------------------------------------|
| >=30 rows| Full layout: header, content, status bar            |
| 20-29    | Header collapses to single line. Status bar stays. |
| <20      | Warning message. Content renders but clipped.       |

### Terminal Resize

`tea.WindowSizeMsg` triggers full layout recalculation. All viewports, lists,
and panes resize. Scroll position preserved as percentage.

### Width Allocation (wide mode detail view)

```
Proposal pane: W * (100 - chatPanelWidth)% - 1
Chat pane:     W * chatPanelWidth%
```

Chat panel minimum: 30 columns. If terminal too narrow for both panes at the
configured ratio, falls back to standard mode.

### Adaptive Elements

| Element              | Wide           | Standard        | Narrow          |
|----------------------|----------------|-----------------|-----------------|
| Dashboard stats      | Right sidebar  | Inline header   | Abbreviated     |
| Proposal ID          | Full UUID      | First 12 chars  | First 8 chars   |
| Target path          | Full path      | Last 40 chars   | Last 25 chars   |
| Session evidence     | 3 per group    | 2 per group     | 1 per group     |
| Status bar shortcuts | 6 shown        | 4 shown         | 3 shown         |
| Chat panel           | Side pane      | Overlay         | Overlay         |
| Source columns       | All 5          | 4 (no health)   | 3 (name/mode/n) |

---

## 12. Error Handling

### Store Not Initialized

If `~/.cabrero/` doesn't exist or is empty, show a first-run screen:

```
Welcome to Cabrero Review

The store at ~/.cabrero/ is empty.
Run 'cabrero setup' to initialize, or 'cabrero import' to seed
from existing CC sessions.

Press q to exit.
```

### Daemon Not Running

Dashboard header shows warning-colored status: `Daemon: ● stopped`. No
blocking — TUI works without daemon. Pipeline monitor shows hint.

### No Proposals

Dashboard empty state with flavor text: `"The flock is calm. No proposals
pending."` and hint to check sessions or run pipeline.

### `claude` CLI Not Found

Detected on startup. Chat and approve features disabled with inline notice:
`"AI features unavailable: 'claude' CLI not found in PATH."` Inspect,
source management, and pipeline monitoring still work.

### CLI Invocation Failures

- **Chat error:** Renders inline styled error message. Input stays active
  for retry.
- **Apply error:** `applyState` resets to idle. Error in status bar.
  Proposal remains pending.
- **Retry error:** Error shown in pipeline monitor. Session stays errored.

### Concurrent Access

CLI commands and daemon share the store. Handled via file watcher:
`StoreChanged` messages trigger list reloads. If a proposal is deleted
externally while viewing: `"This proposal is no longer available."` with
Esc to go back.

### Long-Running Apply

Spinner with elapsed time: `"Blending change into SKILL.md... (8s)"`. After
60 seconds, cancel option appears: `"Taking longer than expected. [Esc] to
cancel"`. Cancel sends SIGTERM and resets state.

---

## 13. Bubble Tea Model Detail

### Root Model Fields

```go
type reviewModel struct {
    state           viewState
    viewStack       []viewState
    config          *Config
    statusMsg       string
    statusExpiry    time.Time

    dashboard       dashboardModel
    proposalDetail  proposalDetailModel
    fitnessDetail   fitnessDetailModel
    sourceManager   sourceManagerModel
    pipelineMonitor pipelineMonitorModel
    logViewer       logViewerModel

    help            help.Model
    helpOpen        bool
    width, height   int
}
```

### Dashboard Model

```go
type dashboardModel struct {
    list            list.Model      // pending items
    recentList      list.Model      // recently decided
    focus           dashboardFocus  // pending | recent
    filterBar       textinput.Model
    filterActive    bool
    stats           dashboardStats  // counts, daemon, hooks
}
```

### Proposal Detail Model

```go
type proposalDetailModel struct {
    proposal        *ProposalWithSession
    diffViewport    viewport.Model
    citations       []citationEntry     // with expanded bool
    citationViewport viewport.Model
    chat            chatModel
    focus           detailFocus         // proposal | chat
    applyState      applyState          // idle | confirming | blending | reviewing | done
    revision        *Proposal           // chat-produced alternative
    spinner         spinner.Model
}
```

### Chat Model (shared)

```go
type chatModel struct {
    messages        []chatMessage       // role + content
    viewport        viewport.Model
    input           textarea.Model
    chips           []string            // from Haiku output
    chipsUsed       bool
    streaming       bool
    streamBuf       strings.Builder
    spinner         spinner.Model
    promptHistory   []string
    promptHistoryIdx int
    contextPayload  string              // citation chain JSON
}
```

### Fitness Detail Model

```go
type fitnessDetailModel struct {
    report          *FitnessReport
    evidenceViewport viewport.Model
    evidence        []evidenceGroup     // with expanded state
    chat            chatModel
    focus           fitnessFocus        // report | chat
}
```

### Source Manager Model

```go
type sourceManagerModel struct {
    groups          []sourceGroup       // with collapsed bool
    flatIndex       int                 // cursor across visible items
    detailOpen      bool
    detailViewport  viewport.Model
    classifyForm    *huh.Form
}
```

### Pipeline Monitor Model

```go
type pipelineMonitorModel struct {
    runs            []pipelineRun
    runList         list.Model
    stats           pipelineStats
    sparkline       []int
}
```

### Log Viewer Model

```go
type logViewerModel struct {
    viewport        viewport.Model
    searchInput     textinput.Model
    searchActive    bool
    followMode      bool
    matches         []lineMatch
}
```

---

## 14. Package Structure

```
internal/tui/
├── tui.go                  Entry point: NewProgram(), Run()
├── model.go                reviewModel root, Init/Update/View
├── config.go               Config struct, Load/Save/Watch, defaults
├── keys.go                 Key bindings (arrows + vim), help text
├── styles.go               Lip Gloss styles, adaptive palette, theme
├── message/
│   └── message.go          All message types
├── dashboard/
│   ├── model.go            dashboardModel
│   ├── update.go           Update logic
│   └── view.go             View rendering
├── detail/
│   ├── model.go            proposalDetailModel
│   ├── update.go
│   ├── view.go
│   └── diff.go             Diff parsing and colored rendering
├── fitness/
│   ├── model.go            fitnessDetailModel
│   ├── update.go
│   └── view.go
├── chat/
│   ├── model.go            chatModel (shared)
│   ├── update.go
│   ├── view.go
│   └── stream.go           claude CLI subprocess, streaming
├── sources/
│   ├── model.go            sourceManagerModel
│   ├── update.go
│   └── view.go
├── pipeline/
│   ├── model.go            pipelineMonitorModel
│   ├── update.go
│   ├── view.go
│   └── sparkline.go        Sparkline rendering
├── logview/
│   ├── model.go            logViewerModel
│   ├── update.go
│   └── view.go
└── components/
    ├── confirm.go           Inline [y/N] confirmation
    ├── statusbar.go         Timed message display
    └── flavor.go            Pirategoat loading/empty/confirm text
```

Each view splits Update and View into separate files to keep each under
~300 lines. Model structs and constructors in `model.go`. Shared components
in `components/`.

---

## 15. Testing Strategy

### Architectural Requirement

All TUI models must accept data as constructor arguments, never read from the
filesystem directly. Store reads happen at the `cabrero review` entry point;
data flows down into models via constructors. This makes unit testing trivial
and is also better architecture — models are pure functions of their input.

```go
// CORRECT: data injected
func NewDashboardModel(proposals []ProposalWithSession, stats DashboardStats) dashboardModel

// WRONG: reads from disk
func NewDashboardModel() dashboardModel  // internally calls pipeline.ListProposals()
```

The store-reading layer lives in `tui.go` (the entry point), not in
individual model packages.

### Tier 1 — Pure Unit Tests (primary, sub-second feedback)

Test `Update()` and `View()` directly. Construct models with test data, send
key messages, assert on rendered output and model state. Use `ansi.Strip()`
for color-independent layout comparisons. Golden files for snapshot testing.

```go
func TestDashboardNavigation(t *testing.T) {
    m := NewDashboardModel(testProposals, testStats)
    m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

    // Navigate down twice
    m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
    m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

    // Verify selection
    if m.SelectedIndex() != 2 {
        t.Errorf("expected index 2, got %d", m.SelectedIndex())
    }

    // Verify rendered output
    got := ansi.Strip(m.View())
    golden.RequireEqual(t, []byte(got))
}
```

Golden files live in `testdata/<TestName>.golden`. Run `go test -update` to
regenerate after intentional changes. Add `*.golden -text` to
`.gitattributes` to prevent line ending corruption.

**What to test at this tier:**

- Navigation: cursor movement, view stack push/pop, focus cycling
- Rendering: correct layout at different terminal sizes (80x24, 120x40, 60x20)
- State transitions: approve flow states, filter activation, sort cycling
- Data display: proposals render with correct type indicators, timestamps, targets
- Config effects: vim vs arrows keybindings, personality toggle, theme
- Edge cases: empty proposal list, single item, long strings truncated
- Each component in isolation: diff renderer, sparkline, citation expansion

**Test data factory:**

```go
// internal/tui/testdata/fixtures.go
func TestProposal(overrides ...func(*Proposal)) ProposalWithSession
func TestFitnessReport(overrides ...func(*FitnessReport)) FitnessReport
func TestPipelineRun(overrides ...func(*PipelineRun)) PipelineRun
func TestConfig(overrides ...func(*Config)) *Config
```

### Tier 2 — teatest Integration Tests (full program lifecycle)

Charm's official test harness. Creates a `tea.Program` with fake I/O, sends
keystrokes programmatically, waits for output, captures final model state.

```go
func TestFullApproveFlow(t *testing.T) {
    m := NewReviewModel(testStore)
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
    t.Cleanup(func() { tm.Quit() })

    // Wait for dashboard to render
    teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
        return bytes.Contains(b, []byte("PENDING REVIEW"))
    })

    // Open first proposal
    tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

    // Wait for detail view
    teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
        return bytes.Contains(b, []byte("PROPOSED CHANGE"))
    })

    // Verify final state
    fm := tm.FinalModel(t).(reviewModel)
    assert(fm.state == proposalDetail)
}
```

**What to test at this tier:**

- Full navigation flows: dashboard -> detail -> back to dashboard
- Message routing: commands return correctly, async messages arrive
- View transitions: correct child model active after navigation
- Quit behavior: confirmation when pending actions exist

### Tier 3 — Freeze Visual Snapshots (visual acceptance)

[Freeze](https://github.com/charmbracelet/freeze) generates PNG/SVG/WebP
images from terminal output. Instant, self-contained, no heavy dependencies.
Install: `brew install charmbracelet/tap/freeze`.

**Two capture modes:**

**Mode A — Non-interactive CLI output.** Use `--execute` for commands that
print and exit (proposals list, status, inspect):

```bash
freeze --execute "cabrero proposals" -o snapshots/proposals.png
freeze --execute "cabrero inspect prop-abc123" -o snapshots/inspect.png
freeze --execute "cabrero status" -o snapshots/status.png
```

**Mode B — TUI view rendering.** A test harness renders a model's `View()`
to stdout, then pipes through Freeze. This captures any TUI state without
needing an interactive terminal:

```go
// cmd/snapshot/main.go — test harness for visual snapshots
func main() {
    switch os.Args[1] {
    case "dashboard":
        m := NewDashboardModel(fixtureProposals, fixtureStats)
        m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
        fmt.Print(m.View())
    case "detail":
        m := NewProposalDetailModel(fixtureProposal)
        m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
        fmt.Print(m.View())
    // ...
    }
}
```

```bash
go run ./cmd/snapshot dashboard | freeze --language ansi -o snapshots/dashboard.png
go run ./cmd/snapshot detail    | freeze --language ansi -o snapshots/detail.png
go run ./cmd/snapshot dashboard-narrow | freeze --language ansi -o snapshots/dashboard-narrow.png
```

**Snapshots to maintain:**

| Snapshot | Covers |
|----------|--------|
| `dashboard.png` | Dashboard at 120x40 with proposals |
| `dashboard-narrow.png` | Dashboard at 80x24 (compact) |
| `dashboard-empty.png` | Empty state with flavor text |
| `proposal-detail.png` | Detail view with diff and citations |
| `proposal-detail-chat.png` | Detail with chat panel and chips |
| `fitness-report.png` | Fitness report with assessment bars |
| `source-manager.png` | Source list with groups |
| `pipeline-monitor.png` | Pipeline status with sparkline |
| `help-overlay.png` | Help overlay (arrows mode) |
| `help-overlay-vim.png` | Help overlay (vim mode) |

Snapshots are generated on demand (`make snapshots`) and gitignored. The
snapshot harness is a simple Go program — no tmux, no browser, no ffmpeg.
Freeze configuration in `freeze.json`:

```json
{
  "window": true,
  "padding": [20, 40, 20, 40],
  "border": { "radius": 8 },
  "font": { "family": "JetBrains Mono", "size": 14 },
  "theme": "catppuccin-mocha"
}
```

```bash
# Generate all snapshots with consistent styling
make snapshots  # runs snapshot harness + freeze with freeze.json
```

**Why Freeze over VHS:** VHS spawns ttyd + headless Chrome + ffmpeg for
animated GIFs — heavy, slow, built for demos. Freeze generates static
images instantly with zero heavy dependencies. For point-in-time visual
verification during development, Freeze is the right tool.

### Test Infrastructure in Package Structure

```
internal/tui/
├── testdata/
│   ├── fixtures.go              Test data factory functions
│   └── *.golden                 Golden file snapshots
├── dashboard/
│   ├── model_test.go            Tier 1: unit tests
│   └── testdata/*.golden
├── detail/
│   ├── model_test.go
│   ├── diff_test.go             Diff renderer unit tests
│   └── testdata/*.golden
├── ...
├── integration_test.go          Tier 2: teatest full-program tests
cmd/snapshot/
├── main.go                      Tier 3: snapshot harness (renders View to stdout)
freeze.json                      Freeze styling config
snapshots/                       Generated (gitignored)
├── dashboard.png
├── proposal-detail.png
└── ...
```

### CI Considerations

For golden file consistency across environments, force ASCII color profile
in test init:

```go
func init() {
    lipgloss.SetColorProfile(termenv.Ascii)
}
```

This strips ANSI color codes from golden files, making them pure layout
assertions that don't break across terminal emulators.

---

## Charm Library Dependencies

| Library   | Version | Purpose                                    |
|-----------|---------|--------------------------------------------|
| bubbletea | v1.x    | Core TUI framework (Elm architecture)      |
| bubbles   | v0.x    | list, viewport, textinput, textarea, spinner, help, key, paginator, progress |
| lipgloss  | v1.x    | Styling, layout composition, adaptive color|
| huh       | v0.x    | Classification form for unclassified sources|
| glamour   | v0.x    | Markdown rendering in chat responses       |
| x/exp/teatest | v0.x | Tier 2 integration test harness           |
| x/exp/golden  | v0.x | Golden file snapshot comparisons          |
| x/ansi        | v0.x | ANSI code stripping for test output       |

### Dev Tools

| Tool   | Purpose                                            |
|--------|----------------------------------------------------|
| Freeze | Tier 3 visual snapshot generation (PNG/SVG/WebP)   |

---

## References

### Charm Ecosystem

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — Elm-architecture TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) — Reusable components
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — Styling and layout
- [Huh](https://github.com/charmbracelet/huh) — Interactive forms
- [Glamour](https://github.com/charmbracelet/glamour) — Markdown rendering

### Testing

- [teatest](https://github.com/charmbracelet/x/tree/main/exp/teatest) — Bubble Tea test harness
- [Freeze](https://github.com/charmbracelet/freeze) — Terminal output to PNG/SVG images

### Design Inspiration

- [circumflex](https://github.com/bensadeh/circumflex) — Hacker News TUI.
  Message-as-intent pattern, tea.ExecProcess for long content.
- [crush](https://github.com/charmbracelet/crush) — Charm's AI coding TUI.
  Rectangle-based layout, dialog overlay stack, responsive breakpoints,
  pub/sub for async events.
- [lazygit](https://github.com/jesseduffield/lazygit) — Git TUI. Panel
  layout, command transparency, discoverable shortcuts.
- [OpenCode](https://opencode.ai) — AI coding TUI. Chat streaming,
  dialog system, component architecture.
- [tuicr](https://github.com/agavra/tuicr) — Code review TUI. Typed
  comments, infinite-scroll diffs, LLM-optimized export.
