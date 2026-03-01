package pipeline

import (
	"encoding/json"
	"fmt"
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

func TestParseCuratorManifest(t *testing.T) {
	raw := `{
      "target": "/Users/foo/.claude/CLAUDE.md",
      "decisions": [
        {"proposalId": "prop-abc-1", "action": "synthesize", "reason": "merged", "supersededBy": "prop-curator-a1b2-1"},
        {"proposalId": "prop-abc-2", "action": "synthesize", "reason": "merged", "supersededBy": "prop-curator-a1b2-1"}
      ],
      "clusters": [
        {
          "clusterName": "Edit precondition failures",
          "sourceIds": ["prop-abc-1", "prop-abc-2"],
          "synthesis": {
            "id": "prop-curator-a1b2-1",
            "type": "claude_addition",
            "confidence": "high",
            "target": "/Users/foo/.claude/CLAUDE.md",
            "change": "Always read a file before editing it.",
            "rationale": "Synthesized from 2 proposals.",
            "citedUuids": []
          }
        }
      ]
    }`

	cleaned := cleanLLMJSON(raw)
	var manifest CuratorManifest
	if err := json.Unmarshal([]byte(cleaned), &manifest); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if manifest.Target != "/Users/foo/.claude/CLAUDE.md" {
		t.Errorf("Target: got %q", manifest.Target)
	}
	if len(manifest.Clusters) != 1 {
		t.Fatalf("Clusters: got %d, want 1", len(manifest.Clusters))
	}
	if manifest.Clusters[0].Synthesis == nil {
		t.Fatal("Synthesis is nil")
	}
	if manifest.Clusters[0].Synthesis.ID != "prop-curator-a1b2-1" {
		t.Errorf("Synthesis.ID: got %q", manifest.Clusters[0].Synthesis.ID)
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

func TestCuratorAllowedToolsAreScoped(t *testing.T) {
	// Verify the curator uses path-scoped tools, not bare "Read,Grep".
	tools := curatorAllowedTools("/home/user/projects/myapp/CLAUDE.md")
	if tools == "Read,Grep" {
		t.Error("curator tools should be path-scoped, not bare Read,Grep")
	}
	if !strings.Contains(tools, "Read(//") {
		t.Errorf("curator tools should contain path-scoped Read, got: %s", tools)
	}
	if !strings.Contains(tools, "Grep(//") {
		t.Errorf("curator tools should contain path-scoped Grep, got: %s", tools)
	}
}

func TestNewProposalTypes_Defined(t *testing.T) {
	// Verify string values match what the design specifies.
	if TypePromptImprovement != "prompt_improvement" {
		t.Errorf("TypePromptImprovement = %q", TypePromptImprovement)
	}
	if TypePipelineInsight != "pipeline_insight" {
		t.Errorf("TypePipelineInsight = %q", TypePipelineInsight)
	}
}

func TestDefaultCuratorChunkSize(t *testing.T) {
	if DefaultCuratorChunkSize < 2 {
		t.Errorf("DefaultCuratorChunkSize = %d, must be >= 2", DefaultCuratorChunkSize)
	}
	if DefaultCuratorChunkSize > 15 {
		t.Errorf("DefaultCuratorChunkSize = %d, should be <= CuratorMaxTurns (15)", DefaultCuratorChunkSize)
	}
}

func TestGroupProposalsByTarget_LargeGroupChunkable(t *testing.T) {
	// Verify that GroupProposalsByTarget can produce groups larger than
	// DefaultCuratorChunkSize, which RunCuratorGroup will chunk.
	proposals := make([]ProposalWithSession, 0, 12)
	for i := 0; i < 12; i++ {
		change := fmt.Sprintf("change %d", i)
		proposals = append(proposals, ProposalWithSession{
			Proposal: Proposal{
				ID:     fmt.Sprintf("prop-%d", i),
				Type:   "claude_addition",
				Target: "/Users/test/.claude/CLAUDE.md",
				Change: &change,
			},
		})
	}

	multi, _ := GroupProposalsByTarget(proposals)
	group, ok := multi["/Users/test/.claude/CLAUDE.md"]
	if !ok {
		t.Fatal("expected multi-target group for CLAUDE.md")
	}
	if len(group) != 12 {
		t.Errorf("group size = %d, want 12", len(group))
	}
	// RunCuratorGroup would split this into 2 chunks of 8+4.
	if len(group) <= DefaultCuratorChunkSize {
		t.Errorf("group size %d should exceed DefaultCuratorChunkSize %d for this test", len(group), DefaultCuratorChunkSize)
	}
}
