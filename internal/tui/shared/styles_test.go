package shared

import "testing"

func TestInitStyles_SetsIsDark(t *testing.T) {
	InitStyles(true)
	if !IsDark {
		t.Error("IsDark should be true after InitStyles(true)")
	}
	InitStyles(false)
	if IsDark {
		t.Error("IsDark should be false after InitStyles(false)")
	}
}

func TestInitStyles_StylesRenderable(t *testing.T) {
	// Styles must be non-zero after InitStyles — Render must not panic.
	InitStyles(true)
	_ = SuccessStyle.Render("ok")
	_ = ErrorStyle.Render("err")
	_ = AccentBoldStyle.Render("section")
	_ = MutedStyle.Render("muted")
}

func TestReinitStyles_ChangesOutput(t *testing.T) {
	InitStyles(false)
	lightOut := SuccessStyle.Render("x")

	ReinitStyles(true)
	darkOut := SuccessStyle.Render("x")

	if lightOut == darkOut {
		t.Error("ReinitStyles should change ANSI output when isDark changes")
	}
}

func TestHighlightBg_DependsOnIsDark(t *testing.T) {
	InitStyles(false)
	light := HighlightBg()

	InitStyles(true)
	dark := HighlightBg()

	if light == dark {
		t.Error("HighlightBg should differ between light and dark")
	}
}
