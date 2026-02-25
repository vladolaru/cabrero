package tui

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

func buildTestAppModel(t *testing.T) appModel {
	t.Helper()
	return newTestRoot()
}

func TestAppModel_Init_RequestsBackgroundColor(t *testing.T) {
	shared.InitStyles(true)
	m := buildTestAppModel(t)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() must return tea.RequestBackgroundColor cmd, got nil")
	}
}

func TestAppModel_BackgroundColorMsg_UpdatesStyles(t *testing.T) {
	shared.InitStyles(true) // start dark
	m := buildTestAppModel(t)
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Simulate a light terminal responding.
	lightColor := color.RGBA{R: 240, G: 240, B: 240, A: 255}
	m2, _ := m.Update(tea.BackgroundColorMsg{Color: lightColor})
	appM, ok := m2.(appModel)
	if !ok {
		t.Fatal("Update returned wrong type")
	}
	if appM.isDark {
		t.Error("isDark should be false after light BackgroundColorMsg")
	}
	if shared.IsDark {
		t.Error("shared.IsDark should be false after light BackgroundColorMsg")
	}
}
