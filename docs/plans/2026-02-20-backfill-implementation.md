# Backfill Feature Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `cabrero backfill` command to process existing CC sessions through the full pipeline, enhance `cabrero import` with pre-parsing, and integrate both into the setup wizard.

**Architecture:** Extract the daemon's smart batching logic into a shared `pipeline.BatchProcessor` (with configurable max batch size) used by both daemon and backfill. Add `store.QuerySessions` for filtered session selection. Backfill shows a preview, confirms, then processes using smart batching with progress callbacks.

**Tech Stack:** Go 1.25, standard library only (no external dependencies). Uses `flag` for CLI, `encoding/json` for store, `time` for date parsing.

**Design doc:** `docs/plans/2026-02-20-backfill-design.md`

---

## Task 1: Add `store.MarkProcessed` and `store.MarkError` helpers

These helpers are currently duplicated in `daemon.go` as private methods. Extract them to the store package so both daemon and the new batch processor can use them.

**Files:**
- Modify: `internal/store/session.go`
- Create: `internal/store/session_test.go`

**Step 1: Write the test**

```go
// internal/store/session_test.go
package store

import (
	"os"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

func TestMarkProcessed(t *testing.T) {
	setupTestStore(t)

	// Create a session with pending status.
	sid := "test-session-mark-processed"
	rawDir := RawDir(sid)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := Metadata{
		SessionID:      sid,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CaptureTrigger: "imported",
		Status:         "pending",
	}
	if err := WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}

	// Mark as processed.
	if err := MarkProcessed(sid); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}

	got, err := ReadMetadata(sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "processed" {
		t.Errorf("status = %q, want %q", got.Status, "processed")
	}
}

func TestMarkError(t *testing.T) {
	setupTestStore(t)

	sid := "test-session-mark-error"
	rawDir := RawDir(sid)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := Metadata{
		SessionID:      sid,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CaptureTrigger: "imported",
		Status:         "pending",
	}
	if err := WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}

	if err := MarkError(sid); err != nil {
		t.Fatalf("MarkError: %v", err)
	}

	got, err := ReadMetadata(sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "error" {
		t.Errorf("status = %q, want %q", got.Status, "error")
	}
}

func TestMarkProcessed_NotFound(t *testing.T) {
	setupTestStore(t)
	err := MarkProcessed("nonexistent-session")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestMark -v`
Expected: FAIL — `MarkProcessed` and `MarkError` undefined.

**Step 3: Implement the helpers**

Add to `internal/store/session.go`:

```go
// MarkProcessed sets a session's status to "processed".
func MarkProcessed(sessionID string) error {
	meta, err := ReadMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("reading metadata for %s: %w", sessionID, err)
	}
	meta.Status = "processed"
	return WriteMetadata(RawDir(sessionID), meta)
}

// MarkError sets a session's status to "error".
func MarkError(sessionID string) error {
	meta, err := ReadMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("reading metadata for %s: %w", sessionID, err)
	}
	meta.Status = "error"
	return WriteMetadata(RawDir(sessionID), meta)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestMark -v`
Expected: PASS (3 tests).

**Step 5: Commit**

```
feat(store): add MarkProcessed and MarkError helpers

These were duplicated as private methods in the daemon package.
Extracting them to the store package enables reuse by the upcoming
batch processor and backfill command.
```

---

## Task 2: Add `store.QuerySessions`

Filtered session query with date range, project substring, and status filtering. Returns sessions sorted oldest-first (processing order).

**Files:**
- Create: `internal/store/query.go`
- Create: `internal/store/query_test.go`

**Step 1: Write the test**

