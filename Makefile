.PHONY: help build test lint clean docker-build docker-push run-server run-warmup install-tools

# Default target
help:
	@echo "Available targets:"
	@echo "  make build          - Build server and warmup binaries"
	@echo "  make test           - Run tests with coverage"
	@echo "  make lint           - Run linters (golangci-lint)"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make run-server     - Run the server locally"
	@echo "  make run-warmup     - Run warmup tool locally"
	@echo "  make install-tools  - Install development tools"

# Build binaries
build:
	@echo "Building server..."
	go build -ldflags="-s -w" -o bin/ntpu-linebot ./cmd/server
	@echo "Building warmup..."
	go build -ldflags="-s -w" -o bin/ntpu-linebot-warmup ./cmd/warmup
	@echo "Build complete!"

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@echo "Coverage:"
	go tool cover -func=coverage.out | grep total

# Run linters
lint:
	@echo "Running golangci-lint..."
	golangci-lint run --timeout=5m ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out
	rm -f *.db
	@echo "Clean complete!"

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t ntpu-linebot:latest .

# Run server locally
run-server:
	@echo "Starting server..."
	go run ./cmd/server

# Run warmup locally
run-warmup:
	@echo "Running warmup..."
	go run ./cmd/warmup

# Install development tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Tools installed!"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy
	@echo "Dependencies ready!"
