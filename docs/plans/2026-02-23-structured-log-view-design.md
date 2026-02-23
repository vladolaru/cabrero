# Structured Log View Design

**Date:** 2026-02-23
**Status:** Approved

## Goal

Replace the plain-text log viewer with a structured, colorized, collapsible log view that parses log entries, colors level badges, separates entries visually, and supports per-entry expand/collapse for multi-line content.

## Current State

The log viewer (`internal/tui/logview/`) displays raw log text in a Bubble Tea viewport. No parsing, no coloring, no structure. Log format:

```
2026-02-20T10:15:03 [INFO] daemon started (PID 4821)
2026-02-20T10:15:04 [ERROR] something went wrong
```

Features: search with highlighting, follow mode, keyboard scrolling.

## Design

### Data Model

```go
type LogEntry struct {
    Timestamp string   // "2026-02-20T10:15:03"
    Level     string   // "INFO", "ERROR"
    Message   string   // first line of the message
    Extra     []string // continuation lines (stack traces, JSON, etc.)
    Expanded  bool     // whether Extra is visible
    Raw       string   // original full text (for search)
}
```

**Parsing:** Lines matching `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\s+\[(\w+)\]\s+(.*)` start a new entry. Lines without this prefix are continuation lines appended to the current entry's `Extra` slice. An entry is "multi-line" when `len(Extra) > 0`.

### Model Changes

The `Model` struct gains:

- `entries []LogEntry` — replaces raw `lines []string` for structured navigation
- `cursor int` — tracks currently selected entry index
- Raw `lines`/`content` retained for search compatibility

### Rendering

**Color mapping (level badges only):**
- `INFO` → `shared.AccentStyle` (purple)
- `ERROR` → `shared.ErrorStyle` (red)
- Timestamp → `shared.MutedStyle` (dimmed)
- Message text → default foreground
- Continuation lines → default foreground, indented to align with message start
- Collapse indicator `[+N lines]` → `shared.MutedStyle`

**Collapsed entry:**
```
  2026-02-20 10:15:03  INFO   daemon started (PID 4821)

> 2026-02-20 10:15:04  ERR    failed to read config [+3 lines]

  2026-02-20 10:15:05  INFO   pipeline run completed
```

**Expanded entry:**
```
> 2026-02-20 10:15:04  ERR    failed to read config [-]
                                at config.Load (config.go:42)
                                at main.init (main.go:15)
                                caused by: file not found

  2026-02-20 10:15:05  INFO   pipeline run completed
```

**Visual separation:** Blank line between every entry.

**Default state:** Multi-line entries collapsed by default.

### Navigation

Dual navigation model:

| Keys | Action |
|------|--------|
| `j`/`k` or `↑`/`↓` | Move cursor between entries (entry-level) |
| `Page Up`/`Page Down`/scroll wheel | Raw line-level scrolling (for reading long expanded entries) |
| `Enter` or `→` | Toggle expand/collapse of current entry |
| `e` | Expand all multi-line entries |
| `E` | Collapse all multi-line entries |
| `/` | Search (existing) |
| `f` | Follow mode toggle (existing) |
| `n`/`p` | Next/prev search match (existing, now entry-aware) |

Viewport auto-scrolls to keep cursor visible when moving between entries. Manual scroll (`PgUp`/`PgDn`) does not move the cursor.

### Long Entries

When an expanded entry exceeds viewport height:
1. Viewport scrolls to show the entry's first line (timestamp+level)
2. User can scroll freely within the viewport to read the rest
3. `j`/`k` jumps to the next/previous entry, auto-scrolling the viewport

### Search Integration

- Searches across all content (message + extra lines), whether expanded or collapsed
- When a match is in a collapsed entry, auto-expand that entry
- Highlighting: same purple highlight as current implementation
- `n`/`p` jumps cursor to the next/prev matching entry
- First Esc clears search and collapses auto-expanded entries; second Esc pops view

### Follow Mode

- New bytes parsed into entries and appended incrementally
- Partial entries (incomplete last line) extend the current entry's message
- In follow mode: cursor tracks last entry, viewport stays at bottom
- Manual cursor movement disables follow mode

### Files Changed

| File | Change |
|------|--------|
| `internal/tui/logview/model.go` | `LogEntry` struct, `entries`/`cursor` fields, parsing logic, expand/collapse methods |
| `internal/tui/logview/view.go` | Render entries with colors, cursor indicator, expand state, blank line separators |
| `internal/tui/logview/update.go` | Entry navigation (j/k), expand toggle (Enter), expand/collapse all (e/E) |
| `internal/tui/logview/model_test.go` | Parsing tests, expand/collapse tests, search+expand tests, navigation tests |
| `internal/tui/shared/keys.go` | `ExpandAll`/`CollapseAll` key bindings (if not present) |
| `cmd/snapshot/main.go` | Add `log-viewer` snapshot view |

### Patterns Followed

- **Viewport + cursor:** Same as fitness view (viewport for scrolling, cursor index for entry selection)
- **Expand/collapse:** Same as citation entries, source groups, evidence groups (boolean `Expanded` field, Enter toggles)
- **Color usage:** Reuses existing `shared.*Style` definitions, adaptive for light/dark terminals
- **Incremental content:** Same as current `AppendContent()` approach, extended to parse new entries
