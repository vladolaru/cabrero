package main

import (
	"strings"
	"testing"
)

// countLines returns the number of visible lines in a rendered string.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n") + 1
	if strings.HasSuffix(s, "\n") {
		n--
	}
	return n
}

// TestSnapshot_HeightInvariant verifies that each view's rendered output does
// not exceed the declared terminal height. Views that fill remaining space with
// padding must produce exactly h lines; other views must not exceed h.
func TestSnapshot_HeightInvariant(t *testing.T) {
	tests := []struct {
		name     string
		w, h     int
		exactFit bool // true = must equal h; false = must not exceed h
	}{
		// Views with fill-to-height padding.
		{"dashboard", 120, 40, true},
		{"dashboard-narrow", 80, 24, true},
		{"dashboard-empty", 120, 40, true},
		{"proposal-detail", 120, 40, true},
		{"proposal-detail-chat", 160, 40, true},
		{"fitness-report", 120, 40, true},
		{"source-manager", 120, 40, true},
		{"pipeline-monitor", 120, 40, true},

		// Views that don't fill to height.
		{"help-overlay", 120, 40, false},
		{"help-overlay-vim", 120, 40, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := render(tt.name, tt.w, tt.h)
			if err != nil {
				t.Fatalf("render error: %v", err)
			}

			lines := countLines(output)
			if lines > tt.h {
				t.Errorf("output exceeds height: got %d lines, want at most %d", lines, tt.h)
			}
			if tt.exactFit && lines != tt.h {
				t.Errorf("output height mismatch: got %d lines, want exactly %d", lines, tt.h)
			}
		})
	}
}
