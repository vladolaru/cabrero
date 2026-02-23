package cmd

import (
	"github.com/vladolaru/cabrero/internal/tui"
)

// Dashboard launches the interactive dashboard TUI.
func Dashboard(args []string, version string) error {
	return tui.Run(version)
}
