package detail

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
	"github.com/vladolaru/cabrero/internal/tui/testdata"
)

func newTestDetail() Model {
	keys := shared.NewKeyMap("arrows")
	cfg := testdata.TestConfig()
	p := testdata.TestProposal()
	citations := testdata.TestCitations()
	m := New(&p, citations, &keys, cfg)
	m.SetSize(120, 40)
	return m
}

func TestDetail_FocusToggle(t *testing.T) {
	m := newTestDetail()

	if m.focus != FocusProposal {
		t.Fatalf("initial focus = %d, want FocusProposal", m.focus)
	}

	// Tab switches to chat.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focus != FocusChat {
		t.Errorf("focus after Tab = %d, want FocusChat", m.focus)
	}

	// Tab switches back.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focus != FocusProposal {
		t.Errorf("focus after second Tab = %d, want FocusProposal", m.focus)
	}
}

func TestDetail_ApproveFlow(t *testing.T) {
	m := newTestDetail()

	// Press 'a' to start approve.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.applyState != ApplyConfirming {
		t.Fatalf("state = %d, want ApplyConfirming", m.applyState)
	}

	// Confirm with 'y'.
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("expected cmd from confirm")
	}

	// The confirm result should trigger the handler.
	msg := cmd()
	result, ok := msg.(components.ConfirmResult)
	if !ok {
		t.Fatalf("expected ConfirmResult, got %T", msg)
	}
	if !result.Confirmed {
		t.Error("expected Confirmed=true")
	}

	// Process the confirm result — should transition to blending.
	m, cmd = m.Update(result)
	if m.applyState != ApplyBlending {
		t.Errorf("state = %d, want ApplyBlending", m.applyState)
	}
}

func TestDetail_ApproveCancelled(t *testing.T) {
	m := newTestDetail()

	// Start approve.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.applyState != ApplyConfirming {
		t.Fatal("should be confirming")
	}

	// Cancel with 'n'.
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if cmd == nil {
		t.Fatal("expected cmd from cancel")
	}

	msg := cmd()
	result := msg.(components.ConfirmResult)
	m, _ = m.Update(result)

	if m.applyState != ApplyIdle {
		t.Errorf("state = %d, want ApplyIdle after cancel", m.applyState)
	}
}

func TestDetail_BlendFinished(t *testing.T) {
	m := newTestDetail()
	m.applyState = ApplyBlending

	diff := "some before/after diff"
	m, _ = m.Update(message.BlendFinished{
		ProposalID:      "prop-abc123",
		BeforeAfterDiff: diff,
	})

	if m.applyState != ApplyReviewing {
		t.Errorf("state = %d, want ApplyReviewing", m.applyState)
	}
	if m.blendResult == nil || *m.blendResult != diff {
		t.Error("blendResult not set correctly")
	}
}

func TestDetail_BlendError(t *testing.T) {
	m := newTestDetail()
	m.applyState = ApplyBlending

	m, cmd := m.Update(message.BlendFinished{
		ProposalID: "prop-abc123",
		Err:        fmt.Errorf("blend failed"),
	})

	if m.applyState != ApplyIdle {
		t.Errorf("state = %d, want ApplyIdle after error", m.applyState)
	}
	if cmd == nil {
		t.Fatal("expected status message cmd")
	}
}

func TestDetail_RejectSendsMessage(t *testing.T) {
	m := newTestDetail()

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("reject should produce cmd")
	}

	msg := cmd()
	reject, ok := msg.(message.RejectFinished)
	if !ok {
		t.Fatalf("expected RejectFinished, got %T", msg)
	}
	if reject.ProposalID != "prop-abc123" {
		t.Errorf("ProposalID = %q, want %q", reject.ProposalID, "prop-abc123")
	}
}

func TestDetail_DeferSendsMessage(t *testing.T) {
	m := newTestDetail()

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if cmd == nil {
		t.Fatal("defer should produce cmd")
	}

	msg := cmd()
	def, ok := msg.(message.DeferFinished)
	if !ok {
		t.Fatalf("expected DeferFinished, got %T", msg)
	}
	if def.ProposalID != "prop-abc123" {
		t.Errorf("ProposalID = %q, want %q", def.ProposalID, "prop-abc123")
	}
}

func TestDetail_CitationNavigation(t *testing.T) {
	m := newTestDetail()

	if m.citationCursor != 0 {
		t.Fatalf("initial citationCursor = %d, want 0", m.citationCursor)
	}

	// Press Down to move to next citation.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.citationCursor != 1 {
		t.Errorf("citationCursor after Down = %d, want 1", m.citationCursor)
	}

	// Press Up to move back.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.citationCursor != 0 {
		t.Errorf("citationCursor after Up = %d, want 0", m.citationCursor)
	}

	// Press Enter to expand.
	if m.citations[0].Expanded {
		t.Fatal("citation should start collapsed")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.citations[0].Expanded {
		t.Error("citation should be expanded after Enter")
	}

	// Verify the expanded content appears in the view.
	view := ansi.Strip(m.View())
	if !strings.Contains(view, m.citations[0].RawJSON[:20]) {
		t.Error("expanded citation JSON not visible in view")
	}

	// Press Enter again to collapse.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.citations[0].Expanded {
		t.Error("citation should be collapsed after second Enter")
	}
}

func TestDetail_View(t *testing.T) {
	m := newTestDetail()
	view := ansi.Strip(m.View())

	// Proposal header is now in SubHeader(), rendered by root model.
	subHeader := ansi.Strip(m.SubHeader())
	if !strings.Contains(subHeader, "Proposal Detail") {
		t.Error("missing proposal type in sub-header")
	}
	if !strings.Contains(view, "PROPOSED CHANGE") {
		t.Error("missing change section")
	}
	if !strings.Contains(view, "RATIONALE") {
		t.Error("missing rationale section")
	}
	if !strings.Contains(view, "CITATION CHAIN") {
		t.Error("missing citation chain")
	}
}
