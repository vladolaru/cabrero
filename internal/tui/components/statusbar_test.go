package components

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
)

func makeBinding(k, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc))
}

func TestRenderStatusBar_FitsWidth(t *testing.T) {
	bindings := []key.Binding{
		makeBinding("?", "help"),
		makeBinding("q", "quit"),
		makeBinding("enter", "open"),
		makeBinding("esc", "back"),
		makeBinding("j", "down"),
	}
	width := 40
	result := RenderStatusBar(bindings, "", width)
	w := ansi.StringWidth(result)
	if w > width {
		t.Errorf("RenderStatusBar width = %d, want ≤ %d", w, width)
	}
}

func TestRenderStatusBar_TimedMsgOverride(t *testing.T) {
	bindings := []key.Binding{makeBinding("q", "quit")}
	result := RenderStatusBar(bindings, "1/3 matches", 60)
	stripped := strings.TrimSpace(ansi.Strip(result))
	if stripped != "1/3 matches" {
		t.Errorf("expected timedMsg in output, got %q", stripped)
	}
}

func TestRenderStatusBar_DropsBindingsToFit(t *testing.T) {
	bindings := []key.Binding{
		makeBinding("ctrl+a", "select all"),
		makeBinding("ctrl+b", "bold"),
		makeBinding("ctrl+c", "copy"),
	}
	width := 10
	result := RenderStatusBar(bindings, "", width)
	w := ansi.StringWidth(result)
	if w > width {
		t.Errorf("RenderStatusBar with narrow width: got %d, want ≤ %d", w, width)
	}
}
