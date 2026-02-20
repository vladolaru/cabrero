// Package apply implements the blend/commit/archive workflow for proposals.
// Used by both the CLI (cabrero approve) and the TUI review flow.
package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Blend invokes Claude to blend the proposed change into the target file.
// Returns a before/after diff string suitable for human review.
func Blend(proposal *pipeline.Proposal, sessionID string) (string, error) {
	if proposal.Target == "" {
		return "", fmt.Errorf("proposal has no target file")
	}

	target := expandPath(proposal.Target)

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

	// Invoke Claude CLI.
	cmd := exec.Command("claude",
		"--model", "claude-sonnet-4-6",
		"--print",
	)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = append(os.Environ(), "CABRERO_SESSION=1")

	out, err := cmd.Output()
	if err != nil {
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

	// Ensure parent directory exists.
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	if err := os.WriteFile(target, []byte(blendedContent), 0o644); err != nil {
		return fmt.Errorf("writing target: %w", err)
	}

	return nil
}

// Archive moves a proposal from proposals/ to proposals/archived/.
func Archive(proposalID string, reason string) error {
	srcDir := filepath.Join(store.Root(), "proposals")
	dstDir := filepath.Join(store.Root(), "proposals", "archived")

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("creating archive dir: %w", err)
	}

	src := filepath.Join(srcDir, proposalID+".json")
	dst := filepath.Join(dstDir, proposalID+".json")

	// Read, annotate with reason, write to archive.
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading proposal: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing proposal: %w", err)
	}

	if reason != "" {
		reasonJSON, _ := json.Marshal(reason)
		raw["archiveReason"] = reasonJSON
	}

	annotated, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling archived proposal: %w", err)
	}

	if err := os.WriteFile(dst, annotated, 0o644); err != nil {
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

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
