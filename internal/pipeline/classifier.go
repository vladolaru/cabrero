package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/patterns"
	"github.com/vladolaru/cabrero/internal/retrieval"
)

const classifierPromptFile = "classifier-v3.txt"

// RunClassifier constructs the prompt, invokes the Classifier via the claude CLI,
// validates the output, and returns the parsed result.
// The patterns parameter is optional cross-session aggregator output; pass nil if unavailable.
func RunClassifier(sessionID string, digest *parser.Digest, aggregatorOutput *patterns.AggregatorOutput, cfg PipelineConfig) (*ClassifierOutput, error) {
	systemPrompt, err := readPromptTemplate(classifierPromptFile)
	if err != nil {
		return nil, fmt.Errorf("reading classifier prompt: %w", err)
	}

	digestJSON, err := json.MarshalIndent(digest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling digest: %w", err)
	}

	// Inject turn budget into the prompt template.
	systemPrompt = strings.ReplaceAll(systemPrompt, "{{MAX_TURNS}}", strconv.Itoa(cfg.ClassifierMaxTurns))

	// System prompt goes via --system-prompt; data goes as the -p prompt.
	data := "<session_digest>\n" + string(digestJSON) + "\n</session_digest>"

	// Conditionally append cross-session patterns.
	if aggregatorOutput != nil && len(aggregatorOutput.Patterns) > 0 {
		patternsJSON, err := json.MarshalIndent(aggregatorOutput, "", "  ")
		if err == nil {
			data += "\n\n<cross_session_patterns>\n" + string(patternsJSON) + "\n</cross_session_patterns>"
		}
	}

	stdout, err := invokeClaude(claudeConfig{
		Model:        "claude-haiku-4-5",
		SystemPrompt: systemPrompt,
		Agentic:      true,
		Prompt:       data,
		AllowedTools: "Read,Grep",
		MaxTurns:     cfg.ClassifierMaxTurns,
		Timeout:      cfg.ClassifierTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("invoking classifier: %w", err)
	}

	// Parse JSON output (instructed via system prompt, cleaned defensively).
	output, err := parseClassifierOutput(stdout)
	if err != nil {
		return nil, fmt.Errorf("parsing classifier output: %w\nRaw output:\n%s", err, truncateForLog(stdout, 500))
	}

	output.SessionID = sessionID
	output.PromptVersion = strings.TrimSuffix(classifierPromptFile, ".txt")

	// Default empty triage to "evaluate" for backward compatibility with v2 prompt.
	if output.Triage == "" {
		output.Triage = "evaluate"
	}

	// Validate cited UUIDs.
	if err := validateClassifierUUIDs(sessionID, output); err != nil {
		return nil, err
	}

	return output, nil
}

func parseClassifierOutput(raw string) (*ClassifierOutput, error) {
	cleaned := cleanLLMJSON(raw)

	var output ClassifierOutput
	if err := json.Unmarshal([]byte(cleaned), &output); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &output, nil
}

// validateClassifierUUIDs checks that all cited UUIDs exist in the raw transcript.
// Drops entries with invalid UUIDs and fails if >50% are invalid.
func validateClassifierUUIDs(sessionID string, output *ClassifierOutput) error {
	allUUIDs := collectClassifierUUIDs(output)
	if len(allUUIDs) == 0 {
		return nil
	}

	valid := make(map[string]bool)
	invalid := 0

	for _, uuid := range allUUIDs {
		if valid[uuid] {
			continue
		}
		_, err := retrieval.GetEntry(sessionID, uuid)
		if err != nil {
			invalid++
			fmt.Fprintf(os.Stderr, "  Warning: Classifier cited non-existent UUID: %s\n", uuid)
		} else {
			valid[uuid] = true
		}
	}

	totalUnique := len(valid) + invalid
	if totalUnique > 0 && float64(invalid)/float64(totalUnique) > 0.5 {
		return fmt.Errorf("critical: >50%% of Classifier-cited UUIDs are invalid (%d/%d)", invalid, totalUnique)
	}

	// Prune invalid UUID references from the output.
	pruneClassifierInvalidUUIDs(output, valid)

	return nil
}

func collectClassifierUUIDs(output *ClassifierOutput) []string {
	seen := make(map[string]bool)
	var uuids []string

	add := func(uuid string) {
		if uuid != "" && !seen[uuid] {
			seen[uuid] = true
			uuids = append(uuids, uuid)
		}
	}

	for _, e := range output.ErrorClassification {
		for _, u := range e.RelatedUUIDs {
			add(u)
		}
	}
	for _, kt := range output.KeyTurns {
		add(kt.UUID)
	}
	for _, ss := range output.SkillSignals {
		add(ss.InvokedAtUUID)
	}
	// claudeMdSignals don't have UUIDs — they reference paths, not transcript entries.

	return uuids
}

func pruneClassifierInvalidUUIDs(output *ClassifierOutput, valid map[string]bool) {
	// Prune error classifications with invalid UUIDs.
	for i := range output.ErrorClassification {
		var kept []string
		for _, u := range output.ErrorClassification[i].RelatedUUIDs {
			if valid[u] {
				kept = append(kept, u)
			}
		}
		output.ErrorClassification[i].RelatedUUIDs = kept
	}

	// Prune key turns with invalid UUIDs.
	var keptTurns []ClassifierKeyTurn
	for _, kt := range output.KeyTurns {
		if valid[kt.UUID] {
			keptTurns = append(keptTurns, kt)
		}
	}
	output.KeyTurns = keptTurns

	// Prune skill signals with invalid UUIDs.
	var keptSkills []ClassifierSkillSignal
	for _, ss := range output.SkillSignals {
		if ss.InvokedAtUUID == "" || valid[ss.InvokedAtUUID] {
			keptSkills = append(keptSkills, ss)
		}
	}
	output.SkillSignals = keptSkills

	// claudeMdSignals don't have UUIDs to prune — they reference CLAUDE.md paths.
}
