package detail

// CitationEntry represents a single entry in the citation chain.
type CitationEntry struct {
	UUID     string
	Summary  string // one-liner: turn number, tool name, target
	RawJSON  string // full formatted entry (shown when expanded)
	Expanded bool
}

// Focus identifies which pane has focus in the detail view.
type Focus int

const (
	FocusProposal Focus = iota
	FocusChat
)

// ApplyState tracks the approve flow state machine.
type ApplyState int

const (
	ApplyIdle ApplyState = iota
	ApplyConfirming
	ApplyBlending
	ApplyReviewing
	ApplyDone
)
