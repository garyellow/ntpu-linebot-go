// Package warmup provides background cache warming functionality.
//
// Daily refresh (3:00 AM Taiwan time):
//   - contact, course, program: Always refreshed (7-day TTL)
//   - syllabus: ONLY processes most recent 2 semesters with data (auto-enabled when LLM API key configured)
//
// Not in daily refresh: id (static; typically startup only), sticker (startup only)
//
// CRITICAL: Syllabus scraping is ONLY performed during warmup - never in real-time user queries.
// User queries (e.g., smart search) use the pre-built BM25 index from cached syllabi (read-only).
package warmup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/data"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/course"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/syllabus"
	"golang.org/x/sync/errgroup"
)

// Stats tracks cache warming statistics for daily refresh operations.
// All fields use atomic operations for concurrent access.
// Note: Students are not tracked here as they are only warmed on startup (static data).
type Stats struct {
	Contacts atomic.Int64
	Courses  atomic.Int64
	Programs atomic.Int64
	Syllabi  atomic.Int64
}

// Options configures cache warming behavior
type Options struct {
	Reset     bool
	HasLLMKey bool // Enables syllabus module for smart search
	WarmID    bool // Enables ID module warmup (static data, startup only, not used in daily refresh)
	Metrics   *metrics.Metrics
	BM25Index *rag.BM25Index
}

// Run executes daily cache refresh: contact, course, program (always), syllabus (if HasLLMKey).
func Run(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, opts Options) (*Stats, error) {
	stats := &Stats{}
	startTime := time.Now()

	if opts.Reset {
		log.Info("Resetting cache data...")
		if err := resetCache(ctx, db); err != nil {
			return nil, fmt.Errorf("failed to reset cache: %w", err)
		}
		log.Info("Cache reset complete")
	}

	hasSyllabus := opts.HasLLMKey

	g, ctx := errgroup.WithContext(ctx)

	// Channel to signal course completion (for syllabus dependency)
	courseDone := make(chan struct{})

	g.Go(func() error {
		if err := warmupContactModule(ctx, db, client, log, stats, opts.Metrics); err != nil {
			log.WithError(err).Error("Contact module warmup failed")
			return fmt.Errorf("contact module: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		defer close(courseDone)
		if err := warmupCourseModule(ctx, db, client, log, stats, opts.Metrics); err != nil {
			log.WithError(err).Error("Course module warmup failed")
			return fmt.Errorf("course module: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := warmupProgramModule(ctx, db, client, log, stats); err != nil {
			log.WithError(err).Warn("Program module warmup failed")
			// Don't fail the entire warmup for program sync errors
			return nil
		}
		return nil
	})

	if opts.WarmID {
		g.Go(func() error {
			if err := warmupIDModule(ctx, db, client, log, opts.Metrics); err != nil {
				log.WithError(err).Error("ID module warmup failed")
				return fmt.Errorf("id module: %w", err)
			}
			return nil
		})
	}

	if hasSyllabus {
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-courseDone:
			}

			if opts.BM25Index == nil {
				return nil
			}

			if err := warmupSyllabusModule(ctx, db, client, opts.BM25Index, log, stats, opts.Metrics); err != nil {
				log.WithError(err).Error("Syllabus module warmup failed")
				return fmt.Errorf("syllabus: %w", err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return stats, fmt.Errorf("warmup: %w", err)
	}

	log.WithField("duration", time.Since(startTime)).
		WithField("contacts", stats.Contacts.Load()).
		WithField("courses", stats.Courses.Load()).
		WithField("programs", stats.Programs.Load()).
		WithField("syllabi", stats.Syllabi.Load()).
		Info("Cache warming complete")

	return stats, nil
}

// RunInBackground executes cache warming asynchronously (non-blocking).
//
//nolint:contextcheck // Intentionally using context.Background() for independent background operation
func RunInBackground(_ context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, opts Options) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.WithField("panic", r).Error("Panic in background cache warming")
			}
		}()

		log.WithField("has_llm_key", opts.HasLLMKey).
			Info("Starting background cache warming")

		stats, err := Run(context.Background(), db, client, log, opts)
		if err != nil {
			log.WithError(err).Warn("Background cache warming finished with errors")
		} else {
			log.WithField("contacts", stats.Contacts.Load()).
				WithField("courses", stats.Courses.Load()).
				WithField("syllabi", stats.Syllabi.Load()).
				Info("Background cache warming completed successfully")
		}
	}()
}

