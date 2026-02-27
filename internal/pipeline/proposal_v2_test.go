package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
)

// --- Adapter tests ---

func TestV1ToV2_SkillImprovement(t *testing.T) {
	change := "new content"
	p := &Proposal{
		ID:                   "prop-abc123-1",
		Type:                 "skill_improvement",
		Confidence:           "high",
		Target:               "~/.claude/skills/SKILL.md",
		Change:               &change,
		Rationale:            "improve pattern matching",
		CitedUUIDs:           []string{"uuid-1", "uuid-2"},
		CitedSkillSignals:    []string{"sig-1"},
		CitedClaudeMdSignals: []string{"cmd-1"},
	}

	v2 := V1ToV2(p)

	if v2.Kind != KindArtifactChange {
		t.Errorf("Kind = %q, want %q", v2.Kind, KindArtifactChange)
	}
	if v2.Action != ActionApplyDiff {
		t.Errorf("Action = %q, want %q", v2.Action, ActionApplyDiff)
	}
	if v2.Target.Path != p.Target {
		t.Errorf("Target.Path = %q, want %q", v2.Target.Path, p.Target)
	}
	if v2.Target.ArtifactType != "skill" {
		t.Errorf("Target.ArtifactType = %q, want %q", v2.Target.ArtifactType, "skill")
	}
	if v2.LegacyType != "skill_improvement" {
		t.Errorf("LegacyType = %q, want %q", v2.LegacyType, "skill_improvement")
	}
	if len(v2.Evidence.CitedUUIDs) != 2 {
		t.Errorf("CitedUUIDs len = %d, want 2", len(v2.Evidence.CitedUUIDs))
	}
	if len(v2.Evidence.CitedSkillSignals) != 1 {
		t.Errorf("CitedSkillSignals len = %d, want 1", len(v2.Evidence.CitedSkillSignals))
	}
	if len(v2.Evidence.CitedClaudeMd) != 1 {
		t.Errorf("CitedClaudeMd len = %d, want 1", len(v2.Evidence.CitedClaudeMd))
	}
	if v2.Change == nil || *v2.Change != change {
		t.Errorf("Change = %v, want %q", v2.Change, change)
	}
}

func TestV1ToV2_ClaudeReview(t *testing.T) {
	flagged := "bad entry"
	assessment := "needs fixing"
	p := &Proposal{
		ID:                "prop-abc123-2",
		Type:              "claude_review",
		Confidence:        "medium",
		Target:            "~/project/CLAUDE.md",
		FlaggedEntry:      &flagged,
		AssessmentSummary: &assessment,
		Rationale:         "entry is outdated",
	}

	v2 := V1ToV2(p)

	if v2.Kind != KindArtifactReview {
		t.Errorf("Kind = %q, want %q", v2.Kind, KindArtifactReview)
	}
	if v2.Action != ActionReviewOnly {
		t.Errorf("Action = %q, want %q", v2.Action, ActionReviewOnly)
	}
	if v2.Target.ArtifactType != "claude_md" {
		t.Errorf("Target.ArtifactType = %q, want %q", v2.Target.ArtifactType, "claude_md")
	}
	if v2.FlaggedEntry == nil || *v2.FlaggedEntry != flagged {
		t.Errorf("FlaggedEntry = %v, want %q", v2.FlaggedEntry, flagged)
	}
}

func TestV1ToV2_PromptImprovement(t *testing.T) {
	change := "updated prompt"
	p := &Proposal{
		ID:         "prop-abc123-3",
		Type:       TypePromptImprovement,
		Confidence: "high",
		Target:     "~/.cabrero/prompts/classifier.md",
		Change:     &change,
		Rationale:  "better prompt",
	}

	v2 := V1ToV2(p)

	if v2.Kind != KindPromptChange {
		t.Errorf("Kind = %q, want %q", v2.Kind, KindPromptChange)
	}
	if v2.Action != ActionApplyDiff {
		t.Errorf("Action = %q, want %q", v2.Action, ActionApplyDiff)
	}
	if v2.Target.ArtifactType != "prompt" {
		t.Errorf("Target.ArtifactType = %q, want %q", v2.Target.ArtifactType, "prompt")
	}
}

