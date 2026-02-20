package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/retrieval"
)

const sonnetPromptFile = "sonnet-evaluator-v2.txt"

// RunSonnet constructs the prompt, invokes the Sonnet evaluator via the claude CLI,
// validates the output, and returns the parsed result.
func RunSonnet(sessionID string, digest *parser.Digest, haikuOutput *HaikuOutput, cfg PipelineConfig) (*SonnetOutput, error) {
	systemPrompt, err := readPromptTemplate(sonnetPromptFile)
	if err != nil {
		return nil, fmt.Errorf("reading sonnet prompt: %w", err)
	}

	haikuJSON, err := json.MarshalIndent(haikuOutput, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling haiku output: %w", err)
	}

	digestJSON, err := json.MarshalIndent(digest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling digest: %w", err)
	}

	// System prompt goes via --system-prompt flag; data goes via stdin.
	data := "<haiku_classification>\n" + string(haikuJSON) + "\n</haiku_classification>" +
		"\n\n<session_digest>\n" + string(digestJSON) + "\n</session_digest>"

	stdout, err := invokeClaude(claudeConfig{
		Model:        "claude-sonnet-4-6",
		SystemPrompt: systemPrompt,
		Effort:       "high",
		Stdin:        strings.NewReader(data),
	})
	if err != nil {
		return nil, fmt.Errorf("invoking sonnet: %w", err)
	}

	// Parse output — --json-schema guarantees valid JSON matching our schema.
	output, err := parseSonnetOutput(stdout)
	if err != nil {
		return nil, fmt.Errorf("parsing sonnet output: %w\nRaw output:\n%s", err, truncateForLog(stdout, 500))
	}

	output.SessionID = sessionID
	output.PromptVersion = strings.TrimSuffix(sonnetPromptFile, ".txt")
	output.HaikuPromptVersion = haikuOutput.PromptVersion

	if err := validateSonnetOutput(sessionID, output, haikuOutput); err != nil {
		return nil, err
	}

	return output, nil
}

func parseSonnetOutput(raw string) (*SonnetOutput, error) {
	cleaned := cleanLLMJSON(raw)

	var output SonnetOutput
	if err := json.Unmarshal([]byte(cleaned), &output); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &output, nil
}

// validateSonnetOutput checks proposals for valid UUIDs, skill references, uniqueness,
// and filters out low-confidence proposals.
func validateSonnetOutput(sessionID string, output *SonnetOutput, haikuOutput *HaikuOutput) error {
	// Build set of skill names from Haiku output for cross-reference.
	haikuSkills := make(map[string]bool)
	for _, ss := range haikuOutput.SkillSignals {
		haikuSkills[ss.SkillName] = true
	}

	// Validate cited UUIDs.
	allUUIDs := collectSonnetUUIDs(output)
	validUUIDs := make(map[string]bool)
	for _, uuid := range allUUIDs {
		if validUUIDs[uuid] {
			continue
		}
		_, err := retrieval.GetEntry(sessionID, uuid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: Sonnet cited non-existent UUID: %s\n", uuid)
		} else {
			validUUIDs[uuid] = true
		}
	}

	// Check proposal ID uniqueness.
	seenIDs := make(map[string]bool)
	var validProposals []Proposal

	for _, p := range output.Proposals {
		// Skip duplicates.
		if seenIDs[p.ID] {
			fmt.Fprintf(os.Stderr, "  Warning: duplicate proposal ID: %s\n", p.ID)
			continue
		}
		seenIDs[p.ID] = true

		// Filter out low-confidence.
		if p.Confidence == "low" {
			fmt.Fprintf(os.Stderr, "  Warning: dropping low-confidence proposal: %s\n", p.ID)
			continue
		}

		// Validate skill_scaffold proposals must have ScaffoldSkillName.
		if p.Type == "skill_scaffold" && (p.ScaffoldSkillName == nil || *p.ScaffoldSkillName == "") {
			fmt.Fprintf(os.Stderr, "  Warning: dropping skill_scaffold proposal without scaffoldSkillName: %s\n", p.ID)
			continue
		}

		// Prune invalid UUID citations.
		var validCited []string
		for _, u := range p.CitedUUIDs {
			if validUUIDs[u] {
				validCited = append(validCited, u)
			}
		}
		p.CitedUUIDs = validCited

		// Validate skill signal references.
		for _, skill := range p.CitedSkillSignals {
			if !haikuSkills[skill] {
				fmt.Fprintf(os.Stderr, "  Warning: proposal %s cites skill '%s' not in Haiku output\n", p.ID, skill)
			}
		}

		validProposals = append(validProposals, p)
	}

	output.Proposals = validProposals
	return nil
}

func collectSonnetUUIDs(output *SonnetOutput) []string {
	seen := make(map[string]bool)
	var uuids []string

	for _, p := range output.Proposals {
		for _, u := range p.CitedUUIDs {
			if u != "" && !seen[u] {
				seen[u] = true
				uuids = append(uuids, u)
			}
		}
	}

	return uuids
}
