package daemon

import (
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

	var ready []QueuedSession
	for _, s := range sessions {
		if s.Status != "queued" {
			continue
		}
		if store.IsBlocked(s.SessionID) {
			continue
		}
		ready = append(ready, QueuedSession{
			SessionID: s.SessionID,
			Project:   s.Project,
		})
	}

	// ListSessions returns newest-first; reverse for oldest-first processing.
	for i, j := 0, len(ready)-1; i < j; i, j = i+1, j-1 {
		ready[i], ready[j] = ready[j], ready[i]
	}

	return ready, nil
}
