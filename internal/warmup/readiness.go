package warmup

import (
	"sync/atomic"
	"time"
)

// ReadinessState manages service readiness state for initial startup warmup.
// It tracks whether the first warmup has completed or if the timeout has elapsed.
// Thread-safe for concurrent reads after initialization. The ready field uses
// atomic operations; startTime and timeout are immutable after construction.
type ReadinessState struct {
	ready     atomic.Bool
	startTime time.Time     // Immutable after construction
	timeout   time.Duration // Immutable after construction
}

// ReadinessStatus contains the current readiness state for API responses.
type ReadinessStatus struct {
	Ready          bool   `json:"ready"`
	Reason         string `json:"reason,omitempty"`
	ElapsedSeconds int    `json:"elapsed_seconds,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

// NewReadinessState creates a new ReadinessState with the specified timeout.
// The state starts as not ready and becomes ready when:
// 1. MarkReady() is called (warmup completed), OR
// 2. The timeout duration has elapsed since creation
func NewReadinessState(timeout time.Duration) *ReadinessState {
	return &ReadinessState{
		startTime: time.Now(),
		timeout:   timeout,
	}
}

// IsReady returns true if the service is ready to accept traffic.
// Ready conditions:
// 1. MarkReady() has been called (warmup completed), OR
// 2. The timeout duration has elapsed (graceful degradation)
func (s *ReadinessState) IsReady() bool {
	if s.ready.Load() {
		return true
	}
	// Check timeout
	if time.Since(s.startTime) >= s.timeout {
		return true
	}
	return false
}

// MarkReady marks the service as ready.
// This should be called when the initial warmup completes successfully.
func (s *ReadinessState) MarkReady() {
	s.ready.Store(true)
}

// Status returns the current readiness status for API responses.
func (s *ReadinessState) Status() ReadinessStatus {
	elapsed := time.Since(s.startTime)
	isReady := s.IsReady()

	status := ReadinessStatus{
		Ready:          isReady,
		ElapsedSeconds: int(elapsed.Seconds()),
		TimeoutSeconds: int(s.timeout.Seconds()),
	}

	if !isReady {
		status.Reason = "data refresh in progress"
	} else if !s.ready.Load() {
		// Ready due to timeout, not warmup completion
		status.Reason = "timeout reached (refresh may still be running)"
	}

	return status
}

// WarmupCompleted returns true if MarkReady was called (warmup finished).
// This is different from IsReady() which also considers timeout.
func (s *ReadinessState) WarmupCompleted() bool {
	return s.ready.Load()
}
