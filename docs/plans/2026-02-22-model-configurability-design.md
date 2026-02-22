# Model Configurability & Visibility

## Problem

Pipeline models (classifier: `claude-haiku-4-5`, evaluator: `claude-sonnet-4-6`) are
compile-time constants. Users cannot override them without rebuilding, and no CLI/TUI
surface shows which models are active.

## Design

### Configuration Layer

**Resolution order:** CLI flag → `config.json` → compile-time default.

1. **Rename constants** in `pipeline/classifier.go` and `pipeline/evaluator.go`:
   - `ClassifierModel` → `DefaultClassifierModel`
   - `EvaluatorModel` → `DefaultEvaluatorModel`

2. **Expand `PipelineConfig`** (`pipeline/pipeline.go`):
   - Add `ClassifierModel string` and `EvaluatorModel string` fields.
   - `DefaultPipelineConfig()` reads `config.json` via `store.ReadModelConfig()`,
     falling back to the renamed compile-time defaults.

3. **New `store.ReadModelConfig()`** (`store/store.go`):
   - Reads `classifierModel` and `evaluatorModel` from `~/.cabrero/config.json`.
   - Returns zero-value strings for missing/malformed fields (caller uses defaults).
   - Same pattern as existing `ReadDebugFlag()`.

4. **CLI flags** on `run`, `backfill`, `daemon` commands:
   - `--classifier-model` (default: resolved from config.json → compile-time default)
   - `--evaluator-model` (default: resolved from config.json → compile-time default)

### Pipeline Usage

- `classifier.go`: `RunClassifier()` reads `cfg.ClassifierModel` instead of the constant.
- `evaluator.go`: `RunEvaluator()`/`RunEvaluatorBatch()` reads `cfg.EvaluatorModel`.
- `runner.go`: `buildBaseRecord()` writes `r.Config.ClassifierModel`/`r.Config.EvaluatorModel`
  to `HistoryRecord` (already has these fields).

### Visibility Surfaces

| Surface | Change |
|---------|--------|
| `cabrero status` | New "Pipeline" section: models + prompt versions |
| TUI Pipeline Monitor | New "MODELS" section (hidden in narrow layout) |
| `cabrero run` | One-liner showing active models before pipeline runs |
| `cabrero backfill` preview | Derive model names from config instead of hardcoded prose |
| `cabrero doctor` | Report active models under Pipeline category |

### What Doesn't Change

- `HistoryRecord` schema (already has model fields).
- Prompt file reading mechanism.
- No environment variable layer.
- TUI `shared.Config` (separate concern — display settings only).
- No model name validation (delegated to `claude` CLI).
