# TUI Uniformization Design

**Date:** 2026-02-23
**Status:** Approved

## Goal

Bring consistency to the Cabrero TUI by:
1. Renaming the `review` command to `dashboard` (reflecting what the TUI has become)
2. Simplifying the main header from "Cabrero Review" to "Cabrero"
3. Adding a consistent sub-header section to every view
4. Moving proposal stats from the persistent header into the dashboard view's sub-header
5. Making the help overlay preserve the sub-header (only replacing the content area)

## Current State

- The `review` CLI command launches the TUI via `cmd.Review()` → `tui.Run()`
- The persistent header shows: title ("Cabrero Review"), version, proposal stats, daemon status, hooks
- Only the Source Manager has a sub-header pattern (title + stats + horizontal separator)
- Other views have ad-hoc title rendering inside their content area
- The help overlay replaces the entire content area including any view-local titles

## Design

### 1. Command Rename: `review` → `dashboard`

| File | Change |
|------|--------|
| `main.go` | Rename command `"review"` → `"dashboard"`, update description |
| `internal/cmd/review.go` → `dashboard.go` | Rename file, function `Review` → `Dashboard` |
| `internal/tui/model.go` | Rename `reviewModel` → `appModel` |
| DESIGN.md | Update all `review` references |

### 2. Header: "Cabrero Review" → "Cabrero"

In `RenderHeader()`: change title from `"Cabrero Review"` to `"Cabrero"`.

Remove proposal stats from the header. The header retains: title, version, daemon status, hooks, debug indicator.

### 3. Centralized Sub-Header

The root model's `View()` renders a consistent layout:

```
  Cabrero  v0.13.0                    Daemon: ● running (PID 4821)
                                      Last capture: 1h ago
                                      Hooks: pre-compact ✓  session-end ✗
──────────────────────────────────────────────────────────────────────────
  Proposals
  3 awaiting review  ·  7 approved  ·  2 rejected  ·  1 fitness reports
──────────────────────────────────────────────────────────────────────────
  [view content here]
```

Each view model exposes a `SubHeader(width int) string` method. The root model calls it and wraps with horizontal separators above and below.

### 4. Sub-Header Content Per View

| View | Title | Stats Line |
|------|-------|------------|
| Dashboard | Proposals | `3 awaiting review · 7 approved · 2 rejected · 1 fitness reports` |
| Proposal Detail | Proposal Detail | `<type> · <target> · <confidence>` |
| Fitness Detail | Fitness Report | `<source name> · ownership: <x> · <N> sessions` |
| Source Manager | Source Manager | `5 sources · 2 iterate · 3 evaluate · 1 unclassified` |
| Source Detail | Source Detail | `<source name> · <ownership> · <approach>` |
| Pipeline Monitor | Pipeline Monitor | `captured: N · processed: N · queued: N` |
| Log Viewer | Log Viewer | `N entries · follow ●/○` |

### 5. Help Overlay Preserves Sub-Header

When help is open, the root model renders:

```
[persistent header]
──────────────
[sub-header — same as normal view]
──────────────
[help key binding sections — replaces only the content area]
```

The help overlay renderer (`RenderHelpOverlay`) no longer renders its own title and description — those are redundant with the sub-header. It renders only the key binding sections.

The `HelpContent` struct keeps its `Title` and `Description` fields (used for the sub-header's title/description in help mode, or we can derive from the view's `SubHeader()`). In practice, the sub-header already provides the context, so the help overlay just renders sections.

### 6. Height Calculation

The root model must account for the sub-header height when calculating available space for child views:
- Header: variable (3-4 lines)
- Separator: 1 line
- Sub-header: 2 lines (title + stats)
- Separator: 1 line
- Content: remaining space

Child views receive `height - headerHeight - subHeaderHeight - 2` (for the two separators).
