package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/daemon"
	"github.com/vladolaru/cabrero/internal/store"
)

// Status shows pipeline health and store overview.
func Status(args []string) error {
	root := store.Root()
	fmt.Println()
	fmt.Println(cli.Bold("Cabrero Status"))
	fmt.Println(cli.Accent(strings.Repeat("─", 30)))

	// Store path and status.
	home, _ := os.UserHomeDir()
	display := strings.Replace(root, home, "~", 1)
	if _, err := os.Stat(root); err == nil {
		fmt.Printf("  %s  %s %s\n", cli.Bold("Store:"), display, cli.Success("(initialized)"))
	} else {
		fmt.Printf("  %s  %s %s\n", cli.Bold("Store:"), display, cli.Warn("(not initialized)"))
	}

	// Session counts.
	sessions, err := store.ListSessions()
	if err != nil {
		fmt.Printf("  %s  %s\n", cli.Bold("Sessions:"), cli.Error(fmt.Sprintf("error reading (%v)", err)))
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
		fmt.Printf("  %s  %d captured, %d queued, %d imported, %d processed\n",
			cli.Bold("Sessions:"), len(sessions), queued, imported, processed)
	}

	// Blocklist count.
	blCount := store.BlocklistLen()
	fmt.Printf("  %s  %d entries\n", cli.Bold("Blocklist:"), blCount)

	// Last capture.
	if len(sessions) > 0 {
		latest := sessions[0]
		ts, err := time.Parse(time.RFC3339, latest.Timestamp)
		capDisplay := latest.Timestamp
		if err == nil {
			capDisplay = ts.Local().Format("2006-01-02 15:04")
		}
		shortID := latest.SessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		fmt.Printf("  %s  %s %s\n", cli.Bold("Last capture:"), capDisplay, cli.Muted("(session "+shortID+")"))
	} else {
		fmt.Printf("  %s  %s\n", cli.Bold("Last capture:"), cli.Muted("none"))
	}

	// Daemon status.
	if pid, alive := daemon.IsDaemonRunning(); alive {
		fmt.Printf("  %s  %s %s\n", cli.Bold("Daemon:"), cli.Success("running"), cli.Muted(fmt.Sprintf("(PID %d)", pid)))
	} else {
		fmt.Printf("  %s  %s\n", cli.Bold("Daemon:"), cli.Warn("not running"))
	}

	// Hook status.
	preCompact, sessionEnd := checkHooks()
	fmt.Printf("  %s  pre-compact %s   session-end %s\n",
		cli.Bold("Hooks:"), hookStatus(preCompact), hookStatus(sessionEnd))

	// Debug mode.
	if store.ReadDebugFlag() {
		fmt.Printf("  %s  %s %s\n", cli.Bold("Debug:"), cli.Warn("enabled"), cli.Muted("(via config)"))
	}
	fmt.Println()

	return nil
}

func hookStatus(installed bool) string {
	if installed {
		return cli.Success("✓")
	}
	return cli.Error("✗")
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
