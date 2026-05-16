# Documentation Index

NTPU LineBot Go 專案文件導覽。

## 📚 主要文件

### 使用者導向
- [README.md](../README.md) - 專案說明、使用教學、部署指南

### 開發者導向
- [architecture.md](architecture.md) - 系統架構設計
- [API.md](API.md) - HTTP API 端點說明

## 🗂️ 模組文件

### 核心架構
- [bot/](../internal/bot/README.md) - Bot 核心（訊息處理、模組註冊）
- [modules/](../internal/modules/README.md) - 功能模組總覽

### 功能模組
- [course/](../internal/modules/course/README.md) - 課程查詢（精確/智慧搜尋）
- [id/](../internal/modules/id/README.md) - 學號查詢
- [contact/](../internal/modules/contact/README.md) - 通訊錄查詢
- [program/](../internal/modules/program/README.md) - 學程查詢
- [usage/](../internal/modules/usage/README.md) - 配額查詢

### AI 功能
- [genai/](../internal/genai/README.md) - NLU 意圖解析 & Query Expansion
- [rag/](../internal/rag/README.md) - BM25 智慧搜尋

## 🔧 部署與維護

- [deployments/](../deployments/README.md) - Docker Compose 部署
- [workflows/](../.github/workflows/README.md) - GitHub Actions CI/CD
- [architecture.md](architecture.md#3-s3-compatible-%E5%BF%AB%E7%85%A7%E5%90%8C%E6%AD%A5%EF%BC%88%E5%8F%AF%E9%81%B8%EF%BC%89) - S3-compatible 快照同步（可選）

## 📖 快速導航

### 我想要...

| 需求 | 參考文件 |
|------|----------|
| **使用 Bot** | [README.md](../README.md#-立即使用) |
| **自行部署** | [README.md](../README.md#%EF%B8%8F-自架部署) |
| **了解架構** | [architecture.md](architecture.md) |
| **開發新功能** | [bot/README.md](../internal/bot/README.md#開發指南) |
| **設定 NLU** | [genai/README.md](../internal/genai/README.md#配置) |
| **查看 API** | [API.md](API.md) |
| **修改 CI/CD** | [workflows/README.md](../.github/workflows/README.md) |

## 📝 文件規範

### 結構原則
1. **根目錄 README** - 使用者導向（功能介紹、使用教學）
2. **docs/** - 架構設計、API 文件
3. **內部模組 README** - 開發者導向（實作細節、設計決策）

### 避免重複
- 模組概覽放在 `modules/README.md`
- 模組詳細說明放在各自的 README
- 架構設計放在 `docs/architecture.md`
- API 規格放在 `docs/API.md`

### 跨文件連結
使用相對路徑連結相關文件，方便讀者導航。
