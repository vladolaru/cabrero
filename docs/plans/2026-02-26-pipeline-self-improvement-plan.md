# Pipeline Self-Improvement Loop — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the compound engineering loop on the pipeline by adding blocklist rotation, always-on session persistence, typed outcome tracking, complete model config, and a daily Opus meta-pipeline that generates `prompt_improvement` proposals when quality signals degrade.

**Architecture:** Four sequential components — each is a prerequisite for the next. Blocklist rotation enables scale for persistence; persistence makes cc_session_ids live pointers; outcome tracking + model config unlock metrics; metrics power the meta-pipeline. Tests for each component must pass before starting the next.

**Design doc:** `docs/plans/2026-02-26-pipeline-self-improvement-design.md`

**Tech Stack:** Go 1.23, `encoding/json`, `os/exec` (claude CLI), BubbleTea TUI, `go test ./...`

---

## Component 1: Blocklist Rotation

### Task 1: Add `BlocklistEntry` type with migration

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

**Step 1: Write the failing test**

Add to `internal/store/store_test.go`:
```go
func TestBlocklistMigration_OldArrayFormat(t *testing.T) {
    tmp := t.TempDir()
    t.Setenv("HOME", tmp)
    if err := Init(); err != nil {
        t.Fatalf("Init: %v", err)
    }
    // Write old-format blocklist ([]string array).
    oldData := `["sess-aaa","sess-bbb"]`
    blPath := filepath.Join(tmp, ".cabrero", "blocklist.json")
    if err := os.WriteFile(blPath, []byte(oldData), 0o644); err != nil {
        t.Fatalf("writing old blocklist: %v", err)
    }
    // ReadBlocklist must return both IDs without error.
    m, err := ReadBlocklist()
    if err != nil {
        t.Fatalf("ReadBlocklist after migration: %v", err)
    }
    if !m["sess-aaa"] || !m["sess-bbb"] {
        t.Errorf("expected both sessions in blocklist, got %v", m)
    }
}

func TestBlockSession_WritesTimestamp(t *testing.T) {
    tmp := t.TempDir()
    t.Setenv("HOME", tmp)
    if err := Init(); err != nil {
        t.Fatalf("Init: %v", err)
    }
    before := time.Now()
    if err := BlockSession("sess-xyz", time.Now()); err != nil {
        t.Fatalf("BlockSession: %v", err)
    }
    after := time.Now()

    m, err := ReadBlocklist()
    if err != nil {
        t.Fatalf("ReadBlocklist: %v", err)
    }
    if !m["sess-xyz"] {
        t.Errorf("sess-xyz not in blocklist")
    }

    // Read raw file and verify timestamp field is present.
    blPath := filepath.Join(tmp, ".cabrero", "blocklist.json")
    raw, _ := os.ReadFile(blPath)
    var entries map[string]struct{ BlockedAt time.Time }
    if err := json.Unmarshal(raw, &entries); err != nil {
        t.Fatalf("parsing new-format blocklist: %v", err)
    }
    e := entries["sess-xyz"]
    if e.BlockedAt.Before(before) || e.BlockedAt.After(after) {
        t.Errorf("BlockedAt %v outside expected range [%v, %v]", e.BlockedAt, before, after)
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/store/ -run TestBlocklistMigration_OldArrayFormat -v
go test ./internal/store/ -run TestBlockSession_WritesTimestamp -v
```
Expected: compile error (wrong number of args to `BlockSession`) or FAIL.

**Step 3: Implement**

In `internal/store/store.go`, replace the blocklist section (lines ~120–195):

```go
// BlocklistEntry records when a session was blocked.
type BlocklistEntry struct {
    BlockedAt time.Time `json:"blockedAt"`
}

func readBlocklist() (map[string]bool, error) {
    data, err := os.ReadFile(blocklistPath())
    if err != nil {
        if os.IsNotExist(err) {
            return make(map[string]bool), nil
        }
        return nil, err
    }
    // Migration: if root is a JSON array, it's the old []string format.
    trimmed := strings.TrimSpace(string(data))
    if strings.HasPrefix(trimmed, "[") {
        var ids []string
        if err := json.Unmarshal(data, &ids); err != nil {
            return nil, fmt.Errorf("parsing old-format blocklist: %w", err)
        }
        // Convert to new format with zero time; write back.
        entries := make(map[string]BlocklistEntry, len(ids))
        for _, id := range ids {
            entries[id] = BlocklistEntry{} // zero BlockedAt
        }
        if err := writeBlocklistEntries(entries); err != nil {
            return nil, fmt.Errorf("migrating blocklist: %w", err)
        }
        m := make(map[string]bool, len(ids))
        for _, id := range ids {
            m[id] = true
        }
        return m, nil
    }
    var entries map[string]BlocklistEntry
    if err := json.Unmarshal(data, &entries); err != nil {
        return nil, fmt.Errorf("parsing blocklist: %w", err)
    }
    m := make(map[string]bool, len(entries))
    for id := range entries {
        m[id] = true
    }
    return m, nil
}

func writeBlocklistEntries(entries map[string]BlocklistEntry) error {
    data, err := json.MarshalIndent(entries, "", "  ")
    if err != nil {
        return err
    }
    return AtomicWrite(blocklistPath(), data, 0o644)
}

// writeBlocklist is kept for Init's nil seed (writes empty map).
func writeBlocklist(m map[string]bool) error {
    entries := make(map[string]BlocklistEntry, len(m))
    for id := range m {
        entries[id] = BlocklistEntry{}
    }
    return writeBlocklistEntries(entries)
}

// BlockSession adds a session ID to the blocklist with a timestamp.
func BlockSession(sessionID string, blockedAt time.Time) error {
    blocklistMu.Lock()
    defer blocklistMu.Unlock()

    data, err := os.ReadFile(blocklistPath())
    var entries map[string]BlocklistEntry
    if err != nil {
        if !os.IsNotExist(err) {
            return err
        }
        entries = make(map[string]BlocklistEntry)
    } else {
        trimmed := strings.TrimSpace(string(data))
        if strings.HasPrefix(trimmed, "[") {
            // old format — read via migration path
            entries = make(map[string]BlocklistEntry)
        } else {
            if err2 := json.Unmarshal(data, &entries); err2 != nil {
                entries = make(map[string]BlocklistEntry)
            }
        }
    }
    entries[sessionID] = BlocklistEntry{BlockedAt: blockedAt}
    return writeBlocklistEntries(entries)
}

// IsBlocked returns true if the given session ID is in the blocklist.
func IsBlocked(sessionID string) bool {
    blocklistMu.Lock()
    defer blocklistMu.Unlock()
    m, err := readBlocklist()
    if err != nil {
        return false
    }
    return m[sessionID]
}
```

Add `"strings"` to imports.

**Step 4: Run tests**
```bash
go test ./internal/store/ -run TestBlocklistMigration -v
go test ./internal/store/ -run TestBlockSession_WritesTimestamp -v
go test ./internal/store/ -v
```
Expected: all PASS.

**Step 5: Fix callers of `BlockSession`** (compilation requires it)

Update all callers to pass `time.Now()`:
- `internal/pipeline/invoke.go:173`: `store.BlockSession(debugSessionID, time.Now())`
- `internal/tui/model.go:899`: `store.BlockSession(sessionID, time.Now())`
- `internal/tui/model.go:900` (buildChatConfig): `store.BlockSession(sessionID, time.Now())`

```bash
go build ./...
```
Expected: clean build.

**Step 6: Commit**
```bash
git add internal/store/store.go internal/store/store_test.go \
        internal/pipeline/invoke.go internal/tui/model.go
git commit -m "feat(store): add BlocklistEntry with timestamp and old-array migration"
```

---

### Task 2: Add `RotateBlocklist`

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

**Step 1: Write the failing test**

```go
func TestRotateBlocklist_RemovesOldEntries(t *testing.T) {
    tmp := t.TempDir()
    t.Setenv("HOME", tmp)
    if err := Init(); err != nil {
        t.Fatalf("Init: %v", err)
    }
    old := time.Now().Add(-100 * 24 * time.Hour) // 100 days ago
    fresh := time.Now().Add(-1 * time.Hour)       // 1 hour ago

    if err := BlockSession("old-sess", old); err != nil {
        t.Fatalf("BlockSession old: %v", err)
    }
    if err := BlockSession("fresh-sess", fresh); err != nil {
        t.Fatalf("BlockSession fresh: %v", err)
    }

    removed, err := RotateBlocklist(90 * 24 * time.Hour)
    if err != nil {
        t.Fatalf("RotateBlocklist: %v", err)
    }
    if removed != 1 {
        t.Errorf("removed = %d, want 1", removed)
    }
    m, _ := ReadBlocklist()
    if m["old-sess"] {
        t.Error("old-sess should have been rotated out")
    }
    if !m["fresh-sess"] {
        t.Error("fresh-sess should have been kept")
    }
}

func TestRotateBlocklist_ZeroAgeEntries_Kept(t *testing.T) {
    // Migrated entries have BlockedAt zero — they should NOT be rotated
    // until they age past maxAge from the zero time, which is far in the past.
    // In practice, they will be rotated immediately since zero time is ancient.
    // This test verifies the rotation is based purely on BlockedAt age.
    tmp := t.TempDir()
    t.Setenv("HOME", tmp)
    if err := Init(); err != nil {
        t.Fatalf("Init: %v", err)
    }
    if err := BlockSession("migrated-sess", time.Time{}); err != nil {
        t.Fatalf("BlockSession: %v", err)
    }
    removed, err := RotateBlocklist(90 * 24 * time.Hour)
    if err != nil {
        t.Fatalf("RotateBlocklist: %v", err)
    }
    if removed != 1 {
        t.Errorf("migrated entry with zero time should be rotated, removed = %d", removed)
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/store/ -run TestRotateBlocklist -v
```
Expected: compile error (`RotateBlocklist` undefined).