func TestV1ToV2_PipelineInsight(t *testing.T) {
	p := &Proposal{
		ID:         "prop-abc123-4",
		Type:       TypePipelineInsight,
		Confidence: "medium",
		Rationale:  "observed pattern",
	}

	v2 := V1ToV2(p)

	if v2.Kind != KindSystemInsight {
		t.Errorf("Kind = %q, want %q", v2.Kind, KindSystemInsight)
	}
	if v2.Action != ActionNone {
		t.Errorf("Action = %q, want %q", v2.Action, ActionNone)
	}
}

func TestV1ToV2_SkillScaffold(t *testing.T) {
	change := "new skill content"
	name := "my-skill"
	trigger := "when asked about X"
	p := &Proposal{
		ID:                "prop-abc123-5",
		Type:              "skill_scaffold",
		Confidence:        "high",
		Target:            "~/.claude/skills/my-skill/SKILL.md",
		Change:            &change,
		Rationale:         "creates a new skill",
		ScaffoldSkillName: &name,
		ScaffoldTrigger:   &trigger,
	}

	v2 := V1ToV2(p)

	if v2.Kind != KindArtifactChange {
		t.Errorf("Kind = %q, want %q", v2.Kind, KindArtifactChange)
	}
	if v2.ScaffoldSkillName == nil || *v2.ScaffoldSkillName != name {
		t.Errorf("ScaffoldSkillName = %v, want %q", v2.ScaffoldSkillName, name)
	}
}

func TestV1ToV2_UnknownType(t *testing.T) {
	p := &Proposal{
		ID:         "prop-abc123-6",
		Type:       "future_type",
		Confidence: "low",
		Rationale:  "unknown",
	}

	v2 := V1ToV2(p)

	if v2.Kind != KindSystemInsight {
		t.Errorf("Kind = %q, want fallback %q", v2.Kind, KindSystemInsight)
	}
	if v2.Action != ActionNone {
		t.Errorf("Action = %q, want fallback %q", v2.Action, ActionNone)
	}
	if v2.LegacyType != "future_type" {
		t.Errorf("LegacyType = %q, want %q", v2.LegacyType, "future_type")
	}
}

// --- Round-trip tests ---

func TestV1ToV2_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		v1   Proposal
	}{
		{
			name: "skill_improvement",
			v1:   Proposal{ID: "p-1", Type: "skill_improvement", Confidence: "high", Target: "~/a/SKILL.md", Change: strPtr("c"), Rationale: "r", CitedUUIDs: []string{"u1"}},
		},
		{
			name: "claude_review",
			v1:   Proposal{ID: "p-2", Type: "claude_review", Confidence: "medium", Target: "~/CLAUDE.md", FlaggedEntry: strPtr("f"), AssessmentSummary: strPtr("a"), Rationale: "r"},
		},
		{
			name: "claude_addition",
			v1:   Proposal{ID: "p-3", Type: "claude_addition", Confidence: "high", Target: "~/CLAUDE.md", Change: strPtr("add"), Rationale: "r"},
		},
		{
			name: "skill_scaffold",
			v1:   Proposal{ID: "p-4", Type: "skill_scaffold", Confidence: "high", Target: "~/s/SKILL.md", Change: strPtr("new"), ScaffoldSkillName: strPtr("n"), ScaffoldTrigger: strPtr("t"), Rationale: "r"},
		},
		{
			name: "prompt_improvement",
			v1:   Proposal{ID: "p-5", Type: TypePromptImprovement, Confidence: "high", Target: "~/p/classifier.md", Change: strPtr("fix"), Rationale: "r"},
		},
		{
			name: "pipeline_insight",
			v1:   Proposal{ID: "p-6", Type: TypePipelineInsight, Confidence: "low", Rationale: "r"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v2 := V1ToV2(&tc.v1)
			back := V2ToLegacyView(&v2)

			if back.ID != tc.v1.ID {
				t.Errorf("ID = %q, want %q", back.ID, tc.v1.ID)
			}
			if back.Type != tc.v1.Type {
				t.Errorf("Type = %q, want %q", back.Type, tc.v1.Type)
			}
			if back.Confidence != tc.v1.Confidence {
				t.Errorf("Confidence = %q, want %q", back.Confidence, tc.v1.Confidence)
			}
			if back.Target != tc.v1.Target {
				t.Errorf("Target = %q, want %q", back.Target, tc.v1.Target)
			}
			if back.Rationale != tc.v1.Rationale {
				t.Errorf("Rationale = %q, want %q", back.Rationale, tc.v1.Rationale)
			}
			if !strPtrEqual(back.Change, tc.v1.Change) {
				t.Errorf("Change mismatch")
			}
			if !strPtrEqual(back.FlaggedEntry, tc.v1.FlaggedEntry) {
				t.Errorf("FlaggedEntry mismatch")
			}
			if !strPtrEqual(back.AssessmentSummary, tc.v1.AssessmentSummary) {
				t.Errorf("AssessmentSummary mismatch")
			}
			if !strPtrEqual(back.ScaffoldSkillName, tc.v1.ScaffoldSkillName) {
				t.Errorf("ScaffoldSkillName mismatch")
			}
			if !strPtrEqual(back.ScaffoldTrigger, tc.v1.ScaffoldTrigger) {
				t.Errorf("ScaffoldTrigger mismatch")
			}
		})
	}
}

