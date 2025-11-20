# .github/

GitHub 配置目錄。

## 檔案結構

```
.github/
├── copilot-instructions.md  - AI Agent 開發指引
├── dependabot.yml           - 依賴自動更新設定
└── workflows/
    └── ci.yml               - CI/CD 流程
```

## CI Workflow

**觸發時機**: Push / Pull Request 到 `main` 或 `migrate-from-python`

**檢查項目**:
1. 格式化檢查 (`go fmt`)
2. 靜態分析 (`go vet`)
3. Linter (`golangci-lint`)
4. 單元測試 (`go test`)
5. 編譯 (`go build`)

**本地驗證**:
```bash
task ci  # 執行完整 CI 檢查
```

## Workflows

### `ci.yml` - Continuous Integration

自動化測試與建置流程，在以下情況觸發：
- Push 到任何分支
- Pull Request 到 `main` 或 `migrate-from-python`

**流程步驟**:
1. **Setup**:
   - 安裝 Go 1.25
   - 快取模組依賴 (`go.mod`, `go.sum`)

2. **Code Quality**:
   - `go fmt` 格式檢查
   - `go vet` 靜態分析
   - `golangci-lint` 全面檢查

3. **Testing**:
   - 執行所有單元測試
   - 產生覆蓋率報告
   - 上傳到 Codecov (可選)

4. **Build**:
   - 編譯 Linux/Windows/macOS 版本
   - 上傳建置產物 (artifacts)

**使用範例**:

```yaml
name: CI

on:
  push:
    branches: [ "main", "migrate-from-python" ]
  pull_request:
    branches: [ "main", "migrate-from-python" ]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Run tests
        run: go test -v -race -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          file: ./coverage.out
```

### 本地測試 CI

在 Push 前可先在本地驗證：

```bash
# 格式化檢查
go fmt ./...

# 靜態分析
go vet ./...

# Linter (需先安裝 golangci-lint)
golangci-lint run

# 執行測試
go test -v -race -coverprofile=coverage.out ./...

# 查看覆蓋率
go tool cover -html=coverage.out

# 或使用 Task runner
task ci
```

## Secrets 設定

以下 secrets 需在 GitHub Repository Settings 中設定：

| Secret 名稱 | 說明 | 必要性 |
|------------|------|--------|
| `CODECOV_TOKEN` | Codecov 上傳 token | 可選 |
| `DOCKER_USERNAME` | Docker Hub 帳號 | 部署用 |
| `DOCKER_PASSWORD` | Docker Hub 密碼 | 部署用 |

**設定方式**:
1. 進入 Repository → Settings → Secrets and variables → Actions
2. 點擊 "New repository secret"
3. 輸入名稱與值

## Branch Protection Rules

建議為 `main` 分支設定保護規則：

- ✅ Require pull request reviews (至少 1 人審核)
- ✅ Require status checks to pass (CI 必須通過)
- ✅ Require branches to be up to date
- ✅ Include administrators (管理員也受限制)

## Workflow Badges

在 README.md 中顯示 CI 狀態：

```markdown
[![CI](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml)
```

結果：
[![CI](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml)

## Dependabot

Dependabot 會自動建立 PR 更新依賴：

- **Go modules**: 每週一檢查
- **GitHub Actions**: 每週一檢查
- PR 會自動標記 `dependencies` label
- 小版本更新建議自動合併

**自動合併設定** (`.github/workflows/dependabot-auto-merge.yml`):

```yaml
name: Dependabot auto-merge
on: pull_request

permissions:
  contents: write
  pull-requests: write

jobs:
  auto-merge:
    runs-on: ubuntu-latest
    if: github.actor == 'dependabot[bot]'
    steps:
      - name: Enable auto-merge
        run: gh pr merge --auto --squash "$PR_URL"
        env:
          PR_URL: ${{github.event.pull_request.html_url}}
          GH_TOKEN: ${{secrets.GITHUB_TOKEN}}
```

## 最佳實踐

### Commit Messages

遵循 Conventional Commits 規範：

```
feat: 新增課程查詢功能
fix: 修正學號查詢錯誤
docs: 更新 README
test: 新增單元測試
refactor: 重構爬蟲邏輯
chore: 更新依賴套件
```

### Pull Request

PR 標題應清楚描述變更：

```
feat(bot): 新增教師查詢功能
fix(scraper): 修正 Big5 編碼問題
docs: 新增 Bot 模組文件
```

PR 描述應包含：
- **What**: 做了什麼變更
- **Why**: 為什麼需要這個變更
- **How**: 如何實作
- **Testing**: 如何測試

### Code Review Checklist

- [ ] 程式碼通過 CI
- [ ] 測試覆蓋率 > 80%
- [ ] 無 linter 警告
- [ ] 文件已更新
- [ ] Commit messages 清晰
- [ ] 無破壞性變更 (或已註明)

## Troubleshooting

### CI 失敗常見原因

1. **格式問題**: 執行 `go fmt ./...`
2. **Vet 錯誤**: 執行 `go vet ./...` 查看詳情
3. **測試失敗**: 執行 `go test -v ./...` 本地測試
4. **Lint 錯誤**: 執行 `golangci-lint run` 查看詳情
5. **建置失敗**: 檢查 `go.mod` 依賴是否正確

### Dependabot PR 無法合併

1. 檢查 CI 是否通過
2. 查看是否有破壞性變更
3. 本地測試更新後的版本
4. 必要時手動調整程式碼

### Actions 執行時間過長

1. 使用快取加速依賴下載
2. 分拆大型測試檔案
3. 使用並行測試 (`go test -parallel`)
4. 考慮 self-hosted runners

## 相關連結

- [GitHub Actions 文件](https://docs.github.com/en/actions)
- [Dependabot 文件](https://docs.github.com/en/code-security/dependabot)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [Go CI/CD 最佳實踐](https://golangci-lint.run/usage/install/)
