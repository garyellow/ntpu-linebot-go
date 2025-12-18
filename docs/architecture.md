# NTPU LineBot Go - 架構設計文件

## 概述

NTPU LineBot 是一個為國立臺北大學設計的 LINE 聊天機器人，提供學號查詢、通訊錄查詢、課程查詢等功能。本文件詳細說明系統架構、設計決策和技術細節。

## 系統架構

### 整體架構圖

```
┌─────────────────────────────────────────────────────────────────┐
│                        LINE Platform                            │
│                     (Messaging API v8)                          │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTPS Webhook
                             │ (Signature Verification)
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Webhook Handler Layer                        │
│                     (internal/webhook/)                         │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Gin HTTP Server (Port 10000)                             │  │
│  │  • Signature Validation                                   │  │
│  │  • Request Size Limiting (1MB)                            │  │
│  │  • Rate Limiting (100 rps global, 6 tokens/user)          │  │
│  │  • Context Timeout (60s)                                  │  │
│  └───────────────────────────────────────────────────────────┘  │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Bot Module Dispatcher                         │
│   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐           │
│   │ ID Module   │   │Contact      │   │Course       │           │
│   │             │   │Module       │   │Module       │           │
│   │ CanHandle() │   │             │   │             │           │
│   │ HandleMsg() │   │ • Emergency │   │ • UID Query │           │
│   │ HandlePost()│   │ • Search    │   │ • Title     │           │
│   │             │   │             │   │ • Teacher   │           │
│   │ • ID Query  │   │ • Big5 Enc  │   │ • Smart     │           │
│   │ • Name      │   │             │   │             │           │
│   │ • Year      │   │             │   │             │           │
│   │ • Dept      │   │             │   │             │           │
│   └──────┬──────┘   └──────┬──────┘   └──────┬──────┘           │
└──────────┼─────────────────┼─────────────────┼──────────────────┘
           │                 │                 │
           └─────────────────┴─────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Repository Layer                             │
│                  (Cache-First Strategy)                         │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  1. Check SQLite Cache (TTL: 7 days, configurable)        │  │
│  │  2. If Miss → Trigger Scraper                             │  │
│  │  3. Save to Cache                                         │  │
│  │  4. Return Data                                           │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  Tables:                                                        │
│  • students (id, name, year, department, cached_at)             │
│  • contacts (uid, type, name, organization, ..., cached_at)     │
│  • courses (uid, year, term, no, title, teachers, teacher_urls, │
│             times, locations, detail_url, note, cached_at)      │
│  • stickers (url, source, cached_at, success/failure_count)     │
│  • syllabi (uid, year, term, title, teachers, objectives,       │
│             outline, schedule, content_hash, cached_at)         │
└────────────────────────────┬──────────────────────┬─────────────┘
                             │                      │
                             ▼                      ▼
┌────────────────────────────────────┐ ┌──────────────────────────┐
│          Scraper Layer             │ │   BM25 Search Layer      │
│      (internal/scraper/)           │ │   (internal/rag/)        │
├────────────────────────────────────┤ ├──────────────────────────┤
│  Rate Limiter & Retry              │ │  BM25Index               │
│  • Rate limit: 2s per request      │ │  • Pure Go(Memory Index) │
│  • Timeout: 60s per request        │ │  • Chinese Tokenize      │
│  • Exponential backoff on failure  │ │  • Keyword Matching      │
│  • Jitter: ±25% randomization      │ │  • Query Expansion       │
│  • Max retries: 5 (configurable)   │ │    (Gemini/Groq LLM)     │
└────────────────────────────────────┘ └──────────────────────────┘
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  URL Cache & Failover                                      │ │
│  │  • Automatic failover between URLs                         │ │
│  │  • 3 mirrors per service (IP + domain)                     │ │
│  └────────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  HTTP Client                                               │ │
│  │  • User-Agent Rotation (corpix/uarand)                     │ │
│  │  • Failover URLs (3 mirrors per service)                   │ │
│  │  • goquery for HTML parsing                                │ │
│  └────────────────────────────────────────────────────────────┘ │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Target Websites                           │
│  • LMS (Digital Learning): https://lms.ntpu.edu.tw              │
│  • SEA (Campus Directory): https://sea.cc.ntpu.edu.tw           │
│  Mirrors: 120.126.197.7, 140.126.197.8, ...                     │
└─────────────────────────────────────────────────────────────────┘
```

### 資料流程

#### 1. Webhook 接收流程
```
LINE Platform → Gin Handler → Signature Verify → Parse Event
    ↓
Rate Limit Check (Global + Per-User)
    ↓
Dispatch to Bot Module (based on keywords)
    ↓ (no match)
NLU Intent Parser (if enabled)
    ↓
Process Message
    ↓
Reply to LINE (max 5 messages)
    ↓
Record Metrics
```

