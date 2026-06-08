# RFC: 财报辩论 → 多智能体架构(6 角色 × 7 阶段 Panel Council)

> **Status**: Draft
> **Author**: a-share-flow-agent (hermes-webui 蒸馏 + a-share-flow 现状)
> **Date**: 2026-06-05
> **Target**: a-share-flow-video-go / a-share-flow-video-web
> **Supersedes**: 无(对 `internal/debate/generator.go` 的 2-agent 模式做架构升级)

---

## 1. TL;DR

把 `internal/debate/` 从**单 LLM 调用 + Bull/Bear 乒乓**升级为**显式状态机 + 6 角色 panel + 工具调用 + 跨对话记忆**的多智能体架构。

| 项 | 当前 | 升级后 |
|---|---|---|
| 角色数 | 2(Bull/Bear)| 6(Moderator / Bull / Bear / Sector / Risk / Synthesizer)|
| 编排 | 1 次 LLM 生成 16 轮 | 7 阶段状态机,每步 1 次 LLM |
| 数据来源 | LLM 自说自话 | LLM + 4 个工具(PE/同行业/历史/新闻)|
| 引用 | 无 | 每轮可挂 `Citation[]` |
| 跨对话 | 无 | SQLite 存历史,新辩论注入"上次结论" |
| 渲染阶段 | 4 步 UI | 7 步 UI |

**分 4 期交付**(P0-P4),**P0+P1 = 最小可见升级**。

---

## 2. Motivation(为什么改)

### 2.1 当前痛点(从代码读出)

- `generator.go` 一次 LLM 调用管全部 16 轮 → 无中间校正,易"对称写法"、超轮、忘记引用
- 角色"牛 vs 熊"由 prompt 强约束,无独立 prompt 文件 → 难独立调优
- `parseScript` 有死代码(line 288-290)— 技术债
- `Speaker` 写死仅 2 个 → 加新角色要全文改 10+ 处 switch
- UI 进度条 4 步固定 → 不能反映真实复杂度
- 历史只存 turn 文本 → 跨对话引用无 metadata
- 无工具调用 → LLM 容易"瞎编数字"

### 2.2 价值(为什么值得改)

| 价值 | 度量 |
|---|---|
| 分析质量提升 | 多角度捕捉盲点(行业 / 风控) |
| 事实可信 | 工具调用消除 LLM 幻觉数字 |
| 长期价值 | 跨对话记忆让团队积累"对这只股票的认知" |
| 差异化 | 市场上"AI 财报分析"多单 LLM,多 agent 是壁垒 |
| 与项目契合 | 已有 Debate / AI Config / News,多 agent 是自然下一步 |

---

## 3. Goals & Non-Goals

### 3.1 Goals(P0-P1 必须达成)
- [ ] 引入 Phase 显式状态机
- [ ] 加 Moderator + Synthesizer 2 角色
- [ ] 加 `Citation` / `ToolCall` 数据结构
- [ ] 7 步 UI 进度
- [ ] 4 个 prompt 文件拆分(`prompts/moderator.go` 等)
- [ ] Remotion 主持台 + 6 角色 panel 骨架
- [ ] 向后兼容(老 `GenerateScript` API 仍可工作)

### 3.2 Goals(P2-P4 后续)
- [ ] Sector Specialist + Risk Officer 完整 prompt
- [ ] 工具调用 registry(`FetchMetric` / `CompareToPeers` / `HistoricalContext` / `SearchNews`)
- [ ] 跨对话记忆(`debate_sessions` SQLite 表)
- [ ] 引用浮层在 Remotion 渲染
- [ ] 历史对比卡在 DebatePage

### 3.3 Non-Goals(本期不做)
- 不引入 LangGraph / AutoGen 等外部框架(Go 端无成熟等效,概念借鉴即可)
- 不重写 TTS(`internal/debate/tts.go` 不动)
- 不重写 Remotion 渲染管线(只扩展 Speaker 类型)
- 不做用户级记忆(只股票级)
- 不做实时"边看边改"互动(只批处理)

