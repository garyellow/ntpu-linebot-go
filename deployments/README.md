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
- **.env.example** - 部署範本；完整設定說明請見 [docs/configuration.md](../docs/configuration.md)

## 環境變數

### 必填項目

- `NTPU_LINE_CHANNEL_ACCESS_TOKEN` - LINE Channel Access Token
- `NTPU_LINE_CHANNEL_SECRET` - LINE Channel Secret

### 可選項目（啟用 AI 功能）

需先設定 `NTPU_LLM_ENABLED=true`，並至少設定一個 LLM Provider API Key：
- `NTPU_GEMINI_API_KEY` - Google Gemini API
- `NTPU_GROQ_API_KEY` - Groq API
- `NTPU_CEREBRAS_API_KEY` - Cerebras API
- `NTPU_OPENAI_API_KEY` + `NTPU_OPENAI_ENDPOINT` - 任意 OpenAI-compatible endpoint

完整模型設定與 Provider 選項請見 [docs/configuration.md](../docs/configuration.md#llm--ai-features-optional)。

### 可選項目（S3-compatible 快照同步）

多節點部署建議啟用 `NTPU_S3_ENABLED=true`，啟動時會自動下載最新 SQLite 快照並進行同步。端點可以是 AWS S3、MinIO、Ceph RGW，或其他支援必要 S3 API 的物件儲存。完整設定清單與說明請見 [docs/configuration.md](../docs/configuration.md#s3-compatible-snapshot-sync-optional)。

同步流程把 refresh/cleanup 視為需要互斥的工作：leader lease lock 會避免多個節點同時執行全量爬蟲，且 lease 續約失敗時會取消正在執行的工作。使用者 cache miss 不會等待全域鎖，任一節點可小範圍即時補資料，結果會寫入 append-only delta log，並在下一次 leader snapshot 收斂。底層使用標準 HTTP/S3 條件請求作為 CAS primitive：快照上傳與共享排程狀態會用 `PutObject` + `If-Match` / `If-None-Match` 防止 lost update；已合併 delta log 會保留到新快照成功上傳後才刪除，避免部署中斷時遺失資料。所選 S3-compatible 服務必須支援 `HeadObject`、`GetObject`、`ListObjectsV2`、`DeleteObject`，以及 `PutObject` 條件寫入。

### 可選項目（背景任務排程）

啟用 S3 時，刷新/清理排程狀態會透過 `NTPU_S3_SCHEDULE_KEY` 共享，避免多節點重複執行；未啟用時僅在本地執行。完整設定請見 [docs/configuration.md](../docs/configuration.md#background-jobs)。

### Docker Compose 設定

- `NTPU_DOCKER_IMAGE` - 完整 Docker 映像名稱，包含 registry、repository 和 tag（預設：garyellow/ntpu-linebot-go:latest）
- `NTPU_HOST_PORT` - Host 對外埠號（預設：10000）

### 可選項目（多點部署識別）

- `NTPU_SERVER_NAME` - 覆蓋 server 名稱，用於 log 與 Sentry 分辨節點
- `NTPU_INSTANCE_ID` - 覆蓋 instance id，用於 log/metrics 分辨容器或 Pod

未設定時會自動嘗試取得（依序）：

- `server_name`：`NODE_NAME` / `K8S_NODE_NAME` / `KUBE_NODE_NAME` / `MY_NODE_NAME` → hostname →（最後）`instance_id`
- `instance_id`：`POD_UID` / `MY_POD_UID` / `POD_NAME` / `MY_POD_NAME` / `HOSTNAME` → hostname →（最後）`server_name`

完整環境變數說明請參考 [docs/configuration.md](../docs/configuration.md)。

## 服務端點

| 端點 | 說明 |
|------|------|
| `http://localhost:10000/webhook` | LINE Webhook URL |
| `http://localhost:10000/livez` | Liveness probe |
| `http://localhost:10000/readyz` | Readiness probe |
| `http://localhost:10000/metrics` | Prometheus metrics |

## 更多資訊

- 完整部署說明：[根目錄 README.md](../README.md#%EF%B8%8F-自架部署)
- 環境變數參考：[docs/configuration.md](../docs/configuration.md)
- API 文件：[docs/API.md](../docs/API.md)
- 架構設計：[docs/architecture.md](../docs/architecture.md)
