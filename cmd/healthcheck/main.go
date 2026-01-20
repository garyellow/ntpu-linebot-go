// Package main provides a health check binary for container orchestration.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	os.Exit(run())
}

func run() int {
	port := os.Getenv("NTPU_PORT")
	if port == "" {
		port = "10000"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://localhost:%s/livez", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 1
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}

	// Read status code before closing body
	statusOK := resp.StatusCode == http.StatusOK
	_ = resp.Body.Close()

	if !statusOK {
		return 1
	}
	return 0
}
