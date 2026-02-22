# Model Configurability & Visibility Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make pipeline models (classifier, evaluator) configurable at runtime via CLI flags and config.json, and surface active models across all CLI/TUI surfaces.

**Architecture:** Expand `PipelineConfig` with `ClassifierModel` and `EvaluatorModel` fields. Resolution: CLI flag → config.json → compile-time default. Rename existing constants to `DefaultClassifierModel`/`DefaultEvaluatorModel`. Add `store.ReadModelConfig()` helper. Surface models in `status`, `run`, `backfill`, `doctor`, and TUI pipeline monitor.

**Tech Stack:** Go, Bubble Tea TUI, lipgloss styling

---

### Task 1: Rename model constants to Default prefixes

**Files:**
- Modify: `internal/pipeline/classifier.go:17-18`
- Modify: `internal/pipeline/evaluator.go:19-20`

**Step 1: Rename ClassifierModel constant**

In `internal/pipeline/classifier.go`, change:
```go
// ClassifierModel is the Claude model used for classification.
const ClassifierModel = "claude-haiku-4-5"
```
to:
```go
// DefaultClassifierModel is the compile-time default model for classification.
const DefaultClassifierModel = "claude-haiku-4-5"
```

**Step 2: Rename EvaluatorModel constant**

In `internal/pipeline/evaluator.go`, change:
```go
// EvaluatorModel is the Claude model used for evaluation.
const EvaluatorModel = "claude-sonnet-4-6"
```
to:
```go
// DefaultEvaluatorModel is the compile-time default model for evaluation.
const DefaultEvaluatorModel = "claude-sonnet-4-6"
```

**Step 3: Update all references to the old constant names**

The following files reference `ClassifierModel` and `EvaluatorModel`:
- `internal/pipeline/runner.go:135,137` — `buildBaseRecord()` uses both constants
- `internal/pipeline/runner_test.go` — tests compare against the constants
- `internal/pipeline/history_test.go` — test data uses the constants

For now, update `runner.go:135` from `ClassifierModel` to `DefaultClassifierModel` and `runner.go:137` from `EvaluatorModel` to `DefaultEvaluatorModel`. Update the test files similarly.

Note: `classifier.go:54` and `evaluator.go:59,151` also reference the constants — these will be changed in Task 3 to use `cfg.ClassifierModel`/`cfg.EvaluatorModel` instead.

**Step 4: Verify the build compiles**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./...`
Expected: Clean build

**Step 5: Run tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go test ./internal/pipeline/...`
Expected: All tests pass

**Step 6: Commit**

```
feat(pipeline): rename model constants to Default prefixes

Rename ClassifierModel → DefaultClassifierModel and
EvaluatorModel → DefaultEvaluatorModel to clarify they are
compile-time fallback defaults, not necessarily the active models.

Prepares for runtime model configurability.
```

---

### Task 2: Add model fields to PipelineConfig and store helper

**Files:**
- Modify: `internal/pipeline/pipeline.go:37-62`
- Modify: `internal/store/store.go` (add `ReadModelConfig()` after `ReadDebugFlag()`)

**Step 1: Write the test for `store.ReadModelConfig()`**

Create or add to the store test file. The function should read `classifierModel` and `evaluatorModel` from `~/.cabrero/config.json` and return zero-value strings for missing fields.

Since `ReadDebugFlag()` has no dedicated test (it reads the real config.json), follow the same pattern: `ReadModelConfig()` is a simple JSON reader — test it via the integration with `DefaultPipelineConfig()` in Task 2 Step 4.

**Step 2: Add `ReadModelConfig()` to `internal/store/store.go`**

Add this function directly after the existing `ReadDebugFlag()` (after line ~178):

```go
// ModelConfig holds optional model overrides from config.json.
type ModelConfig struct {
	ClassifierModel string `json:"classifierModel"`
	EvaluatorModel  string `json:"evaluatorModel"`
}

// ReadModelConfig reads model overrides from ~/.cabrero/config.json.
// Returns zero-value fields for missing file, malformed JSON, or absent keys.
func ReadModelConfig() ModelConfig {
	data, err := os.ReadFile(filepath.Join(Root(), "config.json"))
	if err != nil {
		return ModelConfig{}
	}
	var cfg ModelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ModelConfig{}
	}
	return cfg
}
```

