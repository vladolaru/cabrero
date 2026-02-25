package sources

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// testGroups returns source groups for testing:
// - "User-level" with 2 classified sources
// - "Project: acme" with 1 classified source
// - "Unclassified" with 1 unclassified source
func testGroups() []fitness.SourceGroup {
	return []fitness.SourceGroup{
		{
			Label:  "User-level",
			Origin: "user",
			Sources: []fitness.Source{
				{
					Name:         "docx-helper",
					Origin:       "user",
					Ownership:    "mine",
					Approach:     "iterate",
					SessionCount: 12,
					HealthScore:  85.0,
				},
				{
					Name:         "git-workflow",
					Origin:       "user",
					Ownership:    "mine",
					Approach:     "evaluate",
					SessionCount: 8,
					HealthScore:  62.0,
				},
			},
		},
		{
			Label:  "Project: acme",
			Origin: "project:acme",
			Sources: []fitness.Source{
				{
					Name:         "acme-conventions",
					Origin:       "project:acme",
					Ownership:    "not_mine",
					Approach:     "evaluate",
					SessionCount: 5,
					HealthScore:  45.0,
				},
			},
		},
		{
			Label:  "\u26a0 Unclassified",
			Origin: "",
			Sources: []fitness.Source{
				{
					Name:         "unknown-skill",
					Origin:       "user",
					Ownership:    "",
					Approach:     "",
					SessionCount: 2,
					HealthScore:  -1,
				},
			},
		},
	}
}

func newTestModel() Model {
	keys := shared.NewKeyMap("arrows")
	cfg := shared.DefaultConfig()
	return New(testGroups(), &keys, cfg)
}

func TestSources_Navigation(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Expected flat items (3 groups expanded, default config):
	// 0: header "User-level"
	// 1: source "docx-helper"
	// 2: source "git-workflow"
	// 3: header "Project: acme"
	// 4: source "acme-conventions"
	// 5: header "Unclassified"
	// 6: source "unknown-skill"
	if len(m.flatItems) != 7 {
		t.Fatalf("flatItems count = %d, want 7", len(m.flatItems))
	}

	// Start at top (header).
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}

	// Cursor on header: SelectedSource returns nil.
	if s := m.SelectedSource(); s != nil {
		t.Error("cursor on header should return nil SelectedSource")
	}

	// Move down to first source.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("cursor after first down = %d, want 1", m.cursor)
	}

	s := m.SelectedSource()
	if s == nil {
		t.Fatal("SelectedSource nil at cursor 1")
	}
	if s.Name != "docx-helper" {
		t.Errorf("source at cursor 1 = %q, want %q", s.Name, "docx-helper")
	}

	// Move down through list.
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursor != 6 {
		t.Errorf("cursor after navigating to end = %d, want 6", m.cursor)
	}

	// At bottom — should not go further.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 6 {
		t.Errorf("cursor should stay at 6 (bottom), got %d", m.cursor)
	}

	// Move up.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 5 {
		t.Errorf("cursor after up = %d, want 5", m.cursor)
	}

	// At top — should not go further.
	m.cursor = 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor should stay at 0 (top), got %d", m.cursor)
	}
}

func TestSources_GroupCollapse(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Start with all groups expanded: 7 flat items.
	if len(m.flatItems) != 7 {
		t.Fatalf("initial flatItems = %d, want 7", len(m.flatItems))
	}

	// Cursor on "User-level" header (index 0). Collapse with Left.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})

	// User-level collapsed: header + (Project: acme header + source + Unclassified header + source) = 5.
	if len(m.flatItems) != 5 {
		t.Fatalf("after collapse flatItems = %d, want 5", len(m.flatItems))
	}

	// Verify the first group is collapsed.
	if !m.groups[0].Collapsed {
		t.Error("User-level group should be collapsed")
	}

	// Expand with Right.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if len(m.flatItems) != 7 {
		t.Fatalf("after expand flatItems = %d, want 7", len(m.flatItems))
	}
	if m.groups[0].Collapsed {
		t.Error("User-level group should be expanded after Right")
	}

	// Navigate to a source (cursor 1) and collapse its parent group.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor -> 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})  // collapse parent group

	if !m.groups[0].Collapsed {
		t.Error("parent group should be collapsed when cursor is on child source")
	}

	// Cursor should be clamped within range.
	if m.cursor >= len(m.flatItems) {
		t.Errorf("cursor %d out of range after collapse (len=%d)", m.cursor, len(m.flatItems))
	}
}

