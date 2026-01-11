# NTPU LineBot Go - API 文件

## 概述

本文件描述 NTPU LineBot 的所有 HTTP API 端點和 LINE Messaging API 互動方式。

## HTTP API 端點

### 基本資訊

- **Base URL**: `http://localhost:10000` (本機) 或 `https://your-domain.com` (線上)
- **Content-Type**: `application/json`
- **Authentication**: LINE Webhook 需要簽章驗證

---

## 1. Health Check 端點

### 1.1 Liveness Probe (存活探測)

**Kubernetes Liveness Probe** - 檢查進程是否存活且能回應 HTTP 請求。**不檢查外部依賴**（資料庫、快取等）以避免級連失敗。

```http
GET /livez
```

**Response** (200 OK):
```json
{
  "status": "alive"
}
```

**用途**:
- Kubernetes/Docker liveness probe
- **最輕量級檢查**：僅確認進程能回應 HTTP
- **不檢查依賴服務**：避免資料庫暫時不可用導致 Pod 重啟
- **失敗行為**: Kubernetes **重啟 Pod**

**何時失敗**:
- 進程崩潰或死鎖
- 嚴重記憶體洩漏導致無法處理 HTTP 請求

---

### 1.2 Readiness Probe (就緒探測)

**Kubernetes Readiness Probe** - 檢查服務是否準備好接收流量（完整依賴檢查）。包含資料庫連線、快取狀態、功能可用性。

```http
GET /readyz
```

**Response** (200 OK):
```json
{
  "status": "ready",
  "database": "connected",
  "cache": {
    "students": 15234,
    "contacts": 823,
    "courses": 4521,
    "stickers": 42
  },
  "features": {
    "bm25_search": true,
    "nlu": true,
    "query_expansion": true
  }
}
```

**Response** (503 Service Unavailable):
```json
{
  "status": "not ready",
  "reason": "database unavailable"
}
```

**檢查項目**:
- ✅ 資料庫連線（Ping 測試）
- ✅ 快取資料統計（students, contacts, courses, stickers）
- ✅ 功能啟用狀態（BM25, NLU, Query Expansion）

**用途**:
- Kubernetes readiness probe
- 確認服務完全就緒後才接收流量
- **檢查超時**: 3 秒（config.ReadinessCheckTimeout）
- **失敗行為**: Kubernetes **暫時移除流量**（不重啟 Pod）

**何時失敗**:
- 資料庫無法連線
- 快取尚未初始化完成（warmup 進行中）
- 依賴服務暫時不可用

---

### 1.3 Probe 比較表

| 特性 | Liveness (`/livez`) | Readiness (`/readyz`) |
|------|---------------------|------------------------|
| **用途** | 進程是否存活 | 是否準備接流量 |
| **檢查內容** | 僅 HTTP 回應 | DB + 快取 + 功能 |
| **超時** | 無 (立即回傳) | 3 秒 |
| **失敗行為** | 重啟容器 | 移除流量 (不重啟) |
| **Docker Compose** | 用於 healthcheck | 可用於 depends_on (依需求) |

> **說明**:
> - Docker HEALTHCHECK 使用 `/livez` 檢查容器是否存活，不檢查外部依賴（避免因 DB 暫時不可用而重啟容器）
> - 若需在 `depends_on` 中等待服務完全就緒（含 DB 連線），可使用 `/readyz`
> - Kubernetes 環境建議：livenessProbe 使用 `/livez`，readinessProbe 使用 `/readyz`

---

## 2. LINE Webhook 端點

### 2.1 Webhook Callback

接收 LINE Platform 的 Webhook 事件。

```http
POST /webhook
Content-Type: application/json
X-Line-Signature: {signature}
```

**Headers**:
- `X-Line-Signature`: LINE Platform 計算的 HMAC-SHA256 簽章（必填）

**Request Body** (來自 LINE):
```json
{
  "destination": "Uxxxx",
  "events": [
    {
      "type": "message",
      "replyToken": "xxxxx",
      "source": {
        "type": "user",
        "userId": "Uxxxx"
      },
      "timestamp": 1625097600000,
      "message": {
        "type": "text",
        "id": "xxxxx",
        "text": "學號 410123456"
      }
    }
  ]
}
```

