package cmd

import (
	"github.com/vladolaru/cabrero/internal/tui"
)

// Review launches the interactive review TUI.
func Review(args []string) error {
	return tui.Run()
}
