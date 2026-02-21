# Runner Struct Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unify `Run()` and `BatchProcessor` into a single `Runner` struct that owns hook fields, supports both single-session and multi-session paths, and is fully testable without LLM calls.

**Architecture:** A `Runner` struct replaces both the `Run()`/`RunThroughClassifier()` package-level functions and the `BatchProcessor` struct. It exposes `RunOne()` (single session, replaces `Run()`) and `RunGroup()` (multi-session batch, replaces `BatchProcessor.ProcessGroup()`). Both methods share the same triage/evaluate/persist/status-mark logic. Hook fields enable testing without LLM calls — the same pattern already proven on `BatchProcessor`.

**Tech Stack:** Go 1.23, standard `testing` package, existing `store`/`parser`/`patterns`/`retrieval` packages.

---

## Task Map

```
Task 1: Runner struct + hook fields + defaults
Task 2: Move classify logic to Runner
Task 3: Move RunOne (single-session) logic to Runner
Task 4: Move RunGroup (batch) logic to Runner
Task 5: Migrate cmd/run.go caller
Task 6: Migrate daemon callers
Task 7: Migrate cmd/backfill.go caller
Task 8: Remove old Run()/RunThroughClassifier()/BatchProcessor
Task 9: Verify clean build + no dead code
```

---

### Task 1: Define Runner struct with hook fields and constructor

**Files:**
- Create: `internal/pipeline/runner.go`
- Test: `internal/pipeline/runner_test.go`

**Step 1: Write the failing test**

```go
// internal/pipeline/runner_test.go
package pipeline

import (
	"testing"
)

func TestNewRunner_DefaultHooks(t *testing.T) {
	r := NewRunner(PipelineConfig{})

	// All hook fields should be nil (defaults to package-level functions).
	if r.ParseSessionFunc != nil {
		t.Error("ParseSessionFunc should be nil by default")
	}
	if r.AggregateFunc != nil {
		t.Error("AggregateFunc should be nil by default")
	}
	if r.ClassifyFunc != nil {
		t.Error("ClassifyFunc should be nil by default")
	}
	if r.EvalFunc != nil {
		t.Error("EvalFunc should be nil by default")
	}
	if r.EvalBatchFunc != nil {
		t.Error("EvalBatchFunc should be nil by default")
	}
}

func TestNewRunner_CustomLogger(t *testing.T) {
	spy := &spyLogger{}
	cfg := PipelineConfig{Logger: spy}
	r := NewRunner(cfg)

	// Logger should be accessible via the config.
	log := r.Config.logger()
	if log != spy {
		t.Errorf("logger = %T, want *spyLogger", log)
	}
}

func TestRunner_MaxBatch(t *testing.T) {
	r := NewRunner(PipelineConfig{})
	if r.maxBatch() != DefaultMaxBatchSize {
		t.Errorf("maxBatch() = %d, want %d", r.maxBatch(), DefaultMaxBatchSize)
	}

	r.MaxBatchSize = 3
	if r.maxBatch() != 3 {
		t.Errorf("maxBatch() = %d, want 3", r.maxBatch())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pipeline/ -run TestNewRunner -v`
Expected: FAIL — `NewRunner` undefined

**Step 3: Write minimal implementation**

```go
// internal/pipeline/runner.go
package pipeline

import (
	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
)

// Runner orchestrates the analysis pipeline for single or batched sessions.
// Hook fields allow testing without LLM calls — when nil, the real
// package-level functions are used.
type Runner struct {
	Config       PipelineConfig
	MaxBatchSize int // 0 means DefaultMaxBatchSize
	OnStatus     func(sessionID string, event BatchEvent)

	// Testing hooks — when nil, package-level functions are used.
	ParseSessionFunc func(sessionID string) (*parser.Digest, error)
	AggregateFunc    func(sessionID string, project string) (*patterns.AggregatorOutput, error)
	ClassifyFunc     func(sessionID string, cfg PipelineConfig) (*ClassifierResult, error)
	EvalFunc         func(sessionID string, digest *parser.Digest, co *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, error)
	EvalBatchFunc    func(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, error)
}

// NewRunner creates a Runner with default (nil) hooks.
func NewRunner(cfg PipelineConfig) *Runner {
	return &Runner{Config: cfg}
}

func (r *Runner) log() Logger {
	return r.Config.logger()
}

func (r *Runner) maxBatch() int {
	if r.MaxBatchSize > 0 {
		return r.MaxBatchSize
	}
	return DefaultMaxBatchSize
}

func (r *Runner) emit(sessionID string, event BatchEvent) {
	if r.OnStatus != nil {
		r.OnStatus(sessionID, event)
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pipeline/ -run TestNewRunner -v && go test ./internal/pipeline/ -run TestRunner_MaxBatch -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/pipeline/runner.go internal/pipeline/runner_test.go
git commit -m "refactor(pipeline): add Runner struct with hook fields"
```

