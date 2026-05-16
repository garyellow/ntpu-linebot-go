# Configuration Reference

Complete environment variable reference for NTPU LineBot Go.
Copy [`.env.example`](../.env.example) to `.env` to get started.

---

## Required

| Variable | Description |
|----------|-------------|
| `NTPU_LINE_CHANNEL_ACCESS_TOKEN` | LINE Channel Access Token |
| `NTPU_LINE_CHANNEL_SECRET` | LINE Channel Secret |

---

## Server

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_PORT` | `10000` | HTTP listen port |
| `NTPU_LOG_LEVEL` | `info` | Log verbosity: `debug` / `info` / `warn` / `error` |
| `NTPU_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |
| `NTPU_SERVER_NAME` | — | Node name attached to logs, metrics, and Sentry events |
| `NTPU_INSTANCE_ID` | — | Instance identifier for multi-node deployments |

---

## Data & Scraping

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_DATA_DIR` | `/data` (Linux/Mac) or `./data` (Windows) | Directory for SQLite database |
| `NTPU_CACHE_TTL` | `168h` | Absolute TTL for contacts, courses, and programs (7 days) |
| `NTPU_SCRAPER_TIMEOUT` | `60s` | Per-request HTTP timeout for the scraper client |
| `NTPU_SCRAPER_MAX_RETRIES` | `10` | Max retry attempts with exponential backoff |
| `NTPU_WEBHOOK_TIMEOUT` | `60s` | Bot processing timeout per webhook event |

---

## Rate Limits

All limits use token-bucket algorithm unless noted.

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_GLOBAL_RATE_RPS` | `100` | Global request rate limit (requests per second) |
| `NTPU_USER_RATE_BURST` | `15` | Per-user burst capacity |
| `NTPU_USER_RATE_REFILL` | `0.1` | Per-user refill rate (tokens/s); `0.1` = 1 per 10 s |
| `NTPU_LLM_RATE_BURST` | `60` | Per-user LLM burst capacity |
| `NTPU_LLM_RATE_REFILL` | `30` | Per-user LLM refill rate (tokens/hour) |
| `NTPU_LLM_RATE_DAILY` | `180` | Per-user daily LLM cap; `0` = disabled |

---

## Background Jobs

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_WARMUP_WAIT` | `false` | Block `/webhook` until warmup completes (useful when S3 snapshot sync is enabled) |
| `NTPU_WARMUP_MAX_WAIT` | `0` | Max duration to wait for warmup; `0` = wait indefinitely. Governs both `/readyz` (always) and `/webhook` (when `NTPU_WARMUP_WAIT=true`) — both stay 503 until warmup completes or this duration elapses. Warmup always continues in background. |
| `NTPU_MAINTENANCE_REFRESH_INTERVAL` | `24h` | Interval between contact/course/program refresh jobs |
| `NTPU_MAINTENANCE_CLEANUP_INTERVAL` | `24h` | Interval between expired-cache cleanup jobs |

---

## LLM — AI Features (optional)

Set `NTPU_LLM_ENABLED=true` and provide at least one API key to enable NLU intent parsing and query expansion.

### Control

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_LLM_ENABLED` | `false` | Master switch for all LLM features |
| `NTPU_LLM_PROVIDERS` | `gemini,groq,cerebras,openai` | Comma-separated provider priority order for fallback chain |

### Gemini

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_GEMINI_API_KEY` | — | Google AI Studio API key |
| `NTPU_GEMINI_INTENT_MODELS` | `gemma-4-31b-it,gemma-4-26b-a4b-it` | Ordered model list for intent parsing |
| `NTPU_GEMINI_EXPANDER_MODELS` | `gemma-4-31b-it,gemma-4-26b-a4b-it` | Ordered model list for query expansion |

### Groq

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_GROQ_API_KEY` | — | Groq API key |
| `NTPU_GROQ_INTENT_MODELS` | `openai/gpt-oss-120b,openai/gpt-oss-20b,llama-3.3-70b-versatile,qwen/qwen3-32b,llama-3.1-8b-instant` | Ordered model list |
| `NTPU_GROQ_EXPANDER_MODELS` | `openai/gpt-oss-120b,openai/gpt-oss-20b,llama-3.3-70b-versatile,qwen/qwen3-32b,llama-3.1-8b-instant` | Ordered model list |

