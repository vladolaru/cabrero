# Changelog

All notable changes to Cabrero are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.21.0] - 2026-02-26

### Added
- **Proposals Curator stage** — daily automated cleanup of the proposal backlog via a
  third daemon ticker (24h interval). Two-stage pipeline:
  - **Stage 1 (Haiku batch check):** single-proposal file-target proposals are sent to
    `claude-haiku-4-5` in one non-agentic batch call. Proposals whose changes are already
    present in the target file are auto-rejected.
  - **Stage 2 (Sonnet group curator):** multi-proposal targets (2+ proposals for the same
    file) each get an agentic `claude-sonnet-4-6` session with `Read,Grep` access.
    The Curator identifies concern clusters, synthesizes one new proposal per cluster for
    `claude_addition`, and rank-culls `skill_improvement`/`claude_review` proposals.
    `skill_scaffold` proposals are always preserved.
- **`cleanup_history.jsonl`** — append-only JSONL audit log of every cleanup run, stored
  at `~/.cabrero/cleanup_history.jsonl`. Each record captures timestamp, duration, before/after
  proposal counts, per-proposal decisions, and LLM usage for Sonnet Curator and Haiku check
  calls. Rotated on daemon startup (90-day retention, same as `run_history.jsonl`).
- **Curator prompt files** — `~/.cabrero/prompts/curator-v1.txt` and
  `~/.cabrero/prompts/curator-check-v1.txt` written on first curator run if absent.
- **`PipelineRun.Source` field** — pipeline run records now carry a `Source` string
  (`"daemon"`, `"cli-run"`, `"cli-backfill"`, `"cleanup"`). Cleanup runs surface in the
  TUI Pipeline Activity section as `CLEANUP` rows with archived-count and cost.
- **`cleanLLMJSON` array support** — the JSON extractor now handles `[...]` array output
  in addition to `{...}` objects, required for the Haiku batch check response format.
- **`store.RootOverrideForTest` / `ResetRootOverrideForTest`** — test helpers for
  redirecting the store root in unit tests without environment variables.

## [0.20.3] - 2026-02-26

### Fixed
- **Stale proposals after actions** — approve, reject, defer, and dismiss actions
  no longer leave the acted-on item in the dashboard until restart. The proposal
  list and `PendingCount` header now reload from disk after each action.
  Reject/defer archive and reload are sequenced in a single command to avoid a
  race where the reload could read before archiving completed.
- **Frozen `PendingCount` on pipeline tick** — the `PipelineTickMsg` goroutine
  was capturing `m.proposals` at scheduling time rather than reloading from
  disk, so the header count never reflected post-action state. It now reloads
  inside the goroutine.
- **Silent `BlockSession` error in chat** — failure to blocklist a chat session
  was silently discarded, allowing the daemon to process it as a real work
  session and produce spurious proposals. The error is now surfaced as a status
  bar message and the view transition is rolled back.
- **Log viewer blocked main goroutine** — opening the log viewer triggered a
  synchronous `os.ReadFile` on the Bubble Tea event loop. The initial load is
  now dispatched as a background command; the viewer initialises with empty
  content and fills in when the read completes.
- **Data race on disk-usage cache** — `cachedDiskBytes` and `cachedDiskBytesTime`
  were read and written from background tick goroutines without mutex protection.
  A `sync.Mutex` now guards all accesses in `storeDiskBytes`.
- **Hardcoded daemon log path in chat** — `chatLog` constructed
  `~/.cabrero/daemon.log` manually instead of using `store.Root()`. Fixed to
  follow the canonical store path.
- **Session ID truncation inconsistency** — `cabrero proposals` was truncating
  session IDs to 10 characters; all other display sites use the 8-character
  `store.ShortSessionID`. Now consistent.

### Performance
- **Per-frame config.json read eliminated** — `renderModels()` in the pipeline
  monitor called `DefaultPipelineConfig()` (a disk read) on every render frame.
  The config is now cached in `pipeline.Model` and threaded from `appModel`
  via `PipelineDataRefreshed`.

### Refactored
- **Duplicate format helpers removed** — `RelativeTime` and `ShortenHome` were
  reimplemented in `internal/tui/shared`. Both functions now delegate to the
  canonical implementations in `internal/cli`, fixing a zero-time divergence
  bug where `shared.RelativeTime` would produce `"471580d ago"` instead of
  `"unknown"` for a zero `time.Time`.
- **`config.json` parsed once per call** — `ReadDebugFlag` and
  `ReadPipelineOverrides` each opened and parsed `~/.cabrero/config.json`
  independently. A shared `readConfig()` helper now parses the file once.

### Tests
- **Schema drift detection for classifier JSON tags** — added
  `TestClassifierSignalsLocalSchema` to verify that the store's local
  `classifierSignals` struct projection stays in sync with the pipeline
  package's `ClassifierOutput` JSON tags.

## [0.20.2] - 2026-02-25

### Fixed
- **Chat panel muting restored** — package-level style variables in the chat
  view captured `shared.ColorChat` / `shared.ColorMuted` at package init time,
  before `shared.InitStyles()` was called. Both colors were `nil` at that
  point, making every chat style colorless and removing all visual distinction
  between focused and unfocused states. Replaced with `shared.ChatAccentStyle`
  and `shared.MutedStyle`, which are properly initialized before any rendering
  occurs.
