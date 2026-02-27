package parser

import (
	"os"
	"path/filepath"
	"strings"
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

// --- detectRetryAnomalies tests ---

func TestDetectRetryAnomalies_NoAnomalies(t *testing.T) {
	calls := []recentToolCall{
		{toolName: "Read", uuid: "u1", timestamp: "2024-01-01T00:00:00Z", inputKey: "file_a.go"},
		{toolName: "Read", uuid: "u2", timestamp: "2024-01-01T00:00:10Z", inputKey: "file_b.go"},
	}
	anomalies := detectRetryAnomalies(calls)
	if len(anomalies) != 0 {
		t.Errorf("anomalies = %d, want 0", len(anomalies))
	}
}

func TestDetectRetryAnomalies_SingleCall(t *testing.T) {
	calls := []recentToolCall{
		{toolName: "Bash", uuid: "u1", timestamp: "2024-01-01T00:00:00Z", inputKey: "npm test"},
	}
	anomalies := detectRetryAnomalies(calls)
	if len(anomalies) != 0 {
		t.Errorf("anomalies = %d, want 0 for single call", len(anomalies))
	}
}

func TestDetectRetryAnomalies_SameToolSameInput(t *testing.T) {
	calls := []recentToolCall{
		{toolName: "Bash", uuid: "u1", timestamp: "2024-01-01T00:00:00Z", inputKey: "npm test"},
		{toolName: "Bash", uuid: "u2", timestamp: "2024-01-01T00:00:05Z", inputKey: "npm test"},
		{toolName: "Bash", uuid: "u3", timestamp: "2024-01-01T00:00:10Z", inputKey: "npm test"},
	}
	anomalies := detectRetryAnomalies(calls)
	if len(anomalies) == 0 {
		t.Fatal("expected at least one retry anomaly for 3 identical Bash calls")
	}
	if anomalies[0].ToolName != "Bash" {
		t.Errorf("ToolName = %q, want 'Bash'", anomalies[0].ToolName)
	}
	if len(anomalies[0].UUIDs) != 3 {
		t.Errorf("UUIDs count = %d, want 3", len(anomalies[0].UUIDs))
	}
	if anomalies[0].InputSimilarity != "exact" {
		t.Errorf("InputSimilarity = %q, want 'exact'", anomalies[0].InputSimilarity)
	}
}

func TestDetectRetryAnomalies_OutsideTimeWindow(t *testing.T) {
	// Same tool+input but >60s apart — should not be flagged.
	calls := []recentToolCall{
		{toolName: "Bash", uuid: "u1", timestamp: "2024-01-01T00:00:00Z", inputKey: "npm test"},
		{toolName: "Bash", uuid: "u2", timestamp: "2024-01-01T00:02:00Z", inputKey: "npm test"},
	}
	anomalies := detectRetryAnomalies(calls)
	if len(anomalies) != 0 {
		t.Errorf("anomalies = %d, want 0 for calls outside time window", len(anomalies))
	}
}

// --- detectSearchFumbles tests ---

func TestDetectSearchFumbles_SingleCall(t *testing.T) {
	calls := []recentToolCall{
		{toolName: "Grep", uuid: "u1", timestamp: "2024-01-01T00:00:00Z", inputKey: "pattern1"},
	}
	fumbles := detectSearchFumbles(calls)
	if len(fumbles) != 0 {
		t.Errorf("fumbles = %d, want 0 for single call", len(fumbles))
	}
}

func TestDetectSearchFumbles_TwoDistinctInputs(t *testing.T) {
	// Only 2 distinct inputs — threshold is 3.
	calls := []recentToolCall{
		{toolName: "Grep", uuid: "u1", timestamp: "2024-01-01T00:00:00Z", inputKey: "pattern1"},
		{toolName: "Grep", uuid: "u2", timestamp: "2024-01-01T00:00:05Z", inputKey: "pattern2"},
	}
	fumbles := detectSearchFumbles(calls)
	if len(fumbles) != 0 {
		t.Errorf("fumbles = %d, want 0 for only 2 distinct inputs", len(fumbles))
	}
}

func TestDetectSearchFumbles_ThreeDistinctInputs(t *testing.T) {
	// 3 distinct inputs in sequence within 60s — should be a fumble.
	calls := []recentToolCall{
		{toolName: "Grep", uuid: "u1", timestamp: "2024-01-01T00:00:00Z", inputKey: "pattern1"},
		{toolName: "Grep", uuid: "u2", timestamp: "2024-01-01T00:00:05Z", inputKey: "pattern2"},
		{toolName: "Grep", uuid: "u3", timestamp: "2024-01-01T00:00:10Z", inputKey: "pattern3"},
	}
	fumbles := detectSearchFumbles(calls)
	if len(fumbles) == 0 {
		t.Fatal("expected fumble for 3 distinct Grep inputs in sequence")
	}
	if fumbles[0].Type != "search_fumble" {
		t.Errorf("Type = %q, want 'search_fumble'", fumbles[0].Type)
	}
	if fumbles[0].ToolName != "Grep" {
		t.Errorf("ToolName = %q, want 'Grep'", fumbles[0].ToolName)
	}
}

func TestDetectSearchFumbles_NonSearchTool(t *testing.T) {
	// Non-search tools should not trigger fumbles.
	calls := []recentToolCall{
		{toolName: "Read", uuid: "u1", timestamp: "2024-01-01T00:00:00Z", inputKey: "a.go"},
		{toolName: "Read", uuid: "u2", timestamp: "2024-01-01T00:00:05Z", inputKey: "b.go"},
		{toolName: "Read", uuid: "u3", timestamp: "2024-01-01T00:00:10Z", inputKey: "c.go"},
	}
	fumbles := detectSearchFumbles(calls)
	if len(fumbles) != 0 {
		t.Errorf("fumbles = %d, want 0 for non-search tool", len(fumbles))
	}
}

// --- detectBacktracking tests ---

func TestDetectBacktracking_NoBacktrack(t *testing.T) {
	accesses := []fileAccess{
		{filePath: "a.go", toolName: "Read", uuid: "u1", seqIndex: 0},
		{filePath: "b.go", toolName: "Read", uuid: "u2", seqIndex: 1},
		{filePath: "c.go", toolName: "Read", uuid: "u3", seqIndex: 2},
	}
	signals := detectBacktracking(accesses)
	if len(signals) != 0 {
		t.Errorf("backtrack signals = %d, want 0", len(signals))
	}
}

func TestDetectBacktracking_WithBacktrack(t *testing.T) {
	// Access a.go, then 3 other files, then a.go again — should detect backtrack.
	accesses := []fileAccess{
		{filePath: "a.go", toolName: "Read", uuid: "u1", seqIndex: 0},
		{filePath: "b.go", toolName: "Read", uuid: "u2", seqIndex: 1},
		{filePath: "c.go", toolName: "Read", uuid: "u3", seqIndex: 2},
		{filePath: "d.go", toolName: "Read", uuid: "u4", seqIndex: 3},
		{filePath: "a.go", toolName: "Read", uuid: "u5", seqIndex: 4},
	}
	signals := detectBacktracking(accesses)
	if len(signals) == 0 {
		t.Fatal("expected backtrack signal for a.go after 3 intervening files")
	}
	if signals[0].Type != "backtrack" {
		t.Errorf("Type = %q, want 'backtrack'", signals[0].Type)
	}
	if !strings.Contains(signals[0].Detail, "a.go") {
		t.Errorf("Detail = %q, want to mention 'a.go'", signals[0].Detail)
	}
}

func TestDetectBacktracking_TooFewIntervening(t *testing.T) {
	// Access a.go, then 2 other files, then a.go again — threshold is 3.
	accesses := []fileAccess{
		{filePath: "a.go", toolName: "Read", uuid: "u1", seqIndex: 0},
		{filePath: "b.go", toolName: "Read", uuid: "u2", seqIndex: 1},
		{filePath: "c.go", toolName: "Read", uuid: "u3", seqIndex: 2},
		{filePath: "a.go", toolName: "Read", uuid: "u4", seqIndex: 3},
	}
	signals := detectBacktracking(accesses)
	if len(signals) != 0 {
		t.Errorf("backtrack signals = %d, want 0 for only 2 intervening files", len(signals))
	}
}

// --- inferClaudeMdLoaded tests ---

func TestInferClaudeMdLoaded_WithCwd(t *testing.T) {
	cwd := "/home/user/project"
	d := &Digest{
		Shape: Shape{Cwd: &cwd},
	}
	d.inferClaudeMdLoaded()

	if len(d.ClaudeMd.Loaded) == 0 {
		t.Fatal("expected at least one inferred CLAUDE.md path")
	}

	foundUser := false
	foundProject := false
	for _, loaded := range d.ClaudeMd.Loaded {
		if loaded.Source == "user_config" {
			foundUser = true
		}
		if loaded.Source == "project_cwd" {
			foundProject = true
		}
	}
	if !foundUser {
		t.Error("missing user_config CLAUDE.md")
	}
	if !foundProject {
		t.Error("missing project_cwd CLAUDE.md")
	}
}

func TestInferClaudeMdLoaded_NoCwd(t *testing.T) {
	d := &Digest{}
	d.inferClaudeMdLoaded()

	// Should still have user_config but no project_cwd.
	if len(d.ClaudeMd.Loaded) == 0 {
		t.Fatal("expected user_config CLAUDE.md even without cwd")
	}
	for _, loaded := range d.ClaudeMd.Loaded {
		if loaded.Source == "project_cwd" {
			t.Error("should not have project_cwd CLAUDE.md when cwd is nil")
		}
	}
}