### Cerebras

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_CEREBRAS_API_KEY` | — | Cerebras API key |
| `NTPU_CEREBRAS_INTENT_MODELS` | `gpt-oss-120b,llama3.1-8b` | Ordered model list |
| `NTPU_CEREBRAS_EXPANDER_MODELS` | `gpt-oss-120b,llama3.1-8b` | Ordered model list |

### OpenAI-Compatible (self-hosted / custom)

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_OPENAI_API_KEY` | — | API key for the custom endpoint |
| `NTPU_OPENAI_ENDPOINT` | — | Base URL, e.g. `http://localhost:1234/v1/`; must start with `http://` or `https://` |
| `NTPU_OPENAI_INTENT_MODELS` | — | Ordered model list |
| `NTPU_OPENAI_EXPANDER_MODELS` | — | Ordered model list |

> `NTPU_OPENAI_API_KEY` and `NTPU_OPENAI_ENDPOINT` must be set together (or neither). When `openai` is listed in `NTPU_LLM_PROVIDERS`, at least one of `NTPU_OPENAI_INTENT_MODELS` or `NTPU_OPENAI_EXPANDER_MODELS` is required.

---

## S3-Compatible Snapshot Sync (optional)

Enables distributed SQLite snapshot sharing for multi-node deployments.
The endpoint must support `HeadObject`, `GetObject`, `PutObject` with `If-Match`/`If-None-Match`,
`ListObjectsV2`, and `DeleteObject`. No Object Lock, Versioning, or Lifecycle required.

Common endpoint values:
- **AWS S3**: `https://s3.<region>.amazonaws.com`
- **MinIO**: `http://localhost:9000`
- **Cloudflare R2**: `https://<account_id>.r2.cloudflarestorage.com`

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_S3_ENABLED` | `false` | Master switch |
| `NTPU_S3_ENDPOINT` | — | S3-compatible endpoint URL; must start with `http://` or `https://` |
| `NTPU_S3_REGION` | `us-east-1` | AWS signing region |
| `NTPU_S3_ACCESS_KEY_ID` | — | Access key ID |
| `NTPU_S3_SECRET_ACCESS_KEY` | — | Secret access key |
| `NTPU_S3_BUCKET_NAME` | — | Bucket name |
| `NTPU_S3_SNAPSHOT_KEY` | `snapshots/cache.db.zst` | Object key for the compressed SQLite snapshot |
| `NTPU_S3_LOCK_KEY` | `locks/leader.json` | Object key for the leader lease lock |
| `NTPU_S3_LOCK_TTL` | `1h` | Leader lease TTL; minimum `30s` (renews at TTL/3) |
| `NTPU_S3_SNAPSHOT_POLL_INTERVAL` | `15m` | How often followers poll for a new snapshot; must be positive |
| `NTPU_S3_DELTA_PREFIX` | `deltas` | Object key prefix for append-only delta logs |
| `NTPU_S3_SCHEDULE_KEY` | `schedules/maintenance.json` | Object key for shared maintenance schedule state |

> When `NTPU_S3_ENABLED=true`, the following are required: `NTPU_S3_ENDPOINT`, `NTPU_S3_ACCESS_KEY_ID`, `NTPU_S3_SECRET_ACCESS_KEY`, `NTPU_S3_BUCKET_NAME`.

---

## Observability (optional)

### Sentry

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_SENTRY_ENABLED` | `false` | Master switch |
| `NTPU_SENTRY_DSN` | — | Sentry DSN (`https://TOKEN@HOST/PROJECT_ID`); required when enabled |
| `NTPU_SENTRY_ENVIRONMENT` | — | Environment tag, e.g. `production` or `staging` |
| `NTPU_SENTRY_RELEASE` | — | Release version tag, e.g. `ntpu-linebot-go@1.0.0` |
| `NTPU_SENTRY_SAMPLE_RATE` | `1.0` | Error sampling rate (`0.0`–`1.0`) |
| `NTPU_SENTRY_TRACES_SAMPLE_RATE` | `0.0` | Tracing sampling rate; `0.0` = disabled |

### Better Stack

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_BETTERSTACK_ENABLED` | `false` | Master switch |
| `NTPU_BETTERSTACK_TOKEN` | — | Better Stack source token; required when enabled |
| `NTPU_BETTERSTACK_ENDPOINT` | — | Ingestion endpoint, e.g. `https://in.logs.betterstack.com` |

### Metrics

| Variable | Default | Description |
|----------|---------|-------------|
| `NTPU_METRICS_AUTH_ENABLED` | `false` | Enable Basic Auth on `/metrics` |
| `NTPU_METRICS_USERNAME` | `prometheus` | Basic Auth username; must not be empty when auth enabled |
| `NTPU_METRICS_PASSWORD` | — | Basic Auth password; required when auth enabled |