- **Proposal panel muted when chat has focus** — the proposal content viewport
  rendered at full color regardless of which pane held keyboard focus, giving
  no visual indication of which panel was active. The body content is now
  passed through `shared.MuteANSI` when `m.focus == FocusChat`, and the
  viewport is refreshed on every focus transition (Tab and programmatic
  `SetFocus`).

## [0.20.1] - 2026-02-25

### Fixed
- **Path traversal guard in apply** — `validateTarget` was checking for `..`
  after `filepath.Clean` had already resolved all traversal components, making
  the check permanently false. Replaced with a home-directory prefix guard so
  proposals can only write `.md` files inside the user's home directory.
- **Log viewer search bar overflow** — the `[N/M matches]` prefix was appended
  after `RenderStatusBar` had already clamped the output to terminal width,
  producing a string 12–16 chars wider than the terminal. Now routed through the
  existing `timedMsg` parameter so the width constraint applies to the full bar.
- **macOS notification with newlines** — `escapeAppleScript` did not escape `\n`
  or `\r`, causing `osascript` to silently fail when notification text contained
  a newline. Both control characters are now escaped.
- **Hook SESSION_ID path traversal** — hook scripts now reject SESSION_ID values
  containing `/` or `..` before using them to construct filesystem paths.

### Changed
- **Config reads reduced on pipeline monitor** — the pipeline config
  (`ClassifierTimeout`, `EvaluatorTimeout`) was re-read from `config.json` on
  every 5-second tick while the pipeline monitor was open. Now resolved once at
  startup and cached for the TUI session lifetime.
- **Status bar trimming is O(N)** — `RenderStatusBar` now pre-computes
  per-binding widths and uses a cumulative sum to find the cutoff index, avoiding
  the previous O(N²) re-join loop.
- **`SaveConfigTo` uses `store.AtomicWrite`** — was the only write site in the
  codebase that inlined the `CreateTemp + chmod + rename` pattern manually.
  Unified with the shared helper; also fixes file permissions (now explicit
  `0644` instead of umask-dependent).

## [0.20.0] - 2026-02-25

### Added
- **Scrollable help overlay** — the help overlay (`?` key) now wraps content
  in a viewport. When content exceeds the terminal height, Up/Down,
  HalfPageUp/Down, and GotoTop/GotoBottom keys scroll through it. Previously,
  content below the fold was silently clipped.
- **Dashboard filter** — `type:` and `target:` prefix filters plus free-text
  search via the standard bubbles/list component. Replaces the previous
  unfiltered proposal list.
- **Async dark/light background detection** — uses `tea.RequestBackgroundColor`
  and `BackgroundColorMsg` to detect terminal background color at runtime and
  reinitialize styles accordingly. Replaces the previous compile-time default.

### Changed
- **Charm v2 upgrade** — migrated to BubbleTea v2, Bubbles v2, and Lipgloss v2
  (`charm.land` import paths). Key types, view return types, and viewport APIs
  updated throughout.
- **Dashboard rewritten with bubbles/list** — replaces the custom list,
  viewport, and filter implementation with the standard bubbles/list component
  and a custom delegate.
- **Unified TUI utilities** — extracted shared helpers across views:
  `RenderSubHeader`, `RenderSectionHeader`, `FillToBottom`, `RenderBar`,
  `Checkmark`, `RelativeTime`, `RenderConfirmOverlay`. `RenderHeader` moved
  from dashboard to components package.
- **Consolidated TUI styles** — removed local style aliases in detail, fitness,
  and logview; all views use `shared.*Style` directly. Added `AccentBoldStyle`
  for section headers.
- Removed `ViewSourceDetail` ViewState — replaced by `sources.DetailOpen()`
  query method.
- Detail view status bar always rendered by root model; `HideStatusBar` flag
  removed.

### Fixed
- **Pipeline monitor overflow** — added viewport to pipeline monitor for
  overflow-safe scrolling when content exceeds terminal height.
- **Source list overflow** — added viewport to source list for the same reason.
- **Status message routing** — status messages now route to child views instead
  of a dead root handler.
- **Chat panel polish** — improved muting for unfocused chat and detail inline
  layout in narrow mode.

### Removed
- `compat` package — fully replaced by `shared.InitStyles` with
  `lipgloss.LightDark` adaptive colors.
- Unused spinner field in fitness model.

## [0.19.0] - 2026-02-24

### Added
- **Markdown rendering in AI chat** — assistant responses are now parsed as
  markdown via goldmark (pure Go) and styled with lipgloss. Headers render bold,
  bold text is bold, bullet and numbered lists use `•`/`1.` prefixes with proper
  indentation, fenced code blocks are indented and dimmed, and blockquotes use
  `│` prefix. Replaces the previous plain word-wrap rendering.
- **View sub-headers** — all TUI views now have a consistent sub-header area
  rendered between the persistent header and the content area. Proposal stats
  moved from the persistent header to the dashboard sub-header. Help overlay
  preserves sub-headers and renders multi-paragraph descriptions above key
  binding sections.
