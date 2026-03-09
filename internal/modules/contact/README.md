# Contact Module

聯絡資訊查詢模組 - 提供校內單位和個人聯絡方式查詢，包括電話、分機、Email、緊急聯絡電話等。

## 功能特性

### 支援的查詢方式

#### 1. **單位/人員搜尋**
- **關鍵字**：
  - 聯絡：`聯絡 [名稱]` / `contact [name]`
  - 電話：`電話 [名稱]` / `phone [name]`
  - 分機：`分機 [名稱]` / `ext [name]`
  - Email：`信箱 [名稱]` / `email [name]` / `mail [name]`
- **搜尋策略**：2-tier SQL search
  - SQL LIKE：name, title 欄位
  - SQL Fuzzy：name, title, organization, superior 欄位
- **記憶體效率**：SQL-level 字元匹配，不載入全表

#### 2. **緊急聯絡電話**
- **關鍵字**：`緊急` / `emergency` / `urgent` / `911`
- **內容**：
  - 三峽校區保全室
  - 台北校區保全室
  - 校安中心
- **顯示**：紅色 Flex Message（警示效果）

#### 3. **NLU 自然語言查詢**（需要 LLM API Key）
- **Intent Functions**：
  - `contact_search` - 搜尋單位/人員
  - `contact_emergency` - 緊急電話
- **範例**：「資工系的電話」、「圖書館怎麼聯絡」、「緊急電話」

## 架構設計

### Handler 結構

```go
type Handler struct {
    db               *storage.DB
    scraper          *scraper.Client
    metrics          *metrics.Metrics
    logger           *logger.Logger
    stickerManager   *sticker.Manager
    maxContactsLimit int  // 最大結果數限制（預設 100）    orgCache         *ContactOrgCache  // 短 TTL 快取：單位成員列表}
```

### 搜尋策略

採用 **2-Tier SQL Search** 策略，在資料庫層級完成所有篩選，避免載入全表資料：

1. **Tier 1 - 精確匹配**：使用 SQL LIKE 查詢姓名和職稱
2. **Tier 2 - 模糊匹配**：使用 `SearchContactsFuzzy()` 進行字元集合匹配
   - 支援非連續字元匹配（例如：「王明」可匹配「王小明」）
   - 所有篩選都在 SQL 層級執行，確保記憶體效率

詳細實作請參考 `internal/storage/repository.go` 中的查詢方法。

#### 排序邏輯

```go
// 組織單位：依階層排序
Organizations by hierarchy (superior → subordinate)

// 個人聯絡：依匹配度排序
Individuals by match count (more matches first)
```

## 資料模型

### Contact 結構
```go
type Contact struct {
    UID          string  // 唯一識別碼
    Type         string  // "organization" / "individual"
    Name         string  // 名稱/姓名
    Organization string  // 所屬單位
    Title        string  // 職稱
    Phone        string  // 電話
    Extension    string  // 分機
    Email        string  // Email
    Superior     string  // 上級單位（組織階層）
    CachedAt     int64   // 快取時間
}
```

### 資料時效策略

