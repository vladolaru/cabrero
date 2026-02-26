# Pipeline Self-Improvement Loop — Design

**Date:** 2026-02-26
**Status:** Approved for implementation

---

## Goal

Close the compound engineering loop on the pipeline itself: give the classifier and
evaluator the same observability and improvement machinery that they apply to Claude Code
sessions. Every component that invokes an LLM must be independently configurable, always
inspectable, and automatically measurable — with a daily meta-pipeline that generates
concrete `prompt_improvement` proposals when quality signals degrade.

---

## Context: The Gaps

From the observability audit:

1. **Session transcripts discarded in production.** `--no-session-persistence` is passed
   for all non-debug pipeline invocations. `cc_session_id` in every `HistoryRecord` is a
   dead reference unless debug mode was active. No visibility into what the classifier or
   evaluator actually did (which files it read, how many tool calls it made).

2. **Blocklist growth is unbounded.** Currently a flat `[]string` with no timestamps.
   Enabling always-on persistence requires two blocklist entries per pipeline invocation —
   at scale this becomes a performance problem. No rotation mechanism exists.

3. **Proposal outcomes never feed back.** `ProposalsApproved` and `ProposalsRejected` in
   `PipelineStats` are always zero. `archiveReason` is a free-text string with no
   timestamp. No code joins archived proposals to the `HistoryRecord` that generated
   them, so `evaluator_prompt_version → acceptance_rate` is uncomputable automatically.

4. **Model config incomplete.** `CuratorCheckModel` reuses `ClassifierModel`. `Blend`
   (apply stage) and `buildChatArgs` (chat stage) hardcode `"claude-sonnet-4-6"`.
   No `MetaModel` constant exists. No single authoritative place lists every model in use.

5. **No daily quality analysis.** Nothing monitors classifier false-positive rate
   (evaluate→zero-proposals), per-prompt-version acceptance rates, or turn budget
   utilisation. Prompt degradation is invisible until noticed manually.

6. **CC format drift is silent in the main pipeline.** `parser.go` collects unrecognised
   entry types and malformed lines into `Digest.RawUnknown`, but `runner.go` never
   inspects it. If CC adds new entry types or changes its JSONL schema, the parser
   silently drops the affected entries and the classifier/evaluator see a degraded digest
   with no warning anywhere.

---

## Architecture

Four components, each a prerequisite for the next:

```
1. Blocklist rotation
         ↓
2. Always-on session persistence  +  Debug simplification
         ↓
3. Outcome tracking  +  Model config completeness
         ↓
4. Daily meta-pipeline  →  prompt_improvement proposals
```

---

## Component 1: Blocklist Rotation

### Problem

`blocklist.json` is a flat `[]string` with no timestamps. Entries accumulate forever.
With always-on persistence adding two entries per pipeline invocation, the list grows
proportionally to processing throughput. `readBlocklist()` re-reads and re-parses the
entire file on every `BlockSession` and `IsBlocked` call — O(n) disk reads.

### Design

**Schema change:** Replace `[]string` with `map[string]BlocklistEntry`:

```go
type BlocklistEntry struct {
    BlockedAt time.Time `json:"blockedAt"`
}
```

`BlockSession(sessionID string, blockedAt time.Time) error` — adds a timestamped entry.
Existing callers pass `time.Now()`.

`RotateBlocklist(maxAge time.Duration) (int, error)` — removes entries older than maxAge,
rewrites atomically. Same pattern as `RotateHistory` and `RotateCleanupHistory`.

**Daemon startup:** `RotateBlocklist(90 * 24 * time.Hour)` runs alongside the existing
two rotation calls. 90 days matches the other history files; CC's own session store
retains sessions far less than 90 days, so blocklisted entries beyond that age serve
no purpose.

**Migration:** On read, if the JSON root is an array (old format), convert in-place:
treat existing entries as `BlockedAt: time.Time{}` (zero time). They will be rotated
out after 90 days or replaced if re-blocklisted. One-way migration, transparent to callers.

---

## Component 2: Always-On Session Persistence and Debug Simplification

### Design

**Agentic invocations** — move session ID generation and blocklisting out of the debug
branch and into the standard path:

```
Every agentic invokeClaude call:
  1. Generate UUID (always)
  2. store.BlockSession(uuid, time.Now())  (always)
  3. Pass --session-id <uuid> to claude CLI  (always)
  4. Do NOT pass --no-session-persistence  (agentic mode, always)
```

**Print-mode invocations** (non-agentic `--print`) keep `--no-session-persistence`.
These are single-turn calls with no tool use (Curator check, Apply/Blend stage) — their
transcripts contain no useful diagnostic information.

**Debug mode after this change:** The `Debug` flag's only remaining effect is a log line:

```
[debug] CC session <uuid> persisted — classifier for <sessionId>
[debug] CC session <uuid> persisted — evaluator for <sessionId>
```

No behavioural change vs. non-debug — persistence is now always on. Debug is a verbosity
toggle: it surfaces the session ID in `daemon.log` so you can find the transcript without
scanning `run_history.jsonl`. The `--debug` flag and `{"debug": true}` in `config.json`
remain valid and useful for active investigation.

**Result:** Every `HistoryRecord.classifier_usage.cc_session_id` and
`evaluator_usage.cc_session_id` is a live pointer into `~/.claude/projects/`. The
meta-pipeline can read classifier/evaluator transcripts for any historical run.

**CC format drift warning (main pipeline).** After `ParseSession` returns, `runner.go`
checks `len(digest.RawUnknown) > 0` and logs a structured warning if any entries were
unrecognised or malformed:

```
WARN session %s: %d unrecognised transcript entries (types: %v) — CC format may have changed
```

`RawUnknown` already captures this data; this change surfaces it. Same principle as
the transcript validation in `RunMetaAnalysis`.

---

## Component 3: Model Config Completeness and Outcome Tracking

### 3a — Model Config

Every LLM-using entity gets its own named field in `PipelineConfig` and a `Default*Model`
constant. No entity may borrow another stage's config field or hardcode a model string.

| Entity | Field | Default |
|---|---|---|
| Classifier | `ClassifierModel` | `"claude-haiku-4-5"` |
| Evaluator | `EvaluatorModel` | `"claude-sonnet-4-6"` |
| Apply (Blend) | `ApplyModel` | `"claude-sonnet-4-6"` |
| Chat | `ChatModel` | `"claude-sonnet-4-6"` |
| Curator | `CuratorModel` | `"claude-sonnet-4-6"` |
| Curator check | `CuratorCheckModel` | `"claude-haiku-4-5"` |
| Meta | `MetaModel` | `"claude-opus-4-6"` |

Threading:
- `apply.Blend(proposal, sessionID, model string)` — add `model` parameter, callers pass
  `pipelineCfg.ApplyModel` (threaded through from TUI model and CLI approve command).
- `chat.ChatConfig` gains `Model string`; `buildChatArgs` uses `cfg.Model`; `buildChatConfig`
  in `tui/model.go` receives `pipelineCfg.ChatModel`.
- `RunCuratorCheck` uses `cfg.CuratorCheckModel` instead of `cfg.ClassifierModel`.

**TUI:** The Pipeline Monitor `MODELS` section expands to show all seven models, not just
Classifier and Evaluator.

### 3b — Typed ArchiveOutcome

Replace free-text `archiveReason` with a typed enum and add a timestamp:

```go
type ArchiveOutcome string
const (
    OutcomeApproved     ArchiveOutcome = "approved"
    OutcomeRejected     ArchiveOutcome = "rejected"
    OutcomeCulled       ArchiveOutcome = "culled"         // curator rank-cull
    OutcomeAutoRejected ArchiveOutcome = "auto-rejected"  // curator already-applied
    OutcomeDeferred     ArchiveOutcome = "deferred"
)
```

`apply.Archive(proposalID string, outcome ArchiveOutcome, note string) error`
— `note` is the optional human-written rejection reason (empty for curator calls).

Archived proposal JSON gains `"outcome"` and `"archivedAt"` fields. `"archiveReason"`
is retained for backward-compatibility reads but not written by new code.

**Migration:** `readArchiveOutcome(raw map[string]json.RawMessage)` maps old
`archiveReason` strings (`"approved"`, `"rejected"`, `"rejected: ..."`, `"deferred"`,
`"auto-culled: ..."`) to the new enum on read.

