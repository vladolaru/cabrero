# Changelog

All notable changes to Cabrero are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `cabrero backfill` command to process existing sessions through the full
  pipeline with `--since`, `--until`, `--project`, `--retry-errors` filtering,
  preview with confirmation, and smart batching via `pipeline.BatchProcessor`.
- Setup wizard Step 8: offers to import and process existing CC sessions after
  installation (imports in quiet mode, counts pending, offers backfill with
  configurable lookback — default 1 month, skippable).
- `store.QuerySessions` for filtered session queries by date range, project
  substring, and status. Returns oldest-first.
- `pipeline.BatchProcessor` as shared smart batching infrastructure with
  configurable max batch size (default 10) and progress callbacks. Used by
  both daemon and backfill command.

### Changed

- `cabrero import` now runs the pre-parser on each imported session to generate
  digests. `RunImport` function available for programmatic use (quiet mode).
- Daemon batching logic refactored into `pipeline.BatchProcessor` (no behavior
  change).
- `store.MarkProcessed` and `store.MarkError` extracted as public helpers.

## [0.6.0] - 2026-02-20

### Changed

- **Agentic evaluators** — Classifier and Evaluator now run in
  agentic mode with Read/Grep tool access instead of single-shot `--print`.
  Classifier can verify ambiguous signals by reading raw JSONL turns (scoped to
  `~/.cabrero/raw/`). Evaluator can read current skill files and CLAUDE.md to
  inform proposals (unrestricted filesystem access). Both have prompt-based
  turn budgets and wall-clock timeouts.
- **Triage gate** — Classifier now outputs a `triage` field (`"evaluate"` or
  `"clean"`). Clean sessions skip the Evaluator entirely, reducing
  cost for sessions with no actionable signals.
- **Smart batching** — daemon groups pending sessions by project, runs Classifier
  individually (cheap triage), then batches sessions flagged as "evaluate"
  into a single Evaluator invocation per project. Gives Evaluator cross-session
  context within one call while keeping Classifier independent.
- **Prompt versions** — Classifier upgraded to v3 (`classifier-v3.txt`),
  Evaluator upgraded to v3 (`evaluator-v3.txt`).
- **Model-agnostic naming** — renamed "Haiku classifier" → "Classifier" and
  "Sonnet evaluator" → "Evaluator" throughout the codebase, CLI flags, file
  suffixes, and documentation. Decouples pipeline stage names from the Claude
  models that back them. CLI flags renamed: `--haiku-*` → `--classifier-*`,
  `--sonnet-*` → `--evaluator-*`. Output files renamed: `*-haiku.json` →
  `*-classifier.json`, `*-sonnet.json` → `*-evaluator.json`.

### Added

- **CLI flags** — `--classifier-max-turns`, `--evaluator-max-turns`, `--classifier-timeout`,
  `--evaluator-timeout` on both `cabrero daemon` and `cabrero run` for tuning
  agentic evaluator limits.

- **`cabrero uninstall`** — clean removal command that reverses setup: stops
  daemon, removes LaunchAgent, unregisters Claude Code hooks, deletes hook
  scripts and binary. Prompts whether to keep `~/.cabrero` data for
  reinstallation or remove everything. `--yes` skips confirmations,
  `--keep-data`/`--remove-data` control data directory without prompting.

- **`cabrero doctor`** — comprehensive diagnostic command that checks store,
  hook scripts, Claude Code integration, LaunchAgent, daemon, PATH, and
  pipeline health. Reports issues with severity (pass/warn/fail) and offers
  auto-fix for stale hooks, missing registrations, broken LaunchAgent, and
  stopped daemon. `--fix` auto-fixes without prompting, `--json` for scripted
  usage.

## [0.5.0] - 2026-02-20

First tagged release. Covers Phases 0–3.5 of the design.

### Added

- **Capture layer** — `PreCompact` and `SessionEnd` hook scripts back up CC
  transcripts to `~/.cabrero/raw/` with metadata and loop prevention
- **Store** — `~/.cabrero/` directory layout with raw backups, digests,
  evaluations, proposals, prompts, and session ID blocklist
- **Pre-parser** — JSONL → structured digest with citations, compaction
  segments, error attribution, and friction signals (empty search results,
  search fumbles, backtracking)
- **Cross-session pattern aggregator** — detects recurring errors and
  error-prone tool sequences across 3+ project sessions
- **Classifier** (v2 prompt) — goal inference, error classification,
  key turn selection, skill/CLAUDE.md signal assessment, pattern assessment
- **Evaluator** (v2 prompt) — proposal generation with citation
  validation and `skill_scaffold` proposals for recurring patterns
- **Background daemon** — `cabrero daemon` polls for pending sessions, runs
  pipeline, sends macOS notifications, with stale session recovery, PID-based
  single instance, graceful shutdown, and file logging with rotation
- **LaunchAgent** plist template for auto-start on login
- **CLI commands** — `run`, `sessions`, `status`, `proposals`, `inspect`,
  `import`, `daemon`, `setup`, `update`
- **`cabrero setup`** — interactive wizard: prerequisite checks, store init,
  hook installation from embedded scripts, Claude Code hook registration,
  LaunchAgent install, daemon start, PATH check (`--yes`, `--dry-run`)
- **`cabrero update`** — self-update from GitHub Releases with SHA256
  checksum verification and atomic binary replacement (`--check`)
- **Install script** — `install.sh` for curl-pipe-bash one-liner distribution
- **Build infrastructure** — goreleaser config (darwin/amd64 + darwin/arm64),
  GitHub Actions release workflow on tag push, Makefile for local dev
- **Hook scripts embedded** in binary via `//go:embed`

### Fixed

- Import uses original file timestamps and tracks project metadata
- Store preserves hyphens in project display names
- Session-end hook always overwrites transcript (superset of pre-compact)
- Parser attributes errors to tool names and increments ErrorCount
- Parser emits `[]` instead of `null` for empty slices
- Pipeline disables skills and tools in LLM invocations

[0.6.0]: https://github.com/vladolaru/cabrero/releases/tag/v0.6.0
[0.5.0]: https://github.com/vladolaru/cabrero/releases/tag/v0.5.0