---

### Task 2: Move classify logic to Runner

The Runner needs internal `classify`, `evalOne`, `evalMany`, and `preParseAndAggregate` methods that delegate to hooks or real functions.

**Files:**
- Modify: `internal/pipeline/runner.go`
- Test: `internal/pipeline/runner_test.go`

**Step 1: Write the failing test**

```go
// Add to runner_test.go

func TestRunner_Classify_UsesHook(t *testing.T) {
	hookCalled := false
	r := NewRunner(PipelineConfig{})
	r.ClassifyFunc = func(sid string, cfg PipelineConfig) (*ClassifierResult, error) {
		hookCalled = true
		return &ClassifierResult{
			Digest:           &parser.Digest{SessionID: sid},
			ClassifierOutput: &ClassifierOutput{SessionID: sid, Triage: "clean"},
		}, nil
	}

	result, err := r.classify("test-session")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if !hookCalled {
		t.Error("ClassifyFunc hook not called")
	}
	if result.ClassifierOutput.Triage != "clean" {
		t.Errorf("Triage = %q, want 'clean'", result.ClassifierOutput.Triage)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pipeline/ -run TestRunner_Classify -v`
Expected: FAIL — `r.classify` undefined

**Step 3: Write minimal implementation**

Add to `runner.go`:

```go
func (r *Runner) classify(sessionID string) (*ClassifierResult, error) {
	if r.ClassifyFunc != nil {
		return r.ClassifyFunc(sessionID, r.Config)
	}
	return RunThroughClassifier(sessionID, r.Config)
}

func (r *Runner) evalOne(sessionID string, digest *parser.Digest, co *ClassifierOutput) (*EvaluatorOutput, error) {
	if r.EvalFunc != nil {
		return r.EvalFunc(sessionID, digest, co, r.Config)
	}
	return RunEvaluator(sessionID, digest, co, r.Config)
}

func (r *Runner) evalMany(sessions []BatchSession) (*EvaluatorOutput, error) {
	if r.EvalBatchFunc != nil {
		return r.EvalBatchFunc(sessions, r.Config)
	}
	return RunEvaluatorBatch(sessions, r.Config)
}

func (r *Runner) parseSession(sessionID string) (*parser.Digest, error) {
	if r.ParseSessionFunc != nil {
		return r.ParseSessionFunc(sessionID)
	}
	return parser.ParseSession(sessionID)
}

func (r *Runner) aggregate(sessionID, project string) (*patterns.AggregatorOutput, error) {
	if r.AggregateFunc != nil {
		return r.AggregateFunc(sessionID, project)
	}
	return patterns.Aggregate(sessionID, project)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pipeline/ -run TestRunner_Classify -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/pipeline/runner.go internal/pipeline/runner_test.go
git commit -m "refactor(pipeline): add Runner delegation methods with hooks"
```

---

### Task 3: Move RunOne (single-session) to Runner

This is the critical task — it moves the `Run()` logic to `Runner.RunOne()`.

**Files:**
- Modify: `internal/pipeline/runner.go`
- Test: `internal/pipeline/runner_test.go`

**Step 1: Write the failing tests**

