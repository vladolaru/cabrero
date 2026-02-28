package daemon

import (
	"sync"
	"time"
)

// Circuit breaker states.
const (
	CircuitClosed   = "closed"    // normal — processing allowed
	CircuitOpen     = "open"      // tripped — processing paused
	CircuitHalfOpen = "half-open" // probing — one session allowed
)

// CircuitBreaker pauses queue processing when consecutive errors exceed
// a threshold. After a cooldown period, it allows a single probe session.
// Success closes the breaker; failure re-opens it.
type CircuitBreaker struct {
	mu              sync.Mutex
	threshold       int
	cooldown        time.Duration
	consecutiveErrs int
	state           string
	openedAt        time.Time
	totalTrips      int
	lastTripAt      time.Time
	lastResetAt     time.Time
}

// NewCircuitBreaker creates a breaker that trips after threshold consecutive
// errors and waits cooldown before probing.
func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		cooldown:  cooldown,
		state:     CircuitClosed,
	}
}

// stateLocked returns the current breaker state and performs the
// open→half-open transition if the cooldown has elapsed. Must be called
// with mu held.
func (cb *CircuitBreaker) stateLocked() string {
	if cb.state == CircuitOpen && time.Since(cb.openedAt) >= cb.cooldown {
		cb.state = CircuitHalfOpen
	}
	return cb.state
}

// State returns the current breaker state, transitioning from open to
// half-open when the cooldown has elapsed.
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.stateLocked()
}

// Allow returns true if processing is permitted (closed or half-open).
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	s := cb.stateLocked()
	return s == CircuitClosed || s == CircuitHalfOpen
}

// RecordSuccess records a successful session. Resets consecutive errors
// and closes the breaker if it was half-open.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveErrs = 0
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
		cb.lastResetAt = time.Now()
	}
}

// RecordFailure records a failed session. Trips the breaker if the
// consecutive error threshold is reached, or re-opens if half-open.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitOpen
		cb.openedAt = time.Now()
		return
	}
	if cb.state == CircuitOpen {
		return // already tripped, don't accumulate
	}
	cb.consecutiveErrs++
	if cb.consecutiveErrs >= cb.threshold {
		cb.state = CircuitOpen
		cb.openedAt = time.Now()
		cb.totalTrips++
		cb.lastTripAt = cb.openedAt
	}
}

// Reset forces the breaker back to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveErrs = 0
	cb.state = CircuitClosed
	cb.lastResetAt = time.Now()
}

// ConsecutiveErrors returns the current consecutive error count.
func (cb *CircuitBreaker) ConsecutiveErrors() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.consecutiveErrs
}
