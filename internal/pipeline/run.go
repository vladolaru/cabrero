package pipeline

import (
	"os"
	"path/filepath"
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
}

// PromptVersion represents a prompt file with its version and last-used time.
type PromptVersion struct {
	Name     string
	Version  string
	LastUsed time.Time
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

	// Count generated proposals from pre-loaded runs (avoids re-reading evaluator files).
	for _, run := range runs {
		if !run.Timestamp.Before(cutoff) {
			stats.ProposalsGenerated += run.ProposalCount
		}
	}

	stats.SessionsPerDay = bucketSessionsByDay(timestamps, days, time.Now())

	return stats, nil
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
			Name:     name,
			Version:  ver,
			LastUsed: info.ModTime(),
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