- **Configurable pipeline timeouts** — classifier and evaluator timeouts can
  now be set via `config.json` (`classifierTimeout`, `evaluatorTimeout`) using
  Go duration strings (e.g. `"3m"`, `"90s"`). Defaults raised from 2m/5m to
  3m/7m.

### Changed
- **Chat layout breakpoint raised to 160 columns** — the AI chat panel now uses
  a horizontal split (side-by-side) at ≥160 columns and a vertical split (chat
  underneath) below 160. Wide mode gives the chat 50% of the terminal width.
- **AI label inline with response** — assistant messages now start on the same
  line as the "AI:" label with hanging indent for continuation lines, matching
  the "You:" user message style.

### Fixed
- **Chat viewport scrolling during streaming** — the viewport no longer
  force-scrolls to the bottom on every spinner tick or stream token. Auto-scroll
  only triggers when the user was already at the bottom.
- **Narrow mode chat input visibility** — the chat input area is now always
  visible in narrow mode (vertical split) by using a bounded viewport height and
  auto-scrolling the detail body viewport.
- **Glamour removed** — the glamour markdown library caused repeated GC heap
  corruption crashes (`found bad pointer in Go heap`) due to unsafe pointer usage
  in its regexp2 and termenv dependencies. Replaced entirely with goldmark (pure
  Go parser) + lipgloss (pure Go styling), eliminating all unsafe dependencies
  from the rendering path.
- **User hooks firing in subprocess invocations** — all `claude` CLI subprocess
  invocations (pipeline, chat panel, apply) now pass
  `--settings '{"disableAllHooks": true}'` to prevent user-configured hooks from
  firing. Fixes macOS notification prompts and self-observation loops where
  cabrero's own capture hooks would fire during pipeline/chat/apply operations.
- **Nested Claude Code session errors** — subprocess invocations now strip
  `CLAUDECODE` from the environment, preventing "cannot be launched inside
  another Claude Code session" errors when cabrero runs inside a CC terminal.

### Removed
- `glamour` dependency and all transitive unsafe dependencies (regexp2, chroma,
  termenv style operations) from the chat rendering path.

## [0.18.0] - 2026-02-23

### Changed
- **Evaluator prompt v4** — the evaluator now instructs the LLM to use paragraph
  breaks in `change` and `rationale` fields. Proposals will render as structured
  paragraphs in the TUI instead of dense walls of text.

## [0.17.0] - 2026-02-23

### Added
- **Help overlay descriptions** — the help overlay (`?` key) now renders
  multi-paragraph descriptions above the key binding sections, explaining
  each view's purpose, what actions do, and how features connect. Dashboard
  help explains what proposals and fitness reports are, approval effects,
  and rollback availability.

### Fixed
- **Fitness help: Open key mislabeled** — was "Open linked source", actually
  toggles evidence group expand/collapse.
- **Detail help: chat toggle said "wide mode only"** — chat panel works at
  all terminal widths (horizontal split at >=120, vertical below).

## [0.16.1] - 2026-02-23

### Fixed
- **Source detail origin showed group label** — the Origin field in the source
  detail Info section displayed the group label (e.g. "Unclassified") instead of
  the source's actual origin (e.g. "User-level", "Plugin: foo"). Now derives the
  display label from the source's own Origin field.

## [0.16.0] - 2026-02-23

### Added
- **Cross-navigation between top-level views** — Sources and Pipeline now have
  `s`/`p` shortcuts to jump directly to each other. Uses a `SwitchView` message
  that replaces the current view without growing the navigation stack, so Esc
  always returns to Proposals. Help overlays and status bars updated with the
  new bindings.
- **Source detail Info section** — drilling into a source now shows a full Info
  section (origin, ownership, approach, session count, health score, classification
  date) above the Recent Changes list. Sub-header simplified to just the source name.
- **Bare `cabrero` launches the dashboard** — running `cabrero` without arguments
  now opens the dashboard TUI (same as `cabrero dashboard`).

### Changed
- Renamed `review` CLI command to `dashboard`.
- Simplified persistent header from "Cabrero Review" to "Cabrero".
- All views now have a consistent sub-header with title and contextual stats.
- Help overlay preserves the sub-header, only replacing the content area.
- Source manager shows "unknown" for unset ownership and "not set" for unset
  approach instead of `⚠` and `-`.

### Removed
- Proposal stats from the persistent header (moved to dashboard sub-header).

## [0.15.0] - 2026-02-23

### Added

- **Token usage stats in Pipeline Activity** — the pipeline monitor now shows
  aggregate LLM token consumption (input/output) and cost in the activity section.
  Data flows from `HistoryRecord` through `PipelineRun` and `PipelineStats` to
  the TUI. Tokens formatted as compact values (e.g. "12.3K in / 4.5K out"),
  cost as USD (e.g. "$0.35"). Displayed in both narrow and standard/wide layouts.
