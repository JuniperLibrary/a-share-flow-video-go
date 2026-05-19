# 板块情绪流 — A股资金流向视频生成器

> 用东方财富真实资金流向数据，自动生成 Bloomberg Terminal × TradingView × 科技电影 HUD 风格的板块资金流向视频。

---

## 这是什么

每天收盘后，A 股哪个板块在吸金？哪个在失血？这个项目把东方财富的板块主力资金流向数据，变成一段 30 秒的可视化视频——资金像水流一样在图表上涌动，配合时间线事件、底部弹窗和滚动资讯，一眼看懂当日市场情绪。

**两种使用方式**：命令行一键生成，或者 Web 控制台可视化操作。

---

## 核心能力

| 能力 | 说明 |
|------|------|
| **双维度** | 早盘（09:30-11:30）和全天（09:30-15:00）独立生成，各自匹配对应时间轴 |
| **双端输出** | 每个维度同时输出移动端 1080×1920（9:16）和 TV端 1920×1080（16:9） |
| **定时调度** | 11:35 自动拉取早盘、15:05 自动拉取全天，时间可在 Web 控制台修改，自动跳过周末 |
| **AI 文案** | 自动参考前 5 日历史文案，逐板块分析资金动向，标题 ≤20 字，含风险提示 |
| **事件分析** | AI 优先（180s 超时），自动降级到数据驱动，保证始终有可用内容 |
| **SSE 实时流** | 数据拉取和视频生成过程通过 Server-Sent Events 实时推送进度 |
| **实时行情** | Web 端「行情」Tab，SSE 推送实时板块资金流曲线 + AI 异动事件检测 |
| **全量导出** | 支持异步导出全部板块数据（非仅 Top21），带断点续传 |
| **SQLite 持久化** | 板块数据、Ticks、文案统一持久化，支持双写 CSV + SQLite |
| **结构化日志** | 全项目 `zap` 日志系统，彩色终端输出 + 请求追踪 + panic 恢复 |

---

## 快速开始

### 前置条件

- **Go 1.21+**
- **Node.js + npm**（Remotion 视频渲染需要）

### 1. 安装依赖

```bash
go mod tidy
```

### 2. 配置（可选）

```bash
cp .env.example .env
```

编辑 `.env` 设置 AI API Key（不设置也能用，会自动走模板模式）：

```env
OPENAI_API_KEY=sk-xxx
OPENAI_BASE_URL=https://api.openai.com/v1
AI_MODEL=gpt-4o-mini
```

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `OPENAI_API_KEY` | LLM API Key | 无（使用模板/数据驱动模式） |
| `OPENAI_BASE_URL` | LLM API 地址 | `https://api.openai.com/v1` |
| `AI_MODEL` | 模型名称 | `gpt-4o-mini` |

### 3. 生成视频

#### 命令行方式

```bash
# 自动检测当前时段（13:00 前 = 早盘，之后 = 全天）
go run ./cmd/cli/

# 指定日期
go run ./cmd/cli/ 2026-05-12

# 指定维度
go run ./cmd/cli/ --session=morning    # 仅早盘
go run ./cmd/cli/ --session=full       # 仅全天

# AI 文案模式
go run ./cmd/cli/ --ai
go run ./cmd/cli/ --ai --session=morning

# 批量处理
go run ./cmd/cli/ --ai 2026-05-12 2026-05-13 2026-05-14
```

#### Web 控制台方式

```bash
# ⚠️ 修改前端代码后，需重新构建才能生效
cd web/frontend && npm run build

go run ./cmd/web/
# 浏览器打开 http://localhost:8084
```

Web 控制台提供：数据拉取、视频生成、文案优化、调度器配置、AI 参数设置、历史数据浏览。

> **注意**：Go Web 服务直接读取 `web/frontend/dist/` 下的静态文件。修改前端源码（`src/`）后，必须执行 `npm run build` 重新编译，重启 Go 服务才能看到最新变化。

### 4. 编译二进制

```bash
go build -o cli ./cmd/cli/
go build -o web-server ./cmd/web/
```

### 5. SQLite 数据库初始化

数据库在 Web/CLI 启动时自动初始化。也可手动初始化：