**Step 3: Add model fields to `PipelineConfig`**

In `internal/pipeline/pipeline.go`, expand the struct (after line 41, before `Logger`):

```go
// PipelineConfig controls LLM invocation parameters.
type PipelineConfig struct {
	ClassifierModel    string
	EvaluatorModel     string
	ClassifierMaxTurns int
	EvaluatorMaxTurns  int
	ClassifierTimeout  time.Duration
	EvaluatorTimeout   time.Duration
	Logger             Logger // nil defaults to stdLogger (stdout/stderr)
	Debug              bool   // persist CC sessions for classifier/evaluator
}
```

**Step 4: Update `DefaultPipelineConfig()` to read from config.json**

```go
// DefaultPipelineConfig returns production defaults.
// Model names are resolved from config.json, falling back to compile-time defaults.
func DefaultPipelineConfig() PipelineConfig {
	models := store.ReadModelConfig()
	classifierModel := DefaultClassifierModel
	if models.ClassifierModel != "" {
		classifierModel = models.ClassifierModel
	}
	evaluatorModel := DefaultEvaluatorModel
	if models.EvaluatorModel != "" {
		evaluatorModel = models.EvaluatorModel
	}
	return PipelineConfig{
		ClassifierModel:    classifierModel,
		EvaluatorModel:     evaluatorModel,
		ClassifierMaxTurns: 15,
		EvaluatorMaxTurns:  20,
		ClassifierTimeout:  2 * time.Minute,
		EvaluatorTimeout:   5 * time.Minute,
	}
}
```

Add `"github.com/vladolaru/cabrero/internal/store"` to the imports in `pipeline.go`.

**Step 5: Verify the build compiles**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./...`
Expected: Clean build

**Step 6: Run tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go test ./internal/pipeline/... ./internal/store/...`
Expected: All tests pass

**Step 7: Commit**

```
feat(pipeline): add model fields to PipelineConfig

Add ClassifierModel and EvaluatorModel fields to PipelineConfig.
DefaultPipelineConfig() reads config.json via store.ReadModelConfig(),
falling back to compile-time defaults.

Resolution order: CLI flag → config.json → compile-time default.
```

---

### Task 3: Wire model fields through classifier and evaluator

**Files:**
- Modify: `internal/pipeline/classifier.go:54`
- Modify: `internal/pipeline/evaluator.go:59,151`
- Modify: `internal/pipeline/runner.go:135,137`

**Step 1: Update classifier to use `cfg.ClassifierModel`**

In `internal/pipeline/classifier.go:54`, change:
```go
Model:          ClassifierModel,
```
to:
```go
Model:          cfg.ClassifierModel,
```

**Step 2: Update evaluator to use `cfg.EvaluatorModel`**

In `internal/pipeline/evaluator.go:59`, change:
```go
Model:          EvaluatorModel,
```
to:
```go
Model:          cfg.EvaluatorModel,
```

In `internal/pipeline/evaluator.go:151`, change:
```go
Model:          EvaluatorModel,
```
to:
```go
Model:          cfg.EvaluatorModel,
```

**Step 3: Update `buildBaseRecord()` to use config fields**

In `internal/pipeline/runner.go:135-137`, change:
```go
ClassifierModel:         DefaultClassifierModel,
ClassifierPromptVersion: strings.TrimSuffix(classifierPromptFile, ".txt"),
EvaluatorModel:          DefaultEvaluatorModel,
```
to:
```go
ClassifierModel:         r.Config.ClassifierModel,
ClassifierPromptVersion: strings.TrimSuffix(classifierPromptFile, ".txt"),
EvaluatorModel:          r.Config.EvaluatorModel,
```

**Step 4: Update tests that reference the renamed constants**

