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
| **多日/单日切换** | 支持单日 Tick 曲线视频 + 多日 Bar Chart Race 视频 |
| **定时调度** | 09:28 早盘自动采集、12:58 全天自动采集，时区固定 Asia/Shanghai，自动跳过周末 |
| **采集去重** | Tick 采集前自动检查数据库，已存在的时间点自动跳过，避免重复采集 |
| **AI 文案** | 自动参考前 5 日历史文案，逐板块分析资金动向，标题 ≤20 字，含风险提示 |
| **TTS 语音合成** | AI 文案自动转为语音（edge-tts），视频开头朗读标题 + 结尾朗读总结内容 |
| **动态视频时长** | 总帧数根据 TTS 音频长度自动计算，视频时长随文案长度自适应 |
| **事件分析** | AI 优先（180s 超时），自动降级到数据驱动，保证始终有可用内容 |
| **SSE 实时流** | 数据拉取和视频生成过程通过 Server-Sent Events 实时推送进度 |
| **实时行情** | Web 端「行情」Tab，SSE 推送实时板块资金流曲线 + AI 异动事件检测 |
| **新闻监控** | 财联社电报实时轮询（交易时段 30s/次），自动匹配关联板块，Web 端检索查阅 |
| **全量导出** | 支持异步导出全部板块数据（非仅 Top21），带断点续传 |
| **SQLite 持久化** | 板块数据、Ticks、文案、新闻统一持久化，支持双写 CSV + SQLite |
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
| `DATA_MODE` | 存储模式: `sqlite`(本地) / `json`(部署) | `sqlite` |

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

# 仅采集数据，跳过视频渲染（适合 CI/部署场景）
go run ./cmd/cli/ --collect-only

# 批量处理
go run ./cmd/cli/ --ai 2026-05-12 2026-05-13 2026-05-14
```

#### Web 控制台方式

前端已分离为独立项目 [a-share-flow-video-web](../a-share-flow-video-web)，通过 CORS 跨域通信。

```bash
# 1. 启动 Go 后端
go run ./cmd/web/

# 2. 在另一个终端启动前端
cd ../a-share-flow-video-web && npm run dev
# 浏览器打开 http://localhost:5173
```

> **注意**：前后端通过跨域通信，前端直接请求后端 API。后端需在 `.env` 中配置 `CORS_ALLOWED_ORIGINS` 允许前端域名。

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

板块数据、Tick 时序数据、文案、新闻统一持久化到 SQLite，与 CSV 双写兼容。

### Schema

```sql
CREATE TABLE sectors (
    datetime   TEXT NOT NULL,  -- "2026-05-19 09:30" (tick) 或 "2026-05-19" (全量)
    name       TEXT NOT NULL,
    net        REAL NOT NULL,
    rate       REAL NOT NULL DEFAULT 0,
    input_date TEXT NOT NULL DEFAULT '',  -- 录入时间 "2026-05-22 13:14:19"
    PRIMARY KEY (datetime, name)
);

CREATE TABLE copywriting (
    date    TEXT NOT NULL,
    session TEXT NOT NULL,
    type    TEXT NOT NULL,
    content TEXT NOT NULL,
    PRIMARY KEY (date, session, type)
);

CREATE TABLE sectors_all (
    date TEXT NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    net  REAL NOT NULL,
    rate REAL NOT NULL DEFAULT 0,
    PRIMARY KEY (date, name)
);

CREATE TABLE tick_events (
    date    TEXT NOT NULL,
    session TEXT NOT NULL,
    type    TEXT NOT NULL,
    payload TEXT NOT NULL,
    PRIMARY KEY (date, session, type)
);

