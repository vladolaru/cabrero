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

// ConfirmRollback returns the status bar message after rolling back a change.
func ConfirmRollback() string {
	if !flavorEnabled {
		return "Change rolled back."
	}
	return "Change undone. The flock forgets."
}