```go
// Add to runner_test.go

func TestRunOne_DryRun(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-dryrun001"
	createBatchSession(t, sid)
	writeTranscript(t, sid, []string{"uuid-1"})

	parseCalled := false
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ParseSessionFunc = func(sessionID string) (*parser.Digest, error) {
		parseCalled = true
		return &parser.Digest{SessionID: sessionID}, nil
	}
	r.ClassifyFunc = func(_ string, _ PipelineConfig) (*ClassifierResult, error) {
		t.Fatal("ClassifyFunc should not be called in dry-run")
		return nil, nil
	}

	result, err := r.RunOne(context.Background(), sid, true)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if !parseCalled {
		t.Error("ParseSessionFunc not called")
	}
	if !result.DryRun {
		t.Error("DryRun should be true")
	}
}

func TestRunOne_CleanTriage_SkipsEvaluator(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-clean00001"
	createBatchSession(t, sid)

	evalCalled := false
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyClean
	r.EvalFunc = func(_ string, _ *parser.Digest, _ *ClassifierOutput, _ PipelineConfig) (*EvaluatorOutput, error) {
		evalCalled = true
		return nil, nil
	}

	result, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if evalCalled {
		t.Error("EvalFunc called for clean session")
	}
	if result.ClassifierOutput.Triage != "clean" {
		t.Errorf("Triage = %q, want 'clean'", result.ClassifierOutput.Triage)
	}
	if result.EvaluatorOutput != nil {
		t.Error("EvaluatorOutput should be nil for clean session")
	}
}

func TestRunOne_Evaluate_ProducesProposals(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-eval000001"
	createBatchSession(t, sid)

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalFunc = fakeEvalWithProposals(2)

	result, err := r.RunOne(context.Background(), sid, false)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if result.EvaluatorOutput == nil {
		t.Fatal("EvaluatorOutput is nil")
	}
	if len(result.EvaluatorOutput.Proposals) != 2 {
		t.Errorf("got %d proposals, want 2", len(result.EvaluatorOutput.Proposals))
	}

	// Verify status marked as processed.
	meta, err := store.ReadMetadata(sid)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != store.StatusProcessed {
		t.Errorf("Status = %q, want %q", meta.Status, store.StatusProcessed)
	}
}

func TestRunOne_ContextCancel(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-cancel0001"
	createBatchSession(t, sid)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyEvaluate

	_, err := r.RunOne(ctx, sid, false)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRunOne_LoggerReceivesMessages(t *testing.T) {
	setupBatchStore(t)
	sid := "runone-logger00001"
	createBatchSession(t, sid)

	spy := &spyLogger{}
	r := NewRunner(PipelineConfig{Logger: spy})
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalFunc = fakeEvalWithProposals(1)

	r.RunOne(context.Background(), sid, false)

	if len(spy.infos) == 0 {
		t.Error("expected Info calls on spy logger")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pipeline/ -run TestRunOne -v`
Expected: FAIL — `r.RunOne` undefined

**Step 3: Write implementation**

Add to `runner.go` — the `RunOne` method. This consolidates the logic from `Run()` in `pipeline.go` with the status-marking patterns from `BatchProcessor`:

