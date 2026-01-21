# Deployments

Docker Compose 部署配置。

## 快速開始

```bash
cd deployments
cp .env.example .env
# 編輯 .env 填入 LINE 憑證和可選的 LLM API Key
docker compose up -d
```

## 檔案說明

- **compose.yml** - Docker Compose 配置
- **.env.example** - 環境變數範本（完整說明請見檔案內註解）

## 環境變數

### 必填項目

- `NTPU_LINE_CHANNEL_ACCESS_TOKEN` - LINE Channel Access Token
- `NTPU_LINE_CHANNEL_SECRET` - LINE Channel Secret

### 可選項目（啟用 AI 功能）

需先設定 `NTPU_LLM_ENABLED=true`，並至少設定一個 LLM Provider API Key：
- `NTPU_GEMINI_API_KEY` - Google Gemini API
- `NTPU_GROQ_API_KEY` - Groq API
- `NTPU_CEREBRAS_API_KEY` - Cerebras API

### 可選項目（R2 快照同步）

多節點部署建議啟用 `NTPU_R2_ENABLED=true`，啟動時會自動下載最新 SQLite 快照並進行同步：

- `NTPU_R2_ACCOUNT_ID` - Cloudflare Account ID
- `NTPU_R2_ACCESS_KEY_ID` - R2 Access Key ID
- `NTPU_R2_SECRET_ACCESS_KEY` - R2 Secret Access Key
- `NTPU_R2_BUCKET_NAME` - R2 Bucket
- `NTPU_R2_SNAPSHOT_KEY` - 快照物件 key（預設：snapshots/cache.db.zst）
- `NTPU_R2_LOCK_KEY` - 分散式鎖 key（預設：locks/leader.json）
- `NTPU_R2_LOCK_TTL` - 鎖 TTL（預設：1h）
- `NTPU_R2_SNAPSHOT_POLL_INTERVAL` - follower 輪詢快照更新間隔（預設：15m）
- `NTPU_R2_DELTA_PREFIX` - cache miss delta log 前綴（預設：deltas）
- `NTPU_R2_SCHEDULE_KEY` - 刷新/清理排程狀態 key（預設：schedules/maintenance.json）

### 可選項目（背景任務排程）

- `NTPU_MAINTENANCE_REFRESH_INTERVAL` - 資料刷新任務間隔（預設：24h）
- `NTPU_MAINTENANCE_CLEANUP_INTERVAL` - 資料清理任務間隔（預設：24h）
- `NTPU_WARMUP_WAIT` - 是否等待首次 warmup 完成才接受 webhook（預設：false）
- `NTPU_WARMUP_GRACE_PERIOD` - Warmup 就緒寬限期（預設：10m）

> 未啟用 R2 時，排程狀態不會共享，僅在單一節點本地執行。

### Docker Compose 設定

- `NTPU_IMAGE_TAG` - 映像版本（預設：latest）
- `NTPU_HOST_PORT` - Host 對外埠號（預設：10000）

### 可選項目（多點部署識別）

- `NTPU_SERVER_NAME` - 覆蓋 server 名稱，用於 log 與 Sentry 分辨節點
- `NTPU_INSTANCE_ID` - 覆蓋 instance id，用於 log/metrics 分辨容器或 Pod

未設定時會自動嘗試取得（依序）：

- `server_name`：`NODE_NAME` / `K8S_NODE_NAME` / `KUBE_NODE_NAME` / `MY_NODE_NAME` → hostname →（最後）`instance_id`
- `instance_id`：`POD_UID` / `MY_POD_UID` / `POD_NAME` / `MY_POD_NAME` / `HOSTNAME` → hostname →（最後）`server_name`

詳細環境變數說明請參考 [.env.example](.env.example)。

## 服務端點

| 端點 | 說明 |
|------|------|
| `http://localhost:10000/webhook` | LINE Webhook URL |
| `http://localhost:10000/livez` | Liveness probe |
| `http://localhost:10000/readyz` | Readiness probe |
| `http://localhost:10000/metrics` | Prometheus metrics |

## 更多資訊

- 完整部署說明：[根目錄 README.md](../README.md#%EF%B8%8F-自架部署)
- API 文件：[docs/API.md](../docs/API.md)
- 架構設計：[docs/architecture.md](../docs/architecture.md)
