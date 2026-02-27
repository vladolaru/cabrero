# Subprocess Isolation Model

How cabrero isolates `claude` CLI subprocesses from the user's environment.

Cabrero spawns `claude` CLI subprocesses for the pipeline (classifier, evaluator,
curator, meta-analysis), the TUI chat panel, and the blend/apply workflow. Each
subprocess must be isolated from user settings, hooks, plugins, and MCP servers to
prevent interference with pipeline behavior.

## CLAUDECODE Environment Variable

### What it is

Claude Code sets `CLAUDECODE=1` in the environment of **every child process it spawns**:

- **Shell snapshots** — when CC captures the user's shell environment at startup
- **Bash tool commands** — every `bash` command CC runs inherits `CLAUDECODE=1`
- **Teammate agents** — CC's agent spawning also sets `CLAUDECODE=1`

This means any process launched from within a CC terminal session (including
cabrero's TUI) inherits `CLAUDECODE=1` in its environment.

### The nesting guard

At startup, CC checks for this env var and refuses to launch if detected:

```js
// Decompiled from CC v2.1.62 binary
if (process.env.CLAUDECODE === "1"
    && !args.some((a) => a.startsWith("--team-name"))
    && !isAllowedSubcommand(args))
  console.error("Error: Claude Code cannot be launched inside another Claude Code session.");
  process.exit(1);
```

Three conditions must ALL be true for the block:
1. `CLAUDECODE === "1"` — process is inside a CC session
2. NOT a `--team-name` invocation (teammates are allowed to nest)
3. NOT a safe subcommand (see below)

Safe subcommands that bypass the nesting check:
`plugin`, `mcp`, `auth`, `doctor`, `update`, `install`, `rollback`, `log`, `completion`,
and any invocation with `--help` or `-h`.

Notably, `-p` (print/prompt mode) and `--print` are **NOT** safe subcommands.
This means cabrero's pipeline and chat invocations would be blocked without
stripping the env var.

### No IPC, no session corruption

`CLAUDECODE` is a simple sentinel value (`"1"`). There is no IPC socket, no shared
memory, and no mechanism by which a child process could interfere with the parent's
running session through this env var. The check is a hard block at the CLI entry
point: either it exits immediately (`process.exit(1)`), or it proceeds normally with
a completely fresh session.

### Why cabrero strips it

Cabrero's `cleanClaudeEnv()` removes `CLAUDECODE` from the environment before
spawning any `claude` subprocess. Without this, every pipeline invocation launched
from within a CC terminal (via the TUI or the daemon inheriting the env) would
hit the nesting guard and exit silently.

This is safe because:
- The subprocess runs with its own `--session-id` (blocklisted to prevent self-observation)
- There is no IPC channel that stripping the env var would disconnect
- The subprocess is fully isolated via other flags (see below)

## Common Isolation Layer

Applied to **all** subprocess invocations (pipeline, chat, blend):

| Mechanism | What it does | Why |
|-----------|-------------|-----|
| `CLAUDECODE=` stripped | Removes CC nesting guard | CC's Bash tool sets `CLAUDECODE=1` on all children. Without stripping, `claude -p` calls hit the nesting check and `process.exit(1)`. No corruption risk — purely a launch guard. |
| `CABRERO_SESSION=1` added | Marks subprocess as cabrero-spawned | Cabrero's own hooks detect this and skip, preventing self-observation loops. |
| `cmd.Dir = os.TempDir()` | Sets CWD to `/tmp` | Prevents CC project discovery from walking up to `~` and scanning ~/Desktop, ~/Music etc., triggering macOS TCC prompts. |
| `--system-prompt <text>` | Replaces entire system prompt | Pipeline/chat gets cabrero's prompt. Plugin instructions never inject. |
| `--disable-slash-commands` | Disables all skills | No user skills fire inside subprocesses. |
| `--mcp-config '{"mcpServers":{}}'` | Empty MCP server list | Must include `mcpServers` key — bare `{}` silently crashes CLI 2.1.59+. |
| `--strict-mcp-config` | Only servers from `--mcp-config` | Ignores plugins, user settings, project `.mcp.json`. Combined with empty config = zero MCP servers. |
| `--settings '{"disableAllHooks": true, "alwaysThinkingEnabled": false, "enabledPlugins": {}}'` | Override user settings | Suppresses hooks, prevents extended thinking (cost/latency), attempts to disable plugins. |

## Per-Mode Flags

### Agentic mode (classifier, evaluator, curator, meta)

| Flag | Effect |
|------|--------|
| `-p <prompt>` | User prompt as positional arg |
| `--output-format json` | Structured JSON with usage data |
| `--session-id <uuid>` | Pre-generated, blocklisted to prevent self-observation |
| `--allowedTools <scoped>` | Path-scoped `Read` + `Grep` (e.g. `Read(//~/.cabrero/**),Grep(//~/.cabrero/**)`) |
| `--permission-mode dontAsk` | Auto-denies unapproved tools |
| `--effort high` | Evaluator only — deeper reasoning |

### Print mode (curator check, blend)

| Flag | Effect |
|------|--------|
| `--print` | Stdin→stdout, no agentic loop |
| `--tools ""` | Disables ALL tools — pure text-in/text-out |
| `--no-session-persistence` | No transcript saved |

### Chat streaming (TUI chat panel)

| Flag | Effect |
|------|--------|
| `--print --verbose --output-format stream-json` | Streams JSON events for real-time token display |
| `--session-id` / `--resume` | Persistent CC session for multi-turn chat |
| `--permission-mode dontAsk` | No prompts |
| `--allowedTools <scoped>` | Read + Grep scoped to proposal's raw transcript dir |

## What Still Leaks Through

User settings files (`~/.claude/settings.json`, `.claude/settings.json`,
`.claude/settings.local.json`) are still loaded. Our `--settings` overrides sit at
precedence #2 (CLI args), which beats user settings at #5:

| Setting | Override works? | Reason |
|---------|----------------|--------|
| `disableAllHooks: true` | Yes | Boolean, clean override |
| `alwaysThinkingEnabled: false` | Yes | Boolean, clean override |
| `enabledPlugins: {}` | Uncertain | Object merge semantics unclear (shallow vs deep) |
| `model` | N/A | Overridden by `--model` flag |
| Other (theme, cleanupPeriodDays, etc.) | Leak through | Non-critical, no behavioral impact |

The `enabledPlugins: {}` uncertainty is mitigated three ways:
1. `--system-prompt` replaces the entire prompt — plugin instructions can't inject
2. `--disable-slash-commands` blocks skills
3. `--strict-mcp-config` blocks MCP servers from plugins

## Invocation Sites

| Site | Mode | File | Tools |
|------|------|------|-------|
| Classifier | Agentic | `pipeline/invoke.go` | Read + Grep scoped to `~/.cabrero/` |
| Evaluator | Agentic | `pipeline/invoke.go` | Read + Grep scoped to `~/.cabrero/` + project dir + `~/.claude/` |
| Evaluator batch | Agentic | `pipeline/invoke.go` | Read + Grep scoped to union of session cwds |
| Curator group | Agentic | `pipeline/invoke.go` | Read + Grep scoped to target file dir + `~/.cabrero/` |
| Meta-analysis | Agentic | `pipeline/invoke.go` | Read + Grep scoped to `~/` |
| Curator check | Print | `pipeline/invoke.go` | None (`--tools ""`) |
| Chat panel | Streaming | `tui/chat/stream.go` | Read + Grep scoped to raw transcript dir |
| Blend | Print | `apply/apply.go` | None (`--tools ""`) |

## Why Not CLAUDE_CODE_SIMPLE=1

`CLAUDE_CODE_SIMPLE=1` disables MCP, hooks, CLAUDE.md, and restricts tools to
`Bash`, `Read`, `Edit`. The pipeline's `--allowedTools` references the dedicated
`Grep` tool by name — with `CLAUDE_CODE_SIMPLE`, that tool wouldn't exist.
It's also too coarse: removes CLAUDE.md loading entirely. The surgical approach
with individual flags keeps the full tool set while disabling only problematic settings.

## Related Documentation

- `docs/claude-cli-settings-and-hooks.md` — settings hierarchy and flag reference
- Source analysis based on CC v2.1.62 binary (Bun-compiled, strings extraction)
