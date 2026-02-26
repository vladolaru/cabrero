# Proposals Curator Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a daily automated Curator stage to the daemon that cleans up the proposal backlog — synthesizing duplicate `claude_addition` proposals per concern cluster, culling redundant `skill_improvement` proposals, and auto-rejecting proposals whose changes are already present in the target file.

**Architecture:** Two-stage cleanup runs daily via a third daemon ticker. Stage 1: one agentic Sonnet call per multi-proposal target (parallelized via existing semaphore) performs cluster-aware synthesis for `claude_addition` and rank-and-cull for `skill_improvement`. Stage 2: one batched non-agentic Haiku call checks all single-proposal targets for "already applied" status. Results are logged to `cleanup_history.jsonl` and surfaced in the TUI Pipeline Activity section.

**Tech Stack:** Go, existing `invokeClaude` + `cleanLLMJSON` + `AtomicWrite` + `apply.Archive` infrastructure, `claude-sonnet-4-6` (Curator), `claude-haiku-4-5` (check), new prompt files in `~/.cabrero/prompts/`.

**Design document:** `docs/plans/2026-02-26-proposals-curator-design.md`

---

### Task 1: Curator data model

**Files:**
- Create: `internal/pipeline/curator.go`
- Create: `internal/pipeline/curator_test.go`

**Step 1: Write the failing test**

```go
// internal/pipeline/curator_test.go
package pipeline

import (
    "encoding/json"
    "testing"
)

func TestCuratorDecisionRoundtrip(t *testing.T) {
    d := CuratorDecision{
        ProposalID:   "prop-abc123-1",
        Action:       "cull",
        Reason:       "superseded by prop-abc123-2",
        SupersededBy: "prop-abc123-2",
    }
    data, err := json.Marshal(d)
    if err != nil {
        t.Fatal(err)
    }
    var got CuratorDecision
    if err := json.Unmarshal(data, &got); err != nil {
        t.Fatal(err)
    }
    if got != d {
        t.Errorf("got %+v, want %+v", got, d)
    }
}

func TestCuratorManifestRoundtrip(t *testing.T) {
    change := "Add entry: always verify X before Y."
    m := CuratorManifest{
        Target: "/Users/test/.claude/CLAUDE.md",
        Decisions: []CuratorDecision{
            {ProposalID: "prop-abc-1", Action: "synthesize", Reason: "merged into cluster", SupersededBy: "prop-curator-1"},
            {ProposalID: "prop-abc-2", Action: "synthesize", Reason: "merged into cluster", SupersededBy: "prop-curator-1"},
        },
        Clusters: []CuratorCluster{
            {
                ClusterName: "Edit precondition failures",
                SourceIDs:   []string{"prop-abc-1", "prop-abc-2"},
                Synthesis: &Proposal{
                    ID:        "prop-curator-1",
                    Type:      "claude_addition",
                    Confidence: "high",
                    Target:    "/Users/test/.claude/CLAUDE.md",
                    Change:    &change,
                    Rationale: "Synthesized from 2 proposals.",
                },
            },
        },
    }
    data, err := json.MarshalIndent(m, "", "  ")
    if err != nil {
        t.Fatal(err)
    }
    var got CuratorManifest
    if err := json.Unmarshal(data, &got); err != nil {
        t.Fatal(err)
    }
    if got.Target != m.Target {
        t.Errorf("Target: got %q, want %q", got.Target, m.Target)
    }
    if len(got.Clusters) != 1 {
        t.Fatalf("Clusters: got %d, want 1", len(got.Clusters))
    }
    if got.Clusters[0].Synthesis == nil {
        t.Fatal("Clusters[0].Synthesis is nil")
    }
}

func TestCheckDecisionRoundtrip(t *testing.T) {
    d := CheckDecision{ProposalID: "prop-abc-1", AlreadyApplied: true, Reason: "entry already present in file"}
    data, _ := json.Marshal(d)
    var got CheckDecision
    if err := json.Unmarshal(data, &got); err != nil {
        t.Fatal(err)
    }
    if got != d {
        t.Errorf("got %+v, want %+v", got, d)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/vladolaru/Work/a8c/cabrero
go test ./internal/pipeline/... -run TestCurator -v
```
Expected: compile error — `CuratorDecision`, `CuratorManifest`, `CuratorCluster`, `CheckDecision` undefined.

**Step 3: Write minimal implementation**

```go
// internal/pipeline/curator.go
package pipeline

// CuratorDecision is one per-proposal action from the Curator.
type CuratorDecision struct {
    ProposalID   string `json:"proposalId"`
    Action       string `json:"action"`        // "keep" | "synthesize" | "cull" | "auto-reject"
    Reason       string `json:"reason"`
    SupersededBy string `json:"supersededBy,omitempty"`
}

// CuratorCluster is one synthesized concern group for claude_addition targets.
// The Curator identifies distinct concern clusters within a target's proposals,
// then synthesizes one new proposal per cluster rather than merging all proposals
// into one. This prevents vague entries that cover multiple problems superficially.
type CuratorCluster struct {
    ClusterName string    `json:"clusterName"`
    SourceIDs   []string  `json:"sourceIds"`
    Synthesis   *Proposal `json:"synthesis,omitempty"` // nil if all already applied
}

// CuratorManifest is the Curator's output for a single target group.
type CuratorManifest struct {
    Target    string            `json:"target"`
    Decisions []CuratorDecision `json:"decisions"`
    Clusters  []CuratorCluster  `json:"clusters,omitempty"` // claude_addition only
}

// CheckDecision is one per-proposal result from the Haiku "already applied?" batch check.
type CheckDecision struct {
    ProposalID     string `json:"proposalId"`
    AlreadyApplied bool   `json:"alreadyApplied"`
    Reason         string `json:"reason"`
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/pipeline/... -run TestCurator -v
```
Expected: PASS — 3 tests pass.

**Step 5: Commit**

```bash
git add internal/pipeline/curator.go internal/pipeline/curator_test.go
git commit -m "feat(pipeline): add Curator data model types"
```

---

### Task 2: Cleanup history persistence

**Files:**
- Create: `internal/pipeline/cleanup_history.go`
- Create: `internal/pipeline/cleanup_history_test.go`

**Step 1: Write the failing test**