**Response** (200 OK):
```json
{
  "status": "ok"
}
```

**Response** (400 Bad Request - 簽章錯誤):
```json
{
  "error": "invalid signature"
}
```

**Response** (413 Payload Too Large - 請求過大):
```json
{
  "error": "request too large"
}
```

**Response** (503 Service Unavailable - 資料來源不可用):
```json
{
  "error": "Service Unavailable"
}
```

**支援的事件類型**:
- `message` - 文字訊息、貼圖
- `postback` - 按鈕點擊回傳
- `follow` - 使用者加入好友

**限制**:
- Request body < 1MB
- 處理超時: 60 秒
- Global rate limit: 100 rps
- Per-user rate limit: 15 tokens, 1 token/10s refill (Token Bucket)
- LLM rate limit: 60 burst, 30/hr refill, 180/day cap

---

## 3. Prometheus 監控端點

### 3.1 Metrics

提供 Prometheus 格式的監控指標。

```http
GET /metrics
```

**Response** (200 OK):
```prometheus
# HELP ntpu_webhook_total Total webhook events processed
# TYPE ntpu_webhook_total counter
ntpu_webhook_total{event_type="message",status="success"} 1234

# HELP ntpu_webhook_duration_seconds Webhook processing duration in seconds
# TYPE ntpu_webhook_duration_seconds histogram
ntpu_webhook_duration_seconds_bucket{event_type="message",le="0.1"} 100
ntpu_webhook_duration_seconds_bucket{event_type="message",le="2"} 900
ntpu_webhook_duration_seconds_sum{event_type="message"} 234.56
ntpu_webhook_duration_seconds_count{event_type="message"} 1000

# ... 更多指標
```

**指標列表**:

採用 RED (Rate, Errors, Duration) 和 USE (Utilization, Saturation, Errors) 方法論設計。

| 指標名稱 | 類型 | 說明 | Labels |
|---------|------|------|--------|
| **Webhook (RED)** | | | |
| `ntpu_webhook_total` | Counter | Webhook 事件總數 | `event_type`, `status` |
| `ntpu_webhook_duration_seconds` | Histogram | Webhook 處理耗時 | `event_type` |
| **Scraper (RED)** | | | |
| `ntpu_scraper_total` | Counter | 爬蟲請求總數 | `module`, `status` |
| `ntpu_scraper_duration_seconds` | Histogram | 爬蟲請求耗時 | `module` |
| **Cache (USE)** | | | |
| `ntpu_cache_operations_total` | Counter | 快取操作總數 | `module`, `result` |
| `ntpu_cache_size` | Gauge | 快取項目數量 | `module` |
| **LLM (RED)** | | | |
| `ntpu_llm_total` | Counter | LLM API 請求總數 | `operation`, `status` |
| `ntpu_llm_duration_seconds` | Histogram | LLM API 請求耗時 | `operation` |
| **Search (RED)** | | | |
| `ntpu_search_total` | Counter | 智慧搜尋請求總數 | `type`, `status` |
| `ntpu_search_duration_seconds` | Histogram | 搜尋耗時 | `component` |
| `ntpu_search_results` | Histogram | 搜尋結果數量分布 | `type` |
| `ntpu_index_size` | Gauge | 索引文件數量 | `index` |
| **Rate Limiter (USE)** | | | |
| `ntpu_rate_limiter_dropped_total` | Counter | 被丟棄的請求數 | `limiter` |
| `ntpu_rate_limiter_users` | Gauge | 活動用戶限流器數量 | - |
| **Background Jobs** | | | |
| `ntpu_job_duration_seconds` | Histogram | 背景任務耗時 | `job`, `module` |

**PromQL 查詢範例**:

```promql
# Webhook 成功率 (使用 recording rule)
ntpu:webhook_success_rate:5m

# P95 延遲 (使用 recording rule)
ntpu:webhook_latency_p95:5m

# 快取命中率 (使用 recording rule)
ntpu:cache_hit_rate:5m

# 每秒請求數 (RPS)
sum(rate(ntpu_webhook_total[5m]))
```

---

## 4. Root 端點

### 4.1 Redirect to GitHub

```http
GET /
```