In `internal/pipeline/runner_test.go`, update the comparison values:
- `ClassifierModel` → `DefaultClassifierModel`
- `EvaluatorModel` → `DefaultEvaluatorModel`

In `internal/pipeline/history_test.go`, update test data:
- `ClassifierModel` → `DefaultClassifierModel`
- `EvaluatorModel` → `DefaultEvaluatorModel`

**Step 5: Verify build and tests**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./... && go test ./internal/pipeline/...`
Expected: Clean build and all tests pass

**Step 6: Commit**

```
feat(pipeline): wire configurable models through classifier and evaluator

Classifier and evaluator now read model names from PipelineConfig
instead of compile-time constants. History records are populated
from the config, ensuring they reflect the actually-used model.
```

---

### Task 4: Add CLI flags to run, backfill, and daemon commands

**Files:**
- Modify: `internal/cmd/run.go:14-37`
- Modify: `internal/cmd/backfill.go:29-68`
- Modify: `internal/cmd/daemon.go:22-37`

**Step 1: Add model flags to `run` command**

In `internal/cmd/run.go`, after the existing flag definitions (after line 20), add:

```go
classifierModel := fs.String("classifier-model", defaults.ClassifierModel, "Claude model for Classifier")
evaluatorModel := fs.String("evaluator-model", defaults.EvaluatorModel, "Claude model for Evaluator")
```

Then in the config assembly block (around line 32-37), add:

```go
cfg.ClassifierModel = *classifierModel
cfg.EvaluatorModel = *evaluatorModel
```

**Step 2: Add model flags to `backfill` command**

In `internal/cmd/backfill.go`, after the existing pipeline flag definitions (around line 33), add:

```go
classifierModel := fs.String("classifier-model", "", "override Classifier model")
evaluatorModel := fs.String("evaluator-model", "", "override Evaluator model")
```

In the config override block (around line 56-68), add:

```go
if *classifierModel != "" {
	cfg.ClassifierModel = *classifierModel
}
if *evaluatorModel != "" {
	cfg.EvaluatorModel = *evaluatorModel
}
```

**Step 3: Add model flags to `daemon` command**

In `internal/cmd/daemon.go`, after the existing pipeline flag definitions (around line 25), add:

```go
classifierModel := fs.String("classifier-model", cfg.Pipeline.ClassifierModel, "Claude model for Classifier")
evaluatorModel := fs.String("evaluator-model", cfg.Pipeline.EvaluatorModel, "Claude model for Evaluator")
```

In the config assembly block (around line 37), add:

```go
cfg.Pipeline.ClassifierModel = *classifierModel
cfg.Pipeline.EvaluatorModel = *evaluatorModel
```

**Step 4: Verify build**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./...`
Expected: Clean build

**Step 5: Commit**

```
feat(cmd): add --classifier-model and --evaluator-model flags

All three pipeline commands (run, backfill, daemon) now accept
model override flags. Resolution: CLI flag → config.json → default.
```

---

### Task 5: Surface models in `cabrero status`

**Files:**
- Modify: `internal/cmd/status.go:86-91`

**Step 1: Add Pipeline section to status output**

In `internal/cmd/status.go`, add a new section before the Debug section (before line 87). Add the `pipeline` package import.

Insert after the Hooks line (after line 85):

```go
// Pipeline models and prompts.
cfg := pipeline.DefaultPipelineConfig()
prompts, _ := pipeline.ListPromptVersions()
classifierPrompt := ""
evaluatorPrompt := ""
for _, p := range prompts {
	if p.Name == "classifier" {
		classifierPrompt = p.Version
	}
	if p.Name == "evaluator" {
		evaluatorPrompt = p.Version
	}
}
fmt.Printf("  %s\n", cli.Bold("Pipeline:"))
clsLine := fmt.Sprintf("    Classifier:  %s", cfg.ClassifierModel)
if classifierPrompt != "" {
	clsLine += cli.Muted(fmt.Sprintf("  (prompt: %s)", classifierPrompt))
}
fmt.Println(clsLine)
evalLine := fmt.Sprintf("    Evaluator:   %s", cfg.EvaluatorModel)
if evaluatorPrompt != "" {
	evalLine += cli.Muted(fmt.Sprintf("  (prompt: %s)", evaluatorPrompt))
}
fmt.Println(evalLine)
```