func TestV2ToLegacyView_WithoutLegacyType(t *testing.T) {
	// Simulate a v2 proposal that was not converted from v1 (no LegacyType).
	cases := []struct {
		name     string
		v2       ProposalV2
		wantType string
	}{
		{
			name:     "artifact_change skill",
			v2:       ProposalV2{Kind: KindArtifactChange, Action: ActionApplyDiff, Target: ProposalV2Target{ArtifactType: "skill"}},
			wantType: "skill_improvement",
		},
		{
			name:     "artifact_change claude_md",
			v2:       ProposalV2{Kind: KindArtifactChange, Action: ActionApplyDiff, Target: ProposalV2Target{ArtifactType: "claude_md"}},
			wantType: "claude_addition",
		},
		{
			name:     "artifact_change scaffold",
			v2:       ProposalV2{Kind: KindArtifactChange, Action: ActionApplyDiff, ScaffoldSkillName: strPtr("s")},
			wantType: "skill_scaffold",
		},
		{
			name:     "artifact_review",
			v2:       ProposalV2{Kind: KindArtifactReview, Action: ActionReviewOnly},
			wantType: "claude_review",
		},
		{
			name:     "prompt_change",
			v2:       ProposalV2{Kind: KindPromptChange, Action: ActionApplyDiff},
			wantType: TypePromptImprovement,
		},
		{
			name:     "system_insight",
			v2:       ProposalV2{Kind: KindSystemInsight, Action: ActionNone},
			wantType: TypePipelineInsight,
		},
		{
			name:     "policy_gate fallback",
			v2:       ProposalV2{Kind: KindPolicyGate, Action: ActionClassifySource},
			wantType: TypePipelineInsight, // fallback
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := V2ToLegacyView(&tc.v2)
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tc.wantType)
			}
		})
	}
}

// --- Dual-read persistence tests ---

func setupV2Store(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
}

func TestWriteProposal_V2Mode_ReadBack(t *testing.T) {
	setupV2Store(t)

	// Switch to v2 writer, defer restore.
	origMode := DefaultWriterMode
	DefaultWriterMode = WriteV2
	defer func() { DefaultWriterMode = origMode }()

	change := "new content"
	p := &Proposal{
		ID:                   "prop-v2test-1",
		Type:                 "skill_improvement",
		Confidence:           "high",
		Target:               "~/.claude/SKILL.md",
		Change:               &change,
		Rationale:            "test v2 write",
		CitedUUIDs:           []string{"u1"},
		CitedSkillSignals:    []string{"s1"},
		CitedClaudeMdSignals: []string{"c1"},
	}

	if err := WriteProposal(p, "session-v2"); err != nil {
		t.Fatalf("WriteProposal: %v", err)
	}

	// Verify on-disk format has schemaVersion.
	data, err := os.ReadFile(filepath.Join(store.Root(), "proposals", "prop-v2test-1.json"))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	var sv int
	json.Unmarshal(raw["schemaVersion"], &sv)
	if sv != 2 {
		t.Errorf("schemaVersion = %d, want 2", sv)
	}

	// Read back via dual-read — should get v1 view.
	pw, err := ReadProposal("prop-v2test-1")
	if err != nil {
		t.Fatalf("ReadProposal: %v", err)
	}
	if pw.Proposal.ID != "prop-v2test-1" {
		t.Errorf("ID = %q, want %q", pw.Proposal.ID, "prop-v2test-1")
	}
	if pw.Proposal.Type != "skill_improvement" {
		t.Errorf("Type = %q, want %q", pw.Proposal.Type, "skill_improvement")
	}
	if pw.SessionID != "session-v2" {
		t.Errorf("SessionID = %q, want %q", pw.SessionID, "session-v2")
	}
	if pw.Proposal.Change == nil || *pw.Proposal.Change != change {
		t.Errorf("Change = %v, want %q", pw.Proposal.Change, change)
	}
}

