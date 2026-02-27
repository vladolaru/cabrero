#!/bin/sh
# Cabrero session-end hook — final capture when a CC session closes.
# Reads CC hook JSON payload from stdin.

set -e

# Loop prevention: skip if this is a Cabrero-spawned session.
if [ "${CABRERO_SESSION}" = "1" ]; then
  exit 0
fi

# Read stdin JSON payload.
PAYLOAD=$(cat)

# Extract fields using python3 JSON parsing (robust against field ordering and escaping).
eval "$(printf '%s' "$PAYLOAD" | python3 -c '
import json, sys, re
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)
sid = d.get("session_id", "")
tp = d.get("transcript_path", "")
cwd = d.get("cwd", "")
slug = re.sub(r"[/.]", "-", cwd)
# Escape for shell single-quote safety.
def sh(s): return s.replace("\\", "\\\\").replace("\"", "\\\"")
print(f"SESSION_ID=\"{sh(sid)}\"")
print(f"TRANSCRIPT_PATH=\"{sh(tp)}\"")
print(f"SESSION_CWD=\"{sh(cwd)}\"")
print(f"PROJECT_SLUG=\"{sh(slug)}\"")
' 2>/dev/null || echo 'SESSION_ID=""; TRANSCRIPT_PATH=""')"

if [ -z "$SESSION_ID" ] || [ -z "$TRANSCRIPT_PATH" ]; then
  exit 0
fi

# Reject SESSION_ID values that contain path components (traversal guard).
case "$SESSION_ID" in
  */* | *..*) echo "cabrero: invalid session_id, skipping" >&2; exit 0 ;;
esac

CABRERO_ROOT="${HOME}/.cabrero"
BLOCKLIST="${CABRERO_ROOT}/blocklist.json"

# Check blocklist using JSON-aware lookup.
if [ -f "$BLOCKLIST" ]; then
  if python3 -c '
import json, sys
try:
    bl = json.load(open(sys.argv[1]))
    ids = bl if isinstance(bl, list) else bl.get("sessions", [])
    sys.exit(0 if sys.argv[2] in ids else 1)
except Exception:
    sys.exit(1)
' "$BLOCKLIST" "$SESSION_ID" 2>/dev/null; then
    exit 0
  fi
fi

SESSION_DIR="${CABRERO_ROOT}/raw/${SESSION_ID}"
mkdir -p "$SESSION_DIR"

# Always copy transcript — the SessionEnd version is a strict superset of any
# PreCompact capture (compaction appends a boundary, doesn't truncate).
if [ -f "$TRANSCRIPT_PATH" ]; then
  cp "$TRANSCRIPT_PATH" "${SESSION_DIR}/transcript.jsonl"
fi

# Determine capture trigger (note if PreCompact already ran).
CAPTURE_TRIGGER="session-end"
if [ -f "${SESSION_DIR}/metadata.json" ]; then
  EXISTING_TRIGGER=$(python3 -c '
import json, sys
try:
    d = json.load(open(sys.argv[1]))
    print(d.get("capture_trigger", ""))
except Exception:
    pass
' "${SESSION_DIR}/metadata.json" 2>/dev/null)
  if [ -n "$EXISTING_TRIGGER" ]; then
    CAPTURE_TRIGGER="${EXISTING_TRIGGER}+session-end"
  fi
fi

# Get CC version.
CC_VERSION=""
if command -v claude >/dev/null 2>&1; then
  CC_VERSION=$(claude --version 2>/dev/null || true)
fi

cat > "${SESSION_DIR}/metadata.json" << METAEOF
{
  "session_id": "${SESSION_ID}",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "capture_trigger": "${CAPTURE_TRIGGER}",
  "cc_version": "${CC_VERSION}",
  "project": "${PROJECT_SLUG}",
  "work_dir": "${SESSION_CWD}",
  "status": "queued"
}
METAEOF

exit 0