- **Concurrent invocation limiter** — caps the number of simultaneous `claude`
  CLI processes via a channel-based semaphore in `invokeClaude()`. Default
  limit: 3 (configurable via `PipelineConfig.MaxConcurrentInvocations`; 0 =
  unlimited). CLI commands block-wait for a slot with a progress message;
  the daemon uses non-blocking try-acquire and skips busy sessions (retried
  next poll cycle). Timeout timer starts after semaphore acquire so queuing
  time doesn't eat into execution budget.
- **JSON parse retry** — Classifier and Evaluator re-invoke on malformed JSON
  output (markdown fences, prose preamble) up to `MaxLLMRetries` times
  (default 1). Non-JSON errors are not retried.
- **Search auto-expands all entries and jumps to latest match** — searching in
  the log viewer now expands all multi-line entries and moves the cursor to the
  last match for immediate context.
- **Help overlay view title and description** — the help overlay (`?` key) now
  shows a "[View Name] Help" title and a brief description of the view's purpose
  above the key binding sections, providing context for each view.

### Changed

- **Incremental log entry parsing** — `AppendContent` now parses only the
  new bytes and merges them into existing entries instead of re-parsing all
  content from scratch on every 1-second poll tick. The redundant `m.content`
  field is removed from the log viewer model.
- **Daemon log max size reduced** — log rotation threshold lowered from 5 MB
  to 2 MB per file, reducing the data the TUI log viewer reads and parses on
  each follow-mode poll.

### Fixed

- **Pipeline monitor Recent Runs column alignment** — timing columns (parse,
  cls, eval) now use fixed-width formatting so values align vertically regardless
  of which stages completed. Error indicators ("✗ cls failed", "✗ eval failed")
  stay in their column. Project name column widened from 20 to 25 chars in wide
  mode to prevent truncation of longer names.
- **Parse timing preserved on errored runs** — `classify()` now returns a
  partial `ClassifierResult` carrying `ParseDuration` on all error paths,
  so the TUI shows parse timing even when the classifier or pre-parse fails.
- **"eval skipped" shown for clean-triaged runs** — processed sessions that
  passed the classifier but skipped evaluation (clean triage) now display
  "eval skipped" in muted style instead of a blank eval column.
- **Queued sessions without transcript** — `ScanQueued` now skips sessions
  missing a `transcript.jsonl` file, preventing repeated pipeline failures
  from incomplete captures.
- **Log viewer blank lines between entries** — removed spurious blank lines
  and scroll to latest entry on open.
- **Log viewer scroll after expand/collapse** — viewport now scrolls to keep
  the cursor visible after toggling entry expansion.
- **Log viewer collapse on search clear** — all entries collapse when clearing
  a log search via Esc.
- **Log viewer polling** — always polls for new content while the viewer is
  open, regardless of follow mode state.

## [0.14.0] - 2026-02-23

### Added

- **Chat panel toggle and vertical split** — pressing `c` toggles the AI chat
  panel at any terminal width. Wide terminals (≥ 120 cols) use a horizontal
  split (side-by-side), narrow terminals use a vertical split (chat underneath).
  Tab switches focus between proposal and chat panes when chat is open.
- **Structured log viewer** — log entries are now parsed from daemon format into
  colored, structured entries with level badges (INFO=purple, ERROR=red), muted
  timestamps, cursor-based entry navigation, and blank-line separators.
- **Expand/collapse for multi-line log entries** — Enter toggles the current
  entry, `e` expands all, `E` collapses all. Stack traces and continuation lines
  are hidden by default with a `[+N]` indicator.
- **Search auto-expands matching entries** — searching for text in collapsed
  continuation lines auto-expands those entries. `n`/`N` moves cursor to
  matching entries.
- **Log viewer snapshot** — `log-viewer` added to the snapshot command for
  visual regression testing.
- **Source registry** — auto-discover skills and CLAUDE.md files from classifier
  outputs at TUI startup, persist ownership/approach classifications across
  sessions in `sources.json`, and group sources by origin (user, project, plugin)
  in the Sources tab.
- **Persistent header across all TUI views** — the dashboard header (title,
  version, proposal counts, daemon status, hooks) now renders above every view
  instead of only the dashboard. Navigating to proposal detail, source manager,
  pipeline monitor, fitness report, log viewer, or help overlay retains the
  header context. Snapshots also include the header for all views.
- **Context-aware help overlay** — pressing `?` now shows only the key bindings
  relevant to the current view, grouped into sections (Navigation, Actions,
  Views, Global) with full descriptions explaining what each key does. Replaces
  the generic help overlay that dumped all bindings from every view.

### Changed

- **Log viewer uses structured entries** — replaces raw text display with parsed
  entries. Search navigation (`n`/`N`) now moves cursor to matching entry.
  Help overlay updated with entry navigation and expand/collapse bindings.
- **Log viewer key binding** — changed from uppercase `L` to lowercase `l` for
  consistency with other action keys.
- **Session ID standardization** — consolidated all session ID shortening into
  a single `store.ShortSessionID()` function returning the first 8 characters
  (first UUID segment). Proposal IDs also use 8-char prefixes (was 6).

### Fixed

- **Proposal detail text wrapping** — prose change descriptions and rationale
  now word-wrap to the viewport width instead of being clipped at the terminal
  edge. Content width correctly accounts for the chat panel split.
