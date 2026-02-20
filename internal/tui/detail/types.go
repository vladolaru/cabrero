package detail

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
	ApplyRejectConfirming
	ApplyDeferConfirming
)
