// Package warmup provides background data refresh functionality.
//
// Refresh tasks (interval-based):
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

// Stats tracks data refresh statistics for refresh operations.
// All fields use atomic operations for concurrent access.
// Note: Students are not tracked here as they are only warmed on startup (static data).
type Stats struct {
	Contacts atomic.Int64
	Courses  atomic.Int64
	Programs atomic.Int64
	Syllabi  atomic.Int64
}

// Options configures refresh behavior
type Options struct {
	Reset         bool
	HasLLMKey     bool // Enables syllabus module for smart search
	WarmID        bool // Enables ID module warmup (static data, startup only, not used in recurring refresh)
	Metrics       *metrics.Metrics
	BM25Index     *rag.BM25Index
	SemesterCache *course.SemesterCache // Shared cache to update after refresh
}

// courseProgramMap stores raw program requirements keyed by course UID.
// Populated during course warmup, consumed during syllabus warmup.
// This enables dual-source fusion: accurate program names from syllabus page +
// correct required/elective types from course list page.
type courseProgramMap map[string][]storage.RawProgramReq

// Run executes data refresh: contact, course, program (always), syllabus (if HasLLMKey).
func Run(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, opts Options) (*Stats, error) {
	stats := &Stats{}
	startTime := time.Now()

	if opts.Reset {
		log.Info("Resetting cache data")
		if err := resetCache(ctx, db); err != nil {
			return nil, fmt.Errorf("failed to reset cache: %w", err)
		}
		log.Info("Cache reset completed")
	}

	hasSyllabus := opts.HasLLMKey

	g, ctx := errgroup.WithContext(ctx)

	// Channel to pass raw program requirements from course warmup to syllabus warmup
	// This enables dual-source fusion: accurate names from syllabus + correct types from list
	programMapChan := make(chan courseProgramMap, 1)

	g.Go(func() error {
		if err := warmupContactModule(ctx, db, client, log, stats, opts.Metrics); err != nil {
			log.WithError(err).Error("Contact module warmup failed")
			return fmt.Errorf("contact module: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		defer close(programMapChan)
		programMap, err := warmupCourseModule(ctx, db, client, log, stats, opts.Metrics, opts.SemesterCache)
		if err != nil {
			log.WithError(err).Error("Course module warmup failed")
			return fmt.Errorf("course module: %w", err)
		}
		// Send program map to syllabus warmup
		select {
		case programMapChan <- programMap:
		case <-ctx.Done():
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
			// Wait for course warmup to complete and get program map
			var programMap courseProgramMap
			select {
			case <-ctx.Done():
				return ctx.Err()
			case pm, ok := <-programMapChan:
				if !ok {
					// Channel closed without sending, course warmup may have failed
					return nil
				}
				programMap = pm
			}

			if opts.BM25Index == nil {
				return nil
			}

			if err := warmupSyllabusModule(ctx, db, client, opts.BM25Index, log, stats, opts.Metrics, programMap); err != nil {
				log.WithError(err).Error("Syllabus module warmup failed")
				return fmt.Errorf("syllabus: %w", err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return stats, fmt.Errorf("warmup: %w", err)
	}

	log.WithField("duration_ms", time.Since(startTime).Milliseconds()).
		WithField("contacts", stats.Contacts.Load()).
		WithField("courses", stats.Courses.Load()).
		WithField("programs", stats.Programs.Load()).
		WithField("syllabi", stats.Syllabi.Load()).
		Info("Data refresh completed")

	return stats, nil
}

// resetCache deletes all cached data
func resetCache(ctx context.Context, db *storage.DB) error {
	tables := []string{
		"students",
		"contacts",
		"courses",
		"historical_courses",
		"programs",
		"course_programs",
		"syllabi",
		"stickers",
	}
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
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("failed to checkpoint wal after vacuum: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return fmt.Errorf("failed to optimize: %w", err)
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
// Scrapes undergraduate (prefix 4), master's (prefix 7), and PhD (prefix 8) students.
func warmupIDModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, m *metrics.Metrics) error {
	startTime := time.Now()
	defer func() {
		if m != nil {
			m.RecordJob("refresh", "id", time.Since(startTime).Seconds())
		}
	}()

	// Warmup range: 101-112 (數位學苑 2.0 已無 113+ 完整資料)
	currentYear := time.Now().Year() - 1911
	fromYear := min(config.IDDataYearEnd, currentYear)

	// Define student types with their respective department codes
	studentTypes := []struct {
		prefix string
		depts  []string
		name   string
	}{
		{ntpu.StudentTypeUndergrad, ntpu.UndergradDeptCodes, "學士班"},
		{ntpu.StudentTypeMaster, ntpu.MasterDeptCodes, "碩士班"},
		{ntpu.StudentTypePhD, ntpu.PhDDeptCodes, "博士班"},
	}

	// Calculate total tasks
	var totalTasks int
	for _, st := range studentTypes {
		totalTasks += (fromYear - 100) * len(st.depts)
	}
	log.WithField("tasks", totalTasks).Info("Starting ID module warmup (undergrad + master's + PhD)")

	var completed int
	var errorCount int
	var studentCount int64
	var errs []error

	for _, st := range studentTypes {
		log.WithField("type", st.name).WithField("dept_count", len(st.depts)).Info("Warming up student type")

		for year := fromYear; year > 100; year-- {
			for _, dept := range st.depts {
				select {
				case <-ctx.Done():
					log.WithField("completed", completed).
						WithField("errors", errorCount).
						Warn("ID module warmup canceled")
					return fmt.Errorf("canceled: %w", ctx.Err())
				default:
				}

				students, err := ntpu.ScrapeStudentsByYear(ctx, client, year, dept, st.prefix)
				if err != nil {
					log.WithError(err).
						WithField("year", year).
						WithField("dept", dept).
						WithField("type", st.name).
						Warn("Failed to scrape students")
					errs = append(errs, fmt.Errorf("scrape year=%d dept=%s type=%s: %w", year, dept, st.name, err))
					errorCount++
					continue
				}

				// Save to database
				if err := db.SaveStudentsBatch(ctx, students); err != nil {
					log.WithError(err).
						WithField("year", year).
						WithField("dept", dept).
						WithField("type", st.name).
						WithField("count", len(students)).
						Warn("Failed to save student batch")
					errs = append(errs, fmt.Errorf("save year=%d dept=%s type=%s: %w", year, dept, st.name, err))
					errorCount++
					continue
				}

				studentCount += int64(len(students))
				completed++

				// Report progress every 5% or at completion
				progressInterval := max(totalTasks/20, 1) // ~5% intervals, minimum 1
				if completed%progressInterval == 0 || completed == totalTasks {
					elapsed := time.Since(startTime)
					avgTimePerTask := elapsed / time.Duration(completed)
					estimatedRemaining := avgTimePerTask * time.Duration(totalTasks-completed)
					log.WithField("progress", fmt.Sprintf("%d/%d (%.0f%%)", completed, totalTasks, float64(completed)*100/float64(totalTasks))).
						WithField("students", studentCount).
						WithField("type", st.name).
						WithField("elapsed_minutes", int(elapsed.Minutes())).
						WithField("estimated_remaining_minutes", int(estimatedRemaining.Minutes())).
						Info("ID module progress")
				}
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
			m.RecordJob("refresh", "contact", time.Since(startTime).Seconds())
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
		log.WithField("failed_source_error", errs[0]).Info("Contact module completed with partial success")
	}

	return nil
}

// warmupCourseModule warms course cache for the 4 most recent semesters.
// Probes actual data source (scraper) to find semesters with data.
// Updates SemesterCache after successful warmup.
// Returns courseProgramMap for syllabus warmup to use in dual-source fusion.
func warmupCourseModule(ctx context.Context, db *storage.DB, client *scraper.Client, log *logger.Logger, stats *Stats, m *metrics.Metrics, semesterCache *course.SemesterCache) (courseProgramMap, error) {
	programMap := make(courseProgramMap)
	startTime := time.Now()
	defer func() {
		if m != nil {
			m.RecordJob("refresh", "course", time.Since(startTime).Seconds())
		}
	}()

	log.Info("Starting course module warmup with data-driven probing")

	// Probe semesters to find 4 with actual data
	semesters, err := probeSemestersWithData(ctx, client, log)
	if err != nil {
		return programMap, fmt.Errorf("failed to probe semesters: %w", err)
	}

	if len(semesters) == 0 {
		log.Warn("No semesters with data found during probing")
		return programMap, nil
	}

	// Each semester makes 4 requests (U/M/N/P education codes)
	estimatedRequests := len(semesters) * 4
	log.WithField("semester_list", formatSemesters(semesters)).
		WithField("semester_count", len(semesters)).
		WithField("estimated_requests", estimatedRequests).
		Info("Course warmup: fetching courses by semester (data-driven probing)")

	// Scrape courses for each semester individually
	for _, sem := range semesters {
		select {
		case <-ctx.Done():
			return programMap, fmt.Errorf("course module canceled: %w", ctx.Err())
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

		// Collect raw program requirements for syllabus warmup (dual-source fusion)
		// This enables accurate program names + correct required/elective types
		for _, c := range courses {
			if len(c.RawProgramReqs) > 0 {
				programMap[c.UID] = c.RawProgramReqs
			}
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

		stats.Courses.Add(int64(len(courses)))
		log.WithField("year", sem.Year).
			WithField("term", sem.Term).
			WithField("count", len(courses)).
			WithField("program_reqs_collected", len(programMap)).
			WithField("total_cached", stats.Courses.Load()).
			Info("Courses cached for semester")
	}

	// Update shared semester cache after successful warmup
	if semesterCache != nil {
		semesterCache.Update(semesters)
		log.WithField("semester_list", formatSemesters(semesters)).
			Info("Updated semester cache with probed semesters")
	}

	log.WithField("total_courses", stats.Courses.Load()).
		WithField("semesters_processed", len(semesters)).
		WithField("program_map_size", len(programMap)).
		Info("Course module warmup completed")

	return programMap, nil
}

// probeSemestersWithData probes the course system to find 4 semesters with actual data.
// Starts from current ROC year term 2 and probes backwards until 4 semesters are found.
// Uses lightweight probing (single education code) to minimize requests.
func probeSemestersWithData(ctx context.Context, client *scraper.Client, log *logger.Logger) ([]course.Semester, error) {
	const (
		targetCount = 4  // Number of semesters to find
		maxProbes   = 12 // Maximum semesters to probe (prevents infinite loop)
	)

	// Get probe starting point: current ROC year, term 2
	startYear, startTerm := course.GetWarmupProbeStart()

	// Generate probe sequence
	probeSequence := course.GenerateProbeSequence(startYear, startTerm, maxProbes)

	log.WithField("start", fmt.Sprintf("%d-%d", startYear, startTerm)).
		WithField("max_probes", maxProbes).
		Info("Starting semester probing")

	var foundSemesters []course.Semester

	for _, sem := range probeSequence {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Probe with single education code (U = undergraduate) for efficiency
		// If U has data, the semester likely has data for other codes too
		hasData, err := probeSemesterHasData(ctx, client, sem.Year, sem.Term)
		if err != nil {
			log.WithError(err).
				WithField("year", sem.Year).
				WithField("term", sem.Term).
				Warn("Failed to probe semester, skipping")
			continue
		}

		if hasData {
			foundSemesters = append(foundSemesters, sem)
			log.WithField("year", sem.Year).
				WithField("term", sem.Term).
				WithField("found", len(foundSemesters)).
				Debug("Found semester with data")

			if len(foundSemesters) >= targetCount {
				break
			}
		} else {
			log.WithField("year", sem.Year).
				WithField("term", sem.Term).
				Debug("Semester has no data")
		}
	}

	log.WithField("found", len(foundSemesters)).
		WithField("semester_list", formatSemesters(foundSemesters)).
		Info("Semester probing completed")

	return foundSemesters, nil
}

// probeSemesterHasData checks if a semester has course data using a lightweight probe.
// Uses ntpu.ProbeCoursesExist() which only queries a single education code (U = undergraduate)
// to minimize HTTP requests (1 request vs 4 when using ScrapeCourses with empty title).
// Returns true if any courses are found, false otherwise.
func probeSemesterHasData(ctx context.Context, client *scraper.Client, year, term int) (bool, error) {
	return ntpu.ProbeCoursesExist(ctx, client, year, term)
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
// programMap provides raw program requirements from course list page for dual-source fusion
// Optimization: Processes courses in batches to reduce peak memory usage
func warmupSyllabusModule(ctx context.Context, db *storage.DB, client *scraper.Client, bm25Index *rag.BM25Index, log *logger.Logger, stats *Stats, m *metrics.Metrics, programMap courseProgramMap) error {
	startTime := time.Now()
	defer func() {
		if m != nil {
			m.RecordJob("refresh", "syllabus", time.Since(startTime).Seconds())
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

	// Calculate total courses for progress tracking
	var totalCourses int
	for _, sem := range semesters {
		count, err := db.CountCoursesBySemester(ctx, sem.Year, sem.Term)
		if err != nil {
			log.WithError(err).WithField("year", sem.Year).WithField("term", sem.Term).Warn("Failed to count courses")
			continue
		}
		totalCourses += count
	}

	log.WithField("semester_count", len(semesters)).
		WithField("total_courses", totalCourses).
		Info("Found recent semesters for syllabus warmup")

	if totalCourses == 0 {
		return nil
	}

	// Create syllabus scraper
	syllabusScraper := syllabus.NewScraper(client)

	var updatedCount, skippedCount, errorCount, processedCount, touchedCount int
	const batchLoadSize = 100 // Load 100 courses from DB at a time
	const saveBatchSize = 50  // Save to DB every 50 syllabi
	const touchBatchSize = 100

	// Buffer for batched saves
	var newSyllabi []*storage.Syllabus
	var touchedSyllabi []string

processLoop:
	for _, sem := range semesters {
		offset := 0
		for {
			select {
			case <-ctx.Done():
				log.WithField("processed", processedCount).
					Info("Syllabus warmup interrupted")
				break processLoop
			default:
			}

			// Load batch of courses
			courses, err := db.GetCoursesByYearTermPaginated(ctx, sem.Year, sem.Term, batchLoadSize, offset)
			if err != nil {
				log.WithError(err).WithField("year", sem.Year).WithField("term", sem.Term).Error("Failed to load course batch")
				break // Skip this semester on error
			}

			if len(courses) == 0 {
				break // End of semester
			}

			// Process current batch
			for _, course := range courses {
				processedCount++

				// Skip courses without detail URL
				if course.DetailURL == "" {
					skippedCount++
					continue
				}

				// Attach raw program requirements from course list page (dual-source fusion)
				// This enables accurate program names from syllabus + correct types from list
				if rawReqs, ok := programMap[course.UID]; ok {
					course.RawProgramReqs = rawReqs
				}

				// Scrape course detail (syllabus + program requirements)
				result, err := syllabusScraper.ScrapeCourseDetail(ctx, &course)
				if err != nil {
					log.WithError(err).WithField("uid", course.UID).Debug("Failed to scrape course detail")
					errorCount++
					continue
				}

				// Save program requirements to database (always, even if syllabus is empty)
				if len(result.Programs) > 0 {
					if err := db.SaveCoursePrograms(ctx, course.UID, result.Programs); err != nil {
						log.WithError(err).WithField("uid", course.UID).Debug("Failed to save course programs")
					}
				}

				// Skip empty syllabi for indexing (but programs were already saved above)
				if result.Fields.IsEmpty() {
					skippedCount++
					continue
				}

				// Compute content hash (include title/teachers to detect metadata changes)
				teachersForHash := strings.Join(course.Teachers, ",")
				contentForHash := course.Title + "\n" + teachersForHash + "\n" + result.Fields.Objectives + "\n" + result.Fields.Outline + "\n" + result.Fields.Schedule
				contentHash := syllabus.ComputeContentHash(contentForHash)

				// Check if content has changed
				existingHash, err := db.GetSyllabusContentHash(ctx, course.UID)
				if err != nil {
					log.WithError(err).WithField("uid", course.UID).Debug("Failed to get existing hash")
				}

				if existingHash == contentHash {
					touchedSyllabi = append(touchedSyllabi, course.UID)
					touchedCount++
					skippedCount++

					if len(touchedSyllabi) >= touchBatchSize {
						if err := db.TouchSyllabiBatch(ctx, touchedSyllabi); err != nil {
							log.WithError(err).Warn("Failed to touch unchanged syllabi")
							// Do not increase errorCount for touch failures to avoid masking scraping errors
						}
						touchedSyllabi = touchedSyllabi[:0]
					}

					continue
				}

				// Create syllabus record
				syl := &storage.Syllabus{
					UID:         course.UID,
					Year:        course.Year,
					Term:        course.Term,
					Title:       course.Title,
					Teachers:    course.Teachers,
					Objectives:  result.Fields.Objectives,
					Outline:     result.Fields.Outline,
					Schedule:    result.Fields.Schedule,
					ContentHash: contentHash,
				}

				newSyllabi = append(newSyllabi, syl)
				updatedCount++

				// Save batch if size reached
				if len(newSyllabi) >= saveBatchSize {
					if err := db.SaveSyllabusBatch(ctx, newSyllabi); err != nil {
						log.WithError(err).Error("Failed to save syllabi batch")
						errorCount += len(newSyllabi)
					}

					newSyllabi = make([]*storage.Syllabus, 0, saveBatchSize) // pre-allocate
				}

				// Report progress
				progressInterval := max(totalCourses/20, 1)
				if processedCount%progressInterval == 0 {
					elapsed := time.Since(startTime)
					avgTimePerCourse := elapsed / time.Duration(processedCount)
					estimatedRemaining := avgTimePerCourse * time.Duration(totalCourses-processedCount)
					log.WithField("progress", fmt.Sprintf("%d/%d (%.0f%%)", processedCount, totalCourses, float64(processedCount)*100/float64(totalCourses))).
						WithField("updated", updatedCount).
						WithField("skipped", skippedCount).
						WithField("errors", errorCount).
						WithField("estimated_remaining_minutes", int(estimatedRemaining.Minutes())).
						Info("Syllabus warmup progress")
				}
			}

			offset += batchLoadSize
		}
	}

	// Save remaining syllabi
	if len(newSyllabi) > 0 {
		if err := db.SaveSyllabusBatch(ctx, newSyllabi); err != nil {
			log.WithError(err).Error("Failed to save final syllabi batch")
			errorCount += len(newSyllabi)
		}
	}

	// Touch remaining unchanged syllabi
	if len(touchedSyllabi) > 0 {
		if err := db.TouchSyllabiBatch(ctx, touchedSyllabi); err != nil {
			log.WithError(err).Warn("Failed to touch remaining unchanged syllabi")
		}
	}

	// Rebuild BM25 index from database (includes all syllabi with full content)
	if bm25Index != nil {
		if err := bm25Index.Initialize(ctx, db); err != nil {
			log.WithError(err).Warn("Failed to rebuild BM25 index")
		}
	}

	stats.Syllabi.Add(int64(updatedCount))

	log.WithField("new", updatedCount).
		WithField("touched", touchedCount).
		WithField("skipped", skippedCount).
		WithField("errors", errorCount).
		WithField("total_scanned", processedCount).
		Info("Syllabus module warmup completed")

	return nil
}
