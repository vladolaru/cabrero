package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vladolaru/cabrero/internal/store"
)

// IsFileTarget returns true if target looks like a filesystem path
// (starts with "/" or "~/" or contains a path separator) rather than
// a source name like "local-environment" or "pirategoat-tools:foo".
func IsFileTarget(target string) bool {
	if target == "" {
		return false
	}
	if strings.HasPrefix(target, "/") || strings.HasPrefix(target, "~/") {
		return true
	}
	// Contains path separator — likely a relative path.
	if strings.Contains(target, string(filepath.Separator)) {
		return true
	}
	return false
}

// CheckItem is one entry in the Haiku batch check prompt.
type CheckItem struct {
	ProposalID         string `json:"proposalId"`
	Target             string `json:"target"`
	CurrentFileContent string `json:"currentFileContent"`
	ProposedChange     string `json:"proposedChange"`
}

// RunCuratorCheck sends all single-proposal file-target proposals to Haiku
// in one non-agentic --print call to check if their changes are already applied.
// Returns a slice of CheckDecision (same length and order as items) and usage.
// proposals must only contain file-target proposals (IsFileTarget == true).
func RunCuratorCheck(proposals []ProposalWithSession, cfg PipelineConfig) ([]CheckDecision, *ClaudeResult, error) {
	if len(proposals) == 0 {
		return nil, nil, nil
	}

	if err := EnsureCuratorPrompts(); err != nil {
		return nil, nil, fmt.Errorf("ensuring curator prompts: %w", err)
	}

	systemPrompt, err := readPromptTemplate(curatorCheckPromptFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading curator check prompt: %w", err)
	}

	// Build check items — read each target file.
	items := make([]CheckItem, 0, len(proposals))
	for _, pw := range proposals {
		p := pw.Proposal
		change := ""
		if p.Change != nil {
			change = *p.Change
		} else if p.FlaggedEntry != nil {
			change = *p.FlaggedEntry
		}
		content := readFileOrEmpty(p.Target)
		items = append(items, CheckItem{
			ProposalID:         p.ID,
			Target:             p.Target,
			CurrentFileContent: content,
			ProposedChange:     change,
		})
	}

	inputJSON, err := json.Marshal(items)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling check items: %w", err)
	}

	cr, err := invokeClaude(claudeConfig{
		Model:        cfg.CuratorCheckModel, // Haiku
		SystemPrompt: systemPrompt,
		Agentic:      false,
		Stdin:        strings.NewReader(string(inputJSON)),
		Timeout:      cfg.CuratorCheckTimeout,
	})
	if err != nil {
		return nil, cr, fmt.Errorf("curator check invocation failed: %w", err)
	}

	cleaned := cleanLLMJSON(cr.Result)
	var decisions []CheckDecision
	if err := json.Unmarshal([]byte(cleaned), &decisions); err != nil {
		return nil, cr, fmt.Errorf("parsing curator check output: %w", err)
	}
	return decisions, cr, nil
}

// readFileOrEmpty reads a file, expanding "~/" prefix, returning "" on error.
func readFileOrEmpty(target string) string {
	path := target
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

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

// RunCuratorGroup invokes an agentic Sonnet Curator session for a single target group.
// proposals must all target the same file.
// Returns the CuratorManifest, LLM usage, and any error.
func RunCuratorGroup(target string, proposals []ProposalWithSession, cfg PipelineConfig) (*CuratorManifest, *ClaudeResult, error) {
	if len(proposals) == 0 {
		return nil, nil, nil
	}

	if err := EnsureCuratorPrompts(); err != nil {
		return nil, nil, fmt.Errorf("ensuring curator prompts: %w", err)
	}

	systemPrompt, err := readPromptTemplate(curatorPromptFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading curator prompt: %w", err)
	}
	systemPrompt = strings.ReplaceAll(systemPrompt, "{{MAX_TURNS}}", fmt.Sprintf("%d", cfg.CuratorMaxTurns))

	// Serialize proposals as the user prompt.
	proposalData, err := json.MarshalIndent(proposals, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling proposals: %w", err)
	}
	userPrompt := fmt.Sprintf("Target: %s\n\nProposals:\n%s", target, string(proposalData))

	cr, err := invokeClaude(claudeConfig{
		Model:          cfg.CuratorModel,
		SystemPrompt:   systemPrompt,
		Agentic:        true,
		Prompt:         userPrompt,
		AllowedTools:   curatorAllowedTools(target),
		MaxTurns:       cfg.CuratorMaxTurns,
		Timeout:        cfg.CuratorTimeout,
		Logger:         cfg.Logger,
		Debug:          cfg.Debug,
	})
	if err != nil {
		return nil, cr, fmt.Errorf("curator invocation for %s: %w", target, err)
	}

	cleaned := cleanLLMJSON(cr.Result)
	var manifest CuratorManifest
	if err := json.Unmarshal([]byte(cleaned), &manifest); err != nil {
		return nil, cr, fmt.Errorf("parsing curator manifest for %s: %w", target, err)
	}
	manifest.Target = target // ensure target is set even if LLM omitted it
	return &manifest, cr, nil
}

// curatorAllowedTools builds a path-scoped --allowedTools value for the curator.
// Scopes access to the target file's directory and ~/.cabrero.
func curatorAllowedTools(target string) string {
	cabreroRoot := store.Root()
	paths := []string{
		fmt.Sprintf("Read(//%s/**)", cabreroRoot),
		fmt.Sprintf("Grep(//%s/**)", cabreroRoot),
	}

	// Add the target file's parent directory.
	if target != "" {
		targetDir := filepath.Dir(target)
		if targetDir != "" && targetDir != "." {
			paths = append(paths,
				fmt.Sprintf("Read(//%s/**)", targetDir),
				fmt.Sprintf("Grep(//%s/**)", targetDir),
			)
		}
	}

	return strings.Join(paths, ",")
}
