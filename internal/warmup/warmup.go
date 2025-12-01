// Package warmup provides background cache warming functionality for
// proactively fetching and caching student, course, contact, sticker, and syllabus data.
package warmup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/syllabus"
)

// Stats tracks cache warming statistics
// All fields use atomic operations for concurrent access
type Stats struct {
	Students atomic.Int64
	Contacts atomic.Int64
	Courses  atomic.Int64
	Stickers atomic.Int64
	Syllabi  atomic.Int64
}

// Options configures cache warming behavior
type Options struct {
	Modules  []string         // Modules to warm (id, contact, course, sticker, syllabus)
	Reset    bool             // Whether to reset cache before warming
	Metrics  *metrics.Metrics // Optional metrics recorder
	VectorDB *rag.VectorDB    // Optional vector database for syllabus indexing
}

// Run executes cache warming with the given options
func Run(ctx context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, opts Options) (*Stats, error) {
	stats := &Stats{}
	startTime := time.Now()

	// Reset cache if requested
	if opts.Reset {
		log.Warn("Resetting cache data...")
		if err := resetCache(ctx, db); err != nil {
			return nil, fmt.Errorf("failed to reset cache: %w", err)
		}
		log.Info("Cache reset complete")
	}

	// Separate modules:
	// - Independent modules: can run concurrently (id, contact, sticker)
	// - Course module: runs concurrently but syllabus waits for it
	// - Syllabus module: depends on course data, starts after course completes
	var independentModules []string
	var hasCourse, hasSyllabus bool

	for _, module := range opts.Modules {
		switch module {
		case "course":
			hasCourse = true
		case "syllabus":
			hasSyllabus = true
		default:
			independentModules = append(independentModules, module)
		}
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(opts.Modules))

	// Channel to signal course completion (for syllabus dependency)
	courseDone := make(chan struct{})

	// Start independent modules concurrently
	for _, module := range independentModules {
		select {
		case <-ctx.Done():
			return stats, fmt.Errorf("warmup canceled: %w", ctx.Err())
		default:
		}

		wg.Add(1)
		moduleName := module // Capture for goroutine
		go func() {
			defer wg.Done()

			switch moduleName {
			case "id":
				if err := warmupIDModule(ctx, db, client, log, stats, opts.Metrics); err != nil {
					log.WithError(err).Error("ID module warmup failed")
					errChan <- fmt.Errorf("id module: %w", err)
				}
			case "contact":
				if err := warmupContactModule(ctx, db, client, log, stats, opts.Metrics); err != nil {
					log.WithError(err).Error("Contact module warmup failed")
					errChan <- fmt.Errorf("contact module: %w", err)
				}
			case "sticker":
				if err := warmupStickerModule(ctx, stickerMgr, log, stats, opts.Metrics); err != nil {
					log.WithError(err).Error("Sticker module warmup failed")
					errChan <- fmt.Errorf("sticker module: %w", err)
				}
			default:
				log.WithField("module", moduleName).Warn("Unknown module, skipping")
			}
		}()
	}

	// Start course module (syllabus will wait for this)
	if hasCourse {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(courseDone) // Signal course completion

			if err := warmupCourseModule(ctx, db, client, log, stats, opts.Metrics); err != nil {
				log.WithError(err).Error("Course module warmup failed")
				errChan <- fmt.Errorf("course module: %w", err)
			}
		}()
	} else {
		// No course module requested, close channel immediately
		close(courseDone)
	}

	// Start syllabus module (waits for course to complete first)
	if hasSyllabus {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Wait for course module to complete (syllabus needs course data)
			if hasCourse {
				log.Debug("Syllabus waiting for course module to complete")
			}
			select {
			case <-ctx.Done():
				errChan <- fmt.Errorf("syllabus canceled while waiting for course: %w", ctx.Err())
				return
			case <-courseDone:
				// Course completed (or wasn't requested), proceed with syllabus
			}

			if opts.VectorDB == nil {
				log.Info("Syllabus module skipped: VectorDB not configured")
				return
			}

			if err := warmupSyllabusModule(ctx, db, client, opts.VectorDB, log, stats, opts.Metrics); err != nil {
				log.WithError(err).Error("Syllabus module warmup failed")
				errChan <- fmt.Errorf("syllabus module: %w", err)
			}
		}()
	}

	// Wait for all modules to complete
	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	duration := time.Since(startTime)
	log.WithField("duration", duration).
		WithField("students", stats.Students.Load()).
		WithField("contacts", stats.Contacts.Load()).
		WithField("courses", stats.Courses.Load()).
		WithField("stickers", stats.Stickers.Load()).
		WithField("syllabi", stats.Syllabi.Load()).
		Info("Cache warming complete")

	// Record warmup metrics if available
	if opts.Metrics != nil {
		opts.Metrics.RecordWarmupDuration(duration.Seconds())
	}

	// Return combined error if any modules failed
	// Note: We still return stats to allow partial success usage
	if len(errs) > 0 {
		log.WithField("error_count", len(errs)).Warn("Some modules failed during warmup")
		return stats, errors.Join(errs...)
	}

	return stats, nil
}

