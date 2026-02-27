package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

func TestPerformMetaRun_SkipsWhenBelowThreshold(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	logPath := filepath.Join(tmp, "test.log")
	log, err := NewLogger(logPath, 0)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer log.Close()

	d := &Daemon{
		config: DefaultConfig(),
		log:    log,
	}
	d.config.Pipeline.MetaMinSamples = 5
	d.config.Pipeline.MetaRejectionRateThreshold = 0.30
	d.config.Pipeline.Logger = &daemonPipelineLogger{log: log}

	// Ensure required dirs exist so store operations don't error.
	os.MkdirAll(filepath.Join(tmp, "proposals", "archived"), 0o755)

	// ComputePipelineMetrics will find no history → NaN FPR, empty versions.
	// Should log one line and return without error or LLM call.
	d.performMetaRun(context.Background()) // must not panic or call LLM

	// If we get here without panicking, the test passes.
	// Verify a log was written.
	log.Close()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected log output from performMetaRun")
	}
	_ = pipeline.DefaultPipelineConfig() // exercise DefaultPipelineConfig
}

func TestMetaCooldownActive_PendingBlocks(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	// Create proposals dir and write a recent pending meta proposal.
	proposalsDir := filepath.Join(tmp, "proposals")
	os.MkdirAll(proposalsDir, 0o755)

	recentTS := time.Now().Unix()
	propID := fmt.Sprintf("prop-meta-%d-1", recentTS)
	writeProposalFile(t, proposalsDir, propID, "prompt_improvement", "evaluator-v3")

	if !metaCooldownActive("evaluator-v3", 7) {
		t.Error("expected cooldown active for pending recent meta proposal")
	}
}

func TestMetaCooldownActive_ArchivedRecentBlocks(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	// Create dirs.
	os.MkdirAll(filepath.Join(tmp, "proposals"), 0o755)
	archivedDir := filepath.Join(tmp, "proposals", "archived")
	os.MkdirAll(archivedDir, 0o755)

	// Write a recently archived meta proposal.
	recentTS := time.Now().Unix()
	propID := fmt.Sprintf("prop-meta-%d-1", recentTS)
	archivedAt := time.Now().Add(-1 * time.Hour)
	writeArchivedProposalFile(t, archivedDir, propID, "prompt_improvement", "evaluator-v3", "approved", archivedAt)

	if !metaCooldownActive("evaluator-v3", 7) {
		t.Error("expected cooldown active for recently archived meta proposal")
	}
}

func TestMetaCooldownActive_ArchivedOldDoesNotBlock(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	// Create dirs.
	os.MkdirAll(filepath.Join(tmp, "proposals"), 0o755)
	archivedDir := filepath.Join(tmp, "proposals", "archived")
	os.MkdirAll(archivedDir, 0o755)

	// Write an old archived meta proposal (30 days ago).
	oldTS := time.Now().AddDate(0, 0, -30).Unix()
	propID := fmt.Sprintf("prop-meta-%d-1", oldTS)
	archivedAt := time.Now().AddDate(0, 0, -30)
	writeArchivedProposalFile(t, archivedDir, propID, "prompt_improvement", "evaluator-v3", "rejected", archivedAt)

	if metaCooldownActive("evaluator-v3", 7) {
		t.Error("expected cooldown NOT active for old archived proposal")
	}
}

func TestMetaCooldownActive_NeitherPendingNorArchived(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	os.MkdirAll(filepath.Join(tmp, "proposals"), 0o755)
	os.MkdirAll(filepath.Join(tmp, "proposals", "archived"), 0o755)

	if metaCooldownActive("evaluator-v3", 7) {
		t.Error("expected cooldown NOT active with no proposals at all")
	}
}

// --- Test helpers ---

func writeProposalFile(t *testing.T, dir, propID, propType, target string) {
	t.Helper()
	data, _ := json.Marshal(map[string]interface{}{
		"sessionId": "meta",
		"proposal": map[string]interface{}{
			"id":         propID,
			"type":       propType,
			"target":     target,
			"rationale":  target,
			"confidence": "medium",
		},
	})
	if err := os.WriteFile(filepath.Join(dir, propID+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeArchivedProposalFile(t *testing.T, dir, propID, propType, target, outcome string, archivedAt time.Time) {
	t.Helper()
	data, _ := json.Marshal(map[string]interface{}{
		"sessionId": "meta",
		"proposal": map[string]interface{}{
			"id":         propID,
			"type":       propType,
			"target":     target,
			"rationale":  target,
			"confidence": "medium",
		},
		"outcome":    outcome,
		"archivedAt": archivedAt,
	})
	if err := os.WriteFile(filepath.Join(dir, propID+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
