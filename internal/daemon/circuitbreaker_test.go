package daemon

import (
	"testing"
	"time"
)

func TestCircuitBreaker_StartsInClosedState(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Minute)
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("closed breaker should allow processing")
	}
}

func TestCircuitBreaker_TripsAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Minute)
	cb.RecordSuccess() // shouldn't affect error count
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Fatal("should still be closed after 2 failures")
	}
	cb.RecordFailure() // 3rd consecutive failure
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open after 3 failures, got %s", cb.State())
	}
	if cb.Allow() {
		t.Fatal("open breaker should not allow processing")
	}
}

func TestCircuitBreaker_SuccessResetsConsecutiveErrors(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Minute)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess() // resets counter
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Fatal("success should have reset consecutive error count")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure() // trips to open
	time.Sleep(5 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected half-open after cooldown, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("half-open breaker should allow one probe")
	}
}

func TestCircuitBreaker_HalfOpenSuccess_Closes(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond)
	_ = cb.State() // transition to half-open
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after half-open success, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailure_ReopensWithNewCooldown(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond)
	_ = cb.State() // transition to half-open
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open after half-open failure, got %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Minute)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	cb.Reset()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after reset, got %s", cb.State())
	}
}

func TestCircuitBreaker_ConsecutiveErrors(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Minute)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.ConsecutiveErrors() != 2 {
		t.Fatalf("expected 2 consecutive errors, got %d", cb.ConsecutiveErrors())
	}
}

func TestCircuitBreaker_ConsecutiveErrorsMatchesThresholdAfterTrip(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Minute)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.ConsecutiveErrors() != 3 {
		t.Fatalf("expected 3 consecutive errors at trip, got %d", cb.ConsecutiveErrors())
	}
	// Additional failures while open should not inflate the counter
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.ConsecutiveErrors() != 3 {
		t.Fatalf("expected counter to stay at 3 while open, got %d", cb.ConsecutiveErrors())
	}
}
