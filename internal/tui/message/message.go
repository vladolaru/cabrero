// Package message defines all Bubble Tea message types for the TUI.
package message

import (
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
)

// ViewState identifies which view is currently active.
type ViewState int

const (
	ViewDashboard ViewState = iota
	ViewProposalDetail
	ViewFitnessDetail
	ViewSourceManager
	ViewSourceDetail
	ViewPipelineMonitor
	ViewLogViewer
)

// Navigation messages.

// PushView pushes a new view onto the navigation stack.
// Action is an optional follow-up action to trigger after the transition
// (e.g., "approve" to auto-start the approve flow in the detail view).
type PushView struct {
	View   ViewState
	Action string
}

// PopView pops the current view, returning to the previous one.
type PopView struct{}

// SwitchView replaces the current top-level view without modifying the navigation stack.
// Used for peer-to-peer navigation between top-level sections (Proposals, Sources, Pipeline).
type SwitchView struct {
	View ViewState
}

// DashboardStats holds counts and status for the dashboard header.
type DashboardStats struct {
	Version string // build version, e.g. "v0.13.0"

	PendingCount       int
	ApprovedCount      int
	RejectedCount      int
	FitnessReportCount int

	DaemonRunning bool
	DaemonPID     int

	HookPreCompact bool
	HookSessionEnd bool
	DebugMode      bool

	LastCaptureTime *time.Time

	// Daemon metadata
	DaemonStartTime   *time.Time    // nil if not running; from PID file modtime
	PollInterval      time.Duration // 0 if unknown
	StaleInterval     time.Duration
	InterSessionDelay time.Duration

	// Store metrics
	StorePath    string
	SessionCount int
	DiskBytes    int64 // total size of store directory
}

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

// ChatPanelToggled signals that the chat panel was toggled via the 'c' key.
// The root model handles this by resizing detail and chat models.
type ChatPanelToggled struct{}

// AI chat messages.

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

// Fitness report messages.

// DismissFinished carries the result of dismissing a fitness report.
type DismissFinished struct {
	ReportID string
	Err      error
}

// JumpToSources navigates to the source manager with a source pre-selected.
type JumpToSources struct{ SourceName string }

// Source manager messages.

// ToggleApproachFinished carries the result of toggling a source's approach.
type ToggleApproachFinished struct {
	SourceName  string
	NewApproach string
	Err         error
}

// SetOwnershipFinished carries the result of changing a source's ownership.
type SetOwnershipFinished struct {
	SourceName   string
	NewOwnership string
	Err          error
}

// RollbackFinished carries the result of rolling back a change.
type RollbackFinished struct {
	ChangeID string
	Err      error
}

// Pipeline monitor messages.

// RetryRunStarted signals the beginning of a pipeline retry.
type RetryRunStarted struct{ SessionID string }

// RetryRunFinished carries the result of retrying a pipeline run.
type RetryRunFinished struct {
	SessionID string
	Err       error
}

// PipelineTickMsg triggers auto-refresh of pipeline data.
type PipelineTickMsg struct{}

// PipelineDataRefreshed carries refreshed pipeline data from a background I/O operation.
type PipelineDataRefreshed struct {
	Runs      []pipeline.PipelineRun
	Stats     pipeline.PipelineStats
	Prompts   []pipeline.PromptVersion
	DashStats DashboardStats
}

// LogTickMsg triggers log viewer follow-mode refresh.
type LogTickMsg struct{}
