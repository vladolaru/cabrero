# Source Registry Design

## Problem

The source manager view (`cabrero review` → Sources tab) shows an empty list because `tui.go` declares `var sourceGroups []fitness.SourceGroup` and never populates it. There is no persistence layer for sources and no discovery mechanism.

## Goal

Auto-discover sources from processed sessions at TUI startup, persist user classifications (ownership, approach) across sessions, and wire mutations in the source manager TUI to the persistence layer.

## Architecture

Three layers:

1. **Persistence** (`store/sources.go`) — JSON file at `~/.cabrero/sources.json`, following the `calibration.go` pattern (mutex + atomic writes)
2. **Discovery** (`store/sources.go`) — scan classifier outputs to extract all observed skills and CLAUDE.md files, infer origin from path patterns
3. **Integration** (`tui/tui.go` + `tui/sources/update.go`) — load + merge at startup, persist mutations on change

## Persistence

The `sources.json` file stores an array of `fitness.Source` entries. Each source is keyed by `Name` (unique). The file stores user classifications and session counts. Ephemeral data (health scores) is not persisted — those come from fitness reports.

```go
// store/sources.go
type sourcesFile struct {
    Sources []fitness.Source `json:"sources"`
}

func ReadSources() ([]fitness.Source, error)
func WriteSources(sources []fitness.Source) error
func UpdateSource(name string, fn func(*fitness.Source)) error
```

API follows the `calibration.go` pattern: package-level mutex, read/write helpers, atomic writes via `store.AtomicWrite`.

## Discovery

At TUI startup, scan all classifier outputs from the `evaluations/` directory. Each classifier output (`*-classifier.json`) contains:

- `SkillSignals[].SkillName` — skill names (e.g., `"docx-helper"`, `"superpowers:writing-plans"`)
- `ClaudeMdSignals[].Path` — CLAUDE.md paths (e.g., `"CLAUDE.md (woo-payments)"`, `"~/.claude/CLAUDE.md"`)

The discovery function scans these files and returns unique sources with session counts:

```go
func DiscoverSourcesFromEvaluations() ([]fitness.Source, error)
```

### Origin inference

Parse the source name/path to determine origin:

| Pattern | Origin | Example |
|---------|--------|---------|
| `~/.claude/skills/<name>/SKILL.md` | `"user"` | User-level skill |
| `CLAUDE.md (<project>)` | `"project:<project>"` | Project instructions |
| `~/.claude/CLAUDE.md` | `"user"` | User-level instructions |
| `<plugin>:<skill>` (colon-namespaced) | `"plugin:<plugin>"` | Plugin skill |
| Bare name (no path, no colon) | `"user"` | Default assumption |

## Merge Strategy

On startup:

1. Load persisted `sources.json` → existing sources (with classifications)
2. Run discovery → discovered sources (just names + session counts)
3. For each discovered source:
   - If it exists in persisted data: keep classifications, update session count
   - If new: add as unclassified (ownership="", approach="")
4. Persisted sources not found in discovery remain (user may have classified something manually)
5. Write merged result back to `sources.json`

## Grouping

`GroupSources(sources []fitness.Source) []fitness.SourceGroup` organizes flat sources into display groups:

- Unclassified sources (ownership `""`) → group labeled "Unclassified" (shown first)
- Sources with origin `"user"` → group labeled "User-level"
- Sources with origin `"project:<name>"` → group labeled "Project: <name>"
- Sources with origin `"plugin:<name>"` → group labeled "Plugin: <name>"

This matches the existing `TestSourceGroups()` structure.

## TUI Integration

### Startup wiring (`tui/tui.go`)

Replace the empty placeholder:

```go
// Before:
var sourceGroups []fitness.SourceGroup

// After:
sources, _ := store.LoadAndMergeSources()
sourceGroups := store.GroupSources(sources)
```

### Mutation persistence (`tui/sources/update.go`)

The `handleToggleFinished` and `handleOwnershipFinished` handlers currently update local model state only. Add `store.UpdateSource()` calls so mutations persist across sessions.

## Not in Scope

- Fitness reports (health scores, evidence) — separate feature
- Change history tracking — needs apply integration
- Daemon-side discovery — TUI startup is sufficient for now
