package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vladolaru/cabrero/internal/store"
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

func TestNewWiresPipelineLogger(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	logPath := filepath.Join(dir, "daemon.log")
	cfg := Config{
		LogPath: logPath,
		Pipeline: DefaultConfig().Pipeline,
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer d.log.Close()

	// Verify Pipeline.Logger is set to a daemonPipelineLogger.
	if d.config.Pipeline.Logger == nil {
		t.Fatal("Pipeline.Logger is nil after New()")
	}
	if _, ok := d.config.Pipeline.Logger.(*daemonPipelineLogger); !ok {
		t.Errorf("Pipeline.Logger is %T, want *daemonPipelineLogger", d.config.Pipeline.Logger)
	}

	// Verify it actually writes to the daemon log file.
	d.config.Pipeline.Logger.Info("wiring test %s", "ok")
	d.log.Close() // flush

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if !strings.Contains(string(data), "wiring test ok") {
		t.Errorf("expected 'wiring test ok' in daemon log, got:\n%s", string(data))
	}
}
