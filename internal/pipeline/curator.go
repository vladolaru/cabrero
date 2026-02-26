package pipeline

// CuratorDecision is one per-proposal action from the Curator.
type CuratorDecision struct {
	ProposalID   string `json:"proposalId"`
	Action       string `json:"action"`        // "keep" | "synthesize" | "cull" | "auto-reject"
	Reason       string `json:"reason"`
	SupersededBy string `json:"supersededBy,omitempty"`
}

// CuratorCluster is one synthesized concern group for claude_addition targets.
// The Curator identifies distinct concern clusters within a target's proposals,
// then synthesizes one new proposal per cluster rather than merging all proposals
// into one. This prevents vague entries that cover multiple problems superficially.
type CuratorCluster struct {
	ClusterName string    `json:"clusterName"`
	SourceIDs   []string  `json:"sourceIds"`
	Synthesis   *Proposal `json:"synthesis,omitempty"` // nil if all already applied
}

// CuratorManifest is the Curator's output for a single target group.
type CuratorManifest struct {
	Target    string            `json:"target"`
	Decisions []CuratorDecision `json:"decisions"`
	Clusters  []CuratorCluster  `json:"clusters,omitempty"` // claude_addition only
}

// CheckDecision is one per-proposal result from the Haiku "already applied?" batch check.
type CheckDecision struct {
	ProposalID     string `json:"proposalId"`
	AlreadyApplied bool   `json:"alreadyApplied"`
	Reason         string `json:"reason"`
}
