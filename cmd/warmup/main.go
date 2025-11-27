// Package main provides a standalone cache warmup utility.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/warmup"
)

// CLI flags
var (
	resetFlag   = flag.Bool("reset", false, "Delete all cache data before warmup")
	modulesFlag = flag.String("modules", "", "Comma-separated list of modules to warmup (empty = use config default)")
)

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
	log.Info("Starting warmup")

	// Connect to database
	db, err := storage.New(cfg.SQLitePath, cfg.CacheTTL)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}
	defer func() { _ = db.Close() }()

	// Parse modules
	modulesStr := *modulesFlag
	if modulesStr == "" {
		modulesStr = cfg.WarmupModules
	}
	moduleList := warmup.ParseModules(modulesStr)

	// Create scraper client (same settings as server)
	scraperClient := scraper.NewClient(
		cfg.ScraperTimeout,
		cfg.ScraperMaxRetries,
	)

	// Create sticker manager
	stickerManager := sticker.NewManager(db, scraperClient, log)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.WarmupTimeout)
	defer cancel()

	// Run warmup
	stats, err := warmup.Run(ctx, db, scraperClient, stickerManager, log, warmup.Options{
		Modules: moduleList,
		Timeout: cfg.WarmupTimeout,
		Reset:   *resetFlag,
	})

	if err != nil {
		log.WithError(err).Fatal("Warmup failed")
	}

	// Print summary
	total := stats.Students.Load() + stats.Contacts.Load() + stats.Courses.Load() + stats.Stickers.Load()
	fmt.Printf("\n✓ Cached %d records: %d students, %d contacts, %d courses, %d stickers\n",
		total,
		stats.Students.Load(),
		stats.Contacts.Load(),
		stats.Courses.Load(),
		stats.Stickers.Load())
}
