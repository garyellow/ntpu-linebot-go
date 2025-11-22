# Bot Modules

處理 LINE 訊息事件的核心模組。

## 現有模組

- **id/** - 學號查詢、系所代碼
- **contact/** - 聯絡資訊、緊急電話
- **course/** - 課程查詢、教師課表

## 開發規範

- 使用 `lineutil` 建構訊息（不直接使用 LINE SDK）
- Context timeout: 25 秒（LINE webhook 限制）
- 每次最多 5 則訊息（LINE API 限制）
- Table-driven tests

詳細實作請參考 `.github/copilot-instructions.md`。