```bash
# 手动初始化（已存在则提示）
go run ./cmd/initdb/
```

---

## SQLite 持久化

板块数据、Tick 时序数据、文案统一持久化到 SQLite，与 CSV 双写兼容。

### Schema

```sql
CREATE TABLE sectors (
    datetime TEXT NOT NULL,  -- "2026-05-19 09:30" (tick) 或 "2026-05-19" (全量)
    name     TEXT NOT NULL,
    net      REAL NOT NULL,
    PRIMARY KEY (datetime, name)  -- 唯一索引：防重复
);

CREATE TABLE copywriting (
    date    TEXT NOT NULL,
    session TEXT NOT NULL,
    type    TEXT NOT NULL,
    content TEXT NOT NULL,
    PRIMARY KEY (date, session, type)
);
```

### 数据写入

| 来源 | datetime 格式 | 示例 |
|------|--------------|------|
| 全量板块 | `YYYY-MM-DD` | `2026-05-19` |
| Tick 采集 | `YYYY-MM-DD HH:MM` | `2026-05-19 09:30` |
| 文案 | 独立表 | `date + session + type` 唯一 |

同一时间点同一板块重复采集 → `PRIMARY KEY` 冲突 → `INSERT OR REPLACE` 覆盖旧值。

---

## 定时调度

调度器每天在两个时间点自动执行，各自独立跟踪状态：

| 触发时间 | 维度 | 数据文件 | 输出视频 |
|----------|------|----------|----------|
| `11:35`（可配置） | 早盘 | SQLite: `a-share-flow.db` (09:30-11:30) | `早盘.mp4` + `早盘_tv.mp4` |
| `15:05`（可配置） | 全天 | `data/YYYY-MM-DD/sectors.csv` + SQLite | `全天.mp4` + `全天_tv.mp4` |

- 跳过周末（周六、周日不执行）
- 每个维度每日仅执行一次
- 可在 Web 控制台 → 定时任务页面修改触发时间
- 支持「立即执行」按钮（执行全天维度）

---

## 文案生成

### 模板模式（无需 API Key）

基于数据自动生成结构化文案：

```
📊 05月14日 早盘资金流向

💰 净流入 8 个 · 净流出 7 个
📈 合计净流入 12.3亿

🏆 榜首：半导体 +5.2亿
⚠️ 流出最多：银行 -3.1亿

🔥 净流入TOP5：
  1. 半导体  +5.2亿
  2. AI应用  +3.8亿
  ...

❄️ 净流出TOP5：
  1. 银行  -3.1亿
  ...

💡 半导体大幅领跑，主力进攻意愿较强

⚠️ 风险提示：以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。

#A股 #资金流向 #投资理财 #财经分析
#早盘资金流向
```

### AI 模式（需要 API Key）

AI 文案相比模板模式增强：

- **历史参考**：自动读取前 5 日同维度文案，分析资金流向连续性
- **逐板块分析**：对每个流入/流出板块逐一分析资金动向
- **趋势判断**：结合历史文案指出板块资金的变化趋势
- **标题 ≤20 字**：适合小红书/抖音等平台
- **风险提示**：文末固定包含投资风险提示

> 设置了 AI API Key 时，系统只生成 AI 文案；AI 生成失败时自动降级为模板文案。

---

## 输出目录

```
output/YYYY-MM-DD/
├── 早盘.mp4              # 移动端 9:16 (1080×1920)
├── 早盘_tv.mp4           # TV端 16:9 (1920×1080)
├── 全天.mp4
└── 全天_tv.mp4

data/YYYY-MM-DD/
├── sectors.csv               # 全天板块数据（21个监控板块）
└── 板块全量_YYYY-MM-DD.csv   # 全量板块导出（异步任务）

data/
└── a-share-flow.db           # SQLite 数据库（板块+Tick+文案统一持久化）
                              #   sectors 表: datetime+name 唯一索引
                              #     datetime -- 时间 "2026-05-19 09:30" (tick) 或 "2026-05-19" (全量)
                              #     name     -- 板块名称
                              #     net      -- 主力资金净流入（亿）
                              #   copywriting 表: date+session+type 唯一索引
                              #     date     -- 日期 "2026-05-19"
                              #     session  -- 时段 "full" / "morning"
                              #     type     -- 类型 "template" / "ai" / "template_tick" / "ai_tick"
                              #     content  -- 文案内容
```

