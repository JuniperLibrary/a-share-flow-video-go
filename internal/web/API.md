# 接口文档（外部依赖 + 对内服务）

> 本文档涵盖系统依赖的所有外部 API（东方财富、财联社、AI LLM）以及对内暴露的 HTTP API。

---

## 一、外部依赖 API

### 1.1 东方财富 — 板块实时资金流

获取板块主力资金净流入实时数据。

| 项目 | 值 |
|------|-----|
| 用途 | 每日板块资金流抓取（Top21 过滤前） |
| 调用方 | `internal/fetcher/fetcher.go` |
| 频率 | 交易时段每 5-10 分钟一次（Tick 采集）；收盘后单次（全量/历史） |
| 重试 | 最多 3 次，间隔 `attempt * 2s` |

#### 实时板块列表（H5 API）

```
GET https://emdatah5.eastmoney.com/dc/ZJLX/getZDYLBData
  ?fields=f12,f14,f3,f5,f6,f62,f66,f69,f72,f75,f184
  &pn={pageNo}
  &pz=500
  &fid=f62
  &po=1
  &fs=m:90+t:2
  &ut=b2884a393a59ad64002292a3e90d46a5
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `pn` | int | 页码，从 1 开始，自动翻页直到返回 < 100 条 |
| `pz` | int | 每页条数，固定 500 |
| `fid` | string | 排序字段，`f62` = 主力净流入 |
| `po` | int | 排序方向：1 = 降序 |
| `fs` | string | 过滤条件：`m:90+t:2` = 东财行业板块 |

**响应字段映射：**

| JSON 字段 | 含义 | Go 类型 |
|-----------|------|---------|
| `data.diff[].f12` | 板块代码 BKxxxx | `string` |
| `data.diff[].f14` | 板块名称 | `string` |
| `data.diff[].f62` | 主力净流入（元） | `float64` → 除以 1e8 转为亿 |
| `data.diff[].f184` | 主力净占比（%） | `float64` |

**请求头：**
- `User-Agent`: Mozilla/5.0 ...
- `Referer`: `https://emdatah5.eastmoney.com/dc/zjlx/index`
- `Accept`: application/json

---

#### 板块 BK 代码查询

```
GET https://82.push2.eastmoney.com/api/qt/clist/get
  ?pn=1&pz=500&po=1&np=1&fltt=2&invt=2
  &fid=f62
  &fs=m:90+t:2
  &fields=f12,f14
  &ut=b2884a393a59ad64002292a3e90d46a5
```

| 参数 | 说明 |
|------|------|
| `fs` | `m:90+t:2` 行业板块 |
| `fields` | `f12`=代码, `f14`=名称 |

返回 `{name → BKCode}` 映射，用于后续历史数据查询。

**Referer:** `https://data.eastmoney.com/bkzj/hy.html`

---

#### 单板块历史资金流

```
GET https://push2his.eastmoney.com/api/qt/stock/fflow/daykline/get
  ?secid=90.{bkCode}
  &lmt=30
  &klt=101
  &fields1=f1,f2,f3,f7
  &fields2=f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61,f62,f63,f64,f65
  &ut=b2884a393a59ad64002292a3e90d46a5
```

| 参数 | 说明 |
|------|------|
| `secid` | `90.{BKCode}`，如 `90.BK1036` |
| `lmt` | 返回天数，固定 30 |
| `klt` | 101 = 日线 |

**响应解析：** `data.klines[]` 每行格式为 CSV：
```
date,net_yuan, ,...,f62(主力净流入),...
```
取与目标日期匹配的 `f62` 列，转为亿。

**请求间隔：** 每个板块间隔 500ms，连续失败 5 次停止（防 IP 封锁）。

---

### 1.2 财联社 — 电报列表

获取财联社 7×24 小时实时电报新闻。

| 项目 | 值 |
|------|-----|
| 用途 | 实时新闻监控，辅助板块热点分析 |
| 调用方 | `internal/clsnews/fetcher.go` |
| 频率 | 交易时段 30s，非交易时段 5min |
| 增量 | 传入 `last_time` 参数拉取增量数据 |

#### 电报列表

