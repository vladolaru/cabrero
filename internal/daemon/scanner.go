package daemon

import (
	"slices"

	"github.com/vladolaru/cabrero/internal/store"
)

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