```go
import (
	"context"
	"fmt"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
	"github.com/vladolaru/cabrero/internal/store"
)

// RunOne executes the full pipeline for a single session.
// If dryRun is true, only the pre-parser runs (no LLM invocations).
func (r *Runner) RunOne(ctx context.Context, sessionID string, dryRun bool) (*RunResult, error) {
	if !store.SessionExists(sessionID) {
		return nil, fmt.Errorf("session %s not found in store", sessionID)
	}
	if err := EnsurePrompts(); err != nil {
		return nil, fmt.Errorf("ensuring prompt files: %w", err)
	}

	log := r.log()
	result := &RunResult{DryRun: dryRun}

	// Pre-parse.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	pre, err := r.runPreParseAndAggregate(sessionID)
	if err != nil {
		return nil, err
	}
	result.Digest = pre.Digest
	result.AggregatorOutput = pre.AggregatorOutput

	if dryRun {
		log.Info("  Dry run — stopping after pre-parser.")
		return result, nil
	}

	// Classifier.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	classifierResult, err := r.classify(sessionID)
	if err != nil {
		if markErr := store.MarkError(sessionID); markErr != nil {
			log.Error("  marking error for %s: %v", sessionID, markErr)
		}
		return nil, err
	}
	result.Digest = classifierResult.Digest
	result.AggregatorOutput = classifierResult.AggregatorOutput
	result.ClassifierOutput = classifierResult.ClassifierOutput

	// Triage gate.
	if classifierResult.ClassifierOutput.Triage == "clean" {
		log.Info("  Classifier triage: clean session — skipping Evaluator")
		if markErr := store.MarkProcessed(sessionID); markErr != nil {
			log.Error("  marking processed for %s: %v", sessionID, markErr)
		}
		return result, nil
	}
	log.Info("  Classifier triage: session worth evaluating")

	// Evaluator.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	log.Info("  Running Evaluator...")
	evaluatorOutput, err := r.evalOne(sessionID, classifierResult.Digest, classifierResult.ClassifierOutput)
	if err != nil {
		if markErr := store.MarkError(sessionID); markErr != nil {
			log.Error("  marking error for %s: %v", sessionID, markErr)
		}
		return nil, fmt.Errorf("evaluator failed: %w", err)
	}
	result.EvaluatorOutput = evaluatorOutput

	// Persist.
	proposals := persistEvaluatorResults(sessionID, evaluatorOutput, log)
	if proposals > 0 {
		log.Info("  Evaluator: %d proposals generated", proposals)
	} else {
		reason := "no improvement signals detected"
		if evaluatorOutput.NoProposalReason != nil {
			reason = *evaluatorOutput.NoProposalReason
		}
		log.Info("  Evaluator: no proposals (%s)", reason)
	}

	if markErr := store.MarkProcessed(sessionID); markErr != nil {
		log.Error("  marking processed for %s: %v", sessionID, markErr)
	}

	return result, nil
}

// runPreParseAndAggregate delegates to hooks or real functions.
func (r *Runner) runPreParseAndAggregate(sessionID string) (*preParseResult, error) {
	log := r.log()

	log.Info("  Parsing session %s...", sessionID)
	digest, err := r.parseSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("pre-parser failed: %w", err)
	}
	if err := parser.WriteDigest(digest); err != nil {
		return nil, fmt.Errorf("writing digest: %w", err)
	}
	log.Info("  Digest written: %d entries, %d turns, %d errors, %d friction signals",
		digest.Shape.EntryCount, digest.Shape.TurnCount, len(digest.Errors), len(digest.ToolCalls.FrictionSignals))

	var aggregatorOutput *patterns.AggregatorOutput
	meta, metaErr := store.ReadMetadata(sessionID)
	if metaErr == nil && meta.Project != "" {
		log.Info("  Aggregating cross-session patterns...")
		aggregatorOutput, err = r.aggregate(sessionID, meta.Project)
		if err != nil {
			log.Info("  Warning: pattern aggregation failed: %v", err)
		} else if aggregatorOutput != nil {
			log.Info("  Found %d recurring pattern(s) across %d sessions",
				len(aggregatorOutput.Patterns), aggregatorOutput.SessionsScanned)
		}
	}

	return &preParseResult{
		Digest:           digest,
		AggregatorOutput: aggregatorOutput,
	}, nil
}
```

Note: The `RunOne` method uses `store.MarkProcessed`/`store.MarkError` consistently (like `BatchProcessor` does), fixing the inconsistency where `Run()` used direct `store.WriteMetadata` calls.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pipeline/ -run TestRunOne -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/pipeline/runner.go internal/pipeline/runner_test.go
git commit -m "feat(pipeline): add Runner.RunOne method"
```

---

### Task 4: Move RunGroup (batch) to Runner

Port `BatchProcessor.ProcessGroup()` logic to `Runner.RunGroup()`.

**Files:**
- Modify: `internal/pipeline/runner.go`
- Test: `internal/pipeline/runner_test.go`

**Step 1: Write the failing tests**

```go
// Add to runner_test.go — port the existing ProcessGroup tests to use Runner

func TestRunGroup_AllClean(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-clean00000001")
	s2 := createBatchSession(t, "rg-clean00000002")

	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyClean
	results := r.RunGroup(context.Background(), []BatchSession{s1, s2})

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, res := range results {
		if res.Status != "processed" {
			t.Errorf("%s: Status = %q, want 'processed'", res.SessionID, res.Status)
		}
		if res.Triage != "clean" {
			t.Errorf("%s: Triage = %q, want 'clean'", res.SessionID, res.Triage)
		}
	}
}

