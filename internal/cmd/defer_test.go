package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func setupDeferTest(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	os.MkdirAll(filepath.Join(root, "proposals", "archived"), 0o755)
	old := store.RootOverrideForTest(root)
	t.Cleanup(func() { store.ResetRootOverrideForTest(old) })
	return root
}

func TestDefer_ArchivesAsDeferred(t *testing.T) {
	root := setupDeferTest(t)

	proposalID := "prop-defer-test-1234"
	data := map[string]interface{}{
		"sessionId": "sess-00001111",
		"proposal": map[string]interface{}{
			"id":         proposalID,
			"type":       "skill_improvement",
			"confidence": "medium",
			"target":     "~/.claude/CLAUDE.md",
			"rationale":  "test",
		},
	}
	raw, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(filepath.Join(root, "proposals", proposalID+".json"), raw, 0o644)

	err := Defer([]string{"--yes", proposalID})
	if err != nil {
		t.Fatalf("Defer: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "proposals", proposalID+".json")); !os.IsNotExist(err) {
		t.Error("pending proposal file should have been removed")
	}

	archivedData, err := os.ReadFile(filepath.Join(root, "proposals", "archived", proposalID+".json"))
	if err != nil {
		t.Fatalf("reading archived proposal: %v", err)
	}
	var archived map[string]json.RawMessage
	json.Unmarshal(archivedData, &archived)
	var outcome string
	json.Unmarshal(archived["outcome"], &outcome)
	if outcome != "deferred" {
		t.Errorf("outcome = %q, want deferred", outcome)
	}
}

func TestDefer_MissingProposalID(t *testing.T) {
	setupDeferTest(t)
	err := Defer(nil)
	if err == nil {
		t.Error("expected error for missing proposal ID")
	}
}
