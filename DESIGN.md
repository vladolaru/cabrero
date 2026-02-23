# Cabrero — CC Auto-Improvement System

## What This Is

**Cabrero** is a macOS background process that observes Claude Code sessions, extracts behavioral
signals, and proposes improvements to SKILL.md files — with human approval before any changes land.
Runs detached from CC sessions via launchd. No modifications to CC itself.

Named after the Spanish word for goatherd — the one who tends and guides the goats, keeping insights
from scattering. A quiet authority that makes the whole system work.

**Repository:** https://github.com/vladolaru/cabrero

**The goal is to automate compound engineering** — every CC session makes the next one better.
Skills improve from real usage patterns, not manual curation. Over time, the system accumulates
a continuously refined knowledge base that compounds across all future sessions.

### Scope

Cabrero tracks and improves three artifact types across all sources:
- **Skills** — SKILL.md files loaded by CC sessions
- **Commands** — custom slash commands
- **Agents** — sub-agent definitions
- **CLAUDE.md hierarchy** — project and user-level memory files (distinct proposal types, see below)

---

## Source Registry

Artifacts come from multiple sources with different ownership and therefore different
pipeline approaches. Cabrero maintains a source registry:

```
user-level skills/commands/agents   → owned → ITERATE
project-level skills/commands/agents → owned → ITERATE
my-plugins/*                        → owned → ITERATE
third-party-plugins/*               → not owned → EVALUATE
```

**ITERATE** — pipeline produces skill diffs for human approval. The goal is improvement.

**EVALUATE** — pipeline produces fitness assessments. No diffs. The goal is to determine
whether the source is helping or creating friction, informing whether to keep, replace,
or supplement it.

### Fitness Reports (EVALUATE mode output)

Instead of a proposed diff, the evaluator produces a fitness report:

```
PLUGIN: some-third-party-plugin
SKILL:  docx-helper
─────────────────────────────────
Observed in: 14 sessions
Followed correctly: 5
Worked around: 6
Appeared to cause confusion: 3

ASSESSMENT: Low fitness for your workflow.
Consider replacing or supplementing with a user-level skill.
```

Fitness reports appear in the Cabrero Review App as a distinct item type alongside proposals.

### New Source Discovery

When Cabrero encounters a source it hasn't seen before, it pauses processing for that
source and prompts the user to classify it in the Source Manager before proceeding.

---

## CLAUDE.md Pipeline

CLAUDE.md files are memory and steering, not instructions. They go wrong differently
than SKILL.md files, and require a distinct pipeline treatment.

### How CLAUDE.md files degrade

- **Drift** — entries added months ago that no longer reflect how the project works
- **Contradiction** — two entries pulling behavior in opposite directions
- **Over-specification** — so many rules that CC spends cognitive overhead navigating
  them rather than working
- **Silent misfires** — an entry is steering CC away from something that would actually
  be right, visible only as repeated workarounds across sessions

### Signal the evaluator looks for

For SKILL.md the evaluator looks for a skill being read but not helping. For CLAUDE.md
it looks for CC *ignoring or working around* something that should be in memory, or
*following* an instruction that produced friction across multiple sessions. The raw
turn pattern is different — not "skill loaded but bypassed" but "correction repeated
despite persistent context."

### Two proposal sub-types

**Review flag** — an existing entry may be causing friction. The evaluator surfaces the
entry, the sessions where it appeared to misfire, and a brief assessment. No diff is
proposed. The human decides whether to edit, remove, or leave it — CLAUDE.md content
is too project-specific for the evaluator to rewrite safely.

```
CLAUDE.md REVIEW FLAG
Entry:    "Always use the legacy checkout flow for card payments"
─────────────────────────────────────────────────────────────────
Observed in: 8 sessions
Appeared to cause friction: 5
Pattern: CC followed this instruction but workaround was applied afterward in 4 of 5 cases

ASSESSMENT: Entry may no longer reflect current project direction.
Review and edit or remove as appropriate.
```

**Addition proposal** — a repeated correction pattern suggests something worth making
permanent. If the evaluator sees you correcting the same thing across three or more
sessions, it proposes a new CLAUDE.md entry. This *is* a diff proposal, same flow as
SKILL.md: human sees the proposed addition and the sessions that motivated it, approves
or rejects, CLI blends it in if approved.

### What the evaluator does not do

The evaluator never proposes rewrites of existing CLAUDE.md entries — only flags them
for human review. The addition proposal path is the only case where it generates new
content. This keeps the human in control of steering decisions while automating the
pattern recognition that surfaces when steering may have gone wrong.



## Architecture

### Capture Layer (event-driven)

Three-tier hierarchy, in priority order:

1. **`PreCompact` hook** — fires before CC writes a compaction boundary. Receives `transcript_path`,
   `trigger` (auto/manual), `session_id` via stdin JSON. Copies raw JSONL to backup store immediately.
   Note: compaction appends a boundary marker to the existing file rather than rewriting it —
   pre-boundary entries remain physically on disk but become invisible to CC's runtime.

2. **`SessionEnd` hook** — fires on normal session close. Final backup + queues session for
   analysis pipeline.

3. **Daemon stale scan** — crash/kill recovery. Periodic walk of `~/.claude/projects/`
   for session files not in the store and idle >24h. Recovers sessions where hooks never
   fired (e.g. process killed, machine crash).

Hooks configured in `~/.claude/settings.json` (user-level, applies to all sessions).
`/clear` does NOT delete JSONL — it starts a new session with a new UUID, so no special handling needed.

### Raw Backup Store

```
~/.cabrero/
  raw/
    {sessionId}/
      transcript.jsonl       # copied verbatim from CC session
      metadata.json          # session_id, timestamp, capture_trigger, cc_version, status, project
  digests/
    {sessionId}.json         # pre-parser output (structured digest)
  evaluations/
    {sessionId}-classifier.json   # Classifier output
    {sessionId}-evaluator.json    # Evaluator output
  proposals/
    {proposalId}.json        # improvement proposals awaiting human approval
  prompts/
    classifier-v3.txt        # Classifier stage prompt (v3: agentic with Read/Grep, triage, turn budget)
    evaluator-v3.txt         # Evaluator stage prompt (v3: agentic with unrestricted Read/Grep, turn budget)
  replays/
    {replayId}/
      meta.json              # replay metadata (session, stage, prompt, original decision)
      classifier.json        # classifier output (when classifier was replayed)
      evaluator.json         # evaluator output (when evaluator was replayed)
  calibration.json           # calibration set: sessions tagged as ground-truth examples
  config.json                # TUI and daemon settings (debug toggle, navigation, theme, model overrides, etc.)
  blocklist.json             # session IDs to never process (loop prevention)
  run_history.jsonl          # append-only pipeline run history (one JSON object per line)
  daemon.log                 # background daemon log (rotated, 5MB × 3)
  daemon.pid                 # PID file for single-instance enforcement
```

