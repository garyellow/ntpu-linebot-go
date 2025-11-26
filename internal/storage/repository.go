package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SaveStudent inserts or updates a student record
func (db *DB) SaveStudent(student *Student) error {
	query := `
		INSERT INTO students (id, name, department, year, cached_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			department = excluded.department,
			year = excluded.year,
			cached_at = excluded.cached_at
	`
	_, err := db.conn.Exec(query, student.ID, student.Name, student.Department, student.Year, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to save student: %w", err)
	}
	return nil
}

// SaveStudentsBatch inserts or updates multiple student records in a single transaction
// This reduces lock contention during warmup by batching writes
func (db *DB) SaveStudentsBatch(students []*Student) error {
	if len(students) == 0 {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT INTO students (id, name, department, year, cached_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			department = excluded.department,
			year = excluded.year,
			cached_at = excluded.cached_at
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	cachedAt := time.Now().Unix()
	for _, student := range students {
		if _, err = stmt.Exec(student.ID, student.Name, student.Department, student.Year, cachedAt); err != nil {
			return fmt.Errorf("failed to execute statement for student %s: %w", student.ID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetStudentByID retrieves a student by ID and validates cache freshness (7 days = 168 hours)
func (db *DB) GetStudentByID(id string) (*Student, error) {
	query := `SELECT id, name, department, year, cached_at FROM students WHERE id = ?`

	var student Student
	err := db.conn.QueryRow(query, id).Scan(
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
		return nil, fmt.Errorf("failed to get student by ID: %w", err)
	}

	// Check TTL using configured cache duration
	ttl := int64(db.cacheTTL.Seconds())
	if student.CachedAt+ttl <= time.Now().Unix() {
		return nil, nil // Cache expired
	}

	return &student, nil
}

// SearchStudentsByName searches students by partial name match (max 500 results)
// Only returns non-expired cache entries based on configured TTL
func (db *DB) SearchStudentsByName(name string) ([]Student, error) {
	// Validate input to prevent SQL injection (even though we use prepared statements)
	if len(name) > 100 {
		return nil, fmt.Errorf("search term too long")
	}

	// Sanitize search term to prevent SQL LIKE special character issues
	sanitized := sanitizeSearchTerm(name)

	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT id, name, department, year, cached_at FROM students WHERE name LIKE ? ESCAPE '\' AND cached_at > ? ORDER BY year DESC, id DESC LIMIT 500`

	rows, err := db.conn.Query(query, "%"+sanitized+"%", ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to search students by name: %w", err)
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

	return students, nil
}

// GetStudentsByYearDept retrieves students by year and department
// Only returns non-expired cache entries based on configured TTL
func (db *DB) GetStudentsByYearDept(year int, dept string) ([]Student, error) {
	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT id, name, department, year, cached_at FROM students WHERE year = ? AND department = ? AND cached_at > ?`

	rows, err := db.conn.Query(query, year, dept, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get students by year and department: %w", err)
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

	return students, nil
}

// DeleteExpiredStudents removes students older than the specified TTL
// Returns the number of deleted entries
func (db *DB) DeleteExpiredStudents(ttl time.Duration) (int64, error) {
	query := `DELETE FROM students WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.conn.Exec(query, expiryTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired students: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected for students: %w", err)
	}
	return rowsAffected, nil
}

// CountStudents returns the total number of students
func (db *DB) CountStudents() (int, error) {
	query := `SELECT COUNT(*) FROM students`

	var count int
	err := db.conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count students: %w", err)
	}
	return count, nil
}

// ContactRepository provides CRUD operations for contacts table

// SaveContact inserts or updates a contact record
func (db *DB) SaveContact(contact *Contact) error {
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
	_, err := db.conn.Exec(query,
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
func (db *DB) SaveContactsBatch(contacts []*Contact) error {
	if len(contacts) == 0 {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
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
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	cachedAt := time.Now().Unix()
	for _, contact := range contacts {
		_, err = stmt.Exec(
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
			return fmt.Errorf("failed to execute statement for contact %s: %w", contact.UID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetContactByUID retrieves a contact by UID and validates cache freshness
func (db *DB) GetContactByUID(uid string) (*Contact, error) {
	query := `SELECT uid, type, name, name_en, title, organization, extension, phone, email, website, location, superior, cached_at FROM contacts WHERE uid = ?`

	var contact Contact
	var nameEn, title, org, extension, phone, email, website, location, superior sql.NullString

	err := db.conn.QueryRow(query, uid).Scan(
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
func (db *DB) SearchContactsByName(name string) ([]Contact, error) {
	// Validate input
	if len(name) > 100 {
		return nil, fmt.Errorf("search term too long")
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
	rows, err := db.conn.Query(query, likePattern, likePattern, ttlTimestamp)
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
func (db *DB) GetContactsByOrganization(org string) ([]Contact, error) {
	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, type, name, name_en, title, organization, superior, extension, phone, email, cached_at FROM contacts WHERE organization = ? AND cached_at > ?`

	rows, err := db.conn.Query(query, org, ttlTimestamp)
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
func (db *DB) GetAllContacts() ([]Contact, error) {
	ttlTimestamp := db.getTTLTimestamp()

	query := `SELECT uid, type, name, name_en, title, organization, extension, phone, email, website, location, superior, cached_at
		FROM contacts WHERE cached_at > ? ORDER BY type, name LIMIT 1000`

	rows, err := db.conn.Query(query, ttlTimestamp)
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
func (db *DB) DeleteExpiredContacts(ttl time.Duration) (int64, error) {
	query := `DELETE FROM contacts WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.conn.Exec(query, expiryTime)
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
func (db *DB) CountContacts() (int, error) {
	query := `SELECT COUNT(*) FROM contacts`

	var count int
	err := db.conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count contacts: %w", err)
	}
	return count, nil
}

// CourseRepository provides CRUD operations for courses table

// SaveCourse inserts or updates a course record (serializes arrays as JSON)
func (db *DB) SaveCourse(course *Course) error {
	// Check data integrity and record metrics if available
	if db.metrics != nil {
		if course.No == "" {
			db.metrics.RecordCourseIntegrityIssue("missing_no")
		}
		if course.Title == "" {
			db.metrics.RecordCourseIntegrityIssue("empty_title")
		}
	}

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
	_, err = db.conn.Exec(query,
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
func (db *DB) SaveCoursesBatch(courses []*Course) error {
	if len(courses) == 0 {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
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
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	cachedAt := time.Now().Unix()
	for _, course := range courses {
		// Serialize JSON fields
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

		_, err = stmt.Exec(
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
			return fmt.Errorf("failed to execute statement for course %s: %w", course.UID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetCourseByUID retrieves a course by UID and validates cache freshness
func (db *DB) GetCourseByUID(uid string) (*Course, error) {
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at FROM courses WHERE uid = ?`

	var course Course
	var teachersJSON, teacherURLsJSON, timesJSON, locationsJSON string
	var detailURL, note sql.NullString

	err := db.conn.QueryRow(query, uid).Scan(
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
func (db *DB) SearchCoursesByTitle(title string) ([]Course, error) {
	// Validate input
	if len(title) > 100 {
		return nil, fmt.Errorf("search term too long")
	}

	// Sanitize search term to prevent SQL LIKE special character issues
	sanitized := sanitizeSearchTerm(title)

	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at FROM courses WHERE title LIKE ? ESCAPE '\' AND cached_at > ? ORDER BY year DESC, term DESC LIMIT 500`

	rows, err := db.conn.Query(query, "%"+sanitized+"%", ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to search courses by title: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// SearchCoursesByTeacher searches courses by teacher name (max 500 results)
// Only returns non-expired cache entries based on configured TTL
func (db *DB) SearchCoursesByTeacher(teacher string) ([]Course, error) {
	// Validate input
	if len(teacher) > 100 {
		return nil, fmt.Errorf("search term too long")
	}

	// Sanitize search term to prevent SQL LIKE special character issues
	sanitized := sanitizeSearchTerm(teacher)

	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at FROM courses WHERE teachers LIKE ? ESCAPE '\' AND cached_at > ? ORDER BY year DESC, term DESC LIMIT 500`

	rows, err := db.conn.Query(query, "%"+sanitized+"%", ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to search courses by teacher: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// GetCoursesByYearTerm retrieves courses by year and term
// Only returns non-expired cache entries based on configured TTL
func (db *DB) GetCoursesByYearTerm(year, term int) ([]Course, error) {
	// Add TTL filter to prevent returning stale data
	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at FROM courses WHERE year = ? AND term = ? AND cached_at > ?`

	rows, err := db.conn.Query(query, year, term, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get courses by year and term: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// GetCoursesByRecentSemesters retrieves all courses from recent semesters (current + previous)
// Used for fuzzy character-set matching when SQL LIKE doesn't find results
// Only returns non-expired cache entries based on configured TTL
func (db *DB) GetCoursesByRecentSemesters() ([]Course, error) {
	ttlTimestamp := db.getTTLTimestamp()

	// Get up to 2000 most recent courses ordered by semester (year, term)
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at
		FROM courses WHERE cached_at > ? ORDER BY year DESC, term DESC LIMIT 2000`

	rows, err := db.conn.Query(query, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get courses by recent semesters: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// DeleteExpiredCourses removes courses older than the specified TTL
// Returns the number of deleted entries
func (db *DB) DeleteExpiredCourses(ttl time.Duration) (int64, error) {
	query := `DELETE FROM courses WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.conn.Exec(query, expiryTime)
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
func (db *DB) CountCourses() (int, error) {
	query := `SELECT COUNT(*) FROM courses`

	var count int
	err := db.conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count courses: %w", err)
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
func (db *DB) SaveSticker(sticker *Sticker) error {
	query := `
		INSERT INTO stickers (url, source, cached_at, success_count, failure_count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			source = excluded.source,
			cached_at = excluded.cached_at,
			success_count = excluded.success_count,
			failure_count = excluded.failure_count
	`
	_, err := db.conn.Exec(query,
		sticker.URL,
		sticker.Source,
		time.Now().Unix(),
		sticker.SuccessCount,
		sticker.FailureCount,
	)
	if err != nil {
		return fmt.Errorf("failed to save sticker: %w", err)
	}
	return nil
}

// GetAllStickers retrieves all stickers from database and validates cache freshness
func (db *DB) GetAllStickers() ([]Sticker, error) {
	query := `SELECT url, source, cached_at, success_count, failure_count FROM stickers`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all stickers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stickers []Sticker
	// Use configured cache duration
	ttl := int64(db.cacheTTL.Seconds())
	currentTime := time.Now().Unix()

	for rows.Next() {
		var sticker Sticker
		if err := rows.Scan(&sticker.URL, &sticker.Source, &sticker.CachedAt, &sticker.SuccessCount, &sticker.FailureCount); err != nil {
			return nil, fmt.Errorf("failed to scan sticker row: %w", err)
		}
		// Only include non-expired stickers
		if sticker.CachedAt+ttl > currentTime {
			stickers = append(stickers, sticker)
		}
	}

	return stickers, nil
}

// GetStickersBySource retrieves stickers by source type and validates TTL
func (db *DB) GetStickersBySource(source string) ([]Sticker, error) {
	query := `SELECT url, source, cached_at, success_count, failure_count FROM stickers WHERE source = ?`

	rows, err := db.conn.Query(query, source)
	if err != nil {
		return nil, fmt.Errorf("failed to get stickers by source: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stickers []Sticker
	// Use configured cache duration
	ttl := int64(db.cacheTTL.Seconds())
	currentTime := time.Now().Unix()

	for rows.Next() {
		var sticker Sticker
		if err := rows.Scan(&sticker.URL, &sticker.Source, &sticker.CachedAt, &sticker.SuccessCount, &sticker.FailureCount); err != nil {
			return nil, fmt.Errorf("failed to scan sticker row: %w", err)
		}
		// Only include non-expired stickers
		if sticker.CachedAt+ttl > currentTime {
			stickers = append(stickers, sticker)
		}
	}

	return stickers, nil
}

// UpdateStickerSuccess increments the success count for a sticker
func (db *DB) UpdateStickerSuccess(url string) error {
	query := `UPDATE stickers SET success_count = success_count + 1 WHERE url = ?`

	_, err := db.conn.Exec(query, url)
	if err != nil {
		return fmt.Errorf("failed to update sticker success count: %w", err)
	}
	return nil
}

// UpdateStickerFailure increments the failure count for a sticker
func (db *DB) UpdateStickerFailure(url string) error {
	query := `UPDATE stickers SET failure_count = failure_count + 1 WHERE url = ?`

	_, err := db.conn.Exec(query, url)
	if err != nil {
		return fmt.Errorf("failed to update sticker failure count: %w", err)
	}
	return nil
}

// CleanupExpiredStickers removes stickers older than configured TTL
// Returns the number of deleted entries
func (db *DB) CleanupExpiredStickers() (int64, error) {
	// Use configured cache duration instead of hardcoded value
	query := `DELETE FROM stickers WHERE cached_at < ?`
	expiryTime := time.Now().Add(-db.cacheTTL).Unix()

	result, err := db.conn.Exec(query, expiryTime)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired stickers: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected for stickers: %w", err)
	}
	return rowsAffected, nil
}

// CountStickers returns the total number of stickers
func (db *DB) CountStickers() (int, error) {
	query := `SELECT COUNT(*) FROM stickers`

	var count int
	err := db.conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count stickers: %w", err)
	}
	return count, nil
}

// GetStickerStats returns statistics about sticker sources
func (db *DB) GetStickerStats() (map[string]int, error) {
	query := `SELECT source, COUNT(*) as count FROM stickers GROUP BY source`

	rows, err := db.conn.Query(query)
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
// This table stores courses older than 2 years with on-demand caching and 7-day TTL

// SaveHistoricalCourse inserts or updates a historical course record
func (db *DB) SaveHistoricalCourse(course *Course) error {
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
	_, err = db.conn.Exec(query,
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
func (db *DB) SaveHistoricalCoursesBatch(courses []*Course) error {
	if len(courses) == 0 {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
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
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	cachedAt := time.Now().Unix()
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

		_, err = stmt.Exec(
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
			return fmt.Errorf("failed to execute statement for historical course %s: %w", course.UID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// SearchHistoricalCoursesByYearAndTitle searches historical courses by year and partial title match
// Only returns non-expired cache entries based on configured TTL
func (db *DB) SearchHistoricalCoursesByYearAndTitle(year int, title string) ([]Course, error) {
	// Validate input
	if len(title) > 100 {
		return nil, fmt.Errorf("search term too long")
	}

	// Sanitize search term
	sanitized := sanitizeSearchTerm(title)

	ttlTimestamp := db.getTTLTimestamp()
	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at
		FROM historical_courses WHERE year = ? AND title LIKE ? ESCAPE '\' AND cached_at > ?
		ORDER BY term DESC LIMIT 500`

	rows, err := db.conn.Query(query, year, "%"+sanitized+"%", ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to search historical courses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// SearchHistoricalCoursesByYear searches historical courses by year only
// Returns all courses for the specified year (both semesters)
// Only returns non-expired cache entries based on configured TTL
func (db *DB) SearchHistoricalCoursesByYear(year int) ([]Course, error) {
	ttlTimestamp := db.getTTLTimestamp()

	query := `SELECT uid, year, term, no, title, teachers, teacher_urls, times, locations, detail_url, note, cached_at
		FROM historical_courses WHERE year = ? AND cached_at > ?
		ORDER BY term DESC, title LIMIT 500`

	rows, err := db.conn.Query(query, year, ttlTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get historical courses by year: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanCourses(rows)
}

// DeleteExpiredHistoricalCourses removes historical courses older than the specified TTL
// Returns the number of deleted entries
func (db *DB) DeleteExpiredHistoricalCourses(ttl time.Duration) (int64, error) {
	query := `DELETE FROM historical_courses WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.conn.Exec(query, expiryTime)
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
func (db *DB) CountHistoricalCourses() (int, error) {
	query := `SELECT COUNT(*) FROM historical_courses`

	var count int
	err := db.conn.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count historical courses: %w", err)
	}
	return count, nil
}
