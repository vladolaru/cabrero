package pipeline

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// PipelineMetrics holds computed quality metrics for the pipeline.
type PipelineMetrics struct {
	AcceptanceByVersion []AcceptanceStats

	ClassifierFPR       float64 // evaluate→zero-proposals / total evaluate sessions
	ClassifierFPRWindow int     // days of history used

	ClassifierMedianTurns float64
	EvaluatorMedianTurns  float64

	CostPerAcceptedProposal float64

	ComputedAt time.Time
}

// ComputePipelineMetrics reads run history and archived proposals to compute
// pipeline quality metrics. No LLM calls.
func ComputePipelineMetrics(cfg PipelineConfig) (PipelineMetrics, error) {
	const window = 30
	cutoff := time.Now().AddDate(0, 0, -window)

	records, err := ReadHistory()
	if err != nil {
		return PipelineMetrics{}, err
	}

	var evaluateSessions, fpSessions int
	var classifierTurns, evaluatorTurns []float64
	var totalCost float64
	var totalApproved int

	for _, r := range records {
		if r.Timestamp.Before(cutoff) {
			continue
		}
		if r.Triage == "evaluate" {
			evaluateSessions++
			if r.ProposalCount == 0 {
				fpSessions++
			}
			if r.ClassifierUsage != nil && r.ClassifierUsage.NumTurns > 0 {
				classifierTurns = append(classifierTurns, float64(r.ClassifierUsage.NumTurns))
			}
			if r.EvaluatorUsage != nil && r.EvaluatorUsage.NumTurns > 0 {
				evaluatorTurns = append(evaluatorTurns, float64(r.EvaluatorUsage.NumTurns))
			}
			totalCost += r.TotalCostUSD
		}
	}

	fpr := math.NaN()
	if evaluateSessions > 0 {
		fpr = float64(fpSessions) / float64(evaluateSessions)
	}

	acceptanceByVersion, _ := ListAcceptanceRateByPromptVersion()

	// Count total approved for cost-per-accepted-proposal.
	for _, a := range acceptanceByVersion {
		totalApproved += a.Approved
	}
	costPerAccepted := math.NaN()
	if totalApproved > 0 {
		costPerAccepted = totalCost / float64(totalApproved)
	}

	return PipelineMetrics{
		AcceptanceByVersion:     acceptanceByVersion,
		ClassifierFPR:           fpr,
		ClassifierFPRWindow:     window,
		ClassifierMedianTurns:   median(classifierTurns),
		EvaluatorMedianTurns:    median(evaluatorTurns),
		CostPerAcceptedProposal: costPerAccepted,
		ComputedAt:              time.Now(),
	}, nil
}

// MetaCooldownCutoff returns the cutoff time for the meta-pipeline cooldown.
func MetaCooldownCutoff(cooldownDays int) time.Time {
	return time.Now().AddDate(0, 0, -cooldownDays)
}

// ProposalCreatedAfter returns true if the proposal was created after the given cutoff.
// For meta proposals (prop-meta-<unix_ts>-N), parses the timestamp from the ID.
// For other proposals with no embedded timestamp, returns true (fail-open: treat as recent).
func ProposalCreatedAfter(pw ProposalWithSession, cutoff time.Time) bool {
	// Meta proposal IDs: prop-meta-<unix_timestamp>-<index>
	if strings.HasPrefix(pw.Proposal.ID, "prop-meta-") {
		parts := strings.Split(pw.Proposal.ID, "-")
		// Expected: ["prop", "meta", "<timestamp>", "<index>"]
		if len(parts) >= 4 {
			ts, err := strconv.ParseInt(parts[2], 10, 64)
			if err == nil {
				return time.Unix(ts, 0).After(cutoff)
			}
		}
	}
	// No timestamp available — fail open (treat as recent).
	return true
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return math.NaN()
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// filterValidTranscripts checks each path: logs a warning and skips if
// the file is not found or contains no tool_use entries.
func filterValidTranscripts(paths []string, log Logger) []string {
	var valid []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			log.Error("WARN transcript not found for cc_session_id at expected path %s — CC storage conventions may have changed", p)
			continue
		}
		if !transcriptHasToolUse(data) {
			log.Error("WARN transcript %s contains no tool_use entries — CC format may have changed or this is a print-mode session", p)
			continue
		}
		valid = append(valid, p)
	}
	return valid
}

