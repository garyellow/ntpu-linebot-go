package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// StudentRepository provides CRUD operations for students table
type StudentRepository struct {
	db *DB
}

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

	// Check TTL (7 days = 168 hours = 604800 seconds)
	ttl := int64(168 * 60 * 60)
	if student.CachedAt+ttl <= time.Now().Unix() {
		return nil, nil // Cache expired
	}

	return &student, nil
}

// SearchStudentsByName searches students by partial name match
func (db *DB) SearchStudentsByName(name string) ([]Student, error) {
	query := `SELECT id, name, department, year, cached_at FROM students WHERE name LIKE ?`

	rows, err := db.conn.Query(query, "%"+name+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to search students by name: %w", err)
	}
	defer rows.Close()

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
func (db *DB) GetStudentsByYearDept(year int, dept string) ([]Student, error) {
	query := `SELECT id, name, department, year, cached_at FROM students WHERE year = ? AND department = ?`

	rows, err := db.conn.Query(query, year, dept)
	if err != nil {
		return nil, fmt.Errorf("failed to get students by year and department: %w", err)
	}
	defer rows.Close()

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
func (db *DB) DeleteExpiredStudents(ttl time.Duration) error {
	query := `DELETE FROM students WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	_, err := db.conn.Exec(query, expiryTime)
	if err != nil {
		return fmt.Errorf("failed to delete expired students: %w", err)
	}
	return nil
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
		INSERT INTO contacts (uid, type, name, title, organization, extension, phone, email, website, location, superior, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			type = excluded.type,
			name = excluded.name,
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

// GetContactByUID retrieves a contact by UID and validates cache freshness
func (db *DB) GetContactByUID(uid string) (*Contact, error) {
	query := `SELECT uid, type, name, title, organization, extension, phone, email, website, location, superior, cached_at FROM contacts WHERE uid = ?`

	var contact Contact
	var title, org, extension, phone, email, website, location, superior sql.NullString

	err := db.conn.QueryRow(query, uid).Scan(
		&contact.UID,
		&contact.Type,
		&contact.Name,
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

	contact.Title = title.String
	contact.Organization = org.String
	contact.Extension = extension.String
	contact.Phone = phone.String
	contact.Email = email.String
	contact.Website = website.String
	contact.Location = location.String
	contact.Superior = superior.String

	// Check TTL (7 days = 168 hours = 604800 seconds)
	ttl := int64(168 * 60 * 60)
	if contact.CachedAt+ttl <= time.Now().Unix() {
		return nil, nil // Cache expired
	}

	return &contact, nil
}

// SearchContactsByName searches contacts by partial name match
func (db *DB) SearchContactsByName(name string) ([]Contact, error) {
	query := `SELECT uid, name, title, organization, phone, email, cached_at FROM contacts WHERE name LIKE ?`

	rows, err := db.conn.Query(query, "%"+name+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to search contacts by name: %w", err)
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var contact Contact
		var title, org, phone, email sql.NullString

		if err := rows.Scan(&contact.UID, &contact.Name, &title, &org, &phone, &email, &contact.CachedAt); err != nil {
			return nil, fmt.Errorf("failed to scan contact row: %w", err)
		}

		contact.Title = title.String
		contact.Organization = org.String
		contact.Phone = phone.String
		contact.Email = email.String

		contacts = append(contacts, contact)
	}

	return contacts, nil
}

// GetContactsByOrganization retrieves contacts by organization
func (db *DB) GetContactsByOrganization(org string) ([]Contact, error) {
	query := `SELECT uid, name, title, organization, phone, email, cached_at FROM contacts WHERE organization = ?`

	rows, err := db.conn.Query(query, org)
	if err != nil {
		return nil, fmt.Errorf("failed to get contacts by organization: %w", err)
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var contact Contact
		var title, org, phone, email sql.NullString

		if err := rows.Scan(&contact.UID, &contact.Name, &title, &org, &phone, &email, &contact.CachedAt); err != nil {
			return nil, fmt.Errorf("failed to scan contact row: %w", err)
		}

		contact.Title = title.String
		contact.Organization = org.String
		contact.Phone = phone.String
		contact.Email = email.String

		contacts = append(contacts, contact)
	}

	return contacts, nil
}

// DeleteExpiredContacts removes contacts older than the specified TTL
func (db *DB) DeleteExpiredContacts(ttl time.Duration) error {
	query := `DELETE FROM contacts WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	_, err := db.conn.Exec(query, expiryTime)
	if err != nil {
		return fmt.Errorf("failed to delete expired contacts: %w", err)
	}
	return nil
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
	teachersJSON, err := json.Marshal(course.Teachers)
	if err != nil {
		return fmt.Errorf("failed to marshal teachers: %w", err)
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
		INSERT INTO courses (uid, title, teachers, times, locations, year, term, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			title = excluded.title,
			teachers = excluded.teachers,
			times = excluded.times,
			locations = excluded.locations,
			year = excluded.year,
			term = excluded.term,
			cached_at = excluded.cached_at
	`
	_, err = db.conn.Exec(query,
		course.UID,
		course.Title,
		string(teachersJSON),
		string(timesJSON),
		string(locationsJSON),
		course.Year,
		course.Term,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to save course: %w", err)
	}
	return nil
}

// GetCourseByUID retrieves a course by UID and validates cache freshness
func (db *DB) GetCourseByUID(uid string) (*Course, error) {
	query := `SELECT uid, title, teachers, times, locations, year, term, cached_at FROM courses WHERE uid = ?`

	var course Course
	var teachersJSON, timesJSON, locationsJSON string

	err := db.conn.QueryRow(query, uid).Scan(
		&course.UID,
		&course.Title,
		&teachersJSON,
		&timesJSON,
		&locationsJSON,
		&course.Year,
		&course.Term,
		&course.CachedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get course by UID: %w", err)
	}

	// Deserialize JSON arrays
	if err := json.Unmarshal([]byte(teachersJSON), &course.Teachers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal teachers: %w", err)
	}
	if err := json.Unmarshal([]byte(timesJSON), &course.Times); err != nil {
		return nil, fmt.Errorf("failed to unmarshal times: %w", err)
	}
	if err := json.Unmarshal([]byte(locationsJSON), &course.Locations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal locations: %w", err)
	}

	// Check TTL (7 days = 168 hours = 604800 seconds)
	ttl := int64(168 * 60 * 60)
	if course.CachedAt+ttl <= time.Now().Unix() {
		return nil, nil // Cache expired
	}

	return &course, nil
}

// SearchCoursesByTitle searches courses by partial title match
func (db *DB) SearchCoursesByTitle(title string) ([]Course, error) {
	query := `SELECT uid, title, teachers, times, locations, year, term, cached_at FROM courses WHERE title LIKE ?`

	rows, err := db.conn.Query(query, "%"+title+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to search courses by title: %w", err)
	}
	defer rows.Close()

	return scanCourses(rows)
}

// SearchCoursesByTeacher searches courses by teacher name
func (db *DB) SearchCoursesByTeacher(teacher string) ([]Course, error) {
	query := `SELECT uid, title, teachers, times, locations, year, term, cached_at FROM courses WHERE teachers LIKE ?`

	rows, err := db.conn.Query(query, "%"+teacher+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to search courses by teacher: %w", err)
	}
	defer rows.Close()

	return scanCourses(rows)
}

// GetCoursesByYearTerm retrieves courses by year and term
func (db *DB) GetCoursesByYearTerm(year, term int) ([]Course, error) {
	query := `SELECT uid, title, teachers, times, locations, year, term, cached_at FROM courses WHERE year = ? AND term = ?`

	rows, err := db.conn.Query(query, year, term)
	if err != nil {
		return nil, fmt.Errorf("failed to get courses by year and term: %w", err)
	}
	defer rows.Close()

	return scanCourses(rows)
}

// DeleteExpiredCourses removes courses older than the specified TTL
func (db *DB) DeleteExpiredCourses(ttl time.Duration) error {
	query := `DELETE FROM courses WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	_, err := db.conn.Exec(query, expiryTime)
	if err != nil {
		return fmt.Errorf("failed to delete expired courses: %w", err)
	}
	return nil
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
		var teachersJSON, timesJSON, locationsJSON string

		if err := rows.Scan(
			&course.UID,
			&course.Title,
			&teachersJSON,
			&timesJSON,
			&locationsJSON,
			&course.Year,
			&course.Term,
			&course.CachedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan course row: %w", err)
		}

		// Deserialize JSON arrays
		if err := json.Unmarshal([]byte(teachersJSON), &course.Teachers); err != nil {
			return nil, fmt.Errorf("failed to unmarshal teachers: %w", err)
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
