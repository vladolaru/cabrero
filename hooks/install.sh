#!/bin/sh
# Installs Cabrero hook scripts into ~/.claude/settings.json.
# Preserves any existing hooks already configured.
# Requires: python3 (available on macOS by default) for JSON manipulation.

set -e

SETTINGS_FILE="${HOME}/.claude/settings.json"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

PRE_COMPACT_HOOK="${SCRIPT_DIR}/pre-compact-backup.sh"
SESSION_END_HOOK="${SCRIPT_DIR}/session-end.sh"

# Ensure hook scripts are executable.
chmod +x "$PRE_COMPACT_HOOK"
chmod +x "$SESSION_END_HOOK"

if [ ! -f "$SETTINGS_FILE" ]; then
  echo "Error: ${SETTINGS_FILE} not found. Is Claude Code installed?"
  exit 1
fi

# Use python3 for reliable JSON manipulation (ships with macOS).
python3 << PYEOF
import json
import sys

settings_path = "${SETTINGS_FILE}"
pre_compact_cmd = "${PRE_COMPACT_HOOK}"
session_end_cmd = "${SESSION_END_HOOK}"

with open(settings_path, "r") as f:
    settings = json.load(f)

if "hooks" not in settings:
    settings["hooks"] = {}

hooks = settings["hooks"]

def has_cabrero_hook(hook_list, script_name):
    """Check if a Cabrero hook is already installed."""
    for group in hook_list:
        for h in group.get("hooks", []):
            if h.get("type") == "command" and script_name in h.get("command", ""):
                return True
    return False

# Add PreCompact hook if not present.
if "PreCompact" not in hooks:
    hooks["PreCompact"] = []
if not has_cabrero_hook(hooks["PreCompact"], "pre-compact-backup.sh"):
    hooks["PreCompact"].append({
        "matcher": "",
        "hooks": [{
            "type": "command",
            "command": pre_compact_cmd,
            "timeout": 30
        }]
    })
    print("Installed PreCompact hook.")
else:
    print("PreCompact hook already installed.")

# Add SessionEnd hook if not present.
if "SessionEnd" not in hooks:
    hooks["SessionEnd"] = []
if not has_cabrero_hook(hooks["SessionEnd"], "session-end.sh"):
    hooks["SessionEnd"].append({
        "matcher": "",
        "hooks": [{
            "type": "command",
            "command": session_end_cmd,
            "timeout": 30
        }]
    })
    print("Installed SessionEnd hook.")
else:
    print("SessionEnd hook already installed.")

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)
    f.write("\n")

print("Done. Hooks written to " + settings_path)
PYEOF
