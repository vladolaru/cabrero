package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

const staleThreshold = 24 * time.Hour

// ScanStale walks ~/.claude/projects/ for session JSONL files that are not in
// the store (hooks never fired, e.g. due to a crash) and idle for over 24 hours.
// Recovered sessions are written to the store with trigger "stale-recovery".
// Returns the number of sessions recovered.
func ScanStale(log *Logger) (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return 0, nil
	}

	recovered := 0
	now := time.Now()

	err = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}

		sessionID := strings.TrimSuffix(info.Name(), ".jsonl")
		if sessionID == "" || len(sessionID) < 8 {
			return nil
		}

		// Skip if already in store or blocklist.
		if store.SessionExists(sessionID) {
			return nil
		}
		if store.IsBlocked(sessionID) {
			return nil
		}

		// Only recover sessions idle for over 24 hours (likely finished).
		if now.Sub(info.ModTime()) < staleThreshold {
			return nil
		}

		// Extract project slug from parent directory name.
		project := filepath.Base(filepath.Dir(path))
		if project == filepath.Base(projectsDir) {
			project = ""
		}

		if err := store.WriteSession(sessionID, path, "stale-recovery", "", info.ModTime(), project); err != nil {
			log.Error("stale recovery: failed to import %s: %v", sessionID, err)
			return nil
		}
		// Stale-recovered sessions should be processed by the daemon.
		if err := store.MarkQueued(sessionID); err != nil {
			log.Error("stale recovery: failed to queue %s: %v", sessionID, err)
		}

		log.Info("stale recovery: imported session %s (idle since %s)", sessionID, info.ModTime().Format("2006-01-02 15:04"))
		recovered++
		return nil
	})

	return recovered, err
}
