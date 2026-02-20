package daemon

import (
	"strings"

	"github.com/vladolaru/cabrero/internal/store"
)

// ScanPending returns session IDs that are ready for processing:
// status is "pending" and capture_trigger contains "session-end".
// Results are ordered oldest-first so the daemon processes in chronological order.
func ScanPending() ([]string, error) {
	sessions, err := store.ListSessions()
	if err != nil {
		return nil, err
	}

	var ready []string
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
		ready = append(ready, s.SessionID)
	}

	// ListSessions returns newest-first; reverse for oldest-first processing.
	for i, j := 0, len(ready)-1; i < j; i, j = i+1, j-1 {
		ready[i], ready[j] = ready[j], ready[i]
	}

	return ready, nil
}
