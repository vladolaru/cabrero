package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

func TestValidateTarget_TraversalEscapingHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	// Craft a path that resolves outside home after filepath.Clean.
	escaped := filepath.Clean(filepath.Join(home, "../../etc/hosts.md"))
	if strings.HasPrefix(escaped, home+string(filepath.Separator)) {
		t.Skipf("path %s still inside home — unusual test environment", escaped)
	}
	if err := validateTarget(escaped); err == nil {
		t.Errorf("validateTarget(%q) = nil, want error for path outside home", escaped)
	}
}

func TestValidateTarget_ValidInsideHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	valid := filepath.Join(home, ".claude", "SKILL.md")
	if err := validateTarget(valid); err != nil {
		t.Errorf("validateTarget(%q) = %v, want nil", valid, err)
	}
}

func TestValidateTarget_NotMarkdown(t *testing.T) {
	home, _ := os.UserHomeDir()
	notMd := filepath.Join(home, ".claude", "script.sh")
	if err := validateTarget(notMd); err == nil {
		t.Errorf("validateTarget(%q) = nil, want error for non-.md", notMd)
	}
}

func TestValidateTarget_AtHomeRoot(t *testing.T) {
	// A path exactly equal to home (without a child) must be rejected.
	home, _ := os.UserHomeDir()
	if err := validateTarget(home); err == nil {
		t.Errorf("validateTarget(%q) = nil, want error for home root", home)
	}
}

func TestArchive_WritesOutcomeAndTimestamp(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	// Create proposals dir and a minimal proposal file.
	proposalsDir := filepath.Join(tmp, "proposals")
	archivedDir := filepath.Join(tmp, "proposals", "archived")
	os.MkdirAll(proposalsDir, 0o755)
	os.MkdirAll(archivedDir, 0o755)

	proposalID := "prop-test01-1"
	content := `{"sessionId":"sess-1","proposal":{"id":"prop-test01-1","type":"skill_improvement","confidence":"high","target":"~/.claude/SKILL.md","change":"test","rationale":"test"}}`
	os.WriteFile(filepath.Join(proposalsDir, proposalID+".json"), []byte(content), 0o644)

	before := time.Now()
	if err := Archive(proposalID, OutcomeRejected, "not relevant"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	after := time.Now()

	// Read archived file and verify fields.
	archivedPath := filepath.Join(archivedDir, proposalID+".json")
	data, err := os.ReadFile(archivedPath)
	if err != nil {
		t.Fatalf("archived file not found: %v", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing archived: %v", err)
	}

	var outcome string
	json.Unmarshal(result["outcome"], &outcome)
	if outcome != string(OutcomeRejected) {
		t.Errorf("outcome = %q, want %q", outcome, OutcomeRejected)
	}

	var archivedAt time.Time
	json.Unmarshal(result["archivedAt"], &archivedAt)
	if archivedAt.Before(before) || archivedAt.After(after) {
		t.Errorf("archivedAt %v outside expected range", archivedAt)
	}

	// archiveReason must NOT be written by new code.
	if _, ok := result["archiveReason"]; ok {
		t.Error("archiveReason should not be written by new code")
	}
}

func TestArchive_PersistsNote(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)
	store.Init()

	// Create a proposal to archive.
	p := &pipeline.Proposal{
		ID:         "prop-note-test-1",
		Type:       "skill_improvement",
		Confidence: "high",
		Rationale:  "test proposal",
	}
	if err := pipeline.WriteProposal(p, "test-session"); err != nil {
		t.Fatalf("writing proposal: %v", err)
	}

	// Archive with a note.
	note := "Rejected because the change is too broad"
	if err := Archive("prop-note-test-1", OutcomeRejected, note); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	// Read the archived file and verify note is persisted.
	archivedPath := filepath.Join(store.Root(), "proposals", "archived", "prop-note-test-1.json")
	data, err := os.ReadFile(archivedPath)
	if err != nil {
		t.Fatalf("reading archived file: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parsing archived JSON: %v", err)
	}

	noteRaw, ok := raw["note"]
	if !ok {
		t.Fatal("archived JSON missing 'note' field")
	}
	var got string
	json.Unmarshal(noteRaw, &got)
	if got != note {
		t.Errorf("note = %q, want %q", got, note)
	}
}

func TestArchive_OmitsEmptyNote(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)
	store.Init()

	p := &pipeline.Proposal{
		ID:         "prop-no-note-1",
		Type:       "skill_improvement",
		Confidence: "high",
		Rationale:  "test proposal",
	}
	if err := pipeline.WriteProposal(p, "test-session"); err != nil {
		t.Fatalf("writing proposal: %v", err)
	}

	// Archive with empty note.
	if err := Archive("prop-no-note-1", OutcomeRejected, ""); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	// Read the archived file and verify note is NOT present.
	archivedPath := filepath.Join(store.Root(), "proposals", "archived", "prop-no-note-1.json")
	data, err := os.ReadFile(archivedPath)
	if err != nil {
		t.Fatalf("reading archived file: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parsing archived JSON: %v", err)
	}

	if _, ok := raw["note"]; ok {
		t.Error("archived JSON has 'note' field for empty note — should be omitted")
	}
}

func TestReadArchiveOutcome_MigratesOldReasons(t *testing.T) {
	cases := []struct {
		reason string
		want   ArchiveOutcome
	}{
		{"approved", OutcomeApproved},
		{"rejected", OutcomeRejected},
		{"rejected: not useful", OutcomeRejected},
		{"deferred", OutcomeDeferred},
		{"auto-culled: already applied to target", OutcomeCulled},
		{"auto-culled: synthesized into prop-abc-1", OutcomeCulled},
	}
	for _, c := range cases {
		raw := map[string]json.RawMessage{
			"archiveReason": json.RawMessage(`"` + c.reason + `"`),
		}
		got := readArchiveOutcome(raw)
		if got != c.want {
			t.Errorf("readArchiveOutcome(%q) = %q, want %q", c.reason, got, c.want)
		}
	}
}
