// Package message defines all Bubble Tea message types for the review TUI.
package message

import "time"

// ViewState identifies which view is currently active.
type ViewState int

const (
	ViewDashboard ViewState = iota
	ViewProposalDetail
)

// Navigation messages.

// PushView pushes a new view onto the navigation stack.
type PushView struct{ View ViewState }

// PopView pops the current view, returning to the previous one.
type PopView struct{}

// Data loading messages.

// ProposalsLoaded carries the result of loading proposals from the store.
type ProposalsLoaded struct {
	Proposals []ProposalRef
	Err       error
}

// ProposalRef is a minimal reference to a proposal for message passing.
// The full pipeline.ProposalWithSession is used in the actual models.
type ProposalRef struct {
	ProposalID string
	SessionID  string
}

// StatsLoaded carries dashboard statistics.
type StatsLoaded struct {
	Stats DashboardStats
	Err   error
}

// DashboardStats holds counts and status for the dashboard header.
type DashboardStats struct {
	PendingCount  int
	ApprovedCount int
	RejectedCount int

	DaemonRunning bool
	DaemonPID     int

	HookPreCompact bool
	HookSessionEnd bool

	LastCaptureTime *time.Time
}

// Store change messages.

// StoreChanged signals that a watched store directory was modified.
type StoreChanged struct{ Dir string }

// Review action messages.

// ApproveStarted signals the beginning of an approve flow.
type ApproveStarted struct{ ProposalID string }

// BlendFinished carries the result of blending a proposal into the target file.
type BlendFinished struct {
	ProposalID     string
	BeforeAfterDiff string
	Err            error
}

// ApplyConfirmed signals the user confirmed applying the blended change.
type ApplyConfirmed struct{ ProposalID string }

// ApplyFinished carries the result of writing the blended change to disk.
type ApplyFinished struct {
	ProposalID string
	Err        error
}

// RejectFinished carries the result of rejecting a proposal.
type RejectFinished struct {
	ProposalID string
	Err        error
}

// DeferFinished carries the result of deferring a proposal.
type DeferFinished struct {
	ProposalID string
	Err        error
}

// AI chat messages.

// ChatStreamToken carries a single streaming token from the claude CLI.
type ChatStreamToken struct{ Token string }

// ChatStreamDone signals that streaming is complete.
type ChatStreamDone struct{ FullResponse string }

// ChatStreamError carries an error from the chat subprocess.
type ChatStreamError struct{ Err error }

// Status bar messages.

// StatusMessage displays a timed message in the status bar.
type StatusMessage struct {
	Text     string
	Duration time.Duration
}

// StatusMessageExpired signals that the timed status message should be cleared.
type StatusMessageExpired struct{}
