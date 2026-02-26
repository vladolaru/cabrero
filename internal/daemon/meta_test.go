package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

func TestPerformMetaRun_SkipsWhenBelowThreshold(t *testing.T) {
	tmp := t.TempDir()
	old := store.RootOverrideForTest(tmp)
	defer store.ResetRootOverrideForTest(old)

	logPath := filepath.Join(tmp, "test.log")
	log, err := NewLogger(logPath, 0)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer log.Close()

	d := &Daemon{
		config: DefaultConfig(),
		log:    log,
	}
	d.config.Pipeline.MetaMinSamples = 5
	d.config.Pipeline.MetaRejectionRateThreshold = 0.30
	d.config.Pipeline.Logger = &daemonPipelineLogger{log: log}

	// Ensure required dirs exist so store operations don't error.
	os.MkdirAll(filepath.Join(tmp, "proposals", "archived"), 0o755)

	// ComputePipelineMetrics will find no history → NaN FPR, empty versions.
	// Should log one line and return without error or LLM call.
	d.performMetaRun(context.Background()) // must not panic or call LLM

	// If we get here without panicking, the test passes.
	// Verify a log was written.
	log.Close()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected log output from performMetaRun")
	}
	_ = pipeline.DefaultPipelineConfig() // exercise DefaultPipelineConfig
}
