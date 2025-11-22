package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// CLI flags
var (
	resetFlag   = flag.Bool("reset", false, "Delete all cache data before warmup")
	modulesFlag = flag.String("modules", "id,contact,course", "Comma-separated list of modules to warmup (id,contact,course)")
	workersFlag = flag.Int("workers", 0, "Worker pool size (0 = use config default)")
)

// Module statistics
type moduleStats struct {
	students int64
	contacts int64
	courses  int64
}

func main() {
	// Parse command-line flags
	flag.Parse()

	// Load configuration for warmup mode (LINE credentials not required)
	cfg, err := config.LoadForMode(config.WarmupMode)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.LogLevel)
	log.Info("Starting warmup tool")

	// Connect to database with configured TTL
	db, err := storage.New(cfg.SQLitePath, cfg.CacheTTL)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}
	defer func() { _ = db.Close() }()
	log.WithField("path", cfg.SQLitePath).
		WithField("cache_ttl", cfg.CacheTTL).
		Info("Database connected")

	// Handle reset flag
	if *resetFlag {
		log.Warn("Resetting cache data...")
		if err := resetCache(db); err != nil {
			log.WithError(err).Fatal("Failed to reset cache")
		}
		log.Info("Cache reset complete")
	}

	// Parse modules
	moduleList := parseModules(*modulesFlag)
	if len(moduleList) == 0 {
		log.Info("No modules specified, exiting")
		fmt.Println("⏭️  No modules to warmup, skipping")
		return
	}
	log.WithField("modules", moduleList).Info("Modules to warmup")

	// Determine worker count
	workers := *workersFlag
	if workers <= 0 {
		workers = cfg.ScraperWorkers
	}
	log.WithField("workers", workers).Info("Worker pool size")

	// Create scraper client
	scraperClient := scraper.NewClient(
		cfg.ScraperTimeout,
		workers,
		cfg.ScraperMinDelay,
		cfg.ScraperMaxDelay,
		cfg.ScraperMaxRetries,
	)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.WarmupTimeout)
	defer cancel()

	// Track statistics
	stats := &moduleStats{}

	// Warmup sticker module first (always runs)
	if err := warmupStickerModule(ctx, db, scraperClient, log); err != nil {
		log.WithError(err).Error("Sticker module warmup failed")
		// Continue with other modules
	}

	// Execute warmup for each module
	startTime := time.Now()
	var hasError bool
	for _, module := range moduleList {
		switch module {
		case "id":
			if err := warmupIDModule(ctx, db, scraperClient, log, stats, workers); err != nil {
				log.WithError(err).Error("ID module warmup failed")
				hasError = true
			}
		case "contact":
			if err := warmupContactModule(ctx, db, scraperClient, log, stats); err != nil {
				log.WithError(err).Error("Contact module warmup failed")
				hasError = true
			}
		case "course":
			if err := warmupCourseModule(ctx, db, scraperClient, log, stats); err != nil {
				log.WithError(err).Error("Course module warmup failed")
				hasError = true
			}
		default:
			log.WithField("module", module).Warn("Unknown module, skipping")
		}
	}
	duration := time.Since(startTime)

	// Print final summary
	if hasError {
		log.WithField("duration", duration).Error("Warmup completed with errors")
		fmt.Fprintf(os.Stderr, "\n❌ Warmup completed with errors: %d students, %d contacts, %d courses cached\n",
			atomic.LoadInt64(&stats.students),
			atomic.LoadInt64(&stats.contacts),
			atomic.LoadInt64(&stats.courses))
		fmt.Fprintf(os.Stderr, "Total time: %v\n", duration.Round(time.Second))
		os.Exit(1)
	} else {
		log.WithField("duration", duration).Info("Warmup complete")
		fmt.Printf("\n✅ Warmup complete: %d students, %d contacts, %d courses cached\n",
			atomic.LoadInt64(&stats.students),
			atomic.LoadInt64(&stats.contacts),
			atomic.LoadInt64(&stats.courses))
		fmt.Printf("Total time: %v\n", duration.Round(time.Second))
	}
}