**Response** (301 Moved Permanently):
```
Location: https://github.com/garyellow/ntpu-linebot-go
```

---

## 業務邏輯

### 課程查詢學期判斷

系統使用**智慧檢測**來判斷要查詢哪 4 個學期的課程：

**檢測策略**：
1. 根據當前月份計算日曆基礎的最新學期
2. 檢查該學期是否有資料（> 0 門課程）
3. 如有資料：使用該學期作為起點，往前推 4 個學期
4. 如無資料：往前推移一個學期，重新生成 4 個學期

**日曆基礎計算**：

| 月份 | 日曆基礎最新學期 | 說明 |
|------|-------------|------|
| 2-8月 | 當年度第2學期 | 下學期進行中或暑假 |
| 9-12月 + 1月 | 次年度第1學期 | 上學期進行中或寒假 |

**學年度定義**：西元年減去 1911 即為學年度（如 2024 年 9 月開始為 113 學年度）

**範例**（假設 N 為當前學年度）：
- 2025/01 → 日曆基礎：114-1，檢測後若有資料 → [114-1, 113-2, 113-1, 112-2]
- 2025/03 → 日曆基礎：113-2，檢測後若有資料 → [113-2, 113-1, 112-2, 112-1]
- 2025/08 → 日曆基礎：113-2，檢測後若有資料 → [113-2, 113-1, 112-2, 112-1]（暑假期間）
- 2025/09 → 日曆基礎：114-1，檢測後若有資料 → [114-1, 113-2, 113-1, 112-2]，若無資料 → [113-2, 113-1, 112-2, 112-1]（延遲容錯）

**優勢**：
- ✅ **延遲容錯**：9月若新學期資料未上傳，自動回退使用上學期資料
- ✅ **穩定查詢**：確保總是查詢有資料的 4 個學期

### 學號查詢年度限制

由於數位學苑 2.0 已於 2024 年停止使用，學號查詢功能僅提供 **94-112 學年度**的完整資料。

**113 學年度**：資料不完整（僅少數手動建立 數位學苑 2.0 帳號的學生），系統允許查詢但會顯示警告。

查詢 114 學年度（含）以後的資料時，系統會回應：
- 圖片：RIP 紀念圖
- 訊息：「數位學苑 2.0 已停止使用，無法取得資料」

---

## LINE Messaging API 互動

### 回覆訊息流程

```mermaid
sequenceDiagram
    participant User
    participant LINE
    participant Bot
    participant NTPU

    User->>LINE: 傳送訊息「學號 410123456」
    LINE->>Bot: POST /webhook (webhook)
    Bot->>Bot: 驗證簽章
    Bot->>Bot: 解析事件
    Bot->>LINE: ShowLoadingAnimation API (...)
    Bot->>Bot: 查詢快取
    alt Cache Miss
        Bot->>NTPU: HTTP GET (爬蟲)
        NTPU-->>Bot: HTML Response
        Bot->>Bot: 解析 HTML
        Bot->>Bot: 儲存快取
    end
    Bot->>LINE: ReplyMessage API (with Quick Reply)
    LINE->>User: 顯示學生資訊 + 快速選項
```

### LINE API 限制

| 項目 | 限制 | 說明 |
|------|------|------|
| Reply Token | 單次使用 | 一個 replyToken 只能回覆一次 |
| 訊息數量 | 5 則 | 每次回覆最多 5 則訊息 |
| Quick Reply | 13 個 | 快速回覆按鈕最多 13 個 |
| 文字長度 | 5000 字元 | 超過會被截斷 |
| Postback Data | 300 bytes | 按鈕回傳資料長度 |
| Carousel Columns | 10 個 | 輪播訊息最多 10 個欄位 |
| API Rate Limit | 100 rps | 全域限制 |

### LINE Bot UX 最佳實踐

本專案遵循 LINE Messaging API 最佳實踐：

1. **Loading Animation (載入動畫)**
   - 在長查詢前顯示「...」動畫
   - 使用 `ShowLoadingAnimation` API
   - 最長顯示 60 秒

2. **Quick Reply (快速回覆)**
   - 在訊息下方提供快速選項
   - 引導使用者下一步操作
   - 減少輸入錯誤