func TestRunGroup_BatchEval(t *testing.T) {
	setupBatchStore(t)
	s1 := createBatchSession(t, "rg-batch00000001")
	s2 := createBatchSession(t, "rg-batch00000002")

	batchCalled := false
	r := NewRunner(PipelineConfig{Logger: &discardLogger{}})
	r.ClassifyFunc = fakeClassifyEvaluate
	r.EvalBatchFunc = func(sessions []BatchSession, _ PipelineConfig) (*EvaluatorOutput, error) {
		batchCalled = true
		return &EvaluatorOutput{Proposals: []Proposal{
			{ID: "prop-rg-bat-0", Type: "skill_improvement", Confidence: "high", Rationale: "t"},
		}}, nil
	}

	r.RunGroup(context.Background(), []BatchSession{s1, s2})

	if !batchCalled {
		t.Error("EvalBatchFunc not called")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pipeline/ -run TestRunGroup -v`
Expected: FAIL — `r.RunGroup` undefined

**Step 3: Write implementation**

Add `RunGroup` to `runner.go` — this is a direct port of `BatchProcessor.ProcessGroup()`, `evalSingle()`, and `evalBatch()` but calling `r.classify()`, `r.evalOne()`, `r.evalMany()`:

```go
// RunGroup runs all sessions through the Classifier, then batches "evaluate"
// sessions through the Evaluator. Returns one BatchResult per input session.
func (r *Runner) RunGroup(ctx context.Context, sessions []BatchSession) []BatchResult {
	// Identical logic to BatchProcessor.ProcessGroup, using r.classify/evalOne/evalMany.
	// See batch.go for the source — the body is a direct port.
	...
}
```

The implementation is identical to `BatchProcessor.ProcessGroup()` / `evalSingle()` / `evalBatch()` but using `r.` methods. Copy the logic, replace `bp.` with `r.`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/pipeline/ -run TestRunGroup -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/pipeline/runner.go internal/pipeline/runner_test.go
git commit -m "feat(pipeline): add Runner.RunGroup method"
```

---

### Task 5: Migrate cmd/run.go to Runner

**Files:**
- Modify: `internal/cmd/run.go`

**Step 1: Read current code**

Read `internal/cmd/run.go` to confirm the call site at line 36.

**Step 2: Change `pipeline.Run()` to `Runner.RunOne()`**

Replace:
```go
result, err := pipeline.Run(sessionID, *dryRun, cfg)
```

With:
```go
runner := pipeline.NewRunner(cfg)
result, err := runner.RunOne(context.Background(), sessionID, *dryRun)
```

Add `"context"` import.

**Step 3: Run tests and build**

Run: `go build ./... && go test ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/cmd/run.go
git commit -m "refactor(cmd): migrate run command to Runner.RunOne"
```

---

### Task 6: Migrate daemon callers to Runner

The daemon has two call sites:
- `daemon.go:233` — `pipeline.Run()` in `processOne()`
- `daemon.go:194` — `&pipeline.BatchProcessor{...}` in `processProjectBatch()`

Both should be replaced by a single `Runner` stored on the `Daemon` struct.

**Files:**
- Modify: `internal/daemon/daemon.go`
- Test: `internal/daemon/daemon_test.go`

**Step 1: Write the failing test**

```go
// Add to daemon_test.go

func TestNewWiresRunner(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	logPath := filepath.Join(dir, "daemon.log")
	cfg := Config{
		LogPath:  logPath,
		Pipeline: DefaultConfig().Pipeline,
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer d.log.Close()

	if d.runner == nil {
		t.Fatal("runner is nil after New()")
	}
	if d.runner.Config.Logger == nil {
		t.Fatal("runner.Config.Logger is nil")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -run TestNewWiresRunner -v`
Expected: FAIL — `d.runner` undefined

**Step 3: Write implementation**

In `daemon.go`:
- Add `runner *pipeline.Runner` field to `Daemon` struct
- In `New()`, after setting `cfg.Pipeline.Logger`, create: `runner := pipeline.NewRunner(cfg.Pipeline)`
- Store on daemon: `d.runner = runner`
- In `processOne()`: replace `pipeline.Run(sessionID, false, d.config.Pipeline)` with `d.runner.RunOne(ctx, sessionID, false)` — note: `processOne` needs a `ctx` parameter now
- In `processProjectBatch()`: replace `&pipeline.BatchProcessor{...}` with `d.runner` (set `OnStatus` on the runner before calling `RunGroup`)

Actually, since `OnStatus` varies per call site, the daemon should either:
- Set `OnStatus` on the runner before each batch call (simple, works since daemon is single-threaded), or
- Create a new `Runner` per batch (unnecessary overhead)

The simplest: set `d.runner.OnStatus` in `processProjectBatch` before calling `RunGroup`. Reset it after. The daemon is single-threaded so this is safe.

For `processOne`, add `ctx context.Context` parameter and change caller in `processQueued` to pass `ctx`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/daemon/ -run TestNewWiresRunner -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test ./...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/daemon/daemon.go internal/daemon/daemon_test.go
git commit -m "refactor(daemon): migrate to Runner for single and batch paths"
```

---

### Task 7: Migrate cmd/backfill.go to Runner

**Files:**
- Modify: `internal/cmd/backfill.go`

**Step 1: Read current code**

Read `internal/cmd/backfill.go` around line 263 where `BatchProcessor` is constructed.

**Step 2: Replace `BatchProcessor` with `Runner`**

Replace:
```go
bp := &pipeline.BatchProcessor{
    Config: cfg,
    OnStatus: func(...) { ... },
}
results := bp.ProcessGroup(ctx, batchSessions)
```

With:
```go
runner := pipeline.NewRunner(cfg)
runner.OnStatus = func(...) { ... }
results := runner.RunGroup(ctx, batchSessions)
```

**Step 3: Run tests and build**

Run: `go build ./... && go test ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/cmd/backfill.go
git commit -m "refactor(cmd): migrate backfill to Runner.RunGroup"
```

---

### Task 8: Remove old Run()/RunThroughClassifier()/BatchProcessor

Now that all callers are migrated, remove the old code.

**Files:**
- Modify: `internal/pipeline/pipeline.go` — remove `Run()`, `RunThroughClassifier()`, and the package-level `runPreParseAndAggregate()`
- Modify: `internal/pipeline/batch.go` — remove `BatchProcessor` struct and all its methods
- Modify: `internal/pipeline/batch_test.go` — remove `TestProcessGroup` and related tests (replaced by runner_test.go tests)

**Step 1: Verify no remaining references**

Run: `grep -rn 'pipeline\.Run\b\|BatchProcessor\|RunThroughClassifier' internal/ --include='*.go' | grep -v _test.go | grep -v runner.go`

Expected: Only `runner.go` references to `RunThroughClassifier` (internal delegation) should remain. If `runner.go` calls `RunThroughClassifier` directly, those will need to be inlined or kept as internal helpers.

**Decision point:** `Runner.classify()` currently delegates to `RunThroughClassifier()` when no hook is set. Since `RunThroughClassifier` calls `runPreParseAndAggregate` + `RunClassifier`, and `Runner.RunOne` also calls `r.runPreParseAndAggregate` separately for dry-run, the cleanest approach is:
- Keep `RunClassifier` (the LLM invocation) as a package-level function (it's not orchestration, it's a leaf call)
- Remove `RunThroughClassifier` — its logic is now in `Runner.classify()` which calls `r.runPreParseAndAggregate` + `RunClassifier` directly
- Remove `Run()` — replaced by `Runner.RunOne()`
- Remove `BatchProcessor` — replaced by `Runner.RunGroup()`
- Keep `persistEvaluatorResults`, `filterProposals`, `shortID` — these are shared utilities

**Step 2: Update Runner.classify to inline RunThroughClassifier logic**

The default (no hook) path in `Runner.classify()` should call `RunClassifier` directly (not `RunThroughClassifier`):

```go
func (r *Runner) classify(sessionID string) (*ClassifierResult, error) {
	if r.ClassifyFunc != nil {
		return r.ClassifyFunc(sessionID, r.Config)
	}
	// Inline what RunThroughClassifier did.
	if !store.SessionExists(sessionID) {
		return nil, fmt.Errorf("session %s not found in store", sessionID)
	}
	if err := EnsurePrompts(); err != nil {
		return nil, fmt.Errorf("ensuring prompt files: %w", err)
	}
	log := r.log()
	pre, err := r.runPreParseAndAggregate(sessionID)
	if err != nil {
		return nil, err
	}
	log.Info("  Running Classifier...")
	classifierOutput, err := RunClassifier(sessionID, pre.Digest, pre.AggregatorOutput, r.Config)
	if err != nil {
		return nil, fmt.Errorf("classifier failed: %w", err)
	}
	if err := WriteClassifierOutput(sessionID, classifierOutput); err != nil {
		return nil, fmt.Errorf("writing classifier output: %w", err)
	}
	log.Info("  Classifier: goal=%q, %d errors, %d key turns, %d skill signals, triage=%s",
		classifierOutput.Goal.Summary,
		len(classifierOutput.ErrorClassification),
		len(classifierOutput.KeyTurns),
		len(classifierOutput.SkillSignals),
		classifierOutput.Triage)
	return &ClassifierResult{
		Digest:           pre.Digest,
		AggregatorOutput: pre.AggregatorOutput,
		ClassifierOutput: classifierOutput,
	}, nil
}
```

Wait — this duplicates pre-parse in both `classify` and `RunOne`. Currently `Run()` calls `RunThroughClassifier` which does pre-parse + classify, so `Run()` doesn't pre-parse separately (except for dry-run). For `Runner.RunOne`, the non-dry-run path can just call `r.classify()` which does pre-parse + classify. The dry-run path calls `r.runPreParseAndAggregate()` directly.

This matches the current structure. `Runner.classify()` = pre-parse + classify (for non-dry-run). `Runner.RunOne` dry-run = pre-parse only.

**Step 3: Remove dead code**

- Delete `Run()`, `RunThroughClassifier()`, package-level `runPreParseAndAggregate()` from `pipeline.go`
- Delete `BatchProcessor` and all methods from `batch.go`
- Delete `TestProcessGroup` tests from `batch_test.go` (keep `TestShortID`, `TestFilterProposals`, `TestFilterAndValidateProposals`, `TestValidateClassifierUUIDs`, `TestValidateEvaluatorOutput`, `writeTranscript`, helpers)
- Update imports

**Step 4: Run tests and build**

Run: `go build ./... && go test ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/pipeline/pipeline.go internal/pipeline/batch.go internal/pipeline/batch_test.go internal/pipeline/runner.go
git commit -m "chore(pipeline): remove Run, RunThroughClassifier, and BatchProcessor"
```

---

### Task 9: Verify clean build + no dead code

**Step 1: Check for stale references**

Run: `grep -rn 'BatchProcessor\|RunThroughClassifier\|pipeline\.Run(' internal/ --include='*.go'`

Expected: Zero results (or only comments/docs).

**Step 2: Check no direct stdout/stderr in pipeline**

Run: `grep -rn 'fmt\.Printf\|fmt\.Println\|fmt\.Fprintf(os\.Stderr' internal/pipeline/ --include='*.go' | grep -v _test.go`

Expected: Only `stdLogger.Info`/`stdLogger.Error` in `pipeline.go`.

**Step 3: Full test suite**

Run: `go test ./... -count=1`
Expected: All PASS.

**Step 4: Clean build**

Run: `go build ./...`
Expected: Success.

**Step 5: Commit (if any stragglers)**

Only if previous steps revealed cleanup needed.

**Step 6: Update CHANGELOG.md**

Add entry under `[Unreleased]`:

```markdown
### Changed

- **Runner struct** — unified `Run()` and `BatchProcessor` into a single `Runner`
  struct with `RunOne()` and `RunGroup()` methods. Consistent status marking via
  `store.MarkProcessed`/`MarkError`, context cancellation on all paths, and full
  testability via hook fields.

### Removed

- `Run()`, `RunThroughClassifier()`, and `BatchProcessor` — replaced by `Runner`.
```

```bash
git add CHANGELOG.md
git commit -m "docs: add changelog entry for Runner struct refactor"
```

---

## Summary Table

| Task | Description | Files | Commit |
|------|------------|-------|--------|
| 1 | Runner struct + hooks + constructor | `runner.go`, `runner_test.go` | `refactor(pipeline): add Runner struct with hook fields` |
| 2 | Delegation methods (classify, eval, parse) | `runner.go`, `runner_test.go` | `refactor(pipeline): add Runner delegation methods with hooks` |
| 3 | RunOne method (replaces Run) | `runner.go`, `runner_test.go` | `feat(pipeline): add Runner.RunOne method` |
| 4 | RunGroup method (replaces ProcessGroup) | `runner.go`, `runner_test.go` | `feat(pipeline): add Runner.RunGroup method` |
| 5 | Migrate cmd/run.go | `cmd/run.go` | `refactor(cmd): migrate run command to Runner.RunOne` |
| 6 | Migrate daemon | `daemon.go`, `daemon_test.go` | `refactor(daemon): migrate to Runner for single and batch paths` |
| 7 | Migrate cmd/backfill.go | `cmd/backfill.go` | `refactor(cmd): migrate backfill to Runner.RunGroup` |
| 8 | Remove old code | `pipeline.go`, `batch.go`, `batch_test.go`, `runner.go` | `chore(pipeline): remove Run, RunThroughClassifier, and BatchProcessor` |
| 9 | Verify + changelog | verify + `CHANGELOG.md` | `docs: add changelog entry for Runner struct refactor` |
