package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/retrieval"
	"github.com/vladolaru/cabrero/internal/store"
)

const evaluatorPromptFile = "evaluator-v3.txt"

// DefaultEvaluatorModel is the compile-time default Claude model for evaluation.
const DefaultEvaluatorModel = "claude-sonnet-4-6"

// BatchSession holds data for one session in an Evaluator batch.
type BatchSession struct {
	SessionID        string
	Digest           *parser.Digest
	ClassifierOutput *ClassifierOutput
}

// RunEvaluator constructs the prompt, invokes the Evaluator via the claude CLI,
// validates the output, and returns the parsed result.
// Returns the ClaudeResult alongside the parsed output for usage tracking.
func RunEvaluator(sessionID string, digest *parser.Digest, classifierOutput *ClassifierOutput, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
	systemPrompt, err := readPromptTemplate(evaluatorPromptFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading evaluator prompt: %w", err)
	}
	return runEvaluatorCore(sessionID, digest, classifierOutput, cfg, systemPrompt)
}

// RunEvaluatorWithPrompt is like RunEvaluator but uses a caller-supplied system prompt
// instead of reading the default prompt file from disk. Intended for replay/testing workflows
// that need to exercise an alternate prompt version.
func RunEvaluatorWithPrompt(sessionID string, digest *parser.Digest, classifierOutput *ClassifierOutput, cfg PipelineConfig, systemPrompt string) (*EvaluatorOutput, *ClaudeResult, error) {
	return runEvaluatorCore(sessionID, digest, classifierOutput, cfg, systemPrompt)
}

// runEvaluatorCore holds the shared implementation called by RunEvaluator and
// RunEvaluatorWithPrompt. systemPrompt is the full prompt text (already loaded).
func runEvaluatorCore(sessionID string, digest *parser.Digest, classifierOutput *ClassifierOutput, cfg PipelineConfig, systemPrompt string) (*EvaluatorOutput, *ClaudeResult, error) {
	classifierJSON, err := json.MarshalIndent(classifierOutput, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling classifier output: %w", err)
	}

	digestJSON, err := json.MarshalIndent(digest, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling digest: %w", err)
	}

	// System prompt goes via --system-prompt; data goes as the -p prompt.
	data := "<classification>\n" + string(classifierJSON) + "\n</classification>" +
		"\n\n<session_digest>\n" + string(digestJSON) + "\n</session_digest>"

	// Inject turn budget into the prompt template.
	systemPrompt = strings.ReplaceAll(systemPrompt, "{{MAX_TURNS}}", strconv.Itoa(cfg.EvaluatorMaxTurns))

	// Scope filesystem access: ~/.cabrero + project dir + ~/.claude.
	allowedTools := evaluatorAllowedTools(digest.Shape.Cwd)

	cr, err := invokeClaude(claudeConfig{
		Model:          cfg.EvaluatorModel,
		SystemPrompt:   systemPrompt,
		Effort:         "high",
		Agentic:        true,
		Prompt:         data,
		AllowedTools:   allowedTools,
		MaxTurns:       cfg.EvaluatorMaxTurns,
		Timeout:        cfg.EvaluatorTimeout,
		Debug:          cfg.Debug,
		Logger:         cfg.logger(),
		PermissionMode: "dontAsk",
		SettingSources: &emptyStr,
	})
	if err != nil {
		return nil, cr, fmt.Errorf("invoking evaluator: %w", err)
	}

	// Parse JSON output (instructed via system prompt, cleaned defensively).
	output, err := parseEvaluatorOutput(cr.Result)
	if err != nil {
		return nil, cr, fmt.Errorf("parsing evaluator output: %w\nRaw output:\n%s", err, truncateForLog(cr.Result, 500))
	}

	output.SessionID = sessionID
	output.PromptVersion = strings.TrimSuffix(evaluatorPromptFile, ".txt")
	output.ClassifierPromptVersion = classifierOutput.PromptVersion

	if err := validateEvaluatorOutput(sessionID, output, classifierOutput, cfg.logger()); err != nil {
		return nil, cr, err
	}

	return output, cr, nil
}

