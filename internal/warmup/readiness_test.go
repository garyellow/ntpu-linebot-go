package warmup

import (
	"sync"
	"testing"
	"time"
)

func TestReadinessStateInitial(t *testing.T) {
	t.Parallel()
	state := NewReadinessState(10 * time.Minute)

	if state.IsReady() {
		t.Error("Expected IsReady() to return false initially")
	}

	if state.WarmupCompleted() {
		t.Error("Expected WarmupCompleted() to return false initially")
	}

	status := state.Status()
	if status.Ready {
		t.Error("Expected status.Ready to be false initially")
	}
	if status.Reason != "warmup in progress" {
		t.Errorf("Expected reason 'warmup in progress', got %q", status.Reason)
	}
}

func TestReadinessStateMarkReady(t *testing.T) {
	t.Parallel()
	state := NewReadinessState(10 * time.Minute)

	state.MarkReady()

	if !state.IsReady() {
		t.Error("Expected IsReady() to return true after MarkReady()")
	}

	if !state.WarmupCompleted() {
		t.Error("Expected WarmupCompleted() to return true after MarkReady()")
	}

	status := state.Status()
	if !status.Ready {
		t.Error("Expected status.Ready to be true after MarkReady()")
	}
	if status.Reason != "" {
		t.Errorf("Expected empty reason after MarkReady(), got %q", status.Reason)
	}
}

func TestReadinessStateTimeout(t *testing.T) {
	t.Parallel()
	// Use a very short timeout for testing
	state := NewReadinessState(50 * time.Millisecond)

	// Initially not ready
	if state.IsReady() {
		t.Error("Expected IsReady() to return false before timeout")
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should be ready due to timeout
	if !state.IsReady() {
		t.Error("Expected IsReady() to return true after timeout")
	}

	// But warmup did not complete
	if state.WarmupCompleted() {
		t.Error("Expected WarmupCompleted() to return false (warmup didn't finish)")
	}

	status := state.Status()
	if !status.Ready {
		t.Error("Expected status.Ready to be true after timeout")
	}
	if status.Reason != "timeout reached (warmup may still be running)" {
		t.Errorf("Expected timeout reason, got %q", status.Reason)
	}
}

func TestReadinessStateStatusElapsedTime(t *testing.T) {
	t.Parallel()
	timeout := 10 * time.Minute
	state := NewReadinessState(timeout)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	status := state.Status()

	if status.TimeoutSeconds != int(timeout.Seconds()) {
		t.Errorf("Expected TimeoutSeconds=%d, got %d", int(timeout.Seconds()), status.TimeoutSeconds)
	}

	// ElapsedSeconds should be at least 0 (could be 0 or 1 depending on timing)
	if status.ElapsedSeconds < 0 {
		t.Errorf("Expected ElapsedSeconds >= 0, got %d", status.ElapsedSeconds)
	}
}

func TestReadinessStateConcurrent(t *testing.T) {
	t.Parallel()
	state := NewReadinessState(10 * time.Minute)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half goroutines read state
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 100 {
				_ = state.IsReady()
				_ = state.Status()
				_ = state.WarmupCompleted()
			}
		}()
	}

	// Half goroutines write state
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 100 {
				state.MarkReady()
			}
		}()
	}

	wg.Wait()

	// After all writes, should be ready
	if !state.IsReady() {
		t.Error("Expected IsReady() to return true after concurrent MarkReady calls")
	}
}

func TestReadinessStateMarkReadyIdempotent(t *testing.T) {
	t.Parallel()
	state := NewReadinessState(10 * time.Minute)

	// Call MarkReady multiple times
	state.MarkReady()
	state.MarkReady()
	state.MarkReady()

	if !state.IsReady() {
		t.Error("Expected IsReady() to return true")
	}

	if !state.WarmupCompleted() {
		t.Error("Expected WarmupCompleted() to return true")
	}
}
