package pipeline

// Proposal Taxonomy v2: normalized envelope with explicit semantics.
//
// The v2 schema introduces structured kind/action/target/evidence fields
// instead of the v1 flat type string. During rollout, v1 and v2 co-exist
// via dual-read: readers try v2 first, fall back to v1. Writers default
// to legacy (v1) and flip to v2-only after a release cycle.

// WriterMode controls whether proposals are persisted in v1, v2, or both formats.
type WriterMode int

const (
	// WriteLegacy writes v1 format only (default during initial rollout).
	WriteLegacy WriterMode = iota
	// WriteV2 writes v2 format only.
	WriteV2
)

// DefaultWriterMode is the active writer mode. Change this to advance the
// migration. During initial rollout this stays WriteLegacy.
var DefaultWriterMode = WriteLegacy

// --- v2 Kind constants ---

// ProposalKind classifies the nature of a proposal.
type ProposalKind string

const (
	KindArtifactChange ProposalKind = "artifact_change"
	KindArtifactReview ProposalKind = "artifact_review"
	KindPolicyGate     ProposalKind = "policy_gate"
	KindSystemInsight  ProposalKind = "system_insight"
	KindPromptChange   ProposalKind = "prompt_change"
)

// --- v2 Action constants ---

// ProposalAction describes what the proposal requests.
type ProposalAction string

const (
	ActionApplyDiff      ProposalAction = "apply_diff"
	ActionReviewOnly     ProposalAction = "review_only"
	ActionClassifySource ProposalAction = "classify_source"
	ActionNone           ProposalAction = "none"
)

// --- v2 structured types ---

// ProposalV2Target identifies what the proposal targets.
type ProposalV2Target struct {
	Path         string `json:"path,omitempty"`         // file path (e.g. ~/path/to/SKILL.md)
	SourceName   string `json:"sourceName,omitempty"`   // source identity name
	SourceOrigin string `json:"sourceOrigin,omitempty"` // "project", "global", "plugin"
	ArtifactType string `json:"artifactType,omitempty"` // "skill", "claude_md", "prompt"
}

// ProposalV2Evidence consolidates citation references.
type ProposalV2Evidence struct {
	CitedUUIDs        []string `json:"citedUuids,omitempty"`
	CitedSkillSignals []string `json:"citedSkillSignals,omitempty"`
	CitedClaudeMd     []string `json:"citedClaudeMd,omitempty"`
}

// ProposalV2 is the v2 envelope for a single proposal.
type ProposalV2 struct {
	ID         string         `json:"id"`
	Kind       ProposalKind   `json:"kind"`
	Action     ProposalAction `json:"action"`
	Confidence string         `json:"confidence"`
	Target     ProposalV2Target   `json:"target"`
	Evidence   ProposalV2Evidence `json:"evidence"`

	// Content fields (depend on kind/action).
	Change            *string `json:"change,omitempty"`
	FlaggedEntry      *string `json:"flaggedEntry,omitempty"`
	AssessmentSummary *string `json:"assessmentSummary,omitempty"`
	Rationale         string  `json:"rationale"`

	// Scaffold-specific.
	ScaffoldSkillName *string `json:"scaffoldSkillName,omitempty"`
	ScaffoldTrigger   *string `json:"scaffoldTrigger,omitempty"`

	// LegacyType preserves the v1 type for lossless round-trip.
	LegacyType string `json:"legacyType,omitempty"`
}

// ProposalV2WithSession is the v2 on-disk wrapper (replaces ProposalWithSession).
type ProposalV2WithSession struct {
	SchemaVersion int        `json:"schemaVersion"` // always 2
	SessionID     string     `json:"sessionId"`
	Proposal      ProposalV2 `json:"proposal"`
}

// --- v1 → v2 adapter ---

// v1TypeToKind maps legacy proposal types to v2 kinds.
var v1TypeToKind = map[string]ProposalKind{
	"skill_improvement":  KindArtifactChange,
	"claude_addition":    KindArtifactChange,
	"skill_scaffold":     KindArtifactChange,
	"claude_review":      KindArtifactReview,
	TypePromptImprovement: KindPromptChange,
	TypePipelineInsight:   KindSystemInsight,
}

