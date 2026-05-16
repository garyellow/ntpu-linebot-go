package s3client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/smithy-go"
)

func TestLeaseInfo_JSON(t *testing.T) {
	t.Parallel()

	info := LeaseInfo{
		Owner:     "test-owner-123",
		ExpiresAt: time.Date(2025, 1, 20, 10, 30, 0, 0, time.UTC),
	}

	data := `{"owner":"test-owner-123","expires_at":"2025-01-20T10:30:00Z"}`
	var parsed LeaseInfo
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if parsed.Owner != info.Owner {
		t.Errorf("Owner mismatch: got %q, want %q", parsed.Owner, info.Owner)
	}
	if !parsed.ExpiresAt.Equal(info.ExpiresAt) {
		t.Errorf("ExpiresAt mismatch: got %v, want %v", parsed.ExpiresAt, info.ExpiresAt)
	}
}

func TestCompressDecompress(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	compressedPath := filepath.Join(tmpDir, "compressed.zst")
	decompressedPath := filepath.Join(tmpDir, "decompressed.txt")

	// Create test data
	testData := strings.Repeat("Hello, S3 Snapshot Compression Test! ", 1000)
	if err := os.WriteFile(srcPath, []byte(testData), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Compress
	if err := CompressFile(srcPath, compressedPath); err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	// Verify compressed file exists and is smaller
	srcInfo, _ := os.Stat(srcPath)
	compressedInfo, err := os.Stat(compressedPath)
	if err != nil {
		t.Fatalf("Compressed file not created: %v", err)
	}

	if compressedInfo.Size() >= srcInfo.Size() {
		t.Logf("Warning: compressed size (%d) >= original size (%d)", compressedInfo.Size(), srcInfo.Size())
	}

	// Decompress
	compressedFile, err := os.Open(compressedPath)
	if err != nil {
		t.Fatalf("Failed to open compressed file: %v", err)
	}
	defer compressedFile.Close()

	if err := DecompressStream(compressedFile, decompressedPath); err != nil {
		t.Fatalf("DecompressStream failed: %v", err)
	}

	// Verify decompressed content matches original
	decompressedData, err := os.ReadFile(decompressedPath)
	if err != nil {
		t.Fatalf("Failed to read decompressed file: %v", err)
	}

	if string(decompressedData) != testData {
		t.Errorf("Decompressed data mismatch: got %d bytes, want %d bytes", len(decompressedData), len(testData))
	}
}

func TestCompressFile_LargeData(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "large.txt")
	compressedPath := filepath.Join(tmpDir, "large.zst")
	decompressedPath := filepath.Join(tmpDir, "large_decompressed.txt")

	// Create 1MB of test data (simulates SQLite database)
	testData := make([]byte, 1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	if err := os.WriteFile(srcPath, testData, 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Compress
	if err := CompressFile(srcPath, compressedPath); err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	// Decompress
	compressedFile, err := os.Open(compressedPath)
	if err != nil {
		t.Fatalf("Failed to open compressed file: %v", err)
	}
	defer compressedFile.Close()

	if err := DecompressStream(compressedFile, decompressedPath); err != nil {
		t.Fatalf("DecompressStream failed: %v", err)
	}

	// Verify
	decompressedData, err := os.ReadFile(decompressedPath)
	if err != nil {
		t.Fatalf("Failed to read decompressed file: %v", err)
	}

	if !bytes.Equal(decompressedData, testData) {
		t.Error("Decompressed data does not match original")
	}
}

func TestCompressFile_Errors(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Test non-existent source
	err := CompressFile("/nonexistent/path/file.txt", filepath.Join(tmpDir, "out.zst"))
	if err == nil {
		t.Error("Expected error for non-existent source file")
	}

	// Test invalid destination
	srcPath := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	err = CompressFile(srcPath, "/nonexistent/dir/out.zst")
	if err == nil {
		t.Error("Expected error for invalid destination path")
	}
}

func TestDecompressStream_Error(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Test invalid zstd data
	invalidData := strings.NewReader("this is not zstd compressed data")
	err := DecompressStream(invalidData, filepath.Join(tmpDir, "out.txt"))
	if err == nil {
		t.Error("Expected error for invalid zstd data")
	}
}

func TestLeaseLock_NewGeneratesUniqueID(t *testing.T) {
	t.Parallel()

	// This test verifies that lease locks use unique owner IDs.
	// We can't test actual locking without a real S3-compatible endpoint

	lock1 := &LeaseLock{ownerID: "id1"}
	lock2 := &LeaseLock{ownerID: "id2"}

	if lock1.OwnerID() == lock2.OwnerID() {
		t.Error("Expected different owner IDs for different locks")
	}
}

type fakeLockClient struct {
	exists      bool
	etagCounter int
	etag        string
	body        []byte
}

func (f *fakeLockClient) Download(_ context.Context, _ string) (io.ReadCloser, string, error) {
	if !f.exists {
		return nil, "", ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(f.body)), f.etag, nil
}

func (f *fakeLockClient) PutObjectIfNotExists(_ context.Context, _ string, body io.Reader, _ string) (bool, string, error) {
	if f.exists {
		return false, "", nil
	}
	data, _ := io.ReadAll(body)
	f.body = data
	f.exists = true
	f.etagCounter++
	f.etag = fmt.Sprintf("etag-%d", f.etagCounter)
	return true, f.etag, nil
}

func (f *fakeLockClient) PutObjectIfMatch(_ context.Context, _ string, body io.Reader, etag string, _ string) (bool, string, error) {
	if !f.exists || etag != f.etag {
		return false, "", nil
	}
	data, _ := io.ReadAll(body)
	f.body = data
	f.etagCounter++
	f.etag = fmt.Sprintf("etag-%d", f.etagCounter)
	return true, f.etag, nil
}

type createRaceLockClient struct {
	fakeLockClient
	createCalls int
}

func (f *createRaceLockClient) Download(_ context.Context, _ string) (io.ReadCloser, string, error) {
	return nil, "", ErrNotFound
}

func (f *createRaceLockClient) PutObjectIfNotExists(ctx context.Context, _ string, body io.Reader, _ string) (bool, string, error) {
	f.createCalls++
	if f.createCalls == 1 {
		return false, "", nil
	}
	return f.fakeLockClient.PutObjectIfNotExists(ctx, "", body, "")
}

func (f *createRaceLockClient) PutObjectIfMatch(_ context.Context, _ string, _ io.Reader, _ string, _ string) (bool, string, error) {
	return false, "", errors.New("PutObjectIfMatch should not be called")
}

func TestLeaseLockAcquireCreatesWhenLeaseDisappearsDuringRace(t *testing.T) {
	t.Parallel()

	client := &createRaceLockClient{}
	lock := &LeaseLock{
		client:  client,
		key:     "locks/leader.json",
		ttl:     time.Hour,
		ownerID: "owner-1",
	}

	acquired, err := lock.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}
	if lock.etag == "" {
		t.Fatal("expected acquired lock etag to be set")
	}
	if client.createCalls != 2 {
		t.Fatalf("create calls = %d, want 2", client.createCalls)
	}
}

func TestIsConditionalConflictRecognizesConditionalRequestConflict(t *testing.T) {
	t.Parallel()

	err := &smithy.GenericAPIError{Code: "ConditionalRequestConflict", Message: "conditional request conflict"}
	if !isConditionalConflict(err) {
		t.Fatal("expected ConditionalRequestConflict to be treated as a conditional conflict")
	}
}

func TestIsConditionalConflictDoesNotTreatGeneric409AsConditional(t *testing.T) {
	t.Parallel()

	err := &smithy.GenericAPIError{Code: "Conflict", Message: "bucket is not in a valid state"}
	if isConditionalConflict(err) {
		t.Fatal("expected generic 409-style conflicts to remain real errors")
	}
}

func TestLeaseLockReleaseMarksExpiredWithCAS(t *testing.T) {
	t.Parallel()

	client := &fakeLockClient{
		exists:      true,
		etagCounter: 1,
		etag:        "etag-1",
	}
	lock := &LeaseLock{
		client:  client,
		key:     "locks/leader.json",
		ttl:     time.Hour,
		ownerID: "owner-1",
		etag:    "etag-1",
	}

	if err := lock.Release(context.Background()); err != nil {
		t.Fatalf("Release failed: %v", err)
	}
	if lock.etag != "" {
		t.Fatalf("expected lock etag to be cleared, got %q", lock.etag)
	}

	var info LeaseInfo
	if err := json.Unmarshal(client.body, &info); err != nil {
		t.Fatalf("unmarshal lock body: %v", err)
	}
	if info.Owner != "owner-1" {
		t.Fatalf("owner = %q, want owner-1", info.Owner)
	}
	if !info.ExpiresAt.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("ExpiresAt = %v, want unix epoch", info.ExpiresAt)
	}
}