**Step 3: Implement**

In `internal/store/store.go`, add after `IsBlocked`:
```go
// RotateBlocklist removes blocklist entries older than maxAge.
// Returns the number of entries removed and rewrites the file atomically.
// Entries with a zero BlockedAt (migrated from old format) are treated as
// maximally old and will always be removed.
func RotateBlocklist(maxAge time.Duration) (int, error) {
    blocklistMu.Lock()
    defer blocklistMu.Unlock()

    data, err := os.ReadFile(blocklistPath())
    if err != nil {
        if os.IsNotExist(err) {
            return 0, nil
        }
        return 0, err
    }
    trimmed := strings.TrimSpace(string(data))
    if strings.HasPrefix(trimmed, "[") {
        // Old format: all entries have zero time — all will be rotated.
        var ids []string
        json.Unmarshal(data, &ids) //nolint:errcheck
        if err := writeBlocklistEntries(make(map[string]BlocklistEntry)); err != nil {
            return 0, err
        }
        return len(ids), nil
    }

    var entries map[string]BlocklistEntry
    if err := json.Unmarshal(data, &entries); err != nil {
        return 0, fmt.Errorf("parsing blocklist for rotation: %w", err)
    }

    cutoff := time.Now().Add(-maxAge)
    kept := make(map[string]BlocklistEntry, len(entries))
    removed := 0
    for id, entry := range entries {
        // Zero time is treated as epoch — always older than any cutoff.
        if !entry.BlockedAt.IsZero() && entry.BlockedAt.After(cutoff) {
            kept[id] = entry
        } else {
            removed++
        }
    }
    if removed == 0 {
        return 0, nil
    }
    if err := writeBlocklistEntries(kept); err != nil {
        return 0, err
    }
    return removed, nil
}
```

**Step 4: Run tests**
```bash
go test ./internal/store/ -run TestRotateBlocklist -v
go test ./internal/store/ -v
```
Expected: all PASS.

**Step 5: Wire into daemon startup**

In `internal/daemon/daemon.go`, after the two existing rotation calls (~line 104):
```go
// Rotate old blocklist entries on startup.
if removed, err := store.RotateBlocklist(90 * 24 * time.Hour); err != nil {
    d.log.Info("blocklist rotation failed: %v", err)
} else if removed > 0 {
    d.log.Info("rotated %d old blocklist entries", removed)
}
```

```bash
go build ./...
go test ./...
```
Expected: clean build, all tests pass.

**Step 6: Commit**
```bash
git add internal/store/store.go internal/store/store_test.go internal/daemon/daemon.go
git commit -m "feat(store): add RotateBlocklist with 90-day TTL, wire into daemon startup"
```

---

## Component 2: Always-On Session Persistence + Format Drift Warnings

### Task 3: Always-on agentic persistence in `invoke.go`

**Files:**
- Modify: `internal/pipeline/invoke.go`
- Modify: `internal/pipeline/invoke_test.go`

**Step 1: Write the failing test**

Read existing `invoke_test.go` first to match its pattern. Add:
```go
func TestBuildClaudeArgs_AgenticAlwaysHasSessionID(t *testing.T) {
    // With Debug=false, agentic mode must still generate --session-id
    // and must NOT include --no-session-persistence.
    cfg := claudeConfig{
        Model:   "claude-haiku-4-5",
        Agentic: true,
        Prompt:  "test prompt",
        Debug:   false,
    }
    args := buildClaudeArgs(cfg, "some-uuid")
    hasSessionID := false
    hasPersistFlag := false
    for i, a := range args {
        if a == "--session-id" && i+1 < len(args) {
            hasSessionID = true
        }
        if a == "--no-session-persistence" {
            hasPersistFlag = true
        }
    }
    if !hasSessionID {
        t.Error("agentic mode must include --session-id even when Debug=false")
    }
    if hasPersistFlag {
        t.Error("agentic mode must not include --no-session-persistence")
    }
}

func TestBuildClaudeArgs_PrintModeKeepsNoPersistence(t *testing.T) {
    cfg := claudeConfig{
        Model:   "claude-sonnet-4-6",
        Agentic: false,
        Debug:   false,
    }
    args := buildClaudeArgs(cfg, "")
    hasNoPersist := false
    for _, a := range args {
        if a == "--no-session-persistence" {
            hasNoPersist = true
        }
    }
    if !hasNoPersist {
        t.Error("print mode must keep --no-session-persistence")
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/pipeline/ -run TestBuildClaudeArgs_AgenticAlwaysHasSessionID -v
```
Expected: FAIL (current code only adds session-id in debug mode).

**Step 3: Implement**

In `internal/pipeline/invoke.go`, change `invokeClaude`:

```go
// Generate session ID and blocklist it for ALL agentic invocations
// (not just debug). Print-mode invocations skip this.
var agenticSessionID string
if cfg.Agentic {
    id, err := GenerateUUID()
    if err != nil {
        return nil, fmt.Errorf("generating session ID: %w", err)
    }
    agenticSessionID = id
    if err := store.BlockSession(agenticSessionID, time.Now()); err != nil {
        return nil, fmt.Errorf("blocklisting session: %w", err)
    }
    if cfg.Debug && cfg.Logger != nil {
        name := "agentic"
        cfg.Logger.Info("  [debug] CC session %s persisted for inspection", agenticSessionID)
        _ = name
    }
}
args := buildClaudeArgs(cfg, agenticSessionID)
```

Remove the old `debugSessionID` block entirely. Update `buildClaudeArgs` agentic branch:

```go
// Agentic mode: always pass --session-id; never pass --no-session-persistence.
// sessionID is always non-empty for agentic calls (generated by invokeClaude).
if sessionID != "" {
    args = append(args, "--session-id", sessionID)
}
// (Remove the if/else debug block that was here)
```

Also remove the post-invocation debug log (it moved into the pre-invocation block above).

**Step 4: Run tests**
```bash
go test ./internal/pipeline/ -run TestBuildClaudeArgs -v
go test ./internal/pipeline/ -v
```
Expected: all PASS.

**Step 5: Update `invoke.go` debug log for classifier/evaluator**

In `runner.go`, the debug log now reads from `ClaudeResult.SessionID` (which CC echoes back). Update the debug log in `runner.go` classify path:

```go
// After RunClassifier succeeds and classifierCR != nil:
if r.Config.Debug && r.Config.Logger() != nil && classifierCR.SessionID != "" {
    r.Config.logger().Info("  [debug] CC session %s persisted — classifier for %s",
        classifierCR.SessionID, sessionID)
}
```

Similarly for evaluator. These are the debug log lines from the design doc. Check `runner.go` around lines 210–260 and add them after each successful classify/evaluate call.

```bash
go build ./...
go test ./...
```

**Step 6: Commit**
```bash
git add internal/pipeline/invoke.go internal/pipeline/invoke_test.go internal/pipeline/runner.go
git commit -m "feat(pipeline): always-on agentic session persistence, debug becomes verbosity-only"
```

---

### Task 4: `RawUnknown` format drift warning in `runner.go`

**Files:**
- Modify: `internal/pipeline/runner.go`
- Modify: `internal/pipeline/runner_test.go` (if it exists) or create a small test

**Step 1: Write failing test**

In `internal/pipeline/runner_test.go` (or look for the existing file):
```go
func TestParseSession_LogsRawUnknownWarning(t *testing.T) {
    // Inject a ParseSession that returns a Digest with RawUnknown entries.
    var loggedWarning string
    logger := &capturingLogger{warnFn: func(msg string) { loggedWarning = msg }}

    cfg := PipelineConfig{Logger: logger}
    stages := TestStages{
        ParseSessionFunc: func(sid string) (*parser.Digest, error) {
            return &parser.Digest{
                RawUnknown: []parser.RawUnknown{
                    {Type: "new-unknown-type", LineNumber: 3},
                    {Type: "parse_error", LineNumber: 7},
                },
            }, nil
        },
    }
    r := NewRunnerWithStages(cfg, stages)
    // parseSession is unexported; test via classify which calls it.
    // Alternatively, expose a testable hook. For now test the log output.
    // Set up a minimal session in the store.
    // ... (see existing runner test patterns for store setup)
}
```

If the runner tests are complex to set up here, a simpler approach is an integration-style check: verify the warning text is emitted by checking that `runner.parseSession` returns the digest unchanged and the logger received the call. Look at `internal/pipeline/runner_test.go` first — if it exists, match its patterns.

**Step 2: Run to verify failure**
```bash
go test ./internal/pipeline/ -run TestParseSession_LogsRawUnknown -v
```

**Step 3: Implement**

In `internal/pipeline/runner.go`, in the `parseSession` method (line 125), after the call:

```go
func (r *Runner) parseSession(sessionID string) (*parser.Digest, error) {
    if r.stages != nil {
        if d, err := r.stages.ParseSession(sessionID); d != nil || err != nil {
            if d != nil && len(d.RawUnknown) > 0 {
                r.warnFormatDrift(sessionID, d.RawUnknown)
            }
            return d, err
        }
    }
    d, err := parser.ParseSession(sessionID)
    if err != nil {
        return nil, err
    }
    if len(d.RawUnknown) > 0 {
        r.warnFormatDrift(sessionID, d.RawUnknown)
    }
    return d, nil
}

func (r *Runner) warnFormatDrift(sessionID string, unknown []parser.RawUnknown) {
    types := make(map[string]int)
    for _, u := range unknown {
        types[u.Type]++
    }
    typeList := make([]string, 0, len(types))
    for t := range types {
        typeList = append(typeList, t)
    }
    r.Config.logger().Error("WARN session %s: %d unrecognised transcript entries (types: %v) — CC format may have changed",
        sessionID, len(unknown), typeList)
}
```

**Step 4: Run tests**
```bash
go test ./internal/pipeline/ -v
go test ./...
```
Expected: all PASS.

**Step 5: Commit**
```bash
git add internal/pipeline/runner.go
git commit -m "feat(pipeline): warn on unrecognised CC transcript entry types (format drift detection)"
```

---

## Component 3a: Model Config Completeness

### Task 5: Add model fields + constants to `PipelineConfig`

**Files:**
- Modify: `internal/pipeline/pipeline.go`
- Modify: `internal/pipeline/curator.go`
- Modify: `internal/apply/apply.go`
- Modify: `internal/tui/chat/model.go`
- Modify: `internal/tui/chat/stream.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/cmd/approve.go`

**Step 1: Write failing tests**

In `internal/pipeline/pipeline_test.go` (create if absent):
```go
func TestDefaultPipelineConfig_AllModelFieldsSet(t *testing.T) {
    cfg := DefaultPipelineConfig()
    cases := []struct {
        name  string
        field string
    }{
        {"CuratorCheckModel", cfg.CuratorCheckModel},
        {"ApplyModel", cfg.ApplyModel},
        {"ChatModel", cfg.ChatModel},
        {"MetaModel", cfg.MetaModel},
    }
    for _, c := range cases {
        if c.field == "" {
            t.Errorf("%s is empty in DefaultPipelineConfig", c.name)
        }
    }
}
```

In `internal/tui/chat/model_test.go`, verify `buildChatArgs` uses `cfg.Model`:
```go
func TestBuildChatArgs_UsesConfigModel(t *testing.T) {
    cfg := ChatConfig{Model: "claude-haiku-4-5"}
    args := buildChatArgs(cfg, true)
    for i, a := range args {
        if a == "--model" && i+1 < len(args) {
            if args[i+1] != "claude-haiku-4-5" {
                t.Errorf("--model = %q, want %q", args[i+1], "claude-haiku-4-5")
            }
            return
        }
    }
    t.Error("--model flag not found in args")
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/pipeline/ -run TestDefaultPipelineConfig_AllModelFieldsSet -v
go test ./internal/tui/chat/ -run TestBuildChatArgs_UsesConfigModel -v
```

**Step 3: Implement — `pipeline.go`**

In `internal/pipeline/pipeline.go`, add to `PipelineConfig`:
```go
// Per-entity model config (each LLM-invoking entity has its own field).
CuratorCheckModel string // default: DefaultCuratorCheckModel (Haiku)
ApplyModel        string // default: DefaultApplyModel (Sonnet)
ChatModel         string // default: DefaultChatModel (Sonnet)
MetaModel         string // default: DefaultMetaModel (Opus)

// Meta-pipeline thresholds.
MetaRejectionRateThreshold float64 // default 0.30
MetaClassifierFPRThreshold float64 // default 0.25
MetaMinSamples             int     // default 5
MetaCooldownDays           int     // default 14
MetaMaxTurns               int     // default 20
MetaTimeout                time.Duration // default 5 * time.Minute
```

Add constants (new file or alongside existing constants in `classifier.go`/`evaluator.go`):
```go
// In internal/pipeline/pipeline.go or a new constants file:
const (
    DefaultCuratorCheckModel = "claude-haiku-4-5"
    DefaultApplyModel        = "claude-sonnet-4-6"
    DefaultChatModel         = "claude-sonnet-4-6"
    DefaultMetaModel         = "claude-opus-4-6"
)
```

Update `DefaultPipelineConfig()`:
```go
CuratorCheckModel:          DefaultCuratorCheckModel,
ApplyModel:                 DefaultApplyModel,
ChatModel:                  DefaultChatModel,
MetaModel:                  DefaultMetaModel,
MetaRejectionRateThreshold: 0.30,
MetaClassifierFPRThreshold: 0.25,
MetaMinSamples:             5,
MetaCooldownDays:           14,
MetaMaxTurns:               20,
MetaTimeout:                5 * time.Minute,
```

Also fix the `CuratorModel` line to use its own constant:
```go
CuratorModel: DefaultCuratorModel, // was: DefaultEvaluatorModel
```
Add `const DefaultCuratorModel = "claude-sonnet-4-6"`.

**Step 4: Implement — `curator.go`**

Change line 79:
```go
Model: cfg.CuratorCheckModel, // was: cfg.ClassifierModel
```

**Step 5: Implement — `apply.go` Blend**

Add `model string` parameter:
```go
func Blend(proposal *pipeline.Proposal, sessionID string, model string) (string, error) {
```
Replace the hardcoded `"claude-sonnet-4-6"` in the exec.Command with `model`.

Update all callers:
- `internal/cmd/approve.go:45`: `apply.Blend(p, pw.SessionID, pipeline.DefaultPipelineConfig().ApplyModel)`
- `internal/tui/model.go:157`: `apply.Blend(proposal, sessionID, m.config.Pipeline.ApplyModel)`

**Step 6: Implement — chat `ChatConfig.Model` + `buildChatArgs`**

In `internal/tui/chat/model.go`, add `Model string` to `ChatConfig`:
```go
type ChatConfig struct {
    SessionID    string
    SystemPrompt string
    AllowedTools string
    Model        string // claude model to use (default: DefaultChatModel)
    Debug        bool
}
```

In `internal/tui/chat/stream.go`, replace hardcoded `"claude-sonnet-4-6"`:
```go
model := cfg.Model
if model == "" {
    model = "claude-sonnet-4-6" // fallback if not set
}
args := []string{
    "--model", model,
    // ...
}
```

In `internal/tui/model.go`, `buildChatConfig`:
```go
return chat.ChatConfig{
    SessionID:    sessionID,
    SystemPrompt: buildChatSystemPrompt(p),
    AllowedTools: buildChatAllowedTools(p),
    Model:        m.config.Pipeline.ChatModel,
    Debug:        debug,
}, nil
```
This requires `buildChatConfig` to receive the pipeline config or use `m.config.Pipeline.ChatModel`. Since `buildChatConfig` is a method on `m`, it already has access via `m.config`. If it's a free function, add `pipelineCfg pipeline.PipelineConfig` parameter or pass the model string directly.

**Step 7: Run tests**
```bash
go build ./...
go test ./internal/pipeline/ -run TestDefaultPipelineConfig -v
go test ./internal/tui/chat/ -run TestBuildChatArgs -v
go test ./...
```
Expected: all PASS.

**Step 8: Commit**
```bash
git add internal/pipeline/pipeline.go internal/pipeline/curator.go \
        internal/apply/apply.go internal/tui/chat/model.go \
        internal/tui/chat/stream.go internal/tui/model.go \
        internal/cmd/approve.go
git commit -m "feat(pipeline): complete model config — 7 named fields, remove hardcoded model strings"
```

---

### Task 6: Expand MODELS section in Pipeline Monitor TUI

**Files:**
- Modify: `internal/tui/pipeline/view.go`
- Modify: `internal/tui/pipeline/view_test.go` (if exists) or `internal/tui/pipeline/model_test.go`

**Step 1: Write failing test**

Find the existing test that checks `renderModels()` output. Add assertions for the new fields:
```go
// In the existing models section test:
if !strings.Contains(rendered, "Curator:") {
    t.Error("MODELS section missing Curator line")
}
if !strings.Contains(rendered, "Apply:") {
    t.Error("MODELS section missing Apply line")
}
if !strings.Contains(rendered, "Chat:") {
    t.Error("MODELS section missing Chat line")
}
if !strings.Contains(rendered, "Meta:") {
    t.Error("MODELS section missing Meta line")
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/tui/pipeline/ -run TestModels -v
```

**Step 3: Implement**

In `internal/tui/pipeline/view.go`, `renderModels()`:
```go
func (m Model) renderModels() string {
    var b strings.Builder
    b.WriteString(shared.RenderSectionHeader("MODELS"))
    b.WriteString("\n")
    cfg := m.pipelineCfg

    rows := []struct{ label, model, def string }{
        {"Classifier:  ", cfg.ClassifierModel, pl.DefaultClassifierModel},
        {"Evaluator:   ", cfg.EvaluatorModel, pl.DefaultEvaluatorModel},
        {"Curator:     ", cfg.CuratorModel, pl.DefaultCuratorModel},
        {"Curator chk: ", cfg.CuratorCheckModel, pl.DefaultCuratorCheckModel},
        {"Apply:       ", cfg.ApplyModel, pl.DefaultApplyModel},
        {"Chat:        ", cfg.ChatModel, pl.DefaultChatModel},
        {"Meta:        ", cfg.MetaModel, pl.DefaultMetaModel},
    }
    for _, row := range rows {
        b.WriteString(fmt.Sprintf("  %s%s", row.label, row.model))
        if row.model != row.def {
            b.WriteString(shared.WarningStyle.Render("  (override)"))
        }
        b.WriteString("\n")
    }
    return b.String()
}
```

