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

### Docker Compose 設定

- `NTPU_IMAGE_TAG` - 映像版本（預設：latest）
- `NTPU_HOST_PORT` - Host 對外埠號（預設：10000）

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
