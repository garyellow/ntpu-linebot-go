// Package s3client provides a client for S3-compatible object storage.
// It wraps the AWS S3 SDK to provide object storage operations including
// conditional writes for lease-based coordination, streaming compression,
// and snapshot management.
package s3client

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
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
)

// Config holds S3-compatible client configuration.
type Config struct {
	Endpoint    string // S3-compatible endpoint URL
	Region      string // Signing region; defaults to us-east-1 when empty
	AccessKeyID string
	SecretKey   string
	BucketName  string
}

// Client provides S3-compatible object storage operations.
type Client struct {
	s3     *s3.Client
	bucket string
}

// New creates a new S3-compatible client.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Endpoint == "" || cfg.AccessKeyID == "" || cfg.SecretKey == "" || cfg.BucketName == "" {
		return nil, errors.New("s3client: all config fields are required")
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretKey,
			"",
		)),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("s3client: load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true // Commonly required by S3-compatible endpoints.
	})

	return &Client{
		s3:     s3Client,
		bucket: cfg.BucketName,
	}, nil
}

// Upload uploads an object to S3-compatible storage.
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
		return "", fmt.Errorf("s3client: upload %q: %w", key, err)
	}

	etag := ""
	if result.ETag != nil {
		etag = strings.Trim(*result.ETag, "\"")
	}
	return etag, nil
}

// Download downloads an object from S3-compatible storage.
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
		return nil, "", fmt.Errorf("s3client: download %q: %w", key, err)
	}

	etag := ""
	if result.ETag != nil {
		etag = strings.Trim(*result.ETag, "\"")
	}
	return result.Body, etag, nil
}

// DownloadIfMatch downloads an object only if its ETag matches the provided value.
// Returns ErrPreconditionFailed if the ETag does not match.
func (c *Client) DownloadIfMatch(ctx context.Context, key, etag string) (io.ReadCloser, string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}
	if etag != "" {
		input.IfMatch = aws.String("\"" + etag + "\"")
	}

	result, err := c.s3.GetObject(ctx, input)
	if err != nil {
		if isConditionalConflict(err) {
			return nil, "", ErrPreconditionFailed
		}
		if isNotFound(err) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("s3client: download if-match %q: %w", key, err)
	}

	resultETag := ""
	if result.ETag != nil {
		resultETag = strings.Trim(*result.ETag, "\"")
	}
	return result.Body, resultETag, nil
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
		return "", fmt.Errorf("s3client: head %q: %w", key, err)
	}

	etag := ""
	if result.ETag != nil {
		etag = strings.Trim(*result.ETag, "\"")
	}
	return etag, nil
}

// ListObjects returns object keys under the given prefix.
// It paginates until all keys are fetched.
func (c *Client) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	var token *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket: aws.String(c.bucket),
			Prefix: aws.String(prefix),
		}
		if token != nil {
			input.ContinuationToken = token
		}

		result, err := c.s3.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("s3client: list objects %q: %w", prefix, err)
		}
		for _, obj := range result.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
		if result.IsTruncated != nil && *result.IsTruncated {
			token = result.NextContinuationToken
			continue
		}
		break
	}

	return keys, nil
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
		// A failed conditional write means another writer won the race.
		if isConditionalConflict(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("s3client: put if not exists %q: %w", key, err)
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
		// A failed conditional write means another writer won the race.
		if isConditionalConflict(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("s3client: put if match %q: %w", key, err)
	}

	newEtag := ""
	if result.ETag != nil {
		newEtag = strings.Trim(*result.ETag, "\"")
	}
	return true, newEtag, nil
}

// DeleteObject deletes an object from S3-compatible storage.
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3client: delete %q: %w", key, err)
	}
	return nil
}

// isConditionalConflict checks if a conditional request lost a race.
func isConditionalConflict(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "PreconditionFailed", "ConditionalRequestConflict":
			return true
		}
	}
	return false
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
	return false
}

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("s3client: object not found")

// ErrPreconditionFailed is returned when conditional requests fail (e.g., If-Match mismatch).
var ErrPreconditionFailed = errors.New("s3client: precondition failed")

// LeaseInfo contains information about a leased leader lock.
type LeaseInfo struct {
	Owner     string    `json:"owner"`      // Unique identifier of the lock owner
	ExpiresAt time.Time `json:"expires_at"` // When the lock expires
}

type lockObjectClient interface {
	Download(ctx context.Context, key string) (io.ReadCloser, string, error)
	PutObjectIfNotExists(ctx context.Context, key string, body io.Reader, contentType string) (bool, string, error)
	PutObjectIfMatch(ctx context.Context, key string, body io.Reader, etag string, contentType string) (bool, string, error)
}

// LeaseLock is a lease-based pessimistic lock implemented with S3 conditional writes.
// S3-compatible storage cannot portably issue monotonic fencing tokens, so callers
// must renew the lease before protected writes and fence those writes with their own
// CAS primitive (for example, snapshot ETags) to reject stale leaders.
type LeaseLock struct {
	client  lockObjectClient
	key     string
	ttl     time.Duration
	ownerID string
	etag    string // ETag of the lock we hold (for release verification)
}