// RunInBackground executes cache warming asynchronously
// Returns immediately without blocking. Logs progress to the provided logger.
// Uses context.Background() for independent operation that runs until completion.
//
//nolint:contextcheck // Intentionally using context.Background() for independent background operation
func RunInBackground(_ context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, opts Options) {
	go func() {
		log.WithField("modules", opts.Modules).
			Info("Starting background cache warming")

		stats, err := Run(context.Background(), db, client, stickerMgr, log, opts)
		if err != nil {
			log.WithError(err).Warn("Background cache warming finished with errors")
		} else {
			log.WithField("students", stats.Students.Load()).
				WithField("contacts", stats.Contacts.Load()).
				WithField("courses", stats.Courses.Load()).
				WithField("stickers", stats.Stickers.Load()).
				WithField("syllabi", stats.Syllabi.Load()).
				Info("Background cache warming completed successfully")
		}
	}()
}

// ParseModules converts comma-separated string to module list
func ParseModules(modules string) []string {
	if modules == "" {
		return []string{}
	}

	var result []string
	for _, m := range strings.Split(modules, ",") {
		m = strings.TrimSpace(m)
		if m != "" {
			result = append(result, m)
		}
	}
	return result
}

// resetCache deletes all cached data
func resetCache(ctx context.Context, db *storage.DB) error {
	validTables := map[string]bool{
		"students": true,
		"contacts": true,
		"courses":  true,
		"stickers": true,
	}

	tables := []string{"students", "contacts", "courses", "stickers"}
	for _, table := range tables {
		if !validTables[table] {
			return fmt.Errorf("invalid table name: %s", table)
		}
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

// warmupIDModule warms student ID cache
// Executes tasks sequentially (one at a time) to avoid overwhelming the server
func warmupIDModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats, m *metrics.Metrics) error {
	// Warmup range: 101-112 (LMS 2.0 已無 113+ 資料)
	currentYear := time.Now().Year() - 1911
	fromYear := min(112, currentYear)

	// All department codes
	departments := []string{
		"71", "712", "714", "716", "72", "73", "742", "744",
		"75", "76", "77", "78", "79", "80", "81", "82", "83", "84", "85", "86", "87",
	}

	totalTasks := (fromYear - 100) * len(departments)
	log.WithField("tasks", totalTasks).Info("Starting ID module warmup (sequential)")

	var completed int
	var errorCount int

	// Execute tasks sequentially (one at a time)
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
				errorCount++
				continue
			}

			stats.Students.Add(int64(len(students)))
			completed++

			if completed%10 == 0 || completed == totalTasks {
				log.WithField("progress", fmt.Sprintf("%d/%d", completed, totalTasks)).
					WithField("students", stats.Students.Load()).
					Info("ID module progress")
			}
		}
	}

	// Record metrics
	if m != nil {
		successCount := completed - errorCount
		if successCount > 0 {
			m.WarmupTasksTotal.WithLabelValues("id", "success").Add(float64(successCount))
		}
		if errorCount > 0 {
			m.WarmupTasksTotal.WithLabelValues("id", "error").Add(float64(errorCount))
		}
	}

	if errorCount > 0 {
		return fmt.Errorf("completed with %d errors", errorCount)
	}
	return nil
}

