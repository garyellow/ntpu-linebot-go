# cmd/

應用程式進入點。

- **server/** - 主服務（LINE Bot Webhook）
- **warmup/** - 快取預熱工具（詳見 [warmup/README.md](warmup/README.md)）
- **healthcheck/** - 健康檢查（Docker 容器專用）

## 快速啟動

```bash
# 使用 Task (推薦)
task dev     # 啟動開發服務
task warmup  # 預熱快取

# 或直接執行
go run ./cmd/server
go run ./cmd/warmup
```