**`GatherPipelineStatsFromSessions`** scans `proposals/archived/` within the time window
and counts by outcome → populates `ProposalsApproved` and `ProposalsRejected`.

### 3c — History↔Archive Join

```go
type AcceptanceStats struct {
    PromptVersion  string
    Generated      int
    Approved       int
    Rejected       int
    AcceptanceRate float64  // Approved / (Approved + Rejected), NaN if zero
    SampleSize     int      // Approved + Rejected (excludes culled/deferred)
}

func ListAcceptanceRateByPromptVersion() ([]AcceptanceStats, error)
```

Joins `ReadHistory()` (session → `evaluator_prompt_version`) with archived proposals
(`session → outcome`) on `sessionId`. Returns one entry per evaluator prompt version
with ≥1 archived proposal. Sorted by most-recently-active version first.

---

## Component 4: Daily Meta-Pipeline

### Trigger

A fourth daemon ticker, `metaTicker`, fires every 24h (`Config.MetaInterval`, default
24h). Independent of the Curator and session-processing tickers.

### Stage 1 — Pure Code Metric Computation (no LLM)

`ComputePipelineMetrics(cfg PipelineConfig) (PipelineMetrics, error)` runs on every tick:

```go
type PipelineMetrics struct {
    // Per evaluator prompt version.
    AcceptanceByVersion []AcceptanceStats

    // Classifier quality.
    ClassifierFPR       float64   // evaluate→zero-proposals / total evaluate sessions
    ClassifierFPRWindow int       // days of data used

    // Turn budget utilisation (last 30 days).
    ClassifierMedianTurns float64
    EvaluatorMedianTurns  float64

    // Cost.
    CostPerAcceptedProposal float64  // last 30 days, all versions combined

    // Computed at.
    ComputedAt time.Time
}
```

If no version crosses a threshold **and** no version has enough samples, log one line and
return. Zero LLM cost.

**Thresholds** (configurable in `PipelineConfig`):

```go
MetaRejectionRateThreshold float64       // default 0.30  (30% rejected)
MetaClassifierFPRThreshold float64       // default 0.25  (25% evaluate→no proposals)
MetaMinSamples             int           // default 5     (minimum proposals for analysis)
MetaCooldownDays           int           // default 14    (skip if recent proposal exists)
```

### Stage 2 — Opus Meta-Analysis (conditional LLM)

`RunMetaAnalysis` fires only when a threshold is crossed with `SampleSize ≥ MetaMinSamples`
AND no `prompt_improvement` proposal for that version exists in the last `MetaCooldownDays`.

The Opus session receives (via `Read,Grep` tool access):
- The current prompt file content
- The 10 most-recently rejected proposals for that version (with `RejectionNote`)
- The computed `AcceptanceStats` for that version
- The CC session transcripts for the 3 highest-turn rejected-proposal runs
  (using `cc_session_id` from `HistoryRecord` — requires always-on persistence;
  expected path: `~/.claude/projects/<cwd-slug>/<cc_session_id>.jsonl`, where
  `cwd-slug` is the daemon's working directory slugified by CC's path-to-slug convention)

Before passing transcript paths to Opus, `RunMetaAnalysis` validates each file:

1. **File not found** — log a structured warning and skip that transcript:
   ```
   WARN transcript not found for cc_session_id %s at expected path %s — CC storage conventions may have changed
   ```
2. **File found but contains zero `tool_use` entries** — log a warning and skip:
   ```
   WARN transcript %s contains no tool_use entries — CC format may have changed or this is a print-mode session
   ```

A transcript with no tool calls is useless as behavioral evidence and would cause Opus
to produce low-quality analysis with no visible error. Both warnings surface CC storage
or format drift in `daemon.log` without requiring a separate integration test.

If all three candidate transcripts are skipped, `RunMetaAnalysis` proceeds without
transcript evidence and Opus is instructed (via the meta prompt) to rely solely on the
rejected proposals and acceptance stats — and to emit a `pipeline_insight` rather than
a `prompt_improvement` if the evidence is insufficient.

Session config: `claude-opus-4-6`, agentic, `SettingSources: ""`, max 20 turns,
5-minute timeout, `Read,Grep` tools.

