// Package delta provides S3-backed delta log recording and merging.
package delta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/google/uuid"
)

// Recorder captures cache-miss scrape results for later snapshot merging.
type Recorder interface {
	RecordStudents(ctx context.Context, students []*storage.Student) error
	RecordContacts(ctx context.Context, contacts []*storage.Contact) error
	RecordCourses(ctx context.Context, courses []*storage.Course) error
	RecordHistoricalCourses(ctx context.Context, courses []*storage.Course) error
}

// MergeStats summarizes merge results.
type MergeStats struct {
	ObjectsProcessed int
	ObjectsMerged    int
	ObjectsSkipped   int
	MergedKeys       []string
}

// Entry represents a single append-only delta log record.
type Entry struct {
	Type      string          `json:"type"`
	CreatedAt int64           `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

// EntryType* defines delta entry types.
const (
	EntryTypeStudents          = "students"
	EntryTypeContacts          = "contacts"
	EntryTypeCourses           = "courses"
	EntryTypeHistoricalCourses = "historical_courses"
)

// S3Log writes and merges delta logs stored in S3-compatible storage.
type S3Log struct {
	client     s3LogClient
	prefix     string
	instanceID string
}

type s3LogClient interface {
	Download(ctx context.Context, key string) (io.ReadCloser, string, error)
	ListObjects(ctx context.Context, prefix string) ([]string, error)
	Upload(ctx context.Context, key string, body io.Reader, contentType string) (string, error)
	DeleteObject(ctx context.Context, key string) error
}

// NewS3Log creates a new S3 delta log helper.
func NewS3Log(client s3LogClient, prefix, instanceID string) (*S3Log, error) {
	if client == nil {
		return nil, errors.New("delta: s3 client is required")
	}
	prefix = normalizePrefix(prefix)
	if prefix == "" {
		return nil, errors.New("delta: prefix must not be empty")
	}
	if instanceID == "" {
		instanceID = "unknown"
	}
	return &S3Log{client: client, prefix: prefix, instanceID: instanceID}, nil
}

// RecordStudents appends scraped students to the delta log.
func (l *S3Log) RecordStudents(ctx context.Context, students []*storage.Student) error {
	if len(students) == 0 {
		return nil
	}
	return l.record(ctx, EntryTypeStudents, students)
}

// RecordContacts appends scraped contacts to the delta log.
func (l *S3Log) RecordContacts(ctx context.Context, contacts []*storage.Contact) error {
	if len(contacts) == 0 {
		return nil
	}
	return l.record(ctx, EntryTypeContacts, contacts)
}

// RecordCourses appends scraped courses to the delta log.
func (l *S3Log) RecordCourses(ctx context.Context, courses []*storage.Course) error {
	if len(courses) == 0 {
		return nil
	}
	return l.record(ctx, EntryTypeCourses, courses)
}

// RecordHistoricalCourses appends scraped historical courses to the delta log.
func (l *S3Log) RecordHistoricalCourses(ctx context.Context, courses []*storage.Course) error {
	if len(courses) == 0 {
		return nil
	}
	return l.record(ctx, EntryTypeHistoricalCourses, courses)
}

// MergeIntoDB applies all pending delta logs into the database.
func (l *S3Log) MergeIntoDB(ctx context.Context, db *storage.DB) (MergeStats, error) {
	keys, err := l.client.ListObjects(ctx, l.objectPrefix())
	if err != nil {
		return MergeStats{}, fmt.Errorf("delta: list objects: %w", err)
	}

	sort.Slice(keys, func(i, j int) bool {
		ti, okI := parseDeltaTimestamp(keys[i])
		tj, okJ := parseDeltaTimestamp(keys[j])
		if okI && okJ {
			return ti < tj
		}
		return keys[i] < keys[j]
	})

	stats := MergeStats{}
	for _, key := range keys {
		stats.ObjectsProcessed++
		if err := l.mergeObject(ctx, db, key); err != nil {
			stats.ObjectsSkipped++
			continue
		}
		stats.ObjectsMerged++
		stats.MergedKeys = append(stats.MergedKeys, key)
	}

	return stats, nil
}

// DeleteMerged removes delta log objects after a snapshot containing their data
// has been uploaded successfully. Keeping deletion separate from MergeIntoDB
// preserves deltas when snapshot upload fails or the leader lease is lost.
func (l *S3Log) DeleteMerged(ctx context.Context, stats MergeStats) (int, error) {
	var deleted int
	var joinedErr error
	for _, key := range stats.MergedKeys {
		if err := l.client.DeleteObject(ctx, key); err != nil {
			joinedErr = errors.Join(joinedErr, fmt.Errorf("delete entry %s: %w", key, err))
			continue
		}
		deleted++
	}
	return deleted, joinedErr
}

func parseDeltaTimestamp(key string) (int64, bool) {
	base := filepath.Base(key)
	parts := strings.SplitN(base, "-", 2)
	if len(parts) == 0 {
		return 0, false
	}
	ts, err := parseInt64(parts[0])
	if err != nil {
		return 0, false
	}
	return ts, true
}

func parseInt64(value string) (int64, error) {
	var n int64
	_, err := fmt.Sscan(value, &n)
	return n, err
}

func (l *S3Log) mergeObject(ctx context.Context, db *storage.DB, key string) error {
	body, _, err := l.client.Download(ctx, key)
	if err != nil {
		return fmt.Errorf("download %s: %w", key, err)
	}
	defer func() {
		_ = body.Close()
	}()

	var entry Entry
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&entry); err != nil {
		return fmt.Errorf("decode entry %s: %w", key, err)
	}

	if err := applyEntry(ctx, db, entry); err != nil {
		return fmt.Errorf("apply entry %s: %w", key, err)
	}

	return nil
}

func applyEntry(ctx context.Context, db *storage.DB, entry Entry) error {
	cachedAt := entryCachedAt(entry)

	switch entry.Type {
	case EntryTypeStudents:
		var students []storage.Student
		if err := json.Unmarshal(entry.Payload, &students); err != nil {
			return fmt.Errorf("decode students: %w", err)
		}
		if len(students) == 0 {
			return nil
		}
		ptrs := make([]*storage.Student, len(students))
		for i := range students {
			students[i].CachedAt = cachedAt
			ptrs[i] = &students[i]
		}
		return db.SaveStudentsBatch(ctx, ptrs)

	case EntryTypeContacts:
		var contacts []storage.Contact
		if err := json.Unmarshal(entry.Payload, &contacts); err != nil {
			return fmt.Errorf("decode contacts: %w", err)
		}
		if len(contacts) == 0 {
			return nil
		}
		ptrs := make([]*storage.Contact, len(contacts))
		for i := range contacts {
			contacts[i].CachedAt = cachedAt
			ptrs[i] = &contacts[i]
		}
		return db.SaveContactsBatch(ctx, ptrs)

	case EntryTypeCourses:
		var courses []storage.Course
		if err := json.Unmarshal(entry.Payload, &courses); err != nil {
			return fmt.Errorf("decode courses: %w", err)
		}
		if len(courses) == 0 {
			return nil
		}
		ptrs := make([]*storage.Course, len(courses))
		for i := range courses {
			courses[i].CachedAt = cachedAt
			ptrs[i] = &courses[i]
		}
		return db.SaveCoursesBatch(ctx, ptrs)

	case EntryTypeHistoricalCourses:
		var courses []storage.Course
		if err := json.Unmarshal(entry.Payload, &courses); err != nil {
			return fmt.Errorf("decode historical courses: %w", err)
		}
		if len(courses) == 0 {
			return nil
		}
		ptrs := make([]*storage.Course, len(courses))
		for i := range courses {
			courses[i].CachedAt = cachedAt
			ptrs[i] = &courses[i]
		}
		return db.SaveHistoricalCoursesBatch(ctx, ptrs)

	default:
		return fmt.Errorf("unknown entry type: %s", entry.Type)
	}
}

func entryCachedAt(entry Entry) int64 {
	if entry.CreatedAt > 0 {
		return entry.CreatedAt
	}
	return time.Now().UTC().Unix()
}

func (l *S3Log) record(ctx context.Context, entryType string, payload any) error {
	payloadData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("delta: marshal payload: %w", err)
	}

	entry := Entry{
		Type:      entryType,
		CreatedAt: time.Now().UTC().Unix(),
		Payload:   payloadData,
	}
	entryData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("delta: marshal entry: %w", err)
	}

	key := l.objectKey()
	if _, err := l.client.Upload(ctx, key, bytes.NewReader(entryData), "application/json"); err != nil {
		return fmt.Errorf("delta: upload entry: %w", err)
	}
	return nil
}

func (l *S3Log) objectPrefix() string {
	return l.prefix + "/"
}

func (l *S3Log) objectKey() string {
	return fmt.Sprintf("%s/%s/%d-%s.json", l.prefix, l.instanceID, time.Now().UnixNano(), uuid.NewString())
}

func normalizePrefix(prefix string) string {
	return strings.Trim(strings.TrimSpace(prefix), "/")
}