**Step 4: Run tests**
```bash
go test ./internal/tui/pipeline/ -v
go test ./...
```

**Step 5: Commit**
```bash
git add internal/tui/pipeline/view.go internal/tui/pipeline/
git commit -m "feat(tui): expand MODELS section to show all 7 model fields"
```

---

## Component 3b: Typed ArchiveOutcome

### Task 7: `ArchiveOutcome` enum + updated `Archive` signature

**Files:**
- Modify: `internal/apply/apply.go`
- Modify: `internal/apply/apply_test.go`

**Step 1: Write the failing test**

```go
func TestArchive_WritesOutcomeAndTimestamp(t *testing.T) {
    tmp := t.TempDir()
    old := store.RootOverrideForTest(tmp)
    defer store.ResetRootOverrideForTest(old)

    // Create proposals dir and a minimal proposal file.
    proposalsDir := filepath.Join(tmp, "proposals")
    archivedDir := filepath.Join(tmp, "proposals", "archived")
    os.MkdirAll(proposalsDir, 0o755)
    os.MkdirAll(archivedDir, 0o755)

    proposalID := "prop-test01-1"
    content := `{"sessionId":"sess-1","proposal":{"id":"prop-test01-1","type":"skill_improvement","confidence":"high","target":"~/.claude/SKILL.md","change":"test","rationale":"test"}}`
    os.WriteFile(filepath.Join(proposalsDir, proposalID+".json"), []byte(content), 0o644)

    before := time.Now()
    if err := Archive(proposalID, OutcomeRejected, "not relevant"); err != nil {
        t.Fatalf("Archive: %v", err)
    }
    after := time.Now()

    // Read archived file and verify fields.
    archivedPath := filepath.Join(archivedDir, proposalID+".json")
    data, err := os.ReadFile(archivedPath)
    if err != nil {
        t.Fatalf("archived file not found: %v", err)
    }

    var result map[string]json.RawMessage
    if err := json.Unmarshal(data, &result); err != nil {
        t.Fatalf("parsing archived: %v", err)
    }

    var outcome string
    json.Unmarshal(result["outcome"], &outcome)
    if outcome != string(OutcomeRejected) {
        t.Errorf("outcome = %q, want %q", outcome, OutcomeRejected)
    }

    var archivedAt time.Time
    json.Unmarshal(result["archivedAt"], &archivedAt)
    if archivedAt.Before(before) || archivedAt.After(after) {
        t.Errorf("archivedAt %v outside expected range", archivedAt)
    }

    // archiveReason must NOT be written by new code.
    if _, ok := result["archiveReason"]; ok {
        t.Error("archiveReason should not be written by new code")
    }
}

func TestReadArchiveOutcome_MigratesOldReasons(t *testing.T) {
    cases := []struct {
        reason  string
        want    ArchiveOutcome
    }{
        {"approved", OutcomeApproved},
        {"rejected", OutcomeRejected},
        {"rejected: not useful", OutcomeRejected},
        {"deferred", OutcomeDeferred},
        {"auto-culled: already applied to target", OutcomeCulled},
        {"auto-culled: synthesized into prop-abc-1", OutcomeCulled},
    }
    for _, c := range cases {
        raw := map[string]json.RawMessage{
            "archiveReason": json.RawMessage(`"` + c.reason + `"`),
        }
        got := readArchiveOutcome(raw)
        if got != c.want {
            t.Errorf("readArchiveOutcome(%q) = %q, want %q", c.reason, got, c.want)
        }
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/apply/ -run TestArchive_WritesOutcome -v
go test ./internal/apply/ -run TestReadArchiveOutcome -v
```

**Step 3: Implement**

In `internal/apply/apply.go`, add before `Archive`:
```go
// ArchiveOutcome is the typed outcome of a proposal archival.
type ArchiveOutcome string

const (
    OutcomeApproved     ArchiveOutcome = "approved"
    OutcomeRejected     ArchiveOutcome = "rejected"
    OutcomeCulled       ArchiveOutcome = "culled"        // curator rank-cull
    OutcomeAutoRejected ArchiveOutcome = "auto-rejected" // curator already-applied
    OutcomeDeferred     ArchiveOutcome = "deferred"
)

// readArchiveOutcome migrates old free-text archiveReason strings to ArchiveOutcome.
// Returns OutcomeRejected for unrecognised strings (safe default).
func readArchiveOutcome(raw map[string]json.RawMessage) ArchiveOutcome {
    reasonRaw, ok := raw["archiveReason"]
    if !ok {
        return OutcomeRejected // unknown
    }
    var reason string
    json.Unmarshal(reasonRaw, &reason)
    switch {
    case reason == "approved":
        return OutcomeApproved
    case reason == "deferred":
        return OutcomeDeferred
    case strings.HasPrefix(reason, "rejected"):
        return OutcomeRejected
    case strings.HasPrefix(reason, "auto-culled"):
        return OutcomeCulled
    default:
        return OutcomeRejected
    }
}
```

Update `Archive` signature:
```go
// Archive moves a proposal to proposals/archived/ with a typed outcome.
// note is an optional human-written reason (empty string for curator calls).
// archiveReason is NOT written; outcome + archivedAt replace it.
func Archive(proposalID string, outcome ArchiveOutcome, note string) error {
```

In the body, replace the `archiveReason` write with:
```go
outcomeJSON, _ := json.Marshal(string(outcome))
raw["outcome"] = outcomeJSON

archivedAtJSON, _ := json.Marshal(time.Now())
raw["archivedAt"] = archivedAtJSON

// Do NOT write "archiveReason" — reads use readArchiveOutcome for migration.
delete(raw, "archiveReason")  // remove if present from old data
```

**Step 4: Fix all callers** (compilation will fail until done):

