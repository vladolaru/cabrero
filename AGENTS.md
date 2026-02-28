# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Repository

**cabrero** — CC auto-improvement system (MIT licensed). Go binary that observes Claude Code sessions, extracts behavioral signals, and proposes improvements to SKILL.md files with human approval. See DESIGN.md for full architecture.

**Authorship model:** This codebase is AI-agent-written and AI-agent-maintained. The human owner directs work but does not hold the code in memory. No agent carries context between sessions — every agent reads the code cold. Implications:

- Prefer single canonical implementations over duplicated patterns. An agent will copy whichever pattern it encounters first; if two conventions exist for the same thing, drift is inevitable.
- Constants, types, and named helpers are more valuable here than in a human-authored codebase — they are the primary mechanism by which agents discover the "right" way to do something.
- When refactoring, consolidating duplicated logic is not polish — it is drift prevention.

## Documentation

- **DESIGN.md** is the living architecture document. Keep it updated whenever adding, changing, or removing features, commands, pipeline stages, or architectural decisions. If a commit changes behavior documented in DESIGN.md, update DESIGN.md in the same commit.
- **CHANGELOG.md** follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Add an entry under an `[Unreleased]` section for every user-visible change (new feature, fix, removal, deprecation). When a version is tagged, move unreleased entries under the new version heading with the release date.
- **docs/claude-cli-settings-and-hooks.md** — reference for `--setting-sources`, `--settings`, and `disableAllHooks` when invoking the `claude` CLI programmatically. Consult when modifying `invokeClaude`, `buildChatArgs`, or any code that spawns `claude` subprocesses.
- **docs/subprocess-isolation.md** — complete isolation model for `claude` CLI subprocesses: env vars (`CLAUDECODE`, `CABRERO_SESSION`), CLI flags, settings overrides, per-mode specifics, and what still leaks through. Consult when adding new subprocess invocation sites or debugging isolation failures.

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

Version is derived from `git describe --tags` at build time (see `Makefile`). There is no version constant to update manually. A clean tag build shows `v0.9.3`; a dirty build shows `v0.9.3-2-gabcdef`.

**Determine the next version** from the latest tag using semver and the commits since that tag:

| Commits contain | Bump | Example |
|-----------------|------|---------|
| `feat!:` or `BREAKING CHANGE:` | MAJOR | 0.8.1 → 1.0.0 |
| `feat:` | MINOR | 0.8.1 → 0.9.0 |
| `fix:`, `perf:`, `refactor:`, `docs:`, etc. | PATCH | 0.8.1 → 0.8.2 |

Use the highest applicable bump. Check with `git log --oneline $(git describe --tags --abbrev=0)..HEAD`.

**Steps:**

1. Review commits since last tag: `git log --oneline $(git describe --tags --abbrev=0)..HEAD`.
2. Update `CHANGELOG.md`: move any existing `[Unreleased]` entries and add new ones for commits not yet covered, all under a new `## [X.Y.Z] - YYYY-MM-DD` heading. Add the `[X.Y.Z]` comparison link at the bottom of the file.
3. Commit: `chore: release vX.Y.Z`.
4. Tag: `git tag vX.Y.Z`.
5. Push both: `git push origin HEAD && git push origin vX.Y.Z`.
6. Install and verify:

```bash
make install && cabrero version   # should show clean vX.Y.Z
```

7. Restart the daemon so it picks up the new binary:

```bash
launchctl unload ~/Library/LaunchAgents/com.cabrero.daemon.plist
launchctl load ~/Library/LaunchAgents/com.cabrero.daemon.plist
```

Or equivalently: `cabrero setup --yes` (detects plist is current, restarts daemon if needed).

8. Smoke test:

```bash
cabrero doctor    # all checks should pass
cabrero status    # daemon running, hooks registered
```

**Important:** `make install` copies the binary and re-signs it (`codesign -s -`). Without the re-sign step, macOS kills the binary due to an invalidated ad-hoc code signature. If you see `killed` when running cabrero after install, check that the Makefile includes the codesign step.

## CLI Quick Reference

Full details in DESIGN.md § Cabrero CLI. Common operations:

```bash
cabrero status                  # pipeline health: sessions, daemon, hooks
cabrero sessions --status error # list sessions by status (queued/imported/processed/error)
cabrero run <session_id>        # re-process a single session manually
cabrero proposals               # list pending proposals
cabrero inspect <proposal_id>   # show proposal with citation chain
cabrero approve <proposal_id>   # approve and apply
cabrero reject <proposal_id>    # reject with optional reason
cabrero proposals --status approved  # list archived proposals by outcome
cabrero defer <proposal_id>          # defer proposal for later
cabrero rollback <change_id>         # restore file to pre-change content
cabrero blocklist list               # show blocked sessions
cabrero blocklist add <session_id>   # block a session
cabrero history --status error       # show errored pipeline runs
cabrero sources list                 # show tracked sources
cabrero sources set-ownership <name> mine  # set source ownership
cabrero doctor                  # diagnose issues, --fix to auto-repair
cabrero reset-breaker           # reset circuit breaker to resume queue processing
cabrero config list --defaults   # show all config with default annotations
cabrero config get <key>         # read a single value
cabrero config set <key> <value> # set a value
cabrero config unset <key>       # revert to default
cabrero backfill                # bulk re-process sessions
  --retry-errors                #   include errored sessions
  --enqueue                     #   queue for daemon (non-blocking)
  --since 2026-02-20            #   date filter
  --project cabrero             #   project filter
  --dry-run                     #   preview only
cabrero replay --session <id> --prompt <path>  # test alternate prompts
cabrero calibrate tag <id> --label approve     # tag calibration examples
```

## Git

- Remote: `git@github.com:vladolaru/cabrero.git`
- Main branch: `main`

## Code Review

- Always read the flagged file:line before adding any review finding to an action plan. AI review agents commonly misread control flow, miss existing fixes, and misidentify language constructs — producing false positives even on critical findings. Classify each finding as CONFIRMED / FALSE_POSITIVE / LIKELY_VALID before building the plan. Details: .claude/docs/patterns/2026-02-26-verify-review-findings.md