3. **Flex Message (彈性訊息)**
   - 使用卡片式介面呈現資料
   - 支援按鈕、圖片、多欄位佈局
   - 提供更好的視覺體驗

4. **錯誤處理**
   - 隱藏技術細節
   - 提供可操作的選項
   - 友善的錯誤訊息

---

## 錯誤處理

### HTTP 狀態碼

| 狀態碼 | 說明 | 範例 |
|--------|------|------|
| 200 | 成功 | 正常處理 |
| 400 | 錯誤請求 | 簽章驗證失敗 |
| 413 | 請求過大 | Request body > 1MB |
| 429 | 請求過多 | 超過 rate limit |
| 500 | 伺服器錯誤 | 未預期的錯誤 |
| 503 | 服務不可用 | 資料來源無法連線 |

### 錯誤回應格式

```json
{
  "error": "invalid signature"
}
```

---

## 安全性

### 1. Webhook 簽章驗證

LINE Platform 使用 HMAC-SHA256 計算簽章：

```
Signature = Base64(HMAC-SHA256(Channel Secret, Request Body))
```

**驗證流程**:
```go
// LINE SDK 自動處理
cb, err := webhook.ParseRequest(channelSecret, request)
if err == webhook.ErrInvalidSignature {
    // 拒絕請求
}
```

### 2. HTTPS 要求

LINE Webhook **必須**使用 HTTPS：
- 開發測試: 使用 ngrok/cloudflare tunnel
- 線上環境: 使用有效的 SSL 憑證

### 3. Rate Limiting

**Global Level**:
```
100 requests/second (LINE API limit: 100 rps)
```

**Per-User Level (Webhook)**:
```
15 tokens burst, 1 token per 10 seconds refill (Token Bucket)
```

**Per-User Level (LLM - Multi-Layer)**:
```
60 burst, 30/hr refill, 180/day sliding window cap
```

超過一般限制時請求會被靜默丟棄（群組）或回覆提示訊息（個人）。
超過 LLM 限制時會回覆提示訊息，引導使用者使用關鍵字查詢。

---

## 範例請求

### cURL 範例

#### 1. Liveness Check
```bash
curl -X GET http://localhost:10000/livez
```

#### 2. Readiness Check
```bash
curl -X GET http://localhost:10000/readyz
```

#### 3. Prometheus Metrics
```bash
curl -X GET http://localhost:10000/metrics
```

#### 4. 模擬 LINE Webhook (需要簽章)
```bash
# 計算簽章
echo -n '{"events":[{"type":"message","message":{"text":"test"}}]}' | \
openssl dgst -sha256 -hmac "YOUR_CHANNEL_SECRET" -binary | base64

# 發送請求
curl -X POST http://localhost:10000/webhook \
  -H "Content-Type: application/json" \
  -H "X-Line-Signature: {calculated_signature}" \
  -d '{"events":[{"type":"message","message":{"text":"test"}}]}'
```

---

## Postman Collection

匯入以下 JSON 到 Postman:

```json
{
  "info": {
    "name": "NTPU LineBot API",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "item": [
    {
      "name": "Liveness Check",
      "request": {
        "method": "GET",
        "url": "{{base_url}}/livez"
      }
    },
    {
      "name": "Readiness Check",
      "request": {
        "method": "GET",
        "url": "{{base_url}}/readyz"
      }
    },
    {
      "name": "Prometheus Metrics",
      "request": {
        "method": "GET",
        "url": "{{base_url}}/metrics"
      }
    }
  ],
  "variable": [
    {
      "key": "base_url",
      "value": "http://localhost:10000"
    }
  ]
}
```

---

## 附錄

### A. LINE Messaging API 參考

- [Official Documentation](https://developers.line.biz/en/docs/messaging-api/)
- [Webhook Events](https://developers.line.biz/en/reference/messaging-api/#webhook-event-objects)
- [Message Types](https://developers.line.biz/en/reference/messaging-api/#message-objects)

### B. Prometheus 監控

- [Prometheus Query Examples](https://prometheus.io/docs/prometheus/latest/querying/examples/)
- [Grafana Dashboard](http://localhost:3000)

### C. 聯絡方式

- **GitHub Issues**: https://github.com/garyellow/ntpu-linebot-go/issues
- **維護者**: garyellow