// RunEvaluatorBatch evaluates multiple sessions in a single Evaluator invocation.
// Each session's Classifier output and digest are included as indexed entries.
// Max turns and timeout scale with batch size, capped at 60 turns and 15 minutes.
// Returns the ClaudeResult alongside the parsed output for usage tracking.
func RunEvaluatorBatch(sessions []BatchSession, cfg PipelineConfig) (*EvaluatorOutput, *ClaudeResult, error) {
	if len(sessions) == 0 {
		return nil, nil, fmt.Errorf("RunEvaluatorBatch called with zero sessions")
	}

	systemPrompt, err := readPromptTemplate(evaluatorPromptFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading evaluator prompt: %w", err)
	}

	// Build batch prompt with indexed session data.
	var dataBuilder strings.Builder
	dataBuilder.WriteString(fmt.Sprintf("You are evaluating %d sessions from the same project in a single batch.\n", len(sessions)))
	dataBuilder.WriteString("Generate proposals for ALL sessions that warrant them. Use the standard proposal ID format: prop-{first 8 chars of sessionId}-{index}.\n\n")

	for i, s := range sessions {
		classifierJSON, err := json.MarshalIndent(s.ClassifierOutput, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("marshalling classifier output for session %d: %w", i, err)
		}

		digestJSON, err := json.MarshalIndent(s.Digest, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("marshalling digest for session %d: %w", i, err)
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

	// Scope filesystem access: use first session's cwd (all sessions share the same project).
	allowedTools := evaluatorAllowedTools(sessions[0].Digest.Shape.Cwd)

	cr, err := invokeClaude(claudeConfig{
		Model:          cfg.EvaluatorModel,
		SystemPrompt:   systemPrompt,
		Effort:         "high",
		Agentic:        true,
		Prompt:         dataBuilder.String(),
		AllowedTools:   allowedTools,
		MaxTurns:       maxTurns,
		Timeout:        timeout,
		Debug:          cfg.Debug,
		Logger:         cfg.logger(),
		PermissionMode: "dontAsk",
		SettingSources: &emptyStr,
	})
	if err != nil {
		return nil, cr, fmt.Errorf("invoking evaluator batch: %w", err)
	}

	output, err := parseEvaluatorOutput(cr.Result)
	if err != nil {
		return nil, cr, fmt.Errorf("parsing evaluator batch output: %w\nRaw output:\n%s", err, truncateForLog(cr.Result, 500))
	}

	// Set metadata from the first session (batch output covers multiple sessions).
	output.SessionID = sessions[0].SessionID
	output.PromptVersion = strings.TrimSuffix(evaluatorPromptFile, ".txt")
	output.ClassifierPromptVersion = sessions[0].ClassifierOutput.PromptVersion

	// Validate: build combined skill set and UUID set from all sessions.
	if err := validateEvaluatorBatchOutput(sessions, output, cfg.logger()); err != nil {
		return nil, cr, err
	}

	return output, cr, nil
}

// validateEvaluatorBatchOutput validates proposals against all sessions in the batch.
// UUIDs are checked across all sessions. Skill signals are merged from all Classifier outputs.
func validateEvaluatorBatchOutput(sessions []BatchSession, output *EvaluatorOutput, log Logger) error {
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
		log.Error("  Warning: Evaluator batch cited non-existent UUID: %s", uuid)
	}

	return filterAndValidateProposals(output, validUUIDs, allSkills, log)
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
func validateEvaluatorOutput(sessionID string, output *EvaluatorOutput, classifierOutput *ClassifierOutput, log Logger) error {
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
			log.Error("  Warning: Evaluator cited non-existent UUID: %s", uuid)
		} else {
			validUUIDs[uuid] = true
		}
	}

	return filterAndValidateProposals(output, validUUIDs, classifierSkills, log)
}

// filterAndValidateProposals validates proposals for duplicate IDs, low confidence,
// valid skill_scaffold fields, UUID citations, and skill signal references.
// Shared by both single-session and batch validation paths.
func filterAndValidateProposals(output *EvaluatorOutput, validUUIDs map[string]bool, allSkills map[string]bool, log Logger) error {
	seenIDs := make(map[string]bool)
	var validProposals []Proposal

	for _, p := range output.Proposals {
		// Skip duplicates.
		if seenIDs[p.ID] {
			log.Error("  Warning: duplicate proposal ID: %s", p.ID)
			continue
		}
		seenIDs[p.ID] = true

		// Filter out low-confidence.
		if p.Confidence == "low" {
			log.Error("  Warning: dropping low-confidence proposal: %s", p.ID)
			continue
		}

		// Validate skill_scaffold proposals must have ScaffoldSkillName.
		if p.Type == "skill_scaffold" && (p.ScaffoldSkillName == nil || *p.ScaffoldSkillName == "") {
			log.Error("  Warning: dropping skill_scaffold proposal without scaffoldSkillName: %s", p.ID)
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
				log.Error("  Warning: proposal %s cites skill '%s' not in Classifier output", p.ID, skill)
			}
		}

		validProposals = append(validProposals, p)
	}

	output.Proposals = validProposals
	return nil
}

// evaluatorAllowedTools builds a path-scoped --allowedTools value for the evaluator.
// Always includes ~/.cabrero and ~/.claude; includes the project dir when available.
func evaluatorAllowedTools(cwd *string) string {
	cabreroRoot := store.Root()
	paths := []string{
		fmt.Sprintf("Read(//%s/**)", cabreroRoot),
		fmt.Sprintf("Grep(//%s/**)", cabreroRoot),
	}

	// Add project directory if available from the digest.
	if cwd != nil && *cwd != "" {
		paths = append(paths,
			fmt.Sprintf("Read(//%s/**)", *cwd),
			fmt.Sprintf("Grep(//%s/**)", *cwd),
		)
	}

	// Allow reading ~/.claude/ for CLAUDE.md and settings context.
	home, _ := os.UserHomeDir()
	if home != "" {
		claudeDir := filepath.Join(home, ".claude")
		paths = append(paths,
			fmt.Sprintf("Read(//%s/**)", claudeDir),
			fmt.Sprintf("Grep(//%s/**)", claudeDir),
		)
	}

	return strings.Join(paths, ",")
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