**Rules:** immutable writes on raw backups. Configurable retention (e.g. 90 days raw,
indefinite digests). CC version captured at copy time for schema drift detection.

### Pipeline Run History

`run_history.jsonl` is an append-only JSONL file that captures full diagnostic context
for every pipeline run. One JSON object per line, one line per session processed (batch
runs emit one record per session).

**Recording:** `Runner.RunOne` and `Runner.RunGroup` wrap each pipeline stage with
`time.Now()` calls and append a `HistoryRecord` after each session completes (both
success and error paths). History recording is best-effort — failures don't block the
pipeline.

**Source tracking:** The `Runner.Source` field identifies the invocation path:
`"daemon"`, `"cli-run"`, or `"cli-backfill"`. Set by the caller before `RunOne`/`RunGroup`.

**Batch context:** Records from batch runs share `batch_mode: true`, `batch_size`,
and `batch_session_ids`. Evaluator duration and token usage in batch evaluations are
divided equally among sessions in the chunk (a single LLM call covers multiple sessions).

**Token usage tracking:** Each record includes per-stage `InvocationUsage` structs
(`classifier_usage`, `evaluator_usage`) with CC session ID, turn count, input/output
tokens, cache stats, cost, and web search/fetch counts. Usage is captured via
`--output-format json` from the `claude` CLI, which returns a structured JSON envelope
with full usage data at zero additional overhead. Usage is available even on CC-level
errors (`is_error: true`), providing partial data for failed runs. Totals
(`total_cost_usd`, `total_input_tokens`, `total_output_tokens`) are computed across
all stages before appending the record.

**Fields per record:** session identity, timestamp, project, source, batch context,
capture trigger, previous status (retry detection), triage outcome, pipeline status,
proposal count, error detail, per-stage durations (parse, classifier, evaluator, total
in nanoseconds), per-stage token usage (`classifier_usage`, `evaluator_usage` with
CC session ID, turns, input/output tokens, cache creation/read tokens, cost, web
search/fetch requests), usage totals (`total_cost_usd`, `total_input_tokens`,
`total_output_tokens`), model and prompt versions actually used, and config snapshot
(max turns, timeouts, debug flag).

**Rotation:** On daemon startup, records older than 90 days are removed via
`RotateHistory`. Uses atomic rewrite to prevent partial reads.

**TUI integration:** `ListPipelineRunsFromHistory` prefers history data for timing;
falls back to mtime-based estimation for sessions predating the history file.

### Store Write Invariants

The `~/.cabrero/` store is shared between the CLI, daemon, and TUI. All writers
must follow these invariants to prevent corrupt reads:

- **Atomic writes.** Every file write uses a temp file in the same directory
  followed by `os.Rename()`. Rename is atomic on POSIX filesystems when source
  and destination are on the same volume. This guarantees readers never see
  partial JSON files.
- **No in-place mutation.** Files are replaced whole, never patched. A proposal
  status change writes a new version of the proposal JSON, not a partial update.
- **Directory as index.** Listing proposals or sessions means listing a
  directory. Individual file reads are always of complete, atomically-written
  files. No separate index file that could drift from the directory contents.

These invariants are enforced in the `internal/store` package. The TUI's file
watcher (fsnotify) relies on rename events to trigger reloads — which aligns
with the atomic write pattern since the final rename is the event that signals
a complete write.

### Store Query & Status Helpers

- **`store.QuerySessions(filter SessionFilter)`** — returns sessions matching the
  filter, sorted oldest-first. `SessionFilter` fields: `Since`/`Until` (time range),
  `Project` (substring match), `Statuses` (e.g. `["imported"]` or `["imported", "error"]`).
  Used by `cabrero backfill` and setup wizard.
- **`store.MarkQueued(sessionID)`** — sets a session's status to `"queued"` (ready for daemon).
- **`store.MarkProcessed(sessionID)`** — sets a session's status to `"processed"`.
- **`store.MarkError(sessionID)`** — sets a session's status to `"error"`.

All helpers read the current metadata, update the status field, and write atomically.

### Session Status Model

Sessions flow through these statuses:

- **`imported`** — bulk-imported sessions, not yet selected for processing. Created by
  `cabrero import` and `store.WriteSession()` default. Won't be processed by the daemon
  until explicitly enqueued via `cabrero backfill --enqueue`.
- **`queued`** — sessions ready for daemon processing. Set by hook scripts (session-end,
  pre-compact), stale recovery, and `cabrero backfill --enqueue`.
- **`processed`** — pipeline completed successfully.
- **`error`** — pipeline failed. Retry with `cabrero run <session_id>`.

### JSONL Structure (key facts)

Each line is a JSON entry with: `type`, `message`, `uuid`, `parentUuid`, `sessionId`, `timestamp`.
Sub-agents get their own `agent-{shortId}.jsonl` files linked via `parentUuid`.
Compaction appends `compact_boundary` markers inline — prior entries remain in the file.

### Analysis Pipeline

```
SessionEnd hook fires → transcript + metadata written to store
        ↓
Daemon picks up queued session (status: "queued")
        ↓
Pre-parser (pure code, fast) → structured digest with citations + friction signals
        ↓
Pattern aggregator (pure code) → cross-session recurring patterns (if 3+ project sessions)
        ↓
Agentic Classifier (Read/Grep on ~/.cabrero/raw/)
  → digest as system context, verifies ambiguous signals by reading raw turns
  → enriched classification with triage: worth evaluating or clean session
        ↓ (only sessions Classifier flagged as worth evaluating)
Agentic Evaluator (Read/Grep — unrestricted filesystem access)
  → digest + Classifier output as system context
  → reads current skill files, compares against session-time versions in transcript
  → proposal generation, skill_scaffold proposals for recurring patterns
        ↓
macOS notification if proposals generated
        ↓
Human approval gate
        ↓
claude --model sonnet --print (blends approved change into SKILL.md via writing skill)
```