func TestSources_ToggleApproach(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to "docx-helper" (cursor 1).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	s := m.SelectedSource()
	if s == nil || s.Name != "docx-helper" {
		t.Fatal("expected docx-helper at cursor 1")
	}
	if s.Approach != "iterate" {
		t.Fatalf("initial approach = %q, want iterate", s.Approach)
	}

	// Press 't' to toggle — should activate confirmation.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	if m.confirmState != ConfirmToggleApproach {
		t.Fatalf("confirmState = %d, want ConfirmToggleApproach", m.confirmState)
	}
	if !m.confirm.Active {
		t.Fatal("confirm should be active")
	}

	// The rendered view should show the confirm prompt with [y/N].
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "evaluate") {
		t.Error("toggle confirm View() should mention the target approach")
	}
	if !strings.Contains(view, "[y/N]") {
		t.Error("toggle confirm View() should show [y/N] prompt")
	}

	// Confirm with 'y' — the key the prompt advertises.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if cmd == nil {
		t.Fatal("expected cmd after pressing y")
	}

	// Feed the ConfirmResult message back.
	confirmMsg := cmd()
	m, cmd = m.Update(confirmMsg)

	if m.confirm.Active {
		t.Error("confirm should be inactive after confirming")
	}
	if m.confirmState != ConfirmNone {
		t.Errorf("confirmState = %d, want ConfirmNone after confirm", m.confirmState)
	}

	// The cmd should emit ToggleApproachFinished.
	if cmd == nil {
		t.Fatal("expected cmd after confirmation")
	}
	msg := cmd()
	toggle, ok := msg.(message.ToggleApproachFinished)
	if !ok {
		t.Fatalf("expected ToggleApproachFinished, got %T", msg)
	}
	if toggle.SourceName != "docx-helper" {
		t.Errorf("SourceName = %q, want docx-helper", toggle.SourceName)
	}
	if toggle.NewApproach != "evaluate" {
		t.Errorf("NewApproach = %q, want evaluate", toggle.NewApproach)
	}

	// Feed the finished message back to update local state.
	m, _ = m.Update(toggle)
	if m.groups[0].Sources[0].Approach != "evaluate" {
		t.Errorf("source approach after toggle = %q, want evaluate", m.groups[0].Sources[0].Approach)
	}
}

func TestSources_ToggleApproach_Decline(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to "docx-helper".
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press 't' to toggle.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	// Decline with 'n' — produces a cmd with ConfirmResult{Confirmed: false}.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if cmd != nil {
		// Feed the ConfirmResult back.
		confirmMsg := cmd()
		m, cmd = m.Update(confirmMsg)
	}

	if m.confirm.Active {
		t.Error("confirm should be inactive after declining")
	}
	if cmd != nil {
		t.Error("no cmd should be emitted after declining")
	}

	// Approach should remain unchanged.
	if m.groups[0].Sources[0].Approach != "iterate" {
		t.Errorf("approach should still be iterate, got %q", m.groups[0].Sources[0].Approach)
	}
}

func TestSources_ToggleApproach_Unclassified(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to unclassified source (cursor 6).
	m.cursor = 6

	// Press 't' — should work even for unclassified sources.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	if m.confirmState != ConfirmToggleApproach {
		t.Errorf("confirmState = %d, want ConfirmToggleApproach", m.confirmState)
	}
	if !m.confirm.Active {
		t.Fatal("confirm should be active")
	}
}

func TestSources_PreSelect(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// PreSelect "acme-conventions" — should be at flat index 4.
	m = m.PreSelectSource("acme-conventions")

	if m.cursor != 4 {
		t.Errorf("cursor after PreSelect = %d, want 4", m.cursor)
	}

	s := m.SelectedSource()
	if s == nil {
		t.Fatal("SelectedSource nil after PreSelect")
	}
	if s.Name != "acme-conventions" {
		t.Errorf("selected source = %q, want acme-conventions", s.Name)
	}

	// PreSelect nonexistent name — cursor should not change.
	m = m.PreSelectSource("nonexistent")
	if m.cursor != 4 {
		t.Errorf("cursor after nonexistent PreSelect = %d, should stay at 4", m.cursor)
	}
}