// NewLeaseLock creates a new lease lock.
func NewLeaseLock(client *Client, key string, ttl time.Duration) *LeaseLock {
	return &LeaseLock{
		client:  client,
		key:     key,
		ttl:     ttl,
		ownerID: uuid.New().String(),
	}
}

// Acquire attempts to acquire the lease.
// Returns (true, nil) if the lease was acquired.
// Returns (false, nil) if another process holds the lease (lock exists and not expired).
// Returns (false, error) on unexpected errors.
func (l *LeaseLock) Acquire(ctx context.Context) (bool, error) {
	leaseInfo := LeaseInfo{
		Owner:     l.ownerID,
		ExpiresAt: time.Now().UTC().Add(l.ttl),
	}

	data, err := json.Marshal(leaseInfo)
	if err != nil {
		return false, fmt.Errorf("acquire lock: marshal: %w", err)
	}

	// Try to create the lease (fails if it already exists).
	created, etag, err := l.client.PutObjectIfNotExists(ctx, l.key, bytes.NewReader(data), "application/json")
	if err != nil {
		return false, fmt.Errorf("acquire lock: %w", err)
	}

	if created {
		l.etag = etag
		return true, nil
	}

	// Lease exists - check if it's expired.
	expired, info, oldEtag, err := l.checkExpired(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire lock: check expired: %w", err)
	}

	if !expired {
		// Lease is held by another process and not expired.
		return false, nil
	}

	// Lease is expired - try to steal it.
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

// Renew extends the lease TTL if we still own it.
// Returns (true, nil) if renewed, (false, nil) if lost, or (false, error) on error.
func (l *LeaseLock) Renew(ctx context.Context) (bool, error) {
	if l.etag == "" {
		return false, nil
	}

	info := LeaseInfo{
		Owner:     l.ownerID,
		ExpiresAt: time.Now().UTC().Add(l.ttl),
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

// checkExpired checks if the current lease is expired.
// Returns (expired, leaseInfo, etag, error).
func (l *LeaseLock) checkExpired(ctx context.Context) (bool, *LeaseInfo, string, error) {
	body, etag, err := l.client.Download(ctx, l.key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return true, nil, "", nil // Lock was deleted
		}
		return false, nil, "", err
	}
	defer func() {
		_ = body.Close()
	}()

	data, err := io.ReadAll(body)
	if err != nil {
		return false, nil, "", fmt.Errorf("read lock: %w", err)
	}

	var info LeaseInfo
	if err := json.Unmarshal(data, &info); err != nil {
		// Invalid lock data - consider it expired
		return true, nil, etag, nil
	}

	return time.Now().UTC().After(info.ExpiresAt), &info, etag, nil
}

// steal attempts to steal an expired lease using conditional writes.
func (l *LeaseLock) steal(ctx context.Context, _ *LeaseInfo, oldEtag string) (bool, string, error) {
	newInfo := LeaseInfo{
		Owner:     l.ownerID,
		ExpiresAt: time.Now().UTC().Add(l.ttl),
	}

	data, err := json.Marshal(newInfo)
	if err != nil {
		return false, "", fmt.Errorf("marshal: %w", err)
	}
	if oldEtag == "" {
		return l.client.PutObjectIfNotExists(ctx, l.key, bytes.NewReader(data), "application/json")
	}

	return l.client.PutObjectIfMatch(ctx, l.key, bytes.NewReader(data), oldEtag, "application/json")
}

// Release releases the lease.
// Only succeeds if we still own the lease (prevents releasing a stolen lease).
func (l *LeaseLock) Release(ctx context.Context) error {
	if l.etag == "" {
		return nil
	}

	info := LeaseInfo{
		Owner:     l.ownerID,
		ExpiresAt: time.Unix(0, 0).UTC(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("release lock: marshal: %w", err)
	}

	updated, _, err := l.client.PutObjectIfMatch(ctx, l.key, bytes.NewReader(data), l.etag, "application/json")
	if err != nil {
		return fmt.Errorf("release lock: %w", err)
	}

	if updated {
		l.etag = ""
	}
	return nil
}

// OwnerID returns the unique identifier of this lease instance.
func (l *LeaseLock) OwnerID() string {
	return l.ownerID
}

// CompressFile compresses a file using zstd and writes to the destination path.
func CompressFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("compress: open source: %w", err)
	}
	defer func() {
		_ = src.Close()
	}()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("compress: create dest: %w", err)
	}
	defer func() {
		_ = dst.Close()
	}()

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
	defer func() {
		_ = dst.Close()
	}()

	if _, err := io.Copy(dst, decoder); err != nil {
		return fmt.Errorf("decompress: copy: %w", err)
	}

	return nil
}