Add `"github.com/vladolaru/cabrero/internal/pipeline"` to the imports.

**Step 2: Verify build and manual test**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./... && go run . status`
Expected: New "Pipeline" section with model names and prompt versions.

**Step 3: Commit**

```
feat(status): show active pipeline models and prompt versions

The status command now displays which models and prompt versions
the pipeline will use, resolved from config.json or defaults.
```

---

### Task 6: Surface models in `cabrero run`

**Files:**
- Modify: `internal/cmd/run.go:30`

**Step 1: Add model info line after session ID announcement**

In `internal/cmd/run.go`, after line 30 (`fmt.Printf("Running pipeline on session %s\n", sessionID)`), add:

```go
fmt.Printf("  Models: classifier=%s, evaluator=%s\n", cfg.ClassifierModel, cfg.EvaluatorModel)
```

Note: `cfg` is assembled a few lines later (lines 32-37). Move the model info print after line 37 (after `cfg.Debug = *debug`), so the config is fully assembled:

```go
cfg.Debug = *debug

fmt.Printf("  Models: classifier=%s, evaluator=%s\n", cfg.ClassifierModel, cfg.EvaluatorModel)
```

**Step 2: Verify build**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./...`
Expected: Clean build

**Step 3: Commit**

```
feat(run): show active models before pipeline execution

Prints the classifier and evaluator model names at the start
of a pipeline run so the user knows which models are in use.
```

---

### Task 7: Fix backfill preview to use config-derived model names

**Files:**
- Modify: `internal/cmd/backfill.go:141-204`

**Step 1: Update `showBackfillPreview` signature to accept `PipelineConfig`**

Change the function signature from:
```go
func showBackfillPreview(sessions []store.Metadata, filter store.SessionFilter) {
```
to:
```go
func showBackfillPreview(sessions []store.Metadata, filter store.SessionFilter, cfg pipeline.PipelineConfig) {
```

**Step 2: Replace hardcoded model prose with config values**

In lines 202-203, change:
```go
fmt.Printf("    Classifier: %d invocations (Haiku — low cost)\n", len(sessions))
fmt.Printf("    Evaluator:  up to %d batch invocations (Sonnet — one per project)\n", len(projectOrder))
```
to:
```go
fmt.Printf("    Classifier: %d invocations (%s)\n", len(sessions), cfg.ClassifierModel)
fmt.Printf("    Evaluator:  up to %d batch invocations (%s)\n", len(projectOrder), cfg.EvaluatorModel)
```

**Step 3: Update the caller to pass `cfg`**

In the `Backfill` function (around line 71), change:
```go
showBackfillPreview(sessions, filter)
```
to:
```go
showBackfillPreview(sessions, filter, cfg)
```

**Step 4: Verify build**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./...`
Expected: Clean build

**Step 5: Commit**

```
fix(backfill): derive model names from config instead of hardcoded prose

The backfill preview now shows the actual model names from config
instead of hardcoded "Haiku" and "Sonnet" strings that would
silently drift if models were changed.
```

---

### Task 8: Surface models in `cabrero doctor`

**Files:**
- Modify: `internal/cmd/doctor.go:1008-1101`

**Step 1: Add model checks to `checkPipeline()`**

At the start of the `checkPipeline()` function (after line 1009, before the session store check), add model reporting:

```go
// Active models.
cfg := pipeline.DefaultPipelineConfig()
results = append(results, checkResult{
	name:     "Classifier model",
	category: "Pipeline",
	status:   checkPass,
	message:  cfg.ClassifierModel,
})
results = append(results, checkResult{
	name:     "Evaluator model",
	category: "Pipeline",
	status:   checkPass,
	message:  cfg.EvaluatorModel,
})
```

Ensure `"github.com/vladolaru/cabrero/internal/pipeline"` is in the imports for `doctor.go`.

**Step 2: Verify build and manual test**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./... && go run . doctor`
Expected: Pipeline section shows model names with ✓ marks.