func TestSources_EmptyState(t *testing.T) {
	keys := shared.NewKeyMap("arrows")
	cfg := shared.DefaultConfig()
	m := New(nil, &keys, cfg)
	m.SetSize(100, 24)

	view := ansi.Strip(m.View())

	if !strings.Contains(view, "No sources tracked") {
		t.Error("empty state should show 'No sources tracked'")
	}
	// Source Manager header and counts are now in SubHeader(), rendered by root model.
	subHeader := ansi.Strip(m.SubHeader())
	if !strings.Contains(subHeader, "Source Manager") {
		t.Error("empty state should still show the header in sub-header")
	}
	if !strings.Contains(subHeader, "0 sources") {
		t.Error("empty state should show '0 sources' count in sub-header")
	}
}

func TestSources_View_AdaptiveColumns(t *testing.T) {
	m := newTestModel()

	// Wide layout (>=120): should show all columns including HEALTH.
	m.SetSize(130, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "HEALTH") {
		t.Error("wide layout should show HEALTH column")
	}
	if !strings.Contains(view, "OWNERSHIP") {
		t.Error("wide layout should show OWNERSHIP column")
	}

	// Standard layout (80-119): no HEALTH column.
	m.SetSize(100, 40)
	view = ansi.Strip(m.View())
	if strings.Contains(view, "HEALTH") {
		t.Error("standard layout should not show HEALTH column")
	}
	if !strings.Contains(view, "OWNERSHIP") {
		t.Error("standard layout should show OWNERSHIP column")
	}

	// Narrow layout (<80): only name, approach, sessions.
	m.SetSize(60, 40)
	view = ansi.Strip(m.View())
	if strings.Contains(view, "HEALTH") {
		t.Error("narrow layout should not show HEALTH column")
	}
	if strings.Contains(view, "OWNERSHIP") {
		t.Error("narrow layout should not show OWNERSHIP column")
	}
	if !strings.Contains(view, "APPROACH") {
		t.Error("narrow layout should show APPROACH column")
	}
}

func TestSources_DetailAndRollback(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to "docx-helper" and open detail.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.detailOpen {
		t.Fatal("detail should be open after Enter on source")
	}
	if m.detailSource == nil {
		t.Fatal("detailSource should not be nil")
	}
	if m.detailSource.Name != "docx-helper" {
		t.Errorf("detailSource.Name = %q, want docx-helper", m.detailSource.Name)
	}

	// Source name is now in SubHeader(), rendered by root model.
	subHeader := ansi.Strip(m.SubHeader())
	if !strings.Contains(subHeader, "docx-helper") {
		t.Error("detail SubHeader() should show the source name")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "RECENT CHANGES") {
		t.Error("detail View() should show the changes section")
	}

	// Close detail with Esc.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	if m.detailOpen {
		t.Error("detail should be closed after Esc")
	}

	// After closing, the main list should be visible.
	subHeader = ansi.Strip(m.SubHeader())
	if !strings.Contains(subHeader, "Source Manager") {
		t.Error("list view should be visible after closing detail")
	}

	// Reopen detail with changes, test rollback with confirmation.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.changes = []fitness.ChangeEntry{
		{ID: "change-1", SourceName: "docx-helper", Description: "Updated workflow step"},
		{ID: "change-2", SourceName: "docx-helper", Description: "Fixed template"},
	}

	// Rollback requires confirm (default config).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})

	if m.confirmState != ConfirmRollback {
		t.Fatalf("confirmState = %d, want ConfirmRollback", m.confirmState)
	}

	// The confirm prompt should show [y/N] and mention the change ID.
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "[y/N]") {
		t.Error("rollback confirm View() should show [y/N] prompt")
	}
	if !strings.Contains(view, "change-1") {
		t.Error("rollback confirm View() should mention the change ID")
	}

	// Confirm with 'y' — the key the prompt advertises.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if cmd == nil {
		t.Fatal("expected cmd after pressing y")
	}
	// Feed the ConfirmResult back.
	confirmMsg := cmd()
	m, cmd = m.Update(confirmMsg)

	if cmd == nil {
		t.Fatal("expected cmd after rollback confirmation")
	}
	msg := cmd()
	rb, ok := msg.(message.RollbackFinished)
	if !ok {
		t.Fatalf("expected RollbackFinished, got %T", msg)
	}
	if rb.ChangeID != "change-1" {
		t.Errorf("RollbackFinished.ChangeID = %q, want change-1", rb.ChangeID)
	}

	// Feed the finished message back.
	m, _ = m.Update(rb)
	if len(m.changes) != 1 {
		t.Errorf("changes after rollback = %d, want 1", len(m.changes))
	}
	if m.changes[0].ID != "change-2" {
		t.Errorf("remaining change = %q, want change-2", m.changes[0].ID)
	}
}

