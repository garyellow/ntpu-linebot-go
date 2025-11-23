package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}

	client := &http.Client{Timeout: 8 * time.Second}
	url := fmt.Sprintf("http://localhost:%s/healthz", port)

	resp, err := client.Get(url)
	if err != nil {
		os.Exit(1)
	}
	// Error ignored: response body close error is non-critical for healthcheck
	// and process exits immediately anyway
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}

	os.Exit(0)
}