```go
// internal/store/query_test.go
package store

import (
	"os"
	"testing"
	"time"
)

func createTestSession(t *testing.T, id, status, trigger, project string, ts time.Time) {
	t.Helper()
	rawDir := RawDir(id)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a minimal transcript file so the session is valid.
	if err := os.WriteFile(rawDir+"/transcript.jsonl", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := Metadata{
		SessionID:      id,
		Timestamp:      ts.UTC().Format(time.RFC3339),
		CaptureTrigger: trigger,
		Status:         status,
		Project:        project,
	}
	if err := WriteMetadata(rawDir, meta); err != nil {
		t.Fatal(err)
	}
}

func TestQuerySessions_StatusFilter(t *testing.T) {
	setupTestStore(t)
	now := time.Now()
	createTestSession(t, "s1", "pending", "imported", "proj-a", now.Add(-1*time.Hour))
	createTestSession(t, "s2", "processed", "session-end", "proj-a", now.Add(-2*time.Hour))
	createTestSession(t, "s3", "error", "imported", "proj-a", now.Add(-3*time.Hour))

	results, err := QuerySessions(SessionFilter{Statuses: []string{"pending"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].SessionID != "s1" {
		t.Errorf("got %d results, want 1 (s1)", len(results))
	}

	results, err = QuerySessions(SessionFilter{Statuses: []string{"pending", "error"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
	// Should be oldest-first: s3 then s1.
	if results[0].SessionID != "s3" || results[1].SessionID != "s1" {
		t.Errorf("order: got [%s, %s], want [s3, s1]", results[0].SessionID, results[1].SessionID)
	}
}

func TestQuerySessions_DateRange(t *testing.T) {
	setupTestStore(t)
	now := time.Now()
	createTestSession(t, "old", "pending", "imported", "proj", now.Add(-72*time.Hour))
	createTestSession(t, "mid", "pending", "imported", "proj", now.Add(-24*time.Hour))
	createTestSession(t, "new", "pending", "imported", "proj", now.Add(-1*time.Hour))

	since := now.Add(-48 * time.Hour)
	results, err := QuerySessions(SessionFilter{
		Statuses: []string{"pending"},
		Since:    since,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2 (mid, new)", len(results))
	}
}

func TestQuerySessions_ProjectFilter(t *testing.T) {
	setupTestStore(t)
	now := time.Now()
	createTestSession(t, "p1", "pending", "imported", "Work-a8c-woocommerce-payments", now)
	createTestSession(t, "p2", "pending", "imported", "Work-a8c-cabrero", now)

	results, err := QuerySessions(SessionFilter{
		Statuses: []string{"pending"},
		Project:  "woocommerce",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].SessionID != "p1" {
		t.Errorf("got %d results, want 1 (p1)", len(results))
	}
}

func TestQuerySessions_NoStatuses_ReturnsAll(t *testing.T) {
	setupTestStore(t)
	now := time.Now()
	createTestSession(t, "x1", "pending", "imported", "proj", now)
	createTestSession(t, "x2", "processed", "session-end", "proj", now)

	results, err := QuerySessions(SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("got %d results, want 2 (all)", len(results))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestQuerySessions -v`
Expected: FAIL — `QuerySessions` and `SessionFilter` undefined.

**Step 3: Implement QuerySessions**

```go
// internal/store/query.go
package store

import (
	"strings"
	"time"
)

// SessionFilter controls which sessions QuerySessions returns.
type SessionFilter struct {
	Since    time.Time // zero value = no lower bound
	Until    time.Time // zero value = no upper bound
	Project  string    // substring match (empty = all)
	Statuses []string  // e.g. ["pending"] or ["pending", "error"]; empty = all
}

// QuerySessions returns sessions matching the filter, sorted oldest-first.
func QuerySessions(filter SessionFilter) ([]Metadata, error) {
	all, err := ListSessions() // newest-first
	if err != nil {
		return nil, err
	}

	statusSet := make(map[string]bool, len(filter.Statuses))
	for _, s := range filter.Statuses {
		statusSet[s] = true
	}

	var matched []Metadata
	for _, m := range all {
		// Status filter.
		if len(statusSet) > 0 && !statusSet[m.Status] {
			continue
		}

		// Date filter.
		ts, err := time.Parse(time.RFC3339, m.Timestamp)
		if err != nil {
			continue // skip unparseable timestamps
		}
		if !filter.Since.IsZero() && ts.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && ts.After(filter.Until) {
			continue
		}

		// Project filter.
		if filter.Project != "" && !strings.Contains(m.Project, filter.Project) {
			continue
		}

		matched = append(matched, m)
	}

	// ListSessions returns newest-first; reverse for oldest-first.
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}

	return matched, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestQuerySessions -v`
Expected: PASS (4 tests).

**Step 5: Commit**

```
feat(store): add QuerySessions with date, project, and status filtering

Returns sessions matching a SessionFilter sorted oldest-first. Used by
the upcoming backfill command for session selection. Filters compose
with AND: date range, project substring, and status set.
```

---

## Task 3: Extract `pipeline.BatchProcessor` from daemon

Extract the daemon's smart batching logic (`processProjectBatch`, `runEvaluatorSingle`, `runEvaluatorBatch`, `persistEvaluatorOutput`, `filterProposals`) into `internal/pipeline/batch.go` with a callback-based progress API and configurable max batch size.

**Files:**
- Create: `internal/pipeline/batch.go`
- Modify: `internal/daemon/daemon.go`

**Step 1: Create `internal/pipeline/batch.go`**