> 完整的資料時效策略說明請參考 [架構說明文件](/.github/copilot-instructions.md#data-layer-cache-first-strategy)

- **TTL**：7 天（依 `NTPU_MAINTENANCE_REFRESH_INTERVAL` 自動更新）
- **來源**：NTPU 通訊錄系統

## Flex Message 設計

### 聯絡人輪播（Contact Carousel）
- **Colored Header**：
  - 藍色（🏢）：組織單位
  - 青色（👤）：個人聯絡
- **Body**：
  - 第一列：`NewBodyLabel()` 類型標籤（文字色與 header 一致）
  - 聯絡資訊：職稱、單位、電話/分機、Email
- **Footer**：
  - 組織：「成員列表」按鈕（Postback）
  - 個人：「撥打電話」按鈕（URI action）

### 聯絡人詳情（Contact Detail）
- **Colored Header**（青色）：聯絡人姓名
- **Body**：
  - 第一列：類型標籤（🏢 組織單位 / 👤 個人聯絡）
  - 完整資訊：所有欄位展開顯示
- **Footer**：
  - 撥打電話按鈕（綠色，action button）
  - 寄送郵件按鈕（藍色，external link）

### 緊急聯絡（Emergency）
- **Colored Header**（紅色）：🚨 緊急聯絡電話
- **Body**：
  - 第一列：☎️ 校園緊急聯絡（紅色標籤）
  - 三個校區保全/校安電話
- **Footer**：
  - 每個電話一個「立即撥打」按鈕（紅色，危險操作）

### Quick Reply
- 使用 `QuickReplyContactNav()`
- 包含：📞 聯絡、🚨 緊急、📖 說明

## 搜尋流程

```
User Input: "電話 資工系"
    ↓
Extract keyword: "資工系"
    ↓
┌─ Tier 1: SQL LIKE ─────────┐
│ name LIKE '%資工系%'        │
│ title LIKE '%資工系%'       │
└────────────┬────────────────┘
             ↓ (if < limit)
┌─ Tier 2: SQL Fuzzy ────────┐
│ ContainsAllRunes("資工系") │
│ Match: name, title,        │
│        organization,       │
│        superior            │
└────────────┬────────────────┘
             ↓
Sort & Group (org > individual)
    ↓
Build Contact Carousel
    ↓ (if > maxContacts)
Truncate + Warning Message
```

## 多語言支援

### 關鍵字（中英文）
```go
validContactKeywords = []string{
    // 中文
    "聯絡", "電話", "分機", "信箱", "聯繫",
    // 英文
    "contact", "phone", "tel", "ext", "extension",
    "email", "mail",
}
```

### 緊急關鍵字
```go
emergencyKeywords = []string{
    // 中文
    "緊急", "保全",
    // 英文
    "emergency", "urgent", "security", "guard",
    // 通用
    "911", "119", "110",
}
```

## Postback 處理

### 成員列表（View Members）
- **Postback**：`contact:members$[組織 UID]`
- **處理**：
  ```go
  GetContactsByOrganization(organization_name)
      ↓
  Build member list carousel
  ```

### 查詢個人（Query by UID）
- **Postback**：`contact:[UID]`
- **處理**：
  ```go
  GetContactByUID(uid)
      ↓
  Build detail Flex Message
  ```

## 測試覆蓋

### 單元測試
- Keyword matching 測試
- Search tier 測試
- Postback parsing 測試
- Emergency phone 測試

### 整合測試（`-short` flag 跳過）
- Database queries
- Scraper integration

## 效能考量

### 搜尋優化
- **SQL 索引**：name, organization, type
- **2-tier strategy**：逐層過濾，避免全表掃描
- **結果限制**：maxContactsLimit（預設 100）

### Memory 使用
- **No full-table load**：僅載入匹配結果
- **String matching at SQL level**：減少記憶體消耗
- **`ContactOrgCache`**：短 TTL（30s）記憶體快取，降低相同單位成員列表重複 DB 讀取

## 限制與注意事項

### 資料來源
- **通訊錄系統**：可能不完整或過時
- **更新頻率**：依 `NTPU_MAINTENANCE_REFRESH_INTERVAL`
- **資料品質**：取決於學校維護狀況

### 搜尋限制
- **最大結果**：maxContactsLimit（避免訊息過載）
- **模糊搜尋**：字元集合匹配（可能誤判）
- **排序邏輯**：組織優先於個人

### 隱私考量
- **公開資訊**：僅顯示學校公開的聯絡資訊
- **敏感資訊**：不存儲個人隱私資料
- **存取控制**：無額外權限檢查（公開資料）

## 相關文件
- Handler: `internal/modules/contact/handler.go`
- Tests: `internal/modules/contact/handler_test.go`
- Storage: `internal/storage/contact.go`
- Scraper: `internal/scraper/ntpu/contact.go`

## 依賴關係
- `storage.DB` - 聯絡資料查詢
- `scraper.Client` - 即時抓取（fallback）
- `ContactOrgCache` - 短 TTL 快取（模組內部）
- `metrics.Metrics` - 監控指標
- `logger.Logger` - 日誌記錄
- `sticker.Manager` - Sender 頭像
