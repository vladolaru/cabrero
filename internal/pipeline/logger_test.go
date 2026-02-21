package pipeline

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestStdLoggerInfo(t *testing.T) {
	// Capture stdout via os.Pipe.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	stdLogger{}.Info("hello %s", "world")

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	got := buf.String()
	want := "hello world\n"
	if got != want {
		t.Errorf("stdLogger.Info output = %q, want %q", got, want)
	}
}

func TestStdLoggerError(t *testing.T) {
	// Capture stderr via os.Pipe.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	stdLogger{}.Error("warn %d", 42)

	w.Close()
	os.Stderr = origStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)

	got := buf.String()
	want := "warn 42\n"
	if got != want {
		t.Errorf("stdLogger.Error output = %q, want %q", got, want)
	}
}

func TestStdLoggerInfo_NoArgs(t *testing.T) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	stdLogger{}.Info("plain message")

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	got := buf.String()
	want := "plain message\n"
	if got != want {
		t.Errorf("stdLogger.Info output = %q, want %q", got, want)
	}
}

func TestPipelineConfigLogger_Nil(t *testing.T) {
	cfg := PipelineConfig{}
	log := cfg.logger()
	if log == nil {
		t.Fatal("logger() returned nil")
	}
	// Should be a stdLogger.
	if _, ok := log.(stdLogger); !ok {
		t.Errorf("logger() returned %T, want stdLogger", log)
	}
}

func TestPipelineConfigLogger_Custom(t *testing.T) {
	spy := &spyLogger{}
	cfg := PipelineConfig{Logger: spy}
	log := cfg.logger()
	if log != spy {
		t.Errorf("logger() returned %T, want *spyLogger", log)
	}
}

func TestDiscardLogger(t *testing.T) {
	// Smoke test: calling Info and Error on discardLogger must not panic.
	d := discardLogger{}
	d.Info("should not %s", "panic")
	d.Error("should not %s", "panic")
}

// spyLogger records all Info/Error calls for test assertions.
type spyLogger struct {
	infos  []string
	errors []string
}

func (s *spyLogger) Info(format string, args ...any) {
	s.infos = append(s.infos, fmt.Sprintf(format, args...))
}

func (s *spyLogger) Error(format string, args ...any) {
	s.errors = append(s.errors, fmt.Sprintf(format, args...))
}

func (s *spyLogger) hasInfo(substr string) bool {
	for _, msg := range s.infos {
		if strings.Contains(msg, substr) {
			return true
		}
	}
	return false
}

func (s *spyLogger) hasError(substr string) bool {
	for _, msg := range s.errors {
		if strings.Contains(msg, substr) {
			return true
		}
	}
	return false
}