- **Proposal detail shows full session ID** — the header now displays the
  complete session UUID instead of a truncated 12-character version.
- **Status bar no longer wraps at narrow widths** — `RenderStatusBar` now
  measures visual width and drops trailing bindings to guarantee a single-line
  status bar at any terminal width.
- **Detail view respects terminal height** — replaced the unbounded
  strings.Builder body with a single scrollable viewport (matching the fitness
  view pattern). Diff, rationale, citations, and apply-state overlays all render
  inside the viewport, capping output to exactly the declared height.
- **Chat panel respects terminal height** — fixed viewport sizing to account
  for bordered chip chrome and added fill-to-height logic.
- **Pipeline Monitor columns align vertically** — session ID, age, project,
  and timing columns in the Recent Runs list now use fixed-width formatting
  so values line up across rows. Run detail labels (Session, Project, Status,
  etc.) also align consistently.
- **Parse duration no longer shows 0.0s** — the parse stage was never timed
  separately; its duration was lumped into the classifier. Parse and classifier
  are now recorded as independent durations in run history.
- **Project name appears in Pipeline Monitor** — capture hooks wrote `work_dir`
  but not `project` to session metadata, leaving the project column blank.
  Hooks now derive the project slug from the working directory. Existing
  sessions are backfilled from `work_dir` on read.

## [0.13.0] - 2026-02-22

### Added

- `cabrero prompts` — list prompt files with name, version, last-modified time,
  and path to the prompt file on disk.
- `cabrero replay` — re-run pipeline on past sessions with alternate prompt files.
  Supports `--stage` (classifier/evaluator, inferred from filename), `--compare`
  for diffing against original decisions, `--calibration` for batch regression
  testing against the calibration set, and model/timeout overrides.
- `cabrero calibrate` — manage calibration set for prompt regression testing.
  Subcommands: `tag` (with `--label approve|reject` and optional `--note`),
  `untag`, and `list`.
- `~/.cabrero/replays/` directory for replay output persistence (meta.json,
  classifier.json, evaluator.json per replay).
- `~/.cabrero/calibration.json` for calibration set storage.
- `RunClassifierWithPrompt` / `RunEvaluatorWithPrompt` pipeline helpers for
  prompt override without code duplication.

## [0.12.1] - 2026-02-22

### Added

- **Configurable pipeline models** — classifier and evaluator models are now
  configurable via CLI flags (`--classifier-model`, `--evaluator-model`) on `run`,
  `backfill`, and `daemon` commands, and via `config.json` (`classifierModel`,
  `evaluatorModel`). Resolution: CLI flag → config.json → compile-time default.
- `cabrero status` shows active pipeline models and prompt versions.
- `cabrero doctor` reports active pipeline models under Pipeline diagnostics.
- `cabrero run` prints active model names before pipeline execution.
- TUI pipeline monitor shows a MODELS section with override detection.

### Changed

- `cabrero backfill` preview derives model names from configuration instead of
  hardcoded prose.

## [0.12.0] - 2026-02-22

### Added

- **Token usage tracking in pipeline history** — each run history record now captures
  per-stage token consumption, cost, and CC session IDs via `--output-format json`
  from the `claude` CLI. New `InvocationUsage` struct records input/output tokens,
  cache creation/read tokens, cost, turn count, and web search/fetch request counts.
  Per-stage fields (`classifier_usage`, `evaluator_usage`) and totals
  (`total_cost_usd`, `total_input_tokens`, `total_output_tokens`) are added to
  `HistoryRecord`. Batch evaluator usage is split equally among sessions in the chunk.
  Usage is captured even on CC-level errors for partial observability.

## [0.11.0] - 2026-02-22

### Added

- **Pipeline run history** — append-only JSONL log (`~/.cabrero/run_history.jsonl`)
  captures full diagnostic context for every pipeline run: actual wall-clock timing
  per stage, invocation source (daemon/CLI/backfill), batch context, models and
  prompt versions used, config snapshot, and error details. Replaces unreliable
  mtime-based timing estimates in the TUI. Records rotated after 90 days on daemon
  startup. Includes `ComputeStatsFromHistory` for aggregate analysis (median/p95
  latency, evaluator skip rate, retry rate, source breakdown).

- **Debug mode indicator** — `cabrero status`, the dashboard header, and the
  pipeline monitor DAEMON section now show "Debug: enabled" in warning color
  when pipeline debug mode is active. Hidden when off.

- **Scoped filesystem access for pipeline invocations** — Classifier and evaluator
  CC sessions now use `--permission-mode dontAsk`, `--setting-sources ""`, and
  path-scoped `--allowedTools` to restrict filesystem access. The classifier can
  only read `~/.cabrero/`; the evaluator can read `~/.cabrero/`, the session's
  project directory, and `~/.claude/`. Prevents CC from loading user plugins and
  eliminates remaining macOS TCC prompt triggers.

### Fixed

- **macOS network volume TCC prompts** — `claude` child processes inherited
  `cwd=/` from the launchd daemon, causing CC's startup project discovery to
  probe paths reaching the Google Drive FileProvider. Now sets `cmd.Dir` to
  `~/.cabrero/` for a safe, local-only starting directory.

