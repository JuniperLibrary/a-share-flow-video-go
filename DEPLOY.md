# 部署指南: GitHub Actions + GitHub Pages

## 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                  GitHub Actions (定时采集)                     │
│                                                             │
│  a-share-flow-video-go 仓库                                   │
│    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐ │
│    │ 交易日 15:05  │───▶│ 采集板块数据   │───▶│ 导出 JSON     │ │
│    │ 自动触发      │    │ + 新闻       │    │ data/*.json  │ │
│    └──────────────┘    └──────────────┘    └──────┬───────┘ │
│                                                   │         │
│                                                   ▼         │
│                                          git commit & push  │
│                                                   │         │
│                                                   ▼         │
│                              repository_dispatch(data-updated)│
└─────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────┐
│                  GitHub Pages (静态站点)                       │
│                                                             │
│  a-share-flow-video-web 仓库                                  │
│    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐ │
│    │ 收到部署通知   │───▶│ 构建前端      │───▶│ 拉取 JSON     │ │
│    │ 或手动触发    │    │ npm run build │    │ → dist/data/ │ │
│    └──────────────┘    └──────────────┘    └──────┬───────┘ │
│                                                   │         │
│                                                   ▼         │
│                                          GitHub Pages 上线   │
│                                     https://JuniperLibrary.github.io/ │
│                                         a-share-flow-video-web/       │
└─────────────────────────────────────────────────────────────┘
```

## 前置条件

### 1. 创建 Personal Access Token (PAT)

触发跨仓库部署需要 PAT，用于 Go 仓库通知前端仓库更新。

1. 访问 https://github.com/settings/tokens
2. 点击 **Generate new token (classic)**
3. 权限勾选 `repo` (Full control of private repositories)
4. 复制 token 值

### 2. 配置仓库 Secrets

**Go 仓库** (`JuniperLibrary/a-share-flow-video-go`):
- Settings → Secrets and variables → Actions
- **New repository secret**
  - Name: `DEPLOY_PAT`
  - Secret: 粘贴上面创建的 PAT

**可选 — AI 文案 API Key**:
- Name: `OPENAI_API_KEY` — 你的 API Key
- Name: `OPENAI_BASE_URL` — API 地址（如 https://dashscope.aliyuncs.com/compatible-mode/v1）
- Name: `AI_MODEL` — 模型名（如 qwen3.6-plus）

### 3. 启用 GitHub Pages

**前端仓库** (`JuniperLibrary/a-share-flow-video-web`):
1. Settings → Pages
2. Source: **GitHub Actions**

## 工作流说明

### 1. 数据采集 (`a-share-flow-video-go/.github/workflows/data-collection.yml`)

```yaml
on:
  schedule:
    - cron: '5 7 * * 1-5'   # 交易日 15:05 CST (07:05 UTC)
  workflow_dispatch:          # 支持手动触发
```

**执行流程:**
1. Checkout 仓库
2. Setup Go 1.23
3. `DATA_MODE=json go run ./cmd/cli/ --collect-only` — 仅采集数据并导出 JSON，跳过视频渲染
4. `git add data/*.json && git commit && git push` — 提交 JSON 到仓库
5. 触发前端仓库的 `repository_dispatch` 事件 → 触发 Pages 部署

### 2. 静态站点部署 (`a-share-flow-video-web/.github/workflows/deploy-pages.yml`)

**触发方式:**
- 收到 `data-updated` dispatch（由 Go 仓库触发）
- 手动运行 `workflow_dispatch`
- `main` 分支有前端代码提交

**执行流程:**
1. Checkout + Install npm deps
2. `npm run typecheck` — 类型检查
3. `npm run build` — 构建前端
4. 从 Go 仓库下载最新 JSON 数据到 `dist/data/`
5. Deploy to GitHub Pages

## 数据流详解

### 本地开发模式 (`DATA_MODE=sqlite`，默认)

```
采集新数据 → 写入文件 SQLite (data/a-share-flow.db) → 结束
```

- 数据持久化在 SQLite 文件，性能最好

### 部署模式 (`DATA_MODE=json`)

```
启动 → 从 data/*.json 导入历史数据 → 内存 SQLite
→ 采集新数据 → 写入内存 SQLite
→ 管道结束 → 自动导出 JSON → data/*.json 更新
```

- 不产生 `.db` 文件（被 gitignore）
- JSON 文件是唯一的持久化格式
- 适用于 GitHub Actions 等无持久化存储的环境

## 操作步骤

### 第一次部署

```bash
# 1. 推送 Go 仓库到 GitHub
cd a-share-flow-video-go
git remote add origin git@github.com:JuniperLibrary/a-share-flow-video-go.git
git push -u origin main

# 2. 推送前端仓库到 GitHub
cd ../a-share-flow-video-web
git remote add origin git@github.com:JuniperLibrary/a-share-flow-video-web.git
git push -u origin main

# 3. 在 GitHub 上设置 Secrets（见前置条件）

# 4. 手动触发数据采集验证
# GitHub → Go 仓库 → Actions → 数据采集 → Run workflow

# 5. 等采集完成 → 自动触发前端部署
# 访问 https://JuniperLibrary.github.io/a-share-flow-video-web/
```

### 日常使用

数据采集完全自动化，无需人工干预：
- 交易日 15:05 CST 自动运行
- 采集完成后自动部署前端
- 前端自动显示最新数据

需要手动触发时：
```
GitHub → a-share-flow-video-go → Actions → 数据采集 → Run workflow
```

## API Key 管理

AI 文案生成需要 API Key。分三个场景处理：

| 场景 | 做法 |
|------|------|
| **本地开发** | `.env` 文件设置 `OPENAI_API_KEY`，不变 |
| **GitHub Actions** | 在仓库 Settings → Secrets 设置 `OPENAI_API_KEY`，工作流自动注入环境变量 |
| **GitHub Pages 前端** | 前端 ConfigPage（配置 AI Key）需要后端 API，在静态模式下不可用。**不影响**，Pages 只展示数据，不需要 API Key |

> 没有 API Key 也能正常运行，AI 文案会降级为模板模式。

## 视频输出处理

GitHub Actions 上**不渲染视频**，原因：
- `--collect-only` 标志跳过 Remotion 视频生成（需要 GPU / 大量 CPU 资源）
- 视频文件（MP4）体积大，不适合提交到 git 仓库
- Actions runner 没有安装 Remotion 依赖

**如果你想在 CI 上渲染视频：**
1. 在 workflow 中去掉 `--collect-only`，改为 `DATA_MODE=json go run ./cmd/cli/`
2. 使用 `actions/upload-artifact@v4` 上传 `output/` 目录
3. 从 Actions 页面下载产物

示例（在 workflow 末尾添加）：

```yaml
- name: 上传视频产物
  uses: actions/upload-artifact@v4
  with:
    name: videos-${{ github.run_id }}
    path: output/
```

**本地渲染视频不受影响**，照常运行 `go run ./cmd/cli/` 即可。

## 故障排查

### 数据采集失败
1. 检查 Go 仓库 Actions 日志
2. 确认 API 源（东方财富）是否正常
3. 手动触发 workflow 重试

### 前端部署失败
1. 检查前端仓库 Actions 日志
2. 确认 Go 仓库的 JSON 文件是否存在：`https://raw.githubusercontent.com/JuniperLibrary/a-share-flow-video-go/main/data/sectors.json`
3. 确认 PAT 是否有 `repo` 权限

### 前端显示 "静态数据"
- 正常。表示当前没有后端 API 连接，显示的是最近一次采集的数据
- 如果需要实时数据，本地启动 Go 后端：`go run ./cmd/web/`

## 文件清单

### Go 仓库新增/修改
| 文件 | 说明 |
|------|------|
| `.github/workflows/data-collection.yml` | 定时数据采集工作流 |
| `internal/storage/storage.go` | ExportJSON 方法 |
| `internal/config/config.go` | DataMode 配置 |
| `internal/storage/global.go` | 模式切换（内存/文件 SQLite） |
| `.gitignore` | data/*.db* 忽略数据库文件 |
| `DEPLOY.md` | 本文档 |

### 前端仓库新增/修改
| 文件 | 说明 |
|------|------|
| `.github/workflows/deploy-pages.yml` | GitHub Pages 部署工作流 |
| `src/lib/staticData.ts` | 静态数据适配器 |
| `src/api.ts` | API 层静态模式回退支持 |
| `src/pages/DashboardPage.tsx` | 仪表盘静态数据兼容 |
