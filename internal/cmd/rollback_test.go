package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/store"
)

func setupRollbackTest(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".cabrero")
	os.MkdirAll(root, 0o755)
	old := store.RootOverrideForTest(root)
	t.Cleanup(func() { store.ResetRootOverrideForTest(old) })
	return root
}

func TestRollback_RestoresContent(t *testing.T) {
	root := setupRollbackTest(t)
	_ = root

	targetFile := filepath.Join(t.TempDir(), "test-skill.md")
	os.WriteFile(targetFile, []byte("new content after apply"), 0o644)

	entry := fitness.ChangeEntry{
		ID:              "change-001",
		SourceName:      "test-skill",
		ProposalID:      "prop-001",
		Description:     "test change",
		Timestamp:       time.Now(),
		Status:          "approved",
		PreviousContent: "original content before apply",
		FilePath:        targetFile,
	}
	if err := store.AppendChange(entry); err != nil {
		t.Fatalf("AppendChange: %v", err)
	}

	err := RollbackCmd([]string{"--yes", "change-001"})
	if err != nil {
		t.Fatalf("RollbackCmd: %v", err)
	}

	data, _ := os.ReadFile(targetFile)
	if string(data) != "original content before apply" {
		t.Errorf("file content = %q, want original content", string(data))
	}
}

func TestRollback_MissingChangeID(t *testing.T) {
	setupRollbackTest(t)
	err := RollbackCmd(nil)
	if err == nil {
		t.Error("expected error for missing change ID")
	}
}

func TestRollback_UnknownChangeID(t *testing.T) {
	setupRollbackTest(t)
	err := RollbackCmd([]string{"--yes", "nonexistent-id"})
	if err == nil {
		t.Error("expected error for unknown change ID")
	}
}
