# Verify AI review findings against actual code before acting

Date: 2026-02-26
Tags: code-review, ai-agents, false-positives, validation, review-workflow

## Pattern

Before adding any review finding to an implementation plan, read the flagged
file:line and verify the claim against the actual code. AI review agents commonly
produce false positives in three categories: (1) control flow misreads, (2) missed
existing fixes, (3) language construct misidentification. Classify each finding as
CONFIRMED / FALSE_POSITIVE / LIKELY_VALID before acting. The ingest-code-review
skill (`/pirategoat-tools:ingest-code-review`) provides a structured workflow for
this step.

## When to apply

- After running automated review agents (pirategoat-tools, Gemini, Codex reviewers)
- Before creating or executing an implementation plan based on review output
- When a critical/high finding contradicts your understanding of the code
- When a finding describes specific code structure (control flow, loop type, variable scope)

## Alternatives

- When reviewing your own recently written code with fresh context, you can judge
  plausibility quickly — but still read the line for critical findings
- When multiple agents agree with confidence ≥ 0.9, findings are higher-trust;
  focus verification effort on single-agent or low-confidence findings
- Architecture/pattern findings are typically accurate descriptions of structure;
  prioritise verification for bug-class findings that claim specific broken behavior

## Common false positive classes

**Control flow misread** — reviewer describes a code path that doesn't exist.
Example: claimed `f.Close()` was only called inside `if n > 0`; actual code had
`f.Close()` unconditionally before the if-block.
`internal/tui/logview/follow.go:46`

**Existing fix missed** — reviewer flags code that is already correct.
Example: flagged `RenderStatusBar` as O(N²); the code had a comment
"Pre-compute each part's rendered width once to avoid O(N²) re-joins" and was
already linear. `internal/tui/components/statusbar.go:40-57`

**Language construct misidentification** — reviewer misnames a construct.
Example: called `for _, r := range s { b.WriteRune(r) }` a "byte-index loop";
`range` over a Go string iterates runes, not bytes.
`internal/daemon/notify.go:18`

## Reference implementation

Validation workflow used in 2026-02-26 code review session:
`.claude/docs/2026-02-25-tui-architecture-analysis.md` (prior architecture context)
