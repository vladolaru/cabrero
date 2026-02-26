package pipeline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultPipelineConfigHasCuratorFields(t *testing.T) {
	cfg := DefaultPipelineConfig()
	if cfg.CuratorModel == "" {
		t.Error("CuratorModel should not be empty")
	}
	if cfg.CuratorTimeout == 0 {
		t.Error("CuratorTimeout should not be zero")
	}
	if cfg.CuratorMaxTurns == 0 {
		t.Error("CuratorMaxTurns should not be zero")
	}
}

func TestCuratorDecisionRoundtrip(t *testing.T) {
	d := CuratorDecision{
		ProposalID:   "prop-abc123-1",
		Action:       "cull",
		Reason:       "superseded by prop-abc123-2",
		SupersededBy: "prop-abc123-2",
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var got CuratorDecision
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != d {
		t.Errorf("got %+v, want %+v", got, d)
	}
}

func TestCuratorManifestRoundtrip(t *testing.T) {
	change := "Add entry: always verify X before Y."
	m := CuratorManifest{
		Target: "/Users/test/.claude/CLAUDE.md",
		Decisions: []CuratorDecision{
			{ProposalID: "prop-abc-1", Action: "synthesize", Reason: "merged into cluster", SupersededBy: "prop-curator-1"},
			{ProposalID: "prop-abc-2", Action: "synthesize", Reason: "merged into cluster", SupersededBy: "prop-curator-1"},
		},
		Clusters: []CuratorCluster{
			{
				ClusterName: "Edit precondition failures",
				SourceIDs:   []string{"prop-abc-1", "prop-abc-2"},
				Synthesis: &Proposal{
					ID:         "prop-curator-1",
					Type:       "claude_addition",
					Confidence: "high",
					Target:     "/Users/test/.claude/CLAUDE.md",
					Change:     &change,
					Rationale:  "Synthesized from 2 proposals.",
				},
			},
		},
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var got CuratorManifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Target != m.Target {
		t.Errorf("Target: got %q, want %q", got.Target, m.Target)
	}
	if len(got.Clusters) != 1 {
		t.Fatalf("Clusters: got %d, want 1", len(got.Clusters))
	}
	if got.Clusters[0].Synthesis == nil {
		t.Fatal("Clusters[0].Synthesis is nil")
	}
}

func TestCheckDecisionRoundtrip(t *testing.T) {
	d := CheckDecision{ProposalID: "prop-abc-1", AlreadyApplied: true, Reason: "entry already present in file"}
	data, _ := json.Marshal(d)
	var got CheckDecision
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != d {
		t.Errorf("got %+v, want %+v", got, d)
	}
}

func TestIsFileTarget(t *testing.T) {
	cases := []struct {
		target string
		want   bool
	}{
		{"/Users/foo/.claude/CLAUDE.md", true},
		{"~/claude/skills/foo.md", true},
		{"local-environment", false},
		{"write-moltres-snap", false},
		{"pirategoat-tools:ingest-code-review", false},
		{"", false},
	}
	for _, c := range cases {
		got := IsFileTarget(c.target)
		if got != c.want {
			t.Errorf("IsFileTarget(%q) = %v, want %v", c.target, got, c.want)
		}
	}
}

func TestCleanLLMJSONArray(t *testing.T) {
	input := "```json\n[{\"proposalId\": \"p1\", \"alreadyApplied\": false}]\n```"
	got := cleanLLMJSON(input)
	if !strings.HasPrefix(got, "[") {
		t.Errorf("expected array, got: %s", got)
	}
	var out []CheckDecision
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Errorf("unmarshal failed: %v", err)
	}
}
