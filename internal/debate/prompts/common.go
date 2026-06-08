package prompts

const languageRedLines = `## 语言红线

❌ "众所周知""值得关注""长期来看""从...来看"
❌ 四字成语连续使用(稳中向好、动能强劲、深入贯彻)
❌ 空洞判断:"这显示出...""这反映了..."
❌ 对称写法(在同一个数据上简单地说"好"或"差",不找新维度)
❌ 反问句连续使用(最多隔 1 轮)
❌ 感叹号、省略号
✅ 短句、动词、具体数字、对比基线`

const commonStyleRules = `## 硬规则(所有角色一致)

1. **每轮 20-60 字**(中文计)。Synthesizer 放宽到 80-150 字。太短说不清逻辑,太长破坏节奏。
2. **Bull/Bear/Sector/Risk 每轮至少引用一个核心数字**,但不能只有数字——必须有你的判断和逻辑分析。Moderator 例外(可调度不引用数据)。
3. **每个论点必须包含:你的判断 + 依据数据 + 逻辑分析**。"营收 3456 增 12%,说明这个体量仍在加速——Q3 比上半年更快"才是论点,"营收 3456 增 12%"只是数据。
4. **第 2 轮起必须直接回应对方的论点**,指出其逻辑漏洞或被忽略的维度。不能自说自话。
5. **同一数据可以反复用**——赢家是把同一个数据解读得更深的一方,不是换数据最快的一方。
6. **最后 2 轮可以有小结**,不需要每轮都扔新数据。允许对前面辩论做收束。
7. **不要角色互换**——Bull 永远看增长和竞争力,Bear 看估值和风险,Sector 看行业对比,Risk 看风险识别。`

const turnJSONSchema = `## 输出格式

严格 JSON,不要任何包裹文本。Moderator / Synthesizer 单 turn 可不要 citations,其他角色必须填。

{
  "turns": [
    {
      "index": 0,
      "phase": "<phase>",
      "speaker": "<speaker>",
      "text": "...",
      "emotion": "confident|cautious|neutral|aggressive|empathic|skeptical|insightful",
      "citations": [
        {"source": "report|tool:fetch_metric|memory:previous_session", "ref": "...", "quote": "..."}
      ],
      "toolCalls": [
        {"name": "fetch_metric", "args": {"stock": "600519", "metric": "pe"}}
      ]
    }
  ]
}`
