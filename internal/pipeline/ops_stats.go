package pipeline

import (
	"sort"
	"time"
)

// OpsStats holds aggregated operational statistics for the Ops view.
type OpsStats struct {
	// Window metadata.
	WindowStart time.Time
	WindowEnd   time.Time

	// Summary counts.
	GatedRuns      int // evaluator skipped by source policy
	SkippedBusy    int // sessions skipped because invoke slots were busy
	MetaTriggered  int // meta analyses that were triggered and run
	MetaCooldowns  int // meta checks skipped due to cooldown
	MetaNoThreshold int // meta checks with no thresholds exceeded
	ErrorRuns      int
	ProcessedRuns  int

	// Gate reason breakdown.
	GatedUnclassified int // GateReason == "unclassified_source"
	GatedPaused       int // GateReason == "paused_source"

	// Recent events for the event list.
	RecentEvents []OpsEvent

	// Trend data: daily counts for sparklines.
	DailyGated      []int // index 0 = today, 1 = yesterday, etc.
	DailySkipped    []int
	DailyProcessed  []int
	DailyErrors     []int
}

// OpsEvent is a single operational event for the recent events list.
type OpsEvent struct {
	Timestamp time.Time
	SessionID string
	Source    string // "daemon", "cli-run", "meta", etc.
	Status    string // history status constant
	Reason    string // gate reason or meta detail
	Project   string
}

// ComputeOpsStats computes operational statistics from history records
// within the given time window. days controls the trend bucket count.
func ComputeOpsStats(records []HistoryRecord, since time.Time, days int) OpsStats {
	now := time.Now()
	stats := OpsStats{
		WindowStart:    since,
		WindowEnd:      now,
		DailyGated:     make([]int, days),
		DailySkipped:   make([]int, days),
		DailyProcessed: make([]int, days),
		DailyErrors:    make([]int, days),
	}

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for _, rec := range records {
		if rec.Timestamp.Before(since) {
			continue
		}

		dayOffset := daysBetween(todayStart, rec.Timestamp, now.Location())

		switch rec.Status {
		case HistoryStatusProcessed:
			stats.ProcessedRuns++
			if dayOffset >= 0 && dayOffset < days {
				stats.DailyProcessed[dayOffset]++
			}

			// Count gated runs (processed but evaluator skipped by policy).
			if rec.GateReason != "" {
				stats.GatedRuns++
				if dayOffset >= 0 && dayOffset < days {
					stats.DailyGated[dayOffset]++
				}
				switch rec.GateReason {
				case GateReasonUnclassified:
					stats.GatedUnclassified++
				case GateReasonPaused:
					stats.GatedPaused++
				}
			}

		case HistoryStatusError:
			stats.ErrorRuns++
			if dayOffset >= 0 && dayOffset < days {
				stats.DailyErrors[dayOffset]++
			}

		case HistoryStatusSkippedBusy:
			stats.SkippedBusy++
			if dayOffset >= 0 && dayOffset < days {
				stats.DailySkipped[dayOffset]++
			}

		case HistoryStatusMetaTriggered:
			stats.MetaTriggered++

		case HistoryStatusMetaCooldown:
			stats.MetaCooldowns++

		case HistoryStatusMetaNoThreshold:
			stats.MetaNoThreshold++
		}
	}

	// Build recent events list (newest first, limited to 50).
	stats.RecentEvents = buildRecentEvents(records, since, 50)

	return stats
}

// buildRecentEvents extracts operationally interesting events from history.
func buildRecentEvents(records []HistoryRecord, since time.Time, limit int) []OpsEvent {
	var events []OpsEvent

	for _, rec := range records {
		if rec.Timestamp.Before(since) {
			continue
		}

		// Only include operationally interesting events.
		switch {
		case rec.Status == HistoryStatusSkippedBusy:
			events = append(events, OpsEvent{
				Timestamp: rec.Timestamp,
				SessionID: rec.SessionID,
				Source:    rec.Source,
				Status:    rec.Status,
				Project:   rec.Project,
			})

		case rec.Status == HistoryStatusError:
			events = append(events, OpsEvent{
				Timestamp: rec.Timestamp,
				SessionID: rec.SessionID,
				Source:    rec.Source,
				Status:    rec.Status,
				Reason:    rec.ErrorDetail,
				Project:   rec.Project,
			})

		case rec.GateReason != "":
			events = append(events, OpsEvent{
				Timestamp: rec.Timestamp,
				SessionID: rec.SessionID,
				Source:    rec.Source,
				Status:    HistoryStatusProcessed,
				Reason:    rec.GateReason,
				Project:   rec.Project,
			})

		case rec.Status == HistoryStatusMetaTriggered,
			rec.Status == HistoryStatusMetaCooldown:
			events = append(events, OpsEvent{
				Timestamp: rec.Timestamp,
				Source:    rec.Source,
				Status:    rec.Status,
				Reason:    rec.ErrorDetail, // meta detail stored here
			})
		}
	}

	// Sort by timestamp descending (newest first).
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	if len(events) > limit {
		events = events[:limit]
	}

	return events
}

// daysBetween computes the number of days between todayStart and the record timestamp.
// Returns 0 for today, 1 for yesterday, etc.
func daysBetween(todayStart time.Time, ts time.Time, loc *time.Location) int {
	local := ts.In(loc)
	tsDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return int(todayStart.Sub(tsDay).Hours() / 24)
}