Extract and adapt the following from `daemon.go`:
- `processProjectBatch` → `BatchProcessor.ProcessGroup`
- `runEvaluatorSingle` → private method on BatchProcessor
- `runEvaluatorBatch` → private method on BatchProcessor
- `persistEvaluatorOutput` → private function (was daemon method)
- `filterProposals` → stays here (already a pure function)

```go
// internal/pipeline/batch.go
package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vladolaru/cabrero/internal/store"
)

// DefaultMaxBatchSize is the default number of sessions per Evaluator invocation.
// Keeps batches within the Evaluator's 60-turn / 15-minute caps.
const DefaultMaxBatchSize = 10

// BatchEvent reports progress for a single session during batch processing.
type BatchEvent struct {
	Type   string // "classifier_done", "evaluator_done", "error"
	Triage string // "clean" or "evaluate" (set for classifier_done)
	Error  error  // non-nil for "error"
}

// BatchResult holds the outcome for one session after batch processing.
type BatchResult struct {
	SessionID string
	Status    string // "processed" or "error"
	Proposals int
	Triage    string // Classifier triage result
	Error     error
}

// BatchProcessor handles smart batching: Classifier individually per session,
// Evaluator batched per project group with configurable chunk size.
type BatchProcessor struct {
	Config       PipelineConfig
	MaxBatchSize int                                       // max sessions per Evaluator invocation (0 = DefaultMaxBatchSize)
	OnStatus     func(sessionID string, event BatchEvent)  // progress callback (optional)
}

func (bp *BatchProcessor) maxBatch() int {
	if bp.MaxBatchSize > 0 {
		return bp.MaxBatchSize
	}
	return DefaultMaxBatchSize
}

// ProcessGroup runs the full pipeline on a group of sessions from the same project.
// Sessions should all belong to the same project for Evaluator batching to work.
// The ctx parameter allows cancellation between sessions.
func (bp *BatchProcessor) ProcessGroup(ctx context.Context, sessions []BatchSession) []BatchResult {
	results := make([]BatchResult, len(sessions))
	for i := range results {
		results[i] = BatchResult{SessionID: sessions[i].SessionID}
	}

	// Phase 1: Run Classifier individually on each session.
	var toEvaluate []indexedBatchSession
	for i, s := range sessions {
		select {
		case <-ctx.Done():
			for j := i; j < len(sessions); j++ {
				results[j].Status = "error"
				results[j].Error = ctx.Err()
			}
			return results
		default:
		}

		classResult, err := RunThroughClassifier(s.SessionID, bp.Config)
		if err != nil {
			results[i].Status = "error"
			results[i].Error = err
			store.MarkError(s.SessionID)
			bp.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
			continue
		}

		triage := classResult.ClassifierOutput.Triage
		results[i].Triage = triage

		if triage == "clean" {
			results[i].Status = "processed"
			store.MarkProcessed(s.SessionID)
			bp.emit(s.SessionID, BatchEvent{Type: "classifier_done", Triage: "clean"})
			continue
		}

		bp.emit(s.SessionID, BatchEvent{Type: "classifier_done", Triage: "evaluate"})
		toEvaluate = append(toEvaluate, indexedBatchSession{
			index: i,
			BatchSession: BatchSession{
				SessionID:        s.SessionID,
				Digest:           classResult.Digest,
				ClassifierOutput: classResult.ClassifierOutput,
			},
		})
	}

	if len(toEvaluate) == 0 {
		return results
	}

	// Phase 2: Run Evaluator in chunks of maxBatch.
	maxB := bp.maxBatch()
	for start := 0; start < len(toEvaluate); start += maxB {
		end := start + maxB
		if end > len(toEvaluate) {
			end = len(toEvaluate)
		}
		chunk := toEvaluate[start:end]

		select {
		case <-ctx.Done():
			for _, s := range toEvaluate[start:] {
				results[s.index].Status = "error"
				results[s.index].Error = ctx.Err()
			}
			return results
		default:
		}

		if len(chunk) == 1 {
			bp.evaluateSingle(chunk[0], results)
		} else {
			bp.evaluateBatch(chunk, results)
		}
	}

	return results
}

type indexedBatchSession struct {
	index int
	BatchSession
}

func (bp *BatchProcessor) evaluateSingle(s indexedBatchSession, results []BatchResult) {
	output, err := RunEvaluator(s.SessionID, s.Digest, s.ClassifierOutput, bp.Config)
	if err != nil {
		results[s.index].Status = "error"
		results[s.index].Error = err
		store.MarkError(s.SessionID)
		bp.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
		return
	}

	proposals := persistEvaluatorResults(s.SessionID, output)
	results[s.index].Status = "processed"
	results[s.index].Proposals = proposals
	store.MarkProcessed(s.SessionID)
	bp.emit(s.SessionID, BatchEvent{Type: "evaluator_done"})
}

func (bp *BatchProcessor) evaluateBatch(sessions []indexedBatchSession, results []BatchResult) {
	batchSessions := make([]BatchSession, len(sessions))
	for i, s := range sessions {
		batchSessions[i] = s.BatchSession
	}

	output, err := RunEvaluatorBatch(batchSessions, bp.Config)
	if err != nil {
		for _, s := range sessions {
			results[s.index].Status = "error"
			results[s.index].Error = err
			store.MarkError(s.SessionID)
			bp.emit(s.SessionID, BatchEvent{Type: "error", Error: err})
		}
		return
	}

	// Partition proposals by session.
	for _, s := range sessions {
		prefix := "prop-" + shortID(s.SessionID) + "-"
		filtered := filterProposals(output, prefix)
		filtered.SessionID = s.SessionID
		proposals := persistEvaluatorResults(s.SessionID, filtered)
		results[s.index].Status = "processed"
		results[s.index].Proposals = proposals
		store.MarkProcessed(s.SessionID)
		bp.emit(s.SessionID, BatchEvent{Type: "evaluator_done"})
	}
}

// persistEvaluatorResults writes evaluator output and proposals to the store.
// Returns the number of proposals successfully written.
func persistEvaluatorResults(sessionID string, output *EvaluatorOutput) int {
	if err := WriteEvaluatorOutput(sessionID, output); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: writing evaluator output for %s: %v\n", sessionID, err)
		return 0
	}

	count := 0
	for i := range output.Proposals {
		p := &output.Proposals[i]
		if err := WriteProposal(p, sessionID); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: writing proposal %s: %v\n", p.ID, err)
			continue
		}
		count++
	}
	return count
}

// filterProposals returns a shallow copy with only proposals whose ID starts with prefix.
func filterProposals(output *EvaluatorOutput, prefix string) *EvaluatorOutput {
	filtered := *output
	filtered.Proposals = []Proposal{}
	for _, p := range output.Proposals {
		if strings.HasPrefix(p.ID, prefix) {
			filtered.Proposals = append(filtered.Proposals, p)
		}
	}
	return &filtered
}

func (bp *BatchProcessor) emit(sessionID string, event BatchEvent) {
	if bp.OnStatus != nil {
		bp.OnStatus(sessionID, event)
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
```

