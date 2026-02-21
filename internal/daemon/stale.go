package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

const staleThreshold = 24 * time.Hour

// ScanStale scans ~/.claude/projects/ for session JSONL files that are not in
// the store (hooks never fired, e.g. due to a crash) and idle for over 24 hours.
// Recovered sessions are written to the store with trigger "stale-recovery".
// Returns the number of sessions recovered.
//
// Only reads two known levels where JSONL files exist:
//   - <project>/<session>.jsonl
//   - <project>/<session>/subagents/<agent>.jsonl
//
// This avoids descending into tool-results/ and other subdirectories that can
// trigger macOS network volume access prompts.
func ScanStale(log *Logger) (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return 0, nil
	}

	blocked, err := store.ReadBlocklist()
	if err != nil {
		return 0, err
	}

	recovered := 0
	now := time.Now()

	projects, err := os.ReadDir(projectsDir)
	if err != nil {
		return 0, err
	}

	for _, proj := range projects {
		if !proj.IsDir() {
			continue
		}
		projectDir := filepath.Join(projectsDir, proj.Name())

		entries, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				// Check for subagents/ inside session directories.
				scanStaleSubagents(filepath.Join(projectDir, entry.Name()), proj.Name(), blocked, now, log, &recovered)
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			recoverStaleSession(filepath.Join(projectDir, entry.Name()), entry, proj.Name(), blocked, now, log, &recovered)
		}
	}

	return recovered, nil
}

// scanStaleSubagents checks <sessionDir>/subagents/ for agent JSONL files.
func scanStaleSubagents(sessionDir, project string, blocked map[string]bool, now time.Time, log *Logger, recovered *int) {
	subDir := filepath.Join(sessionDir, "subagents")
	entries, err := os.ReadDir(subDir)
	if err != nil {
		return // no subagents dir or unreadable
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		recoverStaleSession(filepath.Join(subDir, entry.Name()), entry, project, blocked, now, log, recovered)
	}
}

func recoverStaleSession(path string, entry os.DirEntry, project string, blocked map[string]bool, now time.Time, log *Logger, recovered *int) {
	sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
	if sessionID == "" || len(sessionID) < 8 {
		return
	}

	if store.SessionExists(sessionID) {
		return
	}
	if blocked[sessionID] {
		return
	}

	info, err := entry.Info()
	if err != nil {
		return
	}

	// Only recover sessions idle for over 24 hours (likely finished).
	if now.Sub(info.ModTime()) < staleThreshold {
		return
	}

	if err := store.WriteSession(sessionID, path, "stale-recovery", "", info.ModTime(), project); err != nil {
		log.Error("stale recovery: failed to import %s: %v", sessionID, err)
		return
	}
	if err := store.MarkQueued(sessionID); err != nil {
		log.Error("stale recovery: failed to queue %s: %v", sessionID, err)
	}

	log.Info("stale recovery: imported session %s (idle since %s)", sessionID, info.ModTime().Format("2006-01-02 15:04"))
	*recovered++
}
