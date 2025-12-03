FROM golang:1.25.5-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY . .

ARG VERSION=dev
ARG BUILD_DATE
ARG VCS_REF

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.GitCommit=${VCS_REF}" \
    -o /bin/ntpu-linebot ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /bin/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder --chown=65532:65532 /bin/ntpu-linebot /app/ntpu-linebot
COPY --from=builder --chown=65532:65532 /bin/healthcheck /app/healthcheck

EXPOSE 10000

ENV PORT=10000

HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
  CMD ["/app/healthcheck"]

USER 65532:65532

ENTRYPOINT ["/app/ntpu-linebot"]
