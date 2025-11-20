# docker/

Docker 部署設定。

## docker-compose.yml

完整的服務堆疊，包含：
- **warmup** - 資料預熱容器（啟動時執行一次）
- **ntpu-linebot** - 主服務
- **prometheus** - 監控指標收集
- **grafana** - 視覺化儀表板

## 快速啟動

```bash
# 進入 docker 目錄
cd docker

# 建立 .env 檔案（或複製 .env.example）
echo "LINE_CHANNEL_ACCESS_TOKEN=your_token" > .env
echo "LINE_CHANNEL_SECRET=your_secret" >> .env

# 啟動所有服務
docker-compose up -d

# 查看日誌
docker-compose logs -f ntpu-linebot

# 停止服務
docker-compose down
```

## 使用 Task 指令（推薦）

```bash
# 在專案根目錄執行
task compose:up     # 啟動
task compose:logs   # 查看日誌
task compose:down   # 停止
task compose:ps     # 查看狀態
```

## 資料持久化

- SQLite 資料庫: `./data/cache.db`
- Prometheus 資料: Docker volume `prometheus-data`
- Grafana 資料: Docker volume `grafana-data`

## 更新映像檔

```bash
# 拉取最新映像檔
docker-compose pull

# 重啟服務
docker-compose up -d
```

或使用提供的腳本：
```bash
./update.sh
```
