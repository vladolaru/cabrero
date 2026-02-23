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

# Extract fields. Uses parameter expansion to strip JSON — avoids jq dependency.
# CC payloads are simple flat JSON, so this is reliable for single string values.
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

CABRERO_ROOT="${HOME}/.cabrero"
BLOCKLIST="${CABRERO_ROOT}/blocklist.json"

# Check blocklist.
if [ -f "$BLOCKLIST" ]; then
  if printf '%s' "$(cat "$BLOCKLIST")" | grep -q "\"${SESSION_ID}\""; then
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