---

## 4. 目标架构

### 4.1 角色模型(6 个)

```
                  ┌─────────────────┐
                  │   Moderator     │ ← 控制议程、调工具、收尾
                  │     主持        │
                  └─────────────────┘
                          │
       ┌──────────┬───────┴───────┬──────────┐
       │          │               │          │
   ┌───▼───┐  ┌───▼───┐      ┌────▼───┐  ┌───▼───┐
   │  Bull │  │  Bear │      │ Sector │  │  Risk │  ← 4 嘉宾
   │  多头  │  │  空头  │      │ 行业   │  │  风控  │
   └───┬───┘  └───┬───┘      └────┬───┘  └───┬───┘
       │          │               │          │
       └──────────┴───────┬───────┴──────────┘
                          │
                  ┌───────▼───────┐
                  │  Synthesizer  │ ← 最终投资观点(0 数字 0 情绪 1 结论)
                  │     总结       │
                  └───────────────┘
```

### 4.2 阶段状态机(7 个)

```
   ┌──────┐   ┌─────────┐   ┌──────────┐   ┌──────────┐
   │ Open │ → │ Opening │ → │ CrossEx  │ → │ Rebuttal │
   └──────┘   └─────────┘   └──────────┘   └──────────┘
                                                │
   ┌──────────┐   ┌─────────┐   ┌──────────────▼─┐
   │ Synthesis│ ← │ Closing │ ← │   FactCheck    │
   └──────────┘   └─────────┘   └────────────────┘
```

| 阶段 | Speakers | 轮数 | 允许工具 |
|---|---|---|---|
| Open | Moderator | 1 | × |
| Opening | Bull, Bear, Sector, Risk | 4(各 1)| × |
| CrossEx | Bull → Bear → Sector → Risk 循环 | 4-6 | × |
| Rebuttal | 同 CrossEx speakers | 4 | × |
| FactCheck | Moderator 调工具,角色补 1-2 轮 | 2-3 | ✓ |
| Closing | Bull, Bear | 2 | × |
| Synthesis | Synthesizer | 1 | × |

**总轮数**:1 + 4 + 5 + 4 + 3 + 2 + 1 = **~20 轮**(比当前 16 略多,但可控)

### 4.3 Orchestrator 流程

```
Run(report, stockCode)
  │
  ├─ 1. 加载跨对话记忆(Memory.GetRecentSessions)
  │     → 注入 state.MemoryContext
  │
  ├─ 2. for phase in [Open, Opening, ..., Synthesis]:
  │     for speaker in phase.Speakers:
  │       ├─ a. buildRolePrompt(speaker, phase, state)
  │       ├─ b. if phase.AllowTools:
  │       │     runToolLoop(speaker, state) → state.ToolHistory
  │       ├─ c. turn = callLLM(speaker, phase, prompt, state)
  │       ├─ d. validate(turn, phase, speaker)
  │       ├─ e. state.Script.Turns.append(turn)
  │       └─ f. emit SSE event("turn", turn)
  │
  ├─ 3. state.Script.Verdict = lastTurn.Text  (Synthesizer 输出)
  │
  └─ 4. Memory.SaveSession(state.Script)
```

### 4.4 工具调用(Tool Registry)

```go
type ToolRegistry struct {
    FetchMetric       func(stock, metric string) (float64, error)
    CompareToPeers    func(stock, sector string) (map[string]float64, error)
    HistoricalContext func(stock, period string) ([]TickPoint, error)
    SearchNews        func(stock, topic string) ([]NewsItem, error)
}
```

**OpenAI function-calling schema**:

```json
{
  "type": "function",
  "function": {
    "name": "fetch_metric",
    "description": "查询某只股票某项财务指标",
    "parameters": {
      "type": "object",
      "properties": {
        "stock": {"type": "string", "description": "股票代码 如 600519"},
        "metric": {"type": "string", "enum": ["revenue", "net_profit", "pe", "pb", "roe", "gross_margin"]}
      },
      "required": ["stock", "metric"]
    }
  }
}
```