All LLM calls are mediated by the `claude` CLI. No API keys in the app — CC's existing
auth is reused throughout. Classifier and Evaluator run as agentic tool-using sessions with
constrained tool access (Read, Grep). Apply stage uses `--print` (non-agentic). The
daemon sets `CABRERO_SESSION=1` on all CLI invocations to prevent loop capture.

**Debug mode.** By default, `invokeClaude()` passes `--no-session-persistence` so CC
discards the session transcript after each invocation — only the final text output is
captured. This means there is zero visibility into what tool calls (Read, Grep) the
Classifier and Evaluator made, including which file paths they accessed. When debug mode
is enabled (`--debug` flag or `{"debug": true}` in `config.json`):

1. A UUID v4 session ID is generated and pre-assigned via `--session-id`
2. `--no-session-persistence` is dropped, so CC saves the full JSONL transcript to
   `~/.claude/projects/`
3. The UUID is immediately added to the blocklist to prevent the stale scanner from
   picking it up as an unprocessed session
4. CLI args and session ID are logged for cross-reference with daemon.log

This enables inspecting exactly what paths the agentic evaluators accessed — useful for
diagnosing TCC prompts from network volumes, iCloud, or Google Drive. The config-based
toggle (`store.ReadDebugFlag()`) is re-read at the top of each daemon poll cycle, so
debug mode can be switched on and off without restarting the daemon.

**Reliability.** Two mechanisms protect against transient LLM failures and resource
contention:

- **Concurrent invocation limiter** — a channel-based semaphore in `invokeClaude()` caps
  the number of simultaneous `claude` CLI processes. Default limit: 3 (configurable via
  `PipelineConfig.MaxConcurrentInvocations`; 0 = unlimited). The timeout timer starts
  *after* the semaphore is acquired, so queuing time doesn't eat into execution budget.
  The CLI blocks when all slots are busy (with a log message); the daemon uses non-blocking
  try-acquire and skips the session instead, leaving it queued for the next poll cycle.
- **JSON parse retry** — Classifier and Evaluator output occasionally contains malformed
  JSON (markdown fences, prose preamble). When parsing fails with a retriable JSON error,
  the stage is re-invoked up to `MaxLLMRetries` times (default 1). Non-JSON errors are
  not retried.

**Trigger:** The daemon only processes sessions with status `"queued"`. Hook-captured
sessions (session-end, pre-compact) are written with `"queued"` status and processed
automatically. Bulk-imported sessions get `"imported"` status and must be explicitly
enqueued via `cabrero backfill --enqueue`.

### Pre-Parser (pure code, no LLM)

Defensive design: skip anything ambiguous, leave nulls for Classifier to fill, validate
JSONL line-by-line, preserve unknown entry types in `raw_unknown` bucket.

Extracts deterministically:
- Session shape: duration, turn count, token usage, cache hit ratio, compaction count
- Sub-agent inventory: count, depth, abandoned agents (JSONL exists but result unreferenced)
- Tool call summary: per-tool counts with error attribution, retry anomalies (same tool,
  near-identical inputs, tight window)
- Friction signals: soft failures beyond hard errors — empty search results (Grep/Glob
  returning nothing), search fumbles (3+ distinct search inputs in 60s), backtracking
  (returning to a file after 3+ intervening file accesses)
- Skill usage: which SKILL.md files read (view calls on /mnt/skills/ paths), timing relative
  to first relevant tool call (read-before vs read-after)
- Error signals: tool_result entries with is_error=true, attributed to tool names via
  tool_use_id → tool_name mapping
- Completion signals: todos checked off vs open, git diff presence
- Compaction segments: boundary markers parsed, each segment labeled

Every digest entry references source by `uuid` + `sessionId`. No inference at parse time.
Output explicitly distinguishes extracted fields from skipped/null fields.

### Retrieval Interface (shared across all layers)

Both Classifier and Evaluator are agentic — they run as tool-using `claude` sessions (not
`--print`) with access to the `claude` CLI's built-in Read and Grep tools. This
replaces the earlier design of custom retrieval functions; the built-in tools are
sufficient for navigating JSONL by UUID, reading raw turns, and inspecting files.

The pre-parser digest provides the map: every signal includes `uuid` + `sessionId`
references. Classifier and Evaluator use these to pull raw turns on demand via Read/Grep
against `~/.cabrero/raw/`.

Full citation chain: every Classifier classification and Evaluator finding cites the UUIDs
it's based on. Skill proposals trace back through Evaluator → Classifier → pre-parser → raw entries.

### LLM Stack

CC's existing authentication is reused — no separate API key management. Classifier and
Evaluator run as agentic tool-using sessions via the `claude` CLI with constrained
tool access. Apply and chat stages remain non-agentic.

- **Classifier** (`claude-haiku-4-5`) — agentic classifier with Read and Grep tools
  scoped to `~/.cabrero/raw/`. Receives the pre-parser digest as system context.
  Classifies signals (goal inference, error classification, key turn selection,
  skill/CLAUDE.md signal assessment, cross-session pattern assessment) and
  verifies ambiguous signals by reading surrounding raw turns. Exercises judgment
  about when to deep-read — the prompt provides guidance on situations that
  benefit from raw turn inspection (near-threshold friction, ambiguous error
  attribution, sparse sessions) but the Classifier decides. Guardrails: max retrieval
  calls per session, token budget for retrieved content, session timeout.
  Produces enriched classification with higher-confidence triage, reducing false
  positives passed to the Evaluator and catching signals the pre-parser's structural
  extraction missed.
- **Evaluator** (`claude-sonnet-4-6`) — agentic evaluator with unrestricted Read and
  Grep access (same filesystem access as the user). Receives
  the digest + Classifier's enriched classification as system context. Reads current
  versions of skills and instruction files to compare against what CC saw during
  the session (captured in the raw transcript). Generates proposals including
  `skill_scaffold` proposals for recurring patterns. Only runs on sessions the Classifier
  marked as worth evaluating — the Classifier's improved triage reduces unnecessary
  Evaluator invocations.