func TestWriteProposal_LegacyMode_ReadBack(t *testing.T) {
	setupV2Store(t)

	origMode := DefaultWriterMode
	DefaultWriterMode = WriteLegacy
	defer func() { DefaultWriterMode = origMode }()

	p := &Proposal{
		ID:         "prop-legacy-1",
		Type:       "skill_improvement",
		Confidence: "high",
		Rationale:  "test legacy write",
	}

	if err := WriteProposal(p, "session-legacy"); err != nil {
		t.Fatalf("WriteProposal: %v", err)
	}

	// Verify on-disk format has NO schemaVersion.
	data, err := os.ReadFile(filepath.Join(store.Root(), "proposals", "prop-legacy-1.json"))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	if _, ok := raw["schemaVersion"]; ok {
		t.Error("legacy mode should not write schemaVersion")
	}

	// Read back.
	pw, err := ReadProposal("prop-legacy-1")
	if err != nil {
		t.Fatalf("ReadProposal: %v", err)
	}
	if pw.Proposal.Type != "skill_improvement" {
		t.Errorf("Type = %q, want %q", pw.Proposal.Type, "skill_improvement")
	}
}

func TestListProposals_MixedV1V2(t *testing.T) {
	setupV2Store(t)

	proposalsDir := filepath.Join(store.Root(), "proposals")

	// Write v1 proposal directly.
	v1Data := `{"sessionId":"s1","proposal":{"id":"prop-v1-1","type":"skill_improvement","confidence":"high","target":"~/a.md","rationale":"v1"}}`
	os.WriteFile(filepath.Join(proposalsDir, "prop-v1-1.json"), []byte(v1Data), 0o644)

	// Write v2 proposal directly.
	v2Data := `{"schemaVersion":2,"sessionId":"s2","proposal":{"id":"prop-v2-1","kind":"artifact_change","action":"apply_diff","confidence":"high","target":{"path":"~/b.md","artifactType":"skill"},"evidence":{},"rationale":"v2","legacyType":"skill_improvement"}}`
	os.WriteFile(filepath.Join(proposalsDir, "prop-v2-1.json"), []byte(v2Data), 0o644)

	proposals, err := ListProposals()
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}

	if len(proposals) != 2 {
		t.Fatalf("len(proposals) = %d, want 2", len(proposals))
	}

	// Both should be readable as v1 ProposalWithSession.
	byID := make(map[string]ProposalWithSession)
	for _, p := range proposals {
		byID[p.Proposal.ID] = p
	}

	v1p, ok := byID["prop-v1-1"]
	if !ok {
		t.Fatal("missing prop-v1-1")
	}
	if v1p.Proposal.Type != "skill_improvement" {
		t.Errorf("v1 Type = %q, want %q", v1p.Proposal.Type, "skill_improvement")
	}
	if v1p.SessionID != "s1" {
		t.Errorf("v1 SessionID = %q, want %q", v1p.SessionID, "s1")
	}

	v2p, ok := byID["prop-v2-1"]
	if !ok {
		t.Fatal("missing prop-v2-1")
	}
	if v2p.Proposal.Type != "skill_improvement" {
		t.Errorf("v2 Type = %q, want %q", v2p.Proposal.Type, "skill_improvement")
	}
	if v2p.SessionID != "s2" {
		t.Errorf("v2 SessionID = %q, want %q", v2p.SessionID, "s2")
	}
	if v2p.Proposal.Target != "~/b.md" {
		t.Errorf("v2 Target = %q, want %q", v2p.Proposal.Target, "~/b.md")
	}
}