### 4.5 记忆层

| 层 | 范围 | 存储 | 注入时机 |
|---|---|---|---|
| 轮次内 | 同一场辩论 | state.Script.Turns | 下一轮 LLM context |
| 跨对话 | 同只股票 | SQLite `debate_sessions` | 新辩论开头 |
| 用户级 | (P4 之后) | SQLite `user_prefs` | (本期不做)|

**SQLite Schema**:
```sql
CREATE TABLE debate_sessions (
    id TEXT PRIMARY KEY,
    stock_code TEXT NOT NULL,
    stock_name TEXT,
    report_hash TEXT,
    turns_json TEXT,    -- 完整 Script JSON
    verdict TEXT,       -- Synthesizer 输出
    created_at TEXT NOT NULL
);
CREATE INDEX idx_debate_stock_time ON debate_sessions(stock_code, created_at DESC);
```

---

## 5. 类型变化(internal/debate/types.go)

```go
// Speaker: 2 → 6
type Speaker string
const (
    Moderator   Speaker = "moderator"
    Bull        Speaker = "bull"
    Bear        Speaker = "bear"
    Sector      Speaker = "sector"
    Risk        Speaker = "risk"
    Synthesizer Speaker = "synthesizer"
)

// Phase: NEW
type Phase string
const (
    PhaseOpen      Phase = "open"
    PhaseOpening   Phase = "opening"
    PhaseCrossExam Phase = "cross_exam"
    PhaseRebuttal  Phase = "rebuttal"
    PhaseFactCheck Phase = "fact_check"
    PhaseClosing   Phase = "closing"
    PhaseSynthesis Phase = "synthesis"
)

// Turn: 增 phase/citations/toolCalls
type Turn struct {
    Index     int        `json:"index"`
    Phase     Phase      `json:"phase"`
    Speaker   Speaker    `json:"speaker"`
    Text      string     `json:"text"`
    Emotion   string     `json:"emotion,omitempty"`
    Citations []Citation `json:"citations,omitempty"`
    ToolCalls []ToolCall `json:"toolCalls,omitempty"`
}

// Citation: NEW
type Citation struct {
    Source string `json:"source"`           // "report" | "tool:fetch_metric" | "memory:previous_session"
    Ref    string `json:"ref"`              // "revenue 3456亿" / "PE 32x"
    Quote  string `json:"quote,omitempty"`
}

// ToolCall: NEW
type ToolCall struct {
    Name   string         `json:"name"`
    Args   map[string]any `json:"args"`
    Result map[string]any `json:"result,omitempty"`
    Error  string         `json:"error,omitempty"`
}

// Script: 增 stock context + session id + verdict
type Script struct {
    Turns        []Turn   `json:"turns"`
    ReportHash   string   `json:"reportHash"`
    StockCode    string   `json:"stockCode,omitempty"`
    StockName    string   `json:"stockName,omitempty"`
    SessionID    string   `json:"sessionId,omitempty"`
    PreviousRefs []string `json:"previousRefs,omitempty"`
    Verdict      string   `json:"verdict,omitempty"`
}

// RenderProps: 加 4 个新角色名
type RenderProps struct {
    TaskID          string
    ModeratorName   string
    BullName        string
    BearName        string
    SectorName      string
    RiskName        string
    SynthesizerName string
    ReportTitle     string
    Turns           []Turn
    AudioTurns      []AudioTurn
    TotalFrames     int
    Width           int
    Height          int
    Format          string
}
```

---

## 6. Orchestrator 接口(internal/debate/orchestrator.go NEW)

