package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// CircuitBreakerState is the persisted circuit breaker state.
type CircuitBreakerState struct {
	State             string    `json:"state"`
	ConsecutiveErrors int       `json:"consecutive_errors"`
	TotalTrips        int       `json:"total_trips"`
	LastTripAt        time.Time `json:"last_trip_at,omitempty"`
	LastResetAt       time.Time `json:"last_reset_at,omitempty"`
}

func circuitBreakerPath() string {
	return filepath.Join(Root(), "circuit_breaker.json")
}

// ReadCircuitBreakerState reads the persisted circuit breaker state.
// Returns a closed state if the file is missing or malformed.
func ReadCircuitBreakerState() (CircuitBreakerState, error) {
	data, err := os.ReadFile(circuitBreakerPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CircuitBreakerState{State: "closed"}, nil
		}
		return CircuitBreakerState{}, err
	}
	var s CircuitBreakerState
	if err := json.Unmarshal(data, &s); err != nil {
		return CircuitBreakerState{State: "closed"}, nil
	}
	return s, nil
}

// WriteCircuitBreakerState persists the circuit breaker state atomically.
func WriteCircuitBreakerState(s CircuitBreakerState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(circuitBreakerPath(), data, 0o644)
}