// resetCache deletes all cached data
func resetCache(ctx context.Context, db *storage.DB) error {
	tables := []string{"students", "contacts", "courses"}
	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s", table)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to delete from %s: %w", table, err)
		}
	}
	// Run VACUUM to reclaim space
	if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("failed to vacuum: %w", err)
	}
	return nil
}

// warmupProgramModule syncs program metadata from LMS to database.
// 1. Try dynamic scraping from LMS (auto-discovers new programs)
// 2. Fall back to static data if scraping fails or returns no results
func warmupProgramModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats) error {
	// Try to scrape programs from LMS
	var programsToSync []struct{ Name, Category, URL string }

	dynamicPrograms, err := ntpu.ScrapePrograms(ctx, client)
	if err == nil && len(dynamicPrograms) > 0 {
		log.WithField("count", len(dynamicPrograms)).Info("Scraped program data from LMS")
		programsToSync = make([]struct{ Name, Category, URL string }, len(dynamicPrograms))
		for i, p := range dynamicPrograms {
			programsToSync[i] = struct{ Name, Category, URL string }{Name: p.Name, Category: p.Category, URL: p.URL}
		}
	} else {
		// Fallback to static data
		if err != nil {
			log.WithError(err).Warn("Failed to scrape programs, using static data")
		} else {
			log.Warn("Scraped 0 programs, using static data as fallback")
		}
		programsToSync = make([]struct{ Name, Category, URL string }, len(data.AllPrograms))
		for i, p := range data.AllPrograms {
			programsToSync[i] = struct{ Name, Category, URL string }{Name: p.Name, Category: "", URL: p.URL}
		}
	}

	if err := db.SyncPrograms(ctx, programsToSync); err != nil {
		return fmt.Errorf("sync programs: %w", err)
	}

	stats.Programs.Store(int64(len(programsToSync)))
	log.WithField("count", len(programsToSync)).Info("Program metadata synced")
	return nil
}

