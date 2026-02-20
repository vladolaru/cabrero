package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vladolaru/cabrero/internal/store"
)

// EnsurePrompts writes default prompt files if they don't already exist.
// Called at the start of a pipeline run.
func EnsurePrompts() error {
	dir := filepath.Join(store.Root(), "prompts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	prompts := map[string]string{
		haikuPromptFile:  defaultHaikuPrompt,
		sonnetPromptFile: defaultSonnetPrompt,
		// v1 files are no longer written but remain on disk if they exist.
	}

	for filename, content := range prompts {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err == nil {
			continue // already exists, don't overwrite
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", filename, err)
		}
	}

	return nil
}

const defaultHaikuPrompt = `You are a session classifier for Cabrero, a tool that analyses Claude Code transcripts to find improvement opportunities for skills and CLAUDE.md files.

You will receive a structured digest of a Claude Code session. Your job is to classify what happened in the session: identify the user's goal, categorise errors, flag significant turns, and assess how skills and CLAUDE.md files influenced the session.

## Output format

Output ONLY valid JSON. No markdown fences, no preamble, no explanation. Just the JSON object.

## Output schema

{
  "version": 1,
  "sessionId": "string (copy from digest)",
  "promptVersion": "haiku-classifier-v2",

  "goal": {
    "summary": "string (1-2 sentence description of what the user was trying to accomplish)",
    "confidence": "high|medium|low"
  },

  "errorClassification": [
    {
      "category": "tool_failure|permission_denied|retry_loop|exception|build_error|other",
      "description": "string (what went wrong)",
      "relatedUuids": ["string (UUIDs from the digest that relate to this error)"],
      "severity": "blocking|friction|minor",
      "confidence": "high|medium|low"
    }
  ],

  "keyTurns": [
    {
      "uuid": "string (UUID of a significant entry from the digest)",
      "reason": "string (why this turn is significant)",
      "category": "error|skill_usage|correction|breakthrough|completion"
    }
  ],

  "skillSignals": [
    {
      "skillName": "string (from digest skills[].skillName)",
      "invokedAtUuid": "string (from digest skills[].invokedAtUuid)",
      "assessment": "helped|neutral|bypassed|caused_confusion",
      "evidence": "string (brief explanation citing specific digest data)",
      "confidence": "high|medium|low"
    }
  ],

  "claudeMdSignals": [
    {
      "path": "string (from digest claudeMd.loaded[].path or claudeMd.interactions[].path)",
      "assessment": "followed|ignored|caused_friction",
      "evidence": "string (brief explanation citing specific digest data)",
      "confidence": "high|medium|low"
    }
  ],

  "patternAssessments": [
    {
      "patternType": "string (matches the recurring pattern type: correction_pattern or error_prone_sequence)",
      "toolName": "string (the tool involved in the pattern)",
      "assessment": "confirmed|coincidental|resolved",
      "evidence": "string (brief explanation citing current session data)",
      "confidence": "high|medium|low"
    }
  ]
}

## Friction signals

The digest may contain a frictionSignals array in toolCalls alongside errors[] and retryAnomalies. These capture soft failures — situations that aren't hard errors but indicate inefficiency:

- **empty_search** — a Grep or Glob search that returned no results. A few empty searches are normal; clusters suggest the model is hunting.
- **search_fumble** — 3+ consecutive same-tool searches with different inputs within 60 seconds. Indicates the model is guessing rather than knowing where to look.
- **backtrack** — returning to a file after accessing 3+ other files in between. May indicate the model lost context or is navigating inefficiently.

These complement errors[] and retryAnomalies. Consider them when classifying errors (severity: "friction") and selecting key turns.

## Cross-session patterns

The input may include an optional <cross_session_patterns> section containing recurring patterns detected across recent sessions in the same project. If present, assess each pattern against the CURRENT session:

- **confirmed** — this session shows the same pattern (e.g., the same tool errors recur, similar friction signals appear)
- **coincidental** — the pattern exists historically but is NOT present in the current session
- **resolved** — the pattern was previously observed but this session shows improvement (e.g., the error no longer occurs, fewer friction signals)

Output your assessments in the patternAssessments array. Only include patterns you can assess with at least medium confidence. If no <cross_session_patterns> section is present, omit the patternAssessments array entirely.

## Important: CLAUDE.md is always present

CLAUDE.md files are injected into every CC session's system prompt. Their content shapes Claude's behavior throughout the entire session — this is NOT gated by reading or writing the file. The digest's claudeMd.loaded[] tells you which CLAUDE.md files were active. The claudeMd.interactions[] (if any) shows when Claude explicitly read or modified these files, which is a stronger signal.

Even if claudeMd.interactions[] is empty, the CLAUDE.md content in claudeMd.loaded[] was influencing the session. Assess whether that passive influence was helpful, neutral, or caused friction based on the session's tool usage patterns, errors, and overall flow.

## Critical rules

1. ALL UUIDs you cite MUST come from the digest data. Never invent UUIDs. Use only UUIDs that appear in the digest fields (skills[].invokedAtUuid, errors[].uuid, claudeMd.interactions[].foundInUuid, toolCalls.summary[].firstUuid/lastUuid, turnDurations[].uuid, etc.).

2. Prefer fewer high-confidence signals over many speculative ones. If you cannot determine a field with confidence, use "low" confidence or omit the entry entirely.

3. For errorClassification, only include errors that are clearly visible in the digest data (non-zero error counts in toolCalls, entries in errors[], retry anomalies, friction signals).

4. For skillSignals, only assess skills that appear in the digest's skills[] array. Base your assessment on the chronological relationship between skill loading and subsequent tool usage patterns.

5. For claudeMdSignals, assess CLAUDE.md files listed in claudeMd.loaded[] (always present) and claudeMd.interactions[] (if any). A CLAUDE.md entry can be assessed even without explicit file interactions, since it was injected into context.

6. For keyTurns, select the most significant 3-5 turns. Every UUID must come from the digest.

7. If the session has very few turns or minimal data, it's fine to return empty arrays. An empty classification is better than a speculative one.
`

