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

1. **每轮 8-25 字**(中文计)。Synthesizer 放宽到 80-150 字。
2. **Bull/Bear/Sector/Risk 每轮必须有且仅有一个具体数字**。没数字的轮次等于废牌。Moderator 例外(可调度不引用数字)。
3. **第 3 轮起必须引用对方/上轮的数据**。"你说 X?但 Y 说明..."——不能自说自话。
4. **每个 turn 只传递一个信息点**。不要在一个 turn 里塞两个论点。
5. **不要"总结来说"式收尾**——继续扔新数据或新角度。
6. **不要角色互换**——Bull 永远看增长,不能转 Bear。`

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
