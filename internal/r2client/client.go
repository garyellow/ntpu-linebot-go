// Package r2client provides a client for Cloudflare R2 object storage.
// It wraps the AWS S3 SDK to provide R2-specific operations including
// conditional writes for distributed locking, streaming compression,
// and snapshot management.
package r2client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
)

// Config holds R2 client configuration.
type Config struct {
	Endpoint    string // R2 endpoint URL (e.g., https://account-id.r2.cloudflarestorage.com)
	AccessKeyID string
	SecretKey   string
	BucketName  string
}

// Client provides R2 object storage operations.
type Client struct {
	s3     *s3.Client
	bucket string
}

// New creates a new R2 client.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Endpoint == "" || cfg.AccessKeyID == "" || cfg.SecretKey == "" || cfg.BucketName == "" {
		return nil, errors.New("r2client: all config fields are required")
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretKey,
			"",
		)),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("r2client: load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true // Required for R2
	})

	return &Client{
		s3:     s3Client,
		bucket: cfg.BucketName,
	}, nil
}

// Upload uploads an object to R2.
// Returns the ETag of the uploaded object.
func (c *Client) Upload(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	input := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	result, err := c.s3.PutObject(ctx, input)
	if err != nil {
		return "", fmt.Errorf("r2client: upload %q: %w", key, err)
	}

	etag := ""
	if result.ETag != nil {
		etag = strings.Trim(*result.ETag, "\"")
	}
	return etag, nil
}

// Download downloads an object from R2.
// Returns the object body and ETag. Caller must close the body.
func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, string, error) {
	result, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("r2client: download %q: %w", key, err)
	}

	etag := ""
	if result.ETag != nil {
		etag = strings.Trim(*result.ETag, "\"")
	}
	return result.Body, etag, nil
}

// HeadObject retrieves metadata for an object without downloading the body.
// Returns the ETag. Returns ErrNotFound if the object does not exist.
func (c *Client) HeadObject(ctx context.Context, key string) (string, error) {
	result, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("r2client: head %q: %w", key, err)
	}

	etag := ""
	if result.ETag != nil {
		etag = strings.Trim(*result.ETag, "\"")
	}
	return etag, nil
}

// PutObjectIfNotExists attempts to create an object only if it doesn't exist.
// Uses If-None-Match: * for conditional writes.
// Returns (true, etag) if the object was created, (false, "") if it already exists.
func (c *Client) PutObjectIfNotExists(ctx context.Context, key string, body io.Reader, contentType string) (bool, string, error) {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        body,
		IfNoneMatch: aws.String("*"),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	result, err := c.s3.PutObject(ctx, input)
	if err != nil {
		// Check for 412 Precondition Failed (object already exists)
		if isPreconditionFailed(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("r2client: put if not exists %q: %w", key, err)
	}

	etag := ""
	if result.ETag != nil {
		etag = strings.Trim(*result.ETag, "\"")
	}
	return true, etag, nil
}

// PutObjectIfMatch attempts to update an object only if its ETag matches.
// Uses If-Match for conditional writes.
// Returns (true, newEtag) if the object was updated, (false, "") if the ETag didn't match.
func (c *Client) PutObjectIfMatch(ctx context.Context, key string, body io.Reader, etag string, contentType string) (bool, string, error) {
	input := &s3.PutObjectInput{
		Bucket:  aws.String(c.bucket),
		Key:     aws.String(key),
		Body:    body,
		IfMatch: aws.String("\"" + etag + "\""),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	result, err := c.s3.PutObject(ctx, input)
	if err != nil {
		// Check for 412 Precondition Failed (ETag mismatch)
		if isPreconditionFailed(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("r2client: put if match %q: %w", key, err)
	}

	newEtag := ""
	if result.ETag != nil {
		newEtag = strings.Trim(*result.ETag, "\"")
	}
	return true, newEtag, nil
}

// DeleteObject deletes an object from R2.
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("r2client: delete %q: %w", key, err)
	}
	return nil
}

// isPreconditionFailed checks if the error is a 412 Precondition Failed response.
func isPreconditionFailed(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
		return true
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 412 {
		return true
	}
	return strings.Contains(err.Error(), "PreconditionFailed")
}

func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
		return true
	}
	return false
}

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("r2client: object not found")

// LockInfo contains information about a distributed lock.
type LockInfo struct {
	Owner     string    `json:"owner"`      // Unique identifier of the lock owner
	ExpiresAt time.Time `json:"expires_at"` // When the lock expires
}

// DistributedLock provides distributed locking using R2 conditional writes.
type DistributedLock struct {
	client  *Client
	key     string
	ttl     time.Duration
	ownerID string
	etag    string // ETag of the lock we hold (for release verification)
}

// NewDistributedLock creates a new distributed lock.
func NewDistributedLock(client *Client, key string, ttl time.Duration) *DistributedLock {
	return &DistributedLock{
		client:  client,
		key:     key,
		ttl:     ttl,
		ownerID: uuid.New().String(),
	}
}