- `internal/cmd/approve.go:67`: `apply.Archive(p.ID, apply.OutcomeApproved, "")`
- `internal/cmd/reject.go:50–55`: `apply.Archive(p.ID, apply.OutcomeRejected, reason)` where `reason` is the user-provided note (was appended to string; now it's the `note` param)
- `internal/tui/model.go:180`: `apply.Archive(proposalID, apply.OutcomeApproved, "")`
- `internal/tui/model.go:200`: `apply.Archive(proposalID, apply.OutcomeRejected, "")`
- `internal/tui/model.go:220`: `apply.Archive(proposalID, apply.OutcomeDeferred, "")`
- `internal/daemon/cleanup.go:98`: `apply.Archive(cd.ProposalID, apply.OutcomeAutoRejected, "already applied to target")`
- `internal/daemon/cleanup.go:214–225`: use `OutcomeCulled` with the reason as note

**Step 5: Run tests**
```bash
go build ./...
go test ./internal/apply/ -v
go test ./...
```

**Step 6: Commit**
```bash
git add internal/apply/apply.go internal/apply/apply_test.go \
        internal/cmd/approve.go internal/cmd/reject.go \
        internal/tui/model.go internal/daemon/cleanup.go
git commit -m "feat(apply): typed ArchiveOutcome enum with archivedAt timestamp, migrate archiveReason on read"
```

---

## Component 3c: Outcome Stats + History↔Archive Join

### Task 8: `GatherPipelineStatsFromSessions` counts archived outcomes

**Files:**
- Modify: `internal/pipeline/run.go`
- Modify: `internal/pipeline/run_test.go`

**Step 1: Write the failing test**

Look at the existing `TestGatherPipelineStats*` tests. Add:
```go
func TestGatherPipelineStatsFromSessions_CountsArchivedOutcomes(t *testing.T) {
    tmp := t.TempDir()
    old := store.RootOverrideForTest(tmp)
    defer store.ResetRootOverrideForTest(old)
    store.Init()

    archivedDir := filepath.Join(tmp, "proposals", "archived")
    os.MkdirAll(archivedDir, 0o755)

    now := time.Now()
    // Write two archived proposals: one approved, one rejected.
    writeArchivedProp := func(id, outcome string) {
        data := fmt.Sprintf(`{"sessionId":"sess-1","outcome":%q,"archivedAt":%q}`,
            outcome, now.Format(time.RFC3339))
        os.WriteFile(filepath.Join(archivedDir, id+".json"), []byte(data), 0o644)
    }
    writeArchivedProp("prop-aaa-1", "approved")
    writeArchivedProp("prop-bbb-1", "rejected")

    stats, err := GatherPipelineStatsFromSessions(nil, nil, 30)
    if err != nil {
        t.Fatalf("GatherPipelineStatsFromSessions: %v", err)
    }
    if stats.ProposalsApproved != 1 {
        t.Errorf("ProposalsApproved = %d, want 1", stats.ProposalsApproved)
    }
    if stats.ProposalsRejected != 1 {
        t.Errorf("ProposalsRejected = %d, want 1", stats.ProposalsRejected)
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/pipeline/ -run TestGatherPipelineStatsFromSessions_CountsArchived -v
```
Expected: FAIL (`ProposalsApproved` and `ProposalsRejected` are always 0).

**Step 3: Implement**

In `internal/pipeline/run.go`, in `GatherPipelineStatsFromSessions`, after the existing proposal counting, add:
```go
// Count archived proposal outcomes within the time window.
archivedDir := store.ArchivedProposalsDir()
if entries, err := os.ReadDir(archivedDir); err == nil {
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
            continue
        }
        data, err := os.ReadFile(filepath.Join(archivedDir, entry.Name()))
        if err != nil {
            continue
        }
        var raw map[string]json.RawMessage
        if err := json.Unmarshal(data, &raw); err != nil {
            continue
        }
        // Check archivedAt is within window.
        if atRaw, ok := raw["archivedAt"]; ok {
            var at time.Time
            if err := json.Unmarshal(atRaw, &at); err == nil && at.Before(cutoff) {
                continue
            }
        }
        // Determine outcome (new field, or migrate from archiveReason).
        outcome := readArchivedOutcome(raw)
        switch outcome {
        case "approved":
            stats.ProposalsApproved++
        case "rejected", "culled", "auto-rejected":
            stats.ProposalsRejected++
        }
    }
}
```

Add a package-local `readArchivedOutcome` helper (mirrors `apply.readArchiveOutcome` but without the import cycle — keep it simple):
```go
func readArchivedOutcome(raw map[string]json.RawMessage) string {
    if outcomeRaw, ok := raw["outcome"]; ok {
        var o string
        json.Unmarshal(outcomeRaw, &o)
        if o != "" {
            return o
        }
    }
    // Migration: read old archiveReason.
    if reasonRaw, ok := raw["archiveReason"]; ok {
        var reason string
        json.Unmarshal(reasonRaw, &reason)
        switch {
        case reason == "approved":
            return "approved"
        case strings.HasPrefix(reason, "rejected"):
            return "rejected"
        case strings.HasPrefix(reason, "auto-culled"):
            return "culled"
        case reason == "deferred":
            return "deferred"
        }
    }
    return ""
}
```

Add required imports: `"encoding/json"`, `"strings"`, `"os"`.

**Step 4: Run tests**
```bash
go test ./internal/pipeline/ -run TestGatherPipelineStats -v
go test ./...
```

**Step 5: Commit**
```bash
git add internal/pipeline/run.go internal/pipeline/run_test.go
git commit -m "feat(pipeline): GatherPipelineStatsFromSessions counts archived proposal outcomes"
```

---

### Task 9: `AcceptanceStats` and `ListAcceptanceRateByPromptVersion`

**Files:**
- Modify: `internal/pipeline/run.go`
- Modify: `internal/pipeline/run_test.go`

**Step 1: Write the failing test**

```go
func TestListAcceptanceRateByPromptVersion(t *testing.T) {
    tmp := t.TempDir()
    old := store.RootOverrideForTest(tmp)
    defer store.ResetRootOverrideForTest(old)
    store.Init()

    // Write history: session sess-1 used evaluator-v3, generated 2 proposals.
    histPath := filepath.Join(tmp, "run_history.jsonl")
    now := time.Now()
    rec := HistoryRecord{
        SessionID:              "sess-001",
        Timestamp:              now,
        Status:                 "processed",
        EvaluatorPromptVersion: "evaluator-v3",
        ProposalCount:          2,
    }
    data, _ := json.Marshal(rec)
    os.WriteFile(histPath, append(data, '\n'), 0o644)

    // Write archived proposals for sess-001: 1 approved, 1 rejected.
    archivedDir := filepath.Join(tmp, "proposals", "archived")
    os.MkdirAll(archivedDir, 0o755)
    writeArchived := func(id, outcome string) {
        d := fmt.Sprintf(`{"sessionId":"sess-001","outcome":%q,"archivedAt":%q}`,
            outcome, now.Format(time.RFC3339))
        os.WriteFile(filepath.Join(archivedDir, id+".json"), []byte(d), 0o644)
    }
    writeArchived("prop-001-1", "approved")
    writeArchived("prop-001-2", "rejected")

    stats, err := ListAcceptanceRateByPromptVersion()
    if err != nil {
        t.Fatalf("ListAcceptanceRateByPromptVersion: %v", err)
    }
    if len(stats) != 1 {
        t.Fatalf("len(stats) = %d, want 1", len(stats))
    }
    s := stats[0]
    if s.PromptVersion != "evaluator-v3" {
        t.Errorf("PromptVersion = %q", s.PromptVersion)
    }
    if s.Approved != 1 || s.Rejected != 1 {
        t.Errorf("Approved=%d Rejected=%d, want 1/1", s.Approved, s.Rejected)
    }
    if s.SampleSize != 2 {
        t.Errorf("SampleSize = %d, want 2", s.SampleSize)
    }
    // AcceptanceRate = 1/2 = 0.5
    if s.AcceptanceRate < 0.49 || s.AcceptanceRate > 0.51 {
        t.Errorf("AcceptanceRate = %f, want ~0.5", s.AcceptanceRate)
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/pipeline/ -run TestListAcceptanceRate -v
```

**Step 3: Implement**

In `internal/pipeline/run.go`, add:
```go
// AcceptanceStats holds acceptance rate data for one evaluator prompt version.
type AcceptanceStats struct {
    PromptVersion  string
    Generated      int
    Approved       int
    Rejected       int
    AcceptanceRate float64 // Approved/(Approved+Rejected); math.NaN() if SampleSize==0
    SampleSize     int     // Approved + Rejected (excludes culled/deferred)
}

// ListAcceptanceRateByPromptVersion joins run history with archived proposals
// to compute acceptance rates per evaluator prompt version.
// Returns one entry per version with ≥1 archived proposal, sorted most-recently-active first.
func ListAcceptanceRateByPromptVersion() ([]AcceptanceStats, error) {
    records, err := ReadHistory()
    if err != nil {
        return nil, fmt.Errorf("reading history: %w", err)
    }

    // Build session → evaluator prompt version map and generated count.
    type versionData struct {
        lastSeen  time.Time
        generated int
        approved  int
        rejected  int
    }
    byVersion := make(map[string]*versionData)
    sessionToVersion := make(map[string]string)
    for _, r := range records {
        if r.EvaluatorPromptVersion == "" {
            continue
        }
        sessionToVersion[r.SessionID] = r.EvaluatorPromptVersion
        vd := byVersion[r.EvaluatorPromptVersion]
        if vd == nil {
            vd = &versionData{}
            byVersion[r.EvaluatorPromptVersion] = vd
        }
        vd.generated += r.ProposalCount
        if r.Timestamp.After(vd.lastSeen) {
            vd.lastSeen = r.Timestamp
        }
    }

    // Scan archived proposals, join on sessionId.
    archivedDir := store.ArchivedProposalsDir()
    entries, _ := os.ReadDir(archivedDir)
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
            continue
        }
        data, err := os.ReadFile(filepath.Join(archivedDir, entry.Name()))
        if err != nil {
            continue
        }
        var raw map[string]json.RawMessage
        if json.Unmarshal(data, &raw) != nil {
            continue
        }
        var sessionID string
        json.Unmarshal(raw["sessionId"], &sessionID)
        version := sessionToVersion[sessionID]
        if version == "" {
            continue
        }
        vd := byVersion[version]
        if vd == nil {
            continue
        }
        outcome := readArchivedOutcome(raw)
        switch outcome {
        case "approved":
            vd.approved++
        case "rejected":
            vd.rejected++
        // culled/auto-rejected/deferred excluded from SampleSize
        }
    }

    // Build results, filter to versions with ≥1 archived proposal.
    var result []AcceptanceStats
    for version, vd := range byVersion {
        sampleSize := vd.approved + vd.rejected
        if sampleSize == 0 {
            continue
        }
        rate := math.NaN()
        if sampleSize > 0 {
            rate = float64(vd.approved) / float64(sampleSize)
        }
        result = append(result, AcceptanceStats{
            PromptVersion:  version,
            Generated:      vd.generated,
            Approved:       vd.approved,
            Rejected:       vd.rejected,
            AcceptanceRate: rate,
            SampleSize:     sampleSize,
        })
    }
    // Sort by lastSeen descending (most recently active version first).
    sort.Slice(result, func(i, j int) bool {
        vi := byVersion[result[i].PromptVersion]
        vj := byVersion[result[j].PromptVersion]
        return vi.lastSeen.After(vj.lastSeen)
    })
    return result, nil
}
```

Add `"math"` and `"sort"` to imports.

**Step 4: Run tests**
```bash
go test ./internal/pipeline/ -run TestListAcceptanceRate -v
go test ./...
```

**Step 5: Commit**
```bash
git add internal/pipeline/run.go internal/pipeline/run_test.go
git commit -m "feat(pipeline): add AcceptanceStats and ListAcceptanceRateByPromptVersion"
```

---

## Component 4: Daily Meta-Pipeline

### Task 10: New proposal types + curator preservation

**Files:**
- Modify: `internal/pipeline/proposal.go`
- Modify: `internal/pipeline/curator.go` (preservation list)
- Modify: `internal/tui/detail/view.go` (META badge)

**Step 1: Write failing test**

```go
// In internal/pipeline/curator_test.go or proposal_test.go:
func TestNewProposalTypes_Defined(t *testing.T) {
    // Verify string values match what the design specifies.
    if TypePromptImprovement != "prompt_improvement" {
        t.Errorf("TypePromptImprovement = %q", TypePromptImprovement)
    }
    if TypePipelineInsight != "pipeline_insight" {
        t.Errorf("TypePipelineInsight = %q", TypePipelineInsight)
    }
}
```

For the META badge in the detail view:
```go
// In internal/tui/detail/view_test.go or model_test.go:
func TestSubHeader_MetaBadgeForPromptImprovement(t *testing.T) {
    // build a Model with a prompt_improvement proposal, check SubHeader contains META
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/pipeline/ -run TestNewProposalTypes -v
```

**Step 3: Implement — new types**

In `internal/pipeline/proposal.go`, add alongside existing type constants:
```go
const (
    TypePromptImprovement = "prompt_improvement" // meta-pipeline: prompt file edits
    TypePipelineInsight   = "pipeline_insight"   // pure-code observation, no change field
)
```

**Step 4: Implement — curator preservation**

In `internal/pipeline/curator.go`, find where `skill_scaffold` is preserved (exempted from culling). Add the two new types there:
```go
// Preserved types are never synthesised or culled by the curator.
var preservedTypes = map[string]bool{
    "skill_scaffold":    true,
    "prompt_improvement": true,
    "pipeline_insight":  true,
}
```
(The exact implementation depends on how the curator currently checks this — read the relevant section of `curator.go` before editing.)

**Step 5: Implement — META badge**

In `internal/tui/detail/view.go`, `SubHeader()`:
```go
func (m Model) SubHeader() string {
    if m.proposal == nil {
        return shared.RenderSubHeader("  Proposal Detail", "")
    }
    p := &m.proposal.Proposal
    badge := ""
    if p.Type == "prompt_improvement" || p.Type == "pipeline_insight" {
        badge = "  [META]"
    }
    statsLine := fmt.Sprintf("  %s%s  ·  %s  ·  %s", p.Type, badge, cli.ShortenHome(p.Target), p.Confidence)
    return shared.RenderSubHeader("  Proposal Detail", statsLine)
}
```

**Step 6: Run tests**
```bash
go test ./internal/pipeline/ -v
go test ./internal/tui/detail/ -v
go test ./...
```

**Step 7: Commit**
```bash
git add internal/pipeline/proposal.go internal/pipeline/curator.go internal/tui/detail/view.go
git commit -m "feat(pipeline): add prompt_improvement and pipeline_insight proposal types, META badge in detail view"
```

---

### Task 11: Meta-pipeline config + `EnsureMetaPrompts`

**Files:**
- Modify: `internal/pipeline/prompts.go`
- Modify: `internal/daemon/daemon.go` (add `MetaInterval` to `Config`)

**Step 1: Write failing test**

```go
func TestEnsureMetaPrompts_CreatesMetaV1(t *testing.T) {
    tmp := t.TempDir()
    old := store.RootOverrideForTest(tmp)
    defer store.ResetRootOverrideForTest(old)
    store.Init()

    if err := EnsureMetaPrompts(); err != nil {
        t.Fatalf("EnsureMetaPrompts: %v", err)
    }
    path := filepath.Join(tmp, "prompts", "meta-v1.txt")
    if _, err := os.Stat(path); os.IsNotExist(err) {
        t.Errorf("meta-v1.txt not created at %s", path)
    }
    data, _ := os.ReadFile(path)
    // Verify it contains key instruction phrases.
    if !strings.Contains(string(data), "prompt_improvement") {
        t.Error("meta prompt should mention prompt_improvement")
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/pipeline/ -run TestEnsureMetaPrompts -v
```

**Step 3: Implement — `EnsureMetaPrompts` in `prompts.go`**

Add alongside `EnsureCuratorPrompts`:
```go
const metaPromptFile = "meta-v1.txt"

const metaPromptV1 = `You are the Cabrero meta-analyst. Your role is to improve the evaluator prompt by analysing patterns in rejected proposals.

You will be provided with:
- The current evaluator prompt file (read it with the Read tool)
- The most recently rejected proposals for the target evaluator prompt version
- Acceptance statistics for that version
- Paths to CC session transcripts for the highest-turn rejected runs (read them with Read/Grep)

## Instructions

1. Read the target prompt file.
2. Read the provided rejected proposals. Identify the common pattern: what did the evaluator consistently propose that humans rejected? (Too aggressive? Wrong scope? Wrong type? Insufficient evidence?)
3. Read the CC session transcripts for the highest-turn rejected runs to understand what evidence the evaluator was working from. If a transcript path is provided but the file does not exist, skip it and note this in your rationale.
4. Produce ONE specific proposed edit to the prompt that addresses the identified pattern. The edit must be concrete: specify exact text to add, remove, or modify — not "consider revising".
5. If the rejection pattern is ambiguous or the evidence is insufficient to identify a clear cause, emit a pipeline_insight instead of a prompt_improvement.
6. Do NOT speculate. Only propose changes with clear evidence from the provided data.

## Output format

Emit a single JSON object:
{
  "type": "prompt_improvement" | "pipeline_insight",
  "target": "<path to the prompt file>",
  "change": "<exact text change — null for pipeline_insight>",
  "rationale": "<the rejection pattern observed, citing specific proposal IDs and session IDs>",
  "citedUuids": ["<proposal IDs and session IDs used as evidence>"]
}
`

// EnsureMetaPrompts writes meta-v1.txt if it does not already exist.
// Same pattern as EnsureCuratorPrompts.
func EnsureMetaPrompts() error {
    promptsDir := filepath.Join(store.Root(), "prompts")
    if err := os.MkdirAll(promptsDir, 0o755); err != nil {
        return fmt.Errorf("creating prompts dir: %w", err)
    }
    path := filepath.Join(promptsDir, metaPromptFile)
    if _, err := os.Stat(path); err == nil {
        return nil // already exists
    }
    return store.AtomicWrite(path, []byte(metaPromptV1), 0o644)
}
```

**Step 4: Add `MetaInterval` to daemon config**

In `internal/daemon/daemon.go`, `Config`:
```go
MetaInterval time.Duration // how often to run the meta-pipeline (default 24h)
```

In `DefaultConfig()`:
```go
MetaInterval: 24 * time.Hour,
```

**Step 5: Run tests**
```bash
go test ./internal/pipeline/ -run TestEnsureMetaPrompts -v
go build ./...
go test ./...
```

**Step 6: Commit**
```bash
git add internal/pipeline/prompts.go internal/daemon/daemon.go
git commit -m "feat(pipeline): add EnsureMetaPrompts with meta-v1.txt, MetaInterval daemon config"
```

---

### Task 12: `ComputePipelineMetrics`

**Files:**
- Create: `internal/pipeline/meta.go`
- Create: `internal/pipeline/meta_test.go`

**Step 1: Write the failing test**

```go
package pipeline

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/vladolaru/cabrero/internal/store"
)

func TestComputePipelineMetrics_ClassifierFPR(t *testing.T) {
    tmp := t.TempDir()
    old := store.RootOverrideForTest(tmp)
    defer store.ResetRootOverrideForTest(old)
    store.Init()

    // Write history: 4 sessions sent to evaluator, 2 generated 0 proposals (FP).
    histPath := filepath.Join(tmp, "run_history.jsonl")
    now := time.Now()
    writeRec := func(sessionID, triage string, proposals int) {
        r := HistoryRecord{
            SessionID:              sessionID,
            Timestamp:              now,
            Status:                 "processed",
            Triage:                 triage,
            ProposalCount:          proposals,
            EvaluatorPromptVersion: "evaluator-v4",
        }
        if triage == "evaluate" {
            r.EvaluatorModel = DefaultEvaluatorModel
        }
        data, _ := json.Marshal(r)
        f, _ := os.OpenFile(histPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
        f.Write(append(data, '\n'))
        f.Close()
    }
    writeRec("sess-1", "evaluate", 2)
    writeRec("sess-2", "evaluate", 0) // FP
    writeRec("sess-3", "evaluate", 1)
    writeRec("sess-4", "evaluate", 0) // FP
    writeRec("sess-5", "clean", 0)    // not sent to evaluator

    cfg := DefaultPipelineConfig()
    metrics, err := ComputePipelineMetrics(cfg)
    if err != nil {
        t.Fatalf("ComputePipelineMetrics: %v", err)
    }
    // 2 FP out of 4 evaluate sessions = 0.50
    if metrics.ClassifierFPR < 0.49 || metrics.ClassifierFPR > 0.51 {
        t.Errorf("ClassifierFPR = %f, want ~0.50", metrics.ClassifierFPR)
    }
    if metrics.ClassifierFPRWindow != 30 {
        t.Errorf("ClassifierFPRWindow = %d, want 30", metrics.ClassifierFPRWindow)
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/pipeline/ -run TestComputePipelineMetrics -v
```

**Step 3: Implement `internal/pipeline/meta.go`**

```go
package pipeline

import (
    "math"
    "sort"
    "time"
)

// PipelineMetrics holds computed quality metrics for the pipeline.
type PipelineMetrics struct {
    AcceptanceByVersion []AcceptanceStats

    ClassifierFPR       float64 // evaluate→zero-proposals / total evaluate sessions
    ClassifierFPRWindow int     // days of history used

    ClassifierMedianTurns float64
    EvaluatorMedianTurns  float64

    CostPerAcceptedProposal float64

    ComputedAt time.Time
}

// ComputePipelineMetrics reads run history and archived proposals to compute
// pipeline quality metrics. No LLM calls.
func ComputePipelineMetrics(cfg PipelineConfig) (PipelineMetrics, error) {
    const window = 30
    cutoff := time.Now().AddDate(0, 0, -window)

    records, err := ReadHistory()
    if err != nil {
        return PipelineMetrics{}, err
    }

    var evaluateSessions, fpSessions int
    var classifierTurns, evaluatorTurns []float64
    var totalCost float64
    var totalApproved int

    for _, r := range records {
        if r.Timestamp.Before(cutoff) {
            continue
        }
        if r.Triage == "evaluate" {
            evaluateSessions++
            if r.ProposalCount == 0 {
                fpSessions++
            }
            if r.ClassifierUsage.NumTurns > 0 {
                classifierTurns = append(classifierTurns, float64(r.ClassifierUsage.NumTurns))
            }
            if r.EvaluatorUsage.NumTurns > 0 {
                evaluatorTurns = append(evaluatorTurns, float64(r.EvaluatorUsage.NumTurns))
            }
            totalCost += r.TotalCostUSD
        }
    }

    fpr := math.NaN()
    if evaluateSessions > 0 {
        fpr = float64(fpSessions) / float64(evaluateSessions)
    }

    acceptanceByVersion, _ := ListAcceptanceRateByPromptVersion()

    // Count total approved for cost-per-accepted-proposal.
    for _, a := range acceptanceByVersion {
        totalApproved += a.Approved
    }
    costPerAccepted := math.NaN()
    if totalApproved > 0 {
        costPerAccepted = totalCost / float64(totalApproved)
    }

    return PipelineMetrics{
        AcceptanceByVersion:     acceptanceByVersion,
        ClassifierFPR:           fpr,
        ClassifierFPRWindow:     window,
        ClassifierMedianTurns:   median(classifierTurns),
        EvaluatorMedianTurns:    median(evaluatorTurns),
        CostPerAcceptedProposal: costPerAccepted,
        ComputedAt:              time.Now(),
    }, nil
}

func median(vals []float64) float64 {
    if len(vals) == 0 {
        return math.NaN()
    }
    sorted := make([]float64, len(vals))
    copy(sorted, vals)
    sort.Float64s(sorted)
    mid := len(sorted) / 2
    if len(sorted)%2 == 0 {
        return (sorted[mid-1] + sorted[mid]) / 2
    }
    return sorted[mid]
}
```

**Step 4: Run tests**
```bash
go test ./internal/pipeline/ -run TestComputePipelineMetrics -v
go test ./...
```

**Step 5: Commit**
```bash
git add internal/pipeline/meta.go internal/pipeline/meta_test.go
git commit -m "feat(pipeline): add ComputePipelineMetrics with classifier FPR and acceptance-by-version"
```

---

### Task 13: `RunMetaAnalysis` with transcript validation

**Files:**
- Modify: `internal/pipeline/meta.go`
- Modify: `internal/pipeline/meta_test.go`

**Step 1: Write failing test**

```go
func TestRunMetaAnalysis_SkipsMissingTranscripts(t *testing.T) {
    // Verify that RunMetaAnalysis logs a warning and skips transcripts
    // that don't exist rather than failing.
    var warnings []string
    logger := &capturingLogger{errorFn: func(msg string) { warnings = append(warnings, msg) }}

    cfg := DefaultPipelineConfig()
    cfg.Logger = logger

    // Non-existent transcript path — should warn and skip.
    transcripts := []string{"/nonexistent/path/fake-uuid.jsonl"}
    valid := filterValidTranscripts(transcripts, logger)
    if len(valid) != 0 {
        t.Errorf("expected 0 valid transcripts, got %d", len(valid))
    }
    if len(warnings) == 0 {
        t.Error("expected a warning for missing transcript")
    }
    if !strings.Contains(warnings[0], "CC storage conventions may have changed") {
        t.Errorf("unexpected warning: %q", warnings[0])
    }
}

func TestRunMetaAnalysis_SkipsTranscriptWithNoToolUse(t *testing.T) {
    tmp := t.TempDir()

    // Write a transcript with no tool_use entries.
    transcriptPath := filepath.Join(tmp, "no-tools.jsonl")
    content := `{"type":"user","message":{"role":"user","content":"hello"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"world"}]}}`
    os.WriteFile(transcriptPath, []byte(content), 0o644)

    var warnings []string
    logger := &capturingLogger{errorFn: func(msg string) { warnings = append(warnings, msg) }}

    valid := filterValidTranscripts([]string{transcriptPath}, logger)
    if len(valid) != 0 {
        t.Errorf("expected 0 valid transcripts, got %d", len(valid))
    }
    if len(warnings) == 0 {
        t.Error("expected a warning for no tool_use entries")
    }
    if !strings.Contains(warnings[0], "no tool_use entries") {
        t.Errorf("unexpected warning: %q", warnings[0])
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/pipeline/ -run TestRunMetaAnalysis -v
```

