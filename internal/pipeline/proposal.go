// Package pipeline implements the analysis pipeline: pre-parser → Classifier → Evaluator.
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vladolaru/cabrero/internal/store"
)

// ValidateProposalID rejects IDs containing path separators or directory
// traversal components to prevent path traversal attacks.
func ValidateProposalID(id string) error {
	if strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid proposal ID: %q", id)
	}
	return nil
}

// --- Classifier output types ---

// ClassifierOutput is the structured output from the Classifier.
type ClassifierOutput struct {
	Version       int    `json:"version"`
	SessionID     string `json:"sessionId"`
	PromptVersion string `json:"promptVersion"`
	Triage        string `json:"triage"` // "evaluate" or "clean"

	Goal                ClassifierGoal                `json:"goal"`
	ErrorClassification []ClassifierErrorClass        `json:"errorClassification"`
	KeyTurns            []ClassifierKeyTurn           `json:"keyTurns"`
	SkillSignals        []ClassifierSkillSignal       `json:"skillSignals"`
	ClaudeMdSignals     []ClassifierClaudeMdSignal    `json:"claudeMdSignals"`
	PatternAssessments  []ClassifierPatternAssessment `json:"patternAssessments,omitempty"`
}

// ClassifierGoal describes the user's intent in the session.
type ClassifierGoal struct {
	Summary    string `json:"summary"`
	Confidence string `json:"confidence"`
}

// ClassifierErrorClass categorises an error observed in the session.
type ClassifierErrorClass struct {
	Category     string   `json:"category"`
	Description  string   `json:"description"`
	RelatedUUIDs []string `json:"relatedUuids"`
	Severity     string   `json:"severity"`
	Confidence   string   `json:"confidence"`
}

// ClassifierKeyTurn identifies a significant turn in the session.
type ClassifierKeyTurn struct {
	UUID     string `json:"uuid"`
	Reason   string `json:"reason"`
	Category string `json:"category"`
}

// ClassifierSkillSignal assesses a skill's impact in the session.
type ClassifierSkillSignal struct {
	SkillName     string `json:"skillName"`
	InvokedAtUUID string `json:"invokedAtUuid"`
	Assessment    string `json:"assessment"`
	Evidence      string `json:"evidence"`
	Confidence    string `json:"confidence"`
}

// ClassifierClaudeMdSignal assesses a CLAUDE.md file's impact.
type ClassifierClaudeMdSignal struct {
	Path       string `json:"path"`
	Assessment string `json:"assessment"`
	Evidence   string `json:"evidence"`
	Confidence string `json:"confidence"`
}

// ClassifierPatternAssessment assesses a cross-session recurring pattern.
type ClassifierPatternAssessment struct {
	PatternType string `json:"patternType"` // matches RecurringPattern.Type
	ToolName    string `json:"toolName"`
	Assessment  string `json:"assessment"` // "confirmed" | "coincidental" | "resolved"
	Evidence    string `json:"evidence"`
	Confidence  string `json:"confidence"`
}

// --- Evaluator output types ---

// EvaluatorOutput is the structured output from the Evaluator.
type EvaluatorOutput struct {
	Version                 int        `json:"version"`
	SessionID               string     `json:"sessionId"`
	PromptVersion           string     `json:"promptVersion"`
	ClassifierPromptVersion string     `json:"classifierPromptVersion"`
	Proposals               []Proposal `json:"proposals"`
	NoProposalReason        *string    `json:"noProposalReason"`
}

// Proposal type constants.
const (
	TypePromptImprovement = "prompt_improvement" // meta-pipeline: prompt file edits
	TypePipelineInsight   = "pipeline_insight"   // pure-code observation, no change field
)

// Proposal describes a suggested improvement from the Evaluator.
// Types: skill_improvement, claude_review, claude_addition, skill_scaffold,
// prompt_improvement, pipeline_insight.
type Proposal struct {
	ID                   string   `json:"id"`
	Type                 string   `json:"type"`
	Confidence           string   `json:"confidence"`
	Target               string   `json:"target"`
	Change               *string  `json:"change"`
	FlaggedEntry         *string  `json:"flaggedEntry"`
	AssessmentSummary    *string  `json:"assessmentSummary"`
	Rationale            string   `json:"rationale"`
	CitedUUIDs           []string `json:"citedUuids"`
	CitedSkillSignals    []string `json:"citedSkillSignals"`
	CitedClaudeMdSignals []string `json:"citedClaudeMdSignals"`
	ScaffoldSkillName    *string  `json:"scaffoldSkillName,omitempty"`
	ScaffoldTrigger      *string  `json:"scaffoldTrigger,omitempty"`
}

// --- Persistence ---

