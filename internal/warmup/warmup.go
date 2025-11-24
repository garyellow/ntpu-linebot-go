package warmup

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// Stats tracks cache warming statistics
// All fields use atomic operations for concurrent access
type Stats struct {
	Students atomic.Int64
	Contacts atomic.Int64
	Courses  atomic.Int64
	Stickers atomic.Int64
}

// Options configures cache warming behavior
type Options struct {
	Modules []string         // Modules to warm (id, contact, course, sticker)
	Workers int              // Worker pool size for ID module
	Timeout time.Duration    // Overall timeout
	Reset   bool             // Whether to reset cache before warming
	Metrics *metrics.Metrics // Optional metrics recorder
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Run executes cache warming with the given options
func Run(ctx context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, opts Options) (*Stats, error) {
	stats := &Stats{}
	startTime := time.Now()

	// Always create a cancel function for cleanup if timeout is set
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		} else {
			ctx, cancel = context.WithCancel(ctx)
		}
		defer cancel()
	}

	// Reset cache if requested
	if opts.Reset {
		log.Warn("Resetting cache data...")
		if err := resetCache(db); err != nil {
			return nil, fmt.Errorf("failed to reset cache: %w", err)
		}
		log.Info("Cache reset complete")
	}

	// Warm modules concurrently (no dependencies between modules)
	var wg sync.WaitGroup
	errChan := make(chan error, len(opts.Modules))

	for _, module := range opts.Modules {
		select {
		case <-ctx.Done():
			return stats, fmt.Errorf("warmup cancelled: %w", ctx.Err())
		default:
		}

		wg.Add(1)
		moduleName := module // Capture for goroutine
		go func() {
			defer wg.Done()

			switch moduleName {
			case "id":
				if err := warmupIDModule(ctx, db, client, log, stats, opts.Workers, opts.Metrics); err != nil {
					log.WithError(err).Error("ID module warmup failed")
					errChan <- fmt.Errorf("id module: %w", err)
				}
			case "contact":
				if err := warmupContactModule(ctx, db, client, log, stats, opts.Metrics); err != nil {
					log.WithError(err).Error("Contact module warmup failed")
					errChan <- fmt.Errorf("contact module: %w", err)
				}
			case "course":
				if err := warmupCourseModule(ctx, db, client, log, stats, opts.Metrics); err != nil {
					log.WithError(err).Error("Course module warmup failed")
					errChan <- fmt.Errorf("course module: %w", err)
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

	// Wait for all modules to complete
	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		log.WithField("error_count", len(errs)).Warn("Some modules failed during warmup")
	}

	duration := time.Since(startTime)
	log.WithField("duration", duration).
		WithField("students", stats.Students.Load()).
		WithField("contacts", stats.Contacts.Load()).
		WithField("courses", stats.Courses.Load()).
		WithField("stickers", stats.Stickers.Load()).
		Info("Cache warming complete")

	// Record warmup metrics if available
	if opts.Metrics != nil {
		opts.Metrics.RecordWarmupDuration(duration.Seconds())
	}

	return stats, nil
}

// RunInBackground executes cache warming asynchronously
// Returns immediately without blocking. Logs progress to the provided logger.
// Creates an independent context with timeout to prevent goroutine leaks on server shutdown.
func RunInBackground(ctx context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, opts Options) {
	// Create independent context with timeout for warmup
	// This prevents the goroutine from leaking if server context is cancelled
	warmupCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)

	go func() {
		defer cancel() // Ensure cleanup

		log.WithField("modules", opts.Modules).
			WithField("workers", opts.Workers).
			Info("Starting background cache warming")

		stats, err := Run(warmupCtx, db, client, stickerMgr, log, opts)
		if err != nil {
			log.WithError(err).Warn("Background cache warming finished with errors")
		} else {
			log.WithField("students", stats.Students.Load()).
				WithField("contacts", stats.Contacts.Load()).
				WithField("courses", stats.Courses.Load()).
				WithField("stickers", stats.Stickers.Load()).
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
func resetCache(db *storage.DB) error {
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
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to delete from %s: %w", table, err)
		}
	}
	// Run VACUUM to reclaim space
	if _, err := db.Exec("VACUUM"); err != nil {
		return fmt.Errorf("failed to vacuum: %w", err)
	}
	return nil
}

// warmupIDModule warms student ID cache
func warmupIDModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats, workers int, m *metrics.Metrics) error {
	// Match Python version: range(min(112, current_year), 100, -1)
	currentYear := time.Now().Year() - 1911
	fromYear := min(112, currentYear)

	// Department codes from Python's DEPARTMENT_CODE.values()
	departments := []string{
		"71", "712", "714", "716", "72", "73", "742", "744",
		"75", "76", "77", "78", "79", "80", "81", "82", "83", "84", "85", "86", "87",
	}

	totalTasks := (fromYear - 100) * len(departments)
	log.WithField("tasks", totalTasks).
		WithField("workers", workers).
		Info("Starting ID module warmup")

	// Create task channel
	type task struct {
		year int
		dept string
	}
	tasks := make(chan task, totalTasks)
	for year := fromYear; year > 100; year-- {
		for _, dept := range departments {
			tasks <- task{year, dept}
		}
	}
	close(tasks)

	// Worker pool
	var wg sync.WaitGroup
	var completed atomic.Int64
	var errorCount atomic.Int64

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for t := range tasks {
				select {
				case <-ctx.Done():
					return
				default:
				}

				students, err := ntpu.ScrapeStudentsByYear(ctx, client, t.year, t.dept)
				if err != nil {
					log.WithError(err).
						WithField("year", t.year).
						WithField("dept", t.dept).
						Warn("Failed to scrape students")
					errorCount.Add(1)
					continue
				}

				// Save to database using batch operation to reduce lock contention
				if err := db.SaveStudentsBatch(students); err != nil {
					log.WithError(err).
						WithField("year", t.year).
						WithField("dept", t.dept).
						WithField("count", len(students)).
						Warn("Failed to save student batch")
					errorCount.Add(1)
					continue
				}

				stats.Students.Add(int64(len(students)))
				count := completed.Add(1)

				if count%10 == 0 || count == int64(totalTasks) {
					log.WithField("progress", fmt.Sprintf("%d/%d", count, totalTasks)).
						WithField("students", stats.Students.Load()).
						Info("ID module progress")
				}
			}
		}(i)
	}

	wg.Wait()

	// Record metrics
	if m != nil {
		successCount := completed.Load() - errorCount.Load()
		if successCount > 0 {
			for i := int64(0); i < successCount; i++ {
				m.RecordWarmupTask("id", "success")
			}
		}
		if errorCount.Load() > 0 {
			for i := int64(0); i < errorCount.Load(); i++ {
				m.RecordWarmupTask("id", "error")
			}
		}
	}

	if errorCount.Load() > 0 {
		return fmt.Errorf("completed with %d errors", errorCount.Load())
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
		if err := db.SaveContactsBatch(adminContacts); err != nil {
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
		if err := db.SaveContactsBatch(academicContacts); err != nil {
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
		return fmt.Errorf("both contact sources failed - administrative: %v, academic: %v", errs[0], errs[1])
	}

	// Log partial success details
	if len(errs) == 1 {
		log.WithField("failed_source", errs[0]).Info("Contact module completed with partial success")
	}

	return nil
}

// warmupCourseModule warms course cache
func warmupCourseModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats, m *metrics.Metrics) error {
	log.Info("Starting course module warmup")

	currentYear := time.Now().Year() - 1911
	// Course terms to warm: Load 5 years of course data (matching Python version)
	// This includes historical course data for queries about past courses
	var terms []struct {
		year int
		term int
	}
	// Generate terms for current year and previous 4 years (5 years total)
	for year := currentYear; year > currentYear-5; year-- {
		terms = append(terms, struct {
			year int
			term int
		}{year, 1}) // First semester
		terms = append(terms, struct {
			year int
			term int
		}{year, 2}) // Second semester
	}

	// Scrape all courses for each term (ScrapeCourses with empty title will fetch all education codes)
	for _, t := range terms {
		select {
		case <-ctx.Done():
			return fmt.Errorf("course module cancelled: %w", ctx.Err())
		default:
		}

		// Pass empty string as title to fetch ALL courses (all education codes: U, M, N, P)
		// This matches Python's get_simple_courses_by_year which iterates ALL_EDU_CODE
		courses, err := ntpu.ScrapeCourses(ctx, client, t.year, t.term, "")
		if err != nil {
			log.WithError(err).
				WithField("year", t.year).
				WithField("term", t.term).
				Warn("Failed to scrape courses")
			if m != nil {
				m.RecordWarmupTask("course", "error")
			}
			continue
		}

		// Save using batch operation to reduce lock contention
		if err := db.SaveCoursesBatch(courses); err != nil {
			log.WithError(err).
				WithField("year", t.year).
				WithField("term", t.term).
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
		log.WithField("year", t.year).
			WithField("term", t.term).
			WithField("count", len(courses)).
			Info("Courses cached")
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