**Step 3: Implement `filterValidTranscripts` and `RunMetaAnalysis`**

Add to `internal/pipeline/meta.go`:

```go
// filterValidTranscripts checks each path: logs a warning and skips if
// the file is not found or contains no tool_use entries.
func filterValidTranscripts(paths []string, log Logger) []string {
    var valid []string
    for _, p := range paths {
        data, err := os.ReadFile(p)
        if err != nil {
            log.Error("WARN transcript not found for cc_session_id at expected path %s — CC storage conventions may have changed", p)
            continue
        }
        if !transcriptHasToolUse(data) {
            log.Error("WARN transcript %s contains no tool_use entries — CC format may have changed or this is a print-mode session", p)
            continue
        }
        valid = append(valid, p)
    }
    return valid
}

func transcriptHasToolUse(data []byte) bool {
    // Scan JSONL lines for "tool_use" in content blocks.
    // Simple string search is sufficient — avoids full parse overhead.
    return strings.Contains(string(data), `"tool_use"`)
}

// ccProjectsDir returns the path to ~/.claude/projects/.
func ccProjectsDir() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(home, ".claude", "projects"), nil
}

// transcriptPathForSession constructs the expected CC transcript path for a
// given cc_session_id. CC stores transcripts as:
//   ~/.claude/projects/<cwd-slug>/<cc_session_id>.jsonl
// where cwd-slug is the daemon's working directory slugified by CC's convention.
// The cwd-slug for the default daemon cwd (~/.cabrero/) is resolved at runtime.
func transcriptPathForSession(ccSessionID string) (string, error) {
    projectsDir, err := ccProjectsDir()
    if err != nil {
        return "", err
    }
    // Resolve the cwd-slug by scanning for the session ID in all project dirs.
    entries, err := os.ReadDir(projectsDir)
    if err != nil {
        return "", err
    }
    for _, proj := range entries {
        if !proj.IsDir() {
            continue
        }
        candidate := filepath.Join(projectsDir, proj.Name(), ccSessionID+".jsonl")
        if _, err := os.Stat(candidate); err == nil {
            return candidate, nil
        }
    }
    return "", fmt.Errorf("transcript for %s not found in %s", ccSessionID, projectsDir)
}

// RunMetaAnalysis fires only when a version crosses a threshold with sufficient
// samples and no recent meta-proposal exists for that version.
// Invokes Opus via the meta prompt with rejected proposals and CC transcripts.
// Returns the proposal ID written to the queue, or "" if no action was taken.
func RunMetaAnalysis(stats AcceptanceStats, cfg PipelineConfig) (string, error) {
    log := cfg.logger()

    if err := EnsureMetaPrompts(); err != nil {
        return "", fmt.Errorf("ensuring meta prompts: %w", err)
    }

    systemPrompt, err := readPromptTemplate(metaPromptFile)
    if err != nil {
        return "", fmt.Errorf("reading meta prompt: %w", err)
    }

    // Gather the 10 most-recently rejected proposals for this version.
    rejectedProps, err := listRejectedProposalsForVersion(stats.PromptVersion, 10)
    if err != nil {
        log.Error("meta: listing rejected proposals: %v", err)
    }

    // Gather CC session transcripts for the 3 highest-turn rejected runs.
    transcriptPaths := highTurnTranscriptPaths(stats.PromptVersion, 3)
    validTranscripts := filterValidTranscripts(transcriptPaths, log)

    // Build the user prompt from the data gathered.
    userPrompt := buildMetaPrompt(stats, rejectedProps, validTranscripts)

    // Determine allowed tools: Read + Grep scoped to home dir.
    home, _ := os.UserHomeDir()
    allowedTools := fmt.Sprintf("Read(//%s/**),Grep(//%s/**)", home, home)

    cr, err := invokeClaude(claudeConfig{
        Model:          cfg.MetaModel,
        SystemPrompt:   systemPrompt,
        Agentic:        true,
        Prompt:         userPrompt,
        AllowedTools:   allowedTools,
        MaxTurns:       cfg.MetaMaxTurns,
        Timeout:        cfg.MetaTimeout,
        Debug:          cfg.Debug,
        Logger:         log,
        SettingSources: &emptyStr,
    })
    if err != nil {
        return "", fmt.Errorf("meta invocation: %w", err)
    }

    // Parse the output proposal.
    cleaned := cleanLLMJSON(cr.Result)
    proposal, err := parseMetaProposal(cleaned, stats.PromptVersion)
    if err != nil {
        return "", fmt.Errorf("parsing meta output: %w", err)
    }

    propID, err := WriteProposal(proposal, "meta")
    if err != nil {
        return "", fmt.Errorf("writing meta proposal: %w", err)
    }
    log.Info("meta: wrote %s proposal %s for %s", proposal.Type, propID, stats.PromptVersion)
    return propID, nil
}
```

