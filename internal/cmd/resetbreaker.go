package cmd

import (
	"fmt"

	"github.com/vladolaru/cabrero/internal/store"
)

// ResetBreaker clears the circuit breaker state, allowing the daemon
// to resume queue processing immediately on its next poll cycle.
func ResetBreaker(args []string) error {
	state, err := store.ReadCircuitBreakerState()
	if err != nil {
		return fmt.Errorf("reading circuit breaker state: %w", err)
	}

	if state.State == "closed" {
		fmt.Println("Circuit breaker is already closed (normal operation).")
		return nil
	}

	fmt.Printf("Circuit breaker is %s (%d consecutive errors, %d total trips).\n",
		state.State, state.ConsecutiveErrors, state.TotalTrips)

	reset := store.CircuitBreakerState{
		State:      "closed",
		TotalTrips: state.TotalTrips, // preserve history
	}
	if err := store.WriteCircuitBreakerState(reset); err != nil {
		return fmt.Errorf("writing circuit breaker state: %w", err)
	}

	fmt.Println("Circuit breaker reset. The daemon will resume processing on its next poll cycle.")
	return nil
}