---

## 视频内容

每段 30 秒视频包含以下视觉元素：

| 元素 | 说明 |
|------|------|
| **资金流图表** | 板块按净流入排序，资金条动态填充，颜色区分流入/流出 |
| **时间线** | 按交易时间排列的市场事件，标注关键板块异动 |
| **底部弹窗** | 8-10 个市场事件在视频播放过程中依次弹出 |
| **滚动资讯** | 底部 ticker 实时滚动 10-12 条资讯 |
| **Header** | 日期 + 维度标签（早盘/全天） |
| **Disclaimer** | 底部免责声明 |

---

## Web API

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/dates` | GET | 获取所有有数据的日期列表 |
| `/api/data/:date` | GET | 获取指定日期数据（query: `?session=morning/full`） |
| `/api/generate` | POST | SSE 流式生成视频（body: `{"date", "format", "session", "copy_mode"}`） |
| `/api/config` | GET/POST | 获取/保存 AI 配置 |
| `/api/optimize-copy` | POST | 单独生成 AI 文案（body: `{"date", "session"}`） |
| `/api/files/:date` | GET | 获取指定日期的视频和文案文件列表 |
| `/api/scheduler` | GET/POST | 获取/修改调度器状态 |
| `/api/scheduler/run-now` | POST | 立即执行（全天维度） |
| `/api/export-all/:date` | GET | 异步全量板块数据导出 |
| `/api/export-all/status/:task_id` | GET | 查询导出任务状态 |
| `/api/export-all/file/:task_id` | GET | 下载导出文件 |
| `/api/export-hot-sectors/:date` | GET | 获取热门板块数据 |
| `/api/tick/stream` | GET | SSE 实时行情流（推送 tick 数据快照 + 心跳保活） |
| `/api/tick/status` | GET | 获取 Tick 采集器状态 |
| `/api/tick/start` | POST | 手动启动 Tick 采集 |
| `/api/tick/stop` | POST | 停止 Tick 采集 |
| `/api/tick/enable` | POST | 启用/禁用定时采集 |
| `/api/tick/interval` | GET/POST | 获取/设置采集频率 |
| `/api/tick-data/:date` | GET | 获取指定日期 Tick 数据（query: `?session=full/morning`） |
| `/api/generate-tick` | POST | 基于 Tick 数据生成视频 |
| `/output/:date/:file` | GET | 下载视频文件 |

---

## 技术架构

### 数据流

```
东方财富 H5 API
    ↓
FetchTop21HotSectors() / FetchHistoricalSectors()
    ↓
SaveSessionData() → data/YYYY-MM-DD/sectors.csv
                → SQLite: sectors (datetime=date, name, net)
    ↓
AnalyzeAllContent() → AI 生成（180s 超时）→ 降级 DataDrivenGenerate()
    ├── MarketEvent[]    底部弹窗事件
    ├── TimelineEvent[]  时间线事件
    └── TickerItem[]     底部滚动资讯
    ↓
RenderVideo() → npx remotion render → MP4
    ↓
GenerateCopywriting() / GenerateCopywritingAI() → 文案 → SQLite: copywriting

Tick 采集（交易时段每 5 分钟）
    ↓
TickFetcher.collectTick() → SQLite: sectors (datetime="date time", name, net)
                        → broadcast() → SSE subscribers → /api/tick/stream
