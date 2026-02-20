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

const evaluatorPromptFile = "evaluator-v3.txt"

// BatchSession holds data for one session in an Evaluator batch.
type BatchSession struct {
	SessionID        string
	Digest           *parser.Digest
	ClassifierOutput *ClassifierOutput
}

// RunEvaluator constructs the prompt, invokes the Evaluator via the claude CLI,
// validates the output, and returns the parsed result.
func RunEvaluator(sessionID string, digest *parser.Digest, classifierOutput *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, error) {
	systemPrompt, err := readPromptTemplate(evaluatorPromptFile)
	if err != nil {
		return nil, fmt.Errorf("reading evaluator prompt: %w", err)
	}

	classifierJSON, err := json.MarshalIndent(classifierOutput, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling classifier output: %w", err)
	}

	digestJSON, err := json.MarshalIndent(digest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling digest: %w", err)
	}

	// System prompt goes via --system-prompt; data goes as the -p prompt.
	data := "<classification>\n" + string(classifierJSON) + "\n</classification>" +
		"\n\n<session_digest>\n" + string(digestJSON) + "\n</session_digest>"

	// Inject turn budget into the prompt template.
	systemPrompt = strings.ReplaceAll(systemPrompt, "{{MAX_TURNS}}", strconv.Itoa(cfg.EvaluatorMaxTurns))

	stdout, err := invokeClaude(claudeConfig{
		Model:        "claude-sonnet-4-6",
		SystemPrompt: systemPrompt,
		Effort:       "high",
		Agentic:      true,
		Prompt:       data,
		AllowedTools: "Read,Grep",
		MaxTurns:     cfg.EvaluatorMaxTurns,
		Timeout:      cfg.EvaluatorTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("invoking evaluator: %w", err)
	}

	// Parse JSON output (instructed via system prompt, cleaned defensively).
	output, err := parseEvaluatorOutput(stdout)
	if err != nil {
		return nil, fmt.Errorf("parsing evaluator output: %w\nRaw output:\n%s", err, truncateForLog(stdout, 500))
	}

	output.SessionID = sessionID
	output.PromptVersion = strings.TrimSuffix(evaluatorPromptFile, ".txt")
	output.ClassifierPromptVersion = classifierOutput.PromptVersion

	if err := validateEvaluatorOutput(sessionID, output, classifierOutput); err != nil {
		return nil, err
	}

	return output, nil
}

// RunEvaluatorBatch evaluates multiple sessions in a single Evaluator invocation.
// Each session's Classifier output and digest are included as indexed entries.
// Max turns and timeout scale with batch size, capped at 60 turns and 15 minutes.
func RunEvaluatorBatch(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, error) {
	if len(sessions) == 0 {
		return nil, fmt.Errorf("RunEvaluatorBatch called with zero sessions")
	}

	systemPrompt, err := readPromptTemplate(evaluatorPromptFile)
	if err != nil {
		return nil, fmt.Errorf("reading evaluator prompt: %w", err)
	}

	// Build batch prompt with indexed session data.
	var dataBuilder strings.Builder
	dataBuilder.WriteString(fmt.Sprintf("You are evaluating %d sessions from the same project in a single batch.\n", len(sessions)))
	dataBuilder.WriteString("Generate proposals for ALL sessions that warrant them. Use the standard proposal ID format: prop-{first 8 chars of sessionId}-{index}.\n\n")

	for i, s := range sessions {
		classifierJSON, err := json.MarshalIndent(s.ClassifierOutput, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshalling classifier output for session %d: %w", i, err)
		}

		digestJSON, err := json.MarshalIndent(s.Digest, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshalling digest for session %d: %w", i, err)
		}

		dataBuilder.WriteString(fmt.Sprintf("<session index=\"%d\" id=\"%s\">\n", i+1, s.SessionID))
		dataBuilder.WriteString("<classification>\n")
		dataBuilder.Write(classifierJSON)
		dataBuilder.WriteString("\n</classification>\n\n")
		dataBuilder.WriteString("<session_digest>\n")
		dataBuilder.Write(digestJSON)
		dataBuilder.WriteString("\n</session_digest>\n")
		dataBuilder.WriteString("</session>\n\n")
	}

	// Scale turns and timeout with batch size, with caps.
	maxTurns := cfg.EvaluatorMaxTurns * len(sessions)
	if maxTurns > 60 {
		maxTurns = 60
	}

	timeout := cfg.EvaluatorTimeout * time.Duration(len(sessions))
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
		return nil, fmt.Errorf("invoking evaluator batch: %w", err)
	}

	output, err := parseEvaluatorOutput(stdout)
	if err != nil {
		return nil, fmt.Errorf("parsing evaluator batch output: %w\nRaw output:\n%s", err, truncateForLog(stdout, 500))
	}

	// Set metadata from the first session (batch output covers multiple sessions).
	output.SessionID = sessions[0].SessionID
	output.PromptVersion = strings.TrimSuffix(evaluatorPromptFile, ".txt")
	output.ClassifierPromptVersion = sessions[0].ClassifierOutput.PromptVersion

	// Validate: build combined skill set and UUID set from all sessions.
	if err := validateEvaluatorBatchOutput(sessions, output); err != nil {
		return nil, err
	}

	return output, nil
}

