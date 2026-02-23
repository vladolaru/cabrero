package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirm_Yes(t *testing.T) {
	m := NewConfirm("Apply this change?")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.Active {
		t.Error("model should be inactive after response")
	}
	if cmd == nil {
		t.Fatal("expected a command")
	}

	msg := cmd()
	result, ok := msg.(ConfirmResult)
	if !ok {
		t.Fatalf("expected ConfirmResult, got %T", msg)
	}
	if !result.Confirmed {
		t.Error("expected Confirmed=true")
	}
}

func TestConfirm_No(t *testing.T) {
	m := NewConfirm("Apply this change?")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.Active {
		t.Error("model should be inactive after response")
	}
	if cmd == nil {
		t.Fatal("expected a command")
	}

	msg := cmd()
	result, ok := msg.(ConfirmResult)
	if !ok {
		t.Fatalf("expected ConfirmResult, got %T", msg)
	}
	if result.Confirmed {
		t.Error("expected Confirmed=false")
	}
}

func TestConfirm_Esc(t *testing.T) {
	m := NewConfirm("Apply this change?")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.Active {
		t.Error("model should be inactive after Esc")
	}
	if cmd == nil {
		t.Fatal("expected a command")
	}

	msg := cmd()
	result, ok := msg.(ConfirmResult)
	if !ok {
		t.Fatalf("expected ConfirmResult, got %T", msg)
	}
	if result.Confirmed {
		t.Error("expected Confirmed=false on Esc")
	}
}

func TestConfirm_InactiveIgnoresInput(t *testing.T) {
	m := NewConfirm("Apply this change?")
	m.Active = false

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil {
		t.Error("inactive confirm should return nil cmd")
	}
}

func TestConfirm_View(t *testing.T) {
	m := NewConfirm("Apply?")
	v := m.View()
	if v != "Apply? [y/N] " {
		t.Errorf("View() = %q, want %q", v, "Apply? [y/N] ")
	}

	m.Active = false
	v = m.View()
	if v != "" {
		t.Errorf("inactive View() = %q, want empty", v)
	}
}

func TestConfirm_UnrecognizedKeyIgnored(t *testing.T) {
	m := NewConfirm("Apply?")

	// Keys not in y/Y/n/N/esc should be silently ignored.
	for _, r := range []rune{'a', 'b', 'm', 'x', '1'} {
		m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if !m2.Active {
			t.Errorf("pressing %q should not deactivate confirm", string(r))
		}
		if cmd != nil {
			t.Errorf("pressing %q should not produce a command", string(r))
		}
	}
}

func TestConfirm_ViewMatchesAcceptedKeys(t *testing.T) {
	m := NewConfirm("Delete this?")
	v := m.View()

	// The View advertises [y/N]. Verify the keys the view shows are
	// the exact keys the component actually accepts.
	if v != "Delete this? [y/N] " {
		t.Errorf("View() = %q, want prompt with [y/N] suffix", v)
	}

	// 'y' should confirm (matches [y] in prompt).
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m2.Active || cmd == nil {
		t.Error("'y' (shown in prompt) should be accepted")
	}

	// 'N' should decline (matches [N] in prompt).
	m3 := NewConfirm("Delete this?")
	m3, cmd = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if m3.Active || cmd == nil {
		t.Error("'N' (shown in prompt) should be accepted")
	}
}

func TestRevisionConfirm_View(t *testing.T) {
	m := NewRevisionConfirm("Which version?")
	v := m.View()

	want := "Which version? [o]riginal / [r]evision / [c]ancel "
	if v != want {
		t.Errorf("View() = %q, want %q", v, want)
	}

	m.Active = false
	v = m.View()
	if v != "" {
		t.Errorf("inactive View() = %q, want empty", v)
	}
}

func TestRevisionConfirm_UnrecognizedKeyIgnored(t *testing.T) {
	m := NewRevisionConfirm("Which version?")

	for _, r := range []rune{'a', 'y', 'n', 'x'} {
		m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if !m2.Active {
			t.Errorf("pressing %q should not deactivate revision confirm", string(r))
		}
		if cmd != nil {
			t.Errorf("pressing %q should not produce a command", string(r))
		}
	}
}

func TestRevisionConfirm_Original(t *testing.T) {
	m := NewRevisionConfirm("Which version?")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if m.Active {
		t.Error("model should be inactive")
	}

	msg := cmd()
	result, ok := msg.(RevisionChoice)
	if !ok {
		t.Fatalf("expected RevisionChoice, got %T", msg)
	}
	if result.Choice != "original" {
		t.Errorf("Choice = %q, want %q", result.Choice, "original")
	}
}

func TestRevisionConfirm_Revision(t *testing.T) {
	m := NewRevisionConfirm("Which version?")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	msg := cmd()
	result := msg.(RevisionChoice)
	if result.Choice != "revision" {
		t.Errorf("Choice = %q, want %q", result.Choice, "revision")
	}
}

func TestRevisionConfirm_Cancel(t *testing.T) {
	m := NewRevisionConfirm("Which version?")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	msg := cmd()
	result := msg.(RevisionChoice)
	if result.Choice != "cancel" {
		t.Errorf("Choice = %q, want %q", result.Choice, "cancel")
	}
}