```
GET https://www.cls.cn/nodeapi/telegraphList
  ?app=CailianpressWeb
  &os=web
  &refresh_type=1
  &rn=200
  &sv=8.4.6
  [&last_time={unix_timestamp}]
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `app` | string | 固定 `CailianpressWeb` |
| `os` | string | 固定 `web` |
| `refresh_type` | int | 固定 1 |
| `rn` | int | 返回条数，固定 200 |
| `sv` | string | 版本号 `8.4.6` |
| `last_time` | int64 | 可选，Unix 秒级时间戳，增量拉取 |

**响应结构：**

```json
{
  "code": 0,
  "data": {
    "roll_data": [
      {
        "id": 2380085,
        "title": "",
        "content": "财联社5月24日电，乌克兰总理表示...",
        "brief": "财联社5月24日电，乌克兰总理表示...",
        "ctime": 1779613848,
        "level": "C",
        "reading_num": 45788,
        "shareurl": "https://api3.cls.cn/share/article/2380085",
        "type": -1,
        "subjects": [
          {"subject_id": 1556, "subject_name": "环球市场情报"},
          {"subject_id": 9067, "subject_name": "俄乌冲突快报"}
        ]
      }
    ]
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | int | 业务状态码，0=成功 |
| `data.roll_data[].id` | number | 新闻唯一 ID |
| `title` | string | 标题（C 级快讯可能为空） |
| `content` | string | 正文 |
| `brief` | string | 摘要 |
| `ctime` | int64 | 发布时间（Unix 秒） |
| `level` | string | `A`=重点红字, `B`=红字, `C`=普通 |
| `reading_num` | int | 阅读数 |
| `shareurl` | string | 原文链接 |
| `subjects` | array | 关联专题（暂未入库） |

**请求头：**
- `User-Agent`: Mozilla/5.0 ...
- `Referer`: `https://www.cls.cn/telegraph`
- `Accept`: application/json

---

### 1.3 AI / LLM 文案生成

调用 OpenAI 兼容 API 生成文案或事件分析。

| 项目 | 值 |
|------|-----|
| 用途 | AI 文案生成 + AI 事件分析 |
| 调用方 | `internal/analyzer/analyzer.go`, `internal/copy/copy.go` |
| 协议 | OpenAI Chat Completions API 兼容 |
| 超时 | 180s |
| 降级 | AI 失败自动降级为模板/数据驱动模式 |

#### 请求

```
POST {baseURL}/chat/completions
```

| 配置项 | 环境变量 | 默认值 |
|--------|---------|--------|
| `baseURL` | `OPENAI_BASE_URL` | `https://api.openai.com/v1` |
| `apiKey` | `OPENAI_API_KEY` | — |
| `model` | `AI_MODEL` | `gpt-4o-mini` |

**请求头：**
- `Authorization: Bearer {apiKey}`
- `Content-Type: application/json`

**请求体（文案生成）：**
```json
{
  "model": "gpt-4o-mini",
  "messages": [
    {"role": "system", "content": "你是一个金融视频文案撰稿人..."},
    {"role": "user", "content": "数据: 半导体 +5.2亿, AI应用 +3.8亿, ..."}
  ],
  "temperature": 0.7,
  "max_tokens": 1000
}
```

**响应：**
```json
{
  "choices": [
    {
      "message": {
        "content": "📊 05月14日 早盘资金流向..."
      }
    }
  ]
}
```

| 用途 | System Prompt | Max Tokens | 超时 |
|------|--------------|-----------|------|
| 文案生成 | 金融视频文案撰稿人指令 | 1000 | 180s |
| 事件分析 | 市场分析师指令，要求 JSON 输出 | 2000 | 180s |

---

## 二、对内 HTTP API（服务端 → 前端）

基础路径：`http://{host}:8084`，所有接口返回 JSON。

### 2.1 数据查询

#### 日期列表

```
GET /api/dates
```

**响应：**
```json
{
  "dates": [
    {
      "date": "2026-05-24",
      "videos": ["全天", "早盘_tv"],
      "sector_count": 21,
      "morning_count": 21,
      "文案_count": 1,
      "ai_count": 1
    }
  ]
}
```

#### 指定日期数据

```
GET /api/data/:date?session=full|morning
```

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `date` | path | — | 日期 `2026-05-24` |
| `session` | query | `full` | `full`=全天, `morning`=早盘 |

**响应：**
```json
{
  "sectors": [{"name": "半导体", "net": 5.2, "rate": 3.93}],
  "videos": ["全天"],
  "文案": {
    "template": {"full": "...", "morning": "..."},
    "ai": {"full": "..."}
  }
}
```

#### 文件列表

```
GET /api/files/:date
```

**响应：**
```json
{
  "videos": {"全天": "全天.mp4", "早盘_tv": "早盘_tv.mp4"},
  "文案": {
    "template": {"full": "...", "morning": "..."},
    "ai": {"full": "..."}
  }
}
```

#### 仪表盘聚合

```
GET /api/dashboard
```

**响应：**
```json
{
  "dates": ["2026-05-24", "2026-05-23"],
  "marketOverview": {
    "totalSectors": 21,
    "inflowCount": 12,
    "outflowCount": 9,
    "totalNet": 15.3,
    "topSector": {"name": "半导体", "net": 5.2},
    "worstSector": {"name": "银行", "net": -3.1}
  },
  "ranking": [{"name": "半导体", "net": 5.2}],
  "events": [{"time": "09:30", "sector": "半导体", "title": "大幅流入", "description": "...", "sentiment": "positive"}],
  "trend": {"半导体": [{"date": "2026-05-23", "net": 2.1}]},
  "trendDates": ["2026-05-23", "2026-05-24"]
}
```

---

### 2.2 AI 配置

#### 获取配置

```
GET /api/config
```

**响应：**
```json
{
  "has_api_key": true,
  "api_base": "https://api.openai.com/v1",
  "model": "gpt-4o-mini",
  "sessions": {"full": "全天", "morning": "早盘"}
}
```

#### 保存配置

```
POST /api/config
Content-Type: application/json

{
  "api_key": "sk-xxx",
  "api_base": "https://api.openai.com/v1",
  "model": "gpt-4o-mini"
}
```

**响应：** `{"ok": true}`

---

### 2.3 热门板块

#### 实时 Top21 热门板块

```
GET /api/export-hot-sectors/:date
```

| 参数 | 说明 |
|------|------|
| `date` | 日期，留空用当天 |

**响应：**
```json
{
  "date": "2026-05-24",
  "sectors": [{"name": "半导体", "net": 5.2, "rate": 3.93, "color": "#22d3ee"}]
}
```

---

### 2.4 全量板块

#### 异步获取并保存

```
POST /api/sectors-all/save/:date
```

**响应：**
```json
{
  "task_id": "save_20260524_1",
  "message": "已启动后台获取任务"
}
```

#### 任务状态

```
GET /api/sectors-all/save/status/:task_id
```

**响应：**
```json
{
  "task_id": "save_20260524_1",
  "date": "2026-05-24",
  "status": "done|running|error",
  "progress": "完成: 已保存 280 个板块数据",
  "count": 280,
  "error": ""
}
```

#### 查询日期的全量板块

```
GET /api/sectors-all/:date
```

**响应：**
```json
{
  "date": "2026-05-24",
  "sectors": [{"date": "2026-05-24", "code": "BK1036", "name": "半导体", "net": 5.2, "rate": 3.93}]
}
```

#### 全量板块日期列表

```
GET /api/sectors-all/dates
```

**响应：** `{"dates": ["2026-05-24", "2026-05-23"]}`

#### 全量板块名称列表

```
GET /api/sectors-all/names
```

**响应：** `{"names": ["半导体", "AI应用", ...]}`

#### 日期范围数据

```
GET /api/sectors-all/range?start_date=2026-05-20&end_date=2026-05-24
```

**响应：**
```json
{
  "sectors": [
    {"date": "2026-05-20", "code": "BK1036", "name": "半导体", "net": 3.1, "rate": 2.5},
    {"date": "2026-05-21", "code": "BK1036", "name": "半导体", "net": -1.2, "rate": -0.8}
  ]
}
```

---

### 2.5 Tick 采集控制

#### 采集器状态

```
GET /api/tick/status
```

**响应：**
```json
{
  "running": true,
  "enabled": true,
  "date": "2026-05-24",
  "session": "full",
  "lastTime": "14:30",
  "count": 42
}
```

#### 手动启动 / 停止

```
POST /api/tick/start
POST /api/tick/stop
```

**响应：** `{"ok": true, "message": "tick 采集已启动"}` / `{"ok": true, "message": "tick 采集已停止"}`

#### 启用/禁用定时

```
POST /api/tick/enable
Content-Type: application/json

{"enabled": true}
```

**响应：** `{"ok": true}`

#### 采集间隔

```
GET /api/tick/interval              → {"intervalMinutes": 10}
POST /api/tick/interval             → {"ok": true, "intervalMinutes": 15}
Content-Type: application/json

{"intervalMinutes": 15}
```

---

### 2.6 Tick 数据

#### 指定日期 Tick 数据

```
GET /api/tick-data/:date?session=full|morning
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `date` | — | 日期 |
| `session` | `full` | `full` 或 `morning` |

**响应：**
```json
{
  "date": "2026-05-24",
  "session": "full",
  "points": [
    {"Time": "09:30", "Name": "半导体", "Net": 5.2, "Rate": 3.93}
  ]
}
```

#### Tick 日期列表

```
GET /api/tick/dates
```

**响应：** `{"dates": ["2026-05-24", "2026-05-23"]}`

#### SSE 实时流

```
GET /api/tick/stream
```

按 SSE 协议推送 JSON 行：

```
{"type":"tick","text":"{\"points\":[...],\"date\":\"2026-05-24\",\"running\":true,\"count\":1,\"lastTime\":\"09:30\"}"}
{"type":"heartbeat","text":""}
```

| 消息类型 | 说明 |
|---------|------|
| `tick` | Tick 数据快照，text 为 `TickSnapshot` JSON |
| `heartbeat` | 每 15 秒心跳保活 |

#### SSE 历史回放

```
GET /api/tick/replay-stream?date=2026-05-24
```

按 200ms 间隔回放该日所有时间点。消息格式与实时流相同，结束时发送：

```
{"type":"replay_done","text":"回放完成"}
```

#### Tick 事件分析

```
GET /api/tick/events/:date?session=full|morning
```

**响应：**
```json
{
  "timeline": [
    {"time": "09:30", "timeMinutes": 0, "sector": "半导体", "title": "大幅流入", "description": "...", "sentiment": "positive"}
  ],
  "events": [...],
  "ticker": [...]
}
```

---

### 2.7 视频生成

#### 生成 Tick 曲线视频（SSE 流式）

```
POST /api/generate-tick
Content-Type: application/json

{
  "date": "2026-05-24",
  "session": "full",
  "copy_mode": "ai|template"
}
```

SSE 消息事件：

| type | 说明 |
|------|------|
| `log` | 进度日志 |
| `progress` | 进度文本 |
| `error` | 错误信息 |
| `done` | 生成完成 |

#### 生成多日 Bar Chart Race（SSE 流式）

```
POST /api/generate-multiday
Content-Type: application/json

{
  "date": "2026-05-24",
  "days": 3,
  "copy_mode": "ai|template"
}
```

消息格式同上。

---

### 2.8 文案

#### AI 优化文案

```
POST /api/optimize-copy
Content-Type: application/json

{"date": "2026-05-24", "session": "full"}
```

**响应：** `{"text": "📊 05月24日 全天资金流向..."}`

---

### 2.9 异步导出

#### 启动导出

```
GET /api/export-all/:date
```

**响应：** `{"task_id": "exp_20260524_1234567890", "status": "pending"}`

#### 查询导出状态

```
GET /api/export-all/status/:task_id
```

**响应：**
```json
{
  "task_id": "exp_20260524_...",
  "status": "done|running|error",
  "progress": "第5页...",
  "error": ""
}
```

#### 下载导出文件

```
GET /api/export-all/file/:task_id
```

返回 CSV 文件下载。

---

### 2.10 新闻系统

#### 新闻列表（分页）

```
GET /api/news?limit=50&offset=0
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `limit` | 50 | 每页条数（最大 200） |
| `offset` | 0 | 偏移量 |

**响应：**
```json
{
  "records": [
    {
      "id": 2380085,
      "title": "财联社5月24日电，乌克兰总理表示...",
      "content": "财联社5月24日电，乌克兰总理表示...",
      "brief": "...",
      "level": "C",
      "reading_num": 45788,
      "ctime": "2026-05-24 17:10:48",
      "shareurl": "https://api3.cls.cn/share/article/2380085",
      "sectors": "[\"环球市场情报\",\"俄乌冲突快报\"]",
      "created_at": "2026-05-24 17:10:50"
    }
  ],
  "total": 500,
  "limit": 50,
  "offset": 0
}
```

`sectors` 为 JSON 字符串，前端需 `JSON.parse`。

#### 新闻搜索

```
GET /api/news/search?q=半导体&limit=50&offset=0
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `q` | 是 | 搜索关键词（匹配标题和正文） |
| `limit` | 否 | 默认 50 |
| `offset` | 否 | 默认 0 |

**响应：**
```json
{
  "records": [...],
  "total": 12,
  "q": "半导体",
  "limit": 50,
  "offset": 0
}
```

#### 轮询调度器状态

```
GET /api/news/status
```

**响应：**
```json
{
  "status": "running|stopped",
  "total_news": 500,
  "last_poll": "2026-05-24T17:10:50+08:00",
  "last_count": 8
}
```

#### 启动/停止轮询

```
POST /api/news/start
POST /api/news/stop
```

**响应：** `{"ok": true, "message": "新闻轮询已启动"}`

---

### 2.11 视频文件下载

```
GET /output/:date/:file
```

下载 `output/{date}/{file}.mp4` 文件。

---

## 三、Remotion 渲染桥接

系统不直接调用视频渲染库，而是通过 `npx remotion render` CLI 桥接，将 Go 端数据序列化为 JSON props 传递给 React 组件。

| 项目 | 值 |
|------|-----|
| 渲染命令 | `npx remotion render {entry} {compID} {output} --props '{json}' --overwrite --fps 30 --frames 0-899` |
| 入口文件 | `../a-share-flow-video-web/src/renderer/index.ts` |
| 工作目录 | Remotion 前端项目根目录 |
| 帧率 | 30 FPS |
| 总帧数 | 900（30 秒视频） |

### 3.1 单日视频 — RenderProps

| 字段 | 类型 | 说明 |
|------|------|------|
| `dateStr` | string | 日期 `2026-05-24` |
| `displayDate` | string | 显示用 `05-24` |
| `totalFrames` | int | 固定 900 |
| `sectors` | `SectorData[]` | 板块列表 `{name, net, rate, color}` |
| `timelineEvents` | `TimelineEvent[]` | 时间线事件 |
| `tickerItems` | `TickerItem[]` | 底部滚动资讯 |
| `events` | `MarketEvent[]` | 底部弹窗事件 |
| `format` | string | `mobile`=1080×1920, `tv`=1920×1080 |
| `width` | int | 1080 / 1920 |
| `height` | int | 1920 / 1080 |
| `session` | string | `morning` / `full` |
| `xLim` | `[2]int` | 交易 X 轴范围 `[0, 120]` 或 `[0, 330]` |

**组件映射：**

| format | Composition ID | 分辨率 |
|--------|---------------|--------|
| mobile | `BloombergVideo` | 1080×1920 |
| tv | `BloombergVideoTV` | 1920×1080 |

### 3.2 Tick 曲线视频 — TickRenderProps

| 字段 | 类型 | 说明 |
|------|------|------|
| `dateStr` | string | 日期 |
| `displayDate` | string | 显示日期 |
| `totalFrames` | int | 900 |
| `sectorTicks` | `SectorTick[]` | 逐板块时间序列曲线 `{name, color, data[], times[], rate}` |
| `timelineEvents` | `TimelineEvent[]` | 时间线事件 |
| `tickerItems` | `TickerItem[]` | 滚动资讯 |
| `events` | `MarketEvent[]` | 弹窗事件 |
| `format` | string | `mobile` / `tv` |
| `width` | int | 分辨率宽 |
| `height` | int | 分辨率高 |
| `session` | string | `morning` / `full` |
| `xLim` | `[2]int` | 交易时间范围 |

**组件映射：**

| format | Composition ID |
|--------|---------------|
| mobile | `BloombergVideoTick` |
| tv | `BloombergVideoTickTV` |

### 3.3 多日 Bar Chart Race — MultiDayRenderProps

| 字段 | 类型 | 说明 |
|------|------|------|
| `dates` | `string[]` | 交易日列表 |
| `fullDates` | `string[]` | 完整日期（含周末占位） |
| `snapshots` | `BarSnapshot[]` | 逐 tick 快照 `{date, time, bars[]}` |
| `analysis` | `MultiDayAnalysis` | 趋势分析结果 |
| `format` | string | `mobile` / `tv` |
| `width` | int | 分辨率宽 |
| `height` | int | 分辨率高 |
| `totalFrames` | int | 900 |

**组件映射：**

| format | Composition ID |
|--------|---------------|
| mobile | `BloombergVideo3Day` |
| tv | `BloombergVideo3DayTV` |

---

## 四、参数约定

### 4.1 日期格式

| 用途 | 格式 | 示例 |
|------|------|------|
| API 参数 | `YYYY-MM-DD` | `2026-05-24` |
| 显示用 | `MM-DD` | `05-24` |
| Tick 时间 | `HH:MM` | `09:30` |
| 全量板块 datetime | `YYYY-MM-DD` | `2026-05-24` |
| 新闻 ctime | `YYYY-MM-DD HH:MM:SS` | `2026-05-24 17:10:48` |

### 4.2 数值单位

| 数据 | 单位 | 示例 |
|------|------|------|
| 主力净流入 `net` | 亿元（已除 1e8） | `5.2` = 5.2 亿 |
| 主力净占比 `rate` | 百分比 | `3.93` = 3.93% |

### 4.3 请求头

前端所有请求需 CORS 支持。后端通过 `.env` 配置：

```
CORS_ALLOWED_ORIGINS=https://your-frontend.com
```

未配置时不允许跨域。
