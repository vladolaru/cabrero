# Daily Proposals Cleanup — Curator Design

**Date:** 2026-02-26
**Status:** Design complete, revised after decision-critic analysis — ready for implementation planning

## Problem

Cabrero generates 10–15 proposals per day from session analysis. Reviewing even 3–5
takes meaningful mental energy. After a week, the backlog reaches 60+; after a month,
it becomes unusable. The duplication pattern is severe: 14 proposals for a single
`CLAUDE.md` target, many addressing the same root cause (e.g. "read before edit"
appearing in 6 different proposals from different sessions).

The goal is to let the user wake up to a curated, small inbox — not a growing pile.

---

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Trigger | Automatic daemon, daily | User never thinks about it |
| Observability | Structured logs for compound self-improvement | Debug and measure effectiveness |
| Multi-proposal strategy | Type-aware (see below) | `claude_addition` needs synthesis; `skill_improvement` needs ranking; scaffolds are untouched |
| Target state check | Always check current file | Proposals already applied waste review time |
| Curator model | Sonnet (multi-proposal targets) + Haiku (single-proposal check) | Synthesis quality matters; Haiku sufficient for "already applied?" detection |
| Call structure | One agentic Sonnet call per multi-proposal target group (parallelized) + one batched non-agentic Haiku call for all single-proposal targets | Focused context per target → better synthesis; failure isolation; natural parallelism |
| Cleanup on startup | No — first run after 24h | New proposals deserve at least one day; restart shouldn't trigger cleanup |

---

## Architecture

### Two-stage Curator

**Stage 1 — Multi-proposal targets (agentic Sonnet, one call per target group)**

For each target with 2+ pending proposals:

- Reads current target file content via Read tool at session start
- Receives all proposal JSONs for that target as system context
- Applies type-aware strategy (see below)
- Outputs a `CuratorManifest`: per-proposal action + optional synthesized proposal

Calls are parallelized via the existing invoke semaphore — same pattern as
`processProjectBatch`.

**Stage 2 — Single-proposal targets (non-agentic Haiku, one batched call)**

All single-proposal targets in one `--print` call. Go code reads each target file
before the call and injects `currentFileContent` into the prompt directly. No tool
use needed. Output: array of `{proposalID, alreadyApplied, reason}`.

