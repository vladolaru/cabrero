package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDaemonPipelineLoggerAdapter(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	log, err := NewLogger(logPath, 0)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer log.Close()

	adapter := &daemonPipelineLogger{log: log}

	adapter.Info("  Parsing session %s...", "abc123")
	adapter.Error("  Warning: failed to write proposal %s: %v", "prop-1", "disk full")

	log.Close() // flush

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "[INFO]") {
		t.Errorf("expected [INFO] in log, got:\n%s", content)
	}
	if !strings.Contains(content, "Parsing session abc123") {
		t.Errorf("expected Info message in log, got:\n%s", content)
	}
	if !strings.Contains(content, "[ERROR]") {
		t.Errorf("expected [ERROR] in log, got:\n%s", content)
	}
	if !strings.Contains(content, "failed to write proposal prop-1") {
		t.Errorf("expected Error message in log, got:\n%s", content)
	}
}
