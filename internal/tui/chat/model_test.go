package chat

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

func newTestChat() Model {
	chips := []string{
		"Why was this flagged?",
		"Show the raw turns",
		"Conservative version",
		"Risk of approving?",
	}
	cfg := ChatConfig{
		SessionID:    "00000000-0000-0000-0000-000000000001",
		SystemPrompt: "Test proposal context.",
	}
	return New(chips, cfg, 60, 30)
}

func TestChat_ChipSend(t *testing.T) {
	m := newTestChat()

	// Pressing '1' should send the first chip.
	m, cmd := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
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
	m.input.Focus()
	m.input.SetValue("my question")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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

func TestBuildChatArgs_FirstMessage(t *testing.T) {
	cfg := ChatConfig{
		SessionID:    "test-uuid-123",
		SystemPrompt: "You are reviewing a proposal.",
		AllowedTools: "Read(//home/.cabrero/raw/abc/**),Grep(//home/.cabrero/raw/abc/**)",
	}

	args := buildChatArgs(cfg, true)

	assertFlag := func(flag, value string) {
		t.Helper()
		for i, a := range args {
			if a == flag && i+1 < len(args) && args[i+1] == value {
				return
			}
		}
		t.Errorf("args missing %s %q: %v", flag, value, args)
	}

	assertFlag("--session-id", "test-uuid-123")
	assertFlag("--system-prompt", "You are reviewing a proposal.")
	assertFlag("--allowedTools", cfg.AllowedTools)
	assertFlag("--permission-mode", "dontAsk")

	for _, a := range args {
		if a == "--resume" {
			t.Error("first message should not have --resume")
		}
	}
}

func TestBuildChatArgs_ResumeMessage(t *testing.T) {
	cfg := ChatConfig{
		SessionID:    "test-uuid-123",
		SystemPrompt: "You are reviewing a proposal.",
		AllowedTools: "Read(//home/.cabrero/raw/abc/**),Grep(//home/.cabrero/raw/abc/**)",
	}

	args := buildChatArgs(cfg, false)

	assertFlag := func(flag, value string) {
		t.Helper()
		for i, a := range args {
			if a == flag && i+1 < len(args) && args[i+1] == value {
				return
			}
		}
		t.Errorf("args missing %s %q: %v", flag, value, args)
	}

	assertFlag("--resume", "test-uuid-123")

	for _, a := range args {
		if a == "--session-id" {
			t.Error("resume should not have --session-id")
		}
		if a == "--system-prompt" {
			t.Error("resume should not have --system-prompt")
		}
		if a == "--allowedTools" {
			t.Error("resume should not have --allowedTools")
		}
		if a == "--permission-mode" {
			t.Error("resume should not have --permission-mode")
		}
	}
}

func TestBuildChatArgs_NoAllowedTools(t *testing.T) {
	cfg := ChatConfig{
		SessionID:    "test-uuid-456",
		SystemPrompt: "Prompt.",
	}

	args := buildChatArgs(cfg, true)

	for _, a := range args {
		if a == "--allowedTools" {
			t.Error("should not have --allowedTools when empty")
		}
	}
}

func TestChat_ViewportMutedWhenUnfocused(t *testing.T) {
	m := newTestChat()
	m.Focused = true
	m.addMessage("user", "Hello")
	m.addMessage("assistant", "World")

	focusedContent := m.viewport.View()

	// Focused content should contain the message text.
	if !strings.Contains(focusedContent, "You:") || !strings.Contains(focusedContent, "Hello") {
		t.Error("focused viewport should contain user message")
	}
	if !strings.Contains(focusedContent, "AI:") || !strings.Contains(focusedContent, "World") {
		t.Error("focused viewport should contain assistant message")
	}

	// Unfocused: content should still contain the text (muting preserves it).
	m.SetFocused(false)
	unfocusedContent := m.viewport.View()
	if !strings.Contains(unfocusedContent, "You:") || !strings.Contains(unfocusedContent, "Hello") {
		t.Error("unfocused viewport should still contain user message text")
	}
	if !strings.Contains(unfocusedContent, "AI:") || !strings.Contains(unfocusedContent, "World") {
		t.Error("unfocused viewport should still contain assistant message text")
	}

	// Re-focus: content should restore to original.
	m.SetFocused(true)
	refocusedContent := m.viewport.View()
	if refocusedContent != focusedContent {
		t.Error("re-focused viewport should match original focused content")
	}
}

func TestChat_SetFocused_NoopOnSameValue(t *testing.T) {
	m := newTestChat()
	m.Focused = true
	m.addMessage("user", "test")

	contentBefore := m.viewport.View()
	m.SetFocused(true) // same value — should be a no-op
	contentAfter := m.viewport.View()

	if contentBefore != contentAfter {
		t.Error("SetFocused with same value should not change viewport content")
	}
}

func TestChat_MuteANSI_EmptyString(t *testing.T) {
	result := muteANSI("")
	if result != "" {
		t.Errorf("muteANSI(\"\") = %q, want empty", result)
	}
}

func TestChat_MuteANSI_PreservesText(t *testing.T) {
	// muteANSI should strip ANSI codes and still contain the original text.
	input := "\x1b[1;34mYou:\x1b[0m Hello world"
	result := muteANSI(input)
	if !strings.Contains(result, "You:") || !strings.Contains(result, "Hello world") {
		t.Errorf("muteANSI should preserve text, got %q", result)
	}
}

func TestChat_MuteANSI_BlankLines(t *testing.T) {
	// Blank lines should remain blank (not wrapped in a style).
	input := "line1\n\nline3"
	result := muteANSI(input)
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[1] != "" {
		t.Errorf("blank line should remain empty, got %q", lines[1])
	}
}