// Acquire attempts to acquire the lock.
// Returns (true, nil) if the lock was acquired.
// Returns (false, nil) if another process holds the lock (lock exists and not expired).
// Returns (false, error) on unexpected errors.
func (l *DistributedLock) Acquire(ctx context.Context) (bool, error) {
	lockInfo := LockInfo{
		Owner:     l.ownerID,
		ExpiresAt: time.Now().Add(l.ttl),
	}

	data, err := json.Marshal(lockInfo)
	if err != nil {
		return false, fmt.Errorf("acquire lock: marshal: %w", err)
	}

	// Try to create the lock (fails if it already exists)
	created, etag, err := l.client.PutObjectIfNotExists(ctx, l.key, bytes.NewReader(data), "application/json")
	if err != nil {
		return false, fmt.Errorf("acquire lock: %w", err)
	}

	if created {
		l.etag = etag
		return true, nil
	}

	// Lock exists - check if it's expired
	expired, info, oldEtag, err := l.checkExpired(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire lock: check expired: %w", err)
	}

	if !expired {
		// Lock is held by another process and not expired
		return false, nil
	}

	// Lock is expired - try to steal it
	stolen, newEtag, err := l.steal(ctx, info, oldEtag)
	if err != nil {
		return false, fmt.Errorf("acquire lock: steal: %w", err)
	}

	if stolen {
		l.etag = newEtag
		return true, nil
	}

	// Someone else stole it first
	return false, nil
}

// Renew extends the lock TTL if we still own it.
// Returns (true, nil) if renewed, (false, nil) if lost, or (false, error) on error.
func (l *DistributedLock) Renew(ctx context.Context) (bool, error) {
	if l.etag == "" {
		return false, nil
	}

	info := LockInfo{
		Owner:     l.ownerID,
		ExpiresAt: time.Now().Add(l.ttl),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return false, fmt.Errorf("renew lock: marshal: %w", err)
	}

	updated, newEtag, err := l.client.PutObjectIfMatch(ctx, l.key, bytes.NewReader(data), l.etag, "application/json")
	if err != nil {
		return false, fmt.Errorf("renew lock: %w", err)
	}
	if !updated {
		return false, nil
	}

	l.etag = newEtag
	return true, nil
}

// checkExpired checks if the current lock is expired.
// Returns (expired, lockInfo, etag, error).
func (l *DistributedLock) checkExpired(ctx context.Context) (bool, *LockInfo, string, error) {
	body, etag, err := l.client.Download(ctx, l.key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return true, nil, "", nil // Lock was deleted
		}
		return false, nil, "", err
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return false, nil, "", fmt.Errorf("read lock: %w", err)
	}

	var info LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		// Invalid lock data - consider it expired
		return true, nil, etag, nil
	}

	return time.Now().After(info.ExpiresAt), &info, etag, nil
}

// steal attempts to steal an expired lock using conditional writes.
func (l *DistributedLock) steal(ctx context.Context, _ *LockInfo, oldEtag string) (bool, string, error) {
	newInfo := LockInfo{
		Owner:     l.ownerID,
		ExpiresAt: time.Now().Add(l.ttl),
	}

	data, err := json.Marshal(newInfo)
	if err != nil {
		return false, "", fmt.Errorf("marshal: %w", err)
	}

	return l.client.PutObjectIfMatch(ctx, l.key, bytes.NewReader(data), oldEtag, "application/json")
}

// Release releases the lock.
// Only succeeds if we still own the lock (prevents releasing a stolen lock).
func (l *DistributedLock) Release(ctx context.Context) error {
	// Verify we still own the lock before deleting
	body, _, err := l.client.Download(ctx, l.key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil // Lock already gone, that's fine
		}
		return fmt.Errorf("release lock: verify: %w", err)
	}

	data, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		return fmt.Errorf("release lock: read: %w", err)
	}

	var info LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		// Invalid lock data - delete it anyway
		return l.client.DeleteObject(ctx, l.key)
	}

	if info.Owner != l.ownerID {
		// We don't own the lock anymore (it was stolen)
		return nil
	}

	return l.client.DeleteObject(ctx, l.key)
}

// OwnerID returns the unique identifier of this lock instance.
func (l *DistributedLock) OwnerID() string {
	return l.ownerID
}

// CompressFile compresses a file using zstd and writes to the destination path.
func CompressFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("compress: open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("compress: create dest: %w", err)
	}
	defer dst.Close()

	encoder, err := zstd.NewWriter(dst, zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	if err != nil {
		return fmt.Errorf("compress: create encoder: %w", err)
	}

	if _, err := io.Copy(encoder, src); err != nil {
		_ = encoder.Close()
		return fmt.Errorf("compress: copy: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return fmt.Errorf("compress: close encoder: %w", err)
	}

	return nil
}

// DecompressStream decompresses a zstd-compressed stream to the destination path.
// Uses streaming decompression to minimize memory usage.
func DecompressStream(r io.Reader, dstPath string) error {
	decoder, err := zstd.NewReader(r)
	if err != nil {
		return fmt.Errorf("decompress: create decoder: %w", err)
	}
	defer decoder.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("decompress: create dest: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, decoder); err != nil {
		return fmt.Errorf("decompress: copy: %w", err)
	}

	return nil
}
