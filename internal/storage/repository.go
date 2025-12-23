package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	domerrors "github.com/garyellow/ntpu-linebot-go/internal/errors"
	"github.com/garyellow/ntpu-linebot-go/internal/stringutil"
)

// SaveStudent inserts or updates a student record
func (db *DB) SaveStudent(ctx context.Context, student *Student) error {
	query := `
		INSERT INTO students (id, name, department, year, cached_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			department = excluded.department,
			year = excluded.year,
			cached_at = excluded.cached_at
	`
	start := time.Now()
	_, err := db.writer.ExecContext(ctx, query, student.ID, student.Name, student.Department, student.Year, time.Now().Unix())
	if err != nil {
		slog.ErrorContext(ctx, "failed to save student",
			"student_id", student.ID,
			"error", err)
		return fmt.Errorf("failed to save student: %w", err)
	}

	// Warn on slow queries (>100ms)
	if duration := time.Since(start); duration > 100*time.Millisecond {
		slog.WarnContext(ctx, "slow database operation",
			"operation", "SaveStudent",
			"duration_ms", duration.Milliseconds(),
			"student_id", student.ID)
	}
	return nil
}

// SaveStudentsBatch inserts or updates multiple student records in a single transaction
// This reduces lock contention during warmup by batching writes
func (db *DB) SaveStudentsBatch(ctx context.Context, students []*Student) error {
	if len(students) == 0 {
		return nil
	}

	query := `
		INSERT INTO students (id, name, department, year, cached_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			department = excluded.department,
			year = excluded.year,
			cached_at = excluded.cached_at
	`

	start := time.Now()
	cachedAt := time.Now().Unix()
	err := db.ExecBatchContext(ctx, query, func(stmt *sql.Stmt) error {
		for _, student := range students {
			if _, err := stmt.ExecContext(ctx, student.ID, student.Name, student.Department, student.Year, cachedAt); err != nil {
				slog.ErrorContext(ctx, "failed to save student in batch",
					"student_id", student.ID,
					"error", err)
				return fmt.Errorf("failed to save student %s: %w", student.ID, err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Log batch statistics
	duration := time.Since(start)
	slog.DebugContext(ctx, "batch operation completed",
		"operation", "SaveStudentsBatch",
		"count", len(students),
		"duration_ms", duration.Milliseconds())

	if duration > 500*time.Millisecond {
		slog.WarnContext(ctx, "slow batch operation",
			"operation", "SaveStudentsBatch",
			"count", len(students),
			"duration_ms", duration.Milliseconds())
	}

	return nil
}

// GetStudentByID retrieves a student by ID.
// Student data never expires; it is updated only when the cache is rebuilt (typically on startup).
func (db *DB) GetStudentByID(ctx context.Context, id string) (*Student, error) {
	query := `SELECT id, name, department, year, cached_at FROM students WHERE id = ?`

	var student Student
	err := db.reader.QueryRowContext(ctx, query, id).Scan(
		&student.ID,
		&student.Name,
		&student.Department,
		&student.Year,
		&student.CachedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "failed to query student",
			"student_id", id,
			"error", err)
		return nil, fmt.Errorf("query student: %w", err)
	}

	return &student, nil
}

// SearchStudentsByName searches students by partial name match.
// Returns both the total count and limited results (up to 400 students).
// Student data never expires; it is updated only when the cache is rebuilt (typically on startup).
func (db *DB) SearchStudentsByName(ctx context.Context, name string) (*StudentSearchResult, error) {
	if len(name) > 100 {
		return nil, errors.New("search term too long")
	}

	start := time.Now()

	// Load all students from the cache table (ordered by year and id); performance is monitored via slow-query logging below.
	query := `SELECT id, name, department, year, cached_at FROM students ORDER BY year DESC, id DESC`
	rows, err := db.reader.QueryContext(ctx, query)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get students",
			"search_term", name,
			"error", err)
		return nil, fmt.Errorf("query students: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Filter students using character-set matching (supports non-contiguous chars)
	// This allows "王明" to match "王小明"
	matchedStudents := make([]Student, 0, 400)
	for rows.Next() {
		var student Student
		if err := rows.Scan(&student.ID, &student.Name, &student.Department, &student.Year, &student.CachedAt); err != nil {
			return nil, fmt.Errorf("scan student: %w", err)
		}

		// Check if student name contains all characters from search term
		if stringutil.ContainsAllRunes(student.Name, name) {
			matchedStudents = append(matchedStudents, student)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	totalCount := len(matchedStudents)

	// Limit results to first 400 students
	if len(matchedStudents) > 400 {
		matchedStudents = matchedStudents[:400]
	}

	// Warn on slow queries
	if duration := time.Since(start); duration > 100*time.Millisecond {
		slog.WarnContext(ctx, "slow database query",
			"operation", "SearchStudentsByName",
			"duration_ms", duration.Milliseconds(),
			"search_term", name,
			"total_count", totalCount,
			"result_count", len(matchedStudents))
	}

	return &StudentSearchResult{
		Students:   matchedStudents,
		TotalCount: totalCount,
	}, nil
}

// GetStudentsByDepartment retrieves students by year and department.
// Student data never expires; it is updated only when the cache is rebuilt (typically on startup).
func (db *DB) GetStudentsByDepartment(ctx context.Context, dept string, year int) ([]Student, error) {
	query := `SELECT id, name, department, year, cached_at FROM students WHERE year = ? AND department = ?`

	rows, err := db.reader.QueryContext(ctx, query, year, dept)
	if err != nil {
		return nil, fmt.Errorf("query students: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var students []Student
	for rows.Next() {
		var student Student
		if err := rows.Scan(&student.ID, &student.Name, &student.Department, &student.Year, &student.CachedAt); err != nil {
			return nil, fmt.Errorf("scan student: %w", err)
		}
		students = append(students, student)
	}

	return students, rows.Err()
}

// CountStudents returns the total number of students.
// Student data never expires; it is updated only when the cache is rebuilt (typically on startup).
func (db *DB) CountStudents(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM students`

	var count int
	err := db.reader.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count students: %w", err)
	}
	return count, nil
}

// GetAllStudents retrieves all students from cache.
// Used for fuzzy character-set matching when SQL LIKE doesn't find results.
// Student data never expires; it is updated only when the cache is rebuilt (typically on startup).
// NOTE: For best performance, ensure an index on (year, id) exists in the students table.
func (db *DB) GetAllStudents(ctx context.Context) ([]Student, error) {
	// Get up to 3000 most recent students ordered by year and ID
	query := `SELECT id, name, department, year, cached_at
		FROM students ORDER BY year DESC, id DESC LIMIT 3000`

	rows, err := db.reader.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all students: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var students []Student
	for rows.Next() {
		var student Student
		if err := rows.Scan(&student.ID, &student.Name, &student.Department, &student.Year, &student.CachedAt); err != nil {
			return nil, fmt.Errorf("failed to scan student row: %w", err)
		}
		students = append(students, student)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating student rows: %w", err)
	}

	return students, nil
}

// ContactRepository provides CRUD operations for contacts table

// SaveContact inserts or updates a contact record
func (db *DB) SaveContact(ctx context.Context, contact *Contact) error {
	query := `
		INSERT INTO contacts (uid, type, name, name_en, title, organization, extension, phone, email, website, location, superior, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			type = excluded.type,
			name = excluded.name,
			name_en = excluded.name_en,
			title = excluded.title,
			organization = excluded.organization,
			extension = excluded.extension,
			phone = excluded.phone,
			email = excluded.email,
			website = excluded.website,
			location = excluded.location,
			superior = excluded.superior,
			cached_at = excluded.cached_at
	`
	_, err := db.writer.ExecContext(ctx, query,
		contact.UID,
		contact.Type,
		contact.Name,
		nullString(contact.NameEn),
		nullString(contact.Title),
		nullString(contact.Organization),
		nullString(contact.Extension),
		nullString(contact.Phone),
		nullString(contact.Email),
		nullString(contact.Website),
		nullString(contact.Location),
		nullString(contact.Superior),
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to save contact: %w", err)
	}
	return nil
}

// SaveContactsBatch inserts or updates multiple contact records in a single transaction
// This reduces lock contention during warmup by batching writes
func (db *DB) SaveContactsBatch(ctx context.Context, contacts []*Contact) error {
	if len(contacts) == 0 {
		return nil
	}

	query := `
		INSERT INTO contacts (uid, type, name, name_en, title, organization, extension, phone, email, website, location, superior, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			type = excluded.type,
			name = excluded.name,
			name_en = excluded.name_en,
			title = excluded.title,
			organization = excluded.organization,
			extension = excluded.extension,
			phone = excluded.phone,
			email = excluded.email,
			website = excluded.website,
			location = excluded.location,
			superior = excluded.superior,
			cached_at = excluded.cached_at
	`

	cachedAt := time.Now().Unix()
	return db.ExecBatchContext(ctx, query, func(stmt *sql.Stmt) error {
		for _, contact := range contacts {
			_, err := stmt.ExecContext(ctx,
				contact.UID,
				contact.Type,
				contact.Name,
				nullString(contact.NameEn),
				nullString(contact.Title),
				nullString(contact.Organization),
				nullString(contact.Extension),
				nullString(contact.Phone),
				nullString(contact.Email),
				nullString(contact.Website),
				nullString(contact.Location),
				nullString(contact.Superior),
				cachedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to save contact %s: %w", contact.UID, err)
			}
		}
		return nil
	})
}

// GetContactByUID retrieves a contact by UID and validates cache freshness
func (db *DB) GetContactByUID(ctx context.Context, uid string) (*Contact, error) {
	query := `SELECT uid, type, name, name_en, title, organization, extension, phone, email, website, location, superior, cached_at FROM contacts WHERE uid = ?`

	var contact Contact
	var nameEn, title, org, extension, phone, email, website, location, superior sql.NullString

	err := db.reader.QueryRowContext(ctx, query, uid).Scan(
		&contact.UID,
		&contact.Type,
		&contact.Name,
		&nameEn,
		&title,
		&org,
		&extension,
		&phone,
		&email,
		&website,
		&location,
		&superior,
		&contact.CachedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact by UID: %w", err)
	}

	contact.NameEn = nameEn.String
	contact.Title = title.String
	contact.Organization = org.String
	contact.Extension = extension.String
	contact.Phone = phone.String
	contact.Email = email.String
	contact.Website = website.String
	contact.Location = location.String
	contact.Superior = superior.String

	// Check TTL using configured cache duration
	ttl := int64(db.cacheTTL.Seconds())
	if contact.CachedAt+ttl <= time.Now().Unix() {
		return nil, nil // Cache expired
	}

	return &contact, nil
}

// SearchContactsByName searches contacts by partial name or title match (max 500 results)
// SQL searches in: name, title fields only
// Note: The calling code may perform additional fuzzy matching on more fields (name, title, organization, superior)
// Only returns non-expired cache entries based on configured TTL
func (db *DB) SearchContactsByName(ctx context.Context, name string) ([]Contact, error) {
	// Validate input
	if len(name) > 100 {
		return nil, errors.New("search term too long")
	}

	// Sanitize search term to prevent SQL LIKE special character issues
	sanitized := sanitizeSearchTerm(name)

	// Add TTL filter to prevent returning stale data
	// Search in name and title fields
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, type, name, name_en, title, organization, superior, extension, phone, email, website, location, cached_at
		FROM contacts
		WHERE (name LIKE ? ESCAPE '\' OR title LIKE ? ESCAPE '\') AND cached_at > ?
		ORDER BY type, name LIMIT 500`

	likePattern := "%" + sanitized + "%"
	rows, err := db.reader.QueryContext(ctx, query, likePattern, likePattern, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to search contacts by name: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var contacts []Contact
	for rows.Next() {
		var contact Contact
		var nameEn, title, org, superior, extension, phone, email, website, location sql.NullString

		if err := rows.Scan(&contact.UID, &contact.Type, &contact.Name, &nameEn, &title, &org, &superior, &extension, &phone, &email, &website, &location, &contact.CachedAt); err != nil {
			return nil, fmt.Errorf("failed to scan contact row: %w", err)
		}

		contact.NameEn = nameEn.String
		contact.Title = title.String
		contact.Organization = org.String
		contact.Superior = superior.String
		contact.Extension = extension.String
		contact.Phone = phone.String
		contact.Email = email.String
		contact.Website = website.String
		contact.Location = location.String

		contacts = append(contacts, contact)
	}

	return contacts, nil
}

// GetContactsByOrganization retrieves contacts by organization
// Only returns non-expired cache entries based on configured TTL
func (db *DB) GetContactsByOrganization(ctx context.Context, org string) ([]Contact, error) {
	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, type, name, name_en, title, organization, superior, extension, phone, email, cached_at FROM contacts WHERE organization = ? AND cached_at > ?`

	rows, err := db.reader.QueryContext(ctx, query, org, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get contacts by organization: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var contacts []Contact
	for rows.Next() {
		var contact Contact
		var nameEn, title, org, superior, extension, phone, email sql.NullString

		if err := rows.Scan(&contact.UID, &contact.Type, &contact.Name, &nameEn, &title, &org, &superior, &extension, &phone, &email, &contact.CachedAt); err != nil {
			return nil, fmt.Errorf("failed to scan contact row: %w", err)
		}

		contact.NameEn = nameEn.String
		contact.Title = title.String
		contact.Organization = org.String
		contact.Superior = superior.String
		contact.Extension = extension.String
		contact.Phone = phone.String
		contact.Email = email.String

		contacts = append(contacts, contact)
	}

	return contacts, nil
}

// GetAllContacts retrieves all non-expired contacts from cache
// Used for fuzzy character-set matching when SQL LIKE doesn't find results
// Only returns non-expired cache entries based on configured TTL
func (db *DB) GetAllContacts(ctx context.Context) ([]Contact, error) {
	ttlTimestamp := db.getTTLTimestamp()

	query := `SELECT uid, type, name, name_en, title, organization, extension, phone, email, website, location, superior, cached_at
		FROM contacts WHERE cached_at > ? ORDER BY type, name LIMIT 1000`

	rows, err := db.reader.QueryContext(ctx, query, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get all contacts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var contacts []Contact
	for rows.Next() {
		var contact Contact
		var nameEn, title, org, extension, phone, email, website, location, superior sql.NullString

		if err := rows.Scan(&contact.UID, &contact.Type, &contact.Name, &nameEn, &title, &org, &extension, &phone, &email, &website, &location, &superior, &contact.CachedAt); err != nil {
			return nil, fmt.Errorf("failed to scan contact row: %w", err)
		}

		contact.NameEn = nameEn.String
		contact.Title = title.String
		contact.Organization = org.String
		contact.Extension = extension.String
		contact.Phone = phone.String
		contact.Email = email.String
		contact.Website = website.String
		contact.Location = location.String
		contact.Superior = superior.String

		contacts = append(contacts, contact)
	}

	return contacts, nil
}

// DeleteExpiredContacts removes contacts older than the specified TTL
// Returns the number of deleted entries
func (db *DB) DeleteExpiredContacts(ctx context.Context, ttl time.Duration) (int64, error) {
	query := `DELETE FROM contacts WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.writer.ExecContext(ctx, query, expiryTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired contacts: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected for contacts: %w", err)
	}
	return rowsAffected, nil
}

// CountContacts returns the total number of contacts
func (db *DB) CountContacts(ctx context.Context) (int, error) {
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT COUNT(*) FROM contacts WHERE cached_at > ?`

	var count int
	err := db.reader.QueryRowContext(ctx, query, ttlTimestamp).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count contacts: %w", err)
	}
	return count, nil
}

// CourseRepository provides CRUD operations for courses table

// SaveCourse inserts or updates a course record (serializes arrays as JSON)
func (db *DB) SaveCourse(ctx context.Context, course *Course) error {
	teachersJSON, err := json.Marshal(course.Teachers)
	if err != nil {
		return fmt.Errorf("failed to marshal teachers: %w", err)
	}

	teacherURLsJSON, err := json.Marshal(course.TeacherURLs)
	if err != nil {
		return fmt.Errorf("failed to marshal teacher URLs: %w", err)
	}

	timesJSON, err := json.Marshal(course.Times)
	if err != nil {
		return fmt.Errorf("failed to marshal times: %w", err)
	}

	locationsJSON, err := json.Marshal(course.Locations)
	if err != nil {
		return fmt.Errorf("failed to marshal locations: %w", err)
	}

	query := `
		INSERT INTO courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			year = excluded.year,
			term = excluded.term,
			no = excluded.no,
			title = excluded.title,
			teachers = excluded.teachers,
			teacher_urls = excluded.teacher_urls,
			times = excluded.times,
			locations = excluded.locations,
			detail_url = excluded.detail_url,
			note = excluded.note,
			cached_at = excluded.cached_at
	`
	_, err = db.writer.ExecContext(ctx, query,
		course.UID,
		course.Year,
		course.Term,
		course.No,
		course.Title,
		string(teachersJSON),
		string(teacherURLsJSON),
		string(timesJSON),
		string(locationsJSON),
		nullString(course.DetailURL),
		nullString(course.Note),
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to save course: %w", err)
	}
	return nil
}

// SaveCoursesBatch inserts or updates multiple course records in a single transaction
// This reduces lock contention during warmup by batching writes
func (db *DB) SaveCoursesBatch(ctx context.Context, courses []*Course) error {
	if len(courses) == 0 {
		return nil
	}

	query := `
		INSERT INTO courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			year = excluded.year,
			term = excluded.term,
			no = excluded.no,
			title = excluded.title,
			teachers = excluded.teachers,
			teacher_urls = excluded.teacher_urls,
			times = excluded.times,
			locations = excluded.locations,
			detail_url = excluded.detail_url,
			note = excluded.note,
			cached_at = excluded.cached_at
	`

	cachedAt := time.Now().Unix()
	return db.ExecBatchContext(ctx, query, func(stmt *sql.Stmt) error {
		for _, course := range courses {
			teachersJSON, err := json.Marshal(course.Teachers)
			if err != nil {
				return fmt.Errorf("failed to marshal teachers for course %s: %w", course.UID, err)
			}

			teacherURLsJSON, err := json.Marshal(course.TeacherURLs)
			if err != nil {
				return fmt.Errorf("failed to marshal teacher URLs for course %s: %w", course.UID, err)
			}

			timesJSON, err := json.Marshal(course.Times)
			if err != nil {
				return fmt.Errorf("failed to marshal times for course %s: %w", course.UID, err)
			}

			locationsJSON, err := json.Marshal(course.Locations)
			if err != nil {
				return fmt.Errorf("failed to marshal locations for course %s: %w", course.UID, err)
			}

			_, err = stmt.ExecContext(ctx,
				course.UID,
				course.Year,
				course.Term,
				course.No,
				course.Title,
				string(teachersJSON),
				string(teacherURLsJSON),
				string(timesJSON),
				string(locationsJSON),
				nullString(course.DetailURL),
				nullString(course.Note),
				cachedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to save course %s: %w", course.UID, err)
			}
		}
		return nil
	})
}

// GetCourseByUID retrieves a course by UID and validates cache freshness
func (db *DB) GetCourseByUID(ctx context.Context, uid string) (*Course, error) {
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at FROM courses WHERE uid = ?`

	var course Course
	var teachersJSON, teacherURLsJSON, timesJSON, locationsJSON string
	var detailURL, note sql.NullString

	err := db.reader.QueryRowContext(ctx, query, uid).Scan(
		&course.UID,
		&course.Year,
		&course.Term,
		&course.No,
		&course.Title,
		&teachersJSON,
		&teacherURLsJSON,
		&timesJSON,
		&locationsJSON,
		&detailURL,
		&note,
		&course.CachedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get course by UID: %w", err)
	}

	course.DetailURL = detailURL.String
	course.Note = note.String

	// Deserialize JSON arrays
	if err := json.Unmarshal([]byte(teachersJSON), &course.Teachers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal teachers: %w", err)
	}
	if err := json.Unmarshal([]byte(teacherURLsJSON), &course.TeacherURLs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal teacher URLs: %w", err)
	}
	if err := json.Unmarshal([]byte(timesJSON), &course.Times); err != nil {
		return nil, fmt.Errorf("failed to unmarshal times: %w", err)
	}
	if err := json.Unmarshal([]byte(locationsJSON), &course.Locations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal locations: %w", err)
	}

	// Check TTL using configured cache duration
	ttl := int64(db.cacheTTL.Seconds())
	if course.CachedAt+ttl <= time.Now().Unix() {
		return nil, nil // Cache expired
	}

	return &course, nil
}

// SearchCoursesByTitle searches courses by partial title match (max 500 results)
// Only returns non-expired cache entries based on configured TTL
func (db *DB) SearchCoursesByTitle(ctx context.Context, title string) ([]Course, error) {
	// Validate input
	if len(title) > 100 {
		return nil, errors.New("search term too long")
	}

	// Sanitize search term to prevent SQL LIKE special character issues
	sanitized := sanitizeSearchTerm(title)

	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at FROM courses WHERE title LIKE ? ESCAPE '\' AND cached_at > ? ORDER BY year DESC, term DESC LIMIT 500`

	rows, err := db.reader.QueryContext(ctx, query, "%"+sanitized+"%", ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to search courses by title: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// SearchCoursesByTeacher searches courses by teacher name (max 500 results)
// Only returns non-expired cache entries based on configured TTL
func (db *DB) SearchCoursesByTeacher(ctx context.Context, teacher string) ([]Course, error) {
	// Validate input
	if len(teacher) > 100 {
		return nil, errors.New("search term too long")
	}

	// Sanitize search term to prevent SQL LIKE special character issues
	sanitized := sanitizeSearchTerm(teacher)

	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at FROM courses WHERE teachers LIKE ? ESCAPE '\' AND cached_at > ? ORDER BY year DESC, term DESC LIMIT 500`

	rows, err := db.reader.QueryContext(ctx, query, "%"+sanitized+"%", ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to search courses by teacher: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// GetCoursesByYearTerm retrieves courses by year and term
// Only returns non-expired cache entries based on configured TTL
func (db *DB) GetCoursesByYearTerm(ctx context.Context, year, term int) ([]Course, error) {
	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at FROM courses WHERE year = ? AND term = ? AND cached_at > ?`

	rows, err := db.reader.QueryContext(ctx, query, year, term, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get courses by year and term: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// GetDistinctRecentSemesters retrieves the most recent 2 distinct semesters (year, term pairs)
// from courses that have cached data. Returns semesters ordered by year DESC, term DESC.
// Used by syllabus warmup to determine which semesters need BM25 indexing.
func (db *DB) GetDistinctRecentSemesters(ctx context.Context, limit int) ([]struct{ Year, Term int }, error) {
	ttlTimestamp := db.getTTLTimestamp()

	query := `SELECT DISTINCT year, term
		FROM courses
		WHERE cached_at > ?
		ORDER BY year DESC, term DESC
		LIMIT ?`

	rows, err := db.reader.QueryContext(ctx, query, ttlTimestamp, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get distinct recent semesters: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var semesters []struct{ Year, Term int }
	for rows.Next() {
		var semester struct{ Year, Term int }
		if err := rows.Scan(&semester.Year, &semester.Term); err != nil {
			return nil, fmt.Errorf("failed to scan semester: %w", err)
		}
		semesters = append(semesters, semester)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return semesters, nil
}

// GetCoursesByRecentSemesters retrieves all courses from recent semesters (current + previous)
// Used for fuzzy character-set matching when SQL LIKE doesn't find results
// Only returns non-expired cache entries based on configured TTL (7-day cache for courses)
// Returns ALL courses with valid cache entries, regardless of which semesters are currently cached
func (db *DB) GetCoursesByRecentSemesters(ctx context.Context) ([]Course, error) {
	ttlTimestamp := db.getTTLTimestamp()

	// Get all courses from recent semesters ordered by semester (year DESC, term DESC)
	// This returns all courses with cached_at > TTL threshold, typically from the 4 most recent semesters
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at
		FROM courses WHERE cached_at > ? ORDER BY year DESC, term DESC`

	rows, err := db.reader.QueryContext(ctx, query, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get courses by recent semesters: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// DeleteExpiredCourses removes courses older than the specified TTL
// Returns the number of deleted entries
func (db *DB) DeleteExpiredCourses(ctx context.Context, ttl time.Duration) (int64, error) {
	query := `DELETE FROM courses WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.writer.ExecContext(ctx, query, expiryTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired courses: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected for courses: %w", err)
	}
	return rowsAffected, nil
}

// CountCourses returns the total number of courses
func (db *DB) CountCourses(ctx context.Context) (int, error) {
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT COUNT(*) FROM courses WHERE cached_at > ?`

	var count int
	err := db.reader.QueryRowContext(ctx, query, ttlTimestamp).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count courses: %w", err)
	}
	return count, nil
}

// CountCoursesBySemester returns the number of courses for a specific semester
// Returns 0 if no courses found (not an error)
func (db *DB) CountCoursesBySemester(ctx context.Context, year, term int) (int, error) {
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT COUNT(*) FROM courses WHERE year = ? AND term = ? AND cached_at > ?`

	var count int
	err := db.reader.QueryRowContext(ctx, query, year, term, ttlTimestamp).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count courses by semester: %w", err)
	}
	return count, nil
}

// Helper functions

// nullString converts an empty string to sql.NullString
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// scanCourses is a helper to scan multiple course rows
func scanCourses(rows *sql.Rows) ([]Course, error) {
	var courses []Course

	for rows.Next() {
		var course Course
		var teachersJSON, teacherURLsJSON, timesJSON, locationsJSON string
		var detailURL, note sql.NullString

		if err := rows.Scan(
			&course.UID,
			&course.Year,
			&course.Term,
			&course.No,
			&course.Title,
			&teachersJSON,
			&teacherURLsJSON,
			&timesJSON,
			&locationsJSON,
			&detailURL,
			&note,
			&course.CachedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan course row: %w", err)
		}

		course.DetailURL = detailURL.String
		course.Note = note.String

		// Deserialize JSON arrays
		if err := json.Unmarshal([]byte(teachersJSON), &course.Teachers); err != nil {
			return nil, fmt.Errorf("failed to unmarshal teachers: %w", err)
		}
		if err := json.Unmarshal([]byte(teacherURLsJSON), &course.TeacherURLs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal teacher URLs: %w", err)
		}
		if err := json.Unmarshal([]byte(timesJSON), &course.Times); err != nil {
			return nil, fmt.Errorf("failed to unmarshal times: %w", err)
		}
		if err := json.Unmarshal([]byte(locationsJSON), &course.Locations); err != nil {
			return nil, fmt.Errorf("failed to unmarshal locations: %w", err)
		}

		courses = append(courses, course)
	}

	return courses, nil
}

// StickerRepository provides CRUD operations for stickers table

// SaveSticker inserts or updates a sticker record
func (db *DB) SaveSticker(ctx context.Context, sticker *Sticker) error {
	query := `
		INSERT INTO stickers (url, source, cached_at)
		VALUES (?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			source = excluded.source,
			cached_at = excluded.cached_at
	`
	_, err := db.writer.ExecContext(ctx, query,
		sticker.URL,
		sticker.Source,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to save sticker: %w", err)
	}
	return nil
}

// GetAllStickers retrieves all stickers from database.
// Sticker data never expires; it is loaded on startup and updated only by explicit refresh.
func (db *DB) GetAllStickers(ctx context.Context) ([]Sticker, error) {
	query := `SELECT url, source, cached_at FROM stickers`

	rows, err := db.reader.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all stickers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stickers []Sticker

	for rows.Next() {
		var sticker Sticker
		if err := rows.Scan(&sticker.URL, &sticker.Source, &sticker.CachedAt); err != nil {
			return nil, fmt.Errorf("failed to scan sticker row: %w", err)
		}
		stickers = append(stickers, sticker)
	}

	return stickers, nil
}

// CountStickers returns the total number of stickers.
// Sticker data never expires; it is loaded on startup and updated only by explicit refresh.
func (db *DB) CountStickers(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM stickers`

	var count int
	err := db.reader.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count stickers: %w", err)
	}
	return count, nil
}

// GetStickerStats returns statistics about sticker sources
func (db *DB) GetStickerStats(ctx context.Context) (map[string]int, error) {
	query := `SELECT source, COUNT(*) as count FROM stickers GROUP BY source`

	rows, err := db.reader.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get sticker stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stats := make(map[string]int)
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return nil, fmt.Errorf("failed to scan sticker stats row: %w", err)
		}
		stats[source] = count
	}

	return stats, nil
}

// HistoricalCourseRepository provides CRUD operations for historical_courses table
// This table stores courses older than 4 semesters with on-demand caching and 7-day TTL

// SaveHistoricalCourse inserts or updates a historical course record
func (db *DB) SaveHistoricalCourse(ctx context.Context, course *Course) error {
	teachersJSON, err := json.Marshal(course.Teachers)
	if err != nil {
		return fmt.Errorf("failed to marshal teachers: %w", err)
	}

	teacherURLsJSON, err := json.Marshal(course.TeacherURLs)
	if err != nil {
		return fmt.Errorf("failed to marshal teacher URLs: %w", err)
	}

	timesJSON, err := json.Marshal(course.Times)
	if err != nil {
		return fmt.Errorf("failed to marshal times: %w", err)
	}

	locationsJSON, err := json.Marshal(course.Locations)
	if err != nil {
		return fmt.Errorf("failed to marshal locations: %w", err)
	}

	query := `
		INSERT INTO historical_courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			year = excluded.year,
			term = excluded.term,
			no = excluded.no,
			title = excluded.title,
			teachers = excluded.teachers,
			teacher_urls = excluded.teacher_urls,
			times = excluded.times,
			locations = excluded.locations,
			detail_url = excluded.detail_url,
			note = excluded.note,
			cached_at = excluded.cached_at
	`
	_, err = db.writer.ExecContext(ctx, query,
		course.UID,
		course.Year,
		course.Term,
		course.No,
		course.Title,
		string(teachersJSON),
		string(teacherURLsJSON),
		string(timesJSON),
		string(locationsJSON),
		nullString(course.DetailURL),
		nullString(course.Note),
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to save historical course: %w", err)
	}
	return nil
}

// SaveHistoricalCoursesBatch inserts or updates multiple historical course records in a single transaction
func (db *DB) SaveHistoricalCoursesBatch(ctx context.Context, courses []*Course) error {
	if len(courses) == 0 {
		return nil
	}

	query := `
		INSERT INTO historical_courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			year = excluded.year,
			term = excluded.term,
			no = excluded.no,
			title = excluded.title,
			teachers = excluded.teachers,
			teacher_urls = excluded.teacher_urls,
			times = excluded.times,
			locations = excluded.locations,
			detail_url = excluded.detail_url,
			note = excluded.note,
			cached_at = excluded.cached_at
	`

	cachedAt := time.Now().Unix()
	return db.ExecBatchContext(ctx, query, func(stmt *sql.Stmt) error {
		for _, course := range courses {
			teachersJSON, err := json.Marshal(course.Teachers)
			if err != nil {
				return fmt.Errorf("failed to marshal teachers for course %s: %w", course.UID, err)
			}

			teacherURLsJSON, err := json.Marshal(course.TeacherURLs)
			if err != nil {
				return fmt.Errorf("failed to marshal teacher URLs for course %s: %w", course.UID, err)
			}

			timesJSON, err := json.Marshal(course.Times)
			if err != nil {
				return fmt.Errorf("failed to marshal times for course %s: %w", course.UID, err)
			}

			locationsJSON, err := json.Marshal(course.Locations)
			if err != nil {
				return fmt.Errorf("failed to marshal locations for course %s: %w", course.UID, err)
			}

			_, err = stmt.ExecContext(ctx,
				course.UID,
				course.Year,
				course.Term,
				course.No,
				course.Title,
				string(teachersJSON),
				string(teacherURLsJSON),
				string(timesJSON),
				string(locationsJSON),
				nullString(course.DetailURL),
				nullString(course.Note),
				cachedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to save historical course %s: %w", course.UID, err)
			}
		}
		return nil
	})
}

// SearchHistoricalCoursesByYearAndTitle searches historical courses by year and partial title match
// Only returns non-expired cache entries based on configured TTL
func (db *DB) SearchHistoricalCoursesByYearAndTitle(ctx context.Context, year int, title string) ([]Course, error) {
	// Validate input
	if len(title) > 100 {
		return nil, errors.New("search term too long")
	}

	// Sanitize search term
	sanitized := sanitizeSearchTerm(title)

	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at
		FROM historical_courses WHERE year = ? AND title LIKE ? ESCAPE '\' AND cached_at > ?
		ORDER BY term DESC LIMIT 500`

	rows, err := db.reader.QueryContext(ctx, query, year, "%"+sanitized+"%", ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to search historical courses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// SearchHistoricalCoursesByYear searches historical courses by year only
// Returns all courses for the specified year (both semesters)
// Only returns non-expired cache entries based on configured TTL
func (db *DB) SearchHistoricalCoursesByYear(ctx context.Context, year int) ([]Course, error) {
	ttlTimestamp := db.getTTLTimestamp()

	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at
		FROM historical_courses WHERE year = ? AND cached_at > ?
		ORDER BY term DESC, title LIMIT 500`

	rows, err := db.reader.QueryContext(ctx, query, year, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get historical courses by year: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// DeleteExpiredHistoricalCourses removes historical courses older than the specified TTL
// Returns the number of deleted entries
func (db *DB) DeleteExpiredHistoricalCourses(ctx context.Context, ttl time.Duration) (int64, error) {
	query := `DELETE FROM historical_courses WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.writer.ExecContext(ctx, query, expiryTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired historical courses: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected for historical courses: %w", err)
	}
	return rowsAffected, nil
}

// CountHistoricalCourses returns the total number of historical courses
func (db *DB) CountHistoricalCourses(ctx context.Context) (int, error) {
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT COUNT(*) FROM historical_courses WHERE cached_at > ?`

	var count int
	err := db.reader.QueryRowContext(ctx, query, ttlTimestamp).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count historical courses: %w", err)
	}
	return count, nil
}

// ==================== Syllabi Repository Methods ====================

// SaveSyllabus inserts or updates a syllabus record
func (db *DB) SaveSyllabus(ctx context.Context, syllabus *Syllabus) error {
	query := `
		INSERT INTO syllabi (uid, year, term, title, teachers, objectives, outline, schedule, content_hash, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			year = excluded.year,
			term = excluded.term,
			title = excluded.title,
			teachers = excluded.teachers,
			objectives = excluded.objectives,
			outline = excluded.outline,
			schedule = excluded.schedule,
			content_hash = excluded.content_hash,
			cached_at = excluded.cached_at
	`

	teachersJSON, err := json.Marshal(syllabus.Teachers)
	if err != nil {
		return fmt.Errorf("failed to marshal teachers: %w", err)
	}

	_, err = db.writer.ExecContext(ctx, query,
		syllabus.UID,
		syllabus.Year,
		syllabus.Term,
		syllabus.Title,
		string(teachersJSON),
		syllabus.Objectives,
		syllabus.Outline,
		syllabus.Schedule,
		syllabus.ContentHash,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to save syllabus: %w", err)
	}
	return nil
}

// SaveSyllabusBatch inserts or updates multiple syllabus records in a single transaction
func (db *DB) SaveSyllabusBatch(ctx context.Context, syllabi []*Syllabus) error {
	if len(syllabi) == 0 {
		return nil
	}

	query := `
		INSERT INTO syllabi (uid, year, term, title, teachers, objectives, outline, schedule, content_hash, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			year = excluded.year,
			term = excluded.term,
			title = excluded.title,
			teachers = excluded.teachers,
			objectives = excluded.objectives,
			outline = excluded.outline,
			schedule = excluded.schedule,
			content_hash = excluded.content_hash,
			cached_at = excluded.cached_at
	`

	cachedAt := time.Now().Unix()
	return db.ExecBatchContext(ctx, query, func(stmt *sql.Stmt) error {
		for _, syllabus := range syllabi {
			teachersJSON, err := json.Marshal(syllabus.Teachers)
			if err != nil {
				return fmt.Errorf("failed to marshal teachers for %s: %w", syllabus.UID, err)
			}

			if _, err := stmt.ExecContext(ctx, syllabus.UID, syllabus.Year, syllabus.Term, syllabus.Title, string(teachersJSON), syllabus.Objectives, syllabus.Outline, syllabus.Schedule, syllabus.ContentHash, cachedAt); err != nil {
				return fmt.Errorf("failed to save syllabus %s: %w", syllabus.UID, err)
			}
		}
		return nil
	})
}

// GetSyllabusContentHash retrieves the content hash for a syllabus
// Used for incremental update detection - returns empty string if not found
func (db *DB) GetSyllabusContentHash(ctx context.Context, uid string) (string, error) {
	query := `SELECT content_hash FROM syllabi WHERE uid = ?`

	var hash string
	err := db.reader.QueryRowContext(ctx, query, uid).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get syllabus hash: %w", err)
	}
	return hash, nil
}

// GetSyllabusByUID retrieves a syllabus by its UID
func (db *DB) GetSyllabusByUID(ctx context.Context, uid string) (*Syllabus, error) {
	query := `SELECT uid, year, term, title, teachers, objectives, outline, schedule, content_hash, cached_at FROM syllabi WHERE uid = ?`

	var teachersJSON string
	var objectives, outline, schedule sql.NullString
	syllabus := &Syllabus{}
	err := db.reader.QueryRowContext(ctx, query, uid).Scan(
		&syllabus.UID,
		&syllabus.Year,
		&syllabus.Term,
		&syllabus.Title,
		&teachersJSON,
		&objectives,
		&outline,
		&schedule,
		&syllabus.ContentHash,
		&syllabus.CachedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domerrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get syllabus: %w", err)
	}

	if err := json.Unmarshal([]byte(teachersJSON), &syllabus.Teachers); err != nil {
		syllabus.Teachers = []string{}
	}
	syllabus.Objectives = objectives.String
	syllabus.Outline = outline.String
	syllabus.Schedule = schedule.String

	return syllabus, nil
}

// GetAllSyllabi retrieves all syllabi from the database
// Used for loading into BM25 index on startup
func (db *DB) GetAllSyllabi(ctx context.Context) ([]*Syllabus, error) {
	query := `SELECT uid, year, term, title, teachers, objectives, outline, schedule, content_hash, cached_at FROM syllabi`

	rows, err := db.reader.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query syllabi: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var syllabi []*Syllabus
	for rows.Next() {
		var teachersJSON string
		var objectives, outline, schedule sql.NullString
		syllabus := &Syllabus{}
		if err := rows.Scan(
			&syllabus.UID,
			&syllabus.Year,
			&syllabus.Term,
			&syllabus.Title,
			&teachersJSON,
			&objectives,
			&outline,
			&schedule,
			&syllabus.ContentHash,
			&syllabus.CachedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan syllabus: %w", err)
		}

		if err := json.Unmarshal([]byte(teachersJSON), &syllabus.Teachers); err != nil {
			syllabus.Teachers = []string{}
		}
		syllabus.Objectives = objectives.String
		syllabus.Outline = outline.String
		syllabus.Schedule = schedule.String

		syllabi = append(syllabi, syllabus)
	}

	return syllabi, rows.Err()
}

// GetSyllabiByYearTerm retrieves all syllabi for a specific year and term
func (db *DB) GetSyllabiByYearTerm(ctx context.Context, year, term int) ([]*Syllabus, error) {
	query := `SELECT uid, year, term, title, teachers, objectives, outline, schedule, content_hash, cached_at FROM syllabi WHERE year = ? AND term = ?`

	rows, err := db.reader.QueryContext(ctx, query, year, term)
	if err != nil {
		return nil, fmt.Errorf("failed to query syllabi: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var syllabi []*Syllabus
	for rows.Next() {
		var teachersJSON string
		var objectives, outline, schedule sql.NullString
		syllabus := &Syllabus{}
		if err := rows.Scan(
			&syllabus.UID,
			&syllabus.Year,
			&syllabus.Term,
			&syllabus.Title,
			&teachersJSON,
			&objectives,
			&outline,
			&schedule,
			&syllabus.ContentHash,
			&syllabus.CachedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan syllabus: %w", err)
		}

		if err := json.Unmarshal([]byte(teachersJSON), &syllabus.Teachers); err != nil {
			syllabus.Teachers = []string{}
		}
		syllabus.Objectives = objectives.String
		syllabus.Outline = outline.String
		syllabus.Schedule = schedule.String

		syllabi = append(syllabi, syllabus)
	}

	return syllabi, rows.Err()
}

// CountSyllabi returns the total number of syllabi
func (db *DB) CountSyllabi(ctx context.Context) (int, error) {
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT COUNT(*) FROM syllabi WHERE cached_at > ?`

	var count int
	err := db.reader.QueryRowContext(ctx, query, ttlTimestamp).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count syllabi: %w", err)
	}
	return count, nil
}

// DeleteExpiredSyllabi removes syllabi older than the specified TTL
func (db *DB) DeleteExpiredSyllabi(ctx context.Context, ttl time.Duration) (int64, error) {
	query := `DELETE FROM syllabi WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.writer.ExecContext(ctx, query, expiryTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired syllabi: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected for syllabi: %w", err)
	}
	return rowsAffected, nil
}
