package pipeline

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// PipelineRun represents a single pipeline processing run for a session.
type PipelineRun struct {
	SessionID string
	Project   string
	Timestamp time.Time
	Status    string // "queued", "processed", "error"
	Source    string // "daemon", "cli-run", "cli-backfill", "cleanup"

	// Per-stage completion.
	HasDigest     bool
	HasClassifier bool
	HasEvaluator  bool

	// Per-stage timing (zero if stage not completed).
	ParseDuration      time.Duration
	ClassifierDuration time.Duration
	EvaluatorDuration  time.Duration

	// Results.
	ProposalCount int
	ErrorDetail   string

	// LLM usage (from history records).
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// PipelineStats holds aggregated pipeline statistics.
type PipelineStats struct {
	SessionsCaptured   int
	SessionsProcessed  int
	SessionsQueued     int
	SessionsErrored    int
	ProposalsGenerated int
	ProposalsApproved  int
	ProposalsRejected  int
	ProposalsPending   int
	SessionsPerDay     []int // for sparkline, index 0 = today

	// LLM usage totals (aggregated from runs in the time window).
	TotalInputTokens  int
	TotalOutputTokens int
	TotalCostUSD      float64
}

// PromptVersion represents a prompt file with its version and last-modified time.
type PromptVersion struct {
	Name      string
	Version   string
	UpdatedAt time.Time
}

// ListPipelineRunsFromSessions reconstructs run data from pre-loaded session
// metadata and evaluation file existence. Pass limit=0 for no limit.
func ListPipelineRunsFromSessions(sessions []store.Metadata, limit int) ([]PipelineRun, error) {
	var runs []PipelineRun
	for i, meta := range sessions {
		if limit > 0 && i >= limit {
			break
		}

		ts, _ := time.Parse(time.RFC3339, meta.Timestamp)
		run := PipelineRun{
			SessionID: meta.SessionID,
			Project:   store.ProjectDisplayName(meta.Project),
			Timestamp: ts,
			Status:    meta.Status,
		}

		evalDir := filepath.Join(store.Root(), "evaluations")
		digestDir := filepath.Join(store.Root(), "digests")

		// Check stage completion via file existence.
		digestPath := filepath.Join(digestDir, meta.SessionID+".json")
		classifierPath := filepath.Join(evalDir, meta.SessionID+"-classifier.json")
		evaluatorPath := filepath.Join(evalDir, meta.SessionID+"-evaluator.json")

		var digestInfo, classifierInfo, evaluatorInfo os.FileInfo

		if info, err := os.Stat(digestPath); err == nil {
			run.HasDigest = true
			digestInfo = info
		}
		if info, err := os.Stat(classifierPath); err == nil {
			run.HasClassifier = true
			classifierInfo = info
		}
		if info, err := os.Stat(evaluatorPath); err == nil {
			run.HasEvaluator = true
			evaluatorInfo = info
		}

		// Estimate per-stage timing from file modification timestamps.
		if run.HasDigest && !ts.IsZero() {
			run.ParseDuration = digestInfo.ModTime().Sub(ts)
			if run.ParseDuration < 0 {
				run.ParseDuration = 0
			}
		}
		if run.HasClassifier && run.HasDigest {
			run.ClassifierDuration = classifierInfo.ModTime().Sub(digestInfo.ModTime())
			if run.ClassifierDuration < 0 {
				run.ClassifierDuration = 0
			}
		}
		if run.HasEvaluator && run.HasClassifier {
			run.EvaluatorDuration = evaluatorInfo.ModTime().Sub(classifierInfo.ModTime())
			if run.EvaluatorDuration < 0 {
				run.EvaluatorDuration = 0
			}
		}

		// Count proposals from evaluator output.
		if run.HasEvaluator {
			if so, err := ReadEvaluatorOutput(meta.SessionID); err == nil {
				run.ProposalCount = len(so.Proposals)
			}
		}

		runs = append(runs, run)
	}

	return runs, nil
}

// ListPipelineRunsFromHistory builds PipelineRun data from history records,
// falling back to mtime-based estimation for sessions without history.
// Pass limit=0 for no limit.
func ListPipelineRunsFromHistory(sessions []store.Metadata, limit int) ([]PipelineRun, error) {
	history, _ := ReadHistory()

	// Index by session ID — latest record per session wins (handles retries).
	bySessionID := make(map[string]*HistoryRecord, len(history))
	for i := range history {
		rec := &history[i]
		if existing, ok := bySessionID[rec.SessionID]; !ok || rec.Timestamp.After(existing.Timestamp) {
			bySessionID[rec.SessionID] = rec
		}
	}

	var runs []PipelineRun
	for i, meta := range sessions {
		if limit > 0 && i >= limit {
			break
		}

		ts, _ := time.Parse(time.RFC3339, meta.Timestamp)

		if rec, ok := bySessionID[meta.SessionID]; ok {
			// History record available — use actual timing.
			run := PipelineRun{
				SessionID:          meta.SessionID,
				Project:            store.ProjectDisplayName(meta.Project),
				Timestamp:          ts,
				Status:             meta.Status,
				HasDigest:          true,
				HasClassifier:      rec.ClassifierDurationNs > 0,
				HasEvaluator:       rec.EvaluatorDurationNs > 0,
				ParseDuration:      rec.ParseDuration(),
				ClassifierDuration: rec.ClassifierDuration(),
				EvaluatorDuration:  rec.EvaluatorDuration(),
				ProposalCount:      rec.ProposalCount,
				ErrorDetail:        rec.ErrorDetail,
				InputTokens:        rec.TotalInputTokens,
				OutputTokens:       rec.TotalOutputTokens,
				CostUSD:            rec.TotalCostUSD,
			}
			runs = append(runs, run)
			continue
		}

		// No history record — fall back to mtime estimation.
		run := PipelineRun{
			SessionID: meta.SessionID,
			Project:   store.ProjectDisplayName(meta.Project),
			Timestamp: ts,
			Status:    meta.Status,
		}

		evalDir := filepath.Join(store.Root(), "evaluations")
		digestDir := filepath.Join(store.Root(), "digests")

		digestPath := filepath.Join(digestDir, meta.SessionID+".json")
		classifierPath := filepath.Join(evalDir, meta.SessionID+"-classifier.json")
		evaluatorPath := filepath.Join(evalDir, meta.SessionID+"-evaluator.json")

		var digestInfo, classifierInfo, evaluatorInfo os.FileInfo

		if info, err := os.Stat(digestPath); err == nil {
			run.HasDigest = true
			digestInfo = info
		}
		if info, err := os.Stat(classifierPath); err == nil {
			run.HasClassifier = true
			classifierInfo = info
		}
		if info, err := os.Stat(evaluatorPath); err == nil {
			run.HasEvaluator = true
			evaluatorInfo = info
		}

		if run.HasDigest && !ts.IsZero() {
			run.ParseDuration = digestInfo.ModTime().Sub(ts)
			if run.ParseDuration < 0 {
				run.ParseDuration = 0
			}
		}
		if run.HasClassifier && run.HasDigest {
			run.ClassifierDuration = classifierInfo.ModTime().Sub(digestInfo.ModTime())
			if run.ClassifierDuration < 0 {
				run.ClassifierDuration = 0
			}
		}
		if run.HasEvaluator && run.HasClassifier {
			run.EvaluatorDuration = evaluatorInfo.ModTime().Sub(classifierInfo.ModTime())
			if run.EvaluatorDuration < 0 {
				run.EvaluatorDuration = 0
			}
		}

		if run.HasEvaluator {
			if so, err := ReadEvaluatorOutput(meta.SessionID); err == nil {
				run.ProposalCount = len(so.Proposals)
			}
		}

		runs = append(runs, run)
	}

	return runs, nil
}

// HistoryStats holds aggregate timing and operational statistics.
type HistoryStats struct {
	TotalRuns        int
	ClassifierRuns   int
	EvaluatorRuns    int
	EvaluatorSkipped int // triage == "clean"
	SourceGated      int // evaluator skipped by source policy (unclassified or paused)
	SkippedBusy      int // sessions skipped because all invoke slots were busy
	ErrorRuns        int
	RetryRuns        int // PreviousStatus == "error"

	// Timing percentiles.
	MedianClassifier time.Duration
	P95Classifier    time.Duration
	MedianEvaluator  time.Duration
	P95Evaluator     time.Duration
	MedianTotal      time.Duration
	P95Total         time.Duration

	// Source breakdown.
	DaemonRuns   int
	CLIRuns      int
	BackfillRuns int
	BatchRuns    int // BatchMode == true
	SingleRuns   int // BatchMode == false
}

// ComputeStatsFromHistory computes aggregate statistics from history records
// within the given time window.
func ComputeStatsFromHistory(records []HistoryRecord, since time.Time) HistoryStats {
	var stats HistoryStats
	var classifierDurations, evaluatorDurations, totalDurations []time.Duration

	for _, rec := range records {
		if rec.Timestamp.Before(since) {
			continue
		}

		stats.TotalRuns++

		// Source breakdown.
		switch rec.Source {
		case "daemon":
			stats.DaemonRuns++
		case "cli-run":
			stats.CLIRuns++
		case "cli-backfill":
			stats.BackfillRuns++
		}

		// Batch vs single.
		if rec.BatchMode {
			stats.BatchRuns++
		} else {
			stats.SingleRuns++
		}

		// Retry detection.
		if rec.PreviousStatus == "error" {
			stats.RetryRuns++
		}

		// Outcome.
		if rec.Status == "error" {
			stats.ErrorRuns++
		}
		if rec.Status == "skipped_busy" {
			stats.SkippedBusy++
		}

		// Triage.
		if rec.Triage == "clean" {
			stats.EvaluatorSkipped++
		}
		if rec.GateReason != "" {
			stats.SourceGated++
		}

		// Timing — only include durations from non-error runs.
		if rec.ClassifierDurationNs > 0 {
			stats.ClassifierRuns++
			classifierDurations = append(classifierDurations, rec.ClassifierDuration())
		}
		if rec.EvaluatorDurationNs > 0 {
			stats.EvaluatorRuns++
			evaluatorDurations = append(evaluatorDurations, rec.EvaluatorDuration())
		}
		if rec.TotalDurationNs > 0 {
			totalDurations = append(totalDurations, rec.TotalDuration())
		}
	}

	// Compute percentiles.
	stats.MedianClassifier, stats.P95Classifier = durationPercentiles(classifierDurations, 50, 95)
	stats.MedianEvaluator, stats.P95Evaluator = durationPercentiles(evaluatorDurations, 50, 95)
	stats.MedianTotal, stats.P95Total = durationPercentiles(totalDurations, 50, 95)

	return stats
}

// durationPercentiles returns the p1th and p2th percentiles of a duration slice.
func durationPercentiles(durations []time.Duration, p1, p2 int) (time.Duration, time.Duration) {
	if len(durations) == 0 {
		return 0, 0
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	return percentile(durations, p1), percentile(durations, p2)
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// GatherPipelineStatsFromSessions aggregates pipeline statistics over the given
// number of days using pre-loaded sessions and runs to avoid redundant I/O.
func GatherPipelineStatsFromSessions(sessions []store.Metadata, runs []PipelineRun, days int) (PipelineStats, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	stats := PipelineStats{}
	var timestamps []time.Time

	for _, meta := range sessions {
		ts, _ := time.Parse(time.RFC3339, meta.Timestamp)
		if ts.Before(cutoff) {
			continue
		}

		stats.SessionsCaptured++
		timestamps = append(timestamps, ts)

		switch meta.Status {
		case store.StatusProcessed:
			stats.SessionsProcessed++
		case store.StatusQueued:
			stats.SessionsQueued++
		case store.StatusError:
			stats.SessionsErrored++
		}
	}

	// Count pending proposals.
	proposals, _ := ListProposals()
	stats.ProposalsPending = len(proposals)

	// Count generated proposals and aggregate token/cost from pre-loaded runs.
	for _, run := range runs {
		if !run.Timestamp.Before(cutoff) {
			stats.ProposalsGenerated += run.ProposalCount
			stats.TotalInputTokens += run.InputTokens
			stats.TotalOutputTokens += run.OutputTokens
			stats.TotalCostUSD += run.CostUSD
		}
	}

	// Count archived proposal outcomes within the time window.
	archivedDir := store.ArchivedProposalsDir()
	if entries, err := os.ReadDir(archivedDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(archivedDir, entry.Name()))
			if err != nil {
				continue
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(data, &raw); err != nil {
				continue
			}
			// Check archivedAt is within window.
			if atRaw, ok := raw["archivedAt"]; ok {
				var at time.Time
				if err := json.Unmarshal(atRaw, &at); err == nil && at.Before(cutoff) {
					continue
				}
			}
			// Determine outcome (new field, or migrate from archiveReason).
			outcome := readArchivedOutcome(raw)
			switch outcome {
			case "approved":
				stats.ProposalsApproved++
			case "rejected", "culled", "auto-rejected":
				stats.ProposalsRejected++
			}
		}
	}

	stats.SessionsPerDay = bucketSessionsByDay(timestamps, days, time.Now())

	return stats, nil
}

// readArchivedOutcome reads the outcome from an archived proposal's raw JSON.
// Falls back to migrating the old archiveReason field.
func readArchivedOutcome(raw map[string]json.RawMessage) string {
	if outcomeRaw, ok := raw["outcome"]; ok {
		var o string
		json.Unmarshal(outcomeRaw, &o)
		if o != "" {
			return o
		}
	}
	// Migration: read old archiveReason.
	if reasonRaw, ok := raw["archiveReason"]; ok {
		var reason string
		json.Unmarshal(reasonRaw, &reason)
		switch {
		case reason == "approved":
			return "approved"
		case strings.HasPrefix(reason, "rejected"):
			return "rejected"
		case strings.HasPrefix(reason, "auto-culled"):
			return "culled"
		case reason == "deferred":
			return "deferred"
		}
	}
	return ""
}

// bucketSessionsByDay groups timestamps into daily buckets relative to refTime.
// Index 0 = refTime's day, index 1 = day before, etc.
func bucketSessionsByDay(timestamps []time.Time, days int, refTime time.Time) []int {
	buckets := make([]int, days)
	todayStart := time.Date(refTime.Year(), refTime.Month(), refTime.Day(), 0, 0, 0, 0, refTime.Location())

	for _, ts := range timestamps {
		local := ts.In(refTime.Location())
		tsDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, refTime.Location())
		dayOffset := int(todayStart.Sub(tsDay).Hours() / 24)
		if dayOffset >= 0 && dayOffset < days {
			buckets[dayOffset]++
		}
	}
	return buckets
}

// ListPromptVersions reads prompt files from ~/.cabrero/prompts/ and returns
// their names and versions.
func ListPromptVersions() ([]PromptVersion, error) {
	dir := filepath.Join(store.Root(), "prompts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var versions []PromptVersion
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		name, ver := parsePromptFilename(e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		versions = append(versions, PromptVersion{
			Name:      name,
			Version:   ver,
			UpdatedAt: info.ModTime(),
		})
	}
	return versions, nil
}

// parsePromptFilename extracts the prompt name and version from a filename
// like "classifier-v3.txt" -> ("classifier", "v3").
func parsePromptFilename(filename string) (name, version string) {
	base := strings.TrimSuffix(filename, ".txt")
	idx := strings.LastIndex(base, "-v")
	if idx < 0 {
		return base, ""
	}
	return base[:idx], base[idx+1:]
}

// ListCleanupRunsFromHistory returns PipelineRun entries from cleanup_history.jsonl.
// Each cleanup run is one PipelineRun with Source="cleanup".
// Pass limit=0 for no limit.
func ListCleanupRunsFromHistory(limit int) ([]PipelineRun, error) {
	records, err := ReadCleanupHistory()
	if err != nil || len(records) == 0 {
		return nil, err
	}

	var runs []PipelineRun
	for i, rec := range records {
		if limit > 0 && i >= limit {
			break
		}
		// ProposalCount = proposals archived (before - after).
		archived := rec.ProposalsBefore - rec.ProposalsAfter
		if archived < 0 {
			archived = 0
		}
		// Sum curator LLM usage.
		var inputTokens, outputTokens int
		var costUSD float64
		for _, u := range rec.CuratorUsage {
			inputTokens += u.InputTokens
			outputTokens += u.OutputTokens
			costUSD += u.CostUSD
		}
		if rec.CheckUsage != nil {
			inputTokens += rec.CheckUsage.InputTokens
			outputTokens += rec.CheckUsage.OutputTokens
			costUSD += rec.CheckUsage.CostUSD
		}

		run := PipelineRun{
			Source:        "cleanup",
			Timestamp:     rec.Timestamp,
			Status:        "processed",
			ProposalCount: archived,
			InputTokens:   inputTokens,
			OutputTokens:  outputTokens,
			CostUSD:       costUSD,
			ErrorDetail:   rec.Error,
		}
		if rec.Error != "" {
			run.Status = "error"
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// AcceptanceStats holds acceptance rate data for one evaluator prompt version.
type AcceptanceStats struct {
	PromptVersion  string
	Generated      int
	Approved       int
	Rejected       int
	AcceptanceRate float64 // Approved/(Approved+Rejected); math.NaN() if SampleSize==0
	SampleSize     int     // Approved + Rejected (excludes culled/deferred)
}

// ListAcceptanceRateByPromptVersion joins run history with archived proposals
// to compute acceptance rates per evaluator prompt version.
// Returns one entry per version with ≥1 archived proposal, sorted most-recently-active first.
func ListAcceptanceRateByPromptVersion() ([]AcceptanceStats, error) {
	records, err := ReadHistory()
	if err != nil {
		return nil, err
	}

	type versionData struct {
		lastSeen  time.Time
		generated int
		approved  int
		rejected  int
	}
	byVersion := make(map[string]*versionData)
	sessionToVersion := make(map[string]string)
	for _, r := range records {
		if r.EvaluatorPromptVersion == "" {
			continue
		}
		sessionToVersion[r.SessionID] = r.EvaluatorPromptVersion
		vd := byVersion[r.EvaluatorPromptVersion]
		if vd == nil {
			vd = &versionData{}
			byVersion[r.EvaluatorPromptVersion] = vd
		}
		vd.generated += r.ProposalCount
		if r.Timestamp.After(vd.lastSeen) {
			vd.lastSeen = r.Timestamp
		}
	}

	// Scan archived proposals, join on sessionId.
	archivedDir := store.ArchivedProposalsDir()
	entries, _ := os.ReadDir(archivedDir)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
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
		version := sessionToVersion[sessionID]
		if version == "" {
			continue
		}
		vd := byVersion[version]
		if vd == nil {
			continue
		}
		outcome := readArchivedOutcome(raw)
		switch outcome {
		case "approved":
			vd.approved++
		case "rejected":
			vd.rejected++
		// culled/auto-rejected/deferred excluded from SampleSize
		}
	}

	// Build results, filter to versions with ≥1 archived proposal.
	var result []AcceptanceStats
	for version, vd := range byVersion {
		sampleSize := vd.approved + vd.rejected
		if sampleSize == 0 {
			continue
		}
		rate := math.NaN()
		if sampleSize > 0 {
			rate = float64(vd.approved) / float64(sampleSize)
		}
		result = append(result, AcceptanceStats{
			PromptVersion:  version,
			Generated:      vd.generated,
			Approved:       vd.approved,
			Rejected:       vd.rejected,
			AcceptanceRate: rate,
			SampleSize:     sampleSize,
		})
	}
	// Sort by lastSeen descending (most recently active version first).
	sort.Slice(result, func(i, j int) bool {
		vi := byVersion[result[i].PromptVersion]
		vj := byVersion[result[j].PromptVersion]
		return vi.lastSeen.After(vj.lastSeen)
	})
	return result, nil
}
