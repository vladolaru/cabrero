// Package apply implements the blend/commit/archive workflow for proposals.
// Used by both the CLI (cabrero approve) and the TUI review flow.
package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// blendTimeout is the maximum time to wait for a Blend operation.
const blendTimeout = 2 * time.Minute

// Blend invokes Claude to blend the proposed change into the target file.
// Returns a before/after diff string suitable for human review.
func Blend(proposal *pipeline.Proposal, sessionID string, model string) (string, error) {
	if proposal.Target == "" {
		return "", fmt.Errorf("proposal has no target file")
	}

	target := expandPath(proposal.Target)

	if err := validateTarget(target); err != nil {
		return "", fmt.Errorf("unsafe target path: %w", err)
	}

	// Read current file content (may not exist for scaffolds).
	current, _ := os.ReadFile(target)

	var changeText string
	if proposal.Change != nil {
		changeText = *proposal.Change
	}
	if proposal.FlaggedEntry != nil {
		changeText = "FLAGGED ENTRY: " + *proposal.FlaggedEntry
		if proposal.AssessmentSummary != nil {
			changeText += "\nASSESSMENT: " + *proposal.AssessmentSummary
		}
	}

	prompt := buildBlendPrompt(string(current), changeText, proposal.Type, proposal.Rationale)

	// Invoke Claude CLI with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), blendTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude",
		"--model", model,
		"--print",
		"--no-session-persistence",
		"--disable-slash-commands",
		"--tools", "",
		"--settings", `{"disableAllHooks": true}`, // prevent user hooks from firing
	)
	cmd.Dir = store.Root() // safe local cwd; prevents CC project discovery from network volumes
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = cleanClaudeEnv()

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("blend timed out after %s", blendTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude CLI failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("claude CLI failed: %w", err)
	}

	return string(out), nil
}

// Commit writes the blended content to the target file and archives the proposal.
func Commit(proposal *pipeline.Proposal, blendedContent string) error {
	target := expandPath(proposal.Target)

	if err := validateTarget(target); err != nil {
		return fmt.Errorf("unsafe target path: %w", err)
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	if err := store.AtomicWrite(target, []byte(blendedContent), 0o644); err != nil {
		return fmt.Errorf("writing target: %w", err)
	}

	return nil
}

// ArchiveOutcome is the typed outcome of a proposal archival.
type ArchiveOutcome string

const (
	OutcomeApproved     ArchiveOutcome = "approved"
	OutcomeRejected     ArchiveOutcome = "rejected"
	OutcomeCulled       ArchiveOutcome = "culled"        // curator rank-cull
	OutcomeAutoRejected ArchiveOutcome = "auto-rejected" // curator already-applied
	OutcomeDeferred     ArchiveOutcome = "deferred"
)

// readArchiveOutcome migrates old free-text archiveReason strings to ArchiveOutcome.
// Returns OutcomeRejected for unrecognised strings (safe default).
func readArchiveOutcome(raw map[string]json.RawMessage) ArchiveOutcome {
	reasonRaw, ok := raw["archiveReason"]
	if !ok {
		return OutcomeRejected // unknown
	}
	var reason string
	json.Unmarshal(reasonRaw, &reason)
	switch {
	case reason == "approved":
		return OutcomeApproved
	case reason == "deferred":
		return OutcomeDeferred
	case strings.HasPrefix(reason, "rejected"):
		return OutcomeRejected
	case strings.HasPrefix(reason, "auto-culled"):
		return OutcomeCulled
	default:
		return OutcomeRejected
	}
}

// Archive moves a proposal to proposals/archived/ with a typed outcome.
// note is an optional human-written reason (empty string for curator calls).
// archiveReason is NOT written; outcome + archivedAt replace it.
func Archive(proposalID string, outcome ArchiveOutcome, note string) error {
	if err := pipeline.ValidateProposalID(proposalID); err != nil {
		return err
	}

	srcDir := filepath.Join(store.Root(), "proposals")
	dstDir := filepath.Join(store.Root(), "proposals", "archived")

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("creating archive dir: %w", err)
	}

	src := filepath.Join(srcDir, proposalID+".json")
	dst := filepath.Join(dstDir, proposalID+".json")

	// Read, annotate with outcome, write to archive.
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading proposal: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing proposal: %w", err)
	}

	outcomeJSON, _ := json.Marshal(string(outcome))
	raw["outcome"] = outcomeJSON

	archivedAtJSON, _ := json.Marshal(time.Now())
	raw["archivedAt"] = archivedAtJSON

	// Do NOT write "archiveReason" — reads use readArchiveOutcome for migration.
	delete(raw, "archiveReason") // remove if present from old data

	annotated, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling archived proposal: %w", err)
	}

	if err := store.AtomicWrite(dst, annotated, 0o644); err != nil {
		return fmt.Errorf("writing archived proposal: %w", err)
	}

	// Remove original.
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("removing original proposal: %w", err)
	}

	return nil
}

func buildBlendPrompt(currentContent, changeText, proposalType, rationale string) string {
	var b strings.Builder

	b.WriteString("You are a precise code editor. Apply the following proposed change to the file content below.\n\n")

	b.WriteString("## Proposal Type\n")
	b.WriteString(proposalType)
	b.WriteString("\n\n")

	b.WriteString("## Rationale\n")
	b.WriteString(rationale)
	b.WriteString("\n\n")

	b.WriteString("## Proposed Change\n```\n")
	b.WriteString(changeText)
	b.WriteString("\n```\n\n")

	if currentContent != "" {
		b.WriteString("## Current File Content\n```\n")
		b.WriteString(currentContent)
		b.WriteString("\n```\n\n")
	}

	b.WriteString("Output ONLY the complete new file content with the change applied. No explanation, no markdown fencing — just the raw file content.")

	return b.String()
}

// validateTarget checks that a resolved target path is safe to read and write.
// Cabrero only modifies markdown files (CLAUDE.md, SKILL.md, etc.), so we
// reject any path that doesn't end in .md or falls outside the user's home directory.
func validateTarget(resolved string) error {
	cleaned := filepath.Clean(resolved)

	// Must be a markdown file.
	if !strings.HasSuffix(strings.ToLower(cleaned), ".md") {
		return fmt.Errorf("target must be a .md file, got: %s", resolved)
	}

	// Reject path traversal: require the target to be inside the user's home directory.
	// Note: strings.Contains(cleaned, "..") is a no-op after filepath.Clean resolves
	// all traversal components. Use a prefix check instead.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	if !strings.HasPrefix(cleaned, home+string(filepath.Separator)) {
		return fmt.Errorf("target outside home directory: %s", resolved)
	}

	return nil
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// cleanClaudeEnv returns os.Environ() with CLAUDECODE stripped and CABRERO_SESSION=1 added.
func cleanClaudeEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		env = append(env, e)
	}
	return append(env, "CABRERO_SESSION=1")
}
