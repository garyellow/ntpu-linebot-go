# cmd/

應用程式進入點。

- **server/** - 主服務（LINE Bot Webhook，含自動 warmup）
- **warmup/** - 手動快取工具（選用，詳見 [warmup/README.md](warmup/README.md)）
- **healthcheck/** - 健康檢查（Docker 容器專用）

## 快速啟動

```bash
# 使用 Task（推薦）
task dev              # 啟動開發服務
task warmup           # 手動預熱（選用）

# 或直接執行
go run ./cmd/server         # 主服務
go run ./cmd/warmup -reset  # 手動預熱（選用）
```
