# Phase 5: Iteration Tooling Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the three remaining Phase 5 features — `cabrero prompts`, `cabrero replay`, and calibration set tagging — completing Cabrero's prompt iteration workflow.

**Architecture:** All three features build on existing infrastructure. `prompts` exposes `ListPromptVersions()` as a CLI command. `replay` re-runs the pipeline with alternate prompt files against past sessions, storing output to a dedicated `replays/` directory. Calibration tagging adds a `calibration` boolean to archived proposals, letting `replay` filter to a curated ground-truth set. The iteration loop: notice rejection patterns → adjust prompt → replay against calibration set → deploy.

**Tech Stack:** Go, standard library, existing `internal/pipeline`, `internal/store`, `internal/cmd` packages. No new dependencies.

---

## Dependency Graph

```
Task 1: cabrero prompts (standalone)
Task 2: replay store layout (standalone)
Task 3: cabrero replay --session (depends on Task 2)
Task 4: calibration tagging on archived proposals (standalone)
Task 5: cabrero replay --calibration (depends on Tasks 3 + 4)
Task 6: DESIGN.md + CHANGELOG.md updates (depends on all above)
```

Tasks 1, 2, and 4 are independent and can be built in parallel.

---

### Task 1: `cabrero prompts` Command

**Files:**
- Create: `internal/cmd/prompts.go`
- Create: `internal/cmd/prompts_test.go`
- Modify: `main.go:29` (wire up command)

This command lists prompt files from `~/.cabrero/prompts/` with name, version, file path, and last-modified time. The heavy lifting is already done by `pipeline.ListPromptVersions()` — this is a formatting wrapper.

**Step 1: Write the test**

```go
// internal/cmd/prompts_test.go
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestPrompts_ListsFiles(t *testing.T) {
	// Redirect HOME to temp dir so store.Root() points there.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	// Write two prompt files.
	dir := filepath.Join(store.Root(), "prompts")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "classifier-v3.txt"), []byte("prompt"), 0o644)
	os.WriteFile(filepath.Join(dir, "evaluator-v3.txt"), []byte("prompt"), 0o644)

	// Should not error.
	if err := Prompts(nil); err != nil {
		t.Fatalf("Prompts: %v", err)
	}
}

func TestPrompts_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	// No prompt files yet — command should succeed with informational message.
	if err := Prompts(nil); err != nil {
		t.Fatalf("Prompts: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestPrompts -v`
Expected: compilation error — `Prompts` undefined.

**Step 3: Write the implementation**

```go
// internal/cmd/prompts.go
package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Prompts lists prompt files with their versions and last-modified times.
func Prompts(args []string) error {
	versions, err := pipeline.ListPromptVersions()
	if err != nil {
		return fmt.Errorf("listing prompts: %w", err)
	}

	if len(versions) == 0 {
		fmt.Println("No prompt files found in", filepath.Join(store.Root(), "prompts"))
		fmt.Println("Run 'cabrero run' or 'cabrero setup' to create default prompts.")
		return nil
	}

	fmt.Printf("%-14s %-8s %-20s %s\n", "NAME", "VERSION", "LAST MODIFIED", "PATH")
	fmt.Printf("%-14s %-8s %-20s %s\n", "────", "───────", "─────────────", "────")

	dir := filepath.Join(store.Root(), "prompts")
	for _, v := range versions {
		age := formatAge(v.LastUsed)
		filename := v.Name
		if v.Version != "" {
			filename += "-" + v.Version
		}
		fmt.Printf("%-14s %-8s %-20s %s\n", v.Name, v.Version, age, filepath.Join(dir, filename+".txt"))
	}

	return nil
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
```

**Step 4: Wire up in main.go**

In `main.go`, change line 29 from:
```go
{"prompts", "List prompt files with versions", cmdNotImplemented},
```
to:
```go
{"prompts", "List prompt files with versions", cmd.Prompts},
```

**Step 5: Run tests**

Run: `go test ./internal/cmd/ -run TestPrompts -v`
Expected: PASS

**Step 6: Run full test suite**

Run: `go test ./...`
Expected: all pass.

**Step 7: Commit**

```
feat(cli): implement cabrero prompts command

Lists prompt files from ~/.cabrero/prompts/ with name, version,
last-modified time, and file path. Uses existing ListPromptVersions()
infrastructure.
```

---

### Task 2: Replay Store Layout

**Files:**
- Modify: `internal/store/store.go` (add `replays/` to Init, add ReplayDir helper)
- Modify: `internal/store/store_test.go` or create if none exists

The replay command needs a place to write its output. Add a `replays/` subdirectory to the store. Each replay produces a timestamped directory with the classifier and evaluator output.

**Store layout:**
```
~/.cabrero/
  replays/
    {replayId}/              # format: replay-{sessionId}-{timestamp}
      classifier.json        # classifier output (if re-run)
      evaluator.json         # evaluator output (if re-run)
      meta.json              # replay metadata (session, prompts used, original decision)
```

**Step 1: Write the test**

```go
// In internal/store/ — test that Init creates replays/ and ReplayDir returns the right path.
func TestInit_CreatesReplaysDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root(), "replays")
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("replays dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("replays is not a directory")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestInit_CreatesReplaysDir -v`
Expected: FAIL — replays/ directory not created.