CREATE TABLE cls_news (
    id          BIGINT PRIMARY KEY,
    title       TEXT    NOT NULL,
    content     TEXT    NOT NULL DEFAULT '',
    brief       TEXT    NOT NULL DEFAULT '',
    level       TEXT    NOT NULL DEFAULT 'C',
    reading_num BIGINT  NOT NULL DEFAULT 0,
    ctime       DATETIME NOT NULL,
    shareurl    TEXT    NOT NULL DEFAULT '',
    sectors     TEXT    NOT NULL DEFAULT '',  -- JSON 数组
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 数据写入

| 来源 | datetime 格式 | 示例 |
|------|--------------|------|
| 全量板块 | `YYYY-MM-DD` | `2026-05-19` |
| Tick 采集 | `YYYY-MM-DD HH:MM` | `2026-05-19 09:30` |
| 文案 | 独立表 | `date + session + type` 唯一 |
| 新闻 | 独立表 | `cls_news` id 唯一 |

同一时间点同一板块重复采集 → `PRIMARY KEY` 冲突 → `INSERT OR REPLACE` 覆盖旧值。
新闻按 `id` 去重 → `INSERT OR IGNORE` 防止重复入库。

---

## 定时调度

### Tick 采集调度

Tick 采集调度器在交易时段自动采集板块资金流数据：

| 触发时间 | 说明 |
|----------|------|
| `09:28-09:30` | 早盘自动启动采集 |
| `12:58-13:00` | 全天自动启动采集 |

特性：
- 时区固定为 `Asia/Shanghai`（UTC+8），不受系统时区影响
- 跳过周末（周六、周日不执行）
- 采集前自动检查数据库，已存在的时间点自动跳过
- 采集间隔默认 10 分钟，可在 Web 控制台修改（1-30 分钟）
- 调度器 `shouldStop` 阈值为 `15:15`（=15:00 + 最大间隔10min + 5min 缓冲），确保收盘前最后一笔 tick 完整采集
- 调度器 loop 常驻运行，`Stop()` 只暂停采集，`Shutdown()` 才真正退出

### 全量数据调度

| 触发时间 | 维度 | 数据文件 | 输出视频 |
|----------|------|----------|----------|
| `11:35`（可配置） | 早盘 | SQLite (09:30-11:30) | `早盘.mp4` + `早盘_tv.mp4` |
| `15:05`（可配置） | 全天 | `data/YYYY-MM-DD/sectors.csv` + SQLite | `全天.mp4` + `全天_tv.mp4` |

### 新闻轮询调度

| 时段 | 间隔 | 说明 |
|------|------|------|
| 交易时段（09:00-15:00） | 30 秒 | 实时监控财联社电报 |
| 非交易时段 / 周末 | 5 分钟 | 低频率保持数据更新 |

新闻自动匹配 21 个监控板块关键词，存入 `cls_news` 表。

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
- **TTS 语音播报**：文案自动合成为语音（edge-tts），视频开头朗读标题、资金流动动画后朗读正文总结

> 设置了 AI API Key 时，系统只生成 AI 文案；AI 生成失败时自动降级为模板文案。
> TTS 合成失败时静默降级，不阻塞视频渲染。

---

## 输出目录

```
output/YYYY-MM-DD/
├── 早盘.mp4              # 移动端 9:16 (1080×1920)
├── 早盘_tv.mp4           # TV端 16:9 (1920×1080)
├── 全天.mp4
├── 全天_tv.mp4
└── 早盘_tick.mp4         # Tick 曲线视频

data/
├── a-share-flow.db           # SQLite 数据库（gitignored，本地开发）
├── sectors.json              # 板块数据（Git 友好，部署用）
├── sectors_all.json          # 全量板块数据
├── copywriting.json          # 文案数据
├── tick_events.json          # Tick 时序数据
└── cls_news.json             # 财联社新闻数据

data/YYYY-MM-DD/
├── sectors.csv               # 全天板块数据（21个监控板块）
└── 板块全量_YYYY-MM-DD.csv   # 全量板块导出（异步任务）

TTS 语音文件（临时，在 Remotion 项目目录下）：
../a-share-flow-video-web/public/voiceover/
├── title.mp3                 # 标题口播（视频开头段落）
└── content.mp3               # 总结口播（视频结尾段落）
```

---

## Web API

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/dates` | GET | 获取所有有数据的日期列表 |
| `/api/data/:date` | GET | 获取指定日期数据（query: `?session=morning/full`） |
| `/api/generate` | POST | SSE 流式生成视频 |
| `/api/config` | GET/POST | 获取/保存 AI 配置 |
| `/api/optimize-copy` | POST | 单独生成 AI 文案 |
| `/api/files/:date` | GET | 获取指定日期的视频和文案文件列表 |
| `/api/export-all/:date` | GET | 异步全量板块数据导出 |
| `/api/export-all/status/:task_id` | GET | 查询导出任务状态 |
| `/api/export-all/file/:task_id` | GET | 下载导出文件 |
| `/api/export-hot-sectors/:date` | GET | 获取热门板块数据（Top21） |
| `/api/generate-multiday` | POST | 多日视频生成（Bar Chart Race） |
| `/api/generate-tick` | POST | Tick 曲线视频生成 |
| `/api/tick/stream` | GET | SSE 实时行情流 |
| `/api/tick/status` | GET | 获取 Tick 采集器状态 |
| `/api/tick/start` | POST | 手动启动 Tick 采集 |
| `/api/tick/stop` | POST | 停止 Tick 采集 |
| `/api/tick/enable` | POST | 启用/禁用定时采集 |
| `/api/tick/interval` | GET/POST | 获取/设置采集频率 |
| `/api/tick/force-collect` | POST | 强制采集指定时间点 tick（绕过盘时间检查，参数: `time`, `date`） |
| `/api/tick-data/:date` | GET | 获取指定日期 Tick 数据 |
| `/api/tick/dates` | GET | 获取所有有 Tick 数据的日期 |
| `/api/tick/replay-stream` | GET | SSE 回放历史 Tick 数据 |
| `/api/tick/events/:date` | GET | Tick 事件分析数据 |
| `/api/dashboard` | GET | 仪表盘聚合数据 |
| `/api/sectors-all/save/:date` | POST | 异步获取全量板块并保存 |
| `/api/sectors-all/status/:task_id` | GET | 查询全量板块任务状态 |
| `/api/sectors-all/dates` | GET | 全量板块数据日期列表 |
| `/api/sectors-all/range` | GET | 日期范围全量板块数据 |
| `/api/sectors-all/names` | GET | 所有板块名称列表 |
| `/api/news` | GET | 新闻列表（分页，默认 50 条） |
| `/api/news/search` | GET | 搜索新闻（`?q=关键词`，支持分页） |
| `/api/news/status` | GET | 新闻轮询调度器状态 |
| `/api/news/start` | POST | 启动新闻轮询 |
| `/api/news/stop` | POST | 停止新闻轮询 |
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
                → SQLite: sectors
    ↓
AnalyzeAllContent() → AI 生成（180s 超时）→ 降级 DataDrivenGenerate()
    ├── MarketEvent[]    底部弹窗事件
    ├── TimelineEvent[]  时间线事件
    └── TickerItem[]     底部滚动资讯
    ↓
GenerateCopywriting() / GenerateCopywritingAI() → 文案 → SQLite
    ↓
TTS 合成（edge-tts） → voiceover/title.mp3 + voiceover/content.mp3
    ↓
RenderVideo() → npx remotion render → MP4（三段式结构：标题口播 → 资金流动画 → 结论口播）

Tick 采集（交易时段每 5-10 分钟）
    ↓
TickFetcher.collectTick() → 去重检查 → SQLite
                        → broadcast() → SSE → /api/tick/stream

财联社电报新闻
    ↓
NewsScheduler (30s / 5min 轮询) → FetchTelegraphList()
    ↓
MatchSectorsToNews() → 关键词匹配板块
    ↓
SaveCLSNews() → INSERT OR IGNORE → SQLite: cls_news
```

### 核心参数

| 参数 | 值 | 说明 |
|------|-----|------|
| FPS | 30 | 视频帧率 |
| 基础动画帧数 | 900 | 资金流动画帧数（30 秒），TTS 扩展后总帧数动态增加 |
| TTS 引擎 | edge-tts (zh-CN-XiaoxiaoNeural) | 免费中文语音合成 |
| 移动端分辨率 | 1080×1920 | 9:16 竖屏 |
| TV端分辨率 | 1920×1080 | 16:9 横屏 |
| 早盘 X 轴 | 0-120 分钟 | 09:30-11:30 |
| 全天 X 轴 | 0-330 分钟 | 09:30-15:00（含 11:30-13:00 午休） |
| 新闻轮询（交易） | 30 秒 | 交易时段财联社电报拉取间隔 |
| 新闻轮询（非交易） | 5 分钟 | 非交易时段拉取间隔 |

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
│   ├── renderer/renderer.go     # Remotion 桥接：单日/多日视频渲染 + TTS 语音合成集成
│   ├── tts/tts.go               # TTS 语音合成：edge-tts 调用、音频时长解析、文案解析
│   ├── scheduler/scheduler.go   # 定时调度器：双时间点触发，跳过周末，独立状态跟踪
│   ├── storage/storage.go       # SQLite 持久化：板块+Tick+文案+新闻统一存储
│   ├── logger/                  # zap 结构化日志：彩色终端、请求追踪、panic 恢复
│   ├── tickfetcher/             # Tick 采集器：观察者模式 + 时区固定 + 采集去重
│   ├── tickscheduler/           # Tick 定时调度：09:28 早盘 / 12:58 全天
│   ├── clsnews/                 # 财联社新闻系统：API 抓取、板块匹配、轮询调度
│   │   ├── types.go             # CLSNews 类型定义
│   │   ├── fetcher.go           # 电报列表 API 客户端 + 交易时段判断
│   │   ├── sector_matcher.go    # 新闻→板块关键词匹配器
│   │   └── scheduler.go         # 后台轮询调度器（30s/5min）
│   └── web/handlers.go          # HTTP handlers：SSE 流、全量导出、新闻路由、CORS
└── data/                        # 数据目录：CSV + SQLite + JSON (Git 友好)
```

> 前端已分离为独立项目：[a-share-flow-video-web](../a-share-flow-video-web)，通过 CORS 跨域与后端通信。

---

## 常用命令

```bash
# 构建 Go
go build ./...

# 测试（跳过需要网络的 live 测试）
go test ./... -skip "Live"

# SQLite 数据库初始化
go run ./cmd/initdb/

# 前端操作（在独立项目中）
cd ../a-share-flow-video-web && npm run dev       # 开发模式
cd ../a-share-flow-video-web && npm run build     # 生产构建
cd ../a-share-flow-video-web && npm run typecheck # 类型检查
```

---

## 部署方案

支持两种运行模式，通过 `DATA_MODE` 环境变量切换：

### 本地开发（默认）

```bash
# SQLite 持久化，性能最优
go run ./cmd/cli/
```

- 数据写入 `data/a-share-flow.db`（被 gitignore）

### GitHub Pages 部署

```bash
DATA_MODE=json go run ./cmd/cli/
```

- 使用内存 SQLite，启动时自动从 JSON 文件导入历史数据
- 采集完成后自动导出 JSON 到 `data/*.json`
- 不产生 `.db` 文件

> 详细部署步骤（GitHub Actions + GitHub Pages）：[DEPLOY.md](./DEPLOY.md)

---

## 日志系统

全项目使用 `go.uber.org/zap` 结构化日志：

- **彩色终端输出**：按级别着色（INFO 绿色、WARN 黄色、ERROR 红色）
- **请求追踪**：每个 HTTP 请求记录 method、path、status、duration、client IP
- **Panic 恢复**：自动捕获 panic，记录堆栈，返回 500
- **业务日志**：数据拉取、视频生成、文案保存、新闻轮询等关键节点均有结构化日志

---

## 已知问题

- `TestFetchTop21HotSectors_Live` 期望 ≥21 个板块
- 早盘视频使用的是全天累计资金流向数据（定性分析够用，非分时增量）
- 东方财富 API `f62` 字段是当日累计主力净流入，非分时增量
- 15:00 tick 数据采集需确认 `shouldStop` 阈值 ≥ `15:15`（已在 `tickscheduler.go:128` 修复），避免最后一笔 tick 因 race condition 丢失

---

## License

MIT
