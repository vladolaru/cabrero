// Package pipeline implements the analysis pipeline: pre-parser → Haiku → Opus.
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vladolaru/cabrero/internal/store"
)

// --- Haiku classifier output types ---

// HaikuOutput is the structured output from the Haiku classifier.
type HaikuOutput struct {
	Version        int    `json:"version"`
	SessionID      string `json:"sessionId"`
	PromptVersion  string `json:"promptVersion"`
	Triage         string `json:"triage"` // "evaluate" or "clean"

	Goal                HaikuGoal                `json:"goal"`
	ErrorClassification []HaikuErrorClass         `json:"errorClassification"`
	KeyTurns            []HaikuKeyTurn            `json:"keyTurns"`
	SkillSignals        []HaikuSkillSignal        `json:"skillSignals"`
	ClaudeMdSignals     []HaikuClaudeMdSignal     `json:"claudeMdSignals"`
	PatternAssessments  []HaikuPatternAssessment  `json:"patternAssessments,omitempty"`
}

// HaikuGoal describes the user's intent in the session.
type HaikuGoal struct {
	Summary    string `json:"summary"`
	Confidence string `json:"confidence"`
}

// HaikuErrorClass categorises an error observed in the session.
type HaikuErrorClass struct {
	Category     string   `json:"category"`
	Description  string   `json:"description"`
	RelatedUUIDs []string `json:"relatedUuids"`
	Severity     string   `json:"severity"`
	Confidence   string   `json:"confidence"`
}

// HaikuKeyTurn identifies a significant turn in the session.
type HaikuKeyTurn struct {
	UUID     string `json:"uuid"`
	Reason   string `json:"reason"`
	Category string `json:"category"`
}

// HaikuSkillSignal assesses a skill's impact in the session.
type HaikuSkillSignal struct {
	SkillName    string `json:"skillName"`
	InvokedAtUUID string `json:"invokedAtUuid"`
	Assessment   string `json:"assessment"`
	Evidence     string `json:"evidence"`
	Confidence   string `json:"confidence"`
}

// HaikuClaudeMdSignal assesses a CLAUDE.md file's impact.
type HaikuClaudeMdSignal struct {
	Path       string `json:"path"`
	Assessment string `json:"assessment"`
	Evidence    string `json:"evidence"`
	Confidence  string `json:"confidence"`
}

// HaikuPatternAssessment assesses a cross-session recurring pattern.
type HaikuPatternAssessment struct {
	PatternType string `json:"patternType"` // matches RecurringPattern.Type
	ToolName    string `json:"toolName"`
	Assessment  string `json:"assessment"`  // "confirmed" | "coincidental" | "resolved"
	Evidence    string `json:"evidence"`
	Confidence  string `json:"confidence"`
}

// --- Sonnet evaluator output types ---

// SonnetOutput is the structured output from the Sonnet evaluator.
type SonnetOutput struct {
	Version            int        `json:"version"`
	SessionID          string     `json:"sessionId"`
	PromptVersion      string     `json:"promptVersion"`
	HaikuPromptVersion string     `json:"haikuPromptVersion"`
	Proposals          []Proposal `json:"proposals"`
	NoProposalReason   *string    `json:"noProposalReason"`
}

// Proposal describes a suggested improvement from the Sonnet evaluator.
// Types: skill_improvement, claude_review, claude_addition, skill_scaffold.
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

// WriteHaikuOutput writes the Haiku classifier result to disk.
func WriteHaikuOutput(sessionID string, output *HaikuOutput) error {
	return writeEvaluation(sessionID+"-haiku.json", output)
}

// ReadHaikuOutput reads a previously saved Haiku classifier result.
func ReadHaikuOutput(sessionID string) (*HaikuOutput, error) {
	path := evaluationPath(sessionID + "-haiku.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out HaikuOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing haiku output: %w", err)
	}
	return &out, nil
}

// WriteSonnetOutput writes the Sonnet evaluator result to disk.
func WriteSonnetOutput(sessionID string, output *SonnetOutput) error {
	return writeEvaluation(sessionID+"-sonnet.json", output)
}

// ReadSonnetOutput reads a previously saved Sonnet evaluator result.
func ReadSonnetOutput(sessionID string) (*SonnetOutput, error) {
	path := evaluationPath(sessionID + "-sonnet.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out SonnetOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing sonnet output: %w", err)
	}
	return &out, nil
}

// WriteProposal writes a single proposal to the proposals directory.
func WriteProposal(p *Proposal, sessionID string) error {
	dir := filepath.Join(store.Root(), "proposals")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	wrapped := struct {
		SessionID string   `json:"sessionId"`
		Proposal  Proposal `json:"proposal"`
	}{
		SessionID: sessionID,
		Proposal:  *p,
	}

	data, err := json.MarshalIndent(wrapped, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, p.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// ListProposals returns all proposal files from the proposals directory.
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
		var p ProposalWithSession
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		proposals = append(proposals, p)
	}

	return proposals, nil
}

// ProposalWithSession wraps a proposal with its source session ID.
type ProposalWithSession struct {
	SessionID string   `json:"sessionId"`
	Proposal  Proposal `json:"proposal"`
}

// ReadProposal reads a single proposal by ID.
func ReadProposal(proposalID string) (*ProposalWithSession, error) {
	path := filepath.Join(store.Root(), "proposals", proposalID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading proposal: %w", err)
	}
	var p ProposalWithSession
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing proposal: %w", err)
	}
	return &p, nil
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
	return os.WriteFile(filepath.Join(dir, filename), data, 0o644)
}

func evaluationPath(filename string) string {
	return filepath.Join(store.Root(), "evaluations", filename)
}