// v1TypeToAction maps legacy proposal types to v2 actions.
var v1TypeToAction = map[string]ProposalAction{
	"skill_improvement":  ActionApplyDiff,
	"claude_addition":    ActionApplyDiff,
	"skill_scaffold":     ActionApplyDiff,
	"claude_review":      ActionReviewOnly,
	TypePromptImprovement: ActionApplyDiff,
	TypePipelineInsight:   ActionNone,
}

// v1TypeToArtifact maps legacy proposal types to artifact type labels.
var v1TypeToArtifact = map[string]string{
	"skill_improvement":  "skill",
	"claude_addition":    "claude_md",
	"claude_review":      "claude_md",
	"skill_scaffold":     "skill",
	TypePromptImprovement: "prompt",
	TypePipelineInsight:   "",
}

// V1ToV2 converts a v1 Proposal to a v2 ProposalV2.
func V1ToV2(p *Proposal) ProposalV2 {
	kind, ok := v1TypeToKind[p.Type]
	if !ok {
		kind = KindSystemInsight // safe fallback for unknown types
	}
	action, ok := v1TypeToAction[p.Type]
	if !ok {
		action = ActionNone
	}
	artifact := v1TypeToArtifact[p.Type]

	return ProposalV2{
		ID:         p.ID,
		Kind:       kind,
		Action:     action,
		Confidence: p.Confidence,
		Target: ProposalV2Target{
			Path:         p.Target,
			ArtifactType: artifact,
		},
		Evidence: ProposalV2Evidence{
			CitedUUIDs:        p.CitedUUIDs,
			CitedSkillSignals: p.CitedSkillSignals,
			CitedClaudeMd:     p.CitedClaudeMdSignals,
		},
		Change:            p.Change,
		FlaggedEntry:      p.FlaggedEntry,
		AssessmentSummary: p.AssessmentSummary,
		Rationale:         p.Rationale,
		ScaffoldSkillName: p.ScaffoldSkillName,
		ScaffoldTrigger:   p.ScaffoldTrigger,
		LegacyType:        p.Type,
	}
}

// --- v2 → v1 adapter ---

// v2KindActionToType maps (kind, action) back to a legacy type string.
// Falls back to LegacyType when set, then to heuristics.
var v2KindActionToType = map[ProposalKind]map[ProposalAction]string{
	KindArtifactChange: {
		ActionApplyDiff: "skill_improvement", // default; scaffold overridden below
	},
	KindArtifactReview: {
		ActionReviewOnly: "claude_review",
	},
	KindPromptChange: {
		ActionApplyDiff: TypePromptImprovement,
	},
	KindSystemInsight: {
		ActionNone: TypePipelineInsight,
	},
}

// V2ToLegacyView converts a v2 ProposalV2 back to a v1 Proposal for use
// in existing UI and CLI paths during the transition period.
func V2ToLegacyView(p *ProposalV2) Proposal {
	typ := resolveLegacyType(p)

	return Proposal{
		ID:                   p.ID,
		Type:                 typ,
		Confidence:           p.Confidence,
		Target:               p.Target.Path,
		Change:               p.Change,
		FlaggedEntry:         p.FlaggedEntry,
		AssessmentSummary:    p.AssessmentSummary,
		Rationale:            p.Rationale,
		CitedUUIDs:           p.Evidence.CitedUUIDs,
		CitedSkillSignals:    p.Evidence.CitedSkillSignals,
		CitedClaudeMdSignals: p.Evidence.CitedClaudeMd,
		ScaffoldSkillName:    p.ScaffoldSkillName,
		ScaffoldTrigger:      p.ScaffoldTrigger,
	}
}

// resolveLegacyType determines the best v1 type string for a v2 proposal.
func resolveLegacyType(p *ProposalV2) string {
	// Prefer preserved legacy type for perfect round-trip.
	if p.LegacyType != "" {
		return p.LegacyType
	}

	// Scaffold heuristic: if artifact is skill and scaffold fields present.
	if p.Kind == KindArtifactChange && p.ScaffoldSkillName != nil {
		return "skill_scaffold"
	}

	// Claude_md artifact with apply_diff → claude_addition.
	if p.Kind == KindArtifactChange && p.Target.ArtifactType == "claude_md" {
		return "claude_addition"
	}

	// Look up from kind+action table.
	if actions, ok := v2KindActionToType[p.Kind]; ok {
		if typ, ok := actions[p.Action]; ok {
			return typ
		}
	}

	// Fallback: use pipeline_insight as the safest default.
	return TypePipelineInsight
}