// warmupIDModule warms student ID cache (sequential execution).
func warmupIDModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, m *metrics.Metrics) error {
	startTime := time.Now()
	defer func() {
		if m != nil {
			m.RecordJob("warmup", "id", time.Since(startTime).Seconds())
		}
	}()

	// Warmup range: 101-113 (LMS 2.0 已無 114+ 資料)
	// Warmup range: 101-113 (LMS 2.0 已無 114+ 資料)
	currentYear := time.Now().Year() - 1911
	fromYear := min(config.IDDataYearEnd, currentYear)

	// All department codes
	departments := []string{
		"71", "712", "714", "716", "72", "73", "742", "744",
		"75", "76", "77", "78", "79", "80", "81", "82", "83", "84", "85", "86", "87",
	}

	totalTasks := (fromYear - 100) * len(departments)
	log.WithField("tasks", totalTasks).Info("Starting ID module warmup")

	var completed int
	var errorCount int
	var studentCount int64
	var errs []error

	for year := fromYear; year > 100; year-- {
		for _, dept := range departments {
			select {
			case <-ctx.Done():
				log.WithField("completed", completed).
					WithField("errors", errorCount).
					Warn("ID module warmup canceled")
				return fmt.Errorf("canceled: %w", ctx.Err())
			default:
			}

			students, err := ntpu.ScrapeStudentsByYear(ctx, client, year, dept)
			if err != nil {
				log.WithError(err).
					WithField("year", year).
					WithField("dept", dept).
					Warn("Failed to scrape students")
				errs = append(errs, fmt.Errorf("scrape year=%d dept=%s: %w", year, dept, err))
				errorCount++
				continue
			}

			// Save to database
			if err := db.SaveStudentsBatch(ctx, students); err != nil {
				log.WithError(err).
					WithField("year", year).
					WithField("dept", dept).
					WithField("count", len(students)).
					Warn("Failed to save student batch")
				errs = append(errs, fmt.Errorf("save year=%d dept=%s: %w", year, dept, err))
				errorCount++
				continue
			}

			studentCount += int64(len(students))
			completed++

			if completed%10 == 0 || completed == totalTasks {
				elapsed := time.Since(startTime)
				avgTimePerTask := elapsed / time.Duration(completed)
				estimatedRemaining := avgTimePerTask * time.Duration(totalTasks-completed)
				log.WithField("progress", fmt.Sprintf("%d/%d", completed, totalTasks)).
					WithField("students", studentCount).
					WithField("elapsed_min", int(elapsed.Minutes())).
					WithField("est_remaining_min", int(estimatedRemaining.Minutes())).
					Info("ID module progress")
			}
		}
	}

	if errorCount > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// warmupContactModule warms contact cache (allows partial success).
func warmupContactModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats, m *metrics.Metrics) error {
	startTime := time.Now()
	defer func() {
		if m != nil {
			m.RecordJob("warmup", "contact", time.Since(startTime).Seconds())
		}
	}()

	log.Info("Starting contact module warmup")

	var errs []error

	adminContacts, err := ntpu.ScrapeAdministrativeContacts(ctx, client)
	if err != nil {
		log.WithError(err).Warn("Failed to scrape administrative contacts, continuing anyway")
		errs = append(errs, fmt.Errorf("administrative contacts: %w", err))
	} else {
		if err := db.SaveContactsBatch(ctx, adminContacts); err != nil {
			log.WithError(err).Warn("Failed to save administrative contacts batch")
			errs = append(errs, fmt.Errorf("save administrative contacts: %w", err))
		} else {
			stats.Contacts.Add(int64(len(adminContacts)))
			log.WithField("count", len(adminContacts)).Info("Administrative contacts cached")
		}
	}

	academicContacts, err := ntpu.ScrapeAcademicContacts(ctx, client)
	if err != nil {
		log.WithError(err).Warn("Failed to scrape academic contacts, continuing anyway")
		errs = append(errs, fmt.Errorf("academic contacts: %w", err))
	} else {
		// Save using batch operation to reduce lock contention
		if err := db.SaveContactsBatch(ctx, academicContacts); err != nil {
			log.WithError(err).Warn("Failed to save academic contacts batch")
			errs = append(errs, fmt.Errorf("save academic contacts: %w", err))
		} else {
			stats.Contacts.Add(int64(len(academicContacts)))
			log.WithField("count", len(academicContacts)).Info("Academic contacts cached")
		}
	}

	// Return error only if both failed
	// This allows the warmup to succeed with partial data (e.g., only academic or only administrative)
	if len(errs) == 2 {
		return fmt.Errorf("both contact sources failed: %w", errors.Join(errs[0], errs[1]))
	}

	// Log partial success details
	if len(errs) == 1 {
		log.WithField("failed_source", errs[0]).Info("Contact module completed with partial success")
	}

	return nil
}

