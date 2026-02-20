package components

import "testing"

func TestRenderSparkline(t *testing.T) {
	tests := []struct {
		name   string
		data   []int
		width  int
		expect string
	}{
		{"empty", nil, 10, ""},
		{"single", []int{5}, 10, "█"},
		{"all zeros", []int{0, 0, 0}, 10, "▁▁▁"},
		{"ascending", []int{1, 2, 3, 4, 5, 6, 7, 8}, 10, "▁▂▃▄▅▆▇█"},
		{"mixed", []int{0, 4, 8, 4, 0}, 10, "▁▄█▄▁"},
		{"truncated to width", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 5, "▁▂▃▃▄"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderSparkline(tt.data, tt.width)
			if got != tt.expect {
				t.Errorf("RenderSparkline(%v, %d) = %q, want %q", tt.data, tt.width, got, tt.expect)
			}
		})
	}
}
