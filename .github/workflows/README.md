# GitHub Actions Workflows

符合 Go + GitHub Actions 最佳實踐的優化工作流程。

## 工作流程說明

### 🧪 CI (`ci.yml`)
**觸發時機**: Push 到非 main 分支、Pull Request、手動觸發

**功能**:
- ✅ 使用 `go-version-file: go.mod` 自動讀取 Go 版本
- ✅ 內建 Go cache（比手動 `actions/cache` 更快）
- ✅ 測試 + 覆蓋率顯示（不上傳到第三方）
- ✅ golangci-lint 代碼檢查
- ✅ govulncheck 漏洞掃描
- ✅ Docker 構建 + Trivy 安全掃描（僅 PR）
- ✅ Trivy 掃描直接使用 metadata 產出的 `pr-{number}` 標籤，避免標籤與映像不同步
- ✅ 使用 PR 編號標籤 (`pr-123`)，避免分支名稱特殊字符問題

**Cache 策略**:
- Go modules 和 build cache 由 `setup-go@v6` 自動處理
- Docker 使用 `type=gha` cache，範圍限定在 branch

---

### 🚀 Release (`release.yml`)
**觸發時機**:
- Push 到 main 分支（僅代碼變更）
- Push 版本標籤 (`v[0-9]+.[0-9]+.[0-9]+`)

**功能**:
- ✅ 使用可重用 workflow (`_docker-build.yml`)
- ✅ 多平台構建 (linux/amd64, linux/arm64)
- ✅ 同時推送到 Docker Hub 和 GHCR
- ✅ 自動標籤：main → `latest`，tag → 版本號（如 `v1.2.3`）
- ✅ Tag push 忽略 paths 過濾（總是構建）
- ✅ Tag 規則以 metadata 的 `type=raw` 定義，一次生成兩個 registry 需用的所有標籤

---

### 🧹 PR Cleanup (`pr-cleanup.yml`)
**觸發時機**: Pull Request 關閉

**功能**:
- ✅ 自動清理 PR 專用的 Docker image
- ✅ 使用 PR 編號匹配 (`^pr-{number}$`，正則精確匹配)

---

### 🔧 Reusable Workflow (`_docker-build.yml`)
**用途**: 被其他 workflow 調用的可重用構建流程

**優點**:
- ✅ 消除重複代碼
- ✅ 統一構建邏輯
- ✅ 支援參數化（標籤、平台、registry）

---

## 最佳實踐應用

### ✅ Go 項目
- 使用 `go-version-file` 而非硬編碼版本
- `setup-go@v6` 的 `cache: true` 自動處理依賴和構建緩存
- `go mod verify` 驗證依賴完整性（防止供應鏈攻擊）
- 使用 `-short` flag 跳過網路測試（確保 CI 穩定、快速）
- 覆蓋率支援本地顯示（不上傳第三方）

### ✅ Docker 構建
- 使用 `cache-from/cache-to` 加速構建
- Branch-specific cache scope（`ci-pr` / `release`）避免衝突
- Docker metadata action 自動產生語義化標籤
- 單平台構建在 CI（快速），多平台在 release（完整）
- 使用最新的 actions：checkout@v5, setup-go@v6

### ✅ Workflow 設計
- 使用 `concurrency` 避免重複執行浪費資源
- 可重用 workflow 減少維護成本
- 條件執行節省 CI 分鐘數（Docker 構建僅在 PR 時）

### ✅ 安全性
- 最小權限原則（`packages: write` 僅在需要時）
- Trivy 掃描 + CodeQL SARIF 上傳
- govulncheck 檢查 Go 依賴漏洞
- **新增**: 依賴驗證防止篡改

---

## 工作流程矩陣

| Workflow | 觸發 | 執行內容 | 產物 | Cache Scope |
|---------|------|---------|------|-------------|
| **CI** | Push 非 main<br>PR 到 main<br>手動觸發 | 測試<br>Lint<br>漏洞掃描<br>Docker (僅 PR 非 fork)<br>Trivy 掃描 | `pr-143` image<br>SARIF 報告 | `ci-pr` |
| **PR Cleanup** | PR 關閉 | 刪除 GHCR image | - | - |
| **Release** | Push main (代碼變更)<br>Push tag `v*.*.*` | 雙平台 Docker 構建 | `latest` 或 `v1.2.3`<br>推送到 Hub+GHCR | `release` |
| **Docker Build** | 被調用 | 可重用構建邏輯 | 參數化 images | `release` |

---

## 需要的 Secrets

```bash
# Required for Docker Hub push
DOCKERHUB_TOKEN=<your-token>

# Auto-provided by GitHub
GITHUB_TOKEN=<auto>
```

---

## 本地測試

```powershell
# 運行測試（模擬 CI，跳過網路測試）
task test

# 運行完整測試（包含網路測試，較慢）
task test:full

# 構建 Docker（不需要 QEMU）
docker build -t test:local .

# 查看覆蓋率
task test:coverage
```
