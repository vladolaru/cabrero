package daemon

import "testing"

func TestEscapeAppleScript_Quotes(t *testing.T) {
	got := escapeAppleScript(`say "hello"`)
	want := `say \"hello\"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_Backslash(t *testing.T) {
	got := escapeAppleScript(`path\to\file`)
	want := `path\\to\\file`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_Newline(t *testing.T) {
	got := escapeAppleScript("line1\nline2")
	want := `line1\nline2`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_CarriageReturn(t *testing.T) {
	got := escapeAppleScript("line1\rline2")
	want := `line1\rline2`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_Unicode(t *testing.T) {
	// Multi-byte UTF-8 must pass through unchanged.
	got := escapeAppleScript("héllo wörld 🎉")
	want := "héllo wörld 🎉"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeAppleScript_Empty(t *testing.T) {
	if got := escapeAppleScript(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
