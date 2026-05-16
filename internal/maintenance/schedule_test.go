package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/s3client"
)

type fakes3client struct {
	mu              sync.Mutex
	exists          bool
	etagCounter     int
	etag            string
	body            []byte
	forceCreateRace bool
	matchFailCount  int
	downloadErr     error
	downloadErrs    []error
	downloadCalls   int
	downloadCtxHook func(context.Context)
	downloadHook    func()
	putNotExistsErr error
	putIfMatchErr   error
}

func (f *fakes3client) Download(ctx context.Context, _ string) (io.ReadCloser, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.downloadCalls++
	if f.downloadCtxHook != nil {
		f.downloadCtxHook(ctx)
	}
	if f.downloadHook != nil {
		f.downloadHook()
	}
	if len(f.downloadErrs) > 0 {
		err := f.downloadErrs[0]
		f.downloadErrs = f.downloadErrs[1:]
		return nil, "", err
	}

	if f.downloadErr != nil {
		return nil, "", f.downloadErr
	}
	if !f.exists {
		return nil, "", s3client.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(f.body)), f.etag, nil
}

func (f *fakes3client) PutObjectIfNotExists(_ context.Context, _ string, body io.Reader, contentType string) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.putNotExistsErr != nil {
		return false, "", f.putNotExistsErr
	}
	if f.forceCreateRace {
		f.forceCreateRace = false
		if !f.exists {
			f.exists = true
			f.body, _ = io.ReadAll(body)
			f.etagCounter++
			f.etag = "etag-" + strconv.Itoa(f.etagCounter)
		}
		return false, "", nil
	}
	if f.exists {
		return false, "", nil
	}
	data, _ := io.ReadAll(body)
	f.body = data
	f.exists = true
	f.etagCounter++
	f.etag = "etag-" + strconv.Itoa(f.etagCounter)
	_ = contentType
	return true, f.etag, nil
}

func (f *fakes3client) PutObjectIfMatch(_ context.Context, _ string, body io.Reader, etag string, contentType string) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.putIfMatchErr != nil {
		return false, "", f.putIfMatchErr
	}
	if !f.exists || etag != f.etag {
		return false, "", nil
	}
	if f.matchFailCount > 0 {
		f.matchFailCount--
		return false, "", nil
	}
	data, _ := io.ReadAll(body)
	f.body = data
	f.etagCounter++
	f.etag = "etag-" + strconv.Itoa(f.etagCounter)
	_ = contentType
	return true, f.etag, nil
}

func TestStateJSONRoundTrip(t *testing.T) {
	t.Parallel()

	state := State{LastRefresh: 123, LastCleanup: 456, UpdatedAt: 789}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded State
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded != state {
		t.Fatalf("state mismatch: got %+v want %+v", decoded, state)
	}
}

func TestNewS3ScheduleStoreValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewS3ScheduleStore(nil, "key", time.Second); err == nil {
		t.Fatal("expected error for nil client")
	}
	if _, err := NewS3ScheduleStore(&fakes3client{}, "", time.Second); err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestS3ScheduleStoreLoadNotFound(t *testing.T) {
	t.Parallel()

	client := &fakes3client{}
	store, err := NewS3ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewS3ScheduleStore failed: %v", err)
	}

	state, etag, exists, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if exists {
		t.Fatal("expected exists=false")
	}
	if etag != "" {
		t.Fatalf("expected empty etag, got %q", etag)
	}
	if state != (State{}) {
		t.Fatalf("expected zero state, got %+v", state)
	}
}

func TestS3ScheduleStoreEnsureRace(t *testing.T) {
	t.Parallel()

	client := &fakes3client{forceCreateRace: true}
	store, err := NewS3ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewS3ScheduleStore failed: %v", err)
	}

	state, etag, err := store.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if etag == "" {
		t.Fatal("expected etag from ensured object")
	}
	if state.UpdatedAt == 0 {
		t.Fatal("expected UpdatedAt to be set")
	}
}

func TestS3ScheduleStoreUpdateWithRetry(t *testing.T) {
	t.Parallel()

	client := &fakes3client{exists: true, etag: "etag-1", matchFailCount: 1}
	initial := State{LastRefresh: 10, LastCleanup: 20, UpdatedAt: 30}
	data, err := json.Marshal(initial)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	client.body = data

	store, err := NewS3ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewS3ScheduleStore failed: %v", err)
	}

	err = store.Update(context.Background(), func(s *State) {
		s.LastRefresh = 99
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	loaded, _, err := store.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if loaded.LastRefresh != 99 {
		t.Fatalf("expected LastRefresh=99, got %d", loaded.LastRefresh)
	}
	if loaded.UpdatedAt == 0 {
		t.Fatal("expected UpdatedAt set")
	}
}

func TestS3ScheduleStoreWithTimeout(t *testing.T) {
	t.Parallel()

	var sawDeadline bool
	store, err := NewS3ScheduleStore(&fakes3client{
		downloadCtxHook: func(ctx context.Context) {
			_, sawDeadline = ctx.Deadline()
		},
	}, "schedule.json", time.Millisecond)
	if err != nil {
		t.Fatalf("NewS3ScheduleStore failed: %v", err)
	}

	if _, _, _, err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !sawDeadline {
		t.Fatal("expected deadline for positive timeout")
	}

	sawDeadline = false
	storeNoTimeout, err := NewS3ScheduleStore(&fakes3client{}, "schedule.json", 0)
	if err != nil {
		t.Fatalf("NewS3ScheduleStore failed: %v", err)
	}
	storeNoTimeout.client = &fakes3client{
		downloadCtxHook: func(ctx context.Context) {
			_, sawDeadline = ctx.Deadline()
		},
	}
	if _, _, _, err := storeNoTimeout.Load(context.Background()); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if sawDeadline {
		t.Fatal("did not expect deadline for zero timeout")
	}
}

func TestS3ScheduleStoreLoadRetriesTransientErrors(t *testing.T) {
	t.Parallel()

	client := &fakes3client{
		downloadErrs: []error{
			errors.New("boom-1"),
			errors.New("boom-2"),
			errors.New("boom-3"),
		},
	}
	store, err := NewS3ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewS3ScheduleStore failed: %v", err)
	}

	_, _, _, err = store.Load(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if client.downloadCalls != 3 {
		t.Fatalf("expected 3 attempts, got %d", client.downloadCalls)
	}
}

func TestS3ScheduleStoreLoadDoesNotRetryContextCanceled(t *testing.T) {
	t.Parallel()

	client := &fakes3client{
		downloadErrs: []error{context.Canceled},
	}
	store, err := NewS3ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewS3ScheduleStore failed: %v", err)
	}

	_, _, _, err = store.Load(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if client.downloadCalls != 1 {
		t.Fatalf("expected 1 attempt, got %d", client.downloadCalls)
	}
}

func TestS3ScheduleStoreLoadStopsOnCanceledContextDuringBackoff(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	client := &fakes3client{
		downloadErrs: []error{errors.New("temporary")},
		downloadHook: func() {
			cancel()
		},
	}
	store, err := NewS3ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewS3ScheduleStore failed: %v", err)
	}

	_, _, _, err = store.Load(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if client.downloadCalls != 1 {
		t.Fatalf("expected 1 attempt, got %d", client.downloadCalls)
	}
}
