# AGENTS.md — a-share-flow-video-go

## Quick Commands

```bash
# Build Go
go build ./...

# Run tests (skip live API tests)
go test ./... -skip "Live"

# CLI: generate video for today
go run ./cmd/cli/

# CLI: generate for specific date
go run ./cmd/cli/ 2026-05-12

# Web server (default port 8084)
go run ./cmd/web/

# SQLite: manual database initialization
go run ./cmd/initdb/

# Deploy: see DEPLOY.md for GitHub Actions + GitHub Pages setup

# Frontend (separate project: a-share-flow-video-web)
# cd ../a-share-flow-video-web && npm run dev
# cd ../a-share-flow-video-web && npx tsc --noEmit
# cd ../a-share-flow-video-web && npm run build
```

## Architecture

Go backend + React/Remotion frontend. Data flows: 东方财富 API → Go fetcher → analyzer (AI or data-driven) → Remotion renders MP4.

### Go Entry Points
- `cmd/cli/main.go` — CLI video generator
- `cmd/web/main.go` — Web API server (gin, port 8084)
- `cmd/initdb/main.go` — SQLite database manual initialization

### Key Modules
| Module | Purpose |
|--------|---------|
| `internal/fetcher/` | 东方财富 API client, Top21HotSectors filtering, CSV save/load, tick data loading |
| `internal/analyzer/` | AI event generation (AIGenerate) + data-driven fallback (DataDrivenGenerate) |
| `internal/renderer/` | Remotion bridge — serializes props to JSON, calls `npx remotion render` |
| `internal/copy/` | Copywriting generation (template + AI) |
| `internal/scheduler/` | Daily 15:05 auto-run scheduler |
| `internal/web/` | HTTP handlers + SSE streaming |
| `internal/config/` | Central config: FPS=30, TotalFrames=900, resolutions, .env loading |
| `internal/storage/` | SQLite persistence: sectors (datetime+name PK) + copywriting tables |
| `internal/logger/` | zap structured logging: colored terminal, request tracing, panic recovery |
| `internal/tickfetcher/` | Tick collector with observer pattern (Subscribe/GetSnapshot/broadcast) |
| `internal/tickrenderer/` | Tick video rendering based on real tick data curves |
| `internal/tickscheduler/` | Tick auto-start scheduler: 09:28 morning / 12:58 full |

### Frontend (Remotion) — separate project: `a-share-flow-video-web`
- Entry: `src/renderer/index.ts` → `Root.tsx`
- Two compositions: `BloombergVideo` (1080×1920 mobile) and `BloombergVideoTV` (1920×1080)
- Components: Background, Header, Chart, Timeline, RankingPanel, Ticker, Particles, Disclaimer
- Types: `src/renderer/types.ts` — must stay in sync with Go `RenderProps`

### Frontend (Web Pages) — separate project: `a-share-flow-video-web`
- `MarketPage.tsx` — Real-time market chart: SSE fund flow curves + AI anomaly events
- `TickPage.tsx` — Tick collection control + data table + video generation
- `SchedulerPage.tsx` — Dual-time scheduler configuration
- `DataPage.tsx`, `GeneratePage.tsx`, `PreviewPage.tsx`, `ConfigPage.tsx`

## Data Flow

1. **Fetch**: `FetchTop21HotSectors()` calls 东方财富 H5 API (`m:90+t:2`), filters to `Top21HotSectors` list
2. **Analyze**: `AnalyzeAllContent()` tries AI first (180s timeout), falls back to `DataDrivenGenerate()`
3. **Render**: `RenderVideo()` marshals RenderProps → JSON → `npx remotion render` → MP4
4. **Copy**: Generates 文案 files for full/morning/afternoon sessions
5. **Persist**: All sector/tick/copywriting data dual-writes to CSV + SQLite

## Key Constants

- `FPS = 30`, `TotalFrames = 900` (30 second video)
- `X_MAX = 330` (trading minutes: 09:30→15:00 with 11:30-13:00 lunch break)
- `tradingRanges = {{0,120}, {210,330}}` — valid timeMinutes ranges
- AI model: `qwen3.6-plus` via dashscope, timeout 180s

## Sector Struct

```go
type Sector struct {
    Name  string  `json:"name"`
    Net   float64 `json:"net"`   // 亿元 (100M yuan)
    Color string  `json:"color"`
}
```

CSV columns: `name,net` (2 columns only).

## SQLite Schema

```sql
CREATE TABLE sectors (
    datetime TEXT NOT NULL,  -- "2026-05-19 09:30" (tick) or "2026-05-19" (full)
    name     TEXT NOT NULL,
    net      REAL NOT NULL,
    PRIMARY KEY (datetime, name)
);

CREATE TABLE copywriting (
    date    TEXT NOT NULL,
    session TEXT NOT NULL,
    type    TEXT NOT NULL,
    content TEXT NOT NULL,
    PRIMARY KEY (date, session, type)
);
```

Storage helpers: `storage.DateToDatetime(date)` → `"2026-05-19"`, `storage.DateToDatetimeTick(date, time)` → `"2026-05-19 09:30"`, `storage.ExtractDate(datetime)` → `"2026-05-19"`.

## Top21HotSectors

Hardcoded in `fetcher.go`: 半导体, AI应用, CPO概念, 有色金属, 锂矿概念, 商业航天, 电池, 机器人, 创新药, 白酒, 消费电子, 银行, 人工智能, 云计算, 低空经济, 电网设备, 通信设备, 传媒, 国产芯片, 元件, 通信服务

## File Conventions

- Data: `data/YYYY-MM-DD/sectors.csv`
- SQLite: `data/a-share-flow.db`
- Output: `output/YYYY-MM-DD/全天.mp4` (mobile), `全天_tv.mp4` (TV)
- Copywriting: `copy/YYYY-MM-DD/文案_全天.txt`, `文案_ai_全天.txt`
- `.env` is loaded by Go config, not by shell — contains `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `AI_MODEL`

## Testing Quirks

- Live API tests (`TestFetchTop21HotSectors_Live`, `TestFetchHistoricalSectors_Live`, `TestSaveLoadRoundtrip_Live`, `TestColorPaletteAssignment`) require network and trading-day data — skip with `-skip "Live"`
- Unit tests use `config.SetProjectRoot(tmpDir)` for isolation
- `TestFetchTop21HotSectors_Live` expects ≥21 sectors

## Remotion Rendering

- Requires Node.js + npm installed
- Render command: `npx remotion render <entry> <compID> <output> --props '<json>' --overwrite --fps 30 --frames 0-899`
- Entry file: `../a-share-flow-video-web/src/renderer/index.ts`
- Cache: `.remotion-cache/` (gitignored)

## Known Patterns

- `absF`, `roundTo2`, `toFloat64` duplicated across `fetcher.go`, `handlers.go`, `analyzer.go`, `copy.go` — intentional, no shared util package
- `clampTimeline()` corrects AI-generated timeMinutes to valid trading hours
- `DataDrivenGenerate()` produces 10 timeline events, 8 market events, ~10 ticker items
- AI prompt in `AIGenerate()` requests 10-12 timeline events, 8-10 market events, 10-12 ticker items
- SSE streaming uses custom `SSEWriter` with `Send(msgType, text)` — messages are JSON lines, not standard SSE format
- Tick data uses observer pattern: `TickFetcher.Subscribe()` returns channel + unsubscribe function
- SQLite initialization is lazy via `storage.Get()` (sync.Once), called at startup by both CLI and Web