- **False queue-drain notification** — daemon sent "Queue processing complete"
  even when new sessions arrived during processing. Now re-scans after the
  batch and only notifies when the queue is truly empty.

- **Apply workflow hardening** — `apply.Blend` sets `cmd.Dir` to prevent macOS
  TCC prompts; `apply.Commit` validates proposal targets (rejects non-`.md` and
  path traversal) and uses atomic writes; `truncateForLog` uses rune slicing for
  UTF-8 correctness; import batch-reads blocklist once; hook scripts escape
  backslashes/quotes in `SESSION_CWD`.

## [0.10.0] - 2026-02-22

### Added

- **Working directory in session metadata** — hook scripts now extract the `cwd`
  field from Claude Code's JSON payload and store it as `work_dir` in
  `metadata.json`. This records the actual filesystem path the session ran in,
  providing more reliable project identification than the CC project slug alone.
- **Pipeline debug mode** — `--debug` flag on `cabrero run` and `cabrero daemon`
  persists full CC session transcripts for classifier/evaluator invocations.
  Pre-assigns a session ID via `--session-id`, immediately blocklists it to
  prevent stale recovery, and logs CLI args and session ID for cross-reference.
  Also togglable at runtime via `{"debug": true}` in `~/.cabrero/config.json`
  (re-read each poll cycle, no daemon restart needed).

## [0.9.3] - 2026-02-21

### Fixed

- **Daemon can't find claude CLI** — LaunchAgent PATH is now built dynamically
  during setup, including the directory where the `claude` binary is installed
  (e.g., `~/.local/bin`). Previously hardcoded to system paths only, causing all
  pipeline runs to fail with "executable file not found in $PATH".
- **macOS network volume access prompts** — session scanning (import and stale
  recovery) replaced unbounded `filepath.Walk` with targeted `os.ReadDir` at the
  two known levels where JSONL files exist. No longer descends into
  `tool-results/` or other subdirectories that triggered macOS file provider
  dialogs.

## [0.9.2] - 2026-02-21

### Added

- **CLI color output** — all CLI commands (setup, doctor, uninstall, status) now
  use colored output via a shared `internal/cli` package with lipgloss adaptive
  colors. Green checkmarks, orange warnings, red errors, purple accents, bold
  labels, and muted metadata for a consistent visual vocabulary.
- **`uninstall --dry-run`** — preview what uninstall would remove without
  touching anything. Each step shows "Would ..." messages with purple arrows.
- **`make install` symlink** — install target now symlinks the binary to
  `/usr/local/bin/cabrero` for immediate PATH access without shell config
  changes.

### Fixed

- **Binary killed on macOS** — `make install` now re-signs the binary with
  `codesign -s -` after copying to `~/.cabrero/bin/`, preventing macOS
  AppleSystemPolicy from killing the binary due to an invalidated ad-hoc
  code signature.
- **PATH check false negative** — setup and doctor PATH checks now use
  `exec.LookPath` instead of scanning `$PATH` entries, correctly detecting
  the binary when reachable via symlink (e.g., `/usr/local/bin/cabrero`).
- **Tilde in export hint** — PATH suggestions now use `$HOME` instead of
  literal `~` which doesn't expand inside quoted strings.

### Changed

- From-source install instructions in README expanded with PATH/symlink
  guidance.

## [0.9.1] - 2026-02-21

### Changed

- **Runner struct** — unified `Run()` and `BatchProcessor` into a single `Runner`
  struct with `RunOne()` and `RunGroup()` methods. Consistent status marking via
  `store.MarkProcessed`/`MarkError`, context cancellation on all paths, and full
  testability via hook fields.

### Removed

- `Run()`, `RunThroughClassifier()`, and `BatchProcessor` — replaced by `Runner`.

## [0.9.0] - 2026-02-21

### Added

- **Pipeline Logger interface** — `pipeline.Logger` with `Info`/`Error` methods,
  injectable via `PipelineConfig.Logger`. Defaults to `stdLogger` (stdout/stderr)
  for backwards compatibility. Includes `discardLogger` for silent operation.
- **Daemon pipeline log routing** — daemon now wires a `daemonPipelineLogger`
  adapter so pipeline progress and warnings appear in `daemon.log` with
  timestamps and `[INFO]`/`[ERROR]` prefixes instead of leaking to
  stdout/stderr.

### Fixed

- **Pipeline stdout/stderr leaks** — all 25 direct `fmt.Printf`/`Println`/
  `Fprintf(os.Stderr)` calls in the pipeline package now route through the
  Logger interface. Fixes output corruption when running under the daemon or
  future TUI integration.
- **Proposal ID mismatch** — batch evaluator `shortID` now returns 6 chars to
  match the evaluator prompt format. Added zero-match guard to detect silent
  proposal drops during partitioning.
- **Non-atomic store writes** — metadata, blocklist, proposals, evaluations, and
  archive writes now use temp file + rename via `store.AtomicWrite` to prevent
  partial writes on crash.
- **Path traversal in proposals** — `ReadProposal` and `WriteProposal` now
  validate proposal IDs against path separators and directory traversal.