func TestSources_RollbackWithoutConfirm(t *testing.T) {
	keys := shared.NewKeyMap("arrows")
	cfg := shared.DefaultConfig()
	cfg.Confirmations.RollbackRequiresConfirm = false

	m := New(testGroups(), &keys, cfg)
	m.SetSize(120, 40)

	// Navigate to source and open detail.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	m.changes = []fitness.ChangeEntry{
		{ID: "change-1", SourceName: "docx-helper", Description: "Test change"},
	}

	// Rollback without confirm.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})

	if cmd == nil {
		t.Fatal("expected cmd for direct rollback")
	}
	msg := cmd()
	rb, ok := msg.(message.RollbackFinished)
	if !ok {
		t.Fatalf("expected RollbackFinished, got %T", msg)
	}
	if rb.ChangeID != "change-1" {
		t.Errorf("ChangeID = %q, want change-1", rb.ChangeID)
	}
}

func TestSources_OpenUnclassified(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to unclassified source (cursor 6).
	m.cursor = 6

	// Enter should open detail view for unclassified sources too.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.detailOpen {
		t.Fatal("detail should be open after Enter on unclassified source")
	}
	if m.detailSource == nil {
		t.Fatal("detailSource should not be nil")
	}
	if m.detailSource.Name != "unknown-skill" {
		t.Errorf("detailSource.Name = %q, want unknown-skill", m.detailSource.Name)
	}
}

func TestSources_BackEmitsPopView(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	if cmd == nil {
		t.Fatal("Esc should emit a cmd")
	}
	msg := cmd()
	if _, ok := msg.(message.PopView); !ok {
		t.Fatalf("expected PopView, got %T", msg)
	}
}

func TestSources_OwnershipMine(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to "docx-helper".
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press 'o' for ownership change.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})

	if m.confirmState != ConfirmSetOwnership {
		t.Fatalf("confirmState = %d, want ConfirmSetOwnership", m.confirmState)
	}

	// The rendered view should show the prompt with [m]ine option.
	view := m.View()
	if !strings.Contains(view, "[m]ine") {
		t.Error("ownership prompt View() should contain [m]ine")
	}
	if !strings.Contains(view, "[n]ot mine") {
		t.Error("ownership prompt View() should contain [n]ot mine")
	}

	// Press 'm' — the key the prompt advertises for "mine".
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if cmd == nil {
		t.Fatal("expected cmd after pressing m")
	}
	msg := cmd()
	own, ok := msg.(message.SetOwnershipFinished)
	if !ok {
		t.Fatalf("expected SetOwnershipFinished, got %T", msg)
	}
	if own.SourceName != "docx-helper" {
		t.Errorf("SourceName = %q, want docx-helper", own.SourceName)
	}
	if own.NewOwnership != "mine" {
		t.Errorf("NewOwnership = %q, want mine", own.NewOwnership)
	}

	// After choosing, the prompt should be gone and the list should be visible.
	view = m.View()
	if strings.Contains(view, "[m]ine") {
		t.Error("ownership prompt should be dismissed after choosing")
	}
}

func TestSources_OwnershipNotMine(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to "docx-helper".
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press 'o' for ownership change.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})

	// Verify prompt is shown.
	view := m.View()
	if !strings.Contains(view, "[n]ot mine") {
		t.Error("ownership prompt View() should contain [n]ot mine")
	}

	// Press 'n' — the key the prompt advertises for "not mine".
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if cmd == nil {
		t.Fatal("expected cmd after pressing n")
	}
	msg := cmd()
	own, ok := msg.(message.SetOwnershipFinished)
	if !ok {
		t.Fatalf("expected SetOwnershipFinished, got %T", msg)
	}
	if own.NewOwnership != "not_mine" {
		t.Errorf("NewOwnership = %q, want not_mine", own.NewOwnership)
	}
}

