package shared

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

func TestKeyMap_Arrows(t *testing.T) {
	km := NewKeyMap("arrows")

	tests := []struct {
		name    string
		binding key.Binding
		wantKey string
	}{
		{"Up", km.Up, "up"},
		{"Down", km.Down, "down"},
		{"Left", km.Left, "left"},
		{"Right", km.Right, "right"},
		{"HalfPageUp", km.HalfPageUp, "pgup"},
		{"HalfPageDown", km.HalfPageDown, "pgdown"},
		{"GotoTop", km.GotoTop, "home"},
		{"GotoBottom", km.GotoBottom, "end"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := tt.binding.Keys()
			if len(keys) == 0 {
				t.Fatalf("no keys bound for %s", tt.name)
			}
			if keys[0] != tt.wantKey {
				t.Errorf("keys[0] = %q, want %q", keys[0], tt.wantKey)
			}
		})
	}
}

func TestKeyMap_Vim(t *testing.T) {
	km := NewKeyMap("vim")

	tests := []struct {
		name      string
		binding   key.Binding
		wantFirst string
		wantLen   int
	}{
		{"Up", km.Up, "k", 2},           // k + up fallback
		{"Down", km.Down, "j", 2},       // j + down fallback
		{"Left", km.Left, "h", 2},       // h + left fallback
		{"Right", km.Right, "l", 2},     // l + right fallback
		{"HalfPageUp", km.HalfPageUp, "ctrl+u", 1},
		{"HalfPageDown", km.HalfPageDown, "ctrl+d", 1},
		{"GotoTop", km.GotoTop, "g", 1},
		{"GotoBottom", km.GotoBottom, "G", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := tt.binding.Keys()
			if len(keys) != tt.wantLen {
				t.Fatalf("len(keys) = %d, want %d; keys = %v", len(keys), tt.wantLen, keys)
			}
			if keys[0] != tt.wantFirst {
				t.Errorf("keys[0] = %q, want %q", keys[0], tt.wantFirst)
			}
		})
	}
}

func TestKeyMap_ActionKeysIdentical(t *testing.T) {
	arrows := NewKeyMap("arrows")
	vim := NewKeyMap("vim")

	// Action keys must be identical regardless of navigation mode.
	pairs := []struct {
		name string
		a, v key.Binding
	}{
		{"Approve", arrows.Approve, vim.Approve},
		{"Reject", arrows.Reject, vim.Reject},
		{"Defer", arrows.Defer, vim.Defer},
		{"Open", arrows.Open, vim.Open},
		{"Chat", arrows.Chat, vim.Chat},
		{"Quit", arrows.Quit, vim.Quit},
		{"Help", arrows.Help, vim.Help},
	}

	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			aKeys := p.a.Keys()
			vKeys := p.v.Keys()
			if len(aKeys) != len(vKeys) {
				t.Fatalf("key count differs: arrows=%v vim=%v", aKeys, vKeys)
			}
			for i := range aKeys {
				if aKeys[i] != vKeys[i] {
					t.Errorf("key[%d] differs: arrows=%q vim=%q", i, aKeys[i], vKeys[i])
				}
			}
		})
	}
}

func TestKeyMap_HelpBindings(t *testing.T) {
	km := NewKeyMap("arrows")

	short := km.ShortHelp()
	if len(short) == 0 {
		t.Fatal("ShortHelp returned empty")
	}

	full := km.FullHelp()
	if len(full) == 0 {
		t.Fatal("FullHelp returned empty")
	}

	detail := km.DetailShortHelp()
	if len(detail) == 0 {
		t.Fatal("DetailShortHelp returned empty")
	}
}
