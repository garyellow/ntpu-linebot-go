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

	"github.com/garyellow/ntpu-linebot-go/internal/r2client"
)

type fakeR2Client struct {
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
	downloadHook    func()
	putNotExistsErr error
	putIfMatchErr   error
}

func (f *fakeR2Client) Download(_ context.Context, _ string) (io.ReadCloser, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.downloadCalls++
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
		return nil, "", r2client.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(f.body)), f.etag, nil
}

func (f *fakeR2Client) PutObjectIfNotExists(_ context.Context, _ string, body io.Reader, contentType string) (bool, string, error) {
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

func (f *fakeR2Client) PutObjectIfMatch(_ context.Context, _ string, body io.Reader, etag string, contentType string) (bool, string, error) {
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

func TestNewR2ScheduleStoreValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewR2ScheduleStore(nil, "key", time.Second); err == nil {
		t.Fatal("expected error for nil client")
	}
	if _, err := NewR2ScheduleStore(&fakeR2Client{}, "", time.Second); err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestR2ScheduleStoreLoadNotFound(t *testing.T) {
	t.Parallel()

	client := &fakeR2Client{}
	store, err := NewR2ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewR2ScheduleStore failed: %v", err)
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

func TestR2ScheduleStoreEnsureRace(t *testing.T) {
	t.Parallel()

	client := &fakeR2Client{forceCreateRace: true}
	store, err := NewR2ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewR2ScheduleStore failed: %v", err)
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

func TestR2ScheduleStoreUpdateWithRetry(t *testing.T) {
	t.Parallel()

	client := &fakeR2Client{exists: true, etag: "etag-1", matchFailCount: 1}
	initial := State{LastRefresh: 10, LastCleanup: 20, UpdatedAt: 30}
	data, err := json.Marshal(initial)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	client.body = data

	store, err := NewR2ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewR2ScheduleStore failed: %v", err)
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

func TestR2ScheduleStoreWithTimeout(t *testing.T) {
	t.Parallel()

	store, err := NewR2ScheduleStore(&fakeR2Client{}, "schedule.json", time.Millisecond)
	if err != nil {
		t.Fatalf("NewR2ScheduleStore failed: %v", err)
	}

	ctx, cancel := store.withTimeout(context.Background())
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("expected deadline for positive timeout")
	}

	storeNoTimeout, err := NewR2ScheduleStore(&fakeR2Client{}, "schedule.json", 0)
	if err != nil {
		t.Fatalf("NewR2ScheduleStore failed: %v", err)
	}
	ctxNoTimeout, cancelNoTimeout := storeNoTimeout.withTimeout(context.Background())
	defer cancelNoTimeout()
	if _, ok := ctxNoTimeout.Deadline(); ok {
		t.Fatal("did not expect deadline for zero timeout")
	}
}

func TestR2ScheduleStoreLoadRetriesTransientErrors(t *testing.T) {
	t.Parallel()

	client := &fakeR2Client{
		downloadErrs: []error{
			errors.New("boom-1"),
			errors.New("boom-2"),
			errors.New("boom-3"),
		},
	}
	store, err := NewR2ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewR2ScheduleStore failed: %v", err)
	}

	_, _, _, err = store.Load(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if client.downloadCalls != 3 {
		t.Fatalf("expected 3 attempts, got %d", client.downloadCalls)
	}
}

func TestR2ScheduleStoreLoadDoesNotRetryContextCanceled(t *testing.T) {
	t.Parallel()

	client := &fakeR2Client{
		downloadErrs: []error{context.Canceled},
	}
	store, err := NewR2ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewR2ScheduleStore failed: %v", err)
	}

	_, _, _, err = store.Load(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if client.downloadCalls != 1 {
		t.Fatalf("expected 1 attempt, got %d", client.downloadCalls)
	}
}

func TestR2ScheduleStoreLoadStopsOnCanceledContextDuringBackoff(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeR2Client{
		downloadErrs: []error{errors.New("temporary")},
		downloadHook: func() {
			cancel()
		},
	}
	store, err := NewR2ScheduleStore(client, "schedule.json", time.Second)
	if err != nil {
		t.Fatalf("NewR2ScheduleStore failed: %v", err)
	}

	_, _, _, err = store.Load(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if client.downloadCalls != 1 {
		t.Fatalf("expected 1 attempt, got %d", client.downloadCalls)
	}
}