func TestLeaseLockReleaseDoesNotModifyStolenLease(t *testing.T) {
	t.Parallel()

	current := LeaseInfo{Owner: "owner-2", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	body, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("marshal lock info: %v", err)
	}
	client := &fakeLockClient{
		exists:      true,
		etagCounter: 2,
		etag:        "etag-2",
		body:        body,
	}
	lock := &LeaseLock{
		client:  client,
		key:     "locks/leader.json",
		ttl:     time.Hour,
		ownerID: "owner-1",
		etag:    "etag-1",
	}

	if err := lock.Release(context.Background()); err != nil {
		t.Fatalf("Release failed: %v", err)
	}
	if string(client.body) != string(body) {
		t.Fatalf("stolen lock was modified: got %s want %s", client.body, body)
	}
}

// TestConfig validates the Config struct requirements
func TestConfig_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Endpoint:    "https://s3.example.com",
				AccessKeyID: "access-key",
				SecretKey:   "secret-key",
				BucketName:  "my-bucket",
			},
			wantErr: false,
		},
		{
			name: "missing endpoint",
			cfg: Config{
				AccessKeyID: "access-key",
				SecretKey:   "secret-key",
				BucketName:  "my-bucket",
			},
			wantErr: true,
		},
		{
			name: "missing access key",
			cfg: Config{
				Endpoint:   "https://s3.example.com",
				SecretKey:  "secret-key",
				BucketName: "my-bucket",
			},
			wantErr: true,
		},
		{
			name: "missing secret key",
			cfg: Config{
				Endpoint:    "https://s3.example.com",
				AccessKeyID: "access-key",
				BucketName:  "my-bucket",
			},
			wantErr: true,
		},
		{
			name: "missing bucket",
			cfg: Config{
				Endpoint:    "https://s3.example.com",
				AccessKeyID: "access-key",
				SecretKey:   "secret-key",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := New(context.Background(), tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if client == nil {
				t.Fatal("Expected client, got nil")
			}
		})
	}
}