// warmupContactModule warms contact cache
// Returns error only if BOTH administrative and academic contact scraping fail.
// Allows partial success: if one source succeeds, the function returns nil.
// Use logs to identify which source failed when partial success occurs.
func warmupContactModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats, m *metrics.Metrics) error {
	log.Info("Starting contact module warmup")

	var errs []error

	// Scrape administrative contacts
	adminContacts, err := ntpu.ScrapeAdministrativeContacts(ctx, client)
	if err != nil {
		log.WithError(err).Warn("Failed to scrape administrative contacts, continuing anyway")
		errs = append(errs, fmt.Errorf("administrative contacts: %w", err))
	} else {
		// Save using batch operation to reduce lock contention
		if err := db.SaveContactsBatch(ctx, adminContacts); err != nil {
			log.WithError(err).Warn("Failed to save administrative contacts batch")
			errs = append(errs, fmt.Errorf("save administrative contacts: %w", err))
		} else {
			stats.Contacts.Add(int64(len(adminContacts)))
			log.WithField("count", len(adminContacts)).Info("Administrative contacts cached")
		}
	}

	// Scrape academic contacts
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

	// Record metrics
	if m != nil {
		if len(errs) < 2 {
			// At least one source succeeded
			m.RecordWarmupTask("contact", "success")
		}
		if len(errs) > 0 {
			// At least one source failed
			m.RecordWarmupTask("contact", "error")
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

// warmupCourseModule warms course cache
// Uses ScrapeCoursesByYear to fetch all courses for a year in one batch (no qTerm parameter)
// Makes 4 HTTP requests per year (U/M/N/P education codes)
// Only warms up 2 years (current + previous) for regular course queries
// Historical courses (older than 2 years) use separate historical_courses table with on-demand scraping
func warmupCourseModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats, m *metrics.Metrics) error {
	log.Info("Starting course module warmup")

	currentYear := time.Now().Year() - 1911
	// Load 2 years of course data (current + previous year)
	// Historical courses (older than 2 years) are handled by separate historical_courses table
	// with on-demand scraping via "課程 {year} {keyword}" syntax
	var years []int
	for year := currentYear; year > currentYear-2; year-- {
		years = append(years, year)
	}

	log.WithField("years", years).
		WithField("total_years", len(years)).
		Info("Course warmup: fetching recent courses by year (no term filter)")

	// Scrape all courses for each year (both semesters in one request batch)
	for _, year := range years {
		select {
		case <-ctx.Done():
			return fmt.Errorf("course module canceled: %w", ctx.Err())
		default:
		}

		// Fetch ALL courses for this year (both semesters) using ScrapeCoursesByYear
		// This makes 4 HTTP requests (one per education code: U/M/N/P)
		courses, err := ntpu.ScrapeCoursesByYear(ctx, client, year)
		if err != nil {
			log.WithError(err).
				WithField("year", year).
				Warn("Failed to scrape courses for year")
			if m != nil {
				m.RecordWarmupTask("course", "error")
			}
			continue
		}

		// Save using batch operation to reduce lock contention
		if err := db.SaveCoursesBatch(ctx, courses); err != nil {
			log.WithError(err).
				WithField("year", year).
				WithField("count", len(courses)).
				Warn("Failed to save courses batch")
			if m != nil {
				m.RecordWarmupTask("course", "error")
			}
			continue
		}

		stats.Courses.Add(int64(len(courses)))
		if m != nil {
			m.RecordWarmupTask("course", "success")
		}
		log.WithField("year", year).
			WithField("count", len(courses)).
			Info("Courses cached for year")
	}

	return nil
}

// warmupStickerModule warms sticker cache
func warmupStickerModule(ctx context.Context, stickerMgr *sticker.Manager, log *logger.Logger, stats *Stats, m *metrics.Metrics) error {
	log.Info("Starting sticker module warmup")

	if err := stickerMgr.LoadStickers(ctx); err != nil {
		if m != nil {
			m.RecordWarmupTask("sticker", "error")
		}
		return fmt.Errorf("failed to load stickers: %w", err)
	}

	count := stickerMgr.Count()
	stats.Stickers.Store(int64(count))
	if m != nil {
		m.RecordWarmupTask("sticker", "success")
	}
	log.WithField("count", count).Info("Stickers cached")

	return nil
}

// warmupSyllabusModule warms syllabus cache and vector store
// Fetches syllabus content for all courses in cache, using content hash for incremental updates
// Only processes courses that have changed since last warmup
func warmupSyllabusModule(ctx context.Context, db *storage.DB, client *scraper.Client, vectorDB *rag.VectorDB, log *logger.Logger, stats *Stats, m *metrics.Metrics) error {
	log.Info("Starting syllabus module warmup")

	// Get all courses from recent semesters
	courses, err := db.GetCoursesByRecentSemesters(ctx)
	if err != nil {
		if m != nil {
			m.RecordWarmupTask("syllabus", "error")
		}
		return fmt.Errorf("failed to get courses: %w", err)
	}

	if len(courses) == 0 {
		log.Info("No courses found for syllabus warmup")
		return nil
	}

	log.WithField("total_courses", len(courses)).Info("Processing syllabi for courses")

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

		// Compute content hash from all fields for change detection
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

		// Create syllabus record with separate fields
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

		// Log progress every 100 courses
		if i > 0 && i%100 == 0 {
			log.WithField("progress", fmt.Sprintf("%d/%d", i, len(courses))).
				WithField("updated", updatedCount).
				WithField("skipped", skippedCount).
				Info("Syllabus warmup progress")
		}
	}

	// Save syllabi to database
	if len(newSyllabi) > 0 {
		if err := db.SaveSyllabusBatch(ctx, newSyllabi); err != nil {
			log.WithError(err).Error("Failed to save syllabi batch")
			if m != nil {
				m.RecordWarmupTask("syllabus", "error")
			}
			return fmt.Errorf("failed to save syllabi: %w", err)
		}

		// Add to vector store
		if vectorDB != nil {
			if err := vectorDB.AddSyllabi(ctx, newSyllabi); err != nil {
				log.WithError(err).Warn("Failed to add syllabi to vector store")
				// Don't fail the whole warmup for vector store errors
			}
		}
	}

	stats.Syllabi.Add(int64(len(newSyllabi)))
	if m != nil {
		m.RecordWarmupTask("syllabus", "success")
	}

	log.WithField("new", updatedCount).
		WithField("skipped", skippedCount).
		WithField("errors", errorCount).
		WithField("total_indexed", len(newSyllabi)).
		Info("Syllabus module warmup complete")

	return nil
}
