# Distroless variant (default) - minimal attack surface for production
# For debugging/shell access, use Dockerfile.alpine instead

FROM --platform=$BUILDPLATFORM golang:1.25.7-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

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
  CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
  -trimpath \
  -buildvcs=false \
  -ldflags="-s -w -X github.com/garyellow/ntpu-linebot-go/internal/buildinfo.Version=${VERSION} -X github.com/garyellow/ntpu-linebot-go/internal/buildinfo.BuildDate=${BUILD_DATE} -X github.com/garyellow/ntpu-linebot-go/internal/buildinfo.Commit=${VCS_REF}" \
  -o /bin/ntpu-linebot ./cmd/server && \
  CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
  -trimpath \
  -buildvcs=false \
  -ldflags="-s -w" \
  -o /bin/healthcheck ./cmd/healthcheck

RUN mkdir -p /data-dir

FROM gcr.io/distroless/static-debian13:nonroot

ARG VERSION=dev
ARG BUILD_DATE
ARG VCS_REF

LABEL org.opencontainers.image.title="ntpu-linebot-go" \
  org.opencontainers.image.description="NTPU LineBot Go" \
  org.opencontainers.image.source="https://github.com/garyellow/ntpu-linebot-go" \
  org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${VCS_REF}" \
  org.opencontainers.image.created="${BUILD_DATE}"

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /bin/ntpu-linebot /app/ntpu-linebot
COPY --from=builder --chown=nonroot:nonroot /bin/healthcheck /app/healthcheck
COPY --from=builder --chown=nonroot:nonroot /data-dir /data

EXPOSE 10000

ENV PORT=10000 \
  NTPU_SENTRY_RELEASE=${VERSION}

HEALTHCHECK --interval=15s --timeout=5s --start-period=60s --retries=3 \
  CMD ["/app/healthcheck"]

USER nonroot:nonroot

ENTRYPOINT ["/app/ntpu-linebot"]
