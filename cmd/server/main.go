// Package main is the entry point for the NTPU LineBot server.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/garyellow/ntpu-linebot-go/internal/app"
	"github.com/garyellow/ntpu-linebot-go/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	application, err := app.Initialize(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	if err := application.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}
}