```go
// internal/pipeline/cleanup_history_test.go
package pipeline

import (
    "os"
    "path/filepath"
    "testing"
    "time"
)

func TestCleanupHistoryAppendAndRead(t *testing.T) {
    dir := t.TempDir()
    origPath := cleanupHistoryPath
    cleanupHistoryPath = func() string { return filepath.Join(dir, "cleanup_history.jsonl") }
    defer func() { cleanupHistoryPath = origPath }()

    rec := CleanupRecord{
        Timestamp:       time.Now().Truncate(time.Second),
        Duration:        5 * time.Second,
        ProposalsBefore: 10,
        ProposalsAfter:  3,
        Decisions: []CuratorDecision{
            {ProposalID: "prop-abc-1", Action: "cull", Reason: "already applied"},
        },
    }

    if err := AppendCleanupHistory(rec); err != nil {
        t.Fatal(err)
    }

    records, err := ReadCleanupHistory()
    if err != nil {
        t.Fatal(err)
    }
    if len(records) != 1 {
        t.Fatalf("got %d records, want 1", len(records))
    }
    if records[0].ProposalsBefore != 10 {
        t.Errorf("ProposalsBefore: got %d, want 10", records[0].ProposalsBefore)
    }
    if len(records[0].Decisions) != 1 {
        t.Errorf("Decisions: got %d, want 1", len(records[0].Decisions))
    }
}

func TestCleanupHistoryMissingFileReturnsNil(t *testing.T) {
    dir := t.TempDir()
    origPath := cleanupHistoryPath
    cleanupHistoryPath = func() string { return filepath.Join(dir, "no_such_file.jsonl") }
    defer func() { cleanupHistoryPath = origPath }()

    records, err := ReadCleanupHistory()
    if err != nil {
        t.Fatal(err)
    }
    if records != nil {
        t.Errorf("got %v, want nil", records)
    }
}

func TestRotateCleanupHistory(t *testing.T) {
    dir := t.TempDir()
    origPath := cleanupHistoryPath
    cleanupHistoryPath = func() string { return filepath.Join(dir, "cleanup_history.jsonl") }
    defer func() { cleanupHistoryPath = origPath }()

    old := CleanupRecord{Timestamp: time.Now().Add(-100 * 24 * time.Hour)}
    new := CleanupRecord{Timestamp: time.Now()}
    _ = AppendCleanupHistory(old)
    _ = AppendCleanupHistory(new)

    removed, err := RotateCleanupHistory(90 * 24 * time.Hour)
    if err != nil {
        t.Fatal(err)
    }
    if removed != 1 {
        t.Errorf("removed: got %d, want 1", removed)
    }
    records, _ := ReadCleanupHistory()
    if len(records) != 1 {
        t.Errorf("remaining: got %d, want 1", len(records))
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/pipeline/... -run TestCleanupHistory -v
```
Expected: compile error — `CleanupRecord`, `AppendCleanupHistory`, etc. undefined.

**Step 3: Write minimal implementation**

```go
// internal/pipeline/cleanup_history.go
package pipeline

import (
    "bufio"
    "encoding/json"
    "os"
    "sync"
    "time"

    "github.com/vladolaru/cabrero/internal/store"
    "path/filepath"
)

// CleanupRecord captures the full diagnostic context of a single cleanup run.
type CleanupRecord struct {
    Timestamp       time.Time         `json:"timestamp"`
    DurationNs      int64             `json:"duration_ns"`
    ProposalsBefore int               `json:"proposals_before"`
    ProposalsAfter  int               `json:"proposals_after"`
    Decisions       []CuratorDecision `json:"decisions"`
    CuratorUsage    []InvocationUsage `json:"curator_usage,omitempty"` // one per Sonnet call
    CheckUsage      *InvocationUsage  `json:"check_usage,omitempty"`   // the Haiku batch call
    Error           string            `json:"error,omitempty"`
}

// Duration returns the cleanup run duration.
func (r CleanupRecord) Duration() time.Duration {
    return time.Duration(r.DurationNs)
}

var cleanupHistoryMu sync.Mutex

// cleanupHistoryPath returns the path to the cleanup history JSONL file.
// Declared as a variable so tests can override it.
var cleanupHistoryPath = func() string {
    return filepath.Join(store.Root(), "cleanup_history.jsonl")
}

// AppendCleanupHistory appends a single cleanup record to the JSONL file.
// Thread-safe. Best-effort — callers should not fail the cleanup run on error.
func AppendCleanupHistory(rec CleanupRecord) error {
    cleanupHistoryMu.Lock()
    defer cleanupHistoryMu.Unlock()

    data, err := json.Marshal(rec)
    if err != nil {
        return err
    }
    data = append(data, '\n')

    f, err := os.OpenFile(cleanupHistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
    if err != nil {
        return err
    }
    defer f.Close()

    _, err = f.Write(data)
    return err
}

// ReadCleanupHistory reads all cleanup records from the JSONL file.
// Returns nil, nil for a missing or empty file. Malformed lines are skipped.
func ReadCleanupHistory() ([]CleanupRecord, error) {
    cleanupHistoryMu.Lock()
    defer cleanupHistoryMu.Unlock()
    return readCleanupHistoryFrom(cleanupHistoryPath())
}

func readCleanupHistoryFrom(path string) ([]CleanupRecord, error) {
    f, err := os.Open(path)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, err
    }
    defer f.Close()

    var records []CleanupRecord
    scanner := bufio.NewScanner(f)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
    for scanner.Scan() {
        line := scanner.Bytes()
        if len(line) == 0 {
            continue
        }
        var rec CleanupRecord
        if err := json.Unmarshal(line, &rec); err != nil {
            continue // skip malformed lines
        }
        records = append(records, rec)
    }
    if err := scanner.Err(); err != nil {
        return records, err
    }
    return records, nil
}

// RotateCleanupHistory removes records older than maxAge from the history file.
// Returns the count of removed records.
func RotateCleanupHistory(maxAge time.Duration) (int, error) {
    cleanupHistoryMu.Lock()
    defer cleanupHistoryMu.Unlock()

    path := cleanupHistoryPath()
    records, err := readCleanupHistoryFrom(path)
    if err != nil || len(records) == 0 {
        return 0, err
    }

    cutoff := time.Now().Add(-maxAge)
    var kept []CleanupRecord
    removed := 0
    for _, rec := range records {
        if rec.Timestamp.Before(cutoff) {
            removed++
        } else {
            kept = append(kept, rec)
        }
    }
    if removed == 0 {
        return 0, nil
    }

    var data []byte
    for _, rec := range kept {
        line, err := json.Marshal(rec)
        if err != nil {
            continue
        }
        data = append(data, line...)
        data = append(data, '\n')
    }
    if err := store.AtomicWrite(path, data, 0o644); err != nil {
        return 0, err
    }
    return removed, nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/pipeline/... -run TestCleanupHistory -v
```
Expected: PASS — 3 tests pass.

**Step 5: Commit**

```bash
git add internal/pipeline/cleanup_history.go internal/pipeline/cleanup_history_test.go
git commit -m "feat(pipeline): add cleanup history persistence (CleanupRecord, JSONL append/read/rotate)"
```

---

### Task 3: Curator prompts and EnsureCuratorPrompts

**Files:**
- Modify: `internal/pipeline/prompts.go`
- Modify: `internal/pipeline/prompts_test.go` (check curator files are written)

**Step 1: Write the failing test**

In `internal/pipeline/prompts_test.go`, find the existing `TestEnsurePrompts` test and add:

```go
func TestEnsureCuratorPrompts(t *testing.T) {
    dir := t.TempDir()
    // Override store root temporarily — see how existing EnsurePrompts tests do this.
    // The test should verify that curator-v1.txt and curator-check-v1.txt are created.
    origRoot := store.RootOverrideForTest(dir)
    defer store.ResetRootOverrideForTest(origRoot)

    if err := EnsureCuratorPrompts(); err != nil {
        t.Fatal(err)
    }

    for _, f := range []string{curatorPromptFile, curatorCheckPromptFile} {
        path := filepath.Join(dir, "prompts", f)
        if _, err := os.Stat(path); err != nil {
            t.Errorf("expected prompt file %s to exist: %v", f, err)
        }
    }
}
```

> **Note:** Check how existing `prompts_test.go` overrides the store root — mirror that pattern exactly.

**Step 2: Run test to verify it fails**

```bash
go test ./internal/pipeline/... -run TestEnsureCuratorPrompts -v
```
Expected: compile error — `curatorPromptFile`, `curatorCheckPromptFile`, `EnsureCuratorPrompts` undefined.

**Step 3: Add constants and EnsureCuratorPrompts to prompts.go**

Add to the top of `internal/pipeline/prompts.go` after the existing prompt file constants:

```go
const (
    curatorPromptFile      = "curator-v1.txt"
    curatorCheckPromptFile = "curator-check-v1.txt"
)
```

Add `EnsureCuratorPrompts()` function — same pattern as `EnsurePrompts()` but for curator files:

```go
// EnsureCuratorPrompts writes default curator prompt files if they don't already exist.
func EnsureCuratorPrompts() error {
    dir := filepath.Join(store.Root(), "prompts")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
    prompts := map[string]string{
        curatorPromptFile:      defaultCuratorPrompt,
        curatorCheckPromptFile: defaultCuratorCheckPrompt,
    }
    for filename, content := range prompts {
        path := filepath.Join(dir, filename)
        if _, err := os.Stat(path); err == nil {
            continue
        }
        if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
            return fmt.Errorf("writing %s: %w", filename, err)
        }
    }
    return nil
}
```

Add prompt content constants. These go at the bottom of `prompts.go`. Keep them focused and concise — the curator prompts are simpler than the classifier/evaluator prompts:

```go
const defaultCuratorPrompt = `You are a proposal curator for Cabrero. Your job is to clean up a backlog of pending improvement proposals for a single target file by identifying concern clusters, detecting already-applied changes, and producing a CuratorManifest.

## Input

You will receive:
1. A list of pending proposals (JSON array) all targeting the same file
2. The current content of the target file (read it yourself using the Read tool)

## Strategy by proposal type

**claude_addition:**
1. Read the current target file.
2. Identify distinct concern clusters among the proposals. Each cluster groups proposals that address the same root cause (e.g. "Edit precondition failures", "search fumble patterns"). Do NOT merge proposals from different clusters.
3. For each cluster: if the target file already contains the substance of the proposed changes (semantic equivalence, not literal match), set synthesis to null and mark all proposals as "auto-reject" with reason "already applied to target". Otherwise, synthesize one new Proposal that distills the cluster's signal into a single concrete, actionable CLAUDE.md entry.
4. Mark all original proposals as "synthesize" (if a synthesis was produced) or "auto-reject" (if already applied).

**skill_improvement / claude_review:**
1. Read the current target file.
2. Check if any proposals are already addressed by the current file state. If so, mark them "auto-reject" with reason "already applied to target".
3. Among remaining proposals, rank by: specificity of evidence > severity of friction described.
4. Keep the top 1-2. Mark the rest as "cull" with reason "superseded by <winner-id>" or "lower signal than kept proposals".
5. Kept proposals: include them in decisions with action "keep". Do NOT rewrite them.

**skill_scaffold:**
Never touch scaffold proposals. If any are present, mark them "keep" with reason "scaffold always preserved".

## Synthesized proposal format

A synthesized Proposal must have:
- id: "prop-curator-<target-hash-4chars>-<cluster-index>" (e.g. "prop-curator-a1b2-1")
- type: same as source proposals
- confidence: "high" if 3+ source proposals; "medium" otherwise
- target: same target as input proposals
- change: a concrete, actionable entry — not a summary. For claude_addition, write the actual CLAUDE.md rule text.
- rationale: "Synthesized from N proposals (sessions: <short-ids>) by daily cleanup.\n<distilled rationale in 2-3 sentences>"
- citedUuids: [] (empty — cross-session synthesis, no single session UUIDs)

## Output format

Output ONLY valid JSON. No markdown fences, no preamble.

Schema:
{
  "target": "string",
  "decisions": [
    {"proposalId": "string", "action": "keep|synthesize|cull|auto-reject", "reason": "string", "supersededBy": "string (optional)"}
  ],
  "clusters": [
    {
      "clusterName": "string",
      "sourceIds": ["string"],
      "synthesis": <Proposal object or null>
    }
  ]
}

The clusters array is only needed for claude_addition. Omit it for skill_improvement/claude_review/skill_scaffold.

## Budget

You have a budget of {{MAX_TURNS}} tool-call rounds. Read the target file first, then output the manifest. If you exhaust your budget, output your best manifest with what you have.
`

const defaultCuratorCheckPrompt = `You are a proposal checker for Cabrero. For each proposal in the input, determine whether its proposed change is already present in the current target file content.

## Input

A JSON array of check items:
[
  {
    "proposalId": "string",
    "target": "string (file path)",
    "currentFileContent": "string (full file content, may be empty if file does not exist)",
    "proposedChange": "string (the proposed change text)"
  }
]

## Task

For each item: determine if the target file already contains the substance of the proposed change. Use semantic equivalence — a paraphrase counts as already present. Word-for-word match is not required.

If currentFileContent is empty, the file does not exist — the change is NOT already applied.

## Output format

Output ONLY valid JSON array. No markdown fences, no preamble.