// schemaVersionProbe is used for fast schema detection without full decode.
type schemaVersionProbe struct {
	SchemaVersion int `json:"schemaVersion"`
}

// decodeProposalFile reads raw JSON and returns a v1 ProposalWithSession.
// It detects v2 format (schemaVersion == 2) and converts via V2ToLegacyView.
func decodeProposalFile(data []byte) (*ProposalWithSession, error) {
	var probe schemaVersionProbe
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, err
	}

	if probe.SchemaVersion == 2 {
		var v2 ProposalV2WithSession
		if err := json.Unmarshal(data, &v2); err != nil {
			return nil, fmt.Errorf("parsing v2 proposal: %w", err)
		}
		legacy := V2ToLegacyView(&v2.Proposal)
		return &ProposalWithSession{
			SessionID: v2.SessionID,
			Proposal:  legacy,
		}, nil
	}

	// v1 format (no schemaVersion or schemaVersion != 2).
	var p ProposalWithSession
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// WriteClassifierOutput writes the Classifier result to disk.
func WriteClassifierOutput(sessionID string, output *ClassifierOutput) error {
	return writeEvaluation(sessionID+"-classifier.json", output)
}

// ReadClassifierOutput reads a previously saved Classifier result.
func ReadClassifierOutput(sessionID string) (*ClassifierOutput, error) {
	path := evaluationPath(sessionID + "-classifier.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out ClassifierOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing classifier output: %w", err)
	}
	return &out, nil
}

// WriteEvaluatorOutput writes the Evaluator result to disk.
func WriteEvaluatorOutput(sessionID string, output *EvaluatorOutput) error {
	return writeEvaluation(sessionID+"-evaluator.json", output)
}

// ReadEvaluatorOutput reads a previously saved Evaluator result.
func ReadEvaluatorOutput(sessionID string) (*EvaluatorOutput, error) {
	path := evaluationPath(sessionID + "-evaluator.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out EvaluatorOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing evaluator output: %w", err)
	}
	return &out, nil
}

// WriteProposal writes a single proposal to the proposals directory.
// The on-disk format depends on DefaultWriterMode.
func WriteProposal(p *Proposal, sessionID string) error {
	if err := ValidateProposalID(p.ID); err != nil {
		return err
	}
	dir := filepath.Join(store.Root(), "proposals")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(dir, p.ID+".json")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("proposal %s already exists", p.ID)
	}

	var data []byte
	var marshalErr error

	switch DefaultWriterMode {
	case WriteV2, WriteBoth:
		v2 := V1ToV2(p)
		wrapped := ProposalV2WithSession{
			SchemaVersion: 2,
			SessionID:     sessionID,
			Proposal:      v2,
		}
		data, marshalErr = json.MarshalIndent(wrapped, "", "  ")
	default: // WriteLegacy
		wrapped := struct {
			SessionID string   `json:"sessionId"`
			Proposal  Proposal `json:"proposal"`
		}{
			SessionID: sessionID,
			Proposal:  *p,
		}
		data, marshalErr = json.MarshalIndent(wrapped, "", "  ")
	}

	if marshalErr != nil {
		return marshalErr
	}

	return store.AtomicWrite(path, data, 0o644)
}

// ListProposals returns all proposal files from the proposals directory.
// Supports both v1 and v2 on-disk formats via dual-read.
func ListProposals() ([]ProposalWithSession, error) {
	dir := filepath.Join(store.Root(), "proposals")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var proposals []ProposalWithSession
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		p, err := decodeProposalFile(data)
		if err != nil {
			continue
		}
		proposals = append(proposals, *p)
	}

	return proposals, nil
}

// ProposalWithSession wraps a proposal with its source session ID.
type ProposalWithSession struct {
	SessionID string   `json:"sessionId"`
	Proposal  Proposal `json:"proposal"`
}

// ReadProposal reads a single proposal by ID.
// Supports both v1 and v2 on-disk formats via dual-read.
func ReadProposal(proposalID string) (*ProposalWithSession, error) {
	if err := ValidateProposalID(proposalID); err != nil {
		return nil, err
	}
	path := filepath.Join(store.Root(), "proposals", proposalID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading proposal: %w", err)
	}
	p, err := decodeProposalFile(data)
	if err != nil {
		return nil, fmt.Errorf("parsing proposal: %w", err)
	}
	return p, nil
}

func writeEvaluation(filename string, v interface{}) error {
	dir := filepath.Join(store.Root(), "evaluations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return store.AtomicWrite(filepath.Join(dir, filename), data, 0o644)
}

func evaluationPath(filename string) string {
	return filepath.Join(store.Root(), "evaluations", filename)
}
