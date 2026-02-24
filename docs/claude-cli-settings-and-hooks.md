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

**Undocumented:** Passing `""` (empty string) is assumed to skip all three files based on
common CLI patterns, but this behavior is **not officially documented**. The pipeline
currently relies on this — if it stops working, switch to `--settings '{"disableAllHooks": true}'`.

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

Use `--settings '{"disableAllHooks": true}'` on all `claude` subprocess invocations.

This explicitly disables the hook mechanism regardless of source — user, project, local, managed, and plugin hooks are all suppressed. It is the only **documented and reliable** way to prevent hooks from firing.

```bash
claude --settings '{"disableAllHooks": true}' -p "query"
```

### Current cabrero usage

| Invocation site | `--setting-sources` | `--settings` | Hooks disabled? |
|-----------------|---------------------|--------------|-----------------|
| Pipeline (`invokeClaude`) | `""` (defense-in-depth) | `disableAllHooks` | Yes |
| Chat panel (`buildChatArgs`) | — | `disableAllHooks` | Yes |
| Apply (`apply.go`) | — | `disableAllHooks` | Yes |

The pipeline keeps `--setting-sources ""` as defense-in-depth against non-hook settings
leaking through (e.g., `alwaysThinkingEnabled` has no CLI flag override). All three
sites use `--settings '{"disableAllHooks": true}'` as the primary hook suppression.

## Related flags

| Flag | Purpose |
|------|---------|
| `--mcp-config "{}"` | Prevents loading user's MCP servers/plugins |
| `--disable-slash-commands` | Disables slash commands |
| `--permission-mode dontAsk` | Prevents permission prompts |
| `--no-session-persistence` | Doesn't persist the session transcript |