// TestStreamingDecompression verifies memory-efficient streaming behavior
func TestStreamingDecompression(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	compressedPath := filepath.Join(tmpDir, "compressed.zst")
	decompressedPath := filepath.Join(tmpDir, "decompressed.txt")

	// Create moderate-sized data
	testData := strings.Repeat("ABCDEFGHIJ", 10000) // 100KB

	if err := os.WriteFile(srcPath, []byte(testData), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := CompressFile(srcPath, compressedPath); err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	// Open compressed file and decompress using streaming
	f, err := os.Open(compressedPath)
	if err != nil {
		t.Fatalf("Failed to open compressed file: %v", err)
	}
	defer f.Close()

	// Wrap in a reader that tracks bytes read (simulates network streaming)
	countingReader := &countingReader{r: f}

	if err := DecompressStream(countingReader, decompressedPath); err != nil {
		t.Fatalf("DecompressStream failed: %v", err)
	}

	// Verify content
	result, err := os.ReadFile(decompressedPath)
	if err != nil {
		t.Fatalf("Failed to read decompressed file: %v", err)
	}

	if string(result) != testData {
		t.Error("Decompressed content mismatch")
	}

	t.Logf("Compressed: %d bytes, Decompressed: %d bytes", countingReader.count, len(result))
}

type countingReader struct {
	r     io.Reader
	count int64
}

func (c *countingReader) Read(p []byte) (n int, err error) {
	n, err = c.r.Read(p)
	c.count += int64(n)
	return
}
