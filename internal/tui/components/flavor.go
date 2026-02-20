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

// LoadingMessage returns a random pirategoat loading message.
func LoadingMessage() string {
	return loadingMessages[rand.Intn(len(loadingMessages))]
}

// EmptyProposals returns the empty-state message for no proposals.
func EmptyProposals() string {
	return "The flock is calm. No proposals pending."
}

// ConfirmApprove returns the status bar message after approving.
func ConfirmApprove() string {
	return "Change landed. The flock grows stronger."
}

// ConfirmReject returns the status bar message after rejecting.
func ConfirmReject() string {
	return "Noted. The goatherd remembers."
}

// ConfirmDefer returns the status bar message after deferring.
func ConfirmDefer() string {
	return "Back of the line, little goat."
}

// EmptyErrors returns the empty-state message for no errors.
func EmptyErrors() string {
	return "All goats accounted for. No errors."
}
