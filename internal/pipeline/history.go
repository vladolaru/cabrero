package pipeline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// --- History record status constants ---

// HistoryStatusProcessed indicates a session was successfully processed by the pipeline.
const HistoryStatusProcessed = "processed"

// HistoryStatusError indicates a pipeline run that encountered an error.
const HistoryStatusError = "error"

// HistoryStatusSkippedBusy indicates a session was skipped because all invoke slots were busy.
const HistoryStatusSkippedBusy = "skipped_busy"

// --- Triage constants ---

// TriageClean means the session has no actionable signals; evaluator is skipped.
const TriageClean = "clean"

// TriageEvaluate means the session warrants deeper evaluator analysis.
const TriageEvaluate = "evaluate"

// --- Gate reason constants ---

// GateReasonUnclassified blocks evaluator because a touched source has no ownership.
const GateReasonUnclassified = "unclassified_source"

// GateReasonPaused blocks evaluator because a touched source has approach "paused".
const GateReasonPaused = "paused_source"

// --- Meta event status constants ---

// HistoryStatusMetaTriggered indicates a meta analysis was triggered and run.
const HistoryStatusMetaTriggered = "meta_triggered"

// HistoryStatusMetaCooldown indicates a meta check was skipped due to cooldown.
const HistoryStatusMetaCooldown = "meta_cooldown_skip"

// HistoryStatusMetaNoThreshold indicates a meta check found no thresholds exceeded.
const HistoryStatusMetaNoThreshold = "meta_no_threshold"

// InvocationUsage captures token consumption and cost for one LLM invocation.
type InvocationUsage struct {
	CCSessionID         string  `json:"cc_session_id,omitempty"`   // CC session ID for cross-referencing
	NumTurns            int     `json:"num_turns"`                 // actual agentic turns
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
	CostUSD             float64 `json:"cost_usd"`
	WebSearchRequests   int     `json:"web_search_requests,omitempty"`
	WebFetchRequests    int     `json:"web_fetch_requests,omitempty"`
}

// usageFromResult converts a ClaudeResult into an InvocationUsage.
// Returns nil if cr is nil.
func usageFromResult(cr *ClaudeResult) *InvocationUsage {
	if cr == nil {
		return nil
	}
	u := InvocationUsageFromResult(cr)
	return &u
}

// InvocationUsageFromResult converts a ClaudeResult into an InvocationUsage.
// Returns zero value if cr is nil.
func InvocationUsageFromResult(cr *ClaudeResult) InvocationUsage {
	if cr == nil {
		return InvocationUsage{}
	}
	return InvocationUsage{
		CCSessionID:         cr.SessionID,
		NumTurns:            cr.NumTurns,
		InputTokens:         cr.InputTokens,
		OutputTokens:        cr.OutputTokens,
		CacheCreationTokens: cr.CacheCreationTokens,
		CacheReadTokens:     cr.CacheReadTokens,
		CostUSD:             cr.TotalCostUSD,
		WebSearchRequests:   cr.WebSearchRequests,
		WebFetchRequests:    cr.WebFetchRequests,
	}
}

