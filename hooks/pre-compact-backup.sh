#!/bin/sh
# Cabrero pre-compact hook — captures CC transcript before compaction erases it.
# Reads CC hook JSON payload from stdin.

set -e

# Loop prevention: skip if this is a Cabrero-spawned session.
if [ "${CABRERO_SESSION}" = "1" ]; then
  exit 0
fi

# Read stdin JSON payload.
PAYLOAD=$(cat)

# Extract fields using python3 JSON parsing (robust against field ordering and escaping).
# Uses RS (0x1E) as field separator and direct variable assignment — no eval.
_PARSED=$(printf '%s' "$PAYLOAD" | python3 -c '
import json, sys, re
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)
sid = d.get("session_id", "")
tp = d.get("transcript_path", "")
cwd = d.get("cwd", "")
slug = re.sub(r"[/.]", "-", cwd)
# RS (Record Separator) cannot appear in paths or UUIDs.
sys.stdout.write(sid + "\x1e" + tp + "\x1e" + cwd + "\x1e" + slug)
' 2>/dev/null) || _PARSED=""

if [ -n "$_PARSED" ]; then
  _RS=$(printf '\036')
  SESSION_ID=$(printf '%s' "$_PARSED" | cut -d "$_RS" -f1)
  TRANSCRIPT_PATH=$(printf '%s' "$_PARSED" | cut -d "$_RS" -f2)
  SESSION_CWD=$(printf '%s' "$_PARSED" | cut -d "$_RS" -f3)
  PROJECT_SLUG=$(printf '%s' "$_PARSED" | cut -d "$_RS" -f4)
else
  SESSION_ID=""
  TRANSCRIPT_PATH=""
  SESSION_CWD=""
  PROJECT_SLUG=""
fi

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

# Ensure target directory exists.
SESSION_DIR="${CABRERO_ROOT}/raw/${SESSION_ID}"
mkdir -p "$SESSION_DIR"

# Copy transcript.
if [ -f "$TRANSCRIPT_PATH" ]; then
  cp "$TRANSCRIPT_PATH" "${SESSION_DIR}/transcript.jsonl"
fi

# Get CC version for metadata.
CC_VERSION=""
if command -v claude >/dev/null 2>&1; then
  CC_VERSION=$(claude --version 2>/dev/null || true)
fi

# Write metadata.
cat > "${SESSION_DIR}/metadata.json" << METAEOF
{
  "session_id": "${SESSION_ID}",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "capture_trigger": "pre-compact",
  "cc_version": "${CC_VERSION}",
  "project": "${PROJECT_SLUG}",
  "work_dir": "${SESSION_CWD}",
  "status": "queued"
}
METAEOF

exit 0
