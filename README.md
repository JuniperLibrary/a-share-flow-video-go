# 板块情绪流 — A股资金流向视频生成器 (Go 版)

使用东方财富真实资金流向数据，生成 Bloomberg Terminal × TradingView × 科技电影 HUD 风格的 9:16 竖屏情绪流动可视化视频。

本项目是 Python 版本的 Go 语言重写，保留了 Remotion 渲染管线。

## 项目结构

```
a-share-flow-video-go/
├── go.mod
├── cmd/
│   ├── cli/main.go          # CLI 入口
│   └── web/main.go          # Web 服务入口
├── internal/
│   ├── config/              # 配置管理
│   ├── fetcher/             # 东方财富 API 数据获取
│   ├── analyzer/            # 事件分析（数据驱动 + AI）
│   ├── copy/                # 文案生成
│   ├── renderer/            # Remotion 桥接
│   ├── scheduler/           # 定时调度器
│   └── web/                 # HTTP handlers + SSE
└── web/
    ├── frontend/            # Web 前端（React + Vite）
    │   └── src/
    │       ├── renderer/    # Remotion 视频渲染组件
    │       ├── pages/       # Web 页面
    │       └── ...
    └── templates/           # Web 前端模板
```

## 前置条件

1. **Go 1.21+**
2. **Node.js + npm**（Remotion 渲染需要）
3. **Remotion 前端**：已融合到 `web/frontend/src/renderer/`

## 快速开始

### 1. 安装依赖

```bash
go mod tidy
```

### 2. 配置

```bash
cp .env.example .env
# 编辑 .env 设置 AI API Key（可选）
```

### 3. CLI 生成视频

```bash
# 当天数据
go run ./cmd/cli/

# 指定日期
go run ./cmd/cli/ 2026-05-12

# AI 文案模式
go run ./cmd/cli/ --ai
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

## 与 Python 版的区别

| 方面 | Python 版 | Go 版 |
|---|---|---|
| 数据获取 | `curl_cffi`（Chrome TLS 指纹） | 标准 `net/http` |
| Web 框架 | FastAPI + uvicorn | gin |
| 模板引擎 | Jinja2 | Go html/template |
| 数据处理 | pandas | 原生 struct + slice |
| 并发 | threading | goroutine |
| 依赖管理 | uv/pyproject.toml | go.mod |

## 输出

- `output/YYYY-MM-DD/全天.mp4` — 移动端 9:16
- `output/YYYY-MM-DD/全天_tv.mp4` — TV端 16:9
- `data/YYYY-MM-DD/sectors.csv` — 板块数据
- `data/YYYY-MM-DD/copy_全天.txt` — 模板文案

## License

MIT
