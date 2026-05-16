package snapshot

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/s3client"
)

type fakeSnapshotUploadClient struct {
	headETag string
	headErr  error

	created bool
	updated bool
	putETag string
	putErr  error

	headCalled      bool
	ifNoneCalled    bool
	ifMatchCalled   bool
	ifMatchETag     string
	ifMatchBodyRead bool
}

func (f *fakeSnapshotUploadClient) HeadObject(_ context.Context, _ string) (string, error) {
	f.headCalled = true
	return f.headETag, f.headErr
}

func (f *fakeSnapshotUploadClient) PutObjectIfNotExists(_ context.Context, _ string, body io.Reader, _ string) (bool, string, error) {
	f.ifNoneCalled = true
	_, _ = io.ReadAll(body)
	return f.created, f.putETag, f.putErr
}

func (f *fakeSnapshotUploadClient) PutObjectIfMatch(_ context.Context, _ string, body io.Reader, etag string, _ string) (bool, string, error) {
	f.ifMatchCalled = true
	f.ifMatchETag = etag
	_, _ = io.ReadAll(body)
	f.ifMatchBodyRead = true
	return f.updated, f.putETag, f.putErr
}

func TestUploadSnapshotObjectUsesCurrentETag(t *testing.T) {
	t.Parallel()

	client := &fakeSnapshotUploadClient{updated: true, putETag: "new-etag"}
	etag, err := uploadSnapshotObject(context.Background(), client, "snapshots/cache.db.zst", "old-etag", 0, bytes.NewReader([]byte("snapshot")))
	if err != nil {
		t.Fatalf("uploadSnapshotObject failed: %v", err)
	}
	if etag != "new-etag" {
		t.Fatalf("etag = %q, want new-etag", etag)
	}
	if client.headCalled {
		t.Fatal("HeadObject should not be called when current ETag is known")
	}
	if !client.ifMatchCalled || client.ifNoneCalled {
		t.Fatalf("unexpected conditional calls: ifMatch=%v ifNone=%v", client.ifMatchCalled, client.ifNoneCalled)
	}
	if client.ifMatchETag != "old-etag" {
		t.Fatalf("if-match etag = %q, want old-etag", client.ifMatchETag)
	}
}

func TestUploadSnapshotObjectUsesIfNoneMatchWhenMissing(t *testing.T) {
	t.Parallel()

	client := &fakeSnapshotUploadClient{headErr: s3client.ErrNotFound, created: true, putETag: "created-etag"}
	etag, err := uploadSnapshotObject(context.Background(), client, "snapshots/cache.db.zst", "", 0, bytes.NewReader([]byte("snapshot")))
	if err != nil {
		t.Fatalf("uploadSnapshotObject failed: %v", err)
	}
	if etag != "created-etag" {
		t.Fatalf("etag = %q, want created-etag", etag)
	}
	if !client.headCalled || !client.ifNoneCalled || client.ifMatchCalled {
		t.Fatalf("unexpected conditional calls: head=%v ifNone=%v ifMatch=%v", client.headCalled, client.ifNoneCalled, client.ifMatchCalled)
	}
}

func TestUploadSnapshotObjectUsesRemoteETagWhenCurrentUnknown(t *testing.T) {
	t.Parallel()

	client := &fakeSnapshotUploadClient{headETag: "remote-etag", updated: true, putETag: "updated-etag"}
	etag, err := uploadSnapshotObject(context.Background(), client, "snapshots/cache.db.zst", "", 0, bytes.NewReader([]byte("snapshot")))
	if err != nil {
		t.Fatalf("uploadSnapshotObject failed: %v", err)
	}
	if etag != "updated-etag" {
		t.Fatalf("etag = %q, want updated-etag", etag)
	}
	if !client.headCalled || !client.ifMatchCalled || client.ifNoneCalled {
		t.Fatalf("unexpected conditional calls: head=%v ifMatch=%v ifNone=%v", client.headCalled, client.ifMatchCalled, client.ifNoneCalled)
	}
	if client.ifMatchETag != "remote-etag" {
		t.Fatalf("if-match etag = %q, want remote-etag", client.ifMatchETag)
	}
}

func TestUploadSnapshotObjectReturnsPreconditionFailed(t *testing.T) {
	t.Parallel()

	client := &fakeSnapshotUploadClient{updated: false}
	_, err := uploadSnapshotObject(context.Background(), client, "snapshots/cache.db.zst", "old-etag", 0, bytes.NewReader([]byte("snapshot")))
	if err == nil {
		t.Fatal("expected precondition failure")
	}
	if err != s3client.ErrPreconditionFailed {
		t.Fatalf("err = %v, want ErrPreconditionFailed", err)
	}
}

func TestLeaderContextCombinesParentAndLeaseCancellation(t *testing.T) {
	t.Parallel()

	parent, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	leaderCtx, leaderCancel := context.WithCancel(context.Background())
	defer leaderCancel()

	manager := &Manager{leaderCtx: leaderCtx}
	got, gotCancel := manager.LeaderContext(parent)
	defer gotCancel()

	parentCancel()
	select {
	case <-got.Done():
	case <-time.After(time.Second):
		t.Fatal("expected derived context to be canceled by parent")
	}

	parent = context.Background()
	got, gotCancel = manager.LeaderContext(parent)
	leaderCancel()
	select {
	case <-got.Done():
	case <-time.After(time.Second):
		gotCancel()
		t.Fatal("expected derived context to be canceled by leader lease")
	}
	gotCancel()

	manager.leaderCtx = nil
	got, gotCancel = manager.LeaderContext(parent)
	defer gotCancel()
	if got != parent {
		t.Fatal("expected parent context when no leader lease is active")
	}
}

func TestReleaseLeaderLockCancelsLeaderContext(t *testing.T) {
	t.Parallel()

	leaderCtx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		leaderCtx:    leaderCtx,
		leaderCancel: cancel,
	}

	if err := manager.ReleaseLeaderLock(context.Background()); err != nil {
		t.Fatalf("ReleaseLeaderLock failed: %v", err)
	}

	select {
	case <-leaderCtx.Done():
	default:
		t.Fatal("expected leader context to be canceled on release")
	}
}
