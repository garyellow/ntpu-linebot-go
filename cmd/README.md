# cmd/

應用程式進入點。

- **server/** - 主服務（LINE Bot）
- **warmup/** - 資料預熱工具
- **healthcheck/** - 健康檢查（Docker 容器使用）

## 快速啟動

```bash
# 主服務
go run ./cmd/server

# 預熱快取
go run ./cmd/warmup -modules=id,contact,course

# 使用 Task (推薦)
task dev     # 開發模式
task warmup  # 預熱快取
```
