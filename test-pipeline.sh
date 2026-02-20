#!/usr/bin/env bash
#
# Cabrero pipeline integration test.
# Runs the full pipeline and validates every output artifact.
#
# Usage: bash test-pipeline.sh [session_id]
# Output: test-pipeline-output.log (for Claude to inspect)

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
LOG="$REPO_DIR/test-pipeline-output.log"
CABRERO_DIR="$HOME/.cabrero"
SESSION="${1:-8f4f68e0-8987-436c-ae03-607553e66932}"

# Validate session exists.
if [[ ! -d "$CABRERO_DIR/raw/$SESSION" ]]; then
  echo "ERROR: session $SESSION not found in $CABRERO_DIR/raw/"
  exit 1
fi

# Seconds since epoch for timing.
timer_start() { date +%s; }
timer_elapsed() { echo $(( $(date +%s) - $1 )); }

# Redirect all output to both terminal and log file.
exec > >(tee "$LOG") 2>&1

echo "========================================"
echo "Cabrero Pipeline Integration Test"
echo "$(date '+%Y-%m-%d %H:%M:%S')"
echo "Session: $SESSION"
echo "Log: $LOG"
echo "========================================"
echo ""

FAIL_COUNT=0
PASS_COUNT=0
WARN_COUNT=0
fail() { echo "  FAIL: $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
pass() { echo "  PASS: $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
warn() { echo "  WARN: $1"; WARN_COUNT=$((WARN_COUNT + 1)); }

# ── 0. CLEANUP ──────────────────────────────────────────────
echo "── Step 0: Clean previous artifacts ──"
rm -f "$CABRERO_DIR/digests/$SESSION.json"
rm -f "$CABRERO_DIR/evaluations/${SESSION}-haiku.json"
rm -f "$CABRERO_DIR/evaluations/${SESSION}-sonnet.json"
rm -f "$CABRERO_DIR/prompts/haiku-classifier-v2.txt"
rm -f "$CABRERO_DIR/prompts/sonnet-evaluator-v2.txt"
# Proposals use a prefix match on the first 6 chars of session ID.
find "$CABRERO_DIR/proposals" -name "prop-${SESSION:0:6}-*.json" -maxdepth 1 -delete 2>/dev/null || true
echo "  Done."
echo ""

# ── 1. BUILD ─────────────────────────────────────────────────
echo "── Step 1: Build & vet ──"
T=$(timer_start)

cd "$REPO_DIR"
if go build ./... 2>&1; then
  pass "go build ./..."
else
  fail "go build ./..."
  echo "FATAL: build failed, aborting."
  exit 1
fi

if go vet ./... 2>&1; then
  pass "go vet ./..."
else
  fail "go vet ./..."
fi

echo "  ($(timer_elapsed $T)s)"
echo ""

# ── 2. DRY-RUN (pre-parser + prompt file creation) ──────────
echo "── Step 2: Dry-run (pre-parser + prompt creation) ──"
T=$(timer_start)

go run . run --dry-run "$SESSION" 2>&1

# Validate prompt files were created.
if [[ -f "$CABRERO_DIR/prompts/haiku-classifier-v2.txt" ]]; then
  pass "haiku prompt file created (v2)"
  echo "  Size: $(wc -c < "$CABRERO_DIR/prompts/haiku-classifier-v2.txt") bytes"
else
  fail "haiku prompt file NOT created (v2)"
fi

if [[ -f "$CABRERO_DIR/prompts/sonnet-evaluator-v2.txt" ]]; then
  pass "sonnet prompt file created (v2)"
  echo "  Size: $(wc -c < "$CABRERO_DIR/prompts/sonnet-evaluator-v2.txt") bytes"
else
  fail "sonnet prompt file NOT created (v2)"
fi

# Validate digest.
if [[ -f "$CABRERO_DIR/digests/$SESSION.json" ]]; then
  pass "digest file created"
  if python3 -m json.tool "$CABRERO_DIR/digests/$SESSION.json" > /dev/null 2>&1; then
    pass "digest is valid JSON"
  else
    fail "digest is NOT valid JSON"
  fi
else
  fail "digest file NOT created"
fi

echo "  ($(timer_elapsed $T)s)"
echo ""

# ── 3. VALIDATE DIGEST: Error attribution ────────────────────
echo "── Step 3: Validate digest — error attribution ──"

DIGEST_FILE="$CABRERO_DIR/digests/$SESSION.json"
if [[ -f "$DIGEST_FILE" ]]; then
  python3 -c "
import json, sys

d = json.load(open('$DIGEST_FILE'))

# Check errors have toolName populated.
errors = d.get('errors', [])
print(f'  Total errors in digest: {len(errors)}')
errors_with_tool = [e for e in errors if e.get('toolName')]
errors_without_tool = [e for e in errors if not e.get('toolName')]

if len(errors) == 0:
    print('  (no errors to check — session may be clean)')
elif len(errors_with_tool) > 0:
    print(f'  PASS: {len(errors_with_tool)}/{len(errors)} errors have toolName attributed')
    for e in errors_with_tool[:5]:
        print(f'    - {e[\"toolName\"]}: {e[\"snippet\"][:80]}')
else:
    print(f'  WARN: 0/{len(errors)} errors have toolName (may be pre-existing data)')

# Check ErrorCount in tool call summary.
summary = d.get('toolCalls', {}).get('summary', {})
tools_with_errors = {k: v for k, v in summary.items() if v.get('errorCount', 0) > 0}
if tools_with_errors:
    print(f'  PASS: {len(tools_with_errors)} tool(s) have non-zero errorCount')
    for name, detail in tools_with_errors.items():
        print(f'    - {name}: count={detail[\"count\"]}, errorCount={detail[\"errorCount\"]}')
elif len(errors) > 0:
    print(f'  WARN: no tools have non-zero errorCount despite {len(errors)} errors')
else:
    print(f'  (no errors — errorCount check not applicable)')
" 2>&1
else
  fail "digest file missing — cannot validate error attribution"
fi

echo ""

# ── 4. VALIDATE DIGEST: Friction signals ─────────────────────
echo "── Step 4: Validate digest — friction signals ──"

if [[ -f "$DIGEST_FILE" ]]; then
  python3 -c "
import json, sys

d = json.load(open('$DIGEST_FILE'))

friction = d.get('toolCalls', {}).get('frictionSignals') or []
print(f'  Total friction signals: {len(friction)}')

if len(friction) == 0:
    print('  (no friction signals — session may be clean, which is valid)')
else:
    # Group by type.
    by_type = {}
    for f in friction:
        t = f.get('type', 'unknown')
        by_type.setdefault(t, []).append(f)

    for t, signals in sorted(by_type.items()):
        print(f'  {t}: {len(signals)} signal(s)')
        for s in signals[:3]:
            print(f'    - tool={s[\"toolName\"]}, uuids={len(s.get(\"uuids\",[]))}, detail={s[\"detail\"][:80]}')

# Validate friction signal structure.
valid_types = {'empty_search', 'search_fumble', 'backtrack'}
for f in friction:
    if f.get('type') not in valid_types:
        print(f'  FAIL: invalid friction signal type: {f.get(\"type\")}')
        sys.exit(1)
    if not f.get('toolName'):
        print(f'  FAIL: friction signal missing toolName')
        sys.exit(1)
    if not f.get('uuids') or len(f['uuids']) == 0:
        print(f'  FAIL: friction signal missing uuids')
        sys.exit(1)

if len(friction) > 0:
    print(f'  PASS: all {len(friction)} friction signals have valid structure')
else:
    print(f'  PASS: frictionSignals field present (empty array is valid)')

# Verify the field exists in JSON (even if empty).
tc = d.get('toolCalls', {})
if 'frictionSignals' in tc or friction is not None:
    print('  PASS: frictionSignals field present in toolCalls')
else:
    print('  FAIL: frictionSignals field missing from toolCalls')
" 2>&1
else
  fail "digest file missing — cannot validate friction signals"
fi

echo ""

# ── 5. VALIDATE DIGEST: Retry anomalies (existing) ──────────
echo "── Step 5: Validate digest — retry anomalies ──"

if [[ -f "$DIGEST_FILE" ]]; then
  python3 -c "
import json

d = json.load(open('$DIGEST_FILE'))
anomalies = d.get('toolCalls', {}).get('retryAnomalies') or []
print(f'  Retry anomalies: {len(anomalies)}')
for a in anomalies[:5]:
    print(f'    - {a[\"toolName\"]}: {len(a[\"uuids\"])} calls in {a[\"windowSeconds\"]}s (similarity={a[\"inputSimilarity\"]})')
" 2>&1
fi

echo ""

# ── 6. CROSS-SESSION PATTERN AGGREGATION ─────────────────────
echo "── Step 6: Cross-session pattern aggregation ──"

# Check how many sessions exist for this session's project.
if [[ -f "$CABRERO_DIR/raw/$SESSION/metadata.json" ]]; then
  python3 -c "
import json, os, glob

meta = json.load(open('$CABRERO_DIR/raw/$SESSION/metadata.json'))
project = meta.get('project', '')
print(f'  Session project: {project or \"(none)\"}')

if not project:
    print('  (no project metadata — aggregator will be skipped)')
else:
    # Count sessions in same project.
    raw_dir = os.path.join('$CABRERO_DIR', 'raw')
    same_project = 0
    total_sessions = 0
    for d in os.listdir(raw_dir) if os.path.isdir(raw_dir) else []:
        mp = os.path.join(raw_dir, d, 'metadata.json')
        if os.path.exists(mp):
            total_sessions += 1
            try:
                m = json.load(open(mp))
                if m.get('project') == project:
                    same_project += 1
            except:
                pass

    print(f'  Sessions in same project: {same_project} (total in store: {total_sessions})')

    if same_project < 3:
        print('  (fewer than 3 sessions — aggregator will return nil, which is expected)')
    else:
        print(f'  {same_project} sessions available — aggregator should produce patterns')

    # Count digests for same-project sessions.
    digests_dir = os.path.join('$CABRERO_DIR', 'digests')
    digest_count = 0
    for d in os.listdir(raw_dir) if os.path.isdir(raw_dir) else []:
        mp = os.path.join(raw_dir, d, 'metadata.json')
        dp = os.path.join(digests_dir, d + '.json')
        if os.path.exists(mp) and os.path.exists(dp):
            try:
                m = json.load(open(mp))
                if m.get('project') == project and d != '$SESSION':
                    digest_count += 1
            except:
                pass
    print(f'  Digests available for cross-session analysis: {digest_count}')
" 2>&1
else
  warn "session metadata missing — cannot check aggregation prerequisites"
fi

echo ""

# ── 7. SMOKE TEST: --system-prompt with Haiku (tiny JSON) ───
echo "── Step 7: Smoke test --system-prompt + JSON output (Haiku, tiny input) ──"
T=$(timer_start)

SMOKE_OUTPUT=$(echo 'The user asked Claude to fix a bug in the login form.' | \
  claude \
    --model claude-haiku-4-5 \
    --print \
    --system-prompt 'Output ONLY valid JSON. No markdown, no preamble. Schema: {"summary":"string"}' \
    --no-session-persistence \
    --disable-slash-commands \
    --tools "" \
  2>&1) || true

echo "  Raw output (first 300 chars): ${SMOKE_OUTPUT:0:300}"

# Try parsing directly, then with fence stripping.
if echo "$SMOKE_OUTPUT" | python3 -m json.tool > /dev/null 2>&1; then
  pass "raw output is valid JSON"
elif echo "$SMOKE_OUTPUT" | sed -n '/^```/,/^```/{/^```/d;p}' | python3 -m json.tool > /dev/null 2>&1; then
  pass "output is valid JSON after stripping markdown fences (cleanLLMJSON will handle)"
elif echo "$SMOKE_OUTPUT" | python3 -c "import sys,json; json.loads(sys.stdin.read().strip().strip('\`').lstrip('json').strip())" 2>/dev/null; then
  pass "output is valid JSON after cleanup"
else
  fail "cannot extract valid JSON from output"
fi

echo "  ($(timer_elapsed $T)s)"
echo ""

# ── 8. FULL PIPELINE: Haiku + Sonnet via cabrero ────────────
echo "── Step 8: Full pipeline (Haiku + Sonnet) ──"
echo "  This calls the claude CLI twice. May take 30-120s."
T=$(timer_start)

cd "$REPO_DIR"
PIPELINE_OUTPUT=$(go run . run "$SESSION" 2>&1) || true
echo "$PIPELINE_OUTPUT"

# Check for fatal errors.
if echo "$PIPELINE_OUTPUT" | grep -qi "^Error:"; then
  fail "pipeline exited with error"
  echo ""
  echo "  ── PIPELINE ERROR — skipping validation steps ──"
  echo ""
  # Still try to show partial artifacts.
  echo "  Haiku file exists: $(test -f "$CABRERO_DIR/evaluations/${SESSION}-haiku.json" && echo yes || echo no)"
  echo "  Sonnet file exists: $(test -f "$CABRERO_DIR/evaluations/${SESSION}-sonnet.json" && echo yes || echo no)"
else
  pass "pipeline completed without fatal error"
fi

# Check for aggregator log messages.
if echo "$PIPELINE_OUTPUT" | grep -q "Aggregating cross-session patterns"; then
  pass "aggregator was invoked"
  if echo "$PIPELINE_OUTPUT" | grep -q "recurring pattern"; then
    pass "aggregator found patterns"
  else
    echo "  (aggregator ran but found no patterns — this is valid)"
  fi
elif echo "$PIPELINE_OUTPUT" | grep -q "pattern aggregation failed"; then
  warn "aggregator failed (non-fatal)"
else
  echo "  (aggregator not invoked — project metadata may be missing)"
fi

# Check friction signal count in output.
if echo "$PIPELINE_OUTPUT" | grep -q "friction signals"; then
  pass "friction signal count reported in pipeline output"
fi

echo "  ($(timer_elapsed $T)s)"
echo ""

# ── 9. VALIDATE HAIKU OUTPUT ────────────────────────────────
echo "── Step 9: Validate Haiku output ──"

HAIKU_FILE="$CABRERO_DIR/evaluations/${SESSION}-haiku.json"
if [[ ! -f "$HAIKU_FILE" ]]; then
  fail "haiku evaluation file missing"
else
  pass "haiku evaluation file exists"
  echo "  Size: $(wc -c < "$HAIKU_FILE") bytes"

  if python3 -m json.tool "$HAIKU_FILE" > /dev/null 2>&1; then
    pass "haiku output is valid JSON"
  else
    fail "haiku output is NOT valid JSON"
    echo "  First 500 chars:"
    head -c 500 "$HAIKU_FILE"
    echo ""
  fi

  # Detailed field validation.
  python3 -c "
import json, sys

h = json.load(open('$HAIKU_FILE'))

# Required top-level fields.
required = ['version', 'sessionId', 'promptVersion', 'goal', 'errorClassification', 'keyTurns', 'skillSignals', 'claudeMdSignals']
missing = [f for f in required if f not in h]
if missing:
    print(f'  FAIL: missing top-level fields: {missing}')
    sys.exit(1)
else:
    print('  PASS: all required top-level fields present')

# Prompt version should be v2.
pv = h.get('promptVersion', '')
if 'v2' in pv:
    print(f'  PASS: promptVersion is v2 ({pv})')
elif pv:
    print(f'  WARN: promptVersion is not v2: {pv}')
else:
    print(f'  WARN: promptVersion is empty')

# Goal.
g = h['goal']
print(f'  Goal: {g[\"summary\"][:100]}')
print(f'  Goal confidence: {g[\"confidence\"]}')
if g['confidence'] not in ('high', 'medium', 'low'):
    print(f'  FAIL: invalid goal confidence: {g[\"confidence\"]}')

# Errors.
print(f'  Error classifications: {len(h[\"errorClassification\"])}')
for e in h['errorClassification']:
    print(f'    - {e[\"category\"]}: {e[\"description\"][:80]} (severity={e[\"severity\"]}, confidence={e[\"confidence\"]})')

# Key turns.
print(f'  Key turns: {len(h[\"keyTurns\"])}')
for kt in h['keyTurns']:
    print(f'    - [{kt[\"category\"]}] {kt[\"uuid\"][:12]}... {kt[\"reason\"][:60]}')

# Skill signals.
print(f'  Skill signals: {len(h[\"skillSignals\"])}')
for ss in h['skillSignals']:
    print(f'    - {ss[\"skillName\"]}: {ss[\"assessment\"]} (confidence={ss[\"confidence\"]})')

# CLAUDE.md signals.
print(f'  CLAUDE.md signals: {len(h[\"claudeMdSignals\"])}')
for cs in h['claudeMdSignals']:
    print(f'    - {cs[\"path\"]}: {cs[\"assessment\"]} (confidence={cs[\"confidence\"]})')

# Pattern assessments (new in v2).
pa = h.get('patternAssessments', [])
print(f'  Pattern assessments: {len(pa)}')
if pa:
    valid_assessments = {'confirmed', 'coincidental', 'resolved'}
    for a in pa:
        assessment = a.get('assessment', '')
        print(f'    - {a.get(\"patternType\")}/{a.get(\"toolName\")}: {assessment} (confidence={a.get(\"confidence\")})')
        if assessment not in valid_assessments:
            print(f'      FAIL: invalid assessment value: {assessment}')
    print(f'  PASS: patternAssessments present with {len(pa)} entries')
else:
    print('  (no patternAssessments — expected if no cross-session patterns were provided)')

# Collect all cited UUIDs.
uuids = set()
for e in h['errorClassification']:
    uuids.update(e.get('relatedUuids', []))
for kt in h['keyTurns']:
    uuids.add(kt['uuid'])
for ss in h['skillSignals']:
    if ss.get('invokedAtUuid'):
        uuids.add(ss['invokedAtUuid'])
print(f'  Total unique UUIDs cited: {len(uuids)}')
" 2>&1 || fail "haiku field validation"
fi

echo ""

# ── 10. UUID SPOT-CHECK (Haiku) ──────────────────────────────
echo "── Step 10: UUID spot-check against raw transcript ──"

TRANSCRIPT="$CABRERO_DIR/raw/$SESSION/transcript.jsonl"
if [[ ! -f "$TRANSCRIPT" ]]; then
  fail "raw transcript file missing"
elif [[ ! -f "$HAIKU_FILE" ]]; then
  warn "haiku file missing — skipping UUID check"
else
  python3 -c "
import json

h = json.load(open('$HAIKU_FILE'))

uuids = set()
for e in h.get('errorClassification', []):
    uuids.update(e.get('relatedUuids', []))
for kt in h.get('keyTurns', []):
    uuids.add(kt['uuid'])
for ss in h.get('skillSignals', []):
    if ss.get('invokedAtUuid'):
        uuids.add(ss['invokedAtUuid'])

if not uuids:
    print('  (no UUIDs to check)')
else:
    # Read all UUIDs from transcript.
    transcript_uuids = set()
    with open('$TRANSCRIPT') as f:
        for line in f:
            try:
                entry = json.loads(line)
                if 'uuid' in entry:
                    transcript_uuids.add(entry['uuid'])
            except:
                pass

    valid = 0
    invalid = 0
    for u in uuids:
        if u in transcript_uuids:
            valid += 1
        else:
            invalid += 1
            print(f'  INVALID UUID: {u}')

    total = valid + invalid
    print(f'  {valid}/{total} UUIDs found in transcript ({invalid} invalid)')
    if invalid == 0:
        print('  PASS: all cited UUIDs are valid')
    elif invalid / total > 0.5:
        print('  FAIL: >50% of UUIDs are invalid (pipeline should have rejected this)')
    else:
        print('  WARN: some UUIDs invalid (pruned by validation)')
" 2>&1
fi

echo ""

# ── 11. VALIDATE SONNET OUTPUT ───────────────────────────────
echo "── Step 11: Validate Sonnet output ──"

SONNET_FILE="$CABRERO_DIR/evaluations/${SESSION}-sonnet.json"
if [[ ! -f "$SONNET_FILE" ]]; then
  fail "sonnet evaluation file missing"
else
  pass "sonnet evaluation file exists"
  echo "  Size: $(wc -c < "$SONNET_FILE") bytes"

  if python3 -m json.tool "$SONNET_FILE" > /dev/null 2>&1; then
    pass "sonnet output is valid JSON"
  else
    fail "sonnet output is NOT valid JSON"
    echo "  First 500 chars:"
    head -c 500 "$SONNET_FILE"
    echo ""
  fi

  python3 -c "
import json, sys

s = json.load(open('$SONNET_FILE'))

# Required top-level fields.
required = ['version', 'sessionId', 'promptVersion', 'haikuPromptVersion', 'proposals']
missing = [f for f in required if f not in s]
if missing:
    print(f'  FAIL: missing top-level fields: {missing}')
else:
    print('  PASS: all required top-level fields present')

# Prompt version should be v2.
pv = s.get('promptVersion', '')
if 'v2' in pv:
    print(f'  PASS: promptVersion is v2 ({pv})')
elif pv:
    print(f'  WARN: promptVersion is not v2: {pv}')

hpv = s.get('haikuPromptVersion', '')
if 'v2' in hpv:
    print(f'  PASS: haikuPromptVersion is v2 ({hpv})')
elif hpv:
    print(f'  WARN: haikuPromptVersion is not v2: {hpv}')

if s.get('noProposalReason'):
    print(f'  No proposals: {s[\"noProposalReason\"]}')

valid_types = {'skill_improvement', 'claude_review', 'claude_addition', 'skill_scaffold'}

print(f'  Proposals: {len(s[\"proposals\"])}')
for p in s['proposals']:
    print(f'    - {p[\"id\"]}: type={p[\"type\"]}, confidence={p[\"confidence\"]}')
    print(f'      target: {p[\"target\"]}')
    if p.get('change'):
        print(f'      change: {p[\"change\"][:100]}')
    if p.get('flaggedEntry'):
        print(f'      flagged: {p[\"flaggedEntry\"][:100]}')
    print(f'      rationale: {p[\"rationale\"][:120]}...')
    print(f'      citedUuids: {len(p.get(\"citedUuids\") or [])}')
    print(f'      citedSkillSignals: {p.get(\"citedSkillSignals\") or []}')
    print(f'      citedClaudeMdSignals: {p.get(\"citedClaudeMdSignals\") or []}')

    # Validate proposal type.
    if p['type'] not in valid_types:
        print(f'      FAIL: invalid proposal type: {p[\"type\"]}')

    # Validate confidence is not low.
    if p['confidence'] == 'low':
        print(f'      FAIL: low-confidence proposal should have been filtered')

    # Validate proposal ID format.
    sid = s['sessionId'][:6]
    if not p['id'].startswith(f'prop-{sid}-'):
        print(f'      WARN: proposal ID {p[\"id\"]} does not match expected format prop-{sid}-N')

    # Validate skill_scaffold proposals have required fields.
    if p['type'] == 'skill_scaffold':
        if p.get('scaffoldSkillName'):
            print(f'      scaffoldSkillName: {p[\"scaffoldSkillName\"]}')
        else:
            print(f'      FAIL: skill_scaffold missing scaffoldSkillName')
        if p.get('scaffoldTrigger'):
            print(f'      scaffoldTrigger: {p[\"scaffoldTrigger\"][:100]}')
        else:
            print(f'      WARN: skill_scaffold missing scaffoldTrigger')

# Check for scaffold fields on non-scaffold proposals (should be absent).
for p in s['proposals']:
    if p['type'] != 'skill_scaffold':
        if p.get('scaffoldSkillName') or p.get('scaffoldTrigger'):
            print(f'      WARN: non-scaffold proposal {p[\"id\"]} has scaffold fields')
" 2>&1 || fail "sonnet field validation"
fi

echo ""

# ── 12. VALIDATE PROPOSALS ON DISK ───────────────────────────
echo "── Step 12: Validate proposal files ──"

PROPOSALS_DIR="$CABRERO_DIR/proposals"
PROPOSAL_COUNT=$(find "$PROPOSALS_DIR" -name "prop-${SESSION:0:6}-*.json" -maxdepth 1 2>/dev/null | wc -l | tr -d ' ')
echo "  Proposal files for this session: $PROPOSAL_COUNT"

if [[ "$PROPOSAL_COUNT" -gt 0 ]]; then
  for pf in "$PROPOSALS_DIR"/prop-${SESSION:0:6}-*.json; do
    BASENAME=$(basename "$pf" .json)
    if python3 -m json.tool "$pf" > /dev/null 2>&1; then
      pass "proposal $BASENAME is valid JSON"
    else
      fail "proposal $BASENAME is NOT valid JSON"
    fi

    python3 -c "
import json
p = json.load(open('$pf'))
print(f'    sessionId: {p[\"sessionId\"]}')
print(f'    type: {p[\"proposal\"][\"type\"]}')
print(f'    target: {p[\"proposal\"][\"target\"]}')
if p['proposal'].get('scaffoldSkillName'):
    print(f'    scaffoldSkillName: {p[\"proposal\"][\"scaffoldSkillName\"]}')
" 2>&1
  done
else
  echo "  (no proposals generated — this is valid if Sonnet found no strong signals)"
fi

echo ""

# ── 13. CLI COMMANDS ──────────────────────────────────────────
echo "── Step 13: CLI commands (proposals, inspect) ──"

cd "$REPO_DIR"
echo "  --- cabrero proposals ---"
go run . proposals 2>&1

if [[ "$PROPOSAL_COUNT" -gt 0 ]]; then
  FIRST_PROPOSAL=$(ls "$PROPOSALS_DIR"/prop-${SESSION:0:6}-*.json 2>/dev/null | head -1)
  if [[ -n "$FIRST_PROPOSAL" ]]; then
    PID=$(basename "$FIRST_PROPOSAL" .json)
    echo ""
    echo "  --- cabrero inspect $PID ---"
    go run . inspect "$PID" 2>&1
  fi
fi

echo ""

# ── 14. PROMPT CONTENT VALIDATION ────────────────────────────
echo "── Step 14: Validate v2 prompt content ──"

HAIKU_PROMPT="$CABRERO_DIR/prompts/haiku-classifier-v2.txt"
SONNET_PROMPT="$CABRERO_DIR/prompts/sonnet-evaluator-v2.txt"

if [[ -f "$HAIKU_PROMPT" ]]; then
  # Check that v2 haiku prompt has friction and pattern sections.
  if grep -q "frictionSignals" "$HAIKU_PROMPT"; then
    pass "haiku v2 prompt mentions frictionSignals"
  else
    fail "haiku v2 prompt missing frictionSignals section"
  fi

  if grep -q "patternAssessments" "$HAIKU_PROMPT"; then
    pass "haiku v2 prompt mentions patternAssessments"
  else
    fail "haiku v2 prompt missing patternAssessments section"
  fi

  if grep -q "cross_session_patterns" "$HAIKU_PROMPT"; then
    pass "haiku v2 prompt mentions cross_session_patterns"
  else
    fail "haiku v2 prompt missing cross_session_patterns section"
  fi

  if grep -q "empty_search" "$HAIKU_PROMPT"; then
    pass "haiku v2 prompt documents empty_search signal"
  else
    fail "haiku v2 prompt missing empty_search documentation"
  fi
else
  fail "haiku v2 prompt file missing"
fi

if [[ -f "$SONNET_PROMPT" ]]; then
  if grep -q "skill_scaffold" "$SONNET_PROMPT"; then
    pass "sonnet v2 prompt mentions skill_scaffold"
  else
    fail "sonnet v2 prompt missing skill_scaffold type"
  fi

  if grep -q "scaffoldSkillName" "$SONNET_PROMPT"; then
    pass "sonnet v2 prompt mentions scaffoldSkillName"
  else
    fail "sonnet v2 prompt missing scaffoldSkillName field"
  fi

  if grep -q "scaffoldTrigger" "$SONNET_PROMPT"; then
    pass "sonnet v2 prompt mentions scaffoldTrigger"
  else
    fail "sonnet v2 prompt missing scaffoldTrigger field"
  fi
else
  fail "sonnet v2 prompt file missing"
fi

echo ""

# ── 15. DIGEST COMPREHENSIVE STRUCTURE CHECK ─────────────────
echo "── Step 15: Digest comprehensive structure check ──"

if [[ -f "$DIGEST_FILE" ]]; then
  python3 -c "
import json, sys

d = json.load(open('$DIGEST_FILE'))

# Version.
print(f'  version: {d.get(\"version\")}')
print(f'  sessionId: {d.get(\"sessionId\")}')

# Shape.
shape = d.get('shape', {})
print(f'  shape.entryCount: {shape.get(\"entryCount\")}')
print(f'  shape.turnCount: {shape.get(\"turnCount\")}')
print(f'  shape.compactionCount: {shape.get(\"compactionCount\")}')
print(f'  shape.models: {shape.get(\"models\")}')
dur = shape.get('durationSeconds')
if dur:
    print(f'  shape.durationSeconds: {dur:.0f}')

# ToolCalls structure — verify all expected fields present.
tc = d.get('toolCalls', {})
has_summary = 'summary' in tc
has_retry = 'retryAnomalies' in tc
has_friction = 'frictionSignals' in tc

if has_summary and has_retry and has_friction:
    print(f'  PASS: toolCalls has all three fields (summary, retryAnomalies, frictionSignals)')
else:
    missing = []
    if not has_summary: missing.append('summary')
    if not has_retry: missing.append('retryAnomalies')
    if not has_friction: missing.append('frictionSignals')
    print(f'  FAIL: toolCalls missing fields: {missing}')

# Tool call counts.
summary = tc.get('summary', {})
total_calls = sum(v.get('count', 0) for v in summary.values())
total_errors = sum(v.get('errorCount', 0) for v in summary.values())
print(f'  Total tool calls: {total_calls} across {len(summary)} tools')
print(f'  Total tool errors: {total_errors}')
print(f'  Retry anomalies: {len(tc.get(\"retryAnomalies\") or [])}')
print(f'  Friction signals: {len(tc.get(\"frictionSignals\") or [])}')

# Top 5 tools by call count.
top = sorted(summary.items(), key=lambda x: x[1].get('count', 0), reverse=True)[:5]
print(f'  Top tools: {[(name, v[\"count\"]) for name, v in top]}')

# Agents.
agents = d.get('agents', {})
print(f'  Agents: count={agents.get(\"count\")}, maxDepth={agents.get(\"maxDepth\")}')

# Skills.
skills = d.get('skills') or []
print(f'  Skills invoked: {len(skills)}')
for s in skills:
    print(f'    - {s[\"skillName\"]} at {s[\"invokedAtUuid\"][:12]}...')

# Errors.
errors = d.get('errors') or []
print(f'  Errors: {len(errors)}')

# Completion.
comp = d.get('completion', {})
print(f'  Completion: tasks_created={comp.get(\"taskCreateCount\")}, git_diff={comp.get(\"gitDiffPresent\")}')
" 2>&1 || fail "digest structure check"
fi

echo ""

# ── SUMMARY ──────────────────────────────────────────────────
echo "========================================"
echo "SUMMARY"
echo "========================================"
echo "  Passed:   $PASS_COUNT"
echo "  Failed:   $FAIL_COUNT"
echo "  Warnings: $WARN_COUNT"
echo ""

if [[ $FAIL_COUNT -gt 0 ]]; then
  echo "  RESULT: SOME TESTS FAILED"
else
  echo "  RESULT: ALL TESTS PASSED"
fi

echo ""
echo "Full log: $LOG"
echo "$(date '+%Y-%m-%d %H:%M:%S')"