**Step 3: Add `replays/` to `store.Init()` and add `ReplayDir()` helper**

In `internal/store/store.go`, add `"replays"` to the directory list in `Init()`. Add:

```go
// ReplayDir returns the path to ~/.cabrero/replays/.
func ReplayDir() string {
	return filepath.Join(Root(), "replays")
}
```

**Step 4: Run tests**

Run: `go test ./internal/store/ -run TestInit_CreatesReplaysDir -v`
Expected: PASS

**Step 5: Commit**

```
feat(store): add replays directory to store layout

Replay output will be stored in ~/.cabrero/replays/{replayId}/ with
classifier, evaluator, and metadata JSON files. This prepares the store
for the cabrero replay command.
```

---

### Task 3: `cabrero replay` Command

**Files:**
- Create: `internal/cmd/replay.go`
- Create: `internal/cmd/replay_test.go`
- Create: `internal/pipeline/replay.go`
- Create: `internal/pipeline/replay_test.go`
- Modify: `main.go:28` (wire up command)

The replay command re-runs the pipeline on a past session using an alternate prompt file, then optionally compares output against the original. This is the core prompt iteration tool.

**CLI signature (from DESIGN.md):**
```
cabrero replay --session <id> --prompt <path> [--compare] [--stage classifier|evaluator]
```

Flags:
- `--session` (required): session ID to replay
- `--prompt` (required): path to alternate prompt file
- `--stage`: which stage to override — `classifier` or `evaluator` (default: infer from filename)
- `--compare`: show diff between replay output and original
- `--classifier-model`, `--evaluator-model`: optional model overrides

#### Step 1: Write pipeline replay types and test

```go
// internal/pipeline/replay.go

package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// ReplayConfig holds configuration for a replay run.
type ReplayConfig struct {
	SessionID     string
	PromptPath    string         // path to alternate prompt file
	Stage         string         // "classifier" or "evaluator"
	Pipeline      PipelineConfig // model, timeout, turn budget overrides
}

// ReplayResult holds the output of a replay run.
type ReplayResult struct {
	ReplayID         string
	SessionID        string
	Stage            string
	PromptPath       string
	ClassifierOutput *ClassifierOutput // non-nil if classifier was re-run
	EvaluatorOutput  *EvaluatorOutput  // non-nil if evaluator was re-run
}

// ReplayMeta is persisted to replays/{replayId}/meta.json.
type ReplayMeta struct {
	ReplayID     string    `json:"replayId"`
	SessionID    string    `json:"sessionId"`
	Timestamp    time.Time `json:"timestamp"`
	Stage        string    `json:"stage"`
	PromptFile   string    `json:"promptFile"`
	OriginalDecision string `json:"originalDecision,omitempty"` // "approved", "rejected: reason", or ""
}

// InferStage guesses which pipeline stage a prompt file is for based on its filename.
// Returns "classifier" or "evaluator", or empty string if ambiguous.
func InferStage(filename string) string {
	base := filepath.Base(filename)
	if len(base) >= 10 && base[:10] == "classifier" {
		return "classifier"
	}
	if len(base) >= 9 && base[:9] == "evaluator" {
		return "evaluator"
	}
	return ""
}

// WriteReplayResult persists replay output to the replays directory.
func WriteReplayResult(result *ReplayResult, meta *ReplayMeta) error {
	dir := filepath.Join(store.ReplayDir(), result.ReplayID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Write meta.
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := store.AtomicWrite(filepath.Join(dir, "meta.json"), metaData, 0o644); err != nil {
		return err
	}

	// Write classifier output if present.
	if result.ClassifierOutput != nil {
		data, err := json.MarshalIndent(result.ClassifierOutput, "", "  ")
		if err != nil {
			return err
		}
		if err := store.AtomicWrite(filepath.Join(dir, "classifier.json"), data, 0o644); err != nil {
			return err
		}
	}

	// Write evaluator output if present.
	if result.EvaluatorOutput != nil {
		data, err := json.MarshalIndent(result.EvaluatorOutput, "", "  ")
		if err != nil {
			return err
		}
		if err := store.AtomicWrite(filepath.Join(dir, "evaluator.json"), data, 0o644); err != nil {
			return err
		}
	}

	return nil
}
```

```go
// internal/pipeline/replay_test.go

package pipeline

import (
	"testing"
)

func TestInferStage(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"classifier-v4.txt", "classifier"},
		{"evaluator-v5.txt", "evaluator"},
		{"classifier.txt", "classifier"},
		{"evaluator.txt", "evaluator"},
		{"my-prompt.txt", ""},
		{"/path/to/classifier-v4.txt", "classifier"},
		{"/path/to/evaluator-v5.txt", "evaluator"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := InferStage(tt.filename)
			if got != tt.want {
				t.Errorf("InferStage(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestWriteReplayResult(t *testing.T) {
	setupBatchStore(t)

	result := &ReplayResult{
		ReplayID:  "replay-test001-20260222T120000",
		SessionID: "test001",
		Stage:     "classifier",
		ClassifierOutput: &ClassifierOutput{
			SessionID: "test001",
			Triage:    "evaluate",
		},
	}
	meta := &ReplayMeta{
		ReplayID:  result.ReplayID,
		SessionID: "test001",
		Stage:     "classifier",
		PromptFile: "classifier-v4.txt",
	}

	if err := WriteReplayResult(result, meta); err != nil {
		t.Fatalf("WriteReplayResult: %v", err)
	}

	// Verify files exist.
	// (Test uses setupBatchStore which sets HOME to temp dir)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pipeline/ -run "TestInferStage|TestWriteReplayResult" -v`
