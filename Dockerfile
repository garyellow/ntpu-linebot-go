FROM --platform=$BUILDPLATFORM golang:1.25.5-alpine AS builder

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
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.GitCommit=${VCS_REF}" \
    -o /bin/ntpu-linebot ./cmd/server && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -buildvcs=false \
    -ldflags="-s -w" \
    -o /bin/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /bin/ntpu-linebot /app/ntpu-linebot
COPY --from=builder --chown=nonroot:nonroot /bin/healthcheck /app/healthcheck

EXPOSE 10000

ENV PORT=10000

HEALTHCHECK --interval=15s --timeout=5s --start-period=60s --retries=3 \
  CMD ["/app/healthcheck"]

USER nonroot:nonroot

ENTRYPOINT ["/app/ntpu-linebot"]