```

### 核心参数

| 参数 | 值 | 说明 |
|------|-----|------|
| FPS | 30 | 视频帧率 |
| TotalFrames | 900 | 总帧数（30 秒视频） |
| 移动端分辨率 | 1080×1920 | 9:16 竖屏 |
| TV端分辨率 | 1920×1080 | 16:9 横屏 |
| 早盘 X 轴 | 0-120 分钟 | 09:30-11:30 |
| 全天 X 轴 | 0-330 分钟 | 09:30-15:00（含 11:30-13:00 午休） |

### 监控板块（Top21HotSectors）

半导体、AI应用、CPO概念、有色金属、锂矿概念、商业航天、电池、机器人、创新药、白酒、消费电子、银行、人工智能、云计算、低空经济、电网设备、通信设备、传媒、国产芯片、元件、通信服务

---

## 项目结构

```
a-share-flow-video-go/
├── cmd/
│   ├── cli/main.go              # CLI 入口：命令行视频生成器
│   ├── web/main.go              # Web 服务入口：gin HTTP 服务器（端口 8084）
│   └── initdb/main.go           # SQLite 数据库手动初始化脚本
├── internal/
│   ├── config/config.go         # 集中配置：视频参数、SessionConfigs、AI 配置、路径管理
│   ├── fetcher/fetcher.go       # 东方财富 API：数据获取、CSV 保存/加载、Top21 过滤
│   ├── analyzer/analyzer.go     # 事件分析：AIGenerate + DataDrivenGenerate + filterBySession
│   ├── copy/copy.go             # 文案生成：模板模式 + AI 模式（含历史文案参考）
│   ├── renderer/renderer.go     # Remotion 桥接：序列化 props → npx remotion render
│   ├── scheduler/scheduler.go   # 定时调度器：双时间点触发，跳过周末，独立状态跟踪
│   ├── storage/storage.go       # SQLite 持久化：板块+Tick+文案统一存储（datetime+name 唯一索引）
│   ├── logger/                  # zap 结构化日志：彩色终端、请求追踪、panic 恢复
│   ├── tickfetcher/             # Tick 采集器：观察者模式（Subscribe/GetSnapshot/broadcast）
│   ├── tickrenderer/            # Tick 视频渲染：基于真实 tick 数据曲线
│   ├── tickscheduler/           # Tick 定时调度：09:28 早盘 / 12:58 全天自动启动
│   └── web/handlers.go          # HTTP handlers：SSE 流式响应、全量导出、路由注册
└── web/frontend/                # Web 前端：React + Vite + Remotion
    ├── src/
    │   ├── renderer/            # Remotion 视频组件
    │   │   ├── Root.tsx         # 根组件，注册 BloombergVideo 和 BloombergVideoTV
    │   │   ├── BloombergVideo.tsx
    │   │   ├── Chart.tsx        # 资金流图表（动态 X 轴）
    │   │   ├── Timeline.tsx     # 时间线（动态归一化）
    │   │   ├── Header.tsx
    │   │   ├── Ticker.tsx
    │   │   ├── RankingPanel.tsx
    │   │   ├── Particles.tsx
    │   │   └── types.ts         # TypeScript 类型定义
    │   ├── pages/               # Web 页面
    │   │   ├── MarketPage.tsx   # 实时行情：SSE 资金流曲线 + AI 异动事件面板
    │   │   ├── TickPage.tsx     # Tick 采集控制 + 数据表格 + 视频生成
    │   │   ├── SchedulerPage.tsx # 调度器配置（双时间选择器）
    │   │   └── ...
    │   ├── api.ts               # API 客户端
    │   └── types.ts             # 前端类型
    └── dist/                    # 构建产物（Web 服务静态文件）
```

---

## 常用命令

```bash
# 构建 Go
go build ./...

# 测试（跳过需要网络的 live 测试）
go test ./... -skip "Live"

# SQLite 数据库初始化
go run ./cmd/initdb/

# 前端类型检查
cd web/frontend && npx tsc --noEmit

# 前端构建
cd web/frontend && npm run build
```

---

## 日志系统

全项目使用 `go.uber.org/zap` 结构化日志：

- **彩色终端输出**：按级别着色（INFO 绿色、WARN 黄色、ERROR 红色）
- **请求追踪**：每个 HTTP 请求记录 method、path、status、duration、client IP
- **Panic 恢复**：自动捕获 panic，记录堆栈，返回 500
- **业务日志**：数据拉取、视频生成、文案保存等关键节点均有结构化日志

---

## 已知问题

- `TestFetchTop21HotSectors_Live` 期望 ≥21 个板块
- 早盘视频使用的是全天累计资金流向数据（定性分析够用，非分时增量）
- 东方财富 API `f62` 字段是当日累计主力净流入，非分时增量

---

## License

MIT
