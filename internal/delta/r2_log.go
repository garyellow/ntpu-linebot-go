// Package delta provides R2-backed delta log recording and merging.
package delta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/r2client"
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

// R2Log writes and merges delta logs stored in R2.
type R2Log struct {
	client     *r2client.Client
	prefix     string
	instanceID string
}

// NewR2Log creates a new R2 delta log helper.
func NewR2Log(client *r2client.Client, prefix, instanceID string) (*R2Log, error) {
	if client == nil {
		return nil, errors.New("delta: r2 client is required")
	}
	prefix = normalizePrefix(prefix)
	if prefix == "" {
		return nil, errors.New("delta: prefix must not be empty")
	}
	if instanceID == "" {
		instanceID = "unknown"
	}
	return &R2Log{client: client, prefix: prefix, instanceID: instanceID}, nil
}

// RecordStudents appends scraped students to the delta log.
func (l *R2Log) RecordStudents(ctx context.Context, students []*storage.Student) error {
	if len(students) == 0 {
		return nil
	}
	return l.record(ctx, EntryTypeStudents, students)
}

// RecordContacts appends scraped contacts to the delta log.
func (l *R2Log) RecordContacts(ctx context.Context, contacts []*storage.Contact) error {
	if len(contacts) == 0 {
		return nil
	}
	return l.record(ctx, EntryTypeContacts, contacts)
}

// RecordCourses appends scraped courses to the delta log.
func (l *R2Log) RecordCourses(ctx context.Context, courses []*storage.Course) error {
	if len(courses) == 0 {
		return nil
	}
	return l.record(ctx, EntryTypeCourses, courses)
}

// RecordHistoricalCourses appends scraped historical courses to the delta log.
func (l *R2Log) RecordHistoricalCourses(ctx context.Context, courses []*storage.Course) error {
	if len(courses) == 0 {
		return nil
	}
	return l.record(ctx, EntryTypeHistoricalCourses, courses)
}

// MergeIntoDB applies all pending delta logs into the database.
func (l *R2Log) MergeIntoDB(ctx context.Context, db *storage.DB) (MergeStats, error) {
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
	}

	return stats, nil
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

func (l *R2Log) mergeObject(ctx context.Context, db *storage.DB, key string) error {
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

	if err := l.client.DeleteObject(ctx, key); err != nil {
		return fmt.Errorf("delete entry %s: %w", key, err)
	}

	return nil
}

func applyEntry(ctx context.Context, db *storage.DB, entry Entry) error {
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
			ptrs[i] = &courses[i]
		}
		return db.SaveHistoricalCoursesBatch(ctx, ptrs)

	default:
		return fmt.Errorf("unknown entry type: %s", entry.Type)
	}
}

func (l *R2Log) record(ctx context.Context, entryType string, payload any) error {
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

func (l *R2Log) objectPrefix() string {
	return l.prefix + "/"
}

func (l *R2Log) objectKey() string {
	return fmt.Sprintf("%s/%s/%d-%s.json", l.prefix, l.instanceID, time.Now().UnixNano(), uuid.NewString())
}

func normalizePrefix(prefix string) string {
	return strings.Trim(strings.TrimSpace(prefix), "/")
}
