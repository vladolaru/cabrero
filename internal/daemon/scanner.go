package daemon

import (
	"slices"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// danglingQueuedThreshold is how long a queued session can remain without a
// transcript before being marked as capture_failed. Generous to allow for slow
// transcript copies or delayed pre-compact → session-end sequences.
const danglingQueuedThreshold = 1 * time.Hour

// QueuedSession holds a session ID and its project for batching.
type QueuedSession struct {
	SessionID string
	Project   string
}

// ScanQueued returns sessions with status "queued" that are ready for processing.
// Results are ordered oldest-first so the daemon processes in chronological order.
func ScanQueued() ([]QueuedSession, error) {
	sessions, err := store.ListSessions()
	if err != nil {
		return nil, err
	}

	blocked, err := store.ReadBlocklist()
	if err != nil {
		return nil, err
	}

	var ready []QueuedSession
	for _, s := range sessions {
		if s.Status != store.StatusQueued {
			continue
		}
		if blocked[s.SessionID] {
			continue
		}
		if !store.TranscriptExists(s.SessionID) {
			continue
		}
		ready = append(ready, QueuedSession{
			SessionID: s.SessionID,
			Project:   s.Project,
		})
	}

	// ListSessions returns newest-first; reverse for oldest-first processing.
	slices.Reverse(ready)

	return ready, nil
}

// CleanDanglingQueued marks queued sessions as capture_failed if they have no
// transcript and have been queued for longer than danglingQueuedThreshold.
// This handles cases where the hook wrote metadata but the transcript copy
// failed. Returns the number of sessions cleaned up.
func CleanDanglingQueued(log *Logger) int {
	sessions, err := store.ListSessions()
	if err != nil {
		return 0
	}

	now := time.Now()
	cleaned := 0

	for _, s := range sessions {
		if s.Status != store.StatusQueued {
			continue
		}
		if store.TranscriptExists(s.SessionID) {
			continue
		}
		ts, err := time.Parse(time.RFC3339, s.Timestamp)
		if err != nil {
			// Try alternate format without timezone.
			ts, err = time.Parse("2006-01-02T15:04:05Z", s.Timestamp)
			if err != nil {
				continue // unparseable timestamp, skip
			}
		}
		if now.Sub(ts) < danglingQueuedThreshold {
			continue
		}
		if err := store.MarkCaptureFailed(s.SessionID); err != nil {
			log.Error("dangling queue cleanup: failed to mark %s as capture_failed: %v", s.SessionID, err)
			continue
		}
		cleaned++
	}

	return cleaned
}