Add helpers `listRejectedProposalsForVersion`, `highTurnTranscriptPaths`, `buildMetaPrompt`, `parseMetaProposal`. These read from `proposals/archived/` and `run_history.jsonl` — follow existing patterns in `run.go`.

**Step 4: Run tests**
```bash
go test ./internal/pipeline/ -run TestRunMetaAnalysis -v
go test ./...
```

**Step 5: Commit**
```bash
git add internal/pipeline/meta.go internal/pipeline/meta_test.go
git commit -m "feat(pipeline): RunMetaAnalysis with transcript validation and Opus invocation"
```

---

### Task 14: `performMetaRun` in daemon + `metaTicker`

**Files:**
- Create: `internal/daemon/meta.go`
- Modify: `internal/daemon/daemon.go`

**Step 1: Write failing test**

```go
// In internal/daemon/meta_test.go:
func TestPerformMetaRun_SkipsWhenBelowThreshold(t *testing.T) {
    // If no version crosses the threshold, performMetaRun should be a no-op.
    // Use a real metrics object with low FPR and no acceptance stats.
    d := &Daemon{
        config: DefaultConfig(),
        log:    newTestLogger(t),
    }
    d.config.Pipeline.MetaMinSamples = 5
    d.config.Pipeline.MetaRejectionRateThreshold = 0.30
    // ComputePipelineMetrics will find no history → NaN FPR, empty versions.
    // Should log one line and return without error or LLM call.
    d.performMetaRun(context.Background()) // must not panic or call LLM
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/daemon/ -run TestPerformMetaRun -v
```

