package pipeline

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("NO_COLOR", "1")
	os.Exit(m.Run())
}
