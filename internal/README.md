# internal/

內部套件目錄（不對外開放）。詳細說明請參考各子目錄的 README.md。

## 目錄結構

```
internal/
├── bot/         - Bot 功能模組 (id/contact/course)
├── config/      - 設定管理
├── logger/      - 結構化日誌
├── metrics/     - Prometheus 指標
├── scraper/     - 爬蟲系統
├── storage/     - SQLite 資料層
├── sticker/     - 貼圖管理
├── webhook/     - LINE Webhook 處理
└── lineutil/    - LINE 訊息工具
```

## 快速參考

- **新增 Bot 模組**: 參考 [bot/README.md](bot/README.md)
- **爬蟲開發**: 參考 [scraper/README.md](scraper/README.md)
- **資料庫操作**: 參考 [storage/README.md](storage/README.md)
