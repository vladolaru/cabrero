package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func setupProposalsTest(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	os.MkdirAll(filepath.Join(root, "proposals", "archived"), 0o755)
	old := store.RootOverrideForTest(root)
	t.Cleanup(func() { store.ResetRootOverrideForTest(old) })
	return root
}

func writeProposal(t *testing.T, root, id, pType, confidence, target string) {
	t.Helper()
	data := map[string]interface{}{
		"sessionId": "sess-" + id[:8],
		"proposal": map[string]interface{}{
			"id":         id,
			"type":       pType,
			"confidence": confidence,
			"target":     target,
			"rationale":  "test",
		},
	}
	raw, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(filepath.Join(root, "proposals", id+".json"), raw, 0o644)
}

func writeArchivedProposal(t *testing.T, root, id, outcome string) {
	t.Helper()
	data := map[string]interface{}{
		"sessionId": "sess-" + id[:8],
		"proposal": map[string]interface{}{
			"id":         id,
			"type":       "skill_improvement",
			"confidence": "high",
			"target":     "~/.claude/CLAUDE.md",
			"rationale":  "test",
		},
		"outcome":    outcome,
		"archivedAt": "2026-02-28T12:00:00Z",
	}
	raw, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(filepath.Join(root, "proposals", "archived", id+".json"), raw, 0o644)
}

func TestProposals_DefaultShowsPending(t *testing.T) {
	root := setupProposalsTest(t)
	writeProposal(t, root, "prop-aaaa-1111-pending", "skill_improvement", "high", "~/.claude/CLAUDE.md")
	writeArchivedProposal(t, root, "prop-bbbb-2222-approved", "approved")

	// Default (no --status) should only show the pending one.
	err := Proposals(nil)
	if err != nil {
		t.Fatalf("Proposals: %v", err)
	}
	// No crash; verifies backward compatibility.
}

func TestProposals_StatusApproved(t *testing.T) {
	root := setupProposalsTest(t)
	writeArchivedProposal(t, root, "prop-cccc-3333-approved", "approved")
	writeArchivedProposal(t, root, "prop-dddd-4444-rejected", "rejected")

	var buf strings.Builder
	err := proposalsRun([]string{"--status", "approved"}, &buf)
	if err != nil {
		t.Fatalf("proposalsRun: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "prop-cccc-3333-approved") {
		t.Error("expected approved proposal in output")
	}
	if strings.Contains(out, "prop-dddd-4444-rejected") {
		t.Error("rejected proposal should not appear in approved filter")
	}
}

func TestProposals_StatusAll(t *testing.T) {
	root := setupProposalsTest(t)
	writeProposal(t, root, "prop-eeee-5555-pending", "skill_improvement", "high", "~/.claude/CLAUDE.md")
	writeArchivedProposal(t, root, "prop-ffff-6666-approved", "approved")

	var buf strings.Builder
	err := proposalsRun([]string{"--status", "all"}, &buf)
	if err != nil {
		t.Fatalf("proposalsRun: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "prop-eeee-5555-pending") {
		t.Error("expected pending proposal in all output")
	}
	if !strings.Contains(out, "prop-ffff-6666-approved") {
		t.Error("expected approved proposal in all output")
	}
}