[
  {"proposalId": "string", "alreadyApplied": true|false, "reason": "string (brief explanation)"}
]
`
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/pipeline/... -run TestEnsureCuratorPrompts -v
```
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/pipeline/prompts.go internal/pipeline/prompts_test.go
git commit -m "feat(pipeline): add curator prompt files and EnsureCuratorPrompts"
```

---

### Task 4: PipelineConfig extension for Curator

**Files:**
- Modify: `internal/pipeline/pipeline.go`

**Step 1: Write the failing test**

```go
// Add to existing pipeline_test.go or create internal/pipeline/pipeline_test.go
func TestDefaultPipelineConfigHasCuratorFields(t *testing.T) {
    cfg := DefaultPipelineConfig()
    if cfg.CuratorModel == "" {
        t.Error("CuratorModel should not be empty")
    }
    if cfg.CuratorTimeout == 0 {
        t.Error("CuratorTimeout should not be zero")
    }
    if cfg.CuratorMaxTurns == 0 {
        t.Error("CuratorMaxTurns should not be zero")
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/pipeline/... -run TestDefaultPipelineConfigHasCuratorFields -v
```
Expected: compile error — `CuratorModel`, `CuratorTimeout`, `CuratorMaxTurns` undefined on `PipelineConfig`.

**Step 3: Add fields to PipelineConfig and DefaultPipelineConfig**

In `internal/pipeline/pipeline.go`, add to `PipelineConfig`:

```go
// Curator stage (daily cleanup).
CuratorModel    string
CuratorMaxTurns int
CuratorTimeout  time.Duration
CuratorCheckTimeout time.Duration // for the Haiku batch check
```

In `DefaultPipelineConfig()`, add:

```go
CuratorModel:        DefaultEvaluatorModel, // sonnet — same as evaluator
CuratorMaxTurns:     15,
CuratorTimeout:      5 * time.Minute,
CuratorCheckTimeout: 2 * time.Minute,
```

Add constant near the top of `pipeline.go`:

```go
// DefaultClassifierModel and DefaultEvaluatorModel are already defined.
// Add:
// (no new constant needed — curator reuses DefaultEvaluatorModel)
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/pipeline/... -run TestDefaultPipelineConfigHasCuratorFields -v
```
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/pipeline/pipeline.go
git commit -m "feat(pipeline): add CuratorModel/Timeout/MaxTurns to PipelineConfig"
```

---

### Task 5: Haiku batch check — RunCuratorCheck

**Files:**
- Modify: `internal/pipeline/curator.go`
- Modify: `internal/pipeline/curator_test.go`

**Step 1: Write the failing test**

```go
// Add to internal/pipeline/curator_test.go
func TestIsFileTarget(t *testing.T) {
    cases := []struct {
        target string
        want   bool
    }{
        {"/Users/foo/.claude/CLAUDE.md", true},
        {"~/claude/skills/foo.md", true},
        {"local-environment", false},
        {"write-moltres-snap", false},
        {"pirategoat-tools:ingest-code-review", false},
        {"", false},
    }
    for _, c := range cases {
        got := IsFileTarget(c.target)
        if got != c.want {
            t.Errorf("IsFileTarget(%q) = %v, want %v", c.target, got, c.want)
        }
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/pipeline/... -run TestIsFileTarget -v
```
Expected: compile error — `IsFileTarget` undefined.

**Step 3: Fix cleanLLMJSON to handle JSON arrays**

`cleanLLMJSON` in `invoke.go` only handles `{...}` objects — it falls through to a brace-scan that would strip the outer `[...]` from array output. The Haiku check returns `[...]`. Add array support:

In `internal/pipeline/invoke.go`, find `cleanLLMJSON` and update the early-return check:

```go
// Before (line ~391):
// If it already starts with '{', we're done.
if strings.HasPrefix(s, "{") {
    return s
}

// After:
// If it already starts with '{' or '[', we're done.
if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
    return s
}
```

Also update the last-resort brace scan to handle `[` as a valid start character:

```go
// Before:
braceStart := strings.Index(s, "{")
if braceStart == -1 {
    return s
}
braceEnd := strings.LastIndex(s, "}")
if braceEnd == -1 || braceEnd < braceStart {
    return s
}
return s[braceStart : braceEnd+1]

// After:
// Find the first JSON value start: '{' (object) or '[' (array).
braceStart := strings.IndexAny(s, "{[")
if braceStart == -1 {
    return s
}
openChar := s[braceStart]
closeChar := byte('}')
if openChar == '[' {
    closeChar = ']'
}
braceEnd := strings.LastIndexByte(s, closeChar)
if braceEnd == -1 || braceEnd < braceStart {
    return s
}
return s[braceStart : braceEnd+1]
```

Run existing `cleanLLMJSON` tests to confirm no regressions:

```bash
go test ./internal/pipeline/... -run TestClean -v
```

If no existing tests cover this function, add a quick test in `curator_test.go`:

```go
func TestCleanLLMJSONArray(t *testing.T) {
    input := "```json\n[{\"proposalId\": \"p1\", \"alreadyApplied\": false}]\n```"
    got := cleanLLMJSON(input)
    if !strings.HasPrefix(got, "[") {
        t.Errorf("expected array, got: %s", got)
    }
    var out []CheckDecision
    if err := json.Unmarshal([]byte(got), &out); err != nil {
        t.Errorf("unmarshal failed: %v", err)
    }
}
```

Run:
```bash
go test ./internal/pipeline/... -run TestCleanLLMJSONArray -v
```
Expected: PASS.

**Step 5: Add IsFileTarget and RunCuratorCheck skeleton**

Add to `internal/pipeline/curator.go`:

```go
import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/vladolaru/cabrero/internal/store"
)

// IsFileTarget returns true if target looks like a filesystem path
// (starts with "/" or "~/" or contains a path separator) rather than
// a source name like "local-environment" or "pirategoat-tools:foo".
func IsFileTarget(target string) bool {
    if target == "" {
        return false
    }
    if strings.HasPrefix(target, "/") || strings.HasPrefix(target, "~/") {
        return true
    }
    // Contains path separator — likely a relative path.
    if strings.Contains(target, string(filepath.Separator)) {
        return true
    }
    return false
}

// CheckItem is one entry in the Haiku batch check prompt.
type CheckItem struct {
    ProposalID         string `json:"proposalId"`
    Target             string `json:"target"`
    CurrentFileContent string `json:"currentFileContent"`
    ProposedChange     string `json:"proposedChange"`
}

// RunCuratorCheck sends all single-proposal file-target proposals to Haiku
// in one non-agentic --print call to check if their changes are already applied.
// Returns a slice of CheckDecision (same length and order as items) and usage.
// proposals must only contain file-target proposals (IsFileTarget == true).
func RunCuratorCheck(proposals []ProposalWithSession, cfg PipelineConfig) ([]CheckDecision, *ClaudeResult, error) {
    if len(proposals) == 0 {
        return nil, nil, nil
    }

    if err := EnsureCuratorPrompts(); err != nil {
        return nil, nil, fmt.Errorf("ensuring curator prompts: %w", err)
    }

    systemPrompt, err := readPromptTemplate(curatorCheckPromptFile)
    if err != nil {
        return nil, nil, fmt.Errorf("reading curator check prompt: %w", err)
    }

    // Build check items — read each target file.
    items := make([]CheckItem, 0, len(proposals))
    for _, pw := range proposals {
        p := pw.Proposal
        change := ""
        if p.Change != nil {
            change = *p.Change
        } else if p.FlaggedEntry != nil {
            change = *p.FlaggedEntry
        }
        content := readFileOrEmpty(p.Target)
        items = append(items, CheckItem{
            ProposalID:         p.ID,
            Target:             p.Target,
            CurrentFileContent: content,
            ProposedChange:     change,
        })
    }

    inputJSON, err := json.Marshal(items)
    if err != nil {
        return nil, nil, fmt.Errorf("marshaling check items: %w", err)
    }

    cr, err := invokeClaude(claudeConfig{
        Model:        cfg.classifierModel(), // Haiku
        SystemPrompt: systemPrompt,
        Agentic:      false,
        Stdin:        strings.NewReader(string(inputJSON)),
        Timeout:      cfg.CuratorCheckTimeout,
    })
    if err != nil {
        return nil, cr, fmt.Errorf("curator check invocation failed: %w", err)
    }

    cleaned := cleanLLMJSON(cr.Result)
    // The output is a JSON array, not object — handle separately.
    var decisions []CheckDecision
    if err := json.Unmarshal([]byte(cleaned), &decisions); err != nil {
        return nil, cr, fmt.Errorf("parsing curator check output: %w", err)
    }
    return decisions, cr, nil
}

// readFileOrEmpty reads a file, expanding "~/" prefix, returning "" on error.
func readFileOrEmpty(target string) string {
    path := target
    if strings.HasPrefix(path, "~/") {
        home, err := os.UserHomeDir()
        if err == nil {
            path = filepath.Join(home, path[2:])
        }
    }
    data, err := os.ReadFile(path)
    if err != nil {
        return ""
    }
    return string(data)
}
```

Note: the check stage should use `cfg.ClassifierModel` (Haiku) since `CuratorModel` is Sonnet. Check `internal/pipeline/evaluator.go` for the pattern of how models are referenced and adjust accordingly.

**Step 6: Run test to verify it passes**

```bash
go test ./internal/pipeline/... -run TestIsFileTarget -v
```
Expected: PASS.

**Step 7: Compile check**

```bash
go build ./internal/pipeline/...
```
Expected: no errors.

**Step 8: Commit**

```bash
git add internal/pipeline/curator.go internal/pipeline/curator_test.go internal/pipeline/invoke.go
git commit -m "feat(pipeline): add IsFileTarget, RunCuratorCheck, and array support in cleanLLMJSON"
```

---

### Task 6: Sonnet Curator group call — RunCuratorGroup

**Files:**
- Modify: `internal/pipeline/curator.go`
- Modify: `internal/pipeline/curator_test.go`

**Step 1: Write the failing test**

```go
// Add to internal/pipeline/curator_test.go
func TestParseCuratorManifest(t *testing.T) {
    // Test that cleanLLMJSON + json.Unmarshal correctly handles curator output.
    raw := `{
      "target": "/Users/foo/.claude/CLAUDE.md",
      "decisions": [
        {"proposalId": "prop-abc-1", "action": "synthesize", "reason": "merged", "supersededBy": "prop-curator-a1b2-1"},
        {"proposalId": "prop-abc-2", "action": "synthesize", "reason": "merged", "supersededBy": "prop-curator-a1b2-1"}
      ],
      "clusters": [
        {
          "clusterName": "Edit precondition failures",
          "sourceIds": ["prop-abc-1", "prop-abc-2"],
          "synthesis": {
            "id": "prop-curator-a1b2-1",
            "type": "claude_addition",
            "confidence": "high",
            "target": "/Users/foo/.claude/CLAUDE.md",
            "change": "Always read a file before editing it.",
            "rationale": "Synthesized from 2 proposals.",
            "citedUuids": []
          }
        }
      ]
    }`

    cleaned := cleanLLMJSON(raw)
    var manifest CuratorManifest
    if err := json.Unmarshal([]byte(cleaned), &manifest); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if manifest.Target != "/Users/foo/.claude/CLAUDE.md" {
        t.Errorf("Target: got %q", manifest.Target)
    }
    if len(manifest.Clusters) != 1 {
        t.Fatalf("Clusters: got %d, want 1", len(manifest.Clusters))
    }
    if manifest.Clusters[0].Synthesis == nil {
        t.Fatal("Synthesis is nil")
    }
    if manifest.Clusters[0].Synthesis.ID != "prop-curator-a1b2-1" {
        t.Errorf("Synthesis.ID: got %q", manifest.Clusters[0].Synthesis.ID)
    }
}
```

**Step 2: Run test to verify it passes** (it should already pass since `cleanLLMJSON` and the types exist)

```bash
go test ./internal/pipeline/... -run TestParseCuratorManifest -v
```
Expected: PASS.

**Step 3: Add RunCuratorGroup**

Add to `internal/pipeline/curator.go`:

```go
// RunCuratorGroup invokes an agentic Sonnet Curator session for a single target group.
// proposals must all target the same file.
// Returns the CuratorManifest, LLM usage, and any error.
func RunCuratorGroup(target string, proposals []ProposalWithSession, cfg PipelineConfig) (*CuratorManifest, *ClaudeResult, error) {
    if len(proposals) == 0 {
        return nil, nil, nil
    }

    if err := EnsureCuratorPrompts(); err != nil {
        return nil, nil, fmt.Errorf("ensuring curator prompts: %w", err)
    }

    systemPrompt, err := readPromptTemplate(curatorPromptFile)
    if err != nil {
        return nil, nil, fmt.Errorf("reading curator prompt: %w", err)
    }
    systemPrompt = strings.ReplaceAll(systemPrompt, "{{MAX_TURNS}}", fmt.Sprintf("%d", cfg.CuratorMaxTurns))

    // Serialize proposals as the user prompt.
    proposalData, err := json.MarshalIndent(proposals, "", "  ")
    if err != nil {
        return nil, nil, fmt.Errorf("marshaling proposals: %w", err)
    }
    userPrompt := fmt.Sprintf("Target: %s\n\nProposals:\n%s", target, string(proposalData))

    cr, err := invokeClaude(claudeConfig{
        Model:        cfg.CuratorModel,
        SystemPrompt: systemPrompt,
        Agentic:      true,
        Prompt:       userPrompt,
        AllowedTools: "Read,Grep",
        MaxTurns:     cfg.CuratorMaxTurns,
        Timeout:      cfg.CuratorTimeout,
        Logger:       cfg.Logger,
        Debug:        cfg.Debug,
        SettingSources: &emptyStr, // no user settings — curator is isolated
    })
    if err != nil {
        return nil, cr, fmt.Errorf("curator invocation for %s: %w", target, err)
    }

    cleaned := cleanLLMJSON(cr.Result)
    var manifest CuratorManifest
    if err := json.Unmarshal([]byte(cleaned), &manifest); err != nil {
        return nil, cr, fmt.Errorf("parsing curator manifest for %s: %w", target, err)
    }
    manifest.Target = target // ensure target is set even if LLM omitted it
    return &manifest, cr, nil
}
```

**Step 4: Compile check**

```bash
go build ./internal/pipeline/...
```
Expected: no errors. Fix any import issues.

**Step 5: Commit**

```bash
git add internal/pipeline/curator.go internal/pipeline/curator_test.go
git commit -m "feat(pipeline): add RunCuratorGroup (Sonnet agentic curator per target)"
```

---

### Task 7: performCleanup function in daemon

**Files:**
- Modify: `internal/pipeline/history.go` (export InvocationUsageFromResult)
- Create: `internal/daemon/cleanup.go`
- Create: `internal/daemon/cleanup_test.go`

**Step 1: Export InvocationUsageFromResult in history.go**

`usageFromResult` in `internal/pipeline/history.go` is unexported. `cleanup.go` (in package `daemon`) needs to convert `*ClaudeResult` to `InvocationUsage` for the `CleanupRecord`. Add an exported wrapper:

```go
// InvocationUsageFromResult converts a ClaudeResult into an InvocationUsage.
// Returns zero value if cr is nil.
func InvocationUsageFromResult(cr *ClaudeResult) InvocationUsage {
    if cr == nil {
        return InvocationUsage{}
    }
    return InvocationUsage{
        CCSessionID:         cr.SessionID,
        NumTurns:            cr.NumTurns,
        InputTokens:         cr.InputTokens,
        OutputTokens:        cr.OutputTokens,
        CacheCreationTokens: cr.CacheCreationTokens,
        CacheReadTokens:     cr.CacheReadTokens,
        CostUSD:             cr.TotalCostUSD,
        WebSearchRequests:   cr.WebSearchRequests,
        WebFetchRequests:    cr.WebFetchRequests,
    }
}
```

Compile check:
```bash
go build ./internal/pipeline/...
```

**Step 2: Write the failing test**

```go
// internal/daemon/cleanup_test.go
package daemon

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/vladolaru/cabrero/internal/pipeline"
)

func TestGroupProposalsByTarget(t *testing.T) {
    proposals := []pipeline.ProposalWithSession{
        {SessionID: "s1", Proposal: pipeline.Proposal{ID: "p1", Type: "claude_addition", Target: "/a/CLAUDE.md"}},
        {SessionID: "s2", Proposal: pipeline.Proposal{ID: "p2", Type: "claude_addition", Target: "/a/CLAUDE.md"}},
        {SessionID: "s3", Proposal: pipeline.Proposal{ID: "p3", Type: "skill_scaffold", Target: "/b/SKILL.md"}},
        {SessionID: "s4", Proposal: pipeline.Proposal{ID: "p4", Type: "skill_improvement", Target: "/c/skill.md"}},
    }

    multi, single := groupProposalsByTarget(proposals)

    if len(multi) != 1 {
        t.Errorf("multi: got %d targets, want 1", len(multi))
    }
    if len(multi["/a/CLAUDE.md"]) != 2 {
        t.Errorf("multi[/a/CLAUDE.md]: got %d, want 2", len(multi["/a/CLAUDE.md"]))
    }
    // Scaffolds always skip cleanup — should appear in single with action "keep".
    // /b/SKILL.md has 1 proposal (scaffold) — kept in single.
    // /c/skill.md has 1 proposal (skill_improvement) — kept in single.
    if len(single) != 2 {
        t.Errorf("single: got %d, want 2", len(single))
    }
}

func TestSkipNonFileTargets(t *testing.T) {
    proposals := []pipeline.ProposalWithSession{
        {Proposal: pipeline.Proposal{ID: "p1", Type: "skill_improvement", Target: "local-environment"}},
        {Proposal: pipeline.Proposal{ID: "p2", Type: "claude_addition", Target: "/a/CLAUDE.md"}},
    }
    _, single := groupProposalsByTarget(proposals)
    // "local-environment" is not a file target — should be excluded from single-check list.
    for _, pw := range single {
        if !pipeline.IsFileTarget(pw.Proposal.Target) {
            t.Errorf("non-file target %q should not appear in single list", pw.Proposal.Target)
        }
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/daemon/... -run TestGroupProposals -v
go test ./internal/daemon/... -run TestSkipNonFileTargets -v
```
Expected: compile error — `groupProposalsByTarget` undefined.

**Step 3: Write cleanup.go**

```go
// internal/daemon/cleanup.go
package daemon

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/vladolaru/cabrero/internal/apply"
    "github.com/vladolaru/cabrero/internal/pipeline"
)

// groupProposalsByTarget separates proposals into:
//   - multi: targets with 2+ proposals (map from target → proposals)
//   - single: file-target proposals that are the only proposal for their target
//
// Scaffolds in multi targets are moved to single (they are never curated).
// Non-file targets are excluded from single (no "already applied?" check possible).
func groupProposalsByTarget(proposals []pipeline.ProposalWithSession) (
    multi map[string][]pipeline.ProposalWithSession,
    single []pipeline.ProposalWithSession,
) {
    byTarget := make(map[string][]pipeline.ProposalWithSession)
    for _, pw := range proposals {
        byTarget[pw.Proposal.Target] = append(byTarget[pw.Proposal.Target], pw)
    }

    multi = make(map[string][]pipeline.ProposalWithSession)
    for target, group := range byTarget {
        // Split scaffolds out regardless of group size.
        var nonScaffold, scaffold []pipeline.ProposalWithSession
        for _, pw := range group {
            if pw.Proposal.Type == "skill_scaffold" {
                scaffold = append(scaffold, pw)
            } else {
                nonScaffold = append(nonScaffold, pw)
            }
        }
        // Scaffolds always go to single (kept as-is, no curation).
        for _, pw := range scaffold {
            if pipeline.IsFileTarget(pw.Proposal.Target) {
                single = append(single, pw)
            }
        }
        if len(nonScaffold) >= 2 {
            multi[target] = nonScaffold
        } else if len(nonScaffold) == 1 {
            if pipeline.IsFileTarget(nonScaffold[0].Proposal.Target) {
                single = append(single, nonScaffold[0])
            }
        }
    }
    return multi, single
}

// performCleanup runs the daily proposal cleanup:
//  1. Batched Haiku check for all single-proposal file-target proposals.
//  2. Parallelized Sonnet Curator for each multi-proposal target group.
//  3. Archives culled/rejected proposals and writes synthesized proposals.
//  4. Appends a CleanupRecord to cleanup_history.jsonl.
//  5. Sends a macOS notification with the summary.
func (d *Daemon) performCleanup(ctx context.Context) {
    runStart := time.Now()

    proposals, err := pipeline.ListProposals()
    if err != nil {
        d.log.Error("cleanup: listing proposals: %v", err)
        return
    }
    if len(proposals) == 0 {
        d.log.Info("cleanup: no proposals to process")
        return
    }

    d.log.Info("cleanup: starting (%d proposals)", len(proposals))

    multi, single := groupProposalsByTarget(proposals)

    var allDecisions []pipeline.CuratorDecision
    var curatorUsages []pipeline.InvocationUsage
    var checkUsage *pipeline.InvocationUsage

    // Stage 1: Haiku batch check for single-proposal targets.
    if len(single) > 0 {
        checkDecisions, cr, err := pipeline.RunCuratorCheck(single, d.config.Pipeline)
        if err != nil {
            d.log.Error("cleanup: curator check failed: %v", err)
            // Non-fatal: continue with multi-proposal curation.
        } else {
            if cr != nil {
                u := pipeline.InvocationUsageFromResult(cr)
                checkUsage = &u
            }
            // Apply check decisions.
            for _, cd := range checkDecisions {
                if cd.AlreadyApplied {
                    reason := "auto-culled: already applied to target"
                    if archErr := apply.Archive(cd.ProposalID, reason); archErr != nil {
                        d.log.Error("cleanup: archiving %s: %v", cd.ProposalID, archErr)
                        continue
                    }
                    allDecisions = append(allDecisions, pipeline.CuratorDecision{
                        ProposalID: cd.ProposalID,
                        Action:     "auto-reject",
                        Reason:     reason,
                    })
                } else {
                    allDecisions = append(allDecisions, pipeline.CuratorDecision{
                        ProposalID: cd.ProposalID,
                        Action:     "keep",
                        Reason:     "single proposal, not already applied",
                    })
                }
            }
        }
    }

    // Stage 2: Sonnet Curator for multi-proposal targets (parallelized).
    if len(multi) > 0 {
        type curatorResult struct {
            target   string
            manifest *pipeline.CuratorManifest
            cr       *pipeline.ClaudeResult
            err      error
        }

        resultsCh := make(chan curatorResult, len(multi))
        var wg sync.WaitGroup

        for target, group := range multi {
            select {
            case <-ctx.Done():
                break
            default:
            }

            wg.Add(1)
            go func(t string, g []pipeline.ProposalWithSession) {
                defer wg.Done()
                manifest, cr, err := pipeline.RunCuratorGroup(t, g, d.config.Pipeline)
                resultsCh <- curatorResult{target: t, manifest: manifest, cr: cr, err: err}
            }(target, group)
        }

        wg.Wait()
        close(resultsCh)

        for res := range resultsCh {
            if res.err != nil {
                d.log.Error("cleanup: curator for %s: %v", res.target, res.err)
                continue
            }
            if res.cr != nil {
                curatorUsages = append(curatorUsages, pipeline.InvocationUsageFromResult(res.cr))
            }
            if res.manifest == nil {
                continue
            }

            // Apply manifest decisions.
            if err := d.applyManifest(res.manifest); err != nil {
                d.log.Error("cleanup: applying manifest for %s: %v", res.target, err)
                continue
            }
            allDecisions = append(allDecisions, res.manifest.Decisions...)
            d.log.Info("cleanup: target %s — %d decisions", res.target, len(res.manifest.Decisions))
        }
    }

    // Count outcome.
    after, _ := pipeline.ListProposals()
    archived := 0
    synthesized := 0
    for _, d := range allDecisions {
        switch d.Action {
        case "cull", "auto-reject":
            archived++
        case "synthesize":
            synthesized++
        }
    }

    duration := time.Since(runStart)
    d.log.Info("cleanup: complete in %s — %d→%d proposals (%d archived, %d synthesized new)",
        duration.Round(time.Second), len(proposals), len(after), archived, synthesized)

    // Append cleanup record.
    rec := pipeline.CleanupRecord{
        Timestamp:       runStart,
        DurationNs:      int64(duration),
        ProposalsBefore: len(proposals),
        ProposalsAfter:  len(after),
        Decisions:       allDecisions,
        CuratorUsage:    curatorUsages,
        CheckUsage:      checkUsage,
    }
    if err := pipeline.AppendCleanupHistory(rec); err != nil {
        d.log.Error("cleanup: appending history: %v", err)
    }

    // Notify.
    msg := fmt.Sprintf("Cleanup: %d→%d proposals (%d archived)", len(proposals), len(after), archived)
    if err := d.notify("Cabrero", msg); err != nil {
        d.log.Error("cleanup notification failed: %v", err)
    }
}

// applyManifest archives culled/rejected proposals and writes synthesized proposals.
func (d *Daemon) applyManifest(manifest *pipeline.CuratorManifest) error {
    // Build decision map for fast lookup.
    decisionByID := make(map[string]pipeline.CuratorDecision, len(manifest.Decisions))
    for _, dec := range manifest.Decisions {
        decisionByID[dec.ProposalID] = dec
    }

    // Process decisions.
    for _, dec := range manifest.Decisions {
        switch dec.Action {
        case "cull":
            reason := "auto-culled: " + dec.Reason
            if err := apply.Archive(dec.ProposalID, reason); err != nil {
                d.log.Error("cleanup: archiving %s: %v", dec.ProposalID, err)
            }
        case "auto-reject":
            reason := "auto-culled: " + dec.Reason
            if err := apply.Archive(dec.ProposalID, reason); err != nil {
                d.log.Error("cleanup: archiving %s: %v", dec.ProposalID, err)
            }
        case "synthesize":
            reason := "auto-culled: synthesized into " + dec.SupersededBy
            if err := apply.Archive(dec.ProposalID, reason); err != nil {
                d.log.Error("cleanup: archiving %s: %v", dec.ProposalID, err)
            }
        case "keep":
            // No action needed.
        }
    }

    // Write synthesized proposals from clusters.
    for _, cluster := range manifest.Clusters {
        if cluster.Synthesis == nil {
            continue
        }
        if err := pipeline.WriteProposal(cluster.Synthesis, "curator"); err != nil {
            d.log.Error("cleanup: writing synthesized proposal %s: %v", cluster.Synthesis.ID, err)
        }
    }

    return nil
}
```

**Step 3: Run tests**

```bash
go test ./internal/daemon/... -run TestGroupProposals -v
go test ./internal/daemon/... -run TestSkipNonFileTargets -v
```
Expected: PASS.

**Step 4: Compile check**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add internal/pipeline/history.go internal/daemon/cleanup.go internal/daemon/cleanup_test.go
git commit -m "feat(daemon): add performCleanup and applyManifest for daily proposal curation"
```

---

### Task 8: Daemon config and cleanup ticker

**Files:**
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/daemon_test.go`

**Step 1: Write the failing test**

```go
// Add to internal/daemon/daemon_test.go
func TestDefaultConfigHasCleanupInterval(t *testing.T) {
    cfg := DefaultConfig()
    if cfg.CleanupInterval == 0 {
        t.Error("CleanupInterval should not be zero")
    }
    if cfg.CleanupInterval != 24*time.Hour {
        t.Errorf("CleanupInterval: got %v, want 24h", cfg.CleanupInterval)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/daemon/... -run TestDefaultConfigHasCleanupInterval -v
```
Expected: FAIL — `CleanupInterval` field does not exist.

**Step 3: Add CleanupInterval to Config and DefaultConfig**

In `internal/daemon/daemon.go`, add to `Config`:

```go
CleanupInterval time.Duration // how often to run proposal cleanup (default 24h)
```

In `DefaultConfig()`, add:

```go
CleanupInterval: 24 * time.Hour,
```

**Step 4: Add cleanup ticker to the Run loop**

In `func (d *Daemon) Run(ctx context.Context) error`, after the `staleTicker` setup, add:

```go
cleanupTicker := time.NewTicker(d.config.CleanupInterval)
defer cleanupTicker.Stop()
```

Add to the `select` block:

```go
case <-cleanupTicker.C:
    d.performCleanup(ctx)
```

Also add cleanup history rotation to daemon startup (alongside the existing `RotateHistory` call):

```go
if removed, err := pipeline.RotateCleanupHistory(90 * 24 * time.Hour); err != nil {
    d.log.Info("cleanup history rotation failed: %v", err)
} else if removed > 0 {
    d.log.Info("rotated %d old cleanup history records", removed)
}
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/daemon/... -run TestDefaultConfigHasCleanupInterval -v
```
Expected: PASS.

**Step 6: Compile check**

```bash
go build ./...
```

**Step 7: Commit**

```bash
git add internal/daemon/daemon.go internal/daemon/daemon_test.go
git commit -m "feat(daemon): add CleanupInterval config and daily cleanup ticker"
```

---

### Task 9: TUI pipeline activity — load cleanup records

**Files:**
- Modify: `internal/pipeline/run.go`
- Modify: `internal/pipeline/run_test.go` (or add `run_cleanup_test.go`)

**Step 1: Write the failing test**

```go
// Add to internal/pipeline/run_test.go
func TestListCleanupRunsFromHistory(t *testing.T) {
    dir := t.TempDir()
    origPath := cleanupHistoryPath
    cleanupHistoryPath = func() string { return filepath.Join(dir, "cleanup_history.jsonl") }
    defer func() { cleanupHistoryPath = origPath }()

    _ = AppendCleanupHistory(CleanupRecord{
        Timestamp:       time.Now().Add(-1 * time.Hour),
        DurationNs:      int64(47 * time.Second),
        ProposalsBefore: 64,
        ProposalsAfter:  12,
        Decisions: []CuratorDecision{
            {ProposalID: "p1", Action: "cull"},
            {ProposalID: "p2", Action: "auto-reject"},
        },
    })

    runs, err := ListCleanupRunsFromHistory(10)
    if err != nil {
        t.Fatal(err)
    }
    if len(runs) != 1 {
        t.Fatalf("got %d runs, want 1", len(runs))
    }
    run := runs[0]
    if run.Source != "cleanup" {
        t.Errorf("Source: got %q, want cleanup", run.Source)
    }
    if run.ProposalCount != 52 { // 64 - 12 = 52 archived
        t.Errorf("ProposalCount: got %d, want 52", run.ProposalCount)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/pipeline/... -run TestListCleanupRunsFromHistory -v
```
Expected: compile error — `ListCleanupRunsFromHistory` undefined, `PipelineRun.Source` field missing.

**Step 3: Add Source field to PipelineRun and ListCleanupRunsFromHistory**

In `internal/pipeline/run.go`, add `Source` field to `PipelineRun`:

```go
type PipelineRun struct {
    // ... existing fields ...
    Source string // "daemon", "cli-run", "cli-backfill", "cleanup"
}
```

Add `ListCleanupRunsFromHistory` function:

```go
// ListCleanupRunsFromHistory returns PipelineRun entries from cleanup_history.jsonl.
// Each cleanup run is one PipelineRun with Source="cleanup".
// Pass limit=0 for no limit.
func ListCleanupRunsFromHistory(limit int) ([]PipelineRun, error) {
    records, err := ReadCleanupHistory()
    if err != nil || len(records) == 0 {
        return nil, err
    }

    var runs []PipelineRun
    for i, rec := range records {
        if limit > 0 && i >= limit {
            break
        }
        // ProposalCount = proposals archived (before - after).
        archived := rec.ProposalsBefore - rec.ProposalsAfter
        if archived < 0 {
            archived = 0
        }
        // Sum curator LLM usage.
        var inputTokens, outputTokens int
        var costUSD float64
        for _, u := range rec.CuratorUsage {
            inputTokens += u.InputTokens
            outputTokens += u.OutputTokens
            costUSD += u.CostUSD
        }
        if rec.CheckUsage != nil {
            inputTokens += rec.CheckUsage.InputTokens
            outputTokens += rec.CheckUsage.OutputTokens
            costUSD += rec.CheckUsage.CostUSD
        }

        run := PipelineRun{
            Source:        "cleanup",
            Timestamp:     rec.Timestamp,
            Status:        "processed",
            ProposalCount: archived,
            InputTokens:   inputTokens,
            OutputTokens:  outputTokens,
            CostUSD:       costUSD,
            ErrorDetail:   rec.Error,
        }
        if rec.Error != "" {
            run.Status = "error"
        }
        runs = append(runs, run)
    }
    return runs, nil
}
```

In `ListPipelineRunsFromHistory`, after building session runs, prepend cleanup runs and sort by timestamp (or let the TUI sort). Simplest: return both from separate functions and merge at the call site in `tui/model.go`.

Find where `ListPipelineRunsFromHistory` is called in `internal/tui/model.go` or `internal/tui/pipeline/model.go` and add the cleanup runs there:

```go
// Pseudocode at call site — find the exact location:
sessionRuns, err := pipeline.ListPipelineRunsFromHistory(sessions, 50)
cleanupRuns, _ := pipeline.ListCleanupRunsFromHistory(10)
runs := append(cleanupRuns, sessionRuns...)
// Sort by Timestamp descending if not already sorted.
```

Adjust the TUI pipeline view rendering to display `Source: "cleanup"` rows distinctly. In `internal/tui/pipeline/view.go`, find `renderRecentRuns` and add a branch for `run.Source == "cleanup"`:

```go
// In renderRecentRuns or the per-row render helper:
if run.Source == "cleanup" {
    // Render: "CLEANUP  <timestamp>  <before>→<after>  <duration>  $<cost>  <archived> archived"
    // Use existing formatters — cli.FormatDuration, cli.FormatCost, etc.
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/pipeline/... -run TestListCleanupRunsFromHistory -v
```
Expected: PASS.

**Step 5: Compile and snapshot check**

```bash
go build ./...
go run ./cmd/snapshot pipeline-monitor   # verify TUI renders without panic
```

**Step 6: Run all tests**

```bash
go test ./... 2>&1 | tail -20
```
Expected: all pass (or pre-existing failures only).

**Step 7: Commit**

```bash
git add internal/pipeline/run.go internal/tui/pipeline/view.go internal/tui/model.go
git commit -m "feat(tui): surface cleanup runs in Pipeline Activity section"
```

---

## Build verification

After all tasks:

```bash
go build ./...
go test ./...
make snapshots   # verify TUI snapshots render correctly
cabrero doctor   # if daemon is running, verify no regressions
```

---

## Notes for implementer

- Confirm how existing prompt tests override the store root — `TestEnsureCuratorPrompts` must mirror that pattern exactly.
- The Sonnet Curator has unrestricted `Read,Grep` access (like the Evaluator). The `SettingSources: &emptyStr` isolation prevents loading user plugins/MCP servers during cleanup.
- If `go build ./...` fails after Task 7 due to an import cycle (daemon importing pipeline importing store), verify the import graph — the existing codebase already has this structure so it should be fine.
