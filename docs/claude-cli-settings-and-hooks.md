# Claude CLI: Settings and Hooks Control

Reference for controlling settings loading and hook execution when invoking the `claude` CLI programmatically.

## CLI Flags

### `--setting-sources <sources>`

Controls which `settings.json` files are loaded. Accepts a comma-separated list of:

| Source    | File loaded                          |
|-----------|--------------------------------------|
| `user`    | `~/.claude/settings.json`            |
| `project` | `.claude/settings.json` (in project) |
| `local`   | `.claude/settings.local.json`        |

These are the only documented values. The default (when flag is omitted) loads all three.

**Note:** Managed settings (`/Library/Application Support/ClaudeCode/managed-settings.json`
on macOS) always load regardless of this flag and take highest precedence.

**Undocumented:** Passing `""` (empty string) was assumed to skip all three files based on
common CLI patterns, but this behavior is **not officially documented**. It broke in CLI
2.1.59+ (Feb 2026), causing the process to exit with empty stdout. The pipeline no longer
uses this — `--settings '{"disableAllHooks": true}'` is the sole hook suppression mechanism.

### `--settings <file-or-json>`

Adds settings on top of whatever is loaded from setting sources. Accepts either a file path or inline JSON:

```bash
claude --settings ./custom-settings.json -p "query"
claude --settings '{"disableAllHooks": true}' -p "query"
```

This flag is **additive** — it does not replace settings loaded from sources.

### `disableAllHooks` setting

A dedicated setting that disables all hooks and any custom status line:

```json
{
  "disableAllHooks": true
}
```

This can be set in any settings file scope or passed via `--settings`.

## Settings Hierarchy (Precedence Order)

1. **Managed** (highest) — system-level IT admin settings, cannot be overridden
2. **Command-line arguments** — session-specific overrides (`--settings`, `--tools`, etc.)
3. **Local** (`.claude/settings.local.json`) — machine-specific, gitignored
4. **Project** (`.claude/settings.json`) — shared with team via git
5. **User** (`~/.claude/settings.json`) — personal defaults
6. **Defaults** (lowest) — built-in Claude Code defaults

## Disabling Hooks for Programmatic Invocations

### Why it matters

Cabrero spawns `claude` CLI subprocesses for the pipeline (classifier, evaluator) and the chat panel. User-configured hooks (e.g., cabrero's own capture hooks) should not fire during these invocations to avoid:
- Self-observation loops (cabrero observing its own pipeline sessions)
- macOS TCC prompts triggered by hook scripts accessing filesystem paths
- Unnecessary overhead from hooks that aren't relevant to the subprocess

### Recommended approach

Use `--settings` with explicit overrides on all `claude` subprocess invocations:

```bash
claude --settings '{"disableAllHooks": true, "alwaysThinkingEnabled": false, "enabledPlugins": {}}' -p "query"
```

This isolates the subprocess from user settings that would affect pipeline behavior:
- `disableAllHooks: true` — suppresses all hooks and custom status line
- `alwaysThinkingEnabled: false` — prevents extended thinking (controls cost/latency)
- `enabledPlugins: {}` — prevents plugins from injecting instructions into the system prompt

### Current cabrero usage

| Invocation site | `--setting-sources` | `--settings` | Hooks disabled? |
|-----------------|---------------------|--------------|-----------------|
| Pipeline (`invokeClaude`) | — (removed in v0.27.2) | hooks + thinking + plugins | Yes |
| Chat panel (`buildChatArgs`) | — | hooks + thinking + plugins | Yes |
| Apply (`apply.go`) | — | hooks + thinking + plugins | Yes |

All three sites use `--settings` with `disableAllHooks`, `alwaysThinkingEnabled: false`,
and `enabledPlugins: {}` to isolate subprocesses from user settings. The pipeline
previously used `--setting-sources ""` as defense-in-depth, but this broke in CLI 2.1.59+
and was removed. Other non-critical settings (e.g., `cleanupPeriodDays`) may still leak
through from user/project settings files.

## Related flags

| Flag | Purpose |
|------|---------|
| `--mcp-config '{"mcpServers":{}}'` | Prevents loading user's MCP servers/plugins (**must** include `mcpServers` key — bare `{}` silently crashes CLI 2.1.59+) |
| `--strict-mcp-config` | Only use MCP servers from `--mcp-config`, ignoring all other MCP configurations (plugins, settings, project). Combined with the empty config above, this ensures zero MCP servers load. |
| `--disable-slash-commands` | Disables all skills and slash commands |
| `--permission-mode dontAsk` | Prevents permission prompts |
| `--no-session-persistence` | Doesn't persist the session transcript |
