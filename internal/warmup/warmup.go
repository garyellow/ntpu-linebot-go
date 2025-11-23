package warmup

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// Stats tracks cache warming statistics
type Stats struct {
	Students int64
	Contacts int64
	Courses  int64
	Stickers int64
}

// Options configures cache warming behavior
type Options struct {
	Modules []string      // Modules to warm (id, contact, course, sticker)
	Workers int           // Worker pool size for ID module
	Timeout time.Duration // Overall timeout
	Reset   bool          // Whether to reset cache before warming
}

// Run executes cache warming with the given options
func Run(ctx context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, opts Options) (*Stats, error) {
	stats := &Stats{}
	startTime := time.Now()

	// Create context with timeout only if parent context has no deadline
	if opts.Timeout > 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
			defer cancel()
		}
	}

	// Reset cache if requested
	if opts.Reset {
		log.Warn("Resetting cache data...")
		if err := resetCache(db); err != nil {
			return nil, fmt.Errorf("failed to reset cache: %w", err)
		}
		log.Info("Cache reset complete")
	}

	// Warm modules in order
	for _, module := range opts.Modules {
		select {
		case <-ctx.Done():
			return stats, fmt.Errorf("warmup cancelled: %w", ctx.Err())
		default:
		}

		switch module {
		case "id":
			if err := warmupIDModule(ctx, db, client, log, stats, opts.Workers); err != nil {
				log.WithError(err).Error("ID module warmup failed")
			}
		case "contact":
			if err := warmupContactModule(ctx, db, client, log, stats); err != nil {
				log.WithError(err).Error("Contact module warmup failed")
			}
		case "course":
			if err := warmupCourseModule(ctx, db, client, log, stats); err != nil {
				log.WithError(err).Error("Course module warmup failed")
			}
		case "sticker":
			if err := warmupStickerModule(ctx, stickerMgr, log, stats); err != nil {
				log.WithError(err).Error("Sticker module warmup failed")
			}
		default:
			log.WithField("module", module).Warn("Unknown module, skipping")
		}
	}

	duration := time.Since(startTime)
	log.WithField("duration", duration).
		WithField("students", stats.Students).
		WithField("contacts", stats.Contacts).
		WithField("courses", stats.Courses).
		WithField("stickers", stats.Stickers).
		Info("Cache warming complete")

	return stats, nil
}

// RunInBackground executes cache warming asynchronously
// Returns immediately without blocking. Logs progress to the provided logger.
func RunInBackground(ctx context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, opts Options) {
	go func() {
		log.WithField("modules", opts.Modules).
			WithField("workers", opts.Workers).
			Info("Starting background cache warming")

		stats, err := Run(ctx, db, client, stickerMgr, log, opts)
		if err != nil {
			log.WithError(err).Warn("Background cache warming finished with errors")
		} else {
			log.WithField("students", stats.Students).
				WithField("contacts", stats.Contacts).
				WithField("courses", stats.Courses).
				WithField("stickers", stats.Stickers).
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
func warmupIDModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats, workers int) error {
	// Match Python version: from min(112, current_year) down to 101
	years := []int{112, 111, 110, 109, 108, 107, 106, 105, 104, 103, 102, 101}
	departments := []string{
		"A1", "A2", "A3", "A4", "A5", "A6", "A7", "A8", "A9", "B1",
		"B2", "B3", "B4", "B5", "B6", "C1", "C2", "C3", "C4", "C5", "C6", "C7",
	}

	totalTasks := len(years) * len(departments)
	log.WithField("tasks", totalTasks).
		WithField("workers", workers).
		Info("Starting ID module warmup")

	// Create task channel
	type task struct {
		year int
		dept string
	}
	tasks := make(chan task, totalTasks)
	for _, year := range years {
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

				// Save to database
				savedCount := 0
				for _, s := range students {
					if err := db.SaveStudent(s); err != nil {
						log.WithError(err).
							WithField("id", s.ID).
							Warn("Failed to save student")
					} else {
						savedCount++
					}
				}

				atomic.AddInt64(&stats.Students, int64(savedCount))
				count := completed.Add(1)

				if count%10 == 0 || count == int64(totalTasks) {
					log.WithField("progress", fmt.Sprintf("%d/%d", count, totalTasks)).
						WithField("students", atomic.LoadInt64(&stats.Students)).
						Info("ID module progress")
				}
			}
		}(i)
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		return fmt.Errorf("completed with %d errors", errorCount.Load())
	}
	return nil
}

// warmupContactModule warms contact cache
func warmupContactModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats) error {
	log.Info("Starting contact module warmup")

	// Scrape administrative contacts
	adminContacts, err := ntpu.ScrapeAdministrativeContacts(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to scrape administrative contacts: %w", err)
	}

	adminSavedCount := 0
	for _, c := range adminContacts {
		if err := db.SaveContact(c); err != nil {
			log.WithError(err).WithField("uid", c.UID).Warn("Failed to save administrative contact")
		} else {
			adminSavedCount++
		}
	}
	atomic.AddInt64(&stats.Contacts, int64(adminSavedCount))
	log.WithField("count", adminSavedCount).Info("Administrative contacts cached")

	// Scrape academic contacts
	academicContacts, err := ntpu.ScrapeAcademicContacts(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to scrape academic contacts: %w", err)
	}

	academicSavedCount := 0
	for _, c := range academicContacts {
		if err := db.SaveContact(c); err != nil {
			log.WithError(err).WithField("uid", c.UID).Warn("Failed to save academic contact")
		} else {
			academicSavedCount++
		}
	}
	atomic.AddInt64(&stats.Contacts, int64(academicSavedCount))
	log.WithField("count", academicSavedCount).Info("Academic contacts cached")

	return nil
}

// warmupCourseModule warms course cache
func warmupCourseModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats) error {
	log.Info("Starting course module warmup")

	currentYear := time.Now().Year() - 1911
	terms := []struct {
		year int
		term int
	}{
		{currentYear, 1}, {currentYear, 2}, {currentYear - 1, 2},
	}
	educationCodes := []string{"UG", "PG", "MD", "ON"}

	for _, t := range terms {
		for _, code := range educationCodes {
			select {
			case <-ctx.Done():
				return fmt.Errorf("course module cancelled: %w", ctx.Err())
			default:
			}

			courses, err := ntpu.ScrapeCourses(ctx, client, t.year, t.term, code)
			if err != nil {
				log.WithError(err).
					WithField("year", t.year).
					WithField("term", t.term).
					WithField("education_code", code).
					Warn("Failed to scrape courses")
				continue
			}

			savedCount := 0
			for _, c := range courses {
				if err := db.SaveCourse(c); err != nil {
					log.WithError(err).WithField("uid", c.UID).Warn("Failed to save course")
				} else {
					savedCount++
				}
			}

			atomic.AddInt64(&stats.Courses, int64(savedCount))
			log.WithField("year", t.year).
				WithField("term", t.term).
				WithField("education_code", code).
				WithField("count", savedCount).
				Info("Courses cached")
		}
	}

	return nil
}

// warmupStickerModule warms sticker cache
func warmupStickerModule(ctx context.Context, stickerMgr *sticker.Manager, log *logger.Logger, stats *Stats) error {
	log.Info("Starting sticker module warmup")

	if err := stickerMgr.LoadStickers(ctx); err != nil {
		return fmt.Errorf("failed to load stickers: %w", err)
	}

	count := stickerMgr.Count()
	atomic.StoreInt64(&stats.Stickers, int64(count))
	log.WithField("count", count).Info("Stickers cached")

	return nil
}
