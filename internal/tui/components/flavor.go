package components

import (
	"math/rand"
)

var loadingMessages = []string{
	"Herding the goats...",
	"Tending the flock...",
	"The goatherd ponders...",
	"Gathering scattered insights...",
	"Sharpening the horns...",
	"Consulting the elders...",
	"One goat at a time...",
}

// flavorEnabled controls whether pirategoat flavor text is used.
// Set via SetFlavorEnabled at TUI startup from config.
var flavorEnabled = true

// SetFlavorEnabled configures whether flavor text is shown.
func SetFlavorEnabled(enabled bool) {
	flavorEnabled = enabled
}

// LoadingMessage returns a loading message.
func LoadingMessage() string {
	if !flavorEnabled {
		return "Loading..."
	}
	return loadingMessages[rand.Intn(len(loadingMessages))]
}

// EmptyProposals returns the empty-state message for no proposals.
func EmptyProposals() string {
	if !flavorEnabled {
		return "No proposals pending."
	}
	return "The flock is calm. No proposals pending."
}

// ConfirmApprove returns the status bar message after approving.
func ConfirmApprove() string {
	if !flavorEnabled {
		return "Change applied."
	}
	return "Change landed. The flock grows stronger."
}

// ConfirmReject returns the status bar message after rejecting.
func ConfirmReject() string {
	if !flavorEnabled {
		return "Proposal rejected."
	}
	return "Noted. The goatherd remembers."
}

// ConfirmDefer returns the status bar message after deferring.
func ConfirmDefer() string {
	if !flavorEnabled {
		return "Proposal deferred."
	}
	return "Back of the line, little goat."
}

// EmptyErrors returns the empty-state message for no errors.
func EmptyErrors() string {
	if !flavorEnabled {
		return "No errors."
	}
	return "All goats accounted for. No errors."
}

// EmptyFitnessEvidence returns the message when no evidence entries exist.
func EmptyFitnessEvidence() string {
	if !flavorEnabled {
		return "No evidence entries."
	}
	return "No sessions witnessed. The goat was never observed."
}

// EmptySources returns the message when no sources are tracked.
func EmptySources() string {
	if !flavorEnabled {
		return "No sources tracked."
	}
	return "No artifacts tracked yet. The flock is empty."
}

// AllClassified returns the message when all sources are classified.
func AllClassified() string {
	if !flavorEnabled {
		return "All sources classified."
	}
	return "Every goat has a name."
}

// ConfirmDismiss returns the status bar message after dismissing a fitness report.
func ConfirmDismiss() string {
	if !flavorEnabled {
		return "Report dismissed."
	}
	return "Report acknowledged. The goatherd moves on."
}

// ConfirmRollback returns the status bar message after rolling back a change.
func ConfirmRollback() string {
	if !flavorEnabled {
		return "Change rolled back."
	}
	return "Change undone. The flock forgets."
}