// parseModules parses comma-separated module list
func parseModules(modules string) []string {
	parts := strings.Split(modules, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		module := strings.TrimSpace(strings.ToLower(part))
		if module != "" {
			result = append(result, module)
		}
	}
	return result
}

// resetCache deletes all cache data
func resetCache(db *storage.DB) error {
	// Delete all students
	if _, err := db.Conn().Exec("DELETE FROM students"); err != nil {
		return fmt.Errorf("failed to delete students: %w", err)
	}

	// Delete all contacts
	if _, err := db.Conn().Exec("DELETE FROM contacts"); err != nil {
		return fmt.Errorf("failed to delete contacts: %w", err)
	}

	// Delete all courses
	if _, err := db.Conn().Exec("DELETE FROM courses"); err != nil {
		return fmt.Errorf("failed to delete courses: %w", err)
	}

	// Delete all stickers
	if _, err := db.Conn().Exec("DELETE FROM stickers"); err != nil {
		return fmt.Errorf("failed to delete stickers: %w", err)
	}

	return nil
}

// warmupIDModule warms up the ID module cache
func warmupIDModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *moduleStats, workers int) error {
	log.Info("Scraping ID module...")

	// Years to scrape (110-113)
	years := []int{110, 111, 112, 113}

	// Get all department codes
	deptCodes := make([]string, 0)
	for _, code := range ntpu.DepartmentCodes {
		deptCodes = append(deptCodes, code)
	}

	// Calculate total tasks
	totalTasks := len(years) * len(deptCodes)
	log.WithField("total_tasks", totalTasks).Info("ID module tasks")

	// Create worker pool
	type task struct {
		year     int
		deptCode string
	}

	tasks := make(chan task, totalTasks)
	var wg sync.WaitGroup
	var completed int64
	var errors int64

	// Start workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for t := range tasks {
				// Check context
				if ctx.Err() != nil {
					return
				}

				// Scrape students
				students, err := ntpu.ScrapeStudentsByYear(ctx, client, t.year, t.deptCode)
				if err != nil {
					log.WithError(err).
						WithField("year", t.year).
						WithField("dept", t.deptCode).
						Error("Failed to scrape students")
					atomic.AddInt64(&errors, 1)
					continue
				}

				// Save to database
				for _, student := range students {
					if err := db.SaveStudent(student); err != nil {
						log.WithError(err).
							WithField("student_id", student.ID).
							Warn("Failed to save student")
						atomic.AddInt64(&errors, 1)
					} else {
						atomic.AddInt64(&stats.students, 1)
					}
				}

				// Update progress
				current := atomic.AddInt64(&completed, 1)
				if current%5 == 0 || current == int64(totalTasks) {
					log.WithField("progress", fmt.Sprintf("%d/%d", current, totalTasks)).
						Info("ID module progress")
				}
			}
		}(i)
	}

	// Send tasks
	for _, year := range years {
		for _, deptCode := range deptCodes {
			tasks <- task{year: year, deptCode: deptCode}
		}
	}
	close(tasks)

	// Wait for completion
	wg.Wait()

	if errors > 0 {
		log.WithField("errors", errors).Warn("ID module completed with errors")
	} else {
		log.Info("ID module complete")
	}

	fmt.Printf("✓ ID module: %d students cached\n", atomic.LoadInt64(&stats.students))
	return nil
}

