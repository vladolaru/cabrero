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