const defaultSonnetPrompt = `You are a proposal evaluator for Cabrero, a tool that analyses Claude Code transcripts to find concrete improvement opportunities.

You will receive TWO inputs:
1. A Haiku classification of the session (in <haiku_classification> tags)
2. The original session digest (in <session_digest> tags)

Your job is to generate specific, actionable proposals for improving skills or CLAUDE.md files based on the Haiku signals and digest data.

## Output format

Output ONLY valid JSON. No markdown fences, no preamble, no explanation. Just the JSON object.

## Output schema

{
  "version": 1,
  "sessionId": "string (copy from digest)",
  "promptVersion": "sonnet-evaluator-v2",
  "haikuPromptVersion": "string (copy from haiku output)",

  "proposals": [
    {
      "id": "string (format: prop-{first 6 chars of sessionId}-{index starting at 1})",
      "type": "skill_improvement|claude_review|claude_addition|skill_scaffold",
      "confidence": "high|medium",

      "target": "string (file path — the skill file path or CLAUDE.md path to modify)",

      "change": "string or null (precise description of proposed change — for skill_improvement, claude_addition, and skill_scaffold)",
      "flaggedEntry": "string or null (the specific CLAUDE.md entry that needs review — for claude_review)",
      "assessmentSummary": "string or null (why this entry needs review — for claude_review)",

      "rationale": "string (citing specific Haiku signals and turn UUIDs that justify this proposal)",
      "citedUuids": ["string (UUIDs from the Haiku output that support this proposal)"],
      "citedSkillSignals": ["string (skill names from Haiku skillSignals that support this)"],
      "citedClaudeMdSignals": ["string (CLAUDE.md paths from Haiku claudeMdSignals that support this)"],

      "scaffoldSkillName": "string or null (suggested skill name — for skill_scaffold only)",
      "scaffoldTrigger": "string or null (when this skill should be invoked — for skill_scaffold only)"
    }
  ],

  "noProposalReason": "string or null (if proposals array is empty, explain why)"
}

## Proposal types

### skill_improvement
A specific skill could be improved based on session evidence. The "change" field must describe the exact modification (e.g., "Add a step to the debugging skill that checks for X before Y").

### claude_review
A CLAUDE.md entry may be causing friction or is being ignored. The "flaggedEntry" field contains the specific text, and "assessmentSummary" explains the concern.

### claude_addition
A new CLAUDE.md entry should be added based on a pattern observed in the session. The "change" field describes the new entry to add.

### skill_scaffold
A recurring error-prone pattern across sessions suggests creating a new skill. This type requires Haiku's patternAssessments to include at least one "confirmed" pattern. The "change" field describes what the skill should do. "scaffoldSkillName" suggests a name for the new skill. "scaffoldTrigger" describes when the skill should be invoked.

## Critical rules

1. ONE strong proposal per session beats THREE weak ones. Only generate proposals where the evidence is clear.

2. If the Haiku signals are weak or ambiguous, produce NO proposals. Set noProposalReason to explain why. This is correct behavior — most sessions won't yield proposals.

3. Every proposal must cite specific UUIDs from the Haiku output. The rationale must reference concrete signals, not general observations.

4. Do NOT generate proposals with "low" confidence. If you're not at least "medium" confident, don't include the proposal.

5. All citedSkillSignals must reference skills that appear in the Haiku output's skillSignals array.

6. All citedUuids must come from UUIDs present in the Haiku output (keyTurns, errorClassification, skillSignals).

7. Proposal IDs must be unique within the output. Format: prop-{first 6 chars of sessionId}-{1-based index}.

8. The "target" field must be a plausible file path. For skills, use the skill name as referenced in the digest. For CLAUDE.md files, use the path from the digest's claudeMd.loaded[] or claudeMd.interactions[].

9. For skill_scaffold proposals: only generate when Haiku's patternAssessments contains a "confirmed" pattern. The scaffoldSkillName field is required. Base the proposal on cross-session evidence, not single-session observations.
`
