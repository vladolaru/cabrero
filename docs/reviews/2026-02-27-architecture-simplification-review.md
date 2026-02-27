# Architecture Simplification Review

Date: 2026-02-27

## Context

This codebase is **AI-agent-written and AI-agent-maintained**. The human owner directs work but does not hold the code in their head. No agent carries context between sessions — every agent reads the code cold. This has direct implications for cleanup priority: inconsistencies that a human author would "just know" become traps for agents that pattern-match against whatever they read first.

## Decision Critic Assessment

This review was stress-tested against the actual source code, then further validated by a second decision-critic pass that verified claims against source. Key adjustments:

- **Parser tests elevated to HIGH** — but as a parallel workstream, not a strict blocker for the confirmed bug fix (#2).
- **Finalization duplication downgraded to MEDIUM** — the duplication is real but the 6 paths share identical bookkeeping; a parameterized helper is recommended with moderate confidence.
- **TUI god object downgraded to LOW** — 1293 lines is within normal range for Bubble Tea's single-model architecture; defer until real pain.
- **Proposal ID prefix moved into scope** — originally deferred, but the implicit contract between prompt instruction and partitioning code is invisible to the type system. In an AI-maintained codebase, an agent modifying the evaluator prompt can silently break batch partitioning with no compile-time signal. The fallback masks failures by silently doubling LLM calls.
- **Status constants evidence corrected** — original "14 vs 17" count conflated status assignments with all string occurrences; actual split is 4 literal status assignments vs 10 constant-based.
- **Dependency chain restructured** — #2 (bug fix) no longer strictly blocked by #1 (comprehensive parser tests); a focused test-and-fix is the faster path.
- **Hook/LaunchAgent duplication fully verified** — per-location analysis of all 7 sites confirms extractable shared logic. Two distinct subsystems: hook settings (6 locations, HIGH duplication including one literal copy-paste) and LaunchAgent plist (4 locations, MEDIUM duplication concentrated in setup+doctor). Caveat removed.

## Findings (ordered by implementation priority)

### 1) High: parser has no unit tests despite high complexity

Important for long-term safety of parser changes. Parallel workstream — no longer a strict blocker for #2.

- Evidence:
  - [internal/parser/parser.go:102](internal/parser/parser.go:102) — 1041-line file with heuristic-heavy logic
  - [internal/parser/parser.go:707](internal/parser/parser.go:707) — agent finalization logic (contains confirmed bug)
  - Zero `*_test.go` files in `internal/parser/`
  - Note: 3 test files (`runner_test.go`, `batch_test.go`, `processqueued_test.go`) import and exercise the parser at the integration level, so coverage is not truly zero — the gap is unit tests for heuristic-heavy functions.
- Simplification:
  - Add targeted table tests for agent finalization, friction detection, and CLAUDE.md extraction before any refactors.
- Dependency: **Parallel workstream.** A focused test for the specific agent mapping bug (#2) can ship independently; comprehensive parser tests should follow.

### 2) High: agent-to-agentID mapping in parser is logically wrong and will assign the same `agentId` to multiple spawned agents

Confirmed bug. The inner loop assigns the first sorted agentID to every spawn item because each `item` starts with `EntryCount == 0` and the pool is never depleted.

- Evidence:
  - [internal/parser/parser.go:730](internal/parser/parser.go:730) — outer loop over `agentSpawns`
  - [internal/parser/parser.go:742](internal/parser/parser.go:742) — `EntryCount == 0` condition true for every new item
  - [internal/parser/parser.go:747](internal/parser/parser.go:747) — assignment without removing from pool
- Simplification:
  - Stop forcing ambiguous mapping in-place.
  - Move correlation to a dedicated resolver that returns `unknown` when evidence is weak.
  - Record mapping confidence/method (e.g., `exact`, `heuristic`, `unknown`) so downstream analysis can distinguish "unmapped" from "no agent activity."
- Dependency: **Ship with a focused test for the mapping bug.** Comprehensive parser tests (#1) are valuable but not a prerequisite for this specific fix.

### 3) Medium: session statuses are still hard-coded strings in multiple places

Two parallel vocabularies for the same field. In an AI-maintained codebase, an agent adding a new error path will copy whichever pattern it encounters first — if it reads line 248 (`"error"` literal) before line 320 (`HistoryStatusError` constant), it perpetuates the inconsistency. Consolidating to one vocabulary eliminates this class of drift.

- Evidence:
  - [internal/store/session.go:15](internal/store/session.go:15) — `Status*` constants exist
  - [internal/pipeline/runner.go:248](internal/pipeline/runner.go:248) — uses `"error"` literal for `rec.Status`
  - [internal/pipeline/runner.go:320](internal/pipeline/runner.go:320) — uses `HistoryStatusError` constant for `rec.Status` (same field, different approach — copy-paste inconsistency)
  - [internal/cmd/backfill.go:104](internal/cmd/backfill.go:104)
  - [internal/cmd/status.go:43](internal/cmd/status.go:43)
- Note: `runner.go` has 4 status assignments using `"error"` literal (lines 248, 434, 458, 462) alongside 10 using `HistoryStatus*` constants — two parallel vocabularies for the same field. (The broader grep counts of 14/17 include non-status contexts like log messages and event emissions.)
- Simplification:
  - Standardize on `store.Status*` / `HistoryStatus*` constants and consolidate transitions.
- Dependency: **Prerequisite for #4.**

### 4) Medium: duplicated mtime-based run reconstruction logic

Small extraction, low risk.

- Evidence:
  - [internal/pipeline/run.go:70](internal/pipeline/run.go:70) — `ListPipelineRunsFromSessions` builds run from filesystem
  - [internal/pipeline/run.go:187](internal/pipeline/run.go:187) — fallback mtime estimation duplicates the same file-stat pattern
- Simplification:
  - Extract helper `buildRunFromFilesystem(meta)` and reuse.

### 5) Medium: pipeline run orchestration repeats outcome-finalization flow in many branches

The duplication is real (6 locations follow the same set-status/set-error/compute-totals/append-history/mark-store pattern) and is already causing inconsistency (literal vs constant mix). The six paths differ in context (which result struct is updated, whether events are emitted, RunOne vs batch) but share an identical bookkeeping sequence. A `finalizeSessionOutcome()` helper taking `(rec, status, error, duration, sessionID)` would serve all 6 paths — the contextual differences are parameterizable, not semantic.

In an AI-maintained codebase, 6 copy-paste sites means an agent modifying the bookkeeping sequence (e.g., adding a new field to `HistoryRecord`) must find and update all 6 — with no guarantee it finds them all. A single canonical function makes the change once.

- Evidence:
  - [internal/pipeline/runner.go:247](internal/pipeline/runner.go:247) — RunOne classifier error
  - [internal/pipeline/runner.go:271](internal/pipeline/runner.go:271) — RunOne triage-clean path
  - [internal/pipeline/runner.go:319](internal/pipeline/runner.go:319) — RunOne evaluator error
  - [internal/pipeline/runner.go:457](internal/pipeline/runner.go:457) — batch classifier error
  - [internal/pipeline/runner.go:576](internal/pipeline/runner.go:576) — batch per-session evaluator error
  - [internal/pipeline/runner.go:651](internal/pipeline/runner.go:651) — batch partition error
- Simplification:
  - Centralize into one `finalizeSessionOutcome(...)` path after #3 cleans up the status constant mess.
- Note: The Go idiom of explicit error handling applies to error *decisions*, not to bookkeeping sequences that happen to occur near error returns. These 6 paths make the same bookkeeping calls in the same order — that's duplication, not intentional explicitness.
- Dependency: **Benefits from #3 (status constants) first.**

### 6) Medium: hook/LaunchAgent integration logic is duplicated across commands and TUI

Fully verified. Three separate implementations of the same "contains cabrero" check (`hookGroupContainsCabrero`, `hookContainsCabrero`, `containsCabrero`) are exactly the kind of ambiguity that causes an agent to copy-paste a local implementation rather than find the canonical one. Two distinct subsystems with different duplication profiles:

**Subsystem A — Claude Code Hook Settings (6 locations, HIGH duplication):**

All read `~/.claude/settings.json`, parse JSON, inspect/mutate the `hooks` map. Three separate implementations of the "contains cabrero" check exist (`hookGroupContainsCabrero` in setup.go, `hookContainsCabrero` in status.go, `containsCabrero` in tui.go). `tui.go:170` is a literal copy-paste of `status.go:142` (even has comment "Reuse the same logic from cmd/status.go" but duplicates instead of importing).

| Location | Operation | What's shared |
|----------|-----------|---------------|
| [setup.go:172](internal/cmd/setup.go:172) `stepRegisterHooks` | Read settings → check hooks → write new hooks | Read/parse/check + write-back |
| [doctor.go:518](internal/cmd/doctor.go:518) `checkClaudeCodeIntegration` | Read settings → validate hooks + paths | Read/parse/check |
| [doctor.go:668](internal/cmd/doctor.go:668) `makeRegisterHooksFix` | Write hooks (fix callback) | Write-back (near-identical to setup.go:260-274) |
| [uninstall.go:215](internal/cmd/uninstall.go:215) `stepUnregisterHooks` | Read settings → find cabrero hooks → remove → write | Read/parse/check + write-back |
| [status.go:142](internal/cmd/status.go:142) `checkHooks` | Read settings → bool check | Read/parse/check |
| [tui.go:170](internal/tui/tui.go:170) `checkHookStatus` | Read settings → bool check | Read/parse/check (copy-paste of status.go) |

**Subsystem B — LaunchAgent Plist (4 locations, MEDIUM duplication):**

Setup and doctor share the heavy logic (binary path resolution, `renderPlist`, content comparison). Uninstall locations are trivial (path + one OS call).

| Location | Operation | Duplication level |
|----------|-----------|-------------------|
| [setup.go:287](internal/cmd/setup.go:287) `stepInstallLaunchAgent` | Resolve binary → render plist → compare → write + load | HIGH (shared with doctor) |
| [doctor.go:714](internal/cmd/doctor.go:714) `checkLaunchAgent` | Read plist → validate binary → compare with expected → offer fix | HIGH (shared with setup) |
| [uninstall.go:129](internal/cmd/uninstall.go:129) `stepStopDaemon` | `launchctl unload` before kill | LOW (3 lines, path only) |
| [uninstall.go:157](internal/cmd/uninstall.go:157) `stepRemoveLaunchAgent` | Delete plist file | LOW (path + `os.Remove`) |

- Simplification:
  - Extract a `claudeintegration` package (or similar) with:
    - `LoadSettings() (map[string]interface{}, string, error)` — serves all 6 hook locations
    - `WriteSettings(settings, path) error` — serves setup, doctor fix, uninstall
    - `HookStatus() (preCompact, sessionEnd bool)` — replaces 3 duplicate contains-cabrero implementations
    - `PlistPath() string` + `RenderAndCompare() (current bool, error)` — serves setup + doctor
  - The uninstall plist locations (path + one OS call) are too trivial to extract.

### 7) Medium: batch proposal/session association relies on implicit prompt convention instead of typed contract

The link between the evaluator prompt instruction (`evaluator.go:136`: "Use the standard proposal ID format: prop-{first 8 chars of sessionId}-{index}") and the Go partitioning code (`runner.go:644`: prefix matching on `ShortSessionID`) is invisible to the type system. An agent modifying the evaluator prompt can break batch partitioning with no compile-time signal. The fallback at `runner.go:625-631` can roughly double LLM calls; it is logged, but not surfaced as degraded batch efficiency in user-facing status/cost views.

- Evidence:
  - [internal/pipeline/evaluator.go:136](internal/pipeline/evaluator.go:136) — prompt instruction defines the ID format convention
  - [internal/pipeline/runner.go:634](internal/pipeline/runner.go:634) — partitioning relies on the same convention (comment only)
  - [internal/pipeline/runner.go:644](internal/pipeline/runner.go:644) — `ShortSessionID` prefix matching
  - [internal/pipeline/runner.go:625-631](internal/pipeline/runner.go:625) — logged fallback to per-session evaluation on batch failure
  - [internal/pipeline/runner.go:651](internal/pipeline/runner.go:651) — validation catches mismatches but marks all sessions as error
  - [internal/store/session.go:174](internal/store/session.go:174) — `ShortSessionID` serves double duty (display AND partitioning)
- Risks of current approach:
  - **Implicit contract:** No type, constant, or struct field signals that proposal ID format is load-bearing. An agent sees a comment and a prompt string — easy to change one without the other.
  - **Fallback cost visibility gap:** The fallback at line 625 retries every session individually. This is logged, but not currently surfaced as degraded batch efficiency in user-facing status/cost outputs.
  - **8-char collision:** `ShortSessionID` takes the first 8 hex chars (~4B combinations). Astronomically unlikely for typical batch sizes; if it occurs, partition validation fails closed and marks the chunk as error (not silent mis-attribution).
  - **Dual-purpose `ShortSessionID`:** Used for display (TUI, logs) AND partitioning. Changing the display format breaks partitioning.
- Simplification:
  - Add explicit `sessionId` field on `Proposal` struct and in evaluator output schema.
  - Partition by `proposal.SessionID == session.ID` instead of prefix matching.
  - Validate the `sessionId` field is present and matches a known session — compile-time type enforcement + runtime validation.
  - Decouple `ShortSessionID` from partitioning (keep for display only).
- Scope note: Requires updating evaluator prompt, `Proposal` struct, `filterProposals`, partitioning in `runner.go`, and existing tests in both `batch_test.go` and `runner_test.go` (partition mismatch + fallback behavior). Existing stored proposals with old format need graceful handling (tolerate missing `sessionId` field, fall back to prefix matching for legacy data).

### 8) Low: TUI root model is a large single file

1293 lines is within normal range for Bubble Tea's single-model-with-Update() architecture. The framework is designed around this pattern — splitting into per-view controllers adds coordination overhead and fights the framework.

- Evidence:
  - [internal/tui/model.go:110](internal/tui/model.go:110) — `Update()` switch (standard Bubble Tea)
  - [internal/tui/model.go:309](internal/tui/model.go:309) — message routing
  - [internal/tui/model.go:683](internal/tui/model.go:683) — `pushView()` view initialization
  - [internal/tui/model.go:1039](internal/tui/model.go:1039) — `switchView()` navigation
- Simplification (defer):
  - Introduce per-view controllers and keep the root as a router/composer.
  - Move I/O-producing commands to injected services.
- Recommendation: Monitor file size. Revisit when the model exceeds ~2000 lines or when adding new views becomes painful.

## Dependency Chain

For AI-maintained code, cleanup that reduces ambiguity and consolidates patterns has outsized returns — agents cannot carry context between sessions and will propagate whichever pattern they encounter first. Items #3-#7 are not polish; they are drift prevention.

```
#2 Fix agent mapping bug (with focused test — ship independently)
#1 Comprehensive parser tests (parallel workstream)

#3 Status constants cleanup (eliminates dual-vocabulary drift)
 └─► #5 Finalization flow extraction (single canonical bookkeeping path)

#4 Mtime helper extraction (independent)
#6 Hook/LaunchAgent extraction (independent, verified)
#7 Proposal session ID (independent — makes implicit prompt/code contract explicit)
#8 TUI model split (defer — monitor growth)
```

## Open Questions / Assumptions

1. ~~Can evaluator output schema be changed now?~~ **Yes — in scope.** The implicit prompt/code contract is a drift hazard for AI agents. Add explicit `sessionId` field to `Proposal` struct and evaluator schema. Handle legacy proposals gracefully (fall back to prefix matching when `sessionId` is missing).
2. ~~Is the desired target architecture a thin CLI/TUI over shared services, or should behavior stay co-located in command/view packages?~~ **Shared services.** For AI-agent maintainability, a single import path per concern beats logic spread across multiple command files. Key reasons:
   - **Discoverability:** An agent finds one package to import, not 5 files to choose from. Co-located logic is how the current copy-paste (`tui.go` duplicating `status.go`) happened.
   - **Change propagation:** When settings format changes, update one package. With co-location, the agent must find and update each command's inline logic — and may miss some.
   - **Import-enforced consistency:** If `cmd/status` and `tui` both import `integration.HookStatus()`, they can't drift. Separate inline functions will.
   - **Target layout:** Extract `internal/integration/claude/` (settings, hooks) and `internal/integration/launchagent/` (plist). Most of the codebase (`pipeline`, `parser`, `store`) is already service-shaped — the gap is only in Claude Code integration logic that grew organically inside commands. CLI/TUI keep their UX (prompting, formatting, dry-run) and compose the shared services.
3. For parser agent mapping, should unresolved associations remain empty rather than heuristic-filled? **Recommendation: yes — `unknown` is more honest than wrong.**

## Validation

1. `go test ./...` passed.
2. `go vet ./...` passed.
3. `go test -cover ./...` failed in this environment due missing Go coverage toolchain (`covdata`).