**Non-file target guard:** Some proposals have targets that are source names rather
than filesystem paths (e.g. `local-environment`, `write-moltres-snap`,
`pirategoat-tools:ingest-code-review`). Go code checks whether the target resolves
to a readable file before building the batch. Non-file targets are excluded from the
Haiku batch entirely — they are kept as-is and skipped by the Curator (no "already
applied?" check is possible without a file to compare against).

**Total CLI invocations per daily cleanup:**
- N agentic Sonnet calls (one per multi-proposal target, parallelized)
- 1 non-agentic Haiku call (all single-proposal targets batched)

---

## Type-Aware Strategy

| Proposal Type | Action |
|---------------|--------|
| `claude_addition` | Cluster then synthesize: LLM groups proposals by concern cluster (e.g. "Edit precondition failures" vs "search fumble patterns"), then synthesizes one new proposal per cluster (or zero for a cluster if already present in target). Originals archived. |
| `skill_improvement` | Rank and cull: LLM keeps best 1–2 by evidence specificity + friction severity. Rest archived. Kept proposals are unchanged — not rewritten. |
| `skill_scaffold` | Never touched by cleanup. Always preserved. |
| `claude_review` | Rank and cull (same as skill_improvement). |

**Intra-target clustering (claude_addition):**
The Curator must not wholesale merge all `claude_addition` proposals for a target into
one entry. A single target can contain 2–3 distinct concern clusters — merging across
clusters produces vague entries that cover multiple problems superficially rather than
addressing each with specificity. The Curator identifies clusters first, then synthesizes
within each cluster independently. Result: 14 proposals for `cabrero/CLAUDE.md` may
become 2–3 distinct synthesized proposals, not 1.

**"Already applied" detection (all types):**
Semantic equivalence, not word-for-word match. If the target file already contains the
substance of the proposed change, the proposal is auto-rejected regardless of type.

---

## Data Model

### CuratorManifest (Curator output per target group)

```go
type CuratorManifest struct {
    Target    string              `json:"target"`
    Decisions []CuratorDecision   `json:"decisions"`
    Clusters  []CuratorCluster    `json:"clusters,omitempty"` // one per concern cluster (claude_addition only)
}

// CuratorCluster represents one synthesized concern cluster within a target.
// For claude_addition: multiple proposals addressing the same root cause → one new proposal.
type CuratorCluster struct {
    ClusterName string    `json:"clusterName"` // e.g. "Edit precondition failures"
    SourceIDs   []string  `json:"sourceIds"`   // proposal IDs contributing to this cluster
    Synthesis   *Proposal `json:"synthesis,omitempty"` // nil if all already applied
}

type CuratorDecision struct {
    ProposalID   string `json:"proposalId"`
    Action       string `json:"action"`        // "keep" | "synthesize" | "cull" | "auto-reject"
    Reason       string `json:"reason"`        // human-readable, stored in archive
    SupersededBy string `json:"supersededBy,omitempty"` // ID of winning/synthesized proposal
}
```

### CleanupRecord (append-only log entry)

```go
type CleanupRecord struct {
    Timestamp       time.Time         `json:"timestamp"`
    Duration        time.Duration     `json:"duration"`
    ProposalsBefore int               `json:"proposalsBefore"`
    ProposalsAfter  int               `json:"proposalsAfter"`
    Decisions       []CuratorDecision `json:"decisions"`    // all decisions, all targets
    CuratorUsage    []InvocationUsage `json:"curatorUsage"` // one per Sonnet call
    CheckUsage      *InvocationUsage  `json:"checkUsage,omitempty"` // the Haiku batch call
    Error           string            `json:"error,omitempty"`
}
```

Reuses existing `InvocationUsage` struct — token counts, cost, turns — so TUI token
reporting works without new rendering code.

**Log file:** `~/.cabrero/cleanup_history.jsonl` (append-only, same rotation pattern
as `run_history.jsonl` — 90-day rotation on daemon startup).

### Archive reason strings

Written to each archived proposal's `archiveReason` field:

- `"auto-culled: already applied to target"`
- `"auto-culled: superseded by <proposalID>"`
- `"auto-culled: low signal — <curator reason>"`
- `"auto-culled: synthesized into <proposalID>"`

### Synthesized proposal provenance

The `rationale` field of a synthesized proposal includes:

```
Synthesized from N proposals (sessions: abc123…, def456…) by daily cleanup on 2026-02-26.
[original distilled rationale]
```

---

## Daemon Integration

### Config

```go
type Config struct {
    // existing fields unchanged
    CleanupInterval time.Duration // default 24h
}
```

### Daemon loop

Third ticker alongside `pollTicker` and `staleTicker`:

```go
cleanupTicker := time.NewTicker(d.config.CleanupInterval)
defer cleanupTicker.Stop()

for {
    select {
    case <-ctx.Done():       return nil
    case <-pollTicker.C:     d.processQueued(ctx)
    case <-staleTicker.C:    d.scanStale()
    case <-cleanupTicker.C:  d.performCleanup(ctx)
    }
}
```

### performCleanup sketch

```go
func (d *Daemon) performCleanup(ctx context.Context) {
    proposals := pipeline.ListProposals()
    // separate multi-proposal targets from single-proposal targets
    // run batched Haiku check for single-proposal targets
    // run parallelized Sonnet Curator for each multi-proposal target group
    // apply decisions: Archive with reason, WriteProposal for synthesis
    // append CleanupRecord to cleanup_history.jsonl
    // notify: "Cleanup: N archived, M synthesized, K kept"
}
```

### Semaphore coordination

Curator calls acquire the same invoke semaphore slots as pipeline stages — cleanup
won't run concurrently with active session processing and vice versa.

### Daemon log entries

```
[INFO] cleanup: starting (64 proposals across 18 targets)
[INFO] cleanup: target cabrero/CLAUDE.md — 14 proposals → 1 synthesized (13 archived)
[INFO] cleanup: target ~/.claude/CLAUDE.md — 11 proposals → 3 kept (8 archived: 5 already applied, 3 low signal)
[INFO] cleanup: complete in 47s — 64→12 proposals (42 archived, 10 already applied, 2 synthesized new)
```

---

## TUI Integration

**Synthesized proposals** appear in the dashboard as regular proposals — no special
treatment. Provenance is visible in the `rationale` field. The user reviews them
identically to evaluator-generated proposals.

**Pipeline Activity section** gets a new `"cleanup"` source value for `PipelineRun`
rows, rendered alongside session processing rows:

```
CLEANUP  2026-02-26 06:00  64→12  47s  $0.08  42 archived
```

`ListPipelineRunsFromHistory` is extended to load from `cleanup_history.jsonl` in
addition to `run_history.jsonl`. No new rendering code — `Source: "cleanup"` formats
distinctly via the existing source display path.

**Curator quality signal — reviewable archived decisions:**
Automated archival without per-decision review creates an invisible filter. To let
the user spot-check Curator decisions without reviewing everything, the archived
proposals view (accessible from the TUI) surfaces the `archiveReason` field for
auto-culled proposals alongside normal rejected ones. A user who suspects the Curator
made a wrong call can inspect recent archived proposals, see the reason string (e.g.
`"auto-culled: already applied to target"` or `"auto-culled: low signal — vague
rationale, no specific UUID cited"`), and manually re-instate a proposal if needed
(future CLI: `cabrero reinstate <id>`). No new TUI panel required for initial
implementation — the existing archived view with reason strings is sufficient.

---

## Prompt Design

### Curator prompt (agentic Sonnet — per target group)

**System context includes:**
- All proposal JSONs for this target (full content)
- Current content of the target file (injected via initial Read tool call)
- Type-aware strategy rules
- Output schema for `CuratorManifest`

**Key instructions:**
- For `claude_addition`: first identify distinct concern clusters among the proposals
  (e.g. "Edit precondition failures", "search fumble patterns", "build test failures").
  Then synthesize one proposal per cluster, or zero for a cluster if already present
  in the target file. The `change` field must be a concrete, actionable CLAUDE.md entry
  — not a summary. Do not merge proposals across different clusters.
- For `skill_improvement`: rank by specificity of evidence + severity of friction.
  Keep at most 2. Kept proposals are returned unchanged — do not rewrite them.
- "Already applied" means the target file already contains the substance of the
  proposed change. Semantic equivalence, not literal match.
- Output valid JSON only — no prose, no markdown fencing.

### Haiku batch check prompt (non-agentic — all single-proposal targets)

Single `--print` call. System context: array of
`{proposalID, target, currentFileContent, proposedChange}` injected by Go code.

Output: `[{proposalID, alreadyApplied: bool, reason: string}]`

No tool use — file contents are read by Go before the call and injected directly.

---

## Store Layout (additions)

```
~/.cabrero/
  cleanup_history.jsonl   # append-only cleanup run records (90-day rotation)
  proposals/
    {proposalID}.json     # unchanged — synthesized proposals written here too
    archived/
      {proposalID}.json   # archiveReason field added by cleanup
```

No new subdirectories. No index files. Same atomic write invariants as all other store
writes.

---

## Complementary Workstream — Evaluator Deduplication

The Curator is a compensating control: it cleans up proposals *after* they accumulate.
The root cause is that the Evaluator emits a new proposal for every session that hits
a recurring pattern, without checking whether a proposal for the same target and
concern already exists in the pending store.

**Parallel intervention (design separately):**
Before writing a new proposal, the Evaluator reads `~/.cabrero/proposals/` and checks
whether a semantically similar proposal for the same target already exists. If yes, the
Evaluator skips emission. This prevents the backlog from growing in the first place,
reducing Curator load as the system matures.

The two mechanisms are complementary:
- **Curator** clears the existing backlog and handles bursts (e.g. several sessions
  in quick succession before any review)
- **Evaluator deduplication** prevents steady-state accumulation once implemented

Neither replaces the other. Build the Curator first (immediate backlog relief), then
add Evaluator deduplication as a follow-on to reduce Curator work over time.

---

## Out of Scope

- Manual `cabrero cleanup` CLI command (can be added later once the daemon flow is proven)
- Per-proposal age threshold (cleanup culls by signal quality, not age)
- Cross-target consolidation (e.g., merging proposals for `cabrero/CLAUDE.md` and
  `~/.claude/CLAUDE.md` that address the same root cause) — deferred; requires
  understanding which CLAUDE.md is the right home for a given pattern
