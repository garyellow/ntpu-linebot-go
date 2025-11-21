# docker/

## 快速啟動

```bash
cd docker
echo "LINE_CHANNEL_ACCESS_TOKEN=your_token" > .env
echo "LINE_CHANNEL_SECRET=your_secret" >> .env
docker compose up -d
```

## 服務

- **init-data** - 初始化資料目錄權限
- **warmup** - 預熱快取（執行一次）
- **ntpu-linebot** - 主服務
- **prometheus** - 監控 (http://localhost:9090)
- **alertmanager** - 告警 (http://localhost:9093)
- **grafana** - 儀表板 (http://localhost:3000, admin/admin123)

## 環境變數

必填：
- `LINE_CHANNEL_ACCESS_TOKEN`
- `LINE_CHANNEL_SECRET`

可選：
- `IMAGE_TAG` - 映像版本（預設：latest）
- `WARMUP_MODULES` - 預熱模組（預設：id,contact,course，空字串跳過）
- `LOG_LEVEL` - 日誌層級（預設：info）
- `GRAFANA_PASSWORD` - Grafana 密碼（預設：admin123）

## 常用指令

```bash
task compose:up                      # 啟動
task compose:down                    # 停止
task compose:logs                    # 查看所有日誌
task compose:logs -- ntpu-linebot    # 查看特定服務日誌
task compose:restart -- ntpu-linebot # 重啟服務
```

## 疑難排解

權限錯誤：
```bash
docker compose down
rm -rf ./data
docker compose up -d
```

更新映像：
```bash
docker compose pull && docker compose up -d
```
