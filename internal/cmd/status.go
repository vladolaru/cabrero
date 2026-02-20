package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/daemon"
	"github.com/vladolaru/cabrero/internal/store"
)

// Status shows pipeline health and store overview.
func Status(args []string) error {
	root := store.Root()
	fmt.Println("Cabrero status")
	fmt.Println("──────────────")

	// Store path and status.
	if _, err := os.Stat(root); err == nil {
		fmt.Printf("Store:          %s (initialized)\n", root)
	} else {
		fmt.Printf("Store:          %s (not initialized)\n", root)
	}

	// Session counts.
	sessions, err := store.ListSessions()
	if err != nil {
		fmt.Printf("Sessions:       error reading (%v)\n", err)
	} else {
		queued := 0
		imported := 0
		processed := 0
		for _, s := range sessions {
			switch s.Status {
			case "queued":
				queued++
			case "imported":
				imported++
			case "processed":
				processed++
			}
		}
		fmt.Printf("Sessions:       %d captured, %d queued, %d imported, %d processed\n", len(sessions), queued, imported, processed)
	}

	// Blocklist count.
	blCount := store.BlocklistLen()
	fmt.Printf("Blocklist:      %d entries\n", blCount)

	// Last capture.
	if len(sessions) > 0 {
		latest := sessions[0]
		ts, err := time.Parse(time.RFC3339, latest.Timestamp)
		display := latest.Timestamp
		if err == nil {
			display = ts.Local().Format("2006-01-02 15:04")
		}
		shortID := latest.SessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		fmt.Printf("Last capture:   %s (session %s)\n", display, shortID)
	} else {
		fmt.Printf("Last capture:   none\n")
	}

	// Daemon status.
	if pid, alive := daemon.IsDaemonRunning(); alive {
		fmt.Printf("Daemon:         running (PID %d)\n", pid)
	} else {
		fmt.Printf("Daemon:         not running\n")
	}

	// Hook status.
	preCompact, sessionEnd := checkHooks()
	fmt.Printf("Hooks:          pre-compact %s   session-end %s\n",
		hookStatus(preCompact), hookStatus(sessionEnd))

	return nil
}

func hookStatus(installed bool) string {
	if installed {
		return "✓"
	}
	return "✗"
}

func checkHooks() (preCompact, sessionEnd bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, false
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, false
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, false
	}

	hooksRaw, ok := settings["hooks"]
	if !ok {
		return false, false
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		return false, false
	}

	preCompact = hookContainsCabrero(hooks["PreCompact"])
	sessionEnd = hookContainsCabrero(hooks["SessionEnd"])
	return
}

func hookContainsCabrero(raw json.RawMessage) bool {
	if raw == nil {
		return false
	}
	// The hook config is an array of matcher groups.
	return strings.Contains(string(raw), "cabrero")
}