// warmupContactModule warms up the contact module cache
func warmupContactModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *moduleStats) error {
	log.Info("Scraping Contact module...")

	var contactCount int64

	// Check context
	if ctx.Err() != nil {
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	}

	// Scrape administrative contacts
	log.Info("Scraping administrative contacts...")
	adminContacts, err := ntpu.ScrapeAdministrativeContacts(ctx, client)
	if err != nil {
		log.WithError(err).Error("Failed to scrape administrative contacts")
		return fmt.Errorf("failed to scrape administrative contacts: %w", err)
	}

	for _, contact := range adminContacts {
		if err := db.SaveContact(contact); err != nil {
			log.WithError(err).
				WithField("contact", contact.Name).
				Warn("Failed to save contact")
		} else {
			contactCount++
		}
	}
	log.WithField("count", len(adminContacts)).Info("Administrative contacts scraped")

	// Check context again
	if ctx.Err() != nil {
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	}

	// Scrape academic contacts
	log.Info("Scraping academic contacts...")
	academicContacts, err := ntpu.ScrapeAcademicContacts(ctx, client)
	if err != nil {
		log.WithError(err).Error("Failed to scrape academic contacts")
		return fmt.Errorf("failed to scrape academic contacts: %w", err)
	}

	for _, contact := range academicContacts {
		if err := db.SaveContact(contact); err != nil {
			log.WithError(err).
				WithField("contact", contact.Name).
				Warn("Failed to save contact")
		} else {
			contactCount++
		}
	}
	log.WithField("count", len(academicContacts)).Info("Academic contacts scraped")

	atomic.AddInt64(&stats.contacts, contactCount)
	log.Info("Contact module complete")
	fmt.Printf("✓ Contact module: %d contacts cached\n", contactCount)
	return nil
}

// warmupStickerModule warms up the sticker cache by fetching from web sources
func warmupStickerModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger) error {
	log.Info("Fetching stickers from web sources...")

	// Create sticker manager
	manager := sticker.NewManager(db, client, log)

	// Fetch and save stickers (this will scrape Spy Family + Ichigo Production websites)
	if err := manager.FetchAndSaveStickers(ctx); err != nil {
		return fmt.Errorf("failed to fetch stickers: %w", err)
	}

	// Query final count
	dbStickers, err := db.GetAllStickers()
	if err != nil {
		return fmt.Errorf("failed to query stickers: %w", err)
	}

	log.WithField("count", len(dbStickers)).Info("Sticker module complete")
	fmt.Printf("✓ Sticker module: %d stickers cached\n", len(dbStickers))
	return nil
}

// warmupCourseModule warms up the course module cache
func warmupCourseModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *moduleStats) error {
	log.Info("Scraping Course module...")

	// Scrape ALL courses (all education codes) for recent terms
	// This ensures that title/teacher search will work without cache miss
	type term struct {
		year int
		term int
	}

	// Scrape current and recent terms: 113-1, 113-2, 112-2
	terms := []term{
		{year: 113, term: 1},
		{year: 113, term: 2},
		{year: 112, term: 2},
	}

	totalCourses := int64(0)

	for _, t := range terms {
		// Check context before each term
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		log.WithField("year", t.year).
			WithField("term", t.term).
			Info("Scraping ALL courses for term (this may take a while)...")

		// Scrape all courses (empty title = scrape all education codes)
		courses, err := ntpu.ScrapeCourses(ctx, client, t.year, t.term, "")
		if err != nil {
			log.WithError(err).
				WithField("year", t.year).
				WithField("term", t.term).
				Error("Failed to scrape courses")
			continue
		}

		// Save courses
		var saveErrors int
		for _, course := range courses {
			if err := db.SaveCourse(course); err != nil {
				log.WithError(err).
					WithField("course_uid", course.UID).
					Warn("Failed to save course")
				saveErrors++
			} else {
				totalCourses++
			}
		}

		log.WithField("year", t.year).
			WithField("term", t.term).
			WithField("scraped", len(courses)).
			WithField("saved", len(courses)-saveErrors).
			WithField("errors", saveErrors).
			Info("Courses scraped for term")
	}

	atomic.AddInt64(&stats.courses, totalCourses)
	log.Info("Course module complete")
	fmt.Printf("✓ Course module: %d courses cached\n", totalCourses)
	return nil
}