func TestSources_OwnershipCancel(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to "docx-helper".
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press 'o' for ownership change.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})

	// Press Esc to cancel.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	if m.confirmState != ConfirmNone {
		t.Errorf("confirmState = %d, want ConfirmNone after cancel", m.confirmState)
	}
	if cmd != nil {
		t.Error("no cmd expected after cancelling ownership prompt")
	}

	// After cancel, the list should be visible again.
	view := m.View()
	if strings.Contains(view, "[m]ine") {
		t.Error("ownership prompt should be gone after cancel")
	}
	subHeader := ansi.Strip(m.SubHeader())
	if !strings.Contains(subHeader, "Source Manager") {
		t.Error("list view should be visible after cancel")
	}
}

func TestSources_OwnershipUnrecognizedKeyIgnored(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to "docx-helper" and open ownership prompt.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})

	// Keys not in m/n/esc should be silently ignored.
	for _, r := range []rune{'a', 'y', 'x', 'o', '1'} {
		m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if m2.confirmState != ConfirmSetOwnership {
			t.Errorf("pressing %q should not change confirmState", string(r))
		}
		if cmd != nil {
			t.Errorf("pressing %q should not produce a command", string(r))
		}
	}
}

func TestSources_ConfirmModel_Inactive(t *testing.T) {
	// Verify that ConfirmModel with Active=false passes through.
	cm := components.ConfirmModel{Active: false}
	view := cm.View()
	if view != "" {
		t.Errorf("inactive confirm View should be empty, got %q", view)
	}
}

func TestSources_StatusBarShowsKeyHints(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	view := ansi.Strip(m.View())

	// The status bar should advertise the keys available in the source manager.
	for _, hint := range []string{"enter", "open", "esc", "back"} {
		if !strings.Contains(view, hint) {
			t.Errorf("status bar should contain %q", hint)
		}
	}
}

func TestSources_StatusMessage_Shown(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	m, cmd := m.Update(message.StatusMessage{Text: "Rollback complete.", Duration: 3 * time.Second})
	if m.statusMsg != "Rollback complete." {
		t.Errorf("statusMsg = %q", m.statusMsg)
	}
	if cmd == nil {
		t.Fatal("expected expiry tick cmd")
	}

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Rollback complete.") {
		t.Error("View() should show status message in status bar")
	}
}

func TestSources_Viewport_ScrollsOnOverflow(t *testing.T) {
	// Build a model with many sources to force overflow.
	keys := shared.NewKeyMap("arrows")
	cfg := shared.DefaultConfig()

	groups := make([]fitness.SourceGroup, 1)
	groups[0].Label = "User-level"
	groups[0].Origin = "user"
	groups[0].Sources = make([]fitness.Source, 30)
	for i := range groups[0].Sources {
		groups[0].Sources[i] = fitness.Source{
			Name:         fmt.Sprintf("source-%02d", i),
			Origin:       "user",
			Ownership:    "mine",
			Approach:     "iterate",
			SessionCount: i,
			HealthScore:  float64(i) * 3,
		}
	}

	m := New(groups, &keys, cfg)
	m.SetSize(120, 20) // 20 lines total; 31 items won't fit

	// Viewport height = 20 - 2 (column header + status bar) = 18.
	if m.viewport.Height != 18 {
		t.Fatalf("viewport.Height = %d, want 18", m.viewport.Height)
	}

	// Navigate down past the viewport height — cursor should remain visible.
	for i := 0; i < 25; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	cursorLine := m.cursor
	if cursorLine < m.viewport.YOffset || cursorLine >= m.viewport.YOffset+m.viewport.Height {
		t.Errorf("cursor line %d not visible: YOffset=%d Height=%d",
			cursorLine, m.viewport.YOffset, m.viewport.Height)
	}
}

func TestSources_GroupCollapsedDefault(t *testing.T) {
	keys := shared.NewKeyMap("arrows")
	cfg := shared.DefaultConfig()
	cfg.SourceManager.GroupCollapsedDefault = true

	m := New(testGroups(), &keys, cfg)

	// With collapsed default, only headers should be in flatItems.
	if len(m.flatItems) != 3 {
		t.Fatalf("flatItems with collapsed default = %d, want 3 (headers only)", len(m.flatItems))
	}

	for _, item := range m.flatItems {
		if !item.isHeader {
			t.Error("all flatItems should be headers when groups are collapsed")
		}
	}

	// Expand first group.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if len(m.flatItems) != 5 {
		t.Fatalf("after expanding first group, flatItems = %d, want 5", len(m.flatItems))
	}
}