Output: a `prompt_improvement` proposal with:
- `target`: the prompt file path (e.g. `~/.cabrero/prompts/evaluator-v4.txt`)
- `change`: a concrete suggested edit (not "consider revising" — specific text to add,
  remove, or modify)
- `rationale`: the rejection pattern observed, citing specific proposal IDs and session IDs
- `citedUuids`: proposal IDs and session IDs used as evidence

Written via `WriteProposal(p, "meta")` to the standard proposal queue.

### New Proposal Types

```go
const (
    // existing
    TypeSkillImprovement  = "skill_improvement"
    TypeClaudeReview      = "claude_review"
    TypeClaudeAddition    = "claude_addition"
    TypeSkillScaffold     = "skill_scaffold"

    // new
    TypePromptImprovement = "prompt_improvement"  // meta-pipeline: prompt file edits
    TypePipelineInsight   = "pipeline_insight"    // pure-code observation, no change field
)
```

`TypePipelineInsight` is used when metrics are notable but below the LLM trigger
threshold. No `change` field, no apply step — informational only.

Both new types are added to the Curator's preservation list alongside `skill_scaffold`.
The Curator never synthesises or culls meta-proposals.

**TUI:** Proposal detail view shows a `META` badge for both new types. The pipeline
monitor gains a `METRICS` section showing acceptance rate, classifier FPR, and last
meta-run result for the active evaluator prompt version.

---

## Meta-Prompt Design

The meta prompt instructs Opus to:
1. Read the target prompt file
2. Read the provided rejected proposals and identify the common pattern (what did the
   evaluator propose that humans consistently rejected — too aggressive, wrong scope,
   wrong type, insufficient evidence?)
3. Read the CC session transcripts for the highest-turn rejected runs (to understand
   what evidence the evaluator was working from)
4. Produce a single, specific proposed edit to the prompt that addresses the pattern
5. Only propose changes with clear evidence — if the pattern is ambiguous, emit a
   `pipeline_insight` instead of a `prompt_improvement`

The prompt must explicitly instruct Opus not to speculate from insufficient data.

---

## Meta Prompt File

`~/.cabrero/prompts/meta-v1.txt` — written by `EnsureMetaPrompts()` on first meta run
if absent. Same pattern as `EnsureCuratorPrompts`.

---

## File Changes Summary

| File | Change |
|---|---|
| `internal/store/store.go` | `BlocklistEntry` type, timestamped `BlockSession`, `RotateBlocklist`, migration |
| `internal/pipeline/invoke.go` | Always-on session persistence for agentic calls, debug simplification |
| `internal/pipeline/runner.go` | Log warning when `digest.RawUnknown` is non-empty after `ParseSession` |
| `internal/pipeline/pipeline.go` | Add `CuratorCheckModel`, `ApplyModel`, `ChatModel`, `MetaModel`, `MetaRejectionRateThreshold`, `MetaClassifierFPRThreshold`, `MetaMinSamples`, `MetaCooldownDays`, `MetaMaxTurns`, `MetaTimeout` |
| `internal/pipeline/curator.go` | Use `cfg.CuratorCheckModel` |
| `internal/apply/apply.go` | `ArchiveOutcome` enum, `archivedAt`, typed `Archive`, add `model` param to `Blend` |
| `internal/tui/chat/model.go` + `stream.go` | `ChatConfig.Model`, use in `buildChatArgs` |
| `internal/tui/model.go` | Thread `ApplyModel` and `ChatModel` through |
| `internal/pipeline/run.go` | `AcceptanceStats`, `ListAcceptanceRateByPromptVersion`, update `GatherPipelineStatsFromSessions` |
| `internal/pipeline/meta.go` | `PipelineMetrics`, `ComputePipelineMetrics`, `RunMetaAnalysis` |
| `internal/pipeline/prompts.go` | `EnsureMetaPrompts`, meta prompt constants |
| `internal/daemon/meta.go` | `performMetaRun` |
| `internal/daemon/daemon.go` | `Config.MetaInterval`, meta ticker, blocklist rotation call |
| `internal/tui/pipeline/view.go` | Expand models section, add metrics section, META badge |
