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

## Releasing

Version is derived from `git describe --tags` at build time (see `Makefile`). There is no version constant to update manually.

**Determine the next version** from the latest tag using semver and the commits since that tag:

| Commits contain | Bump | Example |
|-----------------|------|---------|
| `feat!:` or `BREAKING CHANGE:` | MAJOR | 0.8.1 → 1.0.0 |
| `feat:` | MINOR | 0.8.1 → 0.9.0 |
| `fix:`, `perf:`, `refactor:`, `docs:`, etc. | PATCH | 0.8.1 → 0.8.2 |

Use the highest applicable bump. Check with `git log --oneline $(git describe --tags --abbrev=0)..HEAD`.

**Steps:**

1. Move any `[Unreleased]` entries in `CHANGELOG.md` under a new `## [X.Y.Z] - YYYY-MM-DD` heading and add the comparison link at the bottom of the file.
2. Commit as `chore: release vX.Y.Z`.
3. Tag: `git tag vX.Y.Z`.
4. Push commit and tag: `git push origin HEAD && git push origin vX.Y.Z`.

## Git

- Remote: `git@github.com:vladolaru/cabrero.git`
- Main branch: `main`
