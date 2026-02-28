package store

import (
	"testing"
	"time"
)

func TestCircuitBreakerState_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := Init(); err != nil {
		t.Fatal(err)
	}

	state := CircuitBreakerState{
		State:             "open",
		ConsecutiveErrors: 5,
		TotalTrips:        2,
		LastTripAt:        time.Now().UTC().Truncate(time.Second),
		LastResetAt:       time.Time{},
	}
	if err := WriteCircuitBreakerState(state); err != nil {
		t.Fatal(err)
	}

	got, err := ReadCircuitBreakerState()
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "open" || got.ConsecutiveErrors != 5 || got.TotalTrips != 2 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestCircuitBreakerState_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := Init(); err != nil {
		t.Fatal(err)
	}

	got, err := ReadCircuitBreakerState()
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "closed" {
		t.Errorf("expected closed for missing file, got %s", got.State)
	}
}
