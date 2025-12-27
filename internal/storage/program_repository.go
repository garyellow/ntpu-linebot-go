package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

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

// GetAllPrograms returns all unique program names with course statistics.
// If years and terms are provided (non-empty, equal length), statistics are limited to those semesters.
// If years and terms are empty or nil, all courses for each program are counted (legacy behavior).
// Programs are sorted alphabetically by name.
func (db *DB) GetAllPrograms(ctx context.Context, years, terms []int) ([]Program, error) {
	var query string
	var args []interface{}

	if len(years) > 0 && len(years) == len(terms) {
		// Build semester filter: (c.year = ? AND c.term = ?) OR ...
		var semesterConditions []string
		for i := range years {
			semesterConditions = append(semesterConditions, "(c.year = ? AND c.term = ?)")
			args = append(args, years[i], terms[i])
		}

		query = `
			SELECT
				cp.program_name,
				SUM(CASE WHEN cp.course_type = '必' THEN 1 ELSE 0 END) as required_count,
				SUM(CASE WHEN cp.course_type != '必' THEN 1 ELSE 0 END) as elective_count,
				COUNT(*) as total_count
			FROM course_programs cp
			JOIN courses c ON cp.course_uid = c.uid
			WHERE ` + strings.Join(semesterConditions, " OR ") + `
			GROUP BY cp.program_name
			ORDER BY cp.program_name
		`
	} else {
		// No semester filter - count all courses (legacy behavior)
		query = `
			SELECT
				program_name,
				SUM(CASE WHEN course_type = '必' THEN 1 ELSE 0 END) as required_count,
				SUM(CASE WHEN course_type != '必' THEN 1 ELSE 0 END) as elective_count,
				COUNT(*) as total_count
			FROM course_programs
			GROUP BY program_name
			ORDER BY program_name
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
		if err := rows.Scan(&p.Name, &p.RequiredCount, &p.ElectiveCount, &p.TotalCount); err != nil {
			return nil, fmt.Errorf("scan program: %w", err)
		}
		programs = append(programs, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate programs: %w", err)
	}

	return programs, nil
}

// GetProgramByName returns a single program with statistics by exact name match.
func (db *DB) GetProgramByName(ctx context.Context, name string) (*Program, error) {
	query := `
		SELECT
			program_name,
			SUM(CASE WHEN course_type = '必' THEN 1 ELSE 0 END) as required_count,
			SUM(CASE WHEN course_type != '必' THEN 1 ELSE 0 END) as elective_count,
			COUNT(*) as total_count
		FROM course_programs
		WHERE program_name = ?
		GROUP BY program_name
	`

	var p Program
	err := db.reader.QueryRowContext(ctx, query, name).Scan(&p.Name, &p.RequiredCount, &p.ElectiveCount, &p.TotalCount)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("query program by name: %w", err)
	}

	return &p, nil
}

// SearchPrograms searches for programs by name using fuzzy matching.
// If years and terms are provided (non-empty, equal length), statistics are limited to those semesters.
// Returns programs where name contains the search term.
func (db *DB) SearchPrograms(ctx context.Context, searchTerm string, years, terms []int) ([]Program, error) {
	// Validate input
	if len(searchTerm) > 100 {
		return nil, errors.New("search term too long")
	}

	// Sanitize search term to prevent SQL LIKE special character issues
	sanitized := sanitizeSearchTerm(searchTerm)

	var query string
	var args []interface{}

	if len(years) > 0 && len(years) == len(terms) {
		// Build semester filter: (c.year = ? AND c.term = ?) OR ...
		var semesterConditions []string
		args = append(args, "%"+sanitized+"%")
		for i := range years {
			semesterConditions = append(semesterConditions, "(c.year = ? AND c.term = ?)")
			args = append(args, years[i], terms[i])
		}

		query = `
			SELECT
				cp.program_name,
				SUM(CASE WHEN cp.course_type = '必' THEN 1 ELSE 0 END) as required_count,
				SUM(CASE WHEN cp.course_type != '必' THEN 1 ELSE 0 END) as elective_count,
				COUNT(*) as total_count
			FROM course_programs cp
			JOIN courses c ON cp.course_uid = c.uid
			WHERE cp.program_name LIKE ? ESCAPE '\' AND (` + strings.Join(semesterConditions, " OR ") + `)
			GROUP BY cp.program_name
			ORDER BY cp.program_name
		`
	} else {
		// No semester filter (legacy behavior)
		query = `
			SELECT
				program_name,
				SUM(CASE WHEN course_type = '必' THEN 1 ELSE 0 END) as required_count,
				SUM(CASE WHEN course_type != '必' THEN 1 ELSE 0 END) as elective_count,
				COUNT(*) as total_count
			FROM course_programs
			WHERE program_name LIKE ? ESCAPE '\'
			GROUP BY program_name
			ORDER BY program_name
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
		if err := rows.Scan(&p.Name, &p.RequiredCount, &p.ElectiveCount, &p.TotalCount); err != nil {
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

	if len(years) > 0 && len(years) == len(terms) {
		// Build semester filter: (year = ? AND term = ?) OR (year = ? AND term = ?) ...
		var semesterConditions []string
		args = append(args, programName)
		for i := range years {
			semesterConditions = append(semesterConditions, "(c.year = ? AND c.term = ?)")
			args = append(args, years[i], terms[i])
		}

		query = `
			SELECT
				c.uid, c.year, c.term, c.no, c.title, c.teachers, c.teacher_urls,
				c.times, c.locations, c.detail_url, c.note, c.cached_at,
				cp.course_type
			FROM course_programs cp
			JOIN courses c ON cp.course_uid = c.uid
			WHERE cp.program_name = ? AND (` + strings.Join(semesterConditions, " OR ") + `)
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

// DeleteExpiredCoursePrograms deletes course-program relationships older than TTL.
func (db *DB) DeleteExpiredCoursePrograms(ctx context.Context) (int64, error) {
	threshold := time.Now().Add(-db.cacheTTL).Unix()

	result, err := db.writer.ExecContext(ctx, "DELETE FROM course_programs WHERE cached_at < ?", threshold)
	if err != nil {
		return 0, fmt.Errorf("delete expired course programs: %w", err)
	}

	return result.RowsAffected()
}

// parseJSONArray parses a JSON array string into a slice.
// Handles empty or invalid JSON gracefully.
func parseJSONArray(jsonStr string) []string {
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return nil
	}

	// Remove brackets and split by comma
	jsonStr = strings.Trim(jsonStr, "[]")
	if jsonStr == "" {
		return nil
	}

	// Split by comma and clean up quotes
	parts := strings.Split(jsonStr, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"")
		if p != "" {
			result = append(result, p)
		}
	}

	return result
}
