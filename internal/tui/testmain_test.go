package tui

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Disable colors in tests for consistent output across terminals and CI.
	os.Setenv("NO_COLOR", "1")
	os.Exit(m.Run())
}
