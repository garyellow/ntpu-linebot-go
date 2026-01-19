# Bot Modules

功能模組實作，每個模組處理特定類型的查詢。

## 模組列表

| 模組 | 關鍵字 | 主要功能 | 文件 |
|------|--------|----------|------|
| **Course** | `課程`, `找課` | 課程查詢（精確/智慧搜尋） | [README](course/README.md) |
| **ID** | `學號`, `學生` | 學號查詢、系所查詢 | [README](id/README.md) |
| **Contact** | `聯絡`, `緊急` | 通訊錄、緊急電話 | [README](contact/README.md) |
| **Program** | `學程` | 學程查詢、學程課程 | [README](program/README.md) |
| **Usage** | `配額`, `額度` | 使用額度查詢 | [README](usage/README.md) |

## 共同特性

### 2-Tier Search 策略

所有搜尋模組使用「SQL LIKE + 模糊匹配」並行搜尋：

```
┌─────────────────┐    ┌─────────────────────┐
│  SQL LIKE       │    │  Fuzzy Search       │
│  (連續字元)      │ +  │  (字元集合匹配)      │
│  "線性代數"      │    │  "線代" → "線性代數" │
└────────┬────────┘    └──────────┬──────────┘
         │                        │
         └───────────┬────────────┘
                     ▼
            合併結果 + 去重
```

### NLU 支援

所有模組可選實作 `DispatchIntent()` 支援自然語言查詢（需 LLM API Key）。

### Flex Message 設計

統一使用 Colored Header 模式：
- **Header**：模組特定色（課程藍、學生紫、聯絡青、學程藍、配額天空藍）
- **Body**：第一列為類型標籤（文字色與 header 一致）
- **Footer**：操作按鈕（顏色與 header 同步）

## 開發指南

新增模組請參考 [../bot/README.md](../bot/README.md)。