// warmupCourseModule warms course cache for the 4 most recent semesters.
// Uses intelligent detection based on actual data availability.
func warmupCourseModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats, m *metrics.Metrics) error {
	startTime := time.Now()
	defer func() {
		if m != nil {
			m.RecordJob("warmup", "course", time.Since(startTime).Seconds())
		}
	}()

	log.Info("Starting course module warmup with intelligent semester detection")

	// Use intelligent detection to determine which 4 semesters to warm up
	// Detection: checks if semester has any data (> 0 courses)
	semesters := detectWarmupSemesters(ctx, db, log)

	// Each semester makes 4 requests (U/M/N/P education codes)
	estimatedRequests := len(semesters) * 4
	log.WithField("semesters", formatSemesters(semesters)).
		WithField("total_semesters", len(semesters)).
		WithField("estimated_requests", estimatedRequests).
		Info("Course warmup: fetching courses by semester (intelligent detection)")

	// Scrape courses for each semester individually
	for _, sem := range semesters {
		select {
		case <-ctx.Done():
			return fmt.Errorf("course module canceled: %w", ctx.Err())
		default:
		}

		// Fetch courses for this specific semester using ScrapeCourses
		// This makes 4 HTTP requests (one per education code: U/M/N/P)
		courses, err := ntpu.ScrapeCourses(ctx, client, sem.Year, sem.Term, "")
		if err != nil {
			log.WithError(err).
				WithField("year", sem.Year).
				WithField("term", sem.Term).
				Warn("Failed to scrape courses for semester")
			continue
		}

		// Save using batch operation to reduce lock contention
		if err := db.SaveCoursesBatch(ctx, courses); err != nil {
			log.WithError(err).
				WithField("year", sem.Year).
				WithField("term", sem.Term).
				WithField("count", len(courses)).
				Warn("Failed to save courses batch")
			continue
		}

		// Cleanup potential cold data to ensure strict partitioning
		// If we successfully saved to 'courses' (Hot), we must remove from 'historical_courses' (Cold)
		if err := db.DeleteHistoricalCoursesByYearTerm(ctx, sem.Year, sem.Term); err != nil {
			log.WithError(err).
				WithField("year", sem.Year).
				WithField("term", sem.Term).
				Warn("Failed to cleanup historical courses (non-critical)")
		}

		// Save course-program relationships
		if err := db.SaveCourseProgramsBatch(ctx, courses); err != nil {
			log.WithError(err).
				WithField("year", sem.Year).
				WithField("term", sem.Term).
				Warn("Failed to save course programs batch")
			// Continue even if program save fails - course data is still valid
		}

		stats.Courses.Add(int64(len(courses)))
		log.WithField("year", sem.Year).
			WithField("term", sem.Term).
			WithField("count", len(courses)).
			WithField("total_cached", stats.Courses.Load()).
			Info("Courses cached for semester")
	}

	log.WithField("total_courses", stats.Courses.Load()).
		WithField("semesters_processed", len(semesters)).
		Info("Course module warmup complete")

	return nil
}

// detectWarmupSemesters determines which 4 semesters to warm up based on data availability.
func detectWarmupSemesters(ctx context.Context, db *storage.DB, log *logger.Logger) []course.Semester {
	// Create semester detector with database count function
	detector := course.NewSemesterDetector(db.CountCoursesBySemester)

	// Use intelligent detection to get 4 most recent semesters with data
	semesters := detector.DetectWarmupSemesters(ctx)

	log.WithField("semesters", formatSemesters(semesters)).
		WithField("total_semesters", len(semesters)).
		Info("Detected warmup semesters using intelligent data-driven detection")

	return semesters
}

// formatSemesters formats semester list for logging
// Example: [{113 2} {113 1} {112 2} {112 1}] -> "113-2, 113-1, 112-2, 112-1"
func formatSemesters(semesters []course.Semester) string {
	if len(semesters) == 0 {
		return ""
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("%d-%d", semesters[0].Year, semesters[0].Term))
	for i := 1; i < len(semesters); i++ {
		result.WriteString(fmt.Sprintf(", %d-%d", semesters[i].Year, semesters[i].Term))
	}
	return result.String()
}

