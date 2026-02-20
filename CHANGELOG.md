# Changelog

All notable changes to Cabrero are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
- **Haiku classifier** (v2 prompt) — goal inference, error classification,
  key turn selection, skill/CLAUDE.md signal assessment, pattern assessment
- **Sonnet evaluator** (v2 prompt) — proposal generation with citation
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

[0.5.0]: https://github.com/vladolaru/cabrero/releases/tag/v0.5.0