- **UTF-8 truncation** — `Truncate`, `TruncatePad`, `TruncateID`, and `PadRight`
  now operate on runes instead of bytes, preventing mid-character splits.
- **Blocklist I/O in loops** — `ScanQueued` and `ScanStale` now pre-load the
  blocklist once instead of reading from disk per session.

### Changed

- `ScanStale` uses `filepath.WalkDir` instead of `filepath.Walk` for lower
  allocation overhead.
- `reject` command uses `flag.NewFlagSet` instead of manual flag parsing.
- Session status strings replaced with typed constants (`store.StatusQueued`,
  `store.StatusImported`, `store.StatusProcessed`, `store.StatusError`).
- `ScanQueued` uses `slices.Reverse` instead of manual loop.

### Removed

- Dead functions: `GetAgentTranscript`, `GetSessionRange`, `ListPipelineRuns`,
  `GatherPipelineStats`, `ListProjects`.

## [0.8.2] - 2026-02-21

### Fixed

- **Text wrapping** — fitness report verdict and proposal detail rationale now
  word-wrap instead of truncating at the terminal edge. New `WrapIndent` helper
  in `shared/format.go`.
- **Dashboard empty state** — status bar no longer shows approve/reject/defer
  keys when there are no items to act on.
- **Fitness header overflow** — ownership/origin line splits onto two lines at
  widths below 100 to prevent horizontal overflow.
- **Dashboard narrow header** — daemon status and hooks now stack onto separate
  lines in standard/narrow layout instead of overflowing.
- **Evidence pluralization** — "(1 entries)" corrected to "(1 entry)".
- **Pipeline status bar anchoring** — status bar now stays pinned to the bottom
  of the terminal instead of floating after the last section.
- **Tab toggle on hidden chat** — Tab key in detail view no longer toggles focus
  to an invisible chat panel at narrow widths; messages are no longer forwarded
  to the chat model when the panel is hidden.
- **Detail status bar** — "tab next pane" hint hidden when chat panel is not
  visible.
- **Pipeline cursor preservation** — auto-refresh no longer resets cursor and
  expansion state every 5 seconds.
- **Snapshot CLI flag parsing** — flags now work after the view name
  (e.g., `snapshot dashboard -w 80`).

## [0.8.1] - 2026-02-21

### Added