func transcriptHasToolUse(data []byte) bool {
	// Scan JSONL lines for "tool_use" in content blocks.
	// Simple string search is sufficient — avoids full parse overhead.
	return strings.Contains(string(data), `"tool_use"`)
}

// ccProjectsDir returns the path to ~/.claude/projects/.
func ccProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// transcriptPathForSession constructs the expected CC transcript path for a
// given cc_session_id by scanning all project dirs for the file.
func transcriptPathForSession(ccSessionID string) (string, error) {
	projectsDir, err := ccProjectsDir()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return "", err
	}
	for _, proj := range entries {
		if !proj.IsDir() {
			continue
		}
		candidate := filepath.Join(projectsDir, proj.Name(), ccSessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("transcript for %s not found in %s", ccSessionID, projectsDir)
}

// listRejectedProposalsForVersion returns the most recently rejected archived
// proposals associated with the given evaluator prompt version (up to limit).
func listRejectedProposalsForVersion(promptVersion string, limit int) ([]ProposalWithSession, error) {
	records, err := ReadHistory()
	if err != nil {
		return nil, err
	}
	sessionToVersion := make(map[string]string)
	for _, r := range records {
		if r.EvaluatorPromptVersion == promptVersion {
			sessionToVersion[r.SessionID] = promptVersion
		}
	}

	archivedDir := store.ArchivedProposalsDir()
	entries, err := os.ReadDir(archivedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []ProposalWithSession
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if len(result) >= limit {
			break
		}
		data, err := os.ReadFile(filepath.Join(archivedDir, entry.Name()))
		if err != nil {
			continue
		}
		var raw map[string]json.RawMessage
		if json.Unmarshal(data, &raw) != nil {
			continue
		}
		var sessionID string
		json.Unmarshal(raw["sessionId"], &sessionID)
		if sessionToVersion[sessionID] == "" {
			continue
		}
		outcome := readArchivedOutcome(raw)
		if outcome != "rejected" {
			continue
		}
		var pw ProposalWithSession
		if err := json.Unmarshal(data, &pw); err == nil {
			result = append(result, pw)
		}
	}
	return result, nil
}

// highTurnTranscriptPaths returns transcript paths for the highest-turn
// rejected runs for the given prompt version (up to limit).
func highTurnTranscriptPaths(promptVersion string, limit int) []string {
	records, err := ReadHistory()
	if err != nil {
		return nil
	}

	type scored struct {
		sessionID string
		turns     int
	}
	var candidates []scored
	for _, r := range records {
		if r.EvaluatorPromptVersion != promptVersion {
			continue
		}
		if r.EvaluatorUsage == nil || r.EvaluatorUsage.NumTurns == 0 {
			continue
		}
		candidates = append(candidates, scored{sessionID: r.SessionID, turns: r.EvaluatorUsage.NumTurns})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].turns > candidates[j].turns
	})

	var paths []string
	for i, c := range candidates {
		if i >= limit {
			break
		}
		if p, err := transcriptPathForSession(c.sessionID); err == nil {
			paths = append(paths, p)
		}
	}
	return paths
}

// buildMetaPrompt constructs the user prompt for the meta-analysis invocation.
func buildMetaPrompt(stats AcceptanceStats, rejectedProps []ProposalWithSession, transcripts []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Acceptance stats for %s\n\n", stats.PromptVersion))
	b.WriteString(fmt.Sprintf("- Sample size: %d\n", stats.SampleSize))
	b.WriteString(fmt.Sprintf("- Approved: %d\n", stats.Approved))
	b.WriteString(fmt.Sprintf("- Rejected: %d\n", stats.Rejected))
	if !math.IsNaN(stats.AcceptanceRate) {
		b.WriteString(fmt.Sprintf("- Acceptance rate: %.0f%%\n", stats.AcceptanceRate*100))
	}

	b.WriteString(fmt.Sprintf("\n## Rejected proposals (%d most recent)\n\n", len(rejectedProps)))
	for _, pw := range rejectedProps {
		b.WriteString(fmt.Sprintf("- %s: type=%s target=%s\n", pw.Proposal.ID, pw.Proposal.Type, pw.Proposal.Target))
	}

	if len(transcripts) > 0 {
		b.WriteString("\n## Transcript paths for high-turn rejected runs\n\n")
		for _, p := range transcripts {
			b.WriteString(fmt.Sprintf("- %s\n", p))
		}
	}

	promptPath := filepath.Join(store.Root(), "prompts", stats.PromptVersion+".txt")
	b.WriteString(fmt.Sprintf("\nTarget evaluator prompt: %s\n", promptPath))
	return b.String()
}