```go
package debate

import (
    "context"
    "github.com/a-share-flow-video-go/internal/config"
)

type Orchestrator struct {
    cfg       config.AIConfig
    memory    *DebateMemory
    tools     *ToolRegistry
    prompts   *PromptRegistry
    emitter   EventEmitter  // SSE
}

type DebateState struct {
    Report       Report
    Script       Script
    StockCode    string
    StockName    string
    SessionID    string
    MemoryCtx    string
    ToolHistory  []ToolCall
}

func (o *Orchestrator) Run(ctx context.Context, state DebateState) (Script, error)
func (o *Orchestrator) RunLegacy(ctx context.Context, report string, structured ...map[string]any) (Script, error) // 向后兼容

// 内部
func (o *Orchestrator) runPhase(ctx context.Context, phase Phase, state *DebateState) error
func (o *Orchestrator) runTurn(ctx context.Context, speaker Speaker, phase Phase, state *DebateState) (Turn, error)
func (o *Orchestrator) buildRolePrompt(speaker Speaker, phase Phase, state DebateState) (string, error)
```

---

## 7. Prompt 设计(internal/debate/prompts/ NEW)

### 7.1 文件结构
```
internal/debate/prompts/
  moderator.go     // 主持 system prompt
  bull.go          // 多头 system prompt(基于现有 debatePrompt 升级)
  bear.go          // 空头
  sector.go        // 行业专家(P2)
  risk.go          // 风控(P2)
  synthesizer.go   // 总结
  common.go        // 共用规则(语言红线、JSON schema)
  builder.go       // 拼装函数(buildRolePrompt)
```

### 7.2 Moderator Prompt 设计要点
- 角色: 不表达观点,只调度
- 输入: 报告 + 当前阶段 + 已完成 transcript
- 输出: 阶段开启陈述(PhaseOpen) / 阶段过渡陈述(PhaseTransition) / 工具调度指令
- 长度: 20-40 字 / 句

### 7.3 Bull/Bear Prompt 升级要点
- 现有 `debatePrompt` 主体保留(规则成熟)
- 加硬要求: 引用上轮数据(强制 Citation)
- 加"数据来源"标注规则
- JSON schema 增 `citations` 字段

### 7.4 Synthesizer Prompt 设计要点
- 零数字(数字都已在 transcript)
- 零情绪(中性)
- 1 结论(明确投资观点: 加仓 / 减仓 / 持有 / 观望)
- 80-150 字

---

## 8. State Machine 转换表

```go
var phasePlan = []PhaseStep{
    {Phase: PhaseOpen,      Speakers: []Speaker{Moderator},           AllowTools: false},
    {Phase: PhaseOpening,   Speakers: []Speaker{Bull, Bear, Sector, Risk}, AllowTools: false},
    {Phase: PhaseCrossExam, Speakers: []Speaker{Bull, Bear, Sector, Risk}, AllowTools: false, Rounds: 5},
    {Phase: PhaseRebuttal,  Speakers: []Speaker{Bull, Bear, Sector, Risk}, AllowTools: false, Rounds: 4},
    {Phase: PhaseFactCheck, Speakers: []Speaker{Moderator, Bull, Bear}, AllowTools: true,  Rounds: 3},
    {Phase: PhaseClosing,   Speakers: []Speaker{Bull, Bear},          AllowTools: false, Rounds: 2},
    {Phase: PhaseSynthesis, Speakers: []Speaker{Synthesizer},         AllowTools: false, Rounds: 1},
}
```

---

## 9. API 变化(internal/web/handlers.go)

### 9.1 现有路由(保留 + 内部走新 Orchestrator)
```
POST   /api/debate/generate           → handleDebateGenerate  (走 RunLegacy,2 角色)
POST   /api/debate/audio              → handleDebateAudio
POST   /api/debate/render             → handleDebateRender
GET    /api/debate/render-status/:id  → handleDebateRenderStatus
...
```

### 9.2 新增路由
```
POST   /api/debate/council/start      → handleDebateCouncilStart   (6 角色)
GET    /api/debate/council/stream/:id → handleDebateCouncilStream  (SSE)
GET    /api/debate/sessions/:code     → handleDebateSessions       (历史列表)
```

