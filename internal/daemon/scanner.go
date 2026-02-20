package daemon

import (
	"strings"

	"github.com/vladolaru/cabrero/internal/store"
)

// PendingSession holds a session ID and its project for batching.
type PendingSession struct {
	SessionID string
	Project   string
}

// ScanPending returns sessions that are ready for processing:
// status is "pending" and capture_trigger contains "session-end".
// Results are ordered oldest-first so the daemon processes in chronological order.
// Each result includes the session's project slug for batch grouping.
func ScanPending() ([]PendingSession, error) {
	sessions, err := store.ListSessions()
	if err != nil {
		return nil, err
	}

	var ready []PendingSession
	for _, s := range sessions {
		if s.Status != "pending" {
			continue
		}
		if !strings.Contains(s.CaptureTrigger, "session-end") {
			continue
		}
		if store.IsBlocked(s.SessionID) {
			continue
		}
		ready = append(ready, PendingSession{
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
