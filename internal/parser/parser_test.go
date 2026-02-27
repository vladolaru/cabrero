package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

func setupTestStore(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	old := store.RootOverrideForTest(tmpDir)
	t.Cleanup(func() { store.ResetRootOverrideForTest(old) })
	return tmpDir
}

func TestFinalizeAgents_MultipleAgents_UniqueMapping(t *testing.T) {
	setupTestStore(t)

	sessionID := "test-session-001"
	rawDir := store.RawDir(sessionID)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create agent transcript files
	for _, id := range []string{"agent-aaa", "agent-bbb"} {
		f := filepath.Join(rawDir, id+".jsonl")
		if err := os.WriteFile(f, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	agentSpawns := map[string]*AgentInventoryItem{
		"toolu_01Xxx": {AgentID: "toolu_01Xxx", ParentUUID: "p1", ToolName: "Task"},
		"toolu_01Yyy": {AgentID: "toolu_01Yyy", ParentUUID: "p2", ToolName: "Task"},
	}
	agentResultIDs := map[string]bool{"toolu_01Xxx": true, "toolu_01Yyy": true}
	agentIDCounts := map[string]int{"aaa": 5, "bbb": 3}

	d := &Digest{}
	d.finalizeAgents(sessionID, agentSpawns, agentResultIDs, agentIDCounts)

	if len(d.Agents.Inventory) != 2 {
		t.Fatalf("inventory count = %d, want 2", len(d.Agents.Inventory))
	}

	// Each agent should get a DIFFERENT agentID — this is the bug
	ids := map[string]bool{}
	for _, item := range d.Agents.Inventory {
		ids[item.AgentID] = true
	}
	if len(ids) != 2 {
		t.Errorf("unique agentIDs = %d, want 2 (got %v)", len(ids), d.Agents.Inventory)
	}
}

func TestFinalizeAgents_SingleAgent(t *testing.T) {
	setupTestStore(t)

	sessionID := "test-session-002"
	rawDir := store.RawDir(sessionID)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(rawDir, "agent-aaa.jsonl"), []byte("{}"), 0o644)

	agentSpawns := map[string]*AgentInventoryItem{
		"toolu_01Xxx": {AgentID: "toolu_01Xxx", ParentUUID: "p1", ToolName: "Task"},
	}
	agentResultIDs := map[string]bool{"toolu_01Xxx": true}
	agentIDCounts := map[string]int{"aaa": 5}

	d := &Digest{}
	d.finalizeAgents(sessionID, agentSpawns, agentResultIDs, agentIDCounts)

	if len(d.Agents.Inventory) != 1 {
		t.Fatalf("inventory count = %d, want 1", len(d.Agents.Inventory))
	}
	if d.Agents.Inventory[0].AgentID != "aaa" {
		t.Errorf("agentID = %q, want 'aaa'", d.Agents.Inventory[0].AgentID)
	}
	if d.Agents.Inventory[0].EntryCount != 5 {
		t.Errorf("entryCount = %d, want 5", d.Agents.Inventory[0].EntryCount)
	}
}

func TestFinalizeAgents_MoreSpawnsThanAgentIDs(t *testing.T) {
	setupTestStore(t)

	sessionID := "test-session-003"
	rawDir := store.RawDir(sessionID)
	os.MkdirAll(rawDir, 0o755)

	// 3 spawns but only 2 known agentIDs — third should get "unknown"
	agentSpawns := map[string]*AgentInventoryItem{
		"toolu_01A": {AgentID: "toolu_01A", ParentUUID: "p1", ToolName: "Task"},
		"toolu_01B": {AgentID: "toolu_01B", ParentUUID: "p2", ToolName: "Task"},
		"toolu_01C": {AgentID: "toolu_01C", ParentUUID: "p3", ToolName: "Task"},
	}
	agentResultIDs := map[string]bool{"toolu_01A": true, "toolu_01B": true, "toolu_01C": true}
	agentIDCounts := map[string]int{"aaa": 5, "bbb": 3}

	d := &Digest{}
	d.finalizeAgents(sessionID, agentSpawns, agentResultIDs, agentIDCounts)

	if len(d.Agents.Inventory) != 3 {
		t.Fatalf("inventory count = %d, want 3", len(d.Agents.Inventory))
	}

	// Exactly 2 should have real agentIDs, 1 should be "unknown"
	unknownCount := 0
	for _, item := range d.Agents.Inventory {
		if item.AgentID == "unknown" {
			unknownCount++
		}
	}
	if unknownCount != 1 {
		t.Errorf("unknown agents = %d, want 1", unknownCount)
	}
}