#### 1.1 NLU 意圖解析流程（可選）
```
User Input → Keyword Matching (existing handlers)
                  ↓ (no match)
         handleUnmatchedMessage()
                  ↓
    ┌─────────────┴─────────────┐
    │                           │
Personal Chat              Group Chat
    │                           │
    │                     @Bot mentioned?
    │                       ↓     ↓
    │                     Yes     No → Silent ignore
    │                       │
    │                  Remove @Bot mentions
    │                       │
    └───────────────────────┘
                  ↓
         NLU Parser enabled?
              ↓        ↓
            Yes        No → Help message
              │
    IntentParser.Parse()
    (Gemini Function Calling)
              │
    ┌─────────┴─────────┐
    │                   │
Function Call      Text Response
    │              (Clarification)
    │                   │
dispatchIntent()   Return text
    │
Route to Handler
(course/id/contact/help)
```

#### 2. 資料查詢流程（Cache-First）
```
User Query → Bot Module → Repository Layer
                              ↓
                         Check Cache
                              ↓
                    ┌─────────┴─────────┐
                    │                   │
                Cache Hit           Cache Miss
                    │                   │
                Return Data         Rate Limiter
                    │                   ↓
                    │              HTTP Scrape
                    │                   ↓
                    │              Parse HTML
                    │                   ↓
                    │              Save Cache
                    │                   │
                    └───────────────────┘
                              ↓
                        Return to User
```

## 設計模式

### 1. Repository Pattern（儲存庫模式）

**目的**: 將資料存取邏輯與業務邏輯分離

**實現**:
- `internal/storage/repository.go`: 定義所有 CRUD 操作
- 使用 interface 方便測試時 mock
- Cache-first 策略：優先查詢快取，miss 時觸發爬蟲

**優點**:
- 易於測試（可 mock Repository）
- 易於切換資料來源（SQLite → PostgreSQL）
- 業務邏輯不依賴資料庫細節

### 2. Rate Limiting（限流）

**兩層限流機制**:

1. **Scraper Level（爬蟲層）**
   - Rate limiting: 2s delay between requests
   - Exponential backoff on failure: 4s → 8s → 16s → 32s → 64s
   - 用於保護目標網站

2. **Webhook Level（API 層）**
   - Global: 100 rps
   - Per-User: 6 tokens, refill 1 token/5s
   - 防止濫用

### 3. Strategy Pattern（策略模式）

**Bot Module 選擇**:
```go
// internal/webhook/handler.go
func (h *Handler) handleMessageEvent(ctx context.Context, event webhook.MessageEvent) {
    text := extractText(event)

    // Strategy pattern: 依關鍵字選擇處理器
    if h.idHandler.CanHandle(text) {
        return h.idHandler.HandleMessage(ctx, text)
    }
    if h.contactHandler.CanHandle(text) {
        return h.contactHandler.HandleMessage(ctx, text)
    }
    if h.courseHandler.CanHandle(text) {
        return h.courseHandler.HandleMessage(ctx, text)
    }

    return h.getHelpMessage()
}
```

## 關鍵技術決策

### 1. 為什麼使用 SQLite 而非 Redis/PostgreSQL？

**優點**:
- ✅ 零配置（embedded database）
- ✅ 支援 WAL mode（併發讀寫）
- ✅ 檔案型資料庫（易於備份）
- ✅ 適合中小型資料量（< 1M records）
- ✅ Pure Go 實現（modernc.org/sqlite）無需 CGO

**缺點**:
- ❌ 單一 Writer（WAL mode 可緩解）
- ❌ 不支援分散式部署
- ❌ 查詢效能略低於 PostgreSQL

**結論**: 對於單機部署的 LINE Bot，SQLite 是最佳選擇。

### 2. 為什麼使用 Gin 而非 Echo/Fiber？

| 框架 | 優點 | 缺點 |
|------|------|------|
| **Gin** | ✅ 生態系統完善<br>✅ 中介層豐富<br>✅ 社群活躍<br>✅ 文件完整 | ❌ 效能略低於 Fiber |
| Fiber | ✅ 效能最高<br>✅ Express-like API | ❌ 非標準庫（自訂 HTTP）<br>❌ 生態較小 |
| Echo | ✅ 效能好<br>✅ 標準庫兼容 | ❌ 中介層較少 |

**選擇 Gin 原因**:
- LINE Bot 效能瓶頸在爬蟲，而非 HTTP 處理
- Gin 中介層生態完善（prometheus, cors, recovery）
- 團隊熟悉度高

### 3. 為什麼不用 gRPC/Protocol Buffers？

**原因**:
- LINE Messaging API 使用 REST + JSON
- 無需微服務間通訊
- JSON 更易於除錯和日誌記錄