// HistoryRecord captures the full diagnostic context of a single pipeline run.
// One record per session processed (batch runs emit one record per session).
type HistoryRecord struct {
	// Identity.
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"` // when the run started (wall clock)
	Project   string    `json:"project"`   // display name from store.ProjectDisplayName

	// Invocation context.
	Source          string   `json:"source"`                          // "daemon", "cli-run", "cli-backfill"
	BatchMode       bool     `json:"batch_mode"`                     // true if processed via RunGroup
	BatchSize       int      `json:"batch_size,omitempty"`           // total sessions in the batch (0 if not batch)
	BatchSessionIDs []string `json:"batch_session_ids,omitempty"`    // sibling session IDs (including self)

	// Session provenance.
	CaptureTrigger string `json:"capture_trigger"`           // from metadata: "session-end", "pre-compact+session-end", "stale-recovery", "imported"
	PreviousStatus string `json:"previous_status,omitempty"` // status before this run: "queued", "imported", "error" (detects retries)

	// Pipeline outcome.
	Triage        string `json:"triage"`                   // "clean", "evaluate", "" (error before triage)
	Status        string `json:"status"`                   // "processed", "error"
	ProposalCount int    `json:"proposal_count"`
	ErrorDetail   string `json:"error_detail,omitempty"`
	GateReason    string `json:"gate_reason,omitempty"`    // "unclassified_source", "paused_source" — set when evaluator skipped by source policy

	// Per-stage wall-clock durations (nanoseconds).
	ParseDurationNs      int64 `json:"parse_duration_ns"`
	ClassifierDurationNs int64 `json:"classifier_duration_ns"`
	EvaluatorDurationNs  int64 `json:"evaluator_duration_ns"`
	TotalDurationNs      int64 `json:"total_duration_ns"`

	// Models and prompts actually used.
	ClassifierModel         string `json:"classifier_model"`                    // e.g. "claude-haiku-4-5"
	EvaluatorModel          string `json:"evaluator_model,omitempty"`           // e.g. "claude-sonnet-4-6" (empty if skipped)
	ClassifierPromptVersion string `json:"classifier_prompt_version"`           // e.g. "classifier-v3"
	EvaluatorPromptVersion  string `json:"evaluator_prompt_version,omitempty"`

	// Per-stage LLM usage (nil when stage was skipped or errored before invocation).
	ClassifierUsage *InvocationUsage `json:"classifier_usage,omitempty"`
	EvaluatorUsage  *InvocationUsage `json:"evaluator_usage,omitempty"`

	// Totals across all stages.
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`

	// Config at invocation time (for detecting non-default settings).
	ClassifierMaxTurns  int   `json:"classifier_max_turns"`
	EvaluatorMaxTurns   int   `json:"evaluator_max_turns"`
	ClassifierTimeoutNs int64 `json:"classifier_timeout_ns"`
	EvaluatorTimeoutNs  int64 `json:"evaluator_timeout_ns"`
	Debug               bool  `json:"debug"`
}

// ParseDuration returns the parse stage duration.
func (r HistoryRecord) ParseDuration() time.Duration {
	return time.Duration(r.ParseDurationNs)
}

// ClassifierDuration returns the classifier stage duration.
func (r HistoryRecord) ClassifierDuration() time.Duration {
	return time.Duration(r.ClassifierDurationNs)
}

// EvaluatorDuration returns the evaluator stage duration.
func (r HistoryRecord) EvaluatorDuration() time.Duration {
	return time.Duration(r.EvaluatorDurationNs)
}

// TotalDuration returns the total pipeline run duration.
func (r HistoryRecord) TotalDuration() time.Duration {
	return time.Duration(r.TotalDurationNs)
}

// computeUsageTotals sets the total cost, input tokens, and output tokens
// from the per-stage usage fields.
func (r *HistoryRecord) computeUsageTotals() {
	var cost float64
	var input, output int
	for _, u := range []*InvocationUsage{r.ClassifierUsage, r.EvaluatorUsage} {
		if u != nil {
			cost += u.CostUSD
			input += u.InputTokens
			output += u.OutputTokens
		}
	}
	r.TotalCostUSD = cost
	r.TotalInputTokens = input
	r.TotalOutputTokens = output
}

var historyMu sync.Mutex

// historyPath returns the path to the run history JSONL file.
// Declared as a variable so tests can override it.
var historyPath = func() string {
	return filepath.Join(store.Root(), "run_history.jsonl")
}

// AppendHistory appends a single history record to the JSONL file.
// Thread-safe via file-level mutex. Best-effort — callers should not
// fail the pipeline run if history recording fails.
func AppendHistory(rec HistoryRecord) error {
	historyMu.Lock()
	defer historyMu.Unlock()

	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	f, err := os.OpenFile(historyPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// AppendSkipRecord writes a lightweight history record for a busy-slot skip.
// The record has status "skipped_busy" with no LLM usage or timing.
func AppendSkipRecord(sessionID, source string) error {
	rec := HistoryRecord{
		SessionID: sessionID,
		Timestamp: time.Now().UTC(),
		Source:    source,
		Status:    HistoryStatusSkippedBusy,
	}
	return AppendHistory(rec)
}

// AppendMetaRecord writes a history record for a meta-analysis event.
// status should be one of HistoryStatusMetaTriggered, HistoryStatusMetaCooldown,
// or HistoryStatusMetaNoThreshold. detail provides context (e.g. prompt version).
func AppendMetaRecord(status, detail string) error {
	rec := HistoryRecord{
		Timestamp:   time.Now().UTC(),
		Source:      "meta",
		Status:      status,
		ErrorDetail: detail, // repurposed for meta event context
	}
	return AppendHistory(rec)
}

// AppendCircuitBreakerRecord writes a circuit breaker event to run history.
func AppendCircuitBreakerRecord(event string, consecutiveErrors int, source string) error {
	rec := HistoryRecord{
		SessionID:   "circuit-breaker",
		Timestamp:   time.Now().UTC(),
		Source:      source,
		Status:      "circuit_breaker_" + event,
		ErrorDetail: fmt.Sprintf("%s: %d consecutive errors", event, consecutiveErrors),
	}
	return AppendHistory(rec)
}

// ReadHistory reads all history records from the JSONL file.
// Returns nil, nil for a missing or empty file. Malformed lines are skipped.
func ReadHistory() ([]HistoryRecord, error) {
	return readHistoryFrom(historyPath())
}

func readHistoryFrom(path string) ([]HistoryRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var records []HistoryRecord
	scanner := bufio.NewScanner(f)
	// Allow up to 1 MB per line (default is 64 KB).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec HistoryRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue // skip malformed lines
		}
		records = append(records, rec)
	}

	if err := scanner.Err(); err != nil {
		return records, err
	}
	return records, nil
}

// RotateHistory removes records older than maxAge from the history file.
// Returns the count of removed records.
func RotateHistory(maxAge time.Duration) (int, error) {
	historyMu.Lock()
	defer historyMu.Unlock()

	path := historyPath()
	records, err := readHistoryFrom(path)
	if err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}

	cutoff := time.Now().Add(-maxAge)
	var kept []HistoryRecord
	removed := 0

	for _, rec := range records {
		if rec.Timestamp.Before(cutoff) {
			removed++
		} else {
			kept = append(kept, rec)
		}
	}

	if removed == 0 {
		return 0, nil
	}

	// Rewrite the file atomically.
	var data []byte
	for _, rec := range kept {
		line, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		data = append(data, line...)
		data = append(data, '\n')
	}

	if err := store.AtomicWrite(path, data, 0o644); err != nil {
		return 0, err
	}
	return removed, nil
}
