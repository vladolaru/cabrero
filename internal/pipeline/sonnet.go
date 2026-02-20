package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/retrieval"
)

const sonnetPromptFile = "sonnet-evaluator-v3.txt"

// BatchSession holds data for one session in a Sonnet batch.
type BatchSession struct {
	SessionID   string
	Digest      *parser.Digest
	HaikuOutput *HaikuOutput
}

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

	// System prompt goes via --system-prompt; data goes as the -p prompt.
	data := "<haiku_classification>\n" + string(haikuJSON) + "\n</haiku_classification>" +
		"\n\n<session_digest>\n" + string(digestJSON) + "\n</session_digest>"

	// Inject turn budget into the prompt template.
	systemPrompt = strings.ReplaceAll(systemPrompt, "{{MAX_TURNS}}", strconv.Itoa(cfg.SonnetMaxTurns))

	stdout, err := invokeClaude(claudeConfig{
		Model:        "claude-sonnet-4-6",
		SystemPrompt: systemPrompt,
		Effort:       "high",
		Agentic:      true,
		Prompt:       data,
		AllowedTools: "Read,Grep",
		MaxTurns:     cfg.SonnetMaxTurns,
		Timeout:      cfg.SonnetTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("invoking sonnet: %w", err)
	}

	// Parse JSON output (instructed via system prompt, cleaned defensively).
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

// RunSonnetBatch evaluates multiple sessions in a single Sonnet invocation.
// Each session's Haiku classification and digest are included as indexed entries.
// Max turns and timeout scale with batch size, capped at 60 turns and 15 minutes.
func RunSonnetBatch(sessions []BatchSession, cfg PipelineConfig) (*SonnetOutput, error) {
	if len(sessions) == 0 {
		return nil, fmt.Errorf("RunSonnetBatch called with zero sessions")
	}

	systemPrompt, err := readPromptTemplate(sonnetPromptFile)
	if err != nil {
		return nil, fmt.Errorf("reading sonnet prompt: %w", err)
	}

	// Build batch prompt with indexed session data.
	var dataBuilder strings.Builder
	dataBuilder.WriteString(fmt.Sprintf("You are evaluating %d sessions from the same project in a single batch.\n", len(sessions)))
	dataBuilder.WriteString("Generate proposals for ALL sessions that warrant them. Use the standard proposal ID format: prop-{first 6 chars of sessionId}-{index}.\n\n")

	for i, s := range sessions {
		haikuJSON, err := json.MarshalIndent(s.HaikuOutput, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshalling haiku output for session %d: %w", i, err)
		}

		digestJSON, err := json.MarshalIndent(s.Digest, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshalling digest for session %d: %w", i, err)
		}

		dataBuilder.WriteString(fmt.Sprintf("<session index=\"%d\" id=\"%s\">\n", i+1, s.SessionID))
		dataBuilder.WriteString("<haiku_classification>\n")
		dataBuilder.Write(haikuJSON)
		dataBuilder.WriteString("\n</haiku_classification>\n\n")
		dataBuilder.WriteString("<session_digest>\n")
		dataBuilder.Write(digestJSON)
		dataBuilder.WriteString("\n</session_digest>\n")
		dataBuilder.WriteString("</session>\n\n")
	}

	// Scale turns and timeout with batch size, with caps.
	maxTurns := cfg.SonnetMaxTurns * len(sessions)
	if maxTurns > 60 {
		maxTurns = 60
	}

	timeout := cfg.SonnetTimeout * time.Duration(len(sessions))
	if timeout > 15*time.Minute {
		timeout = 15 * time.Minute
	}

	// Inject scaled turn budget into the prompt template.
	systemPrompt = strings.ReplaceAll(systemPrompt, "{{MAX_TURNS}}", strconv.Itoa(maxTurns))

	stdout, err := invokeClaude(claudeConfig{
		Model:        "claude-sonnet-4-6",
		SystemPrompt: systemPrompt,
		Effort:       "high",
		Agentic:      true,
		Prompt:       dataBuilder.String(),
		AllowedTools: "Read,Grep",
		MaxTurns:     maxTurns,
		Timeout:      timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("invoking sonnet batch: %w", err)
	}

	output, err := parseSonnetOutput(stdout)
	if err != nil {
		return nil, fmt.Errorf("parsing sonnet batch output: %w\nRaw output:\n%s", err, truncateForLog(stdout, 500))
	}

	// Set metadata from the first session (batch output covers multiple sessions).
	output.SessionID = sessions[0].SessionID
	output.PromptVersion = strings.TrimSuffix(sonnetPromptFile, ".txt")
	output.HaikuPromptVersion = sessions[0].HaikuOutput.PromptVersion

	// Validate: build combined skill set and UUID set from all sessions.
	if err := validateSonnetBatchOutput(sessions, output); err != nil {
		return nil, err
	}

	return output, nil
}

// validateSonnetBatchOutput validates proposals against all sessions in the batch.
// UUIDs are checked against all sessions. Skill signals are merged from all Haiku outputs.
func validateSonnetBatchOutput(sessions []BatchSession, output *SonnetOutput) error {
	// Build combined set of skill names from all Haiku outputs.
	allSkills := make(map[string]bool)
	for _, s := range sessions {
		for _, ss := range s.HaikuOutput.SkillSignals {
			allSkills[ss.SkillName] = true
		}
	}

	// Validate cited UUIDs across all sessions in the batch.
	allUUIDs := collectSonnetUUIDs(output)
	validUUIDs := make(map[string]bool)
	for _, uuid := range allUUIDs {
		if validUUIDs[uuid] {
			continue
		}
		// Try each session until we find the UUID.
		found := false
		for _, s := range sessions {
			_, err := retrieval.GetEntry(s.SessionID, uuid)
			if err == nil {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "  Warning: Sonnet batch cited non-existent UUID: %s\n", uuid)
		} else {
			validUUIDs[uuid] = true
		}
	}

	// Check proposal ID uniqueness and filter.
	seenIDs := make(map[string]bool)
	var validProposals []Proposal

	for _, p := range output.Proposals {
		if seenIDs[p.ID] {
			fmt.Fprintf(os.Stderr, "  Warning: duplicate proposal ID: %s\n", p.ID)
			continue
		}
		seenIDs[p.ID] = true

		if p.Confidence == "low" {
			fmt.Fprintf(os.Stderr, "  Warning: dropping low-confidence proposal: %s\n", p.ID)
			continue
		}

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

		// Validate skill signal references against combined set.
		for _, skill := range p.CitedSkillSignals {
			if !allSkills[skill] {
				fmt.Fprintf(os.Stderr, "  Warning: proposal %s cites skill '%s' not in any Haiku output\n", p.ID, skill)
			}
		}

		validProposals = append(validProposals, p)
	}

	output.Proposals = validProposals
	return nil
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