func TestDecodeProposalFile_V2WithAllFields(t *testing.T) {
	data := []byte(`{
		"schemaVersion": 2,
		"sessionId": "sess-full",
		"proposal": {
			"id": "prop-full-1",
			"kind": "artifact_review",
			"action": "review_only",
			"confidence": "medium",
			"target": {"path": "~/CLAUDE.md", "artifactType": "claude_md"},
			"evidence": {
				"citedUuids": ["u1", "u2"],
				"citedSkillSignals": ["s1"],
				"citedClaudeMd": ["c1"]
			},
			"flaggedEntry": "bad entry",
			"assessmentSummary": "needs fixing",
			"rationale": "outdated entry",
			"legacyType": "claude_review"
		}
	}`)

	p, err := decodeProposalFile(data)
	if err != nil {
		t.Fatalf("decodeProposalFile: %v", err)
	}

	if p.SessionID != "sess-full" {
		t.Errorf("SessionID = %q", p.SessionID)
	}
	if p.Proposal.Type != "claude_review" {
		t.Errorf("Type = %q, want %q", p.Proposal.Type, "claude_review")
	}
	if len(p.Proposal.CitedUUIDs) != 2 {
		t.Errorf("CitedUUIDs len = %d, want 2", len(p.Proposal.CitedUUIDs))
	}
	if len(p.Proposal.CitedSkillSignals) != 1 {
		t.Errorf("CitedSkillSignals len = %d, want 1", len(p.Proposal.CitedSkillSignals))
	}
	if len(p.Proposal.CitedClaudeMdSignals) != 1 {
		t.Errorf("CitedClaudeMdSignals len = %d, want 1", len(p.Proposal.CitedClaudeMdSignals))
	}
	if p.Proposal.FlaggedEntry == nil || *p.Proposal.FlaggedEntry != "bad entry" {
		t.Errorf("FlaggedEntry = %v", p.Proposal.FlaggedEntry)
	}
}

func TestDecodeProposalFile_V1Format(t *testing.T) {
	data := []byte(`{"sessionId":"s1","proposal":{"id":"prop-v1-x","type":"skill_improvement","confidence":"high","target":"~/a.md","rationale":"old"}}`)

	p, err := decodeProposalFile(data)
	if err != nil {
		t.Fatalf("decodeProposalFile: %v", err)
	}

	if p.Proposal.ID != "prop-v1-x" {
		t.Errorf("ID = %q", p.Proposal.ID)
	}
	if p.Proposal.Type != "skill_improvement" {
		t.Errorf("Type = %q", p.Proposal.Type)
	}
}

func TestDecodeProposalFile_InvalidJSON(t *testing.T) {
	_, err := decodeProposalFile([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- JSON serialization tests ---

func TestProposalV2_JSONRoundTrip(t *testing.T) {
	change := "test"
	v2 := ProposalV2WithSession{
		SchemaVersion: 2,
		SessionID:     "sess-json",
		Proposal: ProposalV2{
			ID:         "prop-json-1",
			Kind:       KindArtifactChange,
			Action:     ActionApplyDiff,
			Confidence: "high",
			Target:     ProposalV2Target{Path: "~/a.md", ArtifactType: "skill"},
			Evidence:   ProposalV2Evidence{CitedUUIDs: []string{"u1"}},
			Change:     &change,
			Rationale:  "test",
			LegacyType: "skill_improvement",
		},
	}

	data, err := json.Marshal(v2)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded ProposalV2WithSession
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.SchemaVersion != 2 {
		t.Errorf("SchemaVersion = %d, want 2", decoded.SchemaVersion)
	}
	if decoded.Proposal.Kind != KindArtifactChange {
		t.Errorf("Kind = %q, want %q", decoded.Proposal.Kind, KindArtifactChange)
	}
	if decoded.Proposal.LegacyType != "skill_improvement" {
		t.Errorf("LegacyType = %q, want %q", decoded.Proposal.LegacyType, "skill_improvement")
	}
}

// --- Helpers ---

func strPtr(s string) *string {
	return &s
}

func strPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
