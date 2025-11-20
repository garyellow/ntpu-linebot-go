# Build stage
# Note: Using modernc.org/sqlite (pure Go) instead of mattn/go-sqlite3 (CGO)
# This allows CGO_ENABLED=0 for truly static binaries and cross-compilation
# Trade-off: Slightly lower performance but better portability
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build binaries (CGo-free for static linking)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/ntpu-linebot ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/ntpu-linebot-warmup ./cmd/warmup

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 appuser && adduser -D -u 1000 -G appuser appuser

# Create data directory
RUN mkdir -p /data && chown appuser:appuser /data

WORKDIR /home/appuser

# Copy binaries from builder
COPY --from=builder /bin/ntpu-linebot /usr/local/bin/ntpu-linebot
COPY --from=builder /bin/ntpu-linebot-warmup /usr/local/bin/ntpu-linebot-warmup

# Copy static assets
COPY --chown=appuser:appuser assets ./assets
COPY --chown=appuser:appuser add_friend ./add_friend

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 10000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:10000/healthz || exit 1

# Default command (run server)
ENTRYPOINT ["/usr/local/bin/ntpu-linebot"]