### 9.3 SSE 事件协议
```json
{"type": "phase_start", "phase": "opening"}
{"type": "turn", "phase": "opening", "speaker": "bull", "turn": {...}}
{"type": "tool_call", "tool": {"name": "fetch_metric", "args": {"stock": "600519", "metric": "pe"}}}
{"type": "tool_result", "tool": {...}}
{"type": "phase_end", "phase": "opening"}
{"type": "verdict", "text": "..."}
{"type": "error", "message": "..."}
```

---

## 10. 前端影响(a-share-flow-video-web)

### 10.1 DebatePage.tsx 改
- Stage 状态机:`idle → open → opening → ... → audio → render → done`
- 进度条组件:`PhaseIndicator` 接受动态 phase config
- SpeakerCard 组件:`<SpeakerCard speaker={turn.speaker} ... />` 接受 6 角色
- 引用折叠:`<CitationList citations={turn.citations} />`
- 工具面板:`<ToolCallPanel tools={turn.toolCalls} />` (P3)
- 历史对比:`<PreviousDebateCard sessions={...} />` (P4)

### 10.2 App.tsx 改
- DebatePage 改 `staticOnly: false`(全员可见)
- 可选: DebatePage 改名为 CouncilPage(更准),DebatePage 走老 2-agent 兼容

### 10.3 进度条 4 → 7 步
```
[Open] → [Opening] → [CrossEx] → [Rebuttal] → [FactCheck] → [Closing] → [Synthesis]
   ↓
[Audio] → [Render]  (后处理)
```

---

## 11. Remotion 影响(DebateVideo.tsx)

### 11.1 布局变更
- 中央:Moderator 主持台(顶部 1/3)
- 前排左右:Bull / Bear(各占 1/3,保持现状)
- 后排:Sector / Risk(嘉宾卡,各 1/6,靠下方)
- 底部:Synthesizer 收尾(总结阶段显示)

### 11.2 类型扩展
```typescript
type Speaker = 'moderator' | 'bull' | 'bear' | 'sector' | 'risk' | 'synthesizer';

const COLORS: Record<Speaker, { primary: string; deep: string }> = {
  moderator:   { primary: '#b0b8c4', deep: '#86909c' },  // ink-2
  bull:        { primary: '#00d4ff', deep: '#0e7490' },  // primary(沿用)
  bear:        { primary: '#fbbf24', deep: '#b45309' },  // warning(沿用)
  sector:      { primary: '#30c64a', deep: '#0a7a1f' },  // success
  risk:        { primary: '#f53f3f', deep: '#a8201a' },  // destructive
  synthesizer: { primary: '#ff7875', deep: '#a82a26' },  // inflow light
};
```

### 11.3 新组件
- `<ModeratorStage />` — 主持台
- `<SectorStage />` / `<RiskStage />` — 嘉宾卡
- `<PhaseTransition phase={turn.phase} />` — 切阶段动画
- `<CitationOverlay citations={turn.citations} />` — 引用浮层(P3)
- `<ToolCallIndicator tool={turn.toolCalls[0]} />` — 工具动效(P3)
- `<SynthesisFrame />` — 总结阶段大画面

### 11.4 props 扩展
- 加 `moderatorName` / `sectorName` / `riskName` / `synthesizerName`

---

## 12. 向后兼容策略

### 12.1 API 兼容
- `POST /api/debate/generate` 保留,内部走 `Orchestrator.RunLegacy` — 仍然返回老 Script 格式
- 前端 DebatePage 走老 API,新 `CouncilPage` 走 `council/start`
- 旧 `parseScript` 规则保留为 `parseLegacyScript`,新 `parseTurn` 按 speaker + phase 单独校验

### 12.2 数据兼容
- 老 SQLite `debate_history` 表不动(存储结构不变)
- 新 `debate_sessions` 表只用于 Council 模式
- 老 `RenderProps` 字段保留,新字段(4 个角色名)可选

### 12.3 配置兼容
- `OPENAI_BASE_URL` / `OPENAI_API_KEY` / `AI_MODEL` 不变
- `OPENAI_MAX_TOKENS` 可调(老 2000,新建议 4000-6000 容纳更复杂 prompt)
- `DEBATE_PHASE_TIMEOUT` 新增(P1 引入,默认 30s/phase)

