package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

func TestRenderHeader_ContainsTitleAndDaemonStatus(t *testing.T) {
	stats := message.DashboardStats{
		Version:        "v0.19.0",
		DaemonRunning:  true,
		DaemonPID:      12345,
		HookPreCompact: true,
		HookSessionEnd: false,
	}

	header := RenderHeader(stats, 120)
	stripped := ansi.Strip(header)

	if !strings.Contains(stripped, "Cabrero") {
		t.Error("header should contain application name")
	}
	if !strings.Contains(stripped, "v0.19.0") {
		t.Error("header should contain version")
	}
	if !strings.Contains(stripped, "running") {
		t.Error("header should show daemon running status")
	}
	if !strings.Contains(stripped, "12345") {
		t.Error("header should show daemon PID")
	}
	_ = shared.MutedStyle // ensure shared is used
}

func TestRenderHeader_StoppedDaemon(t *testing.T) {
	stats := message.DashboardStats{DaemonRunning: false}
	header := RenderHeader(stats, 80)
	stripped := ansi.Strip(header)
	if !strings.Contains(stripped, "stopped") {
		t.Error("header should show daemon stopped status")
	}
}

func TestRenderHeader_NarrowLayout(t *testing.T) {
	stats := message.DashboardStats{DaemonRunning: true, DaemonPID: 1}
	// Width < 120 should use stacked layout (no horizontal join).
	header := RenderHeader(stats, 80)
	if header == "" {
		t.Error("RenderHeader should return non-empty output")
	}
}