**Step 2: Refactor `internal/daemon/daemon.go`**

Replace `processProjectBatch`, `runEvaluatorSingle`, `runEvaluatorBatch`, `filterProposals`, `persistEvaluatorOutput`, `markProcessed`, `markError` with calls to the new `BatchProcessor` and `store.MarkProcessed`/`store.MarkError`.

The daemon's `processProjectBatch` becomes:

```go
func (d *Daemon) processProjectBatch(ctx context.Context, project string, sessions []PendingSession) {
	d.log.Info("batch: %d session(s) for project %s", len(sessions), store.ProjectDisplayName(project))

	batchSessions := make([]pipeline.BatchSession, len(sessions))
	for i, s := range sessions {
		batchSessions[i] = pipeline.BatchSession{SessionID: s.SessionID}
	}

	bp := &pipeline.BatchProcessor{
		Config: d.config.Pipeline,
		OnStatus: func(sessionID string, event pipeline.BatchEvent) {
			switch event.Type {
			case "classifier_done":
				if event.Triage == "clean" {
					d.log.Info("session %s triaged as clean", shortID(sessionID))
				}
			case "error":
				d.log.Error("pipeline error for %s: %v", shortID(sessionID), event.Error)
			}
		},
	}

	results := bp.ProcessGroup(ctx, batchSessions)

	// Count proposals and send notifications.
	totalProposals := 0
	toEvalCount := 0
	for _, r := range results {
		totalProposals += r.Proposals
		if r.Triage == "evaluate" {
			toEvalCount++
		}
	}

	d.log.Info("batch: %d of %d session(s) needed evaluation, %d proposals",
		toEvalCount, len(sessions), totalProposals)

	if totalProposals > 0 {
		msg := fmt.Sprintf("%d new proposal(s) from %d session(s)", totalProposals, len(sessions))
		if err := Notify("Cabrero", msg); err != nil {
			d.log.Error("notification failed: %v", err)
		}
	}
}
```

Also update `processOne` to use `store.MarkProcessed`/`store.MarkError` (replace `d.markProcessed`/`d.markError` calls).