Expected: compilation error — types and functions don't exist yet.

**Step 3: Implement replay.go as shown above**

**Step 4: Run tests**

Run: `go test ./internal/pipeline/ -run "TestInferStage|TestWriteReplayResult" -v`
Expected: PASS

**Step 5: Commit**

```
feat(pipeline): add replay types and store writer

Adds ReplayConfig, ReplayResult, ReplayMeta types and WriteReplayResult
for persisting replay output. InferStage guesses classifier vs evaluator
from prompt filename. Prepares infrastructure for cabrero replay.
```

#### Step 6: Write the CLI command

```go
// internal/cmd/replay.go
package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Replay re-runs the pipeline on a past session with an alternate prompt.
func Replay(args []string) error {
	defaults := pipeline.DefaultPipelineConfig()
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	sessionID := fs.String("session", "", "session ID to replay")
	promptPath := fs.String("prompt", "", "path to alternate prompt file")
	stage := fs.String("stage", "", "pipeline stage to override: classifier or evaluator (default: infer from filename)")
	compare := fs.Bool("compare", false, "compare replay output against original")
	classifierModel := fs.String("classifier-model", defaults.ClassifierModel, "Claude model for Classifier")
	evaluatorModel := fs.String("evaluator-model", defaults.EvaluatorModel, "Claude model for Evaluator")
	classifierMaxTurns := fs.Int("classifier-max-turns", defaults.ClassifierMaxTurns, "max agentic turns for Classifier")
	evaluatorMaxTurns := fs.Int("evaluator-max-turns", defaults.EvaluatorMaxTurns, "max agentic turns for Evaluator")
	classifierTimeout := fs.Duration("classifier-timeout", defaults.ClassifierTimeout, "timeout for Classifier")
	evaluatorTimeout := fs.Duration("evaluator-timeout", defaults.EvaluatorTimeout, "timeout for Evaluator")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *sessionID == "" {
		return fmt.Errorf("usage: cabrero replay --session <id> --prompt <path> [--compare] [--stage classifier|evaluator]")
	}
	if *promptPath == "" {
		return fmt.Errorf("--prompt is required")
	}

	// Validate session exists.
	if !store.SessionExists(*sessionID) {
		return fmt.Errorf("session %s not found in store", *sessionID)
	}

	// Validate prompt file exists.
	if _, err := os.Stat(*promptPath); err != nil {
		return fmt.Errorf("prompt file %q: %w", *promptPath, err)
	}

	// Infer stage if not provided.
	if *stage == "" {
		*stage = pipeline.InferStage(*promptPath)
		if *stage == "" {
			return fmt.Errorf("cannot infer stage from filename %q — use --stage classifier or --stage evaluator", *promptPath)
		}
		fmt.Printf("Inferred stage: %s\n", *stage)
	}

	if *stage != "classifier" && *stage != "evaluator" {
		return fmt.Errorf("--stage must be 'classifier' or 'evaluator', got %q", *stage)
	}

	// Read the alternate prompt.
	promptContent, err := os.ReadFile(*promptPath)
	if err != nil {
		return fmt.Errorf("reading prompt: %w", err)
	}
	_ = promptContent // Used below by the pipeline

	cfg := defaults
	cfg.ClassifierModel = *classifierModel
	cfg.EvaluatorModel = *evaluatorModel
	cfg.ClassifierMaxTurns = *classifierMaxTurns
	cfg.EvaluatorMaxTurns = *evaluatorMaxTurns
	cfg.ClassifierTimeout = *classifierTimeout
	cfg.EvaluatorTimeout = *evaluatorTimeout

	fmt.Printf("Replaying session %s\n", *sessionID)
	fmt.Printf("  Stage: %s\n", *stage)
	fmt.Printf("  Prompt: %s\n", *promptPath)
	fmt.Printf("  Models: classifier=%s, evaluator=%s\n", cfg.ClassifierModel, cfg.EvaluatorModel)
	fmt.Println()

	// Run the replay pipeline.
	result, err := runReplay(context.Background(), *sessionID, *stage, string(promptContent), cfg)
	if err != nil {
		return err
	}

	// Generate replay ID and persist.
	replayID := fmt.Sprintf("replay-%s-%s", *sessionID, time.Now().UTC().Format("20060102T150405"))
	result.ReplayID = replayID

	meta := &pipeline.ReplayMeta{
		ReplayID:   replayID,
		SessionID:  *sessionID,
		Timestamp:  time.Now().UTC(),
		Stage:      *stage,
		PromptFile: *promptPath,
	}

	// Look up original decision from archived proposals.
	meta.OriginalDecision = lookupOriginalDecision(*sessionID)

	if err := pipeline.WriteReplayResult(result, meta); err != nil {
		return fmt.Errorf("writing replay result: %w", err)
	}

	fmt.Printf("Replay complete: %s\n", replayID)
	fmt.Printf("  Output: %s\n", store.ReplayDir()+"/"+replayID+"/")

	// Show results summary.
	if result.ClassifierOutput != nil {
		fmt.Printf("  Classifier triage: %s\n", result.ClassifierOutput.Triage)
	}
	if result.EvaluatorOutput != nil {
		fmt.Printf("  Proposals: %d\n", len(result.EvaluatorOutput.Proposals))
		for _, p := range result.EvaluatorOutput.Proposals {
			fmt.Printf("    - [%s] %s → %s\n", p.Confidence, p.Type, p.Target)
		}
	}

	// Compare mode.
	if *compare {
		fmt.Println()
		showComparison(*sessionID, *stage, result, meta.OriginalDecision)
	}

	return nil
}

// runReplay executes the pipeline with an alternate prompt for the specified stage.
func runReplay(ctx context.Context, sessionID, stage, altPrompt string, cfg pipeline.PipelineConfig) (*pipeline.ReplayResult, error) {
	result := &pipeline.ReplayResult{
		SessionID: sessionID,
		Stage:     stage,
	}

	// Always need the digest.
	fmt.Println("  Parsing session...")
	digest, err := pipeline.ParseSessionForReplay(sessionID)
	if err != nil {
		return nil, fmt.Errorf("parsing session: %w", err)
	}

	if stage == "classifier" {
		// Re-run classifier with alternate prompt.
		fmt.Println("  Running Classifier with alternate prompt...")
		co, _, err := pipeline.RunClassifierWithPrompt(sessionID, digest, nil, cfg, altPrompt)
		if err != nil {
			return nil, fmt.Errorf("classifier: %w", err)
		}
		result.ClassifierOutput = co
		return result, nil
	}

	// stage == "evaluator"
	// Need original classifier output to feed the evaluator.
	origClassifier, err := pipeline.ReadClassifierOutput(sessionID)
	if err != nil {
		return nil, fmt.Errorf("reading original classifier output for session %s: %w\nHint: the session must have been processed (not dry-run) to have classifier output", sessionID, err)
	}

	fmt.Println("  Running Evaluator with alternate prompt...")
	eo, _, err := pipeline.RunEvaluatorWithPrompt(sessionID, digest, origClassifier, cfg, altPrompt)
	if err != nil {
		return nil, fmt.Errorf("evaluator: %w", err)
	}
	result.EvaluatorOutput = eo
	return result, nil
}

// lookupOriginalDecision checks archived proposals for the session's decision.
func lookupOriginalDecision(sessionID string) string {
	archiveDir := store.ArchivedProposalsDir()
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return ""
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(archiveDir + "/" + e.Name())
		if err != nil {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		var sid string
		if err := json.Unmarshal(raw["sessionId"], &sid); err != nil {
			continue
		}
		if sid != sessionID {
			continue
		}
		var reason string
		if err := json.Unmarshal(raw["archiveReason"], &reason); err != nil {
			continue
		}
		return reason // "approved", "rejected", "rejected: <reason>", etc.
	}

	return ""
}

// showComparison displays the diff between replay output and original.
func showComparison(sessionID, stage string, result *pipeline.ReplayResult, originalDecision string) {
	fmt.Println("── COMPARISON ──")
	fmt.Println()

	if originalDecision != "" {
		fmt.Printf("  Original decision: %s\n", originalDecision)
	} else {
		fmt.Printf("  Original decision: (no archived proposal found)\n")
	}
	fmt.Println()

	if stage == "classifier" {
		// Compare triage.
		origClassifier, err := pipeline.ReadClassifierOutput(sessionID)
		if err != nil {
			fmt.Printf("  Cannot read original classifier output: %v\n", err)
			return
		}
		fmt.Printf("  Original triage: %s\n", origClassifier.Triage)
		fmt.Printf("  Replay triage:   %s\n", result.ClassifierOutput.Triage)
		if origClassifier.Triage != result.ClassifierOutput.Triage {
			fmt.Println("  *** TRIAGE CHANGED ***")
		}
		return
	}

	// Evaluator comparison.
	origEvaluator, err := pipeline.ReadEvaluatorOutput(sessionID)
	if err != nil {
		fmt.Printf("  Cannot read original evaluator output: %v\n", err)
		fmt.Println("  (session may not have reached evaluator stage)")
		return
	}

	fmt.Printf("  Original proposals: %d\n", len(origEvaluator.Proposals))
	for _, p := range origEvaluator.Proposals {
		fmt.Printf("    - [%s] %s → %s\n", p.Confidence, p.Type, p.Target)
	}
	fmt.Println()
	fmt.Printf("  Replay proposals: %d\n", len(result.EvaluatorOutput.Proposals))
	for _, p := range result.EvaluatorOutput.Proposals {
		fmt.Printf("    - [%s] %s → %s\n", p.Confidence, p.Type, p.Target)
	}

	if len(origEvaluator.Proposals) != len(result.EvaluatorOutput.Proposals) {
		fmt.Println()
		fmt.Println("  *** PROPOSAL COUNT CHANGED ***")
	}
}
```

