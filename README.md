# 板块情绪流 — A股资金流向视频生成器 (Go 版)

使用东方财富真实资金流向数据，生成 Bloomberg Terminal × TradingView × 科技电影 HUD 风格的视频。支持 **早盘/全天** 双维度，适配移动端 9:16 和 TV端 16:9。

## 核心功能

- **双维度视频**：早盘（09:30-11:30）+ 全天（09:30-15:00）独立生成
- **双端适配**：移动端 9:16 + TV端 16:9 同时输出
- **定时调度**：11:35 自动拉取早盘 / 15:05 自动拉取全天
- **AI 文案**：参考前5日历史趋势，逐板块分析，标题≤20字，含风险提示
- **Web 控制台**：可视化拉取数据、生成视频、配置调度

## 项目结构

```
a-share-flow-video-go/
├── cmd/
│   ├── cli/main.go          # CLI 入口
│   └── web/main.go          # Web 服务入口
├── internal/
│   ├── config/              # 配置管理（含早盘/全天 SessionConfigs）
│   ├── fetcher/             # 东方财富 API 数据获取
│   ├── analyzer/            # 事件分析（数据驱动 + AI）
│   ├── copy/                # 文案生成（模板 + AI）
│   ├── renderer/            # Remotion 桥接
│   ├── scheduler/           # 定时调度器（双时间点触发）
│   └── web/                 # HTTP handlers + SSE
└── web/frontend/            # Web 前端（React + Vite + Remotion）
    └── src/
        ├── renderer/        # Remotion 视频渲染组件
        └── pages/           # Web 页面
```

## 前置条件

1. **Go 1.21+**
2. **Node.js + npm**（Remotion 渲染需要）

## 快速开始

### 1. 安装依赖

```bash
go mod tidy
```

### 2. 配置

```bash
cp .env.example .env
# 编辑 .env 设置 AI API Key（可选，用于 AI 文案）
```

### 3. CLI 生成视频

```bash
# 自动检测当前时段（13:00前=早盘，之后=全天）
go run ./cmd/cli/

# 指定日期
go run ./cmd/cli/ 2026-05-12

# 指定维度
go run ./cmd/cli/ --session=morning    # 仅早盘
go run ./cmd/cli/ --session=full       # 仅全天

# AI 文案模式
go run ./cmd/cli/ --ai
go run ./cmd/cli/ --ai --session=morning
```

### 4. 启动 Web 服务

```bash
go run ./cmd/web/
# 浏览器打开 http://localhost:8084
```

### 5. 编译二进制

```bash
go build -o cli ./cmd/cli/
go build -o web-server ./cmd/web/
```

## 定时调度

调度器支持两个独立触发时间点：

| 时间 | 维度 | 数据文件 | 输出视频 |
|------|------|----------|----------|
| 11:35 | 早盘 | `sectors_morning.csv` | `早盘.mp4` / `早盘_tv.mp4` |
| 15:05 | 全天 | `sectors.csv` | `全天.mp4` / `全天_tv.mp4` |

时间可在 Web 控制台 → 定时任务页面修改。

## 文案生成

| 模式 | 说明 | 输出文件 |
|------|------|----------|
| 模板 | 基于数据自动生成 | `文案_早盘.txt` / `文案_全天.txt` |
| AI | 参考前5日历史 + 逐板块分析 + 风险提示 | `文案_ai_早盘.txt` / `文案_ai_全天.txt` |

> 设置了 AI API Key 时，只生成 AI 文案（失败时自动降级为模板文案）。

## 输出目录

```
output/YYYY-MM-DD/
├── 早盘.mp4          # 移动端 9:16
├── 早盘_tv.mp4       # TV端 16:9
├── 全天.mp4
└── 全天_tv.mp4

data/YYYY-MM-DD/
├── sectors_morning.csv   # 早盘数据
└── sectors.csv           # 全天数据

copy/YYYY-MM-DD/
├── 文案_早盘.txt
├── 文案_ai_早盘.txt
├── 文案_全天.txt
└── 文案_ai_全天.txt
```

## Web API

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/fetch` | POST | 拉取板块数据（body: `{"date", "force", "session"}`） |
| `/api/generate` | POST | 生成视频（body: `{"date", "format", "session", "copy_mode"}`） |
| `/api/data/:date` | GET | 获取数据（query: `?session=morning/full`） |
| `/api/scheduler` | GET/POST | 查看/修改调度器配置 |
| `/api/scheduler/run-now` | POST | 立即执行（全天维度） |

## License

MIT
