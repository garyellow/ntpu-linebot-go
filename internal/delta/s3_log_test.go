package delta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/s3client"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

type fakeDeltaClient struct {
	objects     map[string][]byte
	deleted     []string
	deleteErrs  map[string]error
	uploadedKey string
}

func (f *fakeDeltaClient) Download(_ context.Context, key string) (io.ReadCloser, string, error) {
	body, ok := f.objects[key]
	if !ok {
		return nil, "", s3client.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(body)), "etag-1", nil
}

func (f *fakeDeltaClient) ListObjects(_ context.Context, prefix string) ([]string, error) {
	keys := make([]string, 0, len(f.objects))
	for key := range f.objects {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (f *fakeDeltaClient) Upload(_ context.Context, key string, body io.Reader, _ string) (string, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	f.objects[key] = data
	f.uploadedKey = key
	return "etag-upload", nil
}

func (f *fakeDeltaClient) DeleteObject(_ context.Context, key string) error {
	if err := f.deleteErrs[key]; err != nil {
		return err
	}
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}

func TestMergeIntoDBKeepsDeltaUntilDeleteMerged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := setupDeltaTestDB(t)
	key := "deltas/instance-1/100-students.json"
	client := &fakeDeltaClient{objects: map[string][]byte{
		key: marshalEntry(t, Entry{
			Type:      EntryTypeStudents,
			CreatedAt: time.Now().UTC().Unix(),
			Payload: mustMarshalRaw(t, []storage.Student{{
				ID:         "41247001",
				Name:       "測試學生",
				Department: "資訊工程學系",
				Year:       112,
			}}),
		}),
	}}

	log, err := NewS3Log(client, "deltas", "instance-1")
	if err != nil {
		t.Fatalf("NewS3Log failed: %v", err)
	}

	stats, err := log.MergeIntoDB(ctx, db)
	if err != nil {
		t.Fatalf("MergeIntoDB failed: %v", err)
	}
	if stats.ObjectsProcessed != 1 || stats.ObjectsMerged != 1 || stats.ObjectsSkipped != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(stats.MergedKeys) != 1 || stats.MergedKeys[0] != key {
		t.Fatalf("unexpected merged keys: %+v", stats.MergedKeys)
	}
	if _, exists := client.objects[key]; !exists {
		t.Fatal("delta object was deleted before DeleteMerged")
	}

	student, err := db.GetStudentByID(ctx, "41247001")
	if err != nil {
		t.Fatalf("GetStudentByID failed: %v", err)
	}
	if student == nil || student.Name != "測試學生" {
		t.Fatalf("student not merged into DB: %+v", student)
	}

	deleted, err := log.DeleteMerged(ctx, stats)
	if err != nil {
		t.Fatalf("DeleteMerged failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, exists := client.objects[key]; exists {
		t.Fatal("delta object still exists after DeleteMerged")
	}
}

func TestDeleteMergedReturnsPartialErrors(t *testing.T) {
	t.Parallel()

	client := &fakeDeltaClient{
		objects: map[string][]byte{
			"deltas/instance-1/100-one.json": {},
			"deltas/instance-1/200-two.json": {},
		},
		deleteErrs: map[string]error{
			"deltas/instance-1/200-two.json": errors.New("delete failed"),
		},
	}
	log, err := NewS3Log(client, "deltas", "instance-1")
	if err != nil {
		t.Fatalf("NewS3Log failed: %v", err)
	}

	deleted, err := log.DeleteMerged(context.Background(), MergeStats{MergedKeys: []string{
		"deltas/instance-1/100-one.json",
		"deltas/instance-1/200-two.json",
	}})
	if err == nil {
		t.Fatal("expected partial delete error")
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, exists := client.objects["deltas/instance-1/100-one.json"]; exists {
		t.Fatal("successful delete key still exists")
	}
	if _, exists := client.objects["deltas/instance-1/200-two.json"]; !exists {
		t.Fatal("failed delete key should remain")
	}
}

func setupDeltaTestDB(t *testing.T) *storage.DB {
	t.Helper()

	db, err := storage.New(context.Background(), filepath.Join(t.TempDir(), "delta.db"), 168*time.Hour)
	if err != nil {
		t.Fatalf("storage.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db
}

func marshalEntry(t *testing.T, entry Entry) []byte {
	t.Helper()
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	return data
}

func mustMarshalRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}
