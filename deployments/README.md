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

- `LINE_CHANNEL_ACCESS_TOKEN` - LINE Channel Access Token
- `LINE_CHANNEL_SECRET` - LINE Channel Secret

### 可選項目（啟用 AI 功能）

至少設定一個 LLM Provider API Key：
- `GEMINI_API_KEY` - Google Gemini API
- `GROQ_API_KEY` - Groq API
- `CEREBRAS_API_KEY` - Cerebras API

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