---

## 13. 分阶段交付

| 阶段 | 周期 | 内容 | 交付物 | 价值 | 风险 |
|---|---|---|---|---|---|
| **P0 准备** | 2-3 天 | 清死代码 + Phase/Citation/ToolCall 类型 + 单元测试 | `types.go` `parse_test.go` | 技术债清零,数据就位 | 无 |
| **P1 后端骨架** | 3-4 天 | Orchestrator + 6 prompt 文件 + RunLegacy + Moderator/Synthesizer 路径 | `orchestrator.go` `prompts/*.go` | 后端能跑多阶段 | prompt 调优 |
| **P1 前端** | 3-4 天 | DebatePage 7 步进度 + SpeakerCard + 阶段指示 | `DebatePage.tsx` `PhaseIndicator.tsx` `SpeakerCard.tsx` | UI 反映新结构 | 设计细节 |
| **P1 Remotion** | 2-3 天 | DebateVideo 主持台 + 6 角色 panel 骨架 | `DebateVideo.tsx` | 视频端多角色 | Remotion 调试 |
| **P1 集成 + 验证** | 2-3 天 | SSE 联通 + smoke test + Remotion 渲染 | 端到端 | 完整 ship | 集成 bug |
| **P2 Sector + Risk** | 1 周 | 完整 Sector/Risk prompt + Remotion 嘉宾席 | 4 嘉宾齐全 | 多角度分析 | 角色"抢戏" |
| **P3 工具** | 1-2 周 | ToolRegistry + 4 个 tool + tool loop + SSE 事件 | 工具化 | 数据真,LLM 不编 | tool 错误 |
| **P4 记忆** | 1 周 | debate_sessions 表 + 注入 + UI 对比 | 跨对话 | 长期价值 | 隐私 |

**P0 + P1 = 最小可见升级**(可独立 ship 给用户)

---

## 14. 风险与缓解

| 风险 | 量化 | 缓解 |
|---|---|---|
| LLM 成本 | 6 角色 × ~20 轮 ≈ 6× 当前 | 用户选"标准 vs 深度";P1 默认走 4 角色(M+B+Sector+Risk),P2 加 Synthesizer |
| 延迟 | 串行 30s → 90-120s | Opening 阶段可并行 Bull/Bear;FactCheck 异步工具 |
| 提示工程 | 6 prompt × 1-2 天 = 6-12 天 | 优先 P1 验证再上 P2;借鉴 Hermes Agent 系统化 prompt 实践 |
| 质量 ≠ 数量 | 多 agent 不一定更好 | A/B harness(C 任务)先验证 P1 是否比 2-agent 好 |
| Remotion 重构 | ~2 周 | P1 用 2 主持台 + 4 嘉宾占位简化版 |
| 跨对话隐私 | SQLite 存历史 | 加"清除历史"按钮 + 加密 stock_code 关联(可选) |
| 向后兼容 | 现有数据/UI 依赖偶数交替 | P0 加兼容层;老 API 走 RunLegacy |

---

## 15. 验收标准

> **2026-06-05 验收状态**: P0–P4 全部完成(abtest 真跑通: legacy 12 turns / council 63 turns, 6 角色全覆盖, hasCitations=true / hasPhase=true)

### P0
- [x] `parseScript` 无重复 return(已清死代码)
- [x] `Phase` / `Citation` / `ToolCall` 类型定义 + 编译通过
- [x] 现有 Debate 功能(2-agent)100% 不破(RunLegacy 走老 `GenerateScript`)