- **Evaluator** — apply: blends approved changes into SKILL.md using the writing skill.
  Runs on approval only — infrequent, quality-critical. Non-agentic (`--print`).
- **Evaluator** — chat: interactive proposal interrogation in the Review App. Latency-
  sensitive; fast enough to feel conversational. Non-agentic (`--print`).

### Cross-Session Pattern Aggregation

The pipeline processes sessions in isolation, but some inefficiencies only become visible
across sessions. The pattern aggregator runs between the pre-parser and the Classifier, detecting
recurring friction by comparing the current session's digest against recent sessions in
the same project.

**How it works:**
1. Loads digests from the most recent 20 sessions in the same project (30-day window)
2. Requires at least 3 sessions with digests to detect patterns
3. Computes project-wide baseline error rates per tool
4. Detects two pattern types:
   - **correction_pattern** — the same error (normalized snippet) recurring in 3+ sessions
   - **error_prone_sequence** — a tool with error rate >2× the project baseline across 3+ sessions
5. Error anchoring: only emits patterns backed by actual errors or friction — no patterns
   from repetition alone

**Context budget:** Aggregator output is ~200–500 tokens (pre-filtered in code, limited to
top 5 patterns). Included in the Classifier's system context alongside the digest. The Classifier
classifies each pattern as "confirmed", "coincidental", or "resolved" — and can deep-read raw turns
to verify patterns rather than classifying blind. The Evaluator sees the Classifier's assessment, never
the raw cross-session data.

**Graceful degradation:** No project metadata → aggregator skipped. Fewer than 3 sessions →
returns nil. Aggregator error → non-fatal warning, pipeline continues without cross-session
context.

### Context & Cost Budget

The agentic architecture changes the budget model. Instead of fitting everything into
a single stdin payload, the digest is the starting context and evaluators pull
additional data on demand via tool calls. This mitigates the unbounded digest problem
— evaluators don't need everything upfront, they start with the digest map and drill
into areas of interest.

**Starting context** (system prompt, loaded once per session):

| Component | Classifier | Evaluator |
|-----------|------------|-----------|
| System prompt | ~1.5K tokens | ~1.1K tokens |
| Digest | ~15K tokens (largest: 3923-entry session) | ~15K tokens |
| Cross-session patterns | ~500 tokens | — |
| Classifier output | — | ~2K tokens |
| **Starting total** | **~17K tokens** | **~18K tokens** |

Against a 200K context window the starting context is comfortable (~10x headroom).
Each tool call (Read/Grep on raw JSONL) adds context incrementally. The risk shifts
from "digest too large for stdin" to "too many retrieval calls accumulating context."

**Guardrails per agentic session:**

1. **Max turns** — cap agentic turns per invocation (default: Classifier 15, Evaluator 20).
   Configurable in daemon config. Prevents runaway exploration.
2. **Session timeout** — hard wall-clock limit (default: Classifier 2 min, Evaluator 5 min).
   Configurable in daemon config. The daemon needs predictable throughput.
3. **Digest size cap** — still needed for the starting context. Truncate or summarize
   sections when total digest exceeds a configurable token budget (e.g. 50K tokens).
   Less critical than before since evaluators can pull what the digest omitted.
4. **Per-section limits** — cap friction signals (e.g. top 20 by severity), errors
   (e.g. top 30, deduplicated), retry anomalies (e.g. top 10). These keep the
   starting map readable rather than exhaustive.

**Cost model:** The Classifier (`claude-haiku-4-5`) is cheap per call but agentic sessions
use more tokens than `--print`. The cost guard is the Classifier's triage quality — sessions
classified as clean skip the Evaluator entirely. If 60% of sessions are clean, the agentic
Classifier's higher per-session cost is offset by eliminating 60% of Evaluator invocations.

### Smart Batching — `pipeline.BatchProcessor`

When multiple queued sessions belong to the same project, they are batched
to reduce Evaluator invocations and improve cross-session reasoning.

`pipeline.BatchProcessor` is the shared infrastructure for batching, used by both
the daemon and `cabrero backfill`. Configuration:

- `Config` — pipeline settings (turn budgets, timeouts)
- `MaxBatchSize` — max sessions per Evaluator invocation (default 10, keeps within
  the Evaluator's 60-turn / 15-minute caps)
- `OnStatus` — callback for progress events (`classifier_done`, `evaluator_done`, `error`)

`ProcessGroup(ctx, sessions)` runs the two-phase algorithm:

1. **Phase 1:** Run Classifier individually on each session (cheap, independent triage).
   Clean sessions are marked processed immediately.
2. **Phase 2:** Collect sessions where `triage == "evaluate"`, chunk by `MaxBatchSize`.
   Single-session chunks get individual Evaluator calls; multi-session chunks get
   batched Evaluator calls with all digests + Classifier outputs.

The batching decision is pure code — no orchestrator agent needed. The only
criterion is "same project," which is available from session metadata. The Evaluator's
max turns and timeout scale linearly with batch size (capped at reasonable
maximums). Sessions without project metadata fall back to individual processing.

`cabrero run <session_id>` always processes one session individually — batching
only happens in the daemon and backfill when multiple sessions are queued.

### Human Approval Gate

All SKILL.md modifications require explicit approval before writing. Gate shows full
citation chain (proposal → Evaluator reasoning → Classifier classification → raw turns).
Implementation TBD: menu bar app, Raycast extension, or simple TUI.

### Background Daemon

`cabrero daemon` is a long-running process managed by launchd:

- **Polling** — checks for queued sessions every 2 minutes (configurable via `--poll`)
- **Stale recovery** — scans `~/.claude/projects/` every 30 minutes for sessions where
  hooks never fired (crashes). Imports sessions idle >24h with trigger `"stale-recovery"`
- **Single instance** — PID file at `~/.cabrero/daemon.pid` prevents concurrent daemons
- **Rate limiting** — 30-second delay between processing sessions (configurable via `--delay`)
- **Error isolation** — failed sessions marked `status: "error"` to prevent infinite retry;
  use `cabrero run <id>` to retry manually
- **Notifications** — macOS notification via `osascript` when new proposals are generated
  and when queue processing completes
- **Logging** — timestamped log at `~/.cabrero/daemon.log` with size-based rotation
  (5 MB × 3 files)
- **Graceful shutdown** — responds to SIGTERM/SIGINT, finishes current session before exit
- **Concurrency limiting** — caps simultaneous `claude` CLI invocations via a shared
  semaphore (default 3, configurable via `MaxConcurrentInvocations`). Daemon uses
  non-blocking try-acquire: if all slots are busy, sessions stay queued and are retried
  on the next poll cycle. CLI commands block-wait for a slot with a progress message
- **Transcript validation** — `ScanQueued` skips sessions without a `transcript.jsonl`
  file, preventing repeated failures from incomplete captures
- **Smart batching** — uses `pipeline.BatchProcessor` to group queued sessions by project,
  run Classifier individually (cheap triage), then batch "evaluate" sessions into a single
  Evaluator call per project
- **Model selection** — classifier and evaluator models configurable via CLI flags
  (`--classifier-model`, `--evaluator-model`) or `config.json` (`classifierModel`,
  `evaluatorModel`). Resolution: CLI flag → config.json → compile-time default
  (`claude-haiku-4-5` for classifier, `claude-sonnet-4-6` for evaluator). Active models
  surfaced in `cabrero status`, `cabrero doctor`, TUI pipeline monitor, and run output
- **Evaluator tuning** — turn budgets and timeouts configurable via CLI flags
  (`--classifier-max-turns`, `--evaluator-max-turns`, `--classifier-timeout`, `--evaluator-timeout`)
- **Debug mode** — `--debug` flag or `{"debug": true}` in `config.json` persists full CC
  session transcripts for Classifier/Evaluator invocations. Config-based toggle is re-read
  each poll cycle (no restart needed)

LaunchAgent plist template at `launchd/com.cabrero.daemon.plist` with `KeepAlive`,
`RunAtLoad`, low-priority I/O, and `Nice: 10`.

---

---

---

## Iteration & Prompt Management

Cabrero cannot auto-improve its own pipeline prompts — those files live outside monitored
skill paths by design. Iteration is deliberate, not automatic, which is appropriate:
these prompts shape everything the system proposes, and changes to them deserve the same
care as changes to any critical configuration.

### Prompt files as first-class artifacts

Each pipeline stage's prompt is a named file in `~/.cabrero/prompts/` with a version
header and a short changelog:

```
~/.cabrero/prompts/
  classifier-v3.txt
  evaluator-v5.txt
  apply-v2.txt
```

Editing a file and bumping the version is all it takes to deploy a change — the next
pipeline run picks it up. The version is recorded in every evaluation output, so you
always know which prompt produced which proposal.

### The evaluations store as feedback signal

Every rejected proposal is a calibration data point. If you're consistently rejecting
a certain type of proposal — too aggressive, wrong scope, misreading the signal — that
pattern is visible in the rejections store. The reasons you write when rejecting are
the clearest signal you have for what the evaluator prompt is getting wrong.

### Replay mode

The key capability for prompt iteration. Given a past session already in the backup
store, re-run the pipeline with a new prompt and compare its output against the original
output and your actual approval or rejection at the time:

```bash
cabrero replay --session abc123 --prompt prompts/evaluator-v6.txt
```

This is a CLI command — see the Cabrero CLI section.

Output shows: old proposal, new proposal, your actual decision. Ground truth comes for
free from your own decision history. Across a handful of sessions you can see
immediately whether a new prompt is better calibrated to your judgment before deploying
it.

### Calibration set

As proposals accumulate, tag a small set as canonical examples — clear approvals, clear
rejections, instructive edge cases. Any new prompt candidate runs against the calibration
set before deployment. No infrastructure required: replay mode over a fixed session list
is sufficient.

The iteration loop: notice rejection pattern → adjust prompt file → replay against
calibration set → deploy if output looks right.

## Loop Prevention

When Cabrero invokes `claude`, those CLI calls produce their own session transcripts in
`~/.claude/projects/`. Without explicit filtering, the capture layer would pick them up,
analyze Cabrero's own sessions, generate proposals about how Cabrero prompts models, and
apply them — the system improving its own internals unsupervised.

Three layers of defense, each independent:

**Environment variable sentinel.** Every Cabrero CLI invocation sets a marker:

```bash
CABRERO_SESSION=1 claude --model opus --print < apply_prompt.txt
```

The `SessionEnd` hook reads its own environment and exits immediately if the marker is
present:

```bash
if [ "${CABRERO_SESSION}" = "1" ]; then
  exit 0
fi
```

**Session ID blocklist.** Cabrero records the session ID of every CLI process it spawns.
The capture layer checks incoming session IDs against this blocklist before processing.
Independent of the env var — a second structural filter that doesn't rely on hook behavior.
In debug mode, pre-assigned session IDs are blocklisted immediately before invocation to
prevent the stale scanner from recovering them as unprocessed user sessions.

**Skill path restriction.** Cabrero's internal prompts and any operational files are
stored outside all monitored skill paths — not in `~/.claude/` or any project skill
directory. Even if a session slipped through both filters, there would be no actionable
skill to propose changes to.

The env var approach handles the common case cleanly. The blocklist and path restriction
make the guarantee structural rather than behavioral.

## Key Design Principles

- Hooks are the primary capture mechanism, daemon stale scan is the safety net
- Raw backups are immutable and always written first
- Pre-parser is dumb and fast — no inference, explicit about what it skipped
- AI layers are protected from noise: Classifier sees digests, Evaluator sees curated digests + Classifier output
- Full traceability from proposal back to raw JSONL at every layer
- Schema versioning from day one — CC JSONL format is undocumented and can change
- Cabrero's own CLI sessions are never analyzed — env var sentinel, session ID blocklist, and skill path restriction work in concert
- Failed sessions are marked "error" not retried infinitely — human reviews via `cabrero run`

## Known Limitations / Future Work

- **Blocklist growth.** The session ID blocklist (`blocklist.json`) is append-only. Every
  pipeline invocation adds 1-2 entries (classifier + evaluator CC session IDs). No pruning
  or expiration exists. At current rates this is negligible, but after large backfills the
  file will grow. A retention policy (e.g., prune entries older than 90 days on daemon
  startup, matching `run_history.jsonl` rotation) would keep it bounded.

---

---

## Cabrero CLI

The CLI is built first. It provides full control over the pipeline from the terminal,
enabling every stage to be exercised and tested before the SwiftUI app exists. The app
is a UI layer over a system that already works — not a dependency of it.

### Build order

1. **CLI** — capture, backup, pre-parser, pipeline stages, replay, inspection, apply
2. **App** — reads the same `~/.cabrero/` store the CLI writes; approval workflow and
   notifications built on top of a proven foundation

### Implementation

**Go binary with Bubble Tea.** A self-contained compiled binary with no runtime
dependencies — install it and it works, same distribution story as any well-built CLI
tool. Bubble Tea is the right TUI library for the review interface: mature, composable,
and purpose-built for exactly this kind of interactive terminal UI.

Claude Code itself is TypeScript/JavaScript — a single heavily obfuscated 20MB bundle
distributed via npm. Community deobfuscations exist if CC internals are ever worth
referencing, but the language difference doesn't matter here. Go's single binary
compilation is cleaner for a personal tool: no Node runtime assumption, no npm install,
just a binary in PATH.

Lives at `~/.cabrero/bin/cabrero`, added to PATH on install.

### Subcommands

```
cabrero run <session_id>        Run the full pipeline on a session
  --dry-run                       Run only the pre-parser, skip LLM invocations
  --debug                         Persist CC sessions for classifier/evaluator inspection
  --classifier-max-turns <int>    Max agentic turns for Classifier (default 15)
  --evaluator-max-turns <int>     Max agentic turns for Evaluator (default 20)
  --classifier-timeout <duration> Timeout for Classifier (default 2m)
  --evaluator-timeout <duration>  Timeout for Evaluator (default 5m)
cabrero sessions                List captured sessions with status (processed/pending/error)
cabrero status                  Show pipeline health: sessions, daemon, hooks
cabrero proposals               List pending proposals
cabrero inspect <proposal_id>   Show full proposal with citation chain
cabrero approve <proposal_id>   Approve and apply a proposal (same flow as app)
cabrero reject <proposal_id>    Reject with optional reason
cabrero import [--from <path>]  Seed store from existing CC session files
                                  Runs pre-parser on each imported session to generate a digest.
                                  RunImport(from, dryRun, quiet) available for programmatic use
                                  (quiet mode suppresses per-session output, used by setup wizard).
cabrero backfill                Process existing sessions through the full pipeline
  --since <date>                  Start date filter, YYYY-MM-DD (default: 30 days ago)
  --until <date>                  End date filter, YYYY-MM-DD (default: now)
  --project <slug>                Filter by project slug substring
  --dry-run                       Show preview only, don't process
  --yes                           Skip confirmation prompt
  --retry-errors                  Also re-process sessions with status "error"
  --classifier-max-turns <int>    Override Classifier max turns
  --evaluator-max-turns <int>     Override Evaluator max turns
  --classifier-timeout <duration> Override Classifier timeout
  --evaluator-timeout <duration>  Override Evaluator timeout
cabrero daemon                  Run background session processor (for launchd)
  --poll <duration>               Pending session check interval (default 2m)
  --stale <duration>              Stale session scan interval (default 30m)
  --delay <duration>              Pause between processing sessions (default 30s)
  --debug                         Persist CC sessions for classifier/evaluator inspection
  --classifier-max-turns <int>    Max agentic turns for Classifier (default 15)
  --evaluator-max-turns <int>     Max agentic turns for Evaluator (default 20)
  --classifier-timeout <duration> Timeout for Classifier (default 2m)
  --evaluator-timeout <duration>  Timeout for Evaluator (default 5m)
cabrero replay                  Re-run pipeline with a different prompt against a past session
  --session <id>                  Session to replay (required unless --calibration)
  --prompt <path>                 Path to alternate prompt file (required)
  --stage classifier|evaluator    Override stage (default: infer from prompt filename)
  --compare                       Diff new output against original decision
  --calibration                   Replay against all sessions in calibration set
  --debug                         Persist CC sessions for inspection
  --classifier-model <model>      Override classifier model
  --evaluator-model <model>       Override evaluator model
  --classifier-max-turns <int>    Max agentic turns for Classifier
  --evaluator-max-turns <int>     Max agentic turns for Evaluator
  --classifier-timeout <duration> Timeout for Classifier
  --evaluator-timeout <duration>  Timeout for Evaluator
cabrero calibrate               Manage calibration set for prompt regression testing
  tag <session_id>                Tag a session as calibration example
    --label approve|reject          Expected outcome (required)
    --note "text"                   Optional note
  untag <session_id>              Remove from calibration set
  list                            List calibration entries
cabrero prompts                 List prompt files with current versions
cabrero doctor                  Diagnose issues and auto-fix problems
  --fix                           Auto-fix all fixable issues without prompting
  --json                          Output results as JSON (for scripting)
cabrero setup                   Install and configure Cabrero (interactive wizard)
  --yes                           Skip all confirmations
  --dry-run                       Show what would be done without doing it
cabrero uninstall               Remove Cabrero from this system
  --yes                           Skip all confirmations
  --keep-data                     Keep ~/.cabrero data directory
  --remove-data                   Remove ~/.cabrero data directory
cabrero update                  Update Cabrero to latest release from GitHub
  --check                         Check for updates without downloading
```

### Separation of concerns

The app handles the approval workflow with richer UI — diffs, AI chat, side-by-side
comparison. The CLI handles operating and debugging the system itself — pipeline
execution, prompt iteration, batch operations, inspection. Both read and write the same
`~/.cabrero/` store; there is no duplication of state.

## macOS UI — Cabrero Review App

### Form Factor

Menu bar app (no dock icon). Cabrero lives quietly in the background.
Badge on menu bar icon shows count of pending proposals.
Clicking opens the main Review Window.

### Menu Bar States

- **No badge** — nothing pending, pipeline idle
- **Badge (n)** — n proposals awaiting approval
- **Spinner** — pipeline actively processing a session

### Main Review Window — Pipeline View

A vertically scrollable list of pending learnings, each showing:

```
[ SKILL: docx ]  Session: 2h ago  Signal: retry anomaly (3x)
  raw → parsed → classified → evaluated → ● AWAITING APPROVAL
  [Approve]  [Reject]  [Defer]  [→ Details]
```

Each item displays:
- Which skill is affected
- Session age and duration
- Signal type that triggered it (retry anomaly, skill read late, sub-agent failure, etc.)
- Pipeline stage indicator (where it currently sits in the chain)
- Quick actions inline: Approve / Reject / Defer without opening details

### Detail View (drill-down)

Full citation chain, navigable top-to-bottom:

```
PROPOSED DIFF
─────────────
+ Added step: read SKILL.md before first write tool call
- Removed: redundant re-read pattern

EVALUATOR REASONING
───────────────────
"Skill was read after 3 write attempts in session abc123.
 Pattern observed across 2 sessions in the past 7 days."

CLASSIFIER OUTPUT
─────────────────
Goal inferred: "Create a formatted Word report"
Signal: skill read at turn 18, first write at turn 9
Confidence: high

RAW SESSION TURNS  [↗ open in viewer]
────────────────────────────────────
Turn 9:  [tool_use] write → report.docx
Turn 12: [tool_use] write → report.docx (retry)
Turn 15: [tool_use] write → report.docx (retry)
Turn 18: [tool_use] view → /mnt/skills/public/docx/SKILL.md
```

Raw turns are readable inline. "Open in viewer" link opens the full session
JSONL in a dedicated session browser (separate lightweight window).

Diffs render as actual colored diffs — red/green, line numbers — not code blocks.
Prose change descriptions (non-diff content) are word-wrapped to fit the viewport width.
Raw turns cited by the AI are expandable inline without leaving the view.

Session IDs are displayed in two forms: full UUID in the proposal header, and short
ID (first 8 characters — the first UUID segment) everywhere else (dashboard rows,
pipeline runs, CLI output). A canonical `store.ShortSessionID()` function is used
throughout to ensure consistency.

### AI Chat Integration

The detail view includes an AI chat panel for interrogating proposals you want to
understand before deciding. This is not a general-purpose chatbot — it is scoped
entirely to the current proposal.

**Layout** — the chat panel is togglable via the `c` key. In wide terminals (≥ 120
columns), the chat panel appears beside the proposal detail in a horizontal split.
In narrow terminals, the chat panel appears below the proposal in a vertical split.
Both layouts use the `ChatPanelWidth` config percentage (default 35%) for the chat's
share of the available space (width in horizontal mode, height in vertical mode).
`Tab` switches focus between the proposal and chat panes when the chat panel is open.

**Cold start** — the panel never opens blank. The Classifier generates 3–4 proposal-specific
question chips as part of its classification output, e.g.:

```
Why was this flagged?
Show me the turns where this broke down
Make a more conservative version
What's the risk of approving this?
```

Tapping a chip sends it immediately. Writing your own question is always available.

**What the model can do:**
- Explain the evaluator's reasoning in plain terms
- Retrieve and display specific raw turns on demand (via tool call into the retrieval interface — it doesn't guess, it fetches)
- Produce a revised diff if you ask for a modification ("make this a suggestion, not a rule")
- Assess risk of approving or rejecting

**Revised diffs** produced in chat render immediately as a visual diff alongside the
original. You can approve the revision or the original — both are tracked. Provenance
record shows: evaluator diff → chat modification → approved revision.

**Implementation:** The `claude` CLI is invoked directly — no separate API key, no
extra auth. CC's existing authentication is reused. The citation chain for the current
proposal is injected as the system context. Responses stream via stdout capture from the
spawned process. No server, no network dependency beyond what CC already has.

**Approve / Reject / Defer buttons remain visible at all times during chat.** The
conversation sharpens judgment; it does not replace the decision.

### Applying Changes

All changes are applied by invoking the `claude` CLI rather than patching files directly.
This means the SKILL.md writing skill is loaded at apply time, so changes blend into the
existing document — matching its tone, structure, and conventions — rather than landing
as a foreign patch.

The apply flow on approval:

```
User approves proposal
        ↓
App invokes claude CLI with:
  - SKILL.md writing skill loaded
  - Current SKILL.md content
  - Proposed change and its rationale
  - Instruction to blend the change in, preserving style and structure
        ↓
Claude writes the updated file
        ↓
App shows before/after diff for final confirmation
        ↓
User confirms → change committed
User rejects  → file untouched, back to proposal
```

The final confirmation step matters because Claude is blending, not applying verbatim —
you see what it actually produced before anything is locked in.

This also means the system compounds at two levels: as the SKILL.md writing skill itself
improves over time (via Cabrero), the quality of how Cabrero writes its own changes
improves with it.

Three scenarios:

**Approve original** — the evaluator's proposed change is handed to the CLI alongside
the current file. Claude blends it in and writes the result. Backup of the previous state
is written first. Before the write is committed, the app shows the full diff for
confirmation.

**Approve chat-revised change** — the revision produced during chat is stored as a
distinct proposal version alongside the original. Same CLI-mediated apply process. Full
provenance trail: evaluator proposal → chat revision → approved result.

**Reject with reason** — the rejection reason is written back to the evaluations store
and fed to the evaluator as calibration signal over time. Consistent rejection patterns
raise the bar for future proposals of the same type.

**Rollback** — every approved change stores the previous SKILL.md verbatim as a rollback
entry. The Source Manager shows recent changes per skill with one-click revert. Any
change Cabrero has ever applied can be undone without digging through git history.

### File System Access

Cabrero runs as your user and has standard read/write access to your home directory —
including `~/.claude/` and wherever skills live. No special permissions, no sandboxing
issues for a directly distributed or locally built app.

**App Store distribution would break this.** Apple's sandbox restricts arbitrary file
system writes. Since Cabrero is a personal developer tool, direct distribution (or just
building and running locally) is the right call. App Store is not a target.

**First-write confirmation** — before writing to any SKILL.md for the first time, the
app shows the exact target path and asks for confirmation. After that initial
confirmation per skill, subsequent approved proposals write without additional prompts.
This ensures there is never ambiguity about which copy of a skill is being patched.

### Actions

- **Approve** — applies the diff (original or chat-revised) to the target SKILL.md
- **Reject** — discards proposal, optionally prompts for a short rejection reason
- **Defer** — keeps pending, moves to bottom of list
- **Reject All / Approve All** — bulk actions with confirmation dialog

### Notification

macOS notification fires when new proposals arrive while window is closed.
Notification shows skill name and signal type. Click opens Review Window directly
to that proposal.

### Tech Stack

SwiftUI — native macOS menu bar app. Reads from and writes to `~/.cabrero/` and
skill paths directly. No server. File-system driven: the UI watches the `proposals/`
and `evaluations/` directories for changes and updates reactively. The `claude` CLI
is invoked for all AI interactions — chat and change application alike — reusing CC's
existing authentication with no additional keys or dependencies.

### Source Manager View

Accessible from the menu bar or a dedicated tab. Lists all discovered artifact sources
with their classification:

```
SOURCE                        OWNERSHIP    APPROACH
─────────────────────────────────────────────────────
user-level skills             mine         ● Iterate
project: woocommerce          mine         ● Iterate
plugin: my-plugin             mine         ● Iterate
plugin: some-third-party      not mine     ◎ Evaluate
plugin: another-third-party   not mine     ◎ Evaluate  [unclassified ⚠]
```

- Toggle per source between Iterate and Evaluate
- Unclassified sources are flagged — Cabrero pauses processing them until classified
- New source discovery triggers a macOS notification prompting classification

---

## Progress & Next Steps

**Phase 0 — Repository bootstrap** ✓

- README, Go scaffold, `cabrero help` working

**Phase 1 — Foundation (CLI skeleton + capture)** ✓

- Hook scripts (`pre-compact-backup.sh`, `session-end.sh`), `~/.cabrero/` store layout,
  session ID blocklist, loop prevention, `cabrero sessions` + `cabrero status` + `cabrero import`

**Phase 2 — Analysis pipeline** ✓

- Pre-parser (JSONL → structured digest with citations, compaction segments, anomaly detection)
- Friction detection: empty search results, search fumbles, backtracking signals
- Error attribution: tool_use_id → tool_name mapping, ErrorCount incremented per tool
- Retrieval interface (UUID-based raw entry access shared across all pipeline stages)
- Cross-session pattern aggregator (recurring errors/friction across 3+ project sessions)
- Classifier prompt (`classifier-v3.txt`) — agentic with Read/Grep on raw store,
  goal, errors, key turns, skill/CLAUDE.md signals, friction signals, triage gate, turn budget
- Evaluator prompt (`evaluator-v3.txt`) — proposal generation with citation
  validation, `skill_scaffold` proposals for recurring cross-session patterns
- `cabrero run` + `cabrero proposals` + `cabrero inspect`

**Phase 3 — Background daemon** ✓

- `cabrero daemon` — polls for queued sessions, runs pipeline, macOS notifications
- Stale session recovery (crash scenarios where hooks never fired)
- PID-based single instance, graceful shutdown, file logging with rotation
- LaunchAgent plist template for auto-start on login
- Daemon status indicator in `cabrero status`

**Phase 3.5 — Self-packaging & setup** ✓

- goreleaser builds darwin/amd64 + darwin/arm64 on tag push
- One-liner install script downloads binary from GitHub Releases
- `cabrero setup` — interactive wizard: prerequisite checks, store init,
  hook installation, CC registration, LaunchAgent, daemon start, PATH check,
  process existing sessions (Step 8: imports CC sessions in quiet mode, counts
  imported, offers to enqueue recent sessions for background processing with
  configurable lookback — default 1 month, skippable)
- `cabrero update` — self-update from GitHub Releases with checksum verification
- Hook scripts embedded in binary via `//go:embed`
- `--yes` flag for scripted installs, `--dry-run` for preview

**Phase 4a — Review TUI (core review loop)** ✓

12. **Bubble Tea review interface** — dashboard with proposal list, proposal detail
    view with colored diffs, keyboard navigation, approve/reject/defer ✓
13. **AI chat panel** — streaming Evaluator via `claude` CLI, citation chain as context,
    question chips, revised proposal detection via ` ```revision ` marker ✓
14. **`cabrero approve` + `cabrero reject`** — apply flow via Evaluator + writing skill,
    before/after diff confirmation, rollback entry written ✓

Phase 4a delivers the core value proposition: interactive proposal review with
AI chat. Implemented with Bubble Tea v1.x, Bubbles v1.x, Lip Gloss v1.x.
Configurable via `~/.cabrero/config.json` (arrow/vim navigation, theme,
chat panel width, confirmation toggles). Context-aware help overlay (`?` key)
shows only the key bindings relevant to the current view, grouped into sections
with full descriptions. See `docs/plans/2026-02-20-review-tui-design.md`
for the full design specification.

**Phase 4b — Review TUI (assessment & management)** ✅

15. **Fitness report detail view** — visual assessment bars (three-bucket health
    breakdown), expandable session evidence by category, dismiss and jump-to-sources
    navigation. Dashboard shows fitness reports alongside proposals with `◎` indicator.
16. **Source manager** — grouped source list by origin with collapsible sections,
    ownership classification, iterate/evaluate approach toggles with confirmation gates,
    change history detail with rollback support. Adaptive column layout for wide/standard/narrow
    terminals. See `docs/plans/2026-02-20-review-tui-phase4b-plan.md` for the implementation plan.

**Phase 4c — Review TUI (operational monitoring)** ✓

17. **Pipeline monitor** — daemon health, recent runs with per-stage timing,
    sparkline activity chart, prompt versions, retry flow, polling auto-refresh
18. **Log viewer** — structured log view that parses daemon log entries into colored,
    collapsible entries. Colored level badges (INFO=purple, ERROR=red), muted timestamps,
    cursor-based entry navigation (up/down between entries, PgUp/PgDn for line scrolling),
    expand/collapse for multi-line entries (Enter to toggle, `e`/`E` for all), blank-line
    separators, search with auto-expand of matching collapsed entries, follow mode (`f`).
    Accessible from the pipeline monitor via `L`.

**Phase 5 — Iteration tooling** ✓

19. **`cabrero prompts`** — lists prompt files with name, version, last-modified time, and path
20. **`cabrero replay`** — re-runs pipeline on past sessions with alternate prompts,
    `--compare` mode for diffing against original decisions, `--calibration` mode for
    batch regression testing against the calibration set. Refactored Classifier and Evaluator
    to support prompt overrides via `RunClassifierWithPrompt` / `RunEvaluatorWithPrompt`.
21. **`cabrero calibrate`** — manages calibration set: tag/untag sessions as ground-truth
    examples (approve/reject labels with optional notes) for prompt regression testing.
    Stored at `~/.cabrero/calibration.json`.