**Step 3: Commit**

```
feat(doctor): report active pipeline models

The doctor command now shows which models the pipeline will use
under the Pipeline diagnostics category.
```

---

### Task 9: Surface models in TUI Pipeline Monitor

**Files:**
- Modify: `internal/tui/pipeline/view.go:64-67,284-296`

**Step 1: Add MODELS section to the view**

In `internal/tui/pipeline/view.go`, in the `View()` method, add a models section before the prompts section.

After line 63 (after the recent runs section is appended) and before the prompts block (line 65), insert:

```go
// Models: hidden in narrow mode.
if m.layoutMode() != layoutNarrow {
	sections = append(sections, m.renderModels())
}
```

**Step 2: Add `renderModels()` method**

Add this method to `view.go`, for example right before `renderPrompts()`:

```go
func (m Model) renderModels() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("MODELS"))
	b.WriteString("\n")

	cfg := pl.DefaultPipelineConfig()
	b.WriteString(fmt.Sprintf("  Classifier:  %s", cfg.ClassifierModel))
	if cfg.ClassifierModel != pl.DefaultClassifierModel {
		b.WriteString(warningStyle.Render("  (override)"))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Evaluator:   %s", cfg.EvaluatorModel))
	if cfg.EvaluatorModel != pl.DefaultEvaluatorModel {
		b.WriteString(warningStyle.Render("  (override)"))
	}
	return b.String()
}
```

**Step 3: Verify build**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./...`
Expected: Clean build

**Step 4: Commit**

```
feat(tui): add MODELS section to pipeline monitor

Shows active classifier and evaluator model names. Flags
non-default models with an "(override)" indicator.
Hidden in narrow layout alongside prompts.
```

---

### Task 10: Update DESIGN.md and CHANGELOG.md

**Files:**
- Modify: `DESIGN.md` — document model configurability
- Modify: `CHANGELOG.md` — add entries under `[Unreleased]`

**Step 1: Update DESIGN.md**

Find the pipeline configuration section and add documentation for model configurability:
- Resolution order: CLI flag → config.json → compile-time default
- `config.json` schema: `classifierModel`, `evaluatorModel`
- CLI flags: `--classifier-model`, `--evaluator-model` on `run`, `backfill`, `daemon`

**Step 2: Update CHANGELOG.md**

Add under `[Unreleased]`:

```markdown
### Added
- Pipeline models (classifier, evaluator) are now configurable via CLI flags (`--classifier-model`, `--evaluator-model`) and `config.json` (`classifierModel`, `evaluatorModel`).
- `cabrero status` shows active pipeline models and prompt versions.
- `cabrero doctor` reports active pipeline models under Pipeline diagnostics.
- `cabrero run` prints active model names before pipeline execution.
- TUI pipeline monitor shows a MODELS section with override detection.

### Changed
- `cabrero backfill` preview derives model names from configuration instead of hardcoded prose.
```

**Step 3: Commit**

```
docs: document model configurability

Update DESIGN.md with the config resolution chain and config.json
schema. Add CHANGELOG.md entries for all model visibility surfaces.
```

---

### Task 11: Final verification

**Step 1: Full build**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go build ./...`
Expected: Clean build

**Step 2: Full test suite**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go test ./...`
Expected: All tests pass

**Step 3: Manual smoke test**

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go run . status`
Expected: Pipeline section shows models + prompts

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go run . doctor`
Expected: Pipeline section shows model check marks

**Step 4: Verify config.json override works**

Add to `~/.cabrero/config.json`: `"classifierModel": "claude-haiku-4-5"` (same value, just to test the read path).

Run: `cd /Users/vladolaru/Work/a8c/cabrero && go run . status`
Expected: Same output (config matches default).