**Step 3: Implement `internal/daemon/meta.go`**

```go
package daemon

import (
    "context"
    "math"

    "github.com/vladolaru/cabrero/internal/pipeline"
)

// performMetaRun runs Stage 1 (metric computation) unconditionally.
// Stage 2 (Opus meta-analysis) fires only when a threshold is crossed with
// sufficient samples and no recent meta-proposal exists.
func (d *Daemon) performMetaRun(ctx context.Context) {
    d.log.Info("meta: computing pipeline metrics")

    metrics, err := pipeline.ComputePipelineMetrics(d.config.Pipeline)
    if err != nil {
        d.log.Error("meta: computing metrics: %v", err)
        return
    }

    cfg := d.config.Pipeline
    triggered := false

    // Check classifier FPR threshold.
    if !math.IsNaN(metrics.ClassifierFPR) &&
        metrics.ClassifierFPR >= cfg.MetaClassifierFPRThreshold {
        d.log.Info("meta: classifier FPR %.0f%% exceeds threshold %.0f%%",
            metrics.ClassifierFPR*100, cfg.MetaClassifierFPRThreshold*100)
        triggered = true
    }

    // Check per-version rejection rate.
    for _, stats := range metrics.AcceptanceByVersion {
        if stats.SampleSize < cfg.MetaMinSamples {
            continue
        }
        rejectionRate := 1.0 - stats.AcceptanceRate
        if math.IsNaN(rejectionRate) || rejectionRate < cfg.MetaRejectionRateThreshold {
            continue
        }
        if metaCooldownActive(stats.PromptVersion, cfg.MetaCooldownDays) {
            d.log.Info("meta: version %s above threshold but in cooldown period, skipping",
                stats.PromptVersion)
            continue
        }
        d.log.Info("meta: version %s rejection rate %.0f%% exceeds threshold %.0f%%",
            stats.PromptVersion, rejectionRate*100, cfg.MetaRejectionRateThreshold*100)
        triggered = true

        propID, err := pipeline.RunMetaAnalysis(stats, cfg)
        if err != nil {
            d.log.Error("meta: RunMetaAnalysis for %s: %v", stats.PromptVersion, err)
            continue
        }
        if propID != "" {
            d.log.Info("meta: proposal %s written for version %s", propID, stats.PromptVersion)
        }
    }

    if !triggered {
        d.log.Info("meta: no thresholds exceeded (classifier FPR: %.0f%%, %d versions checked)",
            metrics.ClassifierFPR*100, len(metrics.AcceptanceByVersion))
    }
}

// metaCooldownActive returns true if a prompt_improvement proposal for the
// given version was created within the last cooldownDays.
func metaCooldownActive(promptVersion string, cooldownDays int) bool {
    proposals, err := pipeline.ListProposals()
    if err != nil {
        return false
    }
    cutoff := pipeline.MetaCooldownCutoff(cooldownDays)
    for _, pw := range proposals {
        if pw.Proposal.Type != pipeline.TypePromptImprovement {
            continue
        }
        if pw.Proposal.Target == promptVersion || pw.Proposal.Rationale == promptVersion {
            if pipeline.ProposalCreatedAfter(pw, cutoff) {
                return true
            }
        }
    }
    return false
}
```

Add `MetaCooldownCutoff` and `ProposalCreatedAfter` helpers to `pipeline/meta.go` or `pipeline/proposal.go`.

**Step 4: Wire `metaTicker` into `daemon.go`**

In `internal/daemon/daemon.go`, `Run()`:
```go
metaTicker := time.NewTicker(d.config.MetaInterval)
defer metaTicker.Stop()
```

In the select loop:
```go
case <-metaTicker.C:
    d.performMetaRun(ctx)
```

**Step 5: Run tests**
```bash
go build ./...
go test ./internal/daemon/ -v
go test ./...
```

**Step 6: Commit**
```bash
git add internal/daemon/meta.go internal/daemon/daemon.go
git commit -m "feat(daemon): add performMetaRun and metaTicker for daily meta-pipeline"
```

---

### Task 15: METRICS section in Pipeline Monitor TUI

**Files:**
- Modify: `internal/tui/pipeline/view.go`
- Modify: `internal/tui/pipeline/model.go` (if metrics need to be stored in Model)

**Step 1: Write failing test**

```go
func TestRenderMetrics_ShowsAcceptanceRate(t *testing.T) {
    m := buildTestModelWithStats(t)
    // Set acceptance stats directly on the model.
    // Check that the rendered output contains the key metrics.
    rendered := ansi.Strip(m.renderMetrics())
    if !strings.Contains(rendered, "METRICS") {
        t.Error("missing METRICS section header")
    }
}
```

**Step 2: Run to verify failure**
```bash
go test ./internal/tui/pipeline/ -run TestRenderMetrics -v
```

**Step 3: Implement**

Add `pipelineMetrics pipeline.PipelineMetrics` field to `internal/tui/pipeline/model.go` Model struct. Load it alongside the pipeline stats refresh.

In `internal/tui/pipeline/view.go`, add:
```go
func (m Model) renderMetrics() string {
    var b strings.Builder
    b.WriteString(shared.RenderSectionHeader("METRICS"))
    b.WriteString("\n")

    pm := m.pipelineMetrics

    fprStr := "n/a"
    if !math.IsNaN(pm.ClassifierFPR) {
        fprStr = fmt.Sprintf("%.0f%%", pm.ClassifierFPR*100)
    }
    b.WriteString(fmt.Sprintf("  Classifier FPR: %s  (last %dd)\n", fprStr, pm.ClassifierFPRWindow))

    if len(pm.AcceptanceByVersion) > 0 {
        // Show the most recently active version.
        latest := pm.AcceptanceByVersion[0]
        rateStr := "n/a"
        if !math.IsNaN(latest.AcceptanceRate) {
            rateStr = fmt.Sprintf("%.0f%%", latest.AcceptanceRate*100)
        }
        b.WriteString(fmt.Sprintf("  Acceptance (%s): %s  (%d samples)\n",
            latest.PromptVersion, rateStr, latest.SampleSize))
    } else {
        b.WriteString("  Acceptance: no data yet\n")
    }

    if !pm.ComputedAt.IsZero() {
        b.WriteString(fmt.Sprintf("  Last meta run: %s", cli.RelativeTime(pm.ComputedAt)))
    }

    return b.String()
}
```

Add `"math"` to imports.

Wire `renderMetrics()` into the existing `View()` render pipeline, after the MODELS section.

**Step 4: Run tests**
```bash
go test ./internal/tui/pipeline/ -v
go test ./...
```

**Step 5: Update `make snapshots`**

Run `make snapshot VIEW=pipeline-monitor` to verify the TUI looks correct, then update the snapshot:
```bash
make snapshot VIEW=pipeline-monitor
```

**Step 6: Commit**
```bash
git add internal/tui/pipeline/
git commit -m "feat(tui): add METRICS section to Pipeline Monitor with FPR and acceptance rate"
```

---

## Final Verification

**Step 1: Full build and test**
```bash
go build ./...
go test ./...
```
Expected: clean build, all tests PASS.

**Step 2: Smoke test with live daemon**
```bash
cabrero doctor
cabrero status
```
Expected: all checks pass, daemon running.

**Step 3: Check all new log lines appear in daemon.log**
```bash
# Restart daemon to run startup rotations
cabrero setup --yes
sleep 3
tail -20 ~/.cabrero/daemon.log
```
Expected: blocklist rotation log line appears.

**Step 4: Update CHANGELOG.md and DESIGN.md**

Add `[Unreleased]` entry to `CHANGELOG.md` for all new features. Verify `DESIGN.md` reflects final implementation (esp. `MetaMinSamples=5`).

**Step 5: Update snapshots**
```bash
make snapshots
```

---

## Task Dependency Map

```
Task 1 (BlocklistEntry + migration)
  → Task 2 (RotateBlocklist) → wires into daemon
  → Task 3 (always-on persistence) → Task 4 (RawUnknown warning)
     → Task 5 (model config) → Task 6 (MODELS TUI)
        → Task 7 (ArchiveOutcome) → Task 8 (GatherStats counts outcomes)
           → Task 9 (AcceptanceStats / ListAcceptanceRate)
              → Task 10 (new types + META badge)
                 → Task 11 (meta config + EnsureMetaPrompts)
                    → Task 12 (ComputePipelineMetrics)
                       → Task 13 (RunMetaAnalysis)
                          → Task 14 (performMetaRun + metaTicker)
                             → Task 15 (METRICS TUI)
```

All tasks in a chain must be completed in order. Tasks within a single component (e.g. Task 1+2) can be combined if the implementer prefers fewer commits.