**Step 7: Add pipeline helpers**

The replay command needs two new capabilities from the pipeline package:

1. `RunClassifierWithPrompt` — like `RunClassifier` but accepts a prompt string instead of reading from file
2. `RunEvaluatorWithPrompt` — like `RunEvaluator` but accepts a prompt string
3. `ParseSessionForReplay` — runs pre-parser (no aggregator, since replay doesn't need cross-session data)
4. `ReadClassifierOutput` — reads stored classifier output (already partially exists)

Add to `internal/pipeline/replay.go`:

```go
// ParseSessionForReplay runs the pre-parser on a session.
// Unlike the full pipeline, this skips the cross-session aggregator.
func ParseSessionForReplay(sessionID string) (*parser.Digest, error) {
	return parser.ParseSession(sessionID)
}

// RunClassifierWithPrompt runs the classifier with a custom prompt string
// instead of reading from the prompt file.
func RunClassifierWithPrompt(sessionID string, digest *parser.Digest, aggregatorOutput *patterns.AggregatorOutput, cfg PipelineConfig, promptOverride string) (*ClassifierOutput, *ClaudeResult, error) {
	// Same as RunClassifier but uses promptOverride instead of reading file.
	// ... (implementation mirrors classifier.go:RunClassifier but swaps readPromptTemplate with promptOverride)
}

// RunEvaluatorWithPrompt runs the evaluator with a custom prompt string.
func RunEvaluatorWithPrompt(sessionID string, digest *parser.Digest, classifierOutput *ClassifierOutput, cfg PipelineConfig, promptOverride string) (*EvaluatorOutput, *ClaudeResult, error) {
	// Same as RunEvaluator but uses promptOverride instead of reading file.
	// ... (implementation mirrors evaluator.go:RunEvaluator but swaps readPromptTemplate with promptOverride)
}
```

**Implementation note:** To avoid duplicating classifier.go and evaluator.go, the cleanest approach is to refactor `RunClassifier` and `RunEvaluator` to accept an optional prompt override parameter. If the override is non-empty, use it instead of reading from file. This keeps the code DRY and avoids two copies of the invocation logic.

Concretely, change the internal flow:
- Extract the common invocation logic into unexported helpers (e.g., `runClassifierWithSystem`)
- Have both `RunClassifier` (reads from file) and `RunClassifierWithPrompt` (uses override) call the same helper
- Same pattern for the evaluator

**Step 8: Add `ArchivedProposalsDir()` helper to store**

```go
// internal/store/store.go
func ArchivedProposalsDir() string {
	return filepath.Join(Root(), "proposals", "archived")
}
```

**Step 9: Wire up in main.go**

Change line 28 from:
```go
{"replay", "Re-run pipeline with a different prompt", cmdNotImplemented},
```
to:
```go
{"replay", "Re-run pipeline with a different prompt", cmd.Replay},
```

**Step 10: Write tests for the CLI command**

```go
// internal/cmd/replay_test.go
package cmd

import (
	"testing"
)

func TestReplay_MissingSession(t *testing.T) {
	err := Replay([]string{"--prompt", "foo.txt"})
	if err == nil {
		t.Fatal("expected error for missing --session")
	}
}

func TestReplay_MissingPrompt(t *testing.T) {
	err := Replay([]string{"--session", "abc123"})
	if err == nil {
		t.Fatal("expected error for missing --prompt")
	}
}

func TestReplay_InvalidStage(t *testing.T) {
	err := Replay([]string{"--session", "abc123", "--prompt", "foo.txt", "--stage", "bogus"})
	if err == nil {
		t.Fatal("expected error for invalid stage")
	}
}
```

**Step 11: Run tests**

Run: `go test ./internal/cmd/ -run TestReplay -v && go test ./internal/pipeline/ -run "TestInferStage|TestWriteReplayResult" -v`
Expected: PASS

**Step 12: Run full test suite**

Run: `go test ./...`
Expected: all pass.

**Step 13: Commit**

```
feat(cli): implement cabrero replay command

Re-runs the pipeline on a past session with an alternate prompt file.
Supports --stage (classifier|evaluator), --compare for diffing against
original output, and model/timeout overrides. Replay output persisted
to ~/.cabrero/replays/{replayId}/.

This is the core iteration tool for prompt development: edit a prompt
file, replay against past sessions, compare output against your
original decision.
```

---

### Task 4: Calibration Tagging

**Files:**
- Modify: `internal/store/store.go` (add calibration set read/write helpers)
- Create: `internal/store/calibration.go`
- Create: `internal/store/calibration_test.go`
- Create: `internal/cmd/calibrate.go`
- Create: `internal/cmd/calibrate_test.go`
- Modify: `main.go` (add `calibrate` command)

The calibration set is a JSON file at `~/.cabrero/calibration.json` listing session IDs tagged as ground-truth examples. Each entry records the session ID, a label ("approve" or "reject"), and an optional note.

**Design choice:** A flat JSON file rather than annotations on proposals because:
1. Calibration is about sessions, not individual proposals — a session might produce multiple proposals
2. Keeps the calibration set portable and easy to inspect
3. Doesn't require modifying the existing proposal archive format

**Store format:**
```json
{
  "entries": [
    {
      "sessionId": "abc123",
      "label": "approve",
      "note": "clear skill improvement signal",
      "taggedAt": "2026-02-22T10:00:00Z"
    }
  ]
}
```

**CLI subcommands:**
```
cabrero calibrate tag <session_id> --label approve|reject [--note "text"]
cabrero calibrate untag <session_id>
cabrero calibrate list
```

#### Step 1: Write the store layer test

```go
// internal/store/calibration_test.go
package store

import (
	"testing"
)

func TestCalibrationSet_AddAndList(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	Init()

	entry := CalibrationEntry{
		SessionID: "test-session-001",
		Label:     "approve",
		Note:      "clear signal",
	}
	if err := AddCalibrationEntry(entry); err != nil {
		t.Fatalf("AddCalibrationEntry: %v", err)
	}

	entries, err := ListCalibrationEntries()
	if err != nil {
		t.Fatalf("ListCalibrationEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].SessionID != "test-session-001" {
		t.Errorf("SessionID = %q, want %q", entries[0].SessionID, "test-session-001")
	}
	if entries[0].Label != "approve" {
		t.Errorf("Label = %q, want %q", entries[0].Label, "approve")
	}
}

func TestCalibrationSet_Remove(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	Init()

	AddCalibrationEntry(CalibrationEntry{SessionID: "s1", Label: "approve"})
	AddCalibrationEntry(CalibrationEntry{SessionID: "s2", Label: "reject"})

	if err := RemoveCalibrationEntry("s1"); err != nil {
		t.Fatalf("RemoveCalibrationEntry: %v", err)
	}

	entries, _ := ListCalibrationEntries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].SessionID != "s2" {
		t.Errorf("remaining entry SessionID = %q, want %q", entries[0].SessionID, "s2")
	}
}

func TestCalibrationSet_DuplicatePrevented(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	Init()

	AddCalibrationEntry(CalibrationEntry{SessionID: "s1", Label: "approve"})
	err := AddCalibrationEntry(CalibrationEntry{SessionID: "s1", Label: "reject"})
	if err == nil {
		t.Fatal("expected error for duplicate session ID")
	}
}

func TestCalibrationSet_InvalidLabel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	Init()

	err := AddCalibrationEntry(CalibrationEntry{SessionID: "s1", Label: "maybe"})
	if err == nil {
		t.Fatal("expected error for invalid label")
	}
}
```

#### Step 2: Run tests to verify failure

Run: `go test ./internal/store/ -run TestCalibrationSet -v`
Expected: compilation error — types don't exist.

#### Step 3: Implement calibration store

```go
// internal/store/calibration.go
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CalibrationEntry is a single tagged session in the calibration set.
type CalibrationEntry struct {
	SessionID string    `json:"sessionId"`
	Label     string    `json:"label"` // "approve" or "reject"
	Note      string    `json:"note,omitempty"`
	TaggedAt  time.Time `json:"taggedAt"`
}

type calibrationSet struct {
	Entries []CalibrationEntry `json:"entries"`
}

func calibrationPath() string {
	return filepath.Join(Root(), "calibration.json")
}

func readCalibrationSet() (*calibrationSet, error) {
	data, err := os.ReadFile(calibrationPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &calibrationSet{}, nil
		}
		return nil, err
	}
	var cs calibrationSet
	if err := json.Unmarshal(data, &cs); err != nil {
		return nil, err
	}
	return &cs, nil
}

func writeCalibrationSet(cs *calibrationSet) error {
	data, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(calibrationPath(), data, 0o644)
}

// AddCalibrationEntry tags a session as a calibration example.
func AddCalibrationEntry(entry CalibrationEntry) error {
	if entry.Label != "approve" && entry.Label != "reject" {
		return fmt.Errorf("label must be 'approve' or 'reject', got %q", entry.Label)
	}

	cs, err := readCalibrationSet()
	if err != nil {
		return err
	}

	for _, e := range cs.Entries {
		if e.SessionID == entry.SessionID {
			return fmt.Errorf("session %s is already in the calibration set", entry.SessionID)
		}
	}

	if entry.TaggedAt.IsZero() {
		entry.TaggedAt = time.Now().UTC()
	}
	cs.Entries = append(cs.Entries, entry)
	return writeCalibrationSet(cs)
}

// RemoveCalibrationEntry removes a session from the calibration set.
func RemoveCalibrationEntry(sessionID string) error {
	cs, err := readCalibrationSet()
	if err != nil {
		return err
	}

	found := false
	var kept []CalibrationEntry
	for _, e := range cs.Entries {
		if e.SessionID == sessionID {
			found = true
			continue
		}
		kept = append(kept, e)
	}

	if !found {
		return fmt.Errorf("session %s not in calibration set", sessionID)
	}

	cs.Entries = kept
	return writeCalibrationSet(cs)
}

// ListCalibrationEntries returns all entries in the calibration set.
func ListCalibrationEntries() ([]CalibrationEntry, error) {
	cs, err := readCalibrationSet()
	if err != nil {
		return nil, err
	}
	return cs.Entries, nil
}
```

#### Step 4: Run store tests

Run: `go test ./internal/store/ -run TestCalibrationSet -v`
Expected: PASS

#### Step 5: Commit

```
feat(store): add calibration set for prompt regression testing

Simple JSON-backed store at ~/.cabrero/calibration.json for tagging
sessions as ground-truth examples (approve/reject with optional notes).
Used by cabrero replay --calibration to run alternate prompts against
a curated set and compare against recorded decisions.
```

#### Step 6: Write the CLI command

```go
// internal/cmd/calibrate.go
package cmd

import (
	"flag"
	"fmt"

	"github.com/vladolaru/cabrero/internal/store"
)

// Calibrate manages the calibration set for prompt regression testing.
func Calibrate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero calibrate <tag|untag|list> [options]")
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "tag":
		return calibrateTag(rest)
	case "untag":
		return calibrateUntag(rest)
	case "list":
		return calibrateList(rest)
	default:
		return fmt.Errorf("unknown calibrate subcommand: %s (use tag, untag, or list)", sub)
	}
}

func calibrateTag(args []string) error {
	fs := flag.NewFlagSet("calibrate tag", flag.ContinueOnError)
	label := fs.String("label", "", "expected outcome: approve or reject (required)")
	note := fs.String("note", "", "optional note about why this is a good calibration example")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: cabrero calibrate tag <session_id> --label approve|reject [--note \"text\"]")
	}
	if *label == "" {
		return fmt.Errorf("--label is required (approve or reject)")
	}

	sessionID := fs.Arg(0)
	if !store.SessionExists(sessionID) {
		return fmt.Errorf("session %s not found in store", sessionID)
	}

	entry := store.CalibrationEntry{
		SessionID: sessionID,
		Label:     *label,
		Note:      *note,
	}
	if err := store.AddCalibrationEntry(entry); err != nil {
		return err
	}

	fmt.Printf("Tagged session %s as calibration example (label: %s)\n", sessionID, *label)
	return nil
}

func calibrateUntag(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero calibrate untag <session_id>")
	}
	sessionID := args[0]

	if err := store.RemoveCalibrationEntry(sessionID); err != nil {
		return err
	}

	fmt.Printf("Removed session %s from calibration set.\n", sessionID)
	return nil
}

func calibrateList(args []string) error {
	entries, err := store.ListCalibrationEntries()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("Calibration set is empty.")
		fmt.Println("Use 'cabrero calibrate tag <session_id> --label approve|reject' to add entries.")
		return nil
	}

	fmt.Printf("%-20s %-8s %-20s %s\n", "SESSION", "LABEL", "TAGGED", "NOTE")
	fmt.Printf("%-20s %-8s %-20s %s\n", "───────", "─────", "──────", "────")
	for _, e := range entries {
		age := formatAge(e.TaggedAt)
		note := e.Note
		if len(note) > 40 {
			note = note[:37] + "..."
		}
		sid := e.SessionID
		if len(sid) > 18 {
			sid = sid[:18] + ".."
		}
		fmt.Printf("%-20s %-8s %-20s %s\n", sid, e.Label, age, note)
	}
	fmt.Printf("\n%d calibration entries.\n", len(entries))
	return nil
}
```

**Note:** `formatAge` is already defined in `prompts.go` (Task 1). If building in parallel, extract it to a shared `internal/cmd/format.go` to avoid redeclaration.

#### Step 7: Wire up in main.go

Add to the commands slice:
```go
{"calibrate", "Manage calibration set for prompt testing", cmd.Calibrate},
```

#### Step 8: Write CLI tests

```go
// internal/cmd/calibrate_test.go
package cmd

import (
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestCalibrateTag_MissingLabel(t *testing.T) {
	err := Calibrate([]string{"tag", "some-session"})
	if err == nil {
		t.Fatal("expected error for missing --label")
	}
}

func TestCalibrateTag_InvalidLabel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	store.Init()

	err := Calibrate([]string{"tag", "some-session", "--label", "maybe"})
	if err == nil {
		t.Fatal("expected error for invalid label")
	}
}

func TestCalibrate_UnknownSubcommand(t *testing.T) {
	err := Calibrate([]string{"frobnicate"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestCalibrateList_Empty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	store.Init()

	// Should not error on empty set.
	if err := Calibrate([]string{"list"}); err != nil {
		t.Fatalf("calibrate list: %v", err)
	}
}
```

#### Step 9: Run tests

Run: `go test ./internal/cmd/ -run TestCalibrate -v && go test ./internal/store/ -run TestCalibrationSet -v`
Expected: PASS

#### Step 10: Run full test suite

Run: `go test ./...`
Expected: all pass.

#### Step 11: Commit

```
feat(cli): implement cabrero calibrate command

Adds calibration set management: tag/untag sessions as ground-truth
examples with approve/reject labels and optional notes. The calibration
set is stored at ~/.cabrero/calibration.json and used by cabrero replay
to regression-test new prompt versions.

Subcommands: tag, untag, list.
```

---

### Task 5: `cabrero replay --calibration` Mode

**Files:**
- Modify: `internal/cmd/replay.go` (add `--calibration` flag and batch mode)
- Modify: `internal/cmd/replay_test.go` (add tests)

Add a `--calibration` flag that replays the alternate prompt against every session in the calibration set, producing a summary comparing replay output against recorded decisions.

#### Step 1: Add flag and batch logic

In `replay.go`, add:
```go
calibration := fs.Bool("calibration", false, "replay against all sessions in the calibration set")
```

When `--calibration` is set, `--session` is not required. The command:
1. Reads the calibration set
2. For each entry, runs the replay
3. Compares triage/proposal count against original
4. Prints a summary table:

```
── CALIBRATION REPLAY SUMMARY ──

SESSION              LABEL    ORIGINAL        REPLAY          MATCH
───────              ─────    ────────        ──────          ─────
abc123..             approve  2 proposals     2 proposals     YES
def456..             reject   1 proposal      0 proposals     YES
ghi789..             approve  clean           evaluate+1      CHANGED
                                                              ───────
                                                              2/3 match
```

#### Step 2: Write tests

```go
func TestReplay_CalibrationRequiresPrompt(t *testing.T) {
	err := Replay([]string{"--calibration"})
	if err == nil {
		t.Fatal("expected error for --calibration without --prompt")
	}
}

func TestReplay_CalibrationOrSession(t *testing.T) {
	// Both --session and --calibration should error.
	err := Replay([]string{"--session", "abc", "--calibration", "--prompt", "foo.txt"})
	if err == nil {
		t.Fatal("expected error for both --session and --calibration")
	}
}
```

#### Step 3: Implement the calibration replay loop

The implementation reads each entry from `store.ListCalibrationEntries()`, runs the replay for each, collects results into a summary struct, and prints the table. Each individual replay also persists to `replays/` as in single-session mode.

#### Step 4: Run tests

Run: `go test ./internal/cmd/ -run TestReplay -v`
Expected: PASS

#### Step 5: Run full test suite

Run: `go test ./...`
Expected: all pass.

#### Step 6: Commit

```
feat(cli): add --calibration flag to cabrero replay

When --calibration is set, replays the alternate prompt against every
session in the calibration set and produces a summary table comparing
replay output against recorded decisions. This completes the prompt
iteration workflow: edit prompt → replay against calibration set →
compare → deploy if improved.
```

---

### Task 6: Documentation Updates

**Files:**
- Modify: `DESIGN.md` (mark Phase 5 complete, add calibrate command to CLI listing)
- Modify: `CHANGELOG.md` (add entries under [Unreleased])

#### Step 1: Update DESIGN.md

1. Add `cabrero calibrate` to the subcommands list:
```
cabrero calibrate              Manage calibration set for prompt regression testing
  tag <session_id>               Tag a session as calibration example
    --label approve|reject         Expected outcome (required)
    --note "text"                  Optional note
  untag <session_id>             Remove from calibration set
  list                           List calibration entries
```

2. Add `--calibration` flag to the replay entry:
```
cabrero replay                  Re-run pipeline with a different prompt against a past session
  --session <id>
  --prompt <path>
  --stage classifier|evaluator   Override stage (default: infer from filename)
  --compare                      Diff new output against original and show your decision
  --calibration                  Replay against all sessions in calibration set
```

3. Mark Phase 5 as complete:
```
**Phase 5 — Iteration tooling** ✓

19. **`cabrero prompts`** — lists prompt files with name, version, last-modified time, and path
20. **`cabrero replay`** — re-runs pipeline on past sessions with alternate prompts,
    --compare mode for diffing, --calibration mode for batch regression testing
21. **`cabrero calibrate`** — manages calibration set: tag/untag sessions as
    ground-truth examples (approve/reject + notes) for prompt regression testing
```

4. Add `calibration.json` to the store layout diagram.

#### Step 2: Update CHANGELOG.md

Add under `[Unreleased]`:
```markdown
### Added
- `cabrero prompts` — list prompt files with versions and last-modified times
- `cabrero replay` — re-run pipeline on past sessions with alternate prompt files, supports `--compare` for diffing and `--calibration` for batch regression testing
- `cabrero calibrate` — manage calibration set for prompt regression testing (tag, untag, list)
- `~/.cabrero/replays/` directory for replay output persistence
- `~/.cabrero/calibration.json` for calibration set storage
```

#### Step 3: Commit

```
docs: document Phase 5 iteration tooling

Updates DESIGN.md with prompts, replay, and calibrate commands.
Marks Phase 5 complete. Adds CHANGELOG entries for all new features.
```

---

## Notes for the Implementer

1. **`formatAge` duplication:** Both `prompts.go` and `calibrate.go` need `formatAge()`. Extract it to `internal/cmd/format.go` when implementing Task 4 to avoid redeclaration.

2. **Prompt override refactoring in classifier.go/evaluator.go:** The cleanest approach is to add an unexported `runClassifierCore(sessionID, digest, agg, cfg, systemPrompt string)` that both `RunClassifier` (reads from file → calls core) and `RunClassifierWithPrompt` (uses override → calls core) delegate to. Same for the evaluator. This avoids duplicating ~60 lines of invocation logic.

3. **`store.SessionExists`:** Already exists and is used by `Runner.RunOne`. Verify it works for the replay and calibrate commands.

4. **`ReadClassifierOutput`:** Already exists in `proposal.go:132`. Verify it returns the right type.

5. **No replay history records:** Replays should NOT append to `run_history.jsonl` — they're iteration experiments, not production pipeline runs.

6. **Blocklist integration:** Replay invocations of `claude` should set `CABRERO_SESSION=1` just like normal pipeline runs (this happens automatically in `invokeClaude`).

7. **Test helper reuse:** The batch test infrastructure (`setupBatchStore`, `createBatchSession`, `writeTranscript`) in `runner_test.go` can be reused for replay tests. These are in `internal/pipeline/batch_test.go`.