Remove from `daemon.go`: `markProcessed`, `markError`, `persistEvaluatorOutput`, `runEvaluatorSingle`, `runEvaluatorBatch`, `filterProposals`. Keep `shortID` (used by `processOne` for logging).

**Step 3: Verify compilation**

Run: `go build ./...`
Expected: compiles successfully.

**Step 4: Commit**

```
refactor(pipeline): extract BatchProcessor from daemon

Move smart batching logic (Classifier individually, Evaluator batched
per project) into pipeline.BatchProcessor with a callback-based
progress API and configurable max batch size (default 10).

Large groups are chunked into sub-batches to stay within the
Evaluator's 60-turn / 15-minute caps. Both daemon and the upcoming
backfill command reuse this logic.
```

---

## Task 4: Enhance `cabrero import` with pre-parsing

After copying each session's JSONL into the store, run the pre-parser to generate a digest.

**Files:**
- Modify: `internal/cmd/importcmd.go`

**Step 1: Add pre-parser call after import**

In `internal/cmd/importcmd.go`, after the `store.WriteSession` call (line 103) succeeds, add:

```go
// Run pre-parser to generate digest (cheap, no LLM).
digest, parseErr := parser.ParseSession(sessionID)
if parseErr != nil {
	fmt.Fprintf(os.Stderr, "  Warning: pre-parser failed for %s: %v\n", sessionID, parseErr)
} else {
	if writeErr := parser.WriteDigest(digest); writeErr != nil {
		fmt.Fprintf(os.Stderr, "  Warning: writing digest for %s: %v\n", sessionID, writeErr)
	}
}
```

Add `"github.com/vladolaru/cabrero/internal/parser"` to the imports.

Also update the summary output to mention digests:

```go
if *dryRun {
	fmt.Printf("Would import %d sessions (with pre-parsing), skipped %d (already present).\n", imported, skipped)
} else {
	fmt.Printf("Imported %d sessions (with digests), skipped %d (already present).\n", imported, skipped)
}
```

**Step 2: Verify compilation**

Run: `go build ./...`
Expected: compiles successfully.

**Step 3: Commit**

```
feat(import): run pre-parser on imported sessions

After copying each session's JSONL into the store, the import command
now runs the pre-parser to generate a digest. This makes imported
sessions immediately useful for the pattern aggregator without
requiring a separate pipeline run.

Pre-parser failures are non-fatal (warning printed, session still
imported).
```

---

## Task 5: Create `cabrero backfill` command

The main CLI command with flag parsing, session selection, preview, confirmation, and processing with progress output.

**Files:**
- Create: `internal/cmd/backfill.go`
- Modify: `main.go`

**Step 1: Create `internal/cmd/backfill.go`**

