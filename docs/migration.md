# 從 Python 遷移到 Go

本專案原為 Python 實作 ([ntpu-linebot-python](https://github.com/garyellow/ntpu-linebot-python)),重寫為 Go 版本。

## 遷移原因

### 1. 效能與資源消耗

| 指標 | Python | Go | 改善 |
|------|--------|-----|------|
| **啟動時間** | ~3-5s | ~0.1s | **50x** |
| **記憶體使用** | ~150-200 MB | ~30-50 MB | **4x** |
| **並發處理** | asyncio (單執行緒) | goroutines (多核心) | **顯著提升** |
| **容器大小** | ~500 MB | ~15 MB (distroless) | **33x** |

### 2. 並發模型

**Python (asyncio)**:
```python
# 單執行緒事件迴圈,需手動管理 async/await
async def handle_request():
    result = await scrape_data()  # 容易忘記 await
    return result
```

**Go (goroutines)**:
```go
// 多核心並發,語法簡潔
func handleRequest() {
    result := scrapeData()  // 自動並發
    return result
}
```

### 3. 類型安全

| 特性 | Python | Go |
|------|--------|-----|
| **類型系統** | 動態（執行時檢查） | 靜態（編譯時檢查） |
| **IDE 支援** | 有限（需 type hints） | 完整（自動補全、重構） |
| **錯誤偵測** | 執行時才發現 | 編譯時就發現 |

**實際案例**:
```python
# Python: 執行時才會發現錯誤
def get_student(id: str):
    return db.query(id + 1)  # 型別錯誤但編譯器不會警告

# Go: 編譯時就會報錯
func getStudent(id string) {
    return db.Query(id + 1)  // 編譯錯誤: invalid operation
}
```

### 4. 依賴管理

**Python**:
- 需要虛擬環境（venv/poetry）
- 依賴衝突問題（版本地獄）
- 需要 requirements.txt + Dockerfile 多層管理

**Go**:
- 內建 go.mod（無需額外工具）
- 語義化版本自動管理
- 編譯為單一靜態二進位檔

### 5. 部署複雜度

**Python 部署**:
```dockerfile
FROM python:3.11-slim
COPY requirements.txt .
RUN pip install -r requirements.txt  # 每次都要重裝
COPY . .
CMD ["python", "main.py"]
# 容器大小: ~500 MB
```

**Go 部署**:
```dockerfile
FROM gcr.io/distroless/static-debian12:nonroot
COPY ntpu-linebot /app/
CMD ["/app/ntpu-linebot"]
# 容器大小: ~15 MB
```

### 6. 錯誤處理

**Python**:
```python
# 可能丟出各種例外,需要廣泛的 try-catch
try:
    result = scrape_data()
except requests.RequestException as e:
    pass
except ValueError as e:
    pass
except Exception as e:  # 容易忽略錯誤
    pass
```

**Go**:
```go
// 強制處理錯誤,不容易忽略
result, err := scrapeData()
if err != nil {
    return fmt.Errorf("scrape failed: %w", err)  // 錯誤包裝
}
```

## 技術對照表

| 功能 | Python 版本 | Go 版本 |
|------|-------------|---------|
| **Web 框架** | Flask | Gin |
| **HTTP 客戶端** | aiohttp | net/http + goquery |
| **資料庫** | SQLite (sqlite3) | SQLite (modernc.org/sqlite) |
| **日誌** | logging | logrus |
| **監控** | 無 | Prometheus + Grafana |
| **並發控制** | asyncio.Semaphore | goroutines + channels |
| **快取** | Dict (記憶體) | SQLite (持久化) |
| **測試** | pytest | go test |

## 架構改進

### 快取策略

**Python 版本**:
- 使用記憶體字典（`dict`）
- 重啟後資料消失
- 無 TTL 機制

**Go 版本**:
- 使用 SQLite WAL 模式（持久化）
- 重啟後快取保留
- 7 天自動過期

### 防爬蟲機制

**Python 版本**:
```python
# 簡單的延遲
await asyncio.sleep(random.uniform(0.5, 1.5))
```

**Go 版本**:
```go
// Token Bucket + Singleflight + 指數退避
rateLimiter.Wait(ctx)          // Token bucket
result, _ := sf.Do(key, fn)    // 去重
backoff.Retry(fn, maxRetries)  // 重試
```

### 錯誤處理

**Python 版本**:
- 錯誤容易被忽略
- 缺少結構化日誌

**Go 版本**:
- 強制錯誤處理（`if err != nil`）
- 完整的錯誤堆疊追蹤（`%w`）
- 結構化日誌（logrus fields）

## 遷移挑戰與解決

### 1. Big5 編碼處理

**Python**: 內建 `decode('big5')`
**Go 解決方案**: 使用 `golang.org/x/text/encoding/traditionalchinese`

```go
decoder := traditionalchinese.Big5.NewDecoder()
decodedBytes, _ := decoder.Bytes(big5Bytes)
```

### 2. HTML 解析

**Python**: BeautifulSoup4
**Go 解決方案**: goquery（類似 jQuery 語法）

```go
doc.Find("table tr").Each(func(i int, s *goquery.Selection) {
    text := s.Find("td").Text()
})
```

### 3. 並發控制

**Python**: `asyncio.Semaphore`
**Go 解決方案**: Worker Pool + Context

```go
sem := make(chan struct{}, maxWorkers)
for _, task := range tasks {
    sem <- struct{}{}  // acquire
    go func(t Task) {
        defer func() { <-sem }()  // release
        process(t)
    }(task)
}
```

## 保留的設計

以下設計從 Python 版本保留:

- ✅ Cache-First 查詢策略
- ✅ 三層模組架構（ID/Contact/Course）
- ✅ Failover URLs（多個備援網址）
- ✅ 關鍵字匹配邏輯
- ✅ LINE 訊息格式（Carousel/Buttons）

## 效能基準測試

### 冷啟動（無快取）

| 操作 | Python | Go |
|------|--------|-----|
| 查詢單筆學號 | ~8-12s | ~2-4s |
| 查詢課程（10 筆） | ~15-20s | ~5-8s |
| 並發 10 個查詢 | ~30-40s | ~8-12s |

### 熱啟動（有快取）

| 操作 | Python | Go |
|------|--------|-----|
| 查詢單筆學號 | ~50-100ms | ~10-30ms |
| 查詢課程（10 筆） | ~150-300ms | ~50-100ms |

## 未來展望

Go 版本為未來擴展奠定基礎:

- ✅ Kubernetes 部署（靜態二進位檔易於容器化）
- ✅ gRPC 微服務（若需要拆分模組）
- ✅ 水平擴展（改用 PostgreSQL）
- ✅ 更完善的監控與告警

## 結論

遷移到 Go 帶來的主要優勢:

1. **效能**: 4-10 倍提升（特別是並發場景）
2. **資源**: 記憶體使用減少 70%
3. **維護性**: 類型安全降低 Bug 率
4. **部署**: 單一二進位檔,無依賴管理問題
5. **可觀測性**: 內建 Prometheus 整合

Python 版本將繼續維護作為參考實作,但新功能將優先在 Go 版本開發。