### P1
- [x] 7 prompt 文件存在(`common.go` / `moderator.go` / `bull.go` / `bear.go` / `sector.go` / `risk.go` / `synthesizer.go`),各自 `SystemPromptFor(string)` 可调用
- [x] `Orchestrator.Run` 能跑完 7 阶段,产出 63 轮 Script
- [x] `Orchestrator.RunLegacy` 仍产出 12 轮老格式 Script
- [x] DebatePage UI 显示 7 步进度(`PHASE_ORDER` + 进度条)
- [x] Remotion 渲染视频含 Moderator 主持台(`<ModeratorStage />` 顶部居中布局 + `ModeratorIcon` SVG)
- [x] Remotion 4 嘉宾席完整渲染(`SectorIcon` / `RiskIcon` / `SynthesizerIcon` SVG)
- [x] `go build ./...` + `npx tsc --noEmit` + `npm run build` 全绿(5489 modules / 6.73s)

### P2 — 行业对比 tool
- [x] `compare_peers` tool 真实实现(东方财富 `push2.eastmoney.com/api/qt/clist/get?fs=b:` 拉行业头部 10 只 PE/PB)
- [x] Sector Specialist prompt 产出行业对比内容

### P3 — 指标/新闻/风险 tool
- [x] `fetch_metric` 真实实现(`push2.eastmoney.com/api/qt/stock/get?secid=` 拉 PE-TTM/LYR/PB/PEG/PS/总市值,字段 f43/f57/f58/f60/f162/f167/f168/f169/f170/f191/f192)
- [x] `search_news` 真实实现(`np-anotice-stock.eastmoney.com/api/security/ann?stock_list=` 拉近期公告)
- [x] `risk_alerts` 真实实现(`datacenter-web.eastmoney.com/api/data/v1/get?reportName=RPT_DMSK_FN_BALANCE` + 4 阈值:商誉>10% / 应收>20% / 关联购销>5% / 担保>10%)
- [x] Orchestrator FactCheck 阶段自动执行 `turn.ToolCalls`(`executeTools` 串行调用 + 结果注入 `PreviousRefs`)
- [x] Citation 字段在 Script 输出

### P4 — 跨对话记忆
- [x] `DebateMemory` interface + `DebateMemoryNoop` 默认实现
- [x] `DebateMemorySQLite` 真实落盘(`modernc.org/sqlite` 驱动 + `debate_sessions` 表 + SaveSession / GetRecentSessions)
- [x] `DebateMemoryAutoDate` wrapper 自动填 `report_date`
- [x] Orchestrator 启动时 `GetRecentSessions(stockCode, 1)` 预填 `PreviousRefs`(`[历史 verdict ...]` 行)
- [x] Orchestrator 完成后 `SaveSession(SessionEntry{...})`(失败仅 log 不阻塞)

---

## 16. Open Questions(讨论待定)

1. **P1 是否包含 Synthesizer?** — 包含会更完整,但成本 + 风险;不含则可分 2 次 ship
2. **P1 默认 4 角色 还是 6 角色?** — 建议 4(M+B+Sector+Risk),P2 再加 Synthesizer
3. **工具调用的失败 fallback?** — 建议: tool 失败时 LLM 继续,只是不挂 Citation,降级
4. **跨对话记忆注入多少?** — 建议: 上一场 verdict + 最近 3 场 turns 摘要
5. **是否给用户选 Council 模式 / Legacy 模式?** — 建议: P1 默认 Council,UI 加"高级模式"开关

---

## 17. 参考文献(诚实声明)

外部研究因模型问题失败,以下为内化认知,需后续核实:

- **Du et al. 2023** "Improving Factuality and Reasoning in Language Models through Multiagent Debate" (arXiv:2305.14325) — 多 LLM 独立生成 → 互看 → 迭代
- **Liang et al. 2023** "Encouraging Divergent Thinking in LLMs through Multi-Agent Debate"
- **AutoGen** (Microsoft Research) — GroupChat + GroupChatManager
- **LangGraph** (LangChain) — StateGraph 显式状态机
- **CrewAI** — Role/Task/Crew 抽象
- **ChatDev / MetaGPT** — 软件开发多 agent
- **Hermes Agent 自身** — 持久 memory + skills + sub-agent

---

## 18. 变更记录

- 2026-06-05: 创建 RFC — a-share-flow-agent(蒸馏 hermes-webui 核心逻辑 + a-share-flow 现状)