```go
// internal/cmd/backfill.go
package cmd

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Backfill runs the full pipeline on stored sessions matching the given filters.
func Backfill(args []string) error {
	fs := flag.NewFlagSet("backfill", flag.ExitOnError)

	since := fs.String("since", "", "process sessions from this date (YYYY-MM-DD, default: 30 days ago)")
	until := fs.String("until", "", "process sessions up to this date (YYYY-MM-DD, default: now)")
	project := fs.String("project", "", "filter by project slug substring")
	dryRun := fs.Bool("dry-run", false, "show preview only, don't process")
	autoYes := fs.Bool("yes", false, "skip confirmation prompt")
	retryErrors := fs.Bool("retry-errors", false, "also re-process sessions with status 'error'")

	classifierMaxTurns := fs.Int("classifier-max-turns", 0, "override Classifier max turns")
	evaluatorMaxTurns := fs.Int("evaluator-max-turns", 0, "override Evaluator max turns")
	classifierTimeout := fs.Duration("classifier-timeout", 0, "override Classifier timeout")
	evaluatorTimeout := fs.Duration("evaluator-timeout", 0, "override Evaluator timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Parse date range.
	filter, err := buildBackfillFilter(*since, *until, *project, *retryErrors)
	if err != nil {
		return err
	}

	// Query matching sessions.
	sessions, err := store.QuerySessions(filter)
	if err != nil {
		return fmt.Errorf("querying sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found matching filters.")
		return nil
	}

	// Build pipeline config.
	cfg := pipeline.DefaultPipelineConfig()
	if *classifierMaxTurns > 0 {
		cfg.ClassifierMaxTurns = *classifierMaxTurns
	}
	if *evaluatorMaxTurns > 0 {
		cfg.EvaluatorMaxTurns = *evaluatorMaxTurns
	}
	if *classifierTimeout > 0 {
		cfg.ClassifierTimeout = *classifierTimeout
	}
	if *evaluatorTimeout > 0 {
		cfg.EvaluatorTimeout = *evaluatorTimeout
	}

	// Show preview.
	showBackfillPreview(sessions, filter)

	if *dryRun {
		return nil
	}

	// Confirm.
	if !*autoYes {
		if !confirmBackfill() {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Process.
	return runBackfill(sessions, cfg)
}

func buildBackfillFilter(sinceStr, untilStr, project string, retryErrors bool) (store.SessionFilter, error) {
	filter := store.SessionFilter{
		Project:  project,
		Statuses: []string{"pending"},
	}

	if retryErrors {
		filter.Statuses = append(filter.Statuses, "error")
	}

	if sinceStr != "" {
		t, err := time.Parse("2006-01-02", sinceStr)
		if err != nil {
			return filter, fmt.Errorf("invalid --since date %q (use YYYY-MM-DD): %w", sinceStr, err)
		}
		filter.Since = t
	} else {
		// Default: 30 days ago.
		filter.Since = time.Now().AddDate(0, 0, -30)
	}

	if untilStr != "" {
		t, err := time.Parse("2006-01-02", untilStr)
		if err != nil {
			return filter, fmt.Errorf("invalid --until date %q (use YYYY-MM-DD): %w", untilStr, err)
		}
		// End of day.
		filter.Until = t.Add(24*time.Hour - time.Second)
	}

	return filter, nil
}

func showBackfillPreview(sessions []store.Metadata, filter store.SessionFilter) {
	fmt.Println()
	fmt.Println("Backfill Preview")
	fmt.Println(strings.Repeat("═", 40))
	fmt.Println()

	// Count by status.
	pendingCount := 0
	errorCount := 0
	for _, s := range sessions {
		switch s.Status {
		case "pending":
			pendingCount++
		case "error":
			errorCount++
		}
	}

	statusDesc := fmt.Sprintf("pending (%d)", pendingCount)
	if errorCount > 0 {
		statusDesc += fmt.Sprintf(", error (%d)", errorCount)
	}
	fmt.Printf("  Status: %s\n", statusDesc)

	// Date range.
	sinceStr := filter.Since.Format("2006-01-02")
	untilStr := "now"
	if !filter.Until.IsZero() {
		untilStr = filter.Until.Format("2006-01-02")
	}
	fmt.Printf("  Date range: %s → %s\n", sinceStr, untilStr)

	if filter.Project != "" {
		fmt.Printf("  Project filter: *%s*\n", filter.Project)
	}

	// Group by project.
	byProject := make(map[string]int)
	var projectOrder []string
	for _, s := range sessions {
		key := s.Project
		if key == "" {
			key = "(no project)"
		}
		if _, seen := byProject[key]; !seen {
			projectOrder = append(projectOrder, key)
		}
		byProject[key]++
	}

	fmt.Printf("\n  %d session(s) to process across %d project(s):\n", len(sessions), len(projectOrder))
	for _, p := range projectOrder {
		display := store.ProjectDisplayName(p)
		if display == "" {
			display = p
		}
		fmt.Printf("    %-40s (%d sessions)\n", display, byProject[p])
	}

	// Estimate.
	fmt.Printf("\n  Estimated pipeline calls:\n")
	fmt.Printf("    Classifier: %d invocations (Haiku — low cost)\n", len(sessions))
	fmt.Printf("    Evaluator:  up to %d batch invocations (Sonnet — one per project)\n", len(projectOrder))
	fmt.Println()
}

func confirmBackfill() bool {
	fmt.Print("  Proceed? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

func runBackfill(sessions []store.Metadata, cfg pipeline.PipelineConfig) error {
	// Group by project.
	type projectGroup struct {
		project  string
		sessions []store.Metadata
	}
	var groups []projectGroup
	groupMap := make(map[string]int)

	for _, s := range sessions {
		key := s.Project
		if idx, ok := groupMap[key]; ok {
			groups[idx].sessions = append(groups[idx].sessions, s)
		} else {
			groupMap[key] = len(groups)
			groups = append(groups, projectGroup{project: key, sessions: []store.Metadata{s}})
		}
	}

	// Set up cancellation.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Process each project group.
	var totalProcessed, totalClean, totalEvaluated, totalProposals, totalErrors int

	fmt.Printf("Processing %d sessions...\n\n", len(sessions))

	for _, g := range groups {
		select {
		case <-ctx.Done():
			fmt.Println("\nInterrupted. Sessions already processed retain their results.")
			goto summary
		default:
		}

		display := store.ProjectDisplayName(g.project)
		if display == "" {
			display = "(no project)"
		}
		fmt.Printf("  Project: %s (%d sessions)\n", display, len(g.sessions))

		batchSessions := make([]pipeline.BatchSession, len(g.sessions))
		for i, s := range g.sessions {
			batchSessions[i] = pipeline.BatchSession{SessionID: s.SessionID}
		}

		sessionCount := len(g.sessions)
		bp := &pipeline.BatchProcessor{
			Config: cfg,
			OnStatus: func(sessionID string, event pipeline.BatchEvent) {
				sid := sessionID
				if len(sid) > 8 {
					sid = sid[:8]
				}
				switch event.Type {
				case "classifier_done":
					idx := 0
					for i, s := range g.sessions {
						if s.SessionID == sessionID {
							idx = i + 1
							break
						}
					}
					fmt.Printf("    [%d/%d] %s — Classifier: %s", idx, sessionCount, sid, event.Triage)
					if event.Triage == "clean" {
						fmt.Print(" ✓")
					}
					fmt.Println()
				case "error":
					fmt.Printf("    %s — Error: %v\n", sid, event.Error)
				}
			},
		}

		results := bp.ProcessGroup(ctx, batchSessions)

		// Tally results for this group.
		groupProposals := 0
		groupEvaluated := 0
		for _, r := range results {
			if r.Error != nil {
				totalErrors++
			} else {
				totalProcessed++
			}
			if r.Triage == "clean" {
				totalClean++
			}
			if r.Triage == "evaluate" {
				groupEvaluated++
				totalEvaluated++
			}
			groupProposals += r.Proposals
			totalProposals += r.Proposals
		}

		if groupEvaluated > 0 {
			fmt.Printf("    Evaluator batch: %d sessions → %d proposals generated\n", groupEvaluated, groupProposals)
		}
		fmt.Println()
	}

summary:
	fmt.Println("Summary")
	fmt.Println(strings.Repeat("═", 40))
	fmt.Printf("  Processed: %d sessions\n", totalProcessed)
	fmt.Printf("  Clean (skipped Evaluator): %d\n", totalClean)
	fmt.Printf("  Evaluated: %d\n", totalEvaluated)
	fmt.Printf("  Proposals generated: %d\n", totalProposals)
	fmt.Printf("  Errors: %d\n", totalErrors)

	if totalProposals > 0 {
		fmt.Println("\n  Review proposals with: cabrero proposals")
	}

	return nil
}
```

