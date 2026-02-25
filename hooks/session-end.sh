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

SESSION_ID=$(printf '%s' "$PAYLOAD" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
TRANSCRIPT_PATH=$(printf '%s' "$PAYLOAD" | sed -n 's/.*"transcript_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
SESSION_CWD=$(printf '%s' "$PAYLOAD" | sed -n 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
# Derive project slug from cwd: replace / and . with - (matches CC encoding).
PROJECT_SLUG=$(printf '%s' "$SESSION_CWD" | sed 's/[\/.]/-/g')
# Escape backslashes and quotes for safe JSON interpolation in heredoc.
SESSION_CWD=$(printf '%s' "$SESSION_CWD" | sed 's/\\/\\\\/g; s/"/\\"/g')

if [ -z "$SESSION_ID" ] || [ -z "$TRANSCRIPT_PATH" ]; then
  exit 0
fi

# Reject SESSION_ID values that contain path components (traversal guard).
case "$SESSION_ID" in
  */* | *..*) echo "cabrero: invalid session_id, skipping" >&2; exit 0 ;;
esac

CABRERO_ROOT="${HOME}/.cabrero"
BLOCKLIST="${CABRERO_ROOT}/blocklist.json"

# Check blocklist.
if [ -f "$BLOCKLIST" ]; then
  if printf '%s' "$(cat "$BLOCKLIST")" | grep -q "\"${SESSION_ID}\""; then
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
  EXISTING_TRIGGER=$(sed -n 's/.*"capture_trigger"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "${SESSION_DIR}/metadata.json")
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