- **TUI snapshot pipeline** — `cmd/snapshot` renders all TUI views to ANSI
  stdout. Pipe through [freeze](https://github.com/charmbracelet/freeze) via
  `make snapshots` (all views) or `make snapshot VIEW=<name>` (single view)
  to produce SVG and PNG files with catppuccin-mocha styling. PNGs capped at
  1200px width.

## [0.8.0] - 2026-02-21

### Added

- **`cabrero dashboard`** (originally `cabrero review`) — interactive Bubble Tea TUI for reviewing proposals.
  Dashboard shows proposal list with type indicators, confidence, and sort/filter.
  Detail view renders colored unified diffs, rationale, and citation chains.
  Stack-based navigation with configurable arrow or vim keybinding modes.
- **AI chat panel** — streaming Evaluator integration via `claude` CLI for
  interrogating proposals before deciding. Question chips for common queries,
  revision detection via ` ```revision ` fenced blocks.
- **`cabrero approve`** — non-interactive CLI command that reads a proposal,
  invokes Claude to blend the change into the target file, shows a before/after
  diff, and writes the file on confirmation.
- **`cabrero reject`** — non-interactive CLI command that archives a proposal
  with an optional rejection reason (`--reason "text"`).
- **Fitness Report Detail view** — assessment bars showing three-bucket health
  breakdown (followed/worked around/confused), expandable session evidence grouped
  by category, dismiss and jump-to-sources actions. Fitness reports appear in the
  dashboard with `◎` indicator alongside proposals.
- **Source Manager** — grouped source list organized by origin (user, project,
  plugin) with collapsible sections. Ownership classification, iterate/evaluate
  approach toggles with confirmation gates, change history detail with rollback
  support. Adaptive column layout for different terminal widths.
- **Dashboard mixed item list** — unified list showing both proposals and fitness
  reports. `s` keyboard shortcut opens Source Manager from dashboard.
- **TUI configuration** — `~/.cabrero/config.json` with navigation mode,
  theme, dashboard sort order, chat panel width, personality flavor text,
  and per-action confirmation toggles. Partial configs merge with defaults.
- **Pipeline monitor view** — daemon health, recent runs with per-stage timing
  breakdown (parse/classifier/evaluator), sparkline activity chart, prompt
  version listing, inline run detail expansion, and retry flow with
  configurable confirmation gate.
- **Pipeline monitor daemon header** — uptime, poll/stale/delay intervals, store
  metrics (path, session count, disk usage), and two-column layout at width >= 120.
- **Log viewer view** — full-screen scrollable daemon log with incremental
  search, match navigation (n/N), follow mode toggle, and auto-refresh via
  polling.
- **Log viewer search highlighting** — search matches highlighted with accent
  color in the viewport using termenv styling.
- **Log viewer two-stage Esc** — first Esc clears active search matches and
  highlighting, second Esc navigates back to the pipeline monitor.
- **PipelineRun data layer** — reconstructs pipeline run history from store
  artifacts (session metadata, classifier/evaluator output files, timestamps)
  without requiring a dedicated database.
- **Sparkline component** — Unicode block-character sparkline renderer for
  visualizing sessions-per-day activity in the pipeline monitor.
- **Polling auto-refresh** — pipeline monitor refreshes every 5 seconds, log
  viewer refreshes every 1 second when follow mode is active.
- **PipelineConfig settings** — configurable sparkline days, recent runs limit,
  and log follow mode default via `shared.Config`.
- **Pipeline monitor responsive layout** — three-tier layout (wide/standard/narrow)
  adapts daemon header density, activity stats format, sparkline visibility,
  run row detail level, and prompt section visibility to terminal width.

### Fixed

- Chat panel now streams tokens in real-time instead of buffering.
- Chat panel renders alongside detail view in wide terminals.
- Dashboard approve/reject/defer navigates to detail view first.
- Reject and defer actions respect per-action confirmation config toggles.
- Personality flavor text from config is now honored in TUI messages.
- Review flow subprocess isolation hardened to prevent Bubble Tea contract
  violations.
- Fire-and-forget archive goroutines replaced with proper `tea.Cmd` to prevent
  races.
- Config saves no longer strip the `pipeline` section from `config.json`.
- Sparkline bucketing normalizes timestamps to local timezone for correct day
  boundaries.

### Changed

- Log viewer uses incremental reading in follow mode for lower I/O overhead.
- Pipeline monitor deduplicates redundant store reads per tick.
- Consolidated duplicated TUI utilities, styles, and dead code.

## [0.7.0] - 2026-02-20

### Added

- `cabrero backfill` command to process existing sessions through the full
  pipeline with `--since`, `--until`, `--project`, `--retry-errors` filtering,
  preview with confirmation, and smart batching via `pipeline.BatchProcessor`.
- Setup wizard Step 8: offers to import and enqueue existing CC sessions after
  installation (imports in quiet mode, counts imported, offers to enqueue for
  background processing with configurable lookback — default 1 month, skippable).
- `store.QuerySessions` for filtered session queries by date range, project
  substring, and status. Returns oldest-first.
- `pipeline.BatchProcessor` as shared smart batching infrastructure with
  configurable max batch size (default 10) and progress callbacks. Used by
  both daemon and backfill command.
- `backfill --enqueue` flag to mark sessions as queued for background daemon
  processing instead of running the pipeline synchronously.
- `store.MarkQueued()` function for transitioning sessions to queued status.
- Daemon notification when queue processing completes.

### Changed

- Renamed session status `pending` to `queued` for hook-captured sessions.
- Imported sessions now use `imported` status (not automatically processed
  by daemon).
- Daemon scanner simplified: filters on `queued` status only, no longer
  checks capture trigger.
- Setup wizard backfill step now uses `--enqueue` for non-blocking processing.
- Doctor check updated: warns about sessions stuck in `queued` status >24h.
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

[0.21.0]: https://github.com/vladolaru/cabrero/compare/v0.20.3...v0.21.0
[0.20.3]: https://github.com/vladolaru/cabrero/compare/v0.20.2...v0.20.3
[0.20.2]: https://github.com/vladolaru/cabrero/compare/v0.20.1...v0.20.2
[0.20.1]: https://github.com/vladolaru/cabrero/compare/v0.20.0...v0.20.1
[0.20.0]: https://github.com/vladolaru/cabrero/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/vladolaru/cabrero/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/vladolaru/cabrero/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/vladolaru/cabrero/compare/v0.16.1...v0.17.0
[0.16.1]: https://github.com/vladolaru/cabrero/compare/v0.16.0...v0.16.1
[0.16.0]: https://github.com/vladolaru/cabrero/compare/v0.15.0...v0.16.0
[0.15.0]: https://github.com/vladolaru/cabrero/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/vladolaru/cabrero/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/vladolaru/cabrero/compare/v0.12.1...v0.13.0
[0.12.1]: https://github.com/vladolaru/cabrero/compare/v0.12.0...v0.12.1
[0.12.0]: https://github.com/vladolaru/cabrero/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/vladolaru/cabrero/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/vladolaru/cabrero/compare/v0.9.3...v0.10.0
[0.9.3]: https://github.com/vladolaru/cabrero/releases/tag/v0.9.3
[0.9.2]: https://github.com/vladolaru/cabrero/releases/tag/v0.9.2
[0.9.1]: https://github.com/vladolaru/cabrero/releases/tag/v0.9.1
[0.9.0]: https://github.com/vladolaru/cabrero/releases/tag/v0.9.0
[0.8.2]: https://github.com/vladolaru/cabrero/releases/tag/v0.8.2
[0.8.1]: https://github.com/vladolaru/cabrero/releases/tag/v0.8.1
[0.8.0]: https://github.com/vladolaru/cabrero/releases/tag/v0.8.0
[0.7.0]: https://github.com/vladolaru/cabrero/releases/tag/v0.7.0
[0.6.0]: https://github.com/vladolaru/cabrero/releases/tag/v0.6.0
[0.5.0]: https://github.com/vladolaru/cabrero/releases/tag/v0.5.0