// warmupSyllabusModule warms syllabus cache and BM25 index
// ONLY processes courses from the most recent 2 semesters (with cached data)
// Uses content hash for incremental updates - only re-scrapes changed syllabi
// Other semesters are not processed to reduce scraping load
func warmupSyllabusModule(ctx context.Context, db *storage.DB, client *scraper.Client, bm25Index *rag.BM25Index, log *logger.Logger, stats *Stats, m *metrics.Metrics) error {
	startTime := time.Now()
	defer func() {
		if m != nil {
			m.RecordJob("warmup", "syllabus", time.Since(startTime).Seconds())
		}
	}()

	log.Info("Starting syllabus module warmup")

	// Get the most recent 2 semesters that have cached data
	semesters, err := db.GetDistinctRecentSemesters(ctx, 2)
	if err != nil {
		return fmt.Errorf("failed to get recent semesters: %w", err)
	}

	if len(semesters) == 0 {
		log.Info("No recent semesters found for syllabus warmup")
		return nil
	}

	log.WithField("semesters", len(semesters)).Info("Found recent semesters for syllabus warmup")

	// Collect courses from the most recent 2 semesters only
	var courses []storage.Course
	for _, sem := range semesters {
		semCourses, err := db.GetCoursesByYearTerm(ctx, sem.Year, sem.Term)
		if err != nil {
			log.WithError(err).WithField("year", sem.Year).WithField("term", sem.Term).Warn("Failed to get courses for semester")
			continue
		}
		courses = append(courses, semCourses...)
		log.WithField("year", sem.Year).WithField("term", sem.Term).WithField("count", len(semCourses)).Info("Loaded courses for semester")
	}

	if len(courses) == 0 {
		log.Info("No courses found in recent semesters for syllabus warmup")
		return nil
	}

	log.WithField("total_courses", len(courses)).Info("Processing syllabi for courses from recent 2 semesters")

	// Create syllabus scraper
	syllabusScraper := syllabus.NewScraper(client)

	// Process courses and extract syllabi
	var newSyllabi []*storage.Syllabus
	var updatedCount, skippedCount, errorCount int

processLoop:
	for i, course := range courses {
		select {
		case <-ctx.Done():
			log.WithField("processed", i).WithField("total", len(courses)).
				Info("Syllabus warmup interrupted")
			break processLoop
		default:
		}

		// Skip courses without detail URL
		if course.DetailURL == "" {
			skippedCount++
			continue
		}

		// Scrape syllabus content
		fields, err := syllabusScraper.ScrapeSyllabus(ctx, &course)
		if err != nil {
			log.WithError(err).WithField("uid", course.UID).Debug("Failed to scrape syllabus")
			errorCount++
			continue
		}

		// Skip empty syllabi
		if fields.IsEmpty() {
			skippedCount++
			continue
		}

		// Compute content hash from syllabus content fields for change detection
		// Note: Course title is stored separately and not included in hash to avoid
		// unnecessary re-indexing when only the title changes (content remains same)
		contentForHash := fields.Objectives + "\n" + fields.Outline + "\n" + fields.Schedule
		contentHash := syllabus.ComputeContentHash(contentForHash)

		// Check if content has changed (incremental update)
		existingHash, err := db.GetSyllabusContentHash(ctx, course.UID)
		if err != nil {
			log.WithError(err).WithField("uid", course.UID).Debug("Failed to get existing hash")
		}

		if existingHash == contentHash {
			// Content unchanged, skip
			skippedCount++
			continue
		}

		// Create syllabus record with unified content fields
		syl := &storage.Syllabus{
			UID:         course.UID,
			Year:        course.Year,
			Term:        course.Term,
			Title:       course.Title,
			Teachers:    course.Teachers,
			Objectives:  fields.Objectives,
			Outline:     fields.Outline,
			Schedule:    fields.Schedule,
			ContentHash: contentHash,
		}

		newSyllabi = append(newSyllabi, syl)
		updatedCount++

		// Log progress every 50 courses for better visibility
		if i > 0 && i%50 == 0 {
			elapsed := time.Since(startTime)
			avgTimePerCourse := elapsed / time.Duration(i)
			estimatedRemaining := avgTimePerCourse * time.Duration(len(courses)-i)
			log.WithField("progress", fmt.Sprintf("%d/%d", i, len(courses))).
				WithField("updated", updatedCount).
				WithField("skipped", skippedCount).
				WithField("errors", errorCount).
				WithField("elapsed_min", int(elapsed.Minutes())).
				WithField("est_remaining_min", int(estimatedRemaining.Minutes())).
				Info("Syllabus warmup progress")
		}
	}

	// Save syllabi to database
	if len(newSyllabi) > 0 {
		if err := db.SaveSyllabusBatch(ctx, newSyllabi); err != nil {
			log.WithError(err).Error("Failed to save syllabi batch")
			return fmt.Errorf("failed to save syllabi: %w", err)
		}
	}

	// Rebuild BM25 index from database (includes all syllabi with full content)
	// This is done after saving to ensure database is the source of truth
	if bm25Index != nil {
		if err := bm25Index.RebuildFromDB(ctx, db); err != nil {
			log.WithError(err).Warn("Failed to rebuild BM25 index")
			// Don't fail the whole warmup for index errors
		}
	}

	stats.Syllabi.Add(int64(len(newSyllabi)))

	log.WithField("new", updatedCount).
		WithField("skipped", skippedCount).
		WithField("errors", errorCount).
		WithField("total_indexed", len(newSyllabi)).
		Info("Syllabus module warmup complete")

	return nil
}
