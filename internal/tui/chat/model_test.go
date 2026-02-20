package chat

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

func newTestChat() Model {
	chips := []string{
		"Why was this flagged?",
		"Show the raw turns",
		"Conservative version",
		"Risk of approving?",
	}
	return New(chips, `{"citations": []}`, 60, 30)
}

func TestChat_ChipSend(t *testing.T) {
	m := newTestChat()

	// Pressing '1' should send the first chip.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if cmd == nil {
		t.Fatal("expected a cmd from chip press")
	}

	// Should have added a user message.
	if len(m.messages) != 1 {
		t.Fatalf("messages count = %d, want 1", len(m.messages))
	}
	if m.messages[0].Role != "user" {
		t.Errorf("role = %q, want %q", m.messages[0].Role, "user")
	}
	if m.messages[0].Content != "Why was this flagged?" {
		t.Errorf("content = %q, want first chip text", m.messages[0].Content)
	}

	// Should be streaming.
	if !m.streaming {
		t.Error("should be streaming after sending")
	}
}

func TestChat_ChipsHideAfterManualInput(t *testing.T) {
	m := newTestChat()

	if !m.chipsVisible {
		t.Fatal("chips should be visible initially")
	}

	// Focus input and type a message.
	m.Focus()
	m.input.SetValue("my question")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.chipsVisible {
		t.Error("chips should be hidden after manual input")
	}
}

func TestChat_RevisionParsing(t *testing.T) {
	response := "Here's a revised version:\n\n```revision\n- old line\n+ new line\n```\n\nDoes that look better?"

	rev := parseRevision(response)
	if rev == nil {
		t.Fatal("expected revision to be parsed")
	}
	if *rev != "- old line\n+ new line" {
		t.Errorf("revision = %q, want %q", *rev, "- old line\n+ new line")
	}
}

func TestChat_RevisionIgnoresDiff(t *testing.T) {
	response := "Here's a diff:\n\n```diff\n- old line\n+ new line\n```\n\nThis is illustrative."

	rev := parseRevision(response)
	if rev != nil {
		t.Error("```diff``` blocks should NOT be treated as revisions")
	}
}

func TestChat_MalformedRevision(t *testing.T) {
	response := "Here:\n\n```revision\n```\n\nEmpty revision."

	rev := parseRevision(response)
	if rev != nil {
		t.Error("empty revision block should return nil")
	}
}

func TestChat_MultipleRevisions(t *testing.T) {
	response := "First:\n\n```revision\nfirst version\n```\n\nActually:\n\n```revision\nfinal version\n```"

	rev := parseRevision(response)
	if rev == nil {
		t.Fatal("expected revision")
	}
	if *rev != "final version" {
		t.Errorf("revision = %q, want last block %q", *rev, "final version")
	}
}

func TestChat_StreamDoneDetectsRevision(t *testing.T) {
	m := newTestChat()
	m.streaming = true
	m.streamBuf.WriteString("Here:\n\n```revision\n+ improved line\n```\n")

	m, _ = m.Update(message.ChatStreamDone{FullResponse: m.streamBuf.String()})

	if m.streaming {
		t.Error("should not be streaming after done")
	}
	if !m.HasRevision() {
		t.Error("should have detected revision")
	}
}

func TestChat_StreamError(t *testing.T) {
	m := newTestChat()
	m.streaming = true

	m, _ = m.Update(message.ChatStreamError{Err: fmt.Errorf("connection failed")})

	if m.streaming {
		t.Error("should not be streaming after error")
	}
	if len(m.messages) != 1 {
		t.Fatalf("messages count = %d, want 1", len(m.messages))
	}
	if m.messages[0].Role != "assistant" {
		t.Errorf("role = %q, want assistant", m.messages[0].Role)
	}
}
