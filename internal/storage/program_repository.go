package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// buildSemesterConditions creates SQL conditions for filtering by semesters.
// Returns the SQL condition string (e.g., "(c.year = ? AND c.term = ?) OR ..."),
// the placeholder values as args, and whether any conditions were built.
// The tablePrefix should be "c" for courses table references.
func buildSemesterConditions(years, terms []int, tablePrefix string) (conditions string, args []interface{}, ok bool) {
	if len(years) == 0 || len(years) != len(terms) {
		return "", nil, false
	}

	parts := make([]string, 0, len(years))
	args = make([]interface{}, 0, len(years)*2)
	for i := range years {
		parts = append(parts, fmt.Sprintf("(%s.year = ? AND %s.term = ?)", tablePrefix, tablePrefix))
		args = append(args, years[i], terms[i])
	}
	return strings.Join(parts, " OR "), args, true
}

// SaveCoursePrograms saves course-program relationships for a course.
// This replaces any existing program relationships for the course.
func (db *DB) SaveCoursePrograms(ctx context.Context, courseUID string, programs []ProgramRequirement) error {
	if len(programs) == 0 {
		return nil
	}

	tx, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing relationships for this course
	_, err = tx.ExecContext(ctx, "DELETE FROM course_programs WHERE course_uid = ?", courseUID)
	if err != nil {
		return fmt.Errorf("delete existing course programs: %w", err)
	}

	// Insert new relationships
	cachedAt := time.Now().Unix()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO course_programs (course_uid, program_name, course_type, cached_at)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, p := range programs {
		_, err = stmt.ExecContext(ctx, courseUID, p.ProgramName, p.CourseType, cachedAt)
		if err != nil {
			return fmt.Errorf("insert course program: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// SaveCourseProgramsBatch saves course-program relationships for multiple courses.
// This is more efficient than calling SaveCoursePrograms multiple times.
func (db *DB) SaveCourseProgramsBatch(ctx context.Context, courses []*Course) error {
	// Filter courses that have program relationships
	var coursesWithPrograms []*Course
	for _, c := range courses {
		if len(c.Programs) > 0 {
			coursesWithPrograms = append(coursesWithPrograms, c)
		}
	}

	if len(coursesWithPrograms) == 0 {
		return nil
	}

	tx, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	cachedAt := time.Now().Unix()

	// Prepare statements
	deleteStmt, err := tx.PrepareContext(ctx, "DELETE FROM course_programs WHERE course_uid = ?")
	if err != nil {
		return fmt.Errorf("prepare delete statement: %w", err)
	}
	defer func() { _ = deleteStmt.Close() }()

	insertStmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO course_programs (course_uid, program_name, course_type, cached_at)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert statement: %w", err)
	}
	defer func() { _ = insertStmt.Close() }()

	for _, course := range coursesWithPrograms {
		// Delete existing relationships
		_, err = deleteStmt.ExecContext(ctx, course.UID)
		if err != nil {
			return fmt.Errorf("delete existing course programs for %s: %w", course.UID, err)
		}

		// Insert new relationships
		for _, p := range course.Programs {
			_, err = insertStmt.ExecContext(ctx, course.UID, p.ProgramName, p.CourseType, cachedAt)
			if err != nil {
				return fmt.Errorf("insert course program for %s: %w", course.UID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// SyncPrograms synchronizes program metadata (name + category + URL) from static data.
// Uses INSERT OR REPLACE to upsert program information.
func (db *DB) SyncPrograms(ctx context.Context, programs []struct{ Name, Category, URL string }) error {
	if len(programs) == 0 {
		return nil
	}

	tx, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	cachedAt := time.Now().Unix()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO programs (name, category, url, cached_at)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, p := range programs {
		_, err = stmt.ExecContext(ctx, p.Name, p.Category, p.URL, cachedAt)
		if err != nil {
			return fmt.Errorf("insert program %s (%s): %w", p.Name, p.Category, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// GetAllPrograms returns all unique program names with course statistics and LMS URLs.
// If years and terms are provided (non-empty, equal length), statistics are limited to those semesters.
// If years and terms are empty or nil, all courses for each program are counted (legacy behavior).
// Programs are sorted alphabetically by name.
// URL is fetched from the programs table via LEFT JOIN.
func (db *DB) GetAllPrograms(ctx context.Context, years, terms []int) ([]Program, error) {
	var query string
	var args []interface{}

	if semesterCond, semesterArgs, ok := buildSemesterConditions(years, terms, "c"); ok {
		args = semesterArgs

		// Query flipped: Select from programs table first (Source of Truth)
		// Note: course_programs is joined via LEFT JOIN, so programs without courses still appear.
		// Semester condition is embedded in CASE expressions to count only matching courses.
		query = `
			SELECT
				p.name,
				MAX(p.category) as category,
				COALESCE(p.url, '') as url,
				SUM(CASE WHEN cp.course_type = '必' AND (` + semesterCond + `) THEN 1 ELSE 0 END) as required_count,
				SUM(CASE WHEN cp.course_type != '必' AND (` + semesterCond + `) THEN 1 ELSE 0 END) as elective_count,
				SUM(CASE WHEN (` + semesterCond + `) THEN 1 ELSE 0 END) as total_count
			FROM programs p
			LEFT JOIN course_programs cp ON p.name = cp.program_name
			LEFT JOIN courses c ON cp.course_uid = c.uid
			GROUP BY p.name
			ORDER BY p.name
		`
	} else {
		// No semester filter - count all courses for each program in LMS list
		query = `
			SELECT
				p.name,
				MAX(p.category) as category,
				COALESCE(p.url, '') as url,
				SUM(CASE WHEN cp.course_type = '必' THEN 1 ELSE 0 END) as required_count,
				SUM(CASE WHEN cp.course_type != '必' THEN 1 ELSE 0 END) as elective_count,
				COUNT(cp.course_uid) as total_count
			FROM programs p
			LEFT JOIN course_programs cp ON p.name = cp.program_name
			GROUP BY p.name
			ORDER BY p.name
		`
	}
	rows, err := db.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query programs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var programs []Program
	for rows.Next() {
		var p Program
		if err := rows.Scan(&p.Name, &p.Category, &p.URL, &p.RequiredCount, &p.ElectiveCount, &p.TotalCount); err != nil {
			return nil, fmt.Errorf("scan program: %w", err)
		}
		programs = append(programs, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate programs: %w", err)
	}

	return programs, nil
}

// GetProgramByName returns a single program with statistics and URL by exact name match.
func (db *DB) GetProgramByName(ctx context.Context, name string) (*Program, error) {
	query := `
		SELECT
			p.name,
			MAX(p.category) as category,
			COALESCE(p.url, '') as url,
			SUM(CASE WHEN cp.course_type = '必' THEN 1 ELSE 0 END) as required_count,
			SUM(CASE WHEN cp.course_type != '必' THEN 1 ELSE 0 END) as elective_count,
			COUNT(cp.course_uid) as total_count
		FROM programs p
		LEFT JOIN course_programs cp ON p.name = cp.program_name
		WHERE p.name = ?
		GROUP BY p.name
	`

	var prog Program
	err := db.reader.QueryRowContext(ctx, query, name).Scan(&prog.Name, &prog.Category, &prog.URL, &prog.RequiredCount, &prog.ElectiveCount, &prog.TotalCount)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("query program by name: %w", err)
	}

	return &prog, nil
}

// SearchPrograms searches for programs by name using fuzzy matching.
// If years and terms are provided (non-empty, equal length), statistics are limited to those semesters.
// Returns programs where name contains the search term, including URL from programs table.
func (db *DB) SearchPrograms(ctx context.Context, searchTerm string, years, terms []int) ([]Program, error) {
	// Validate input
	if len(searchTerm) > 100 {
		return nil, errors.New("search term too long")
	}

	// Sanitize search term to prevent SQL LIKE special character issues
	sanitized := sanitizeSearchTerm(searchTerm)

	var query string
	var args []interface{}

	if semesterCond, semesterArgs, ok := buildSemesterConditions(years, terms, "c"); ok {
		// Search term first, then semester args
		args = append(args, "%"+sanitized+"%")
		args = append(args, semesterArgs...)

		query = `
			SELECT
				p.name,
				MAX(p.category) as category,
				COALESCE(p.url, '') as url,
				SUM(CASE WHEN cp.course_type = '必' AND (` + semesterCond + `) THEN 1 ELSE 0 END) as required_count,
				SUM(CASE WHEN cp.course_type != '必' AND (` + semesterCond + `) THEN 1 ELSE 0 END) as elective_count,
				SUM(CASE WHEN (` + semesterCond + `) THEN 1 ELSE 0 END) as total_count
			FROM programs p
			LEFT JOIN course_programs cp ON p.name = cp.program_name
			LEFT JOIN courses c ON cp.course_uid = c.uid
			WHERE p.name LIKE ? ESCAPE '\'
			GROUP BY p.name
			ORDER BY p.name
		`
	} else {
		// No semester filter (legacy behavior)
		query = `
			SELECT
				p.name,
				MAX(p.category) as category,
				COALESCE(p.url, '') as url,
				SUM(CASE WHEN cp.course_type = '必' THEN 1 ELSE 0 END) as required_count,
				SUM(CASE WHEN cp.course_type != '必' THEN 1 ELSE 0 END) as elective_count,
				COUNT(cp.course_uid) as total_count
			FROM programs p
			LEFT JOIN course_programs cp ON p.name = cp.program_name
			WHERE p.name LIKE ? ESCAPE '\'
			GROUP BY p.name
			ORDER BY p.name
		`
		args = []interface{}{"%" + sanitized + "%"}
	}

	rows, err := db.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search programs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var programs []Program
	for rows.Next() {
		var p Program
		if err := rows.Scan(&p.Name, &p.Category, &p.URL, &p.RequiredCount, &p.ElectiveCount, &p.TotalCount); err != nil {
			return nil, fmt.Errorf("scan program: %w", err)
		}
		programs = append(programs, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate programs: %w", err)
	}

	return programs, nil
}

// GetProgramCourses returns courses for a given program, optionally filtered by semesters.
// If years and terms are provided (non-empty, equal length), only courses from those semesters are returned.
// If years and terms are empty or nil, all courses for the program are returned.
// Courses are sorted by requirement type (必修 first, then 選修), then by semester (newest first).
func (db *DB) GetProgramCourses(ctx context.Context, programName string, years, terms []int) ([]ProgramCourse, error) {
	// Build query with optional semester filter
	var query string
	var args []interface{}

	if semesterCond, semesterArgs, ok := buildSemesterConditions(years, terms, "c"); ok {
		// Program name first, then semester args
		args = append(args, programName)
		args = append(args, semesterArgs...)

		query = `
			SELECT
				c.uid, c.year, c.term, c.no, c.title, c.teachers, c.teacher_urls,
				c.times, c.locations, c.detail_url, c.note, c.cached_at,
				cp.course_type
			FROM course_programs cp
			JOIN courses c ON cp.course_uid = c.uid
			WHERE cp.program_name = ? AND (` + semesterCond + `)
			ORDER BY
				CASE WHEN cp.course_type = '必' THEN 0 ELSE 1 END,
				c.year DESC,
				c.term DESC
		`
	} else {
		// No semester filter - return all courses for the program
		query = `
			SELECT
				c.uid, c.year, c.term, c.no, c.title, c.teachers, c.teacher_urls,
				c.times, c.locations, c.detail_url, c.note, c.cached_at,
				cp.course_type
			FROM course_programs cp
			JOIN courses c ON cp.course_uid = c.uid
			WHERE cp.program_name = ?
			ORDER BY
				CASE WHEN cp.course_type = '必' THEN 0 ELSE 1 END,
				c.year DESC,
				c.term DESC
		`
		args = []interface{}{programName}
	}

	rows, err := db.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query program courses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var courses []ProgramCourse
	for rows.Next() {
		var pc ProgramCourse
		var teachers, teacherURLs, times, locations string
		var detailURL, note sql.NullString

		err := rows.Scan(
			&pc.Course.UID, &pc.Course.Year, &pc.Course.Term, &pc.Course.No,
			&pc.Course.Title, &teachers, &teacherURLs, &times, &locations,
			&detailURL, &note, &pc.Course.CachedAt,
			&pc.CourseType,
		)
		if err != nil {
			return nil, fmt.Errorf("scan program course: %w", err)
		}

		// Handle nullable fields
		pc.Course.DetailURL = detailURL.String
		pc.Course.Note = note.String

		// Parse JSON arrays
		pc.Course.Teachers = parseJSONArray(teachers)
		pc.Course.TeacherURLs = parseJSONArray(teacherURLs)
		pc.Course.Times = parseJSONArray(times)
		pc.Course.Locations = parseJSONArray(locations)

		courses = append(courses, pc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate program courses: %w", err)
	}

	return courses, nil
}

// GetCoursePrograms returns all programs that a course belongs to.
// This enables the "相關學程" button on course detail pages.
// Programs are sorted by requirement type (必修 first).
func (db *DB) GetCoursePrograms(ctx context.Context, courseUID string) ([]ProgramRequirement, error) {
	query := `
		SELECT program_name, course_type
		FROM course_programs
		WHERE course_uid = ?
		ORDER BY
			CASE WHEN course_type = '必' THEN 0 ELSE 1 END,
			program_name
	`

	rows, err := db.reader.QueryContext(ctx, query, courseUID)
	if err != nil {
		return nil, fmt.Errorf("query course programs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var programs []ProgramRequirement
	for rows.Next() {
		var p ProgramRequirement
		if err := rows.Scan(&p.ProgramName, &p.CourseType); err != nil {
			return nil, fmt.Errorf("scan course program: %w", err)
		}
		programs = append(programs, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate course programs: %w", err)
	}

	return programs, nil
}

// DeleteExpiredCoursePrograms removes course-program relationships older than the specified TTL.
// Returns the number of deleted entries.
func (db *DB) DeleteExpiredCoursePrograms(ctx context.Context, ttl time.Duration) (int64, error) {
	query := `DELETE FROM course_programs WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.writer.ExecContext(ctx, query, expiryTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired course programs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected for course programs: %w", err)
	}
	return rowsAffected, nil
}

// DeleteExpiredPrograms removes programs older than the specified TTL.
// Returns the number of deleted entries.
func (db *DB) DeleteExpiredPrograms(ctx context.Context, ttl time.Duration) (int64, error) {
	query := `DELETE FROM programs WHERE cached_at < ?`
	expiryTime := time.Now().Add(-ttl).Unix()

	result, err := db.writer.ExecContext(ctx, query, expiryTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired programs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected for programs: %w", err)
	}
	return rowsAffected, nil
}

// CountPrograms returns the total number of programs in the database.
func (db *DB) CountPrograms(ctx context.Context) (int, error) {
	var count int
	err := db.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM programs").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count programs: %w", err)
	}
	return count, nil
}

// parseJSONArray parses a JSON array string into a slice.
// Handles empty or invalid JSON gracefully.
func parseJSONArray(jsonStr string) []string {
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return nil
	}

	var result []string
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// On error, return nil instead of failing partially
		// This can happen if the DB data is corrupted or not in JSON format
		return nil
	}
	return result
}
