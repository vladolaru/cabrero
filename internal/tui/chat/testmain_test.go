package chat

import (
	"os"
	"testing"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

func TestMain(m *testing.M) {
	os.Setenv("NO_COLOR", "1")
	shared.InitStyles(true) // seed styles for all tests in this package
	os.Exit(m.Run())
}
