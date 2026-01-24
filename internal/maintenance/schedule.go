// Package maintenance provides shared scheduling state for background jobs.
package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/r2client"
)

// State stores the last successful run timestamps.
type State struct {
	LastRefresh int64 `json:"last_refresh"`
	LastCleanup int64 `json:"last_cleanup"`
	UpdatedAt   int64 `json:"updated_at"`
}

type r2ScheduleClient interface {
	Download(ctx context.Context, key string) (io.ReadCloser, string, error)
	PutObjectIfNotExists(ctx context.Context, key string, body io.Reader, contentType string) (bool, string, error)
	PutObjectIfMatch(ctx context.Context, key string, body io.Reader, etag string, contentType string) (bool, string, error)
}

// R2ScheduleStore persists maintenance state in R2.
type R2ScheduleStore struct {
	client         r2ScheduleClient
	key            string
	requestTimeout time.Duration
}

// NewR2ScheduleStore creates a new schedule store.
func NewR2ScheduleStore(client r2ScheduleClient, key string, requestTimeout time.Duration) (*R2ScheduleStore, error) {
	if client == nil {
		return nil, errors.New("maintenance: r2 client is required")
	}
	if key == "" {
		return nil, errors.New("maintenance: schedule key is required")
	}
	return &R2ScheduleStore{client: client, key: key, requestTimeout: requestTimeout}, nil
}

// Load returns the current state and ETag. exists=false when the object is missing.
// Retries transient errors up to 3 times; context cancellation is not retried.
func (s *R2ScheduleStore) Load(ctx context.Context) (State, string, bool, error) {
	const maxRetries = 3
	var lastErr error

	for attempt := range maxRetries {
		state, etag, exists, err := s.loadOnce(ctx)
		if err == nil {
			return state, etag, exists, nil
		}

		// Don't retry context cancellation or deadline exceeded - these are intentional
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return State{}, "", false, err
		}

		lastErr = err

		// Don't sleep after the last attempt
		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return State{}, "", false, ctx.Err()
			case <-time.After(100 * time.Millisecond * time.Duration(attempt+1)):
				// Exponential backoff: 100ms, 200ms
			}
		}
	}

	return State{}, "", false, lastErr
}

// loadOnce performs a single load attempt.
func (s *R2ScheduleStore) loadOnce(ctx context.Context) (State, string, bool, error) {
	readCtx, cancel := s.withTimeout(ctx)
	body, etag, err := s.client.Download(readCtx, s.key)
	if err != nil {
		cancel()
		if errors.Is(err, r2client.ErrNotFound) {
			return State{}, "", false, nil
		}
		return State{}, "", false, fmt.Errorf("maintenance: download state: %w", err)
	}
	defer cancel()
	defer func() {
		_ = body.Close()
	}()

	var state State
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&state); err != nil {
		return State{}, "", false, fmt.Errorf("maintenance: decode state: %w", err)
	}

	return state, etag, true, nil
}

// Ensure returns the state and ETag, creating the object if needed.
func (s *R2ScheduleStore) Ensure(ctx context.Context) (State, string, error) {
	state, etag, exists, err := s.Load(ctx)
	if err != nil {
		return State{}, "", err
	}
	if exists {
		return state, etag, nil
	}

	state = State{UpdatedAt: time.Now().UTC().Unix()}
	data, err := json.Marshal(state)
	if err != nil {
		return State{}, "", fmt.Errorf("maintenance: marshal state: %w", err)
	}

	writeCtx, cancel := s.withTimeout(ctx)
	created, createdETag, err := s.client.PutObjectIfNotExists(writeCtx, s.key, bytes.NewReader(data), "application/json")
	cancel()
	if err != nil {
		return State{}, "", fmt.Errorf("maintenance: create state: %w", err)
	}
	if created {
		return state, createdETag, nil
	}

	// Another node created the object; load again.
	state, etag, exists, err = s.Load(ctx)
	if err != nil {
		return State{}, "", err
	}
	if !exists {
		return State{}, "", errors.New("maintenance: state missing after create race")
	}
	return state, etag, nil
}

// Update applies an update with optimistic concurrency (ETag compare-and-swap).
func (s *R2ScheduleStore) Update(ctx context.Context, updater func(*State)) error {
	for range 3 {
		state, etag, err := s.Ensure(ctx)
		if err != nil {
			return err
		}

		updater(&state)
		state.UpdatedAt = time.Now().UTC().Unix()

		data, err := json.Marshal(state)
		if err != nil {
			return fmt.Errorf("maintenance: marshal state: %w", err)
		}

		writeCtx, cancel := s.withTimeout(ctx)
		updated, _, err := s.client.PutObjectIfMatch(writeCtx, s.key, bytes.NewReader(data), etag, "application/json")
		cancel()
		if err != nil {
			return fmt.Errorf("maintenance: update state: %w", err)
		}
		if updated {
			return nil
		}
	}

	return errors.New("maintenance: failed to update state after retries")
}

func (s *R2ScheduleStore) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.requestTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, s.requestTimeout)
}