### 4. 並發模型選擇

**Goroutine + errgroup**:
- 每日刷新：contact 與 course 並行，syllabus 等待 course 完成
- 使用 `errgroup.WithContext` 管理並發、錯誤與取消
- 使用 `context.Context` 優雅取消
- Scraper Rate Limiting: 2s 間隔

## 效能優化

### 1. 快取策略（TTL）

採用單層 TTL 策略：

| TTL 類型 | 預設值 | 用途 |
|---------|--------|------|
| **TTL** | 7 天 | 絕對過期，資料必須刪除 |

**資料類型考量**:
- 學生資料：學期內穩定（不每日刷新；通常僅啟動時建立/更新快取）
- 通訊錄：變動頻率低
- 課程資料：學期內穩定
- 課程大綱：學期內穩定（智慧搜尋用）

**背景任務排程** (臺灣時間):
- **Sticker**: 啟動時一次
- **每日刷新** (3:00 AM): contact, course (每日), syllabus (若設定 LLM API Key)
- **Cache Cleanup** (4:00 AM): 刪除過期資料 (7 天 TTL) + VACUUM

### 2. 智慧搜尋架構（可選）

**BM25 + Query Expansion 流程**:
1. **Warmup**: 課程列表 → 抓取大綱 → 存入 SQLite + 建立 BM25 索引
2. **查詢**: 輸入 → Query Expansion (LLM) → BM25 Search → Confidence Scoring

**特性**:
- **Query Expansion**: LLM 擴展同義詞、縮寫
- **BM25**: 中文 unigram 分詞，精確關鍵字匹配
- **Confidence**: 相對分數 (score / maxScore)，0-1 範圍

**關鍵概念**:
- BM25 輸出無界分數，不可跨查詢比較
- 信心分數使用相對分數 (score / maxScore)，非向量相似度
- 分數分佈遵循 Normal-Exponential 混合模型（學術標準）

**啟用條件**:
- 設定 `GEMINI_API_KEY` 或 `GROQ_API_KEY`（自動啟用 syllabus 模組）
- Query Expansion 需要 LLM API Key
- 即使沒有 API Key，基本 BM25 搜尋仍可使用（但需手動載入大綱資料）

**關鍵實作**:
- `internal/genai/gemini_expander.go`: Query Expansion（Gemini）
- `internal/genai/groq_expander.go`: Query Expansion（Groq）
- `internal/rag/bm25.go`: BM25Index（記憶體索引）
- `internal/syllabus/`: 課程大綱擷取與 hash 計算

**效能優化**:
- 使用 `content_hash` 實現增量更新（僅重新索引變更內容）
- BM25 索引在記憶體中運作，查詢延遲 <10ms

### 3. SQL 查詢優化

**索引設計**:
```sql
-- students table
CREATE INDEX idx_students_name ON students(name);            -- 姓名搜尋
CREATE INDEX idx_students_year_dept ON students(year, department); -- 複合查詢
CREATE INDEX idx_students_cached_at ON students(cached_at);  -- TTL 清理

-- contacts table
CREATE INDEX idx_contacts_name ON contacts(name);            -- 姓名搜尋
CREATE INDEX idx_contacts_organization ON contacts(organization); -- 單位過濾

-- courses table
CREATE INDEX idx_courses_title ON courses(title);            -- 課程名稱搜尋
CREATE INDEX idx_courses_teachers ON courses(teachers);      -- 教師搜尋（JSON）
```

**查詢優化**:
- ✅ 使用 Prepared Statements（防 SQL Injection）
- ✅ LIKE 查詢前先 sanitize（escape `%`, `_`）
- ✅ 分頁查詢（避免全表掃描）
- ✅ 使用 `EXPLAIN QUERY PLAN` 分析慢查詢

### 3. 記憶體管理

**避免記憶體洩漏**:
```go
// ✅ Good: 使用 context 控制生命週期
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel() // 確保 goroutine 退出

// ❌ Bad: goroutine 無法停止
go func() {
    for {
        // No way to stop
    }
}()
```

**並行執行模組**:
```go
// Warmup 模組使用 WaitGroup 並行執行 (Go 1.25+)
var wg sync.WaitGroup
for _, module := range modules {
    wg.Go(func() {
        warmupModule(ctx, module)
    })
}
wg.Wait()
```

## 安全性

### 1. Webhook 簽章驗證

```go
func (h *Handler) Handle(c *gin.Context) {
    // LINE SDK 自動驗證 X-Line-Signature
    cb, err := webhook.ParseRequest(h.channelSecret, c.Request)
    if err == webhook.ErrInvalidSignature {
        // 拒絕請求
        c.JSON(400, gin.H{"error": "invalid signature"})
        return
    }
}
```

### 2. SQL Injection 防護