// parseMetaProposal parses the LLM output from RunMetaAnalysis into a Proposal.
func parseMetaProposal(cleaned string, promptVersion string) (*Proposal, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return nil, fmt.Errorf("invalid meta output JSON: %w", err)
	}
	var proposalType string
	json.Unmarshal(raw["type"], &proposalType)
	if proposalType != TypePromptImprovement && proposalType != TypePipelineInsight {
		return nil, fmt.Errorf("unexpected meta proposal type: %q", proposalType)
	}
	var target, rationale string
	json.Unmarshal(raw["target"], &target)
	json.Unmarshal(raw["rationale"], &rationale)
	var changeStr *string
	if raw["change"] != nil {
		var s string
		if json.Unmarshal(raw["change"], &s) == nil && s != "" {
			changeStr = &s
		}
	}
	var citedUUIDs []string
	json.Unmarshal(raw["citedUuids"], &citedUUIDs)
	return &Proposal{
		Type:       proposalType,
		Target:     target,
		Change:     changeStr,
		Rationale:  rationale,
		CitedUUIDs: citedUUIDs,
		Confidence: "medium",
	}, nil
}

// RunMetaAnalysis fires only when a version crosses a threshold with sufficient
// samples and no recent meta-proposal exists for that version.
// Invokes Opus via the meta prompt with rejected proposals and CC transcripts.
// Returns the proposal ID written to the queue, or "" if no action was taken.
func RunMetaAnalysis(stats AcceptanceStats, cfg PipelineConfig) (string, error) {
	log := cfg.logger()

	if err := EnsureMetaPrompts(); err != nil {
		return "", fmt.Errorf("ensuring meta prompts: %w", err)
	}

	systemPrompt, err := readPromptTemplate(metaPromptFile)
	if err != nil {
		return "", fmt.Errorf("reading meta prompt: %w", err)
	}

	// Gather the 10 most-recently rejected proposals for this version.
	rejectedProps, err := listRejectedProposalsForVersion(stats.PromptVersion, 10)
	if err != nil {
		log.Error("meta: listing rejected proposals: %v", err)
	}

	// Gather CC session transcripts for the 3 highest-turn rejected runs.
	transcriptPaths := highTurnTranscriptPaths(stats.PromptVersion, 3)
	validTranscripts := filterValidTranscripts(transcriptPaths, log)

	// Build the user prompt from the data gathered.
	userPrompt := buildMetaPrompt(stats, rejectedProps, validTranscripts)

	// Determine allowed tools: Read + Grep scoped to home dir.
	home, _ := os.UserHomeDir()
	allowedTools := fmt.Sprintf("Read(//%s/**),Grep(//%s/**)", home, home)

	cr, err := invokeClaude(claudeConfig{
		Model:          cfg.MetaModel,
		SystemPrompt:   systemPrompt,
		Agentic:        true,
		Prompt:         userPrompt,
		AllowedTools:   allowedTools,
		MaxTurns:       cfg.MetaMaxTurns,
		Timeout:        cfg.MetaTimeout,
		Debug:          cfg.Debug,
		Logger:         log,
		SettingSources: &emptyStr,
	})
	if err != nil {
		return "", fmt.Errorf("meta invocation: %w", err)
	}

	// Parse the output proposal.
	cleaned := cleanLLMJSON(cr.Result)
	proposal, err := parseMetaProposal(cleaned, stats.PromptVersion)
	if err != nil {
		return "", fmt.Errorf("parsing meta output: %w", err)
	}

	// Generate a proposal ID: prop-meta-<timestamp>-1
	propID := fmt.Sprintf("prop-meta-%d-1", time.Now().Unix())
	proposal.ID = propID

	if err := WriteProposal(proposal, "meta"); err != nil {
		return "", fmt.Errorf("writing meta proposal: %w", err)
	}
	log.Info("meta: wrote %s proposal %s for %s", proposal.Type, propID, stats.PromptVersion)
	return propID, nil
}