// validateEvaluatorBatchOutput validates proposals against all sessions in the batch.
// UUIDs are checked across all sessions. Skill signals are merged from all Classifier outputs.
func validateEvaluatorBatchOutput(sessions []BatchSession, output *EvaluatorOutput) error {
	// Build combined set of skill names from all Classifier outputs.
	allSkills := make(map[string]bool)
	for _, s := range sessions {
		for _, ss := range s.ClassifierOutput.SkillSignals {
			allSkills[ss.SkillName] = true
		}
	}

	// Resolve UUIDs across all sessions using batch lookup per session.
	allUUIDs := collectEvaluatorUUIDs(output)
	validUUIDs := make(map[string]bool)

	// Collect unresolved UUIDs, then resolve per session with a single-pass scan.
	unresolved := make([]string, 0, len(allUUIDs))
	for _, uuid := range allUUIDs {
		unresolved = append(unresolved, uuid)
	}

	for _, s := range sessions {
		if len(unresolved) == 0 {
			break
		}
		results, err := retrieval.GetTurns(s.SessionID, unresolved)
		if err != nil {
			continue
		}
		var stillUnresolved []string
		for i, uuid := range unresolved {
			if results[i] != nil {
				validUUIDs[uuid] = true
			} else {
				stillUnresolved = append(stillUnresolved, uuid)
			}
		}
		unresolved = stillUnresolved
	}

	for _, uuid := range unresolved {
		fmt.Fprintf(os.Stderr, "  Warning: Evaluator batch cited non-existent UUID: %s\n", uuid)
	}

	return filterAndValidateProposals(output, validUUIDs, allSkills)
}

func parseEvaluatorOutput(raw string) (*EvaluatorOutput, error) {
	cleaned := cleanLLMJSON(raw)

	var output EvaluatorOutput
	if err := json.Unmarshal([]byte(cleaned), &output); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &output, nil
}

// validateEvaluatorOutput checks proposals for valid UUIDs, skill references, uniqueness,
// and filters out low-confidence proposals.
func validateEvaluatorOutput(sessionID string, output *EvaluatorOutput, classifierOutput *ClassifierOutput) error {
	// Build set of skill names from Classifier output for cross-reference.
	classifierSkills := make(map[string]bool)
	for _, ss := range classifierOutput.SkillSignals {
		classifierSkills[ss.SkillName] = true
	}

	// Validate cited UUIDs.
	allUUIDs := collectEvaluatorUUIDs(output)
	validUUIDs := make(map[string]bool)
	for _, uuid := range allUUIDs {
		if validUUIDs[uuid] {
			continue
		}
		_, err := retrieval.GetEntry(sessionID, uuid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: Evaluator cited non-existent UUID: %s\n", uuid)
		} else {
			validUUIDs[uuid] = true
		}
	}

	return filterAndValidateProposals(output, validUUIDs, classifierSkills)
}

// filterAndValidateProposals validates proposals for duplicate IDs, low confidence,
// valid skill_scaffold fields, UUID citations, and skill signal references.
// Shared by both single-session and batch validation paths.
func filterAndValidateProposals(output *EvaluatorOutput, validUUIDs map[string]bool, allSkills map[string]bool) error {
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
			if !allSkills[skill] {
				fmt.Fprintf(os.Stderr, "  Warning: proposal %s cites skill '%s' not in Classifier output\n", p.ID, skill)
			}
		}

		validProposals = append(validProposals, p)
	}

	output.Proposals = validProposals
	return nil
}

func collectEvaluatorUUIDs(output *EvaluatorOutput) []string {
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