```go
// ✅ Good: Prepared Statement
db.Query("SELECT * FROM students WHERE name LIKE ?", "%"+sanitizeSearchTerm(name)+"%")

// ❌ Bad: String concatenation
db.Query("SELECT * FROM students WHERE name LIKE '%" + name + "%'")
```

**Sanitize Function**:
```go
func sanitizeSearchTerm(term string) string {
    term = strings.ReplaceAll(term, "\\", "\\\\") // Escape backslash
    term = strings.ReplaceAll(term, "%", "\\%")   // Escape wildcard
    term = strings.ReplaceAll(term, "_", "\\_")   // Escape single char wildcard
    return term
}
```

### 3. Rate Limiting（防 DDoS）

- Global Rate Limit: 100 rps
- Per-User Rate Limit: 6 tokens, 1 token/5s refill (Token Bucket)
- 超過限制靜默丟棄請求

### 4. 輸入驗證

```go
// 訊息長度限制
if len(text) > 20000 {
    return []messaging_api.MessageInterface{
        lineutil.NewTextMessage("❌ 訊息內容過長（最多 20,000 字元）"),
    }
}

// Postback 資料長度限制
if len(data) > 300 {
    return // LINE limit: 300 bytes
}
```

## 監控與可觀測性

### 1. Prometheus 指標

**關鍵指標**:
```
# 請求量 (RED Method)
ntpu_webhook_total{event_type, status}
ntpu_scraper_total{module, status}
ntpu_llm_total{operation, status}
ntpu_search_total{type, status}

# 延遲
ntpu_webhook_duration_seconds{event_type}
ntpu_scraper_duration_seconds{module}
ntpu_llm_duration_seconds{operation}
ntpu_search_duration_seconds{type}

# 快取 (USE Method)
ntpu_cache_operations_total{module, result}  # result: hit, miss
ntpu_cache_size{module}

# 其他
ntpu_index_size{index}  # BM25 索引大小
ntpu_rate_limiter_dropped_total{limiter}
ntpu_job_duration_seconds{job, module}
```

### 2. 結構化日誌

```json
{
  "level": "info",
  "msg": "Webhook received",
  "timestamp": "2024-03-15T10:30:45+08:00",
  "module": "webhook",
  "event_type": "message",
  "user_id": "U1234...",
  "duration_ms": 234
}
```

### 3. 告警規則

**SLO 目標**:
- 可用性: 99.9%（每月最多 43 分鐘停機）
- P95 延遲: < 3 秒
- 錯誤率: < 1%

**告警閾值**:
```yaml
- alert: ScraperHighFailureRate
  expr: rate(ntpu_scraper_total{status="error"}[5m]) > 0.3
  for: 3m

- alert: WebhookHighLatency
  expr: histogram_quantile(0.95, ntpu_webhook_duration_seconds_bucket) > 3
  for: 5m

- alert: ServiceDown
  expr: up{job="ntpu-linebot"} == 0
  for: 2m
```

## 部署架構

### docker compose（推薦）

```yaml
services:
  ntpu-linebot:
    image: garyellow/ntpu-linebot-go:latest
    ports:
      - "10000:10000"
    volumes:
      - data:/data
    environment:
      - LINE_CHANNEL_ACCESS_TOKEN=${TOKEN}
      - LINE_CHANNEL_SECRET=${SECRET}
    depends_on:
      - prometheus

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus:/etc/prometheus

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin123

volumes:
  data:
```

### Kubernetes（未來擴展）

**考慮因素**:
- SQLite 不支援分散式（需改用 PostgreSQL）
- 需要實作 Leader Election（多 Pod 寫入協調）
- PersistentVolume 管理

## 未來擴展方向

### 1. 多語言支援
- 使用 i18n 套件
- 支援英文、中文（繁/簡）

### 2. 分散式部署
- 改用 PostgreSQL
- Redis 作為 Session Store
- Kafka 作為 Message Queue

### 3. 更多資料來源
- NTPU 課程查詢系統 API（若官方開放）
- 校內公告爬蟲
- 圖書館座位查詢

### 4. AI 整合
- 使用 LLM 理解自然語言查詢
- 智能推薦相關資訊
- 多輪對話支援

## 參考資料

- [LINE Messaging API](https://developers.line.biz/en/docs/messaging-api/)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
- [SQLite WAL Mode](https://www.sqlite.org/wal.html)
- [Prometheus Best Practices](https://prometheus.io/docs/practices/)
- [Grafana Dashboard Design](https://grafana.com/docs/grafana/latest/dashboards/)
- [BM25 Algorithm](https://en.wikipedia.org/wiki/Okapi_BM25) - 關鍵字搜尋演算法
- [Google Gemini API](https://ai.google.dev/gemini-api/docs) - NLU 和 Query Expansion