**Step 2: Register in `main.go`**

Add to the `commands` slice in `main.go`, after the `import` entry (line 29):

```go
{"backfill", "Run pipeline on existing sessions with date/project filtering", cmd.Backfill},
```

**Step 3: Verify compilation**

Run: `go build ./...`
Expected: compiles successfully.

**Step 4: Commit**

```
feat(cmd): add cabrero backfill command

New command to process existing CC sessions through the full pipeline
with date range and project filtering. Shows a preview with session
counts and estimated pipeline calls, then requires confirmation before
processing.

Uses smart batching via the shared BatchProcessor. Supports --dry-run,
--yes, --retry-errors, and pipeline budget overrides. Default date
range is 30 days back from today.
```

---

## Task 6: Add backfill offer to setup wizard

Add Step 8 to the setup wizard that offers to import + backfill existing sessions. Import output is suppressed to a summary count (no per-session lines).

**Files:**
- Modify: `internal/cmd/setup.go`
- Modify: `internal/cmd/importcmd.go` (add quiet mode)

**Step 1: Add quiet mode to import**

Add a `Quiet` parameter to the import logic. The simplest approach: extract the import logic into an `ImportOptions` struct or add a package-level function that setup can call with suppressed output.

Add to `internal/cmd/importcmd.go`:

```go
// ImportResult holds the outcome of an import operation.
type ImportResult struct {
	Imported int
	Skipped  int
}

// RunImport executes the import logic with optional quiet mode.
// When quiet is true, per-session output is suppressed.
func RunImport(from string, dryRun bool, quiet bool) (ImportResult, error) {
	// ... same logic as Import() but uses the quiet flag to suppress
	// per-session fmt.Printf lines, and returns ImportResult instead of
	// printing the summary.
}
```

Refactor the existing `Import()` to call `RunImport(from, dryRun, false)` and print the summary.

**Step 2: Add Step 8 to `internal/cmd/setup.go`**

Add to the `steps` slice in `run()`:

```go
{"Process existing sessions", s.stepBackfillOffer},
```

Implement the step:

```go
// Step 8: Offer to process existing sessions.
func (s *setupRunner) stepBackfillOffer(step, total int) error {
	if s.dryRun {
		fmt.Println("  → Would offer to import and process existing sessions (dry-run)")
		return nil
	}

	// Import existing CC sessions (quiet mode — summary only).
	fmt.Println("  Scanning for existing CC sessions...")
	home, err := os.UserHomeDir()
	if err != nil {
		return nil // non-fatal
	}
	from := filepath.Join(home, ".claude", "projects")
	result, importErr := RunImport(from, false, true)
	if importErr != nil {
		fmt.Printf("  Warning: import scan failed: %v\n", importErr)
	} else if result.Imported > 0 {
		fmt.Printf("  Imported %d sessions (%d already present)\n", result.Imported, result.Skipped)
	}

	// Count pending sessions.
	sessions, err := store.QuerySessions(store.SessionFilter{
		Statuses: []string{"pending"},
	})
	if err != nil || len(sessions) == 0 {
		fmt.Println("  No existing sessions to process.")
		return nil
	}

	fmt.Printf("  Found %d session(s) ready for processing\n", len(sessions))

	if !s.confirm("Process recent sessions through the pipeline?") {
		fmt.Println("  — Skipped. Run 'cabrero backfill' later to process.")
		return nil
	}

	// Ask how far back.
	sinceDate := time.Now().AddDate(0, -1, 0)
	if !s.autoYes {
		fmt.Printf("  How far back? (default: 1 month, or enter YYYY-MM-DD) ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			t, err := time.Parse("2006-01-02", input)
			if err != nil {
				fmt.Printf("  Could not parse date %q, using default (1 month)\n", input)
			} else {
				sinceDate = t
			}
		}
	}

	fmt.Printf("  Processing sessions since %s...\n\n", sinceDate.Format("2006-01-02"))

	return Backfill([]string{
		"--since", sinceDate.Format("2006-01-02"),
		"--yes",
	})
}
```

Add `"time"` and `"path/filepath"` to imports if not already present.

**Step 3: Verify compilation**

Run: `go build ./...`
Expected: compiles successfully.

**Step 4: Commit**

```
feat(setup): offer to import and process existing sessions

After completing the standard setup steps, the wizard now scans for
existing CC sessions (quiet mode — summary only), imports them with
digests, and offers to run the backfill pipeline on recent sessions
(default: 1 month back).

Skippable with 'n' at the prompt or automatic with --yes.
```

---

## Task 7: Update DESIGN.md and CHANGELOG.md

**Files:**
- Modify: `DESIGN.md`
- Modify: `CHANGELOG.md`

**Step 1: Update DESIGN.md**

Add `backfill` to the CLI commands section with its full flag documentation. Update the `import` entry to note pre-parsing. Document `BatchProcessor` in the pipeline architecture section. Document Setup Step 8.

Key additions:
- `cabrero backfill` command entry with all flags and defaults
- Note that `import` now runs the pre-parser
- `pipeline.BatchProcessor` as shared infrastructure (with `MaxBatchSize`)
- `store.QuerySessions` as the filtering mechanism
- Setup Step 8 description

**Step 2: Update CHANGELOG.md**

Add under `[Unreleased]`:

```markdown
## [Unreleased]

### Added
- `cabrero backfill` command to process existing sessions through the full pipeline with `--since`, `--until`, `--project`, `--retry-errors` filtering, preview with confirmation, and smart batching.
- Setup wizard Step 8: offers to import and process existing CC sessions after installation.
- `store.QuerySessions` for filtered session queries by date range, project, and status.
- `pipeline.BatchProcessor` as shared smart batching infrastructure with configurable max batch size.

### Changed
- `cabrero import` now runs the pre-parser on each imported session to generate digests.
- Daemon batching logic refactored into `pipeline.BatchProcessor` (no behavior change).
- `store.MarkProcessed` and `store.MarkError` extracted as public helpers.
```

**Step 3: Commit**

```
docs: document backfill command and pipeline changes

Update DESIGN.md with the new backfill command, enhanced import
behavior, BatchProcessor architecture, and setup wizard Step 8.

Add changelog entries for all user-visible changes.
```

---

## Task Dependency Graph

```
Task 1 (store helpers) ──────┐
                              ├──→ Task 3 (BatchProcessor) ──┐
Task 2 (QuerySessions) ──────┤                               ├──→ Task 5 (backfill cmd) ──┐
                              │                               │                             │
Task 4 (import pre-parse) ────┼───────────────────────────────┘                             │
                              │                                                             │
                              └──→ Task 6 (setup Step 8) ←─────────────────────────────────┘
                                          │
                                          └──→ Task 7 (docs)
```

Tasks 1, 2, and 4 are independent and can be done in parallel.
