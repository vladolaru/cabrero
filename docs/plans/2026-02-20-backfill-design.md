# Backfill Design

Process existing CC sessions that predate cabrero installation through the full analysis pipeline.

## Problem

When cabrero is first installed, the user already has valuable CC session history in `~/.claude/projects/`. The existing `cabrero import` command copies these into the store but doesn't process them. Imported sessions have `capture_trigger: "imported"`, which the daemon's `ScanPending` rejects (it requires `"session-end"`). The only way to process them is `cabrero run <id>` one at a time.

## Solution

Two complementary changes:

1. **Enhance `cabrero import`** to run the pre-parser on each imported session (cheap, no LLM cost). This makes imported sessions immediately useful for the pattern aggregator.
2. **New `cabrero backfill` command** that runs the full pipeline (Classifier + Evaluator) on stored sessions, with date range and project filtering. Uses smart batching (same as daemon) for cost efficiency.

## Command Interface

### `cabrero import` (enhanced)

```
cabrero import [--from <path>] [--dry-run]
```

Unchanged flags. New behavior: after copying each session's JSONL into the store, runs the pre-parser to generate a digest. Idempotent — skips sessions already in the store.

### `cabrero backfill` (new)

```
cabrero backfill [--since <date>] [--until <date>] [--project <slug>]
                 [--dry-run] [--yes] [--reprocess]
                 [--classifier-max-turns <int>] [--evaluator-max-turns <int>]
                 [--classifier-timeout <dur>] [--evaluator-timeout <dur>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--since` | 30 days ago | Only process sessions from this date forward (YYYY-MM-DD) |
| `--until` | now | Only process sessions up to this date |
| `--project` | all | Filter by project slug substring |
| `--dry-run` | false | Show preview only |
| `--yes` | false | Skip confirmation prompt |
| `--reprocess` | false | Include sessions with status "error" (not just "pending") |
| Pipeline flags | same as `cabrero run` | Override Classifier/Evaluator budgets |

### Session selection

Backfill selects sessions from the store where:
1. `status == "pending"` (always) OR `status == "error"` (if `--reprocess`)
2. `timestamp` within `[--since, --until]`
3. `project` contains `--project` substring (if provided)

No trigger check — unlike the daemon, backfill processes any session regardless of `capture_trigger`. This is the key difference that makes imported and stale-recovered sessions eligible.

## Data Flow

### Phase 1: Preview (always runs)

Shows number of matching sessions, project breakdown, estimated pipeline calls. Requires confirmation unless `--yes` is passed. Even with `--yes`, preview text is printed (confirmation prompt is skipped).

### Phase 2: Processing (smart batching)

1. Group sessions by project
2. Per project group:
   - Run Classifier individually on each session (Haiku, cheap)
   - `"clean"` sessions → mark `status: "processed"`, done
   - `"evaluate"` sessions → collect for batch Evaluator
3. Run Evaluator batch per project (Sonnet, one invocation per project group)
4. Persist results: evaluator output, proposals, update metadata status

Progress output shows per-session Classifier results and per-project Evaluator summaries. Final summary reports totals: processed, clean, evaluated, proposals generated, errors.

### Error handling

- Session fails at Classifier: mark `status: "error"`, log warning, continue
- Evaluator batch fails: mark all sessions in batch as `"error"`, continue to next project
- Final summary reports error count and session IDs
- Failed sessions retried via `cabrero backfill --reprocess`

### Daemon interaction

No conflict. Backfill marks sessions atomically. Daemon's next poll sees updated status and skips. No locking needed.

## Setup Integration

New Step 8 at end of `cabrero setup`:

1. Run import inline (scan `~/.claude/projects/`, copy + pre-parse)
2. Count pending sessions in store
3. If any found: ask "Process recent sessions? [Y/n]" and "How far back? (default: 1 month)"
4. Run backfill inline with chosen date range

With `--yes`: defaults to 1 month, no prompts. With `--dry-run`: shows what would happen.

## Code Architecture

### New files

| File | Purpose |
|------|---------|
| `internal/pipeline/batch.go` | Shared batching logic extracted from daemon |
| `internal/cmd/backfill.go` | CLI command: flags, session selection, preview, progress |
| `internal/store/query.go` | `QuerySessions(filter)` with date, project, status filtering |

### Modified files

| File | Change |
|------|--------|
| `internal/daemon/daemon.go` | Thin wrapper over `pipeline.BatchProcessor` |
| `internal/daemon/scanner.go` | Unchanged (daemon-specific trigger check stays) |
| `internal/cmd/importcmd.go` | Add pre-parser call after each successful import |
| `internal/cmd/setup.go` | Add Step 8: optional backfill prompt |
| `internal/store/session.go` | Add `MarkProcessed`/`MarkError` helpers |
| `main.go` | Register `backfill` command |
| `DESIGN.md` | Document backfill command |
| `CHANGELOG.md` | Add entries under `[Unreleased]` |

### Shared batch processor

```go
// internal/pipeline/batch.go

type BatchProcessor struct {
    Config    PipelineConfig
    OnStatus  func(sessionID string, event BatchEvent)
}

type BatchEvent struct {
    Type    string  // "classifier_done", "classifier_skip", "evaluator_done", "error"
    Triage  string  // "clean" or "evaluate"
    Error   error
}

type BatchResult struct {
    SessionID  string
    Status     string  // "processed" or "error"
    Proposals  int
    Triage     string
    Error      error
}

func (bp *BatchProcessor) ProcessGroup(sessions []BatchSession) []BatchResult
```

The `OnStatus` callback lets daemon and backfill handle progress differently:
- Daemon: logs to `daemon.log`, sends macOS notifications
- Backfill: prints interactive progress to stdout

### Session query

```go
// internal/store/query.go

type SessionFilter struct {
    Since      time.Time
    Until      time.Time
    Project    string      // substring match
    Statuses   []string    // e.g. ["pending"] or ["pending", "error"]
}

func QuerySessions(filter SessionFilter) ([]Metadata, error)
```

## Edge Cases

- **Empty store**: "No sessions found matching filters." Exit 0.
- **All processed**: "0 sessions to process." Exit 0 without prompting.
- **Large backlog (100+)**: Preview shows count. User confirms. Progress keeps them informed.
- **Ctrl+C**: Completed sessions keep results. Interrupted session stays pending/error. Re-run picks up where it left off.
- **`--reprocess` + date filters**: Compose with AND. Error sessions outside date range excluded.
- **Import pre-parser failure**: Session imported to store, warning printed. Backfill's Classifier handles it or marks error.
