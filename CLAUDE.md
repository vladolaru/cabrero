# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository

**cabrero** — CC auto-improvement system (MIT licensed). Go binary that observes Claude Code sessions, extracts behavioral signals, and proposes improvements to SKILL.md files with human approval. See DESIGN.md for full architecture.

## Documentation

- **DESIGN.md** is the living architecture document. Keep it updated whenever adding, changing, or removing features, commands, pipeline stages, or architectural decisions. If a commit changes behavior documented in DESIGN.md, update DESIGN.md in the same commit.
- **CHANGELOG.md** follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Add an entry under an `[Unreleased]` section for every user-visible change (new feature, fix, removal, deprecation). When a version is tagged, move unreleased entries under the new version heading with the release date.

## Snapshots

Generate PNG/SVG screenshots of TUI views using `cmd/snapshot` + [freeze](https://github.com/charmbracelet/freeze).

```bash
make snapshots                  # all views → snapshots/*.{svg,png}
make snapshot VIEW=dashboard    # single view
```

The snapshot command can also be run directly for previewing ANSI output:

```bash
go run ./cmd/snapshot dashboard          # default 120×40
go run ./cmd/snapshot dashboard -w 80    # custom width
```

Available views: `dashboard`, `dashboard-narrow`, `dashboard-empty`, `proposal-detail`, `proposal-detail-chat`, `fitness-report`, `source-manager`, `pipeline-monitor`, `help-overlay`, `help-overlay-vim`.

When adding a new TUI view, add a render function in `cmd/snapshot/main.go` and register the view name in both the `views` slice and the `SNAPSHOT_VIEWS` Makefile variable.

## Git

- Remote: `git@github.com:vladolaru/cabrero.git`
- Main branch: `main`
