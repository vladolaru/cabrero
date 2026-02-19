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

SESSION_DIR="${CABRERO_ROOT}/raw/${SESSION_ID}"
mkdir -p "$SESSION_DIR"

# If pre-compact already captured this session, skip the copy but update metadata
# with the session-end timestamp.
if [ -f "${SESSION_DIR}/transcript.jsonl" ]; then
  # Update existing metadata with session-end timestamp.
  if [ -f "${SESSION_DIR}/metadata.json" ]; then
    # Read existing metadata and add session_end_timestamp.
    # Simple approach: rewrite with both triggers noted.
    EXISTING_TRIGGER=$(sed -n 's/.*"capture_trigger"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "${SESSION_DIR}/metadata.json")
    CC_VERSION=$(sed -n 's/.*"cc_version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "${SESSION_DIR}/metadata.json")

    cat > "${SESSION_DIR}/metadata.json" << METAEOF
{
  "session_id": "${SESSION_ID}",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "capture_trigger": "${EXISTING_TRIGGER}+session-end",
  "cc_version": "${CC_VERSION}",
  "status": "pending"
}
METAEOF
  fi
  exit 0
fi

# No prior capture — copy transcript now.
if [ -f "$TRANSCRIPT_PATH" ]; then
  cp "$TRANSCRIPT_PATH" "${SESSION_DIR}/transcript.jsonl"
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
  "capture_trigger": "session-end",
  "cc_version": "${CC_VERSION}",
  "status": "pending"
}
METAEOF

exit 0
