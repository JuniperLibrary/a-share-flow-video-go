package tick

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tts"
	"go.uber.org/zap"
)

const (
	FPS         = config.FPS
	TotalFrames = config.TotalFrames
	// Keep in sync with the Remotion chart scene overlap so late narration can
	// consume the full available tail without being cut early.
	tickChartOverlapFrames = 15
)

var outroUILabelBannedReplacer = strings.NewReplacer(
	"最终判断", "",
	"主线结构", "",
	"集中度", "前排占比",
	"结构", "资金结构",
	"风险", "风险点",
	"动作", "应对",
	"明日观察", "明天盯盘的点",
	"信号", "信号提示",
	"建议", "思路",
	"：", "，",
	"“", "",
	"”", "",
)

var outroOralPrefixPool = []string{
	"简单和大家说下今天的盘面，",
	"收盘简单复盘一下，",
	"最后再和大家一起过一遍今天的行情，",
	"一句话给今天定个调，",
}

var outroOralEndingPool = []string{
	"，明天我们盘中再看。",
	"，明天开盘一起盯。",
	"，节奏放慢，别急。",
	"，按计划来。",
}

func buildMainStructureOutroText(r *MainStructureResult, sectorTicks []SectorTick) string {
	var leader, pressure *SectorTick
	var totalIn, totalOut float64
	for i := range sectorTicks {
		st := &sectorTicks[i]
		sum := 0.0
		for _, v := range st.Data {
			sum += v
		}
		if sum > 0 {
			totalIn += sum
			if leader == nil || sum > sumSectorTickNet(leader) {
				leader = st
			}
		} else if sum < 0 {
			totalOut += -sum
			if pressure == nil || sum < sumSectorTickNet(pressure) {
				pressure = st
			}
		}
	}
	leaderName := "前排方向"
	if leader != nil {
		leaderName = leader.Name
	}
	pressureName := "流出侧"
	if pressure != nil {
		pressureName = pressure.Name
	}

	pickActions := func() (high, medium string) {
		if r == nil {
			return "", ""
		}
		for i := range r.Actions {
			a := &r.Actions[i]
			if high != "" && medium != "" {
				break
			}
			t := outroUILabelBannedReplacer.Replace(strings.TrimSpace(a.Text))
			t = strings.Trim(t, "，。；,. \t")
			if t == "" {
				continue
			}
			switch strings.ToLower(a.Priority) {
			case "high":
				if high == "" {
					high = t
				}
			case "medium", "mid", "low":
				if medium == "" {
					medium = t
				}
			default:
				if high == "" {
					high = t
				} else if medium == "" {
					medium = t
				}
			}
		}
		if high == "" && r != nil && strings.TrimSpace(r.Action) != "" {
			high = outroUILabelBannedReplacer.Replace(strings.TrimSpace(r.Action))
			high = strings.Trim(high, "，。；,. \t")
		}
		return high, medium
	}

	cleanOralSentence := func(s string) string {
		s = outroUILabelBannedReplacer.Replace(strings.TrimSpace(s))
		s = strings.Trim(s, "，。；,. \t")
		s = strings.Join(strings.Fields(s), " ")
		s = removeChineseInnerSpaces(s)
		return s
	}

	prefix := outroOralPrefixPool[0]
	ending := outroOralEndingPool[0]

	if r == nil {
		if leader != nil {
			high := fmt.Sprintf("盯%s能不能继续承接", leaderName)
			medium := fmt.Sprintf("看%s流出能不能收敛", pressureName)
			return fmt.Sprintf("%s今天就是%s撑住前排，%s还在被兑现。操作上先%s，再%s%s",
				prefix, leaderName, pressureName, high, medium, ending)
		}
		cumulative := make(map[string][]float64)
		for _, st := range sectorTicks {
			if len(st.Data) == 0 {
				continue
			}
			cum := make([]float64, len(st.Data))
			sum := 0.0
			for i, v := range st.Data {
				sum += v
				cum[i] = sum
			}
			cumulative[st.Name] = cum
		}
		return buildConclusionFromCumulative(cumulative)
	}

	highAction, mediumAction := pickActions()
	if highAction == "" {
		highAction = fmt.Sprintf("盯%s承接", leaderName)
	}
	if mediumAction == "" {
		mediumAction = fmt.Sprintf("看%s流出收敛", pressureName)
	}
	highAction = strings.Join(strings.Fields(highAction), " ")
	mediumAction = strings.Join(strings.Fields(mediumAction), " ")

	hasLLMOral := strings.TrimSpace(r.Recap) != "" || strings.TrimSpace(r.PlanActions) != ""
	if !hasLLMOral {
		conclusion := cleanOralSentence(r.Conclusion)
		outlook := cleanOralSentence(r.Outlook)
		signal := cleanOralSentence(r.Signal)

		var buf strings.Builder
		buf.WriteString(prefix)
		if conclusion != "" {
			buf.WriteString(conclusion)
			buf.WriteString("。")
		} else {
			buf.WriteString(fmt.Sprintf("今天就是%s在前排撑着，%s还在失血。", leaderName, pressureName))
		}
		if signal != "" {
			buf.WriteString(signal)
			buf.WriteString("。")
		}
		buf.WriteString("操作上先")
		buf.WriteString(highAction)
		buf.WriteString("，再")
		buf.WriteString(mediumAction)
		buf.WriteString("。")
		if outlook != "" {
			buf.WriteString(outlook)
			buf.WriteString("。")
		} else {
			buf.WriteString(strings.TrimSuffix(ending, "。"))
			buf.WriteString("。")
		}
		return trimEndingDots(buf.String())
	}

	recap := cleanOralSentence(r.Recap)
	highlight := cleanOralSentence(r.Highlight)
	risks := cleanOralSentence(r.Risks)
	planIntro := cleanOralSentence(r.PlanIntro)
	planActions := cleanOralSentence(r.PlanActions)

	var buf strings.Builder
	if recap != "" && (strings.HasPrefix(recap, "今天") || strings.HasPrefix(recap, "收盘") || strings.HasPrefix(recap, "简单")) {
		buf.WriteString(recap)
	} else {
		buf.WriteString(prefix)
		if recap != "" {
			buf.WriteString(recap)
		} else {
			buf.WriteString(fmt.Sprintf("今天就是%s在前排撑着，%s还在失血", leaderName, pressureName))
		}
	}
	if highlight != "" {
		buf.WriteString("。")
		buf.WriteString(highlight)
	}
	if risks != "" {
		buf.WriteString("。")
		buf.WriteString(risks)
	}
	if planIntro != "" {
		buf.WriteString("。")
		buf.WriteString(planIntro)
		buf.WriteString("：先")
	} else {
		buf.WriteString("。操作上先")
	}
	if planActions != "" {
		buf.WriteString(planActions)
	} else {
		buf.WriteString(highAction)
		buf.WriteString("，再")
		buf.WriteString(mediumAction)
	}
	buf.WriteString(strings.TrimSuffix(ending, "。"))
	buf.WriteString("。")
	return trimEndingDots(polishOutroText(buf.String()))
}

// polishOutroText 对主线收尾口播做最后 4 个人味细节修补，不动逻辑：
// 1. 去「先优先」「先先」等模板拼接导致的语义重复
// 2. 双动作短语缺逗号时自动补一个中文逗号
// 3. 中间句号（最后一个不算）按稳定 hash 30% 概率换逗号，避免每句都是句号工整并列
// 4. 压内部多余空格，剥「，，」重复标点
func polishOutroText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = removeChineseInnerSpaces(s)
	s = strings.ReplaceAll(s, "先优先", "先")
	s = strings.ReplaceAll(s, "先先", "先")
	s = strings.ReplaceAll(s, "：先优先", "：先")
	s = strings.ReplaceAll(s, "，，", "，")
	s = strings.ReplaceAll(s, "。。", "。")

	actionWordCommaReplacer := strings.NewReplacer(
		"流出减仓", "流出，减仓",
		"流出观望", "流出，观望",
		"承接观望", "承接，观望",
		"放量承接", "放量，承接",
		"收敛减仓", "收敛，减仓",
		"减仓观望", "减仓，观望",
		"放量延续", "放量，延续",
		"延续回避", "延续，回避",
		"减仓观望", "减仓，观望",
		"放量承接", "放量，承接",
		"承接减仓", "承接，减仓",
		"承接观望", "承接，观望",
		"盯紧承接", "盯紧，承接",
		"盯承接", "盯，承接",
		"减仓保护", "减仓，保护",
		"观望回避", "观望，回避",
	)
	s = actionWordCommaReplacer.Replace(s)

	{
		type token struct {
			text       string
			isPeriod   bool
			origEnding rune
		}
		var toks []token
		seg := ""
		runes := []rune(s)
		for i, r := range runes {
			if r == '。' || r == '，' || r == '；' {
				toks = append(toks, token{text: seg, isPeriod: r == '。', origEnding: r})
				seg = ""
			} else {
				seg += string(r)
			}
			if i == len(runes)-1 && seg != "" {
				toks = append(toks, token{text: seg})
			}
		}
		if len(toks) >= 2 {
			var sb strings.Builder
			for i, t := range toks {
				sb.WriteString(t.text)
				isLast := i == len(toks)-1
				if !isLast && t.isPeriod {
					sum := 0
					for _, r := range []rune(t.text) {
						sum += int(r)
					}
					if sum%10 < 3 {
						sb.WriteRune('，')
					} else {
						sb.WriteRune(t.origEnding)
					}
				} else if !isLast {
					sb.WriteRune(t.origEnding)
				}
			}
			s = sb.String()
		}
	}

	s = strings.ReplaceAll(s, "，，", "，")
	s = strings.ReplaceAll(s, "。。", "。")
	s = strings.ReplaceAll(s, "，。", "。")
	return s
}

func trimEndingDots(s string) string {
	s = strings.TrimRight(s, "。\t \n")
	if strings.HasSuffix(s, "，") {
		s = s[:len(s)-len("，")]
	}
	return s
}

func removeChineseInnerSpaces(s string) string {
	runes := []rune(s)
	if len(runes) < 3 {
		return s
	}
	isHan := func(r rune) bool {
		return r >= 0x4e00 && r <= 0x9fff
	}
	out := make([]rune, 0, len(runes))
	for i, r := range runes {
		if r == ' ' || r == '\t' {
			prevOk := i > 0 && isHan(runes[i-1])
			nextOk := i < len(runes)-1 && isHan(runes[i+1])
			if prevOk && nextOk {
				continue
			}
		}
		out = append(out, r)
	}
	return string(out)
}

var farewellBlessingPool = []string{
	"愿你明天出手都踩在节奏上，持仓稳稳抬轿。",
	"祝愿持仓长阳，账户净值步步抬升。",
	"祝你交易顺利，看准的方向都能走出延续。",
	"愿你明天下单即逢低，卖飞不回头。",
	"愿你行情稳稳，账户新高，每天好心情。",
	"愿你明天主升能拿住，分歧敢低吸，节奏踩得准。",
	"祝账户长红，每一笔出手都有回报。",
	"祝愿明早开盘有承接，尾盘有溢价，持仓皆主升。",
}

var farewellSignoffPool = []string{
	"明天开盘见！",
	"明天我们盘中再见！",
	"明天开盘，不见不散！",
	"祝好，明天见！",
	"明天再战，晚安！",
}

var farewellOralPool = []string{
	"今天就聊到这里，祝大家明天出手顺顺利利，持仓稳稳抬轿，明天开盘见！",
	"今天就到这里，祝各位账户长红，每一笔交易都有回报，明天我们盘中再见！",
	"收盘了，祝大家明天节奏踩得准，承接拿得住，高位舍得跑，明天开盘不见不散！",
	"今天就说到这里，祝愿大家持仓长阳，账户新高，祝好，明天见！",
	"今天复盘就到这里，祝大家明天行情稳稳，出手都踩对时点，明天再战，晚安！",
}

func buildFarewellTexts(displayDate string) (oral string, blessing string, signoff string) {
	seed := 0
	for _, r := range displayDate {
		seed = (seed*31 + int(r)) % 1000003
	}
	if seed < 0 {
		seed = -seed
	}
	oral = farewellOralPool[seed%len(farewellOralPool)]
	blessing = farewellBlessingPool[(seed/7)%len(farewellBlessingPool)]
	signoff = farewellSignoffPool[(seed/11)%len(farewellSignoffPool)]
	return oral, blessing, signoff
}

func sumSectorTickNet(st *SectorTick) float64 {
	if st == nil {
		return 0
	}
	sum := 0.0
	for _, v := range st.Data {
		sum += v
	}
	return sum
}

func absI(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

type SectorTick struct {
	Name                 string    `json:"name"`
	Color                string    `json:"color"`
	Data                 []float64 `json:"data"`
	Times                []string  `json:"times,omitempty"`
	Rate                 float64   `json:"rate"`
	ChangePct            float64   `json:"changePct"`
	SuperNet             float64   `json:"superNet"`
	SuperRate            float64   `json:"superRate"`
	BigNet               float64   `json:"bigNet"`
	BigRate              float64   `json:"bigRate"`
	MainRate             float64   `json:"mainRate"`
	Volume               float64   `json:"volume"`
	Turnover             float64   `json:"turnover"`
	TurnoverRate         float64   `json:"turnoverRate"`
	LeadStockName        string    `json:"leadStockName"`
	LeadStockChangePct   float64   `json:"leadStockChangePct"`
	TotalMarketCap       float64   `json:"totalMarketCap"`
	CirculatingMarketCap float64   `json:"circulatingMarketCap"`
}

type TickRenderProps struct {
	DateStr                string                   `json:"dateStr"`
	DisplayDate            string                   `json:"displayDate"`
	TotalFrames            int                      `json:"totalFrames"`
	Times                  []string                 `json:"times,omitempty"`
	SectorTicks            []SectorTick             `json:"sectorTicks"`
	TimelineEvents         []analyzer.TimelineEvent `json:"timelineEvents,omitempty"`
	TickerItems            []analyzer.TickerItem    `json:"tickerItems,omitempty"`
	Events                 []analyzer.MarketEvent   `json:"events,omitempty"`
	Format                 string                   `json:"format"`
	Width                  int                      `json:"width"`
	Height                 int                      `json:"height"`
	Session                string                   `json:"session"`
	XLim                   [2]int                   `json:"xLim"`
	Scene1Text             string                   `json:"scene1Text,omitempty"`
	Scene2Text             string                   `json:"scene2Text,omitempty"`
	Scene3Text             string                   `json:"scene3Text,omitempty"`
	Scene4Text             string                   `json:"scene4Text,omitempty"`
	Scene5Text             string                   `json:"scene5Text,omitempty"`
	Scene1Audio            string                   `json:"scene1Audio,omitempty"`
	Scene2Audio            string                   `json:"scene2Audio,omitempty"`
	Scene3Audio            string                   `json:"scene3Audio,omitempty"`
	Scene4Audio            string                   `json:"scene4Audio,omitempty"`
	Scene5Audio            string                   `json:"scene5Audio,omitempty"`
	Scene1Frames           int                      `json:"scene1Frames,omitempty"`
	Scene2Frames           int                      `json:"scene2Frames,omitempty"`
	Scene3Frames           int                      `json:"scene3Frames,omitempty"`
	Scene4Frames           int                      `json:"scene4Frames,omitempty"`
	Scene5Frames           int                      `json:"scene5Frames,omitempty"`
	NewsPages              []hotnews.NewsPage       `json:"newsPages,omitempty"`
	NewsAudioFiles         []string                 `json:"newsAudioFiles,omitempty"`
	NewsAudioFrames        []int                    `json:"newsAudioFrames,omitempty"`
	NewsNarrationTexts     []string                 `json:"newsNarrationTexts,omitempty"`
	BaseAnimationFrames    int                      `json:"baseAnimationFrames,omitempty"`
	ChartNarrationAudios   []string                 `json:"chartNarrationAudios,omitempty"`
	ChartNarrationFrames   []int                    `json:"chartNarrationFrames,omitempty"`
	ChartNarrationSegments []int                    `json:"chartNarrationSegments,omitempty"`
	ChartNarrationTexts    []string                 `json:"chartNarrationTexts,omitempty"`
	MainStructureAudio     string                   `json:"mainStructureAudio,omitempty"`
	MainStructureFrames    int                      `json:"mainStructureFrames,omitempty"`
	MainStructureText      string                   `json:"mainStructureText,omitempty"`
	FarewellText           string                   `json:"farewellText,omitempty"`
	FarewellAudio          string                   `json:"farewellAudio,omitempty"`
	FarewellFrames         int                      `json:"farewellFrames,omitempty"`
	FarewellBlessing       string                   `json:"farewellBlessing,omitempty"`
	FarewellSignoff        string                   `json:"farewellSignoff,omitempty"`

	// LLM 增强字段（可选，为空时前端 fallback 到模板逻辑）
	CatalysisResult     *CatalysisResult     `json:"catalysisResult,omitempty"`
	MainStructureResult *MainStructureResult `json:"mainStructureResult,omitempty"`
}

func generateChartNarrationSegments(sectorTicks []SectorTick, totalFrames int) (texts []string, startFrames []int) {
	if len(sectorTicks) == 0 {
		return nil, nil
	}

	numPoints := len(sectorTicks[0].Data)
	if numPoints < 3 {
		return nil, nil
	}

	type inflection struct {
		name  string
		time  string
		idx   int
		delta float64
		cum   float64
	}

	cumulativeAbs := func(st SectorTick) float64 {
		sum := 0.0
		for _, v := range st.Data {
			sum += v
		}
		return math.Abs(sum)
	}

	rankedTicks := append([]SectorTick(nil), sectorTicks...)
	sort.Slice(rankedTicks, func(i, j int) bool {
		return cumulativeAbs(rankedTicks[i]) > cumulativeAbs(rankedTicks[j])
	})

	topLimit := 3
	if len(rankedTicks) < topLimit {
		topLimit = len(rankedTicks)
	}

	var points []inflection
	minGap := int(math.Max(2, math.Floor(float64(numPoints)*0.1)))
	for si := 0; si < topLimit; si++ {
		st := rankedTicks[si]
		if len(st.Data) < 4 {
			continue
		}
		bestIdx := -1
		bestDelta := 0.0
		cum := 0.0
		bestCum := 0.0
		for i, v := range st.Data {
			cum += v
			if i < minGap || i > len(st.Data)-minGap {
				continue
			}
			if math.Abs(v) > math.Abs(bestDelta) {
				bestDelta = v
				bestIdx = i
				bestCum = cum
			}
		}
		if bestIdx < 0 || math.Abs(bestDelta) < 0.5 {
			continue
		}
		tm := ""
		if bestIdx < len(st.Times) {
			tm = st.Times[bestIdx]
		}
		points = append(points, inflection{
			name:  st.Name,
			time:  tm,
			idx:   bestIdx,
			delta: bestDelta,
			cum:   bestCum,
		})
	}

	sort.Slice(points, func(i, j int) bool { return points[i].idx < points[j].idx })
	type segPair struct {
		frame int
		text  string
	}

	type segSum struct {
		name string
		net  float64
	}

	contentPoints := []float64{0.07, 0.40, 0.70}
	playPoints := []float64{0, 0.40, 0.70}

	var baseTexts []string
	var baseFrames []int
	for i := range contentPoints {
		idx := int(math.Floor(contentPoints[i] * float64(numPoints)))
		if idx >= numPoints {
			idx = numPoints - 1
		}

		sums := make([]segSum, 0, len(sectorTicks))
		var total float64
		for _, st := range sectorTicks {
			s := 0.0
			for j := 0; j <= idx; j++ {
				s += st.Data[j]
			}
			sums = append(sums, segSum{name: st.Name, net: s})
			total += s
		}

		sort.Slice(sums, func(a, b int) bool {
			return math.Abs(sums[a].net) > math.Abs(sums[b].net)
		})

		direction := ChartNarrationDirectionInflow
		if total < 0 {
			direction = ChartNarrationDirectionNetOutflow
		}
		absTotal := math.Abs(total)

		var top2 []string
		aligned := make([]string, 0, 2)
		fallback := make([]string, 0, 2)
		for _, item := range sums {
			if math.Abs(item.net) < 0.5 {
				continue
			}
			sign := ChartNarrationDirectionInflow
			if item.net < 0 {
				sign = ChartNarrationDirectionNetOutflow
			}
			entry := fmt.Sprintf(ChartNarrationSectorFmt, item.name, sign, math.Abs(item.net))
			if (total >= 0 && item.net >= 0) || (total < 0 && item.net <= 0) {
				aligned = append(aligned, entry)
				continue
			}
			fallback = append(fallback, entry)
		}
		top2 = append(top2, aligned...)
		if len(top2) < 2 {
			top2 = append(top2, fallback...)
		}
		if len(top2) > 2 {
			top2 = top2[:2]
		}

		var text string
		switch i {
		case 0:
			if len(top2) > 0 {
				text = fmt.Sprintf(ChartNarrationTmplOpen, strings.Join(top2, "、"), direction, absTotal)
			} else {
				text = fmt.Sprintf(ChartNarrationTmplOpenFallback, direction, absTotal)
			}
		case 1:
			if len(top2) > 0 {
				text = fmt.Sprintf(ChartNarrationTmplMid, strings.Join(top2, "、"), direction, absTotal)
			} else {
				text = fmt.Sprintf(ChartNarrationTmplMidFallback, direction, absTotal)
			}
		case 2:
			if len(top2) > 0 {
				text = fmt.Sprintf(ChartNarrationTmplClose, strings.Join(top2, "、"), direction, absTotal)
			} else {
				text = fmt.Sprintf(ChartNarrationTmplCloseFallback, direction, absTotal)
			}
		}

		baseTexts = append(baseTexts, text)
		baseFrames = append(baseFrames, int(math.Floor(playPoints[i]*float64(totalFrames))))
	}

	pairs := make([]segPair, 0, len(baseTexts)+2)
	for i := range baseTexts {
		if strings.TrimSpace(baseTexts[i]) == "" {
			continue
		}
		pairs = append(pairs, segPair{frame: baseFrames[i], text: baseTexts[i]})
	}

	if len(points) > 0 {
		best := make([]inflection, len(points))
		copy(best, points)
		sort.Slice(best, func(i, j int) bool { return math.Abs(best[i].delta) > math.Abs(best[j].delta) })
		if len(best) > 2 {
			best = best[:2]
		}
		for _, p := range best {
			action := ChartNarrationActionAccelerate
			direction := ChartNarrationDirectionInflow
			if p.delta < 0 {
				action = ChartNarrationActionWeaken
				direction = ChartNarrationDirectionNetOutflow
			}
			cumDirection := ChartNarrationDirectionInflow
			if p.cum < 0 {
				cumDirection = ChartNarrationDirectionNetOutflow
			}
			timePrefix := ""
			if p.time != "" {
				timePrefix = p.time + "，"
			}
			startFrame := int(math.Floor(float64(p.idx) / float64(numPoints) * float64(totalFrames)))
			startFrame -= FPS
			if startFrame < 0 {
				startFrame = 0
			}

			tooClose := false
			for _, bf := range baseFrames {
				if absI(startFrame-bf) < 20 {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}
			pairs = append(pairs, segPair{
				frame: startFrame,
				text:  fmt.Sprintf(ChartNarrationTmplInflection, timePrefix, p.name, action, direction, math.Abs(p.delta), cumDirection, math.Abs(p.cum)),
			})
		}
	}

	sort.Slice(pairs, func(i, j int) bool { return pairs[i].frame < pairs[j].frame })
	for _, p := range pairs {
		texts = append(texts, p.text)
		startFrames = append(startFrames, p.frame)
	}
	return texts, startFrames
}

func computeChartNarrationScheduleEnd(startFrames, audioFrames []int) int {
	cursor := 0
	scheduleEnd := 0
	n := len(audioFrames)
	if len(startFrames) < n {
		n = len(startFrames)
	}
	for i := 0; i < n; i++ {
		frames := audioFrames[i]
		if frames <= 0 {
			continue
		}
		from := startFrames[i]
		if from < cursor {
			from = cursor
		}
		end := from + frames
		if end > scheduleEnd {
			scheduleEnd = end
		}
		cursor = end
	}
	return scheduleEnd
}

func RenderTickVideo(ctx context.Context, dateStr, outputPath, format, session string, events []analyzer.MarketEvent, timeline []analyzer.TimelineEvent, ticker []analyzer.TickerItem, copywriteText string, newsPages []hotnews.NewsPage) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", fmt.Errorf("parse date: %w", err)
	}
	displayDate := t.Format("01-02")

	compID := "BloombergVideoTick"
	w, h := config.MobileWidth, config.MobileHeight
	if format == "tv" {
		compID = "BloombergVideoTickTV"
		w, h = config.TVWidth, config.TVHeight
	}

	sessCfg, ok := config.SessionConfigs[session]
	if !ok {
		sessCfg = config.SessionConfigs["full"]
	}

	points, err := LoadTickCSV(dateStr, session)
	if err != nil || len(points) == 0 {
		return "", fmt.Errorf("no tick data for %s session=%s", dateStr, session)
	}
	logger.Info("tick 数据加载",
		zap.Int("records", len(points)),
		zap.String("date", dateStr),
		zap.String("session", session))

	sectorTicks, tickTimes := buildSectorTicks(points)
	sectorTicks = applyDailyToSectorTicks(sectorTicks, tickTimes, dateStr)
	if len(sectorTicks) > 0 {
		topNames := make([]string, 0, 3)
		for i, st := range sectorTicks {
			if i >= 3 {
				break
			}
			topNames = append(topNames, fmt.Sprintf("%s(%dpts)", st.Name, len(st.Data)))
		}
		logger.Info("tick 时序构建",
			zap.Int("sectors", len(sectorTicks)),
			zap.Int("timePoints", len(tickTimes)),
			zap.Strings("top3", topNames),
			zap.Int("totalDataPoints", len(sectorTicks)*len(tickTimes)))
	} else {
		logger.Warn("tick 时序构建为空", zap.String("date", dateStr))
	}

	if len(timeline) == 0 {
		db, dbErr := storage.Get()
		if dbErr == nil {
			cached, _ := db.LoadTickEvents(dateStr, session)
			if cached != nil {
				var payload struct {
					Timeline []analyzer.TimelineEvent `json:"timeline"`
					Events   []analyzer.MarketEvent   `json:"events"`
					Ticker   []analyzer.TickerItem    `json:"ticker"`
				}
				if err := json.Unmarshal(cached, &payload); err == nil && len(payload.Events) > 0 {
					events, timeline, ticker = payload.Events, payload.Timeline, payload.Ticker
					logger.Info("tick 事件从缓存加载", zap.String("date", dateStr), zap.String("session", session))
				}
			}
		}
		if len(events) == 0 {
			events, timeline, ticker = AnalyzeTickContent(points, dateStr, session)
		}
	}
	if len(events) == 0 {
		events = analyzer.GetFallbackEvents(TotalFrames)
		logger.Warn("tick 事件 fallback：使用默认事件",
			zap.String("date", dateStr))
	}

	logger.Info("tick 事件准备就绪",
		zap.Int("events", len(events)),
		zap.Int("timeline", len(timeline)),
		zap.Int("ticker", len(ticker)),
		zap.String("date", dateStr),
		zap.String("session", session))

	// LLM 资金催化分析
	// NOTE: 在连续 LLM 调用之间轻量节流，避免 macOS TIME_WAIT 端口耗尽。
	// throttleMs 可通过 AI_CALL_THROTTLE_MS 环境变量覆盖，默认 250ms。
	throttle := aiCallThrottle()
	if throttle > 0 {
		time.Sleep(throttle)
	}
	catalysisResult := GenerateCatalysis(sectorTicks, tickTimes, newsPages)
	if catalysisResult != nil {
		high := 0
		med := 0
		for _, s := range catalysisResult.Sectors {
			switch s.Confidence {
			case confidenceHigh:
				high++
			case confidenceMedium:
				med++
			}
		}
		logger.Info("资金催化 LLM 分析完成（B方案规则打分+校验）",
			zap.Int("sectors", len(catalysisResult.Sectors)),
			zap.Int("highConfidence", high),
			zap.Int("mediumConfidence", med))
	}

	// LLM 主线结构收尾分析
	if throttle > 0 {
		time.Sleep(throttle)
	}
	mainStructureResult := GenerateMainStructure(sectorTicks)
	if mainStructureResult != nil {
		logger.Info("主线结构收尾 LLM 分析完成",
			zap.String("conclusion", mainStructureResult.Conclusion),
			zap.String("recap", mainStructureResult.Recap),
			zap.String("highlight", mainStructureResult.Highlight),
			zap.String("risks", mainStructureResult.Risks),
			zap.String("plan_intro", mainStructureResult.PlanIntro),
			zap.String("plan_actions", mainStructureResult.PlanActions),
			zap.Any("actions", mainStructureResult.Actions))
	}

	newsPages = reorderNewsPagesByFlow(newsPages, sectorTicks, format)

	props := TickRenderProps{
		DateStr:             dateStr,
		DisplayDate:         displayDate,
		TotalFrames:         TotalFrames,
		Times:               tickTimes,
		SectorTicks:         sectorTicks,
		TimelineEvents:      timeline,
		TickerItems:         ticker,
		Events:              events,
		Format:              format,
		Width:               w,
		Height:              h,
		Session:             session,
		XLim:                sessCfg.XLim,
		CatalysisResult:     catalysisResult,
		MainStructureResult: mainStructureResult,
	}

	baseFrames := config.GetBaseFrames(format)
	chartNarrationTexts, chartNarrationSegments := generateChartNarrationSegments(sectorTicks, baseFrames)
	if len(chartNarrationTexts) > 0 {
		logger.Info("图表解说分段文案生成",
			zap.Int("segments", len(chartNarrationTexts)),
			zap.Strings("texts", chartNarrationTexts))
	}

	var newsTotalFrames int
	var newsAudioFiles []string
	var newsAudioFrames []int
	var newsNarrationTexts []string

	hasVoiceover := copywriteText != "" || len(newsPages) > 0 || len(chartNarrationTexts) > 0

	if hasVoiceover {
		type sceneRes struct {
			ok     bool
			text   string
			audio  string
			frames int
		}
		sceneResults := make([]sceneRes, 5)

		type segRes struct {
			ok     bool
			text   string
			audio  string
			frames int
		}
		chartResults := make([]segRes, len(chartNarrationTexts))

		// 把 catalysisResult 转成 map，给 GenerateTTSText 做音画统一
		var catalysisMap map[string]interface{}
		if catalysisResult != nil {
			catalysisMap = structToMap(catalysisResult)
		}
		ttsTexts := hotnews.GenerateTTSText(newsPages, catalysisMap)
		newsNarrationTexts = ttsTexts
		newsAudioFiles = make([]string, len(ttsTexts))
		newsAudioFrames = make([]int, len(ttsTexts))

		var outroAudio string
		var outroFrames int
		var outroText string

		var farewellText string
		var farewellAudio string
		var farewellFrames int

		limit := tts.TTSConcurrency()
		sem := make(chan struct{}, limit)
		var wg sync.WaitGroup
		run := func(fn func()) {
			wg.Add(1)
			go func() {
				sem <- struct{}{}
				defer func() {
					<-sem
					wg.Done()
				}()
				fn()
			}()
		}

		if copywriteText != "" {
			scenes := tts.ParseCopywriting(copywriteText)
			for i := 0; i < 5; i++ {
				idx := i
				text := strings.TrimSpace(scenes[i])
				if text == "" {
					continue
				}
				run(func() {
					audioRel, audioAbs, err := tts.TextToSpeechCommentatorCached(text)
					if err != nil {
						logger.Warn("Tick 场景 TTS 合成失败", zap.Int("scene", idx+1), zap.Error(err))
						return
					}
					dur := 0.0
					if d, err := tts.GetAudioDuration(audioAbs); err == nil {
						dur = d
					}
					frames := int(math.Ceil(dur * FPS))
					const audioPadding = 6
					if frames > 0 {
						frames += audioPadding
					}
					sceneResults[idx] = sceneRes{ok: true, text: text, audio: audioRel, frames: frames}
				})
			}

			logger.Info("Tick 文案语音合成完成",
				zap.Int("scenes", 5))
		}

		if len(chartNarrationTexts) > 0 {
			for i, text := range chartNarrationTexts {
				idx := i
				trimmed := strings.TrimSpace(text)
				if trimmed == "" {
					continue
				}
				run(func() {
					audioRel, audioAbs, err := tts.TextToSpeechCommentatorCached(trimmed)
					if err != nil {
						logger.Warn("Tick 图表解说 TTS 合成失败，跳过", zap.Int("segment", idx), zap.Error(err))
						return
					}
					dur := 0.0
					if d, err := tts.GetAudioDuration(audioAbs); err == nil {
						dur = d
					}
					frames := int(math.Ceil(dur * FPS))
					const audioPadding = 6
					if frames > 0 {
						frames += audioPadding
					}
					chartResults[idx] = segRes{ok: true, text: trimmed, audio: audioRel, frames: frames}
				})
			}
		}

		if len(newsPages) > 0 {
			for i, text := range ttsTexts {
				idx := i
				trimmed := strings.TrimSpace(text)
				if trimmed == "" {
					continue
				}
				run(func() {
					audioRel, audioAbs, err := tts.TextToSpeechCommentatorCached(trimmed)
					if err != nil {
						logger.Warn("Tick 新闻 TTS 合成失败，跳过", zap.Int("page", idx), zap.Error(err))
						return
					}
					dur := 0.0
					if d, err := tts.GetAudioDuration(audioAbs); err == nil {
						dur = d
					}
					frames := int(math.Ceil(dur * FPS))
					const audioPadding = 4
					if frames > 0 {
						frames += audioPadding
					}
					newsAudioFiles[idx] = audioRel
					newsAudioFrames[idx] = frames
				})
			}
		}

		if len(props.ChartNarrationSegments) == 0 && len(chartNarrationSegments) > 0 {
			props.ChartNarrationSegments = chartNarrationSegments
			props.ChartNarrationTexts = chartNarrationTexts
		}

		if len(sectorTicks) > 0 {
			outroTextLocal := buildMainStructureOutroText(mainStructureResult, sectorTicks)
			outroTextLocal = strings.TrimSpace(outroTextLocal)
			outroText = outroTextLocal
			if outroTextLocal != "" {
				run(func() {
					audioRel, audioAbs, err := tts.TextToSpeechCommentatorCached(outroTextLocal)
					if err != nil {
						logger.Warn("Tick 主线收尾 TTS 合成失败，跳过", zap.Error(err))
						return
					}
					dur := 0.0
					if d, err := tts.GetAudioDuration(audioAbs); err == nil {
						dur = d
					}
					frames := int(math.Ceil(dur * FPS))
					const audioPadding = 10
					if frames > 0 {
						frames += audioPadding
					}
					const minOutroFrames = 240
					if frames < minOutroFrames {
						frames = minOutroFrames
					}
					outroAudio = audioRel
					outroFrames = frames
				})
			}
		}

		farewellOral, farewellBlessing, farewellSignoff := buildFarewellTexts(displayDate)
		farewellText = farewellOral
		run(func() {
			audioRel, audioAbs, err := tts.TextToSpeechCommentatorCached(farewellText)
			if err != nil {
				logger.Warn("Tick 告别祝福 TTS 合成失败，跳过", zap.Error(err))
				return
			}
			dur := 0.0
			if d, err := tts.GetAudioDuration(audioAbs); err == nil {
				dur = d
			}
			frames := int(math.Ceil(dur * FPS))
			const audioPadding = 12
			if frames > 0 {
				frames += audioPadding
			}
			const minFarewellFrames = 180
			if frames < minFarewellFrames {
				frames = minFarewellFrames
			}
			farewellAudio = audioRel
			farewellFrames = frames
			_ = farewellBlessing
			_ = farewellSignoff
		})

		wg.Wait()

		if sceneResults[0].ok {
			props.Scene1Text = sceneResults[0].text
			props.Scene1Audio = sceneResults[0].audio
			props.Scene1Frames = sceneResults[0].frames
		}
		if sceneResults[1].ok {
			props.Scene2Text = sceneResults[1].text
			props.Scene2Audio = sceneResults[1].audio
			props.Scene2Frames = sceneResults[1].frames
		}
		if sceneResults[2].ok {
			props.Scene3Text = sceneResults[2].text
			props.Scene3Audio = sceneResults[2].audio
			props.Scene3Frames = sceneResults[2].frames
		}
		if sceneResults[3].ok {
			props.Scene4Text = sceneResults[3].text
			props.Scene4Audio = sceneResults[3].audio
			props.Scene4Frames = sceneResults[3].frames
		}
		if sceneResults[4].ok {
			props.Scene5Text = sceneResults[4].text
			props.Scene5Audio = sceneResults[4].audio
			props.Scene5Frames = sceneResults[4].frames
		}

		if len(chartResults) > 0 {
			var okAudios []string
			var okFrames []int
			var okSegs []int
			var okTexts []string
			var okIdxs []int
			for i, r := range chartResults {
				if !r.ok || r.frames <= 0 || r.audio == "" {
					continue
				}
				okAudios = append(okAudios, r.audio)
				okFrames = append(okFrames, r.frames)
				okIdxs = append(okIdxs, i)
				if i < len(chartNarrationSegments) {
					okSegs = append(okSegs, chartNarrationSegments[i])
				} else {
					okSegs = append(okSegs, 0)
				}
				okTexts = append(okTexts, r.text)
			}
			props.ChartNarrationAudios = okAudios
			props.ChartNarrationFrames = okFrames
			props.ChartNarrationTexts = okTexts

			narrationTotalFrames := 0
			for _, f := range okFrames {
				narrationTotalFrames += f
			}
			requiredChartFrames := computeChartNarrationScheduleEnd(okSegs, okFrames)
			if requiredChartFrames > 0 {
				requiredChartFrames += 6
			}
			availableChartFrames := baseFrames + tickChartOverlapFrames
			if requiredChartFrames > availableChartFrames {
				oldBaseFrames := baseFrames
				baseFrames = requiredChartFrames - tickChartOverlapFrames
				_, updatedSegments := generateChartNarrationSegments(sectorTicks, baseFrames)
				okSegs = okSegs[:0]
				for _, idx := range okIdxs {
					if idx < len(updatedSegments) {
						okSegs = append(okSegs, updatedSegments[idx])
					} else {
						okSegs = append(okSegs, 0)
					}
				}
				logger.Info("Tick 图表解说延长 baseFrames 以容纳尾段口播",
					zap.Int("oldBaseFrames", oldBaseFrames),
					zap.Int("newBaseFrames", baseFrames),
					zap.Int("narrationFrames", narrationTotalFrames),
					zap.Int("requiredChartFrames", requiredChartFrames),
					zap.Int("availableChartFrames", availableChartFrames),
				)
			}
			props.ChartNarrationSegments = okSegs
			logger.Info("Tick 图表解说语音合成完成",
				zap.Int("segments", len(props.ChartNarrationAudios)))
		}

		if len(newsAudioFrames) > 0 {
			for _, f := range newsAudioFrames {
				newsTotalFrames += f
			}
			pagesOK := 0
			for i := range newsAudioFrames {
				if newsAudioFrames[i] > 0 && newsAudioFiles[i] != "" {
					pagesOK++
				}
			}
			logger.Info("Tick 新闻语音合成完成",
				zap.Int("pages", pagesOK),
				zap.Int("newsTotalFrames", newsTotalFrames))
		}

		if outroFrames > 0 && outroAudio != "" {
			props.MainStructureAudio = outroAudio
			props.MainStructureFrames = outroFrames
			props.MainStructureText = outroText
		} else if outroText != "" && len(sectorTicks) > 0 {
			props.MainStructureText = outroText
		}

		if farewellFrames > 0 && farewellAudio != "" {
			props.FarewellAudio = farewellAudio
			props.FarewellFrames = farewellFrames
			props.FarewellText = farewellText
			_, b, s := buildFarewellTexts(displayDate)
			props.FarewellBlessing = b
			props.FarewellSignoff = s
		} else {
			_, b, s := buildFarewellTexts(displayDate)
			props.FarewellBlessing = b
			props.FarewellSignoff = s
			props.FarewellText = farewellText
			if props.FarewellFrames <= 0 {
				props.FarewellFrames = 180
			}
		}
	}

	sceneTotalFrames := props.Scene1Frames + props.Scene2Frames + props.Scene3Frames + props.Scene4Frames + props.Scene5Frames
	conclusionFrames := 0
	if len(sectorTicks) > 0 {
		if props.MainStructureFrames > 0 {
			conclusionFrames = props.MainStructureFrames
		} else {
			conclusionFrames = 180
		}
	}
	farewellFrames := 0
	if props.FarewellFrames > 0 {
		farewellFrames = props.FarewellFrames
	} else if len(sectorTicks) > 0 {
		farewellFrames = 180
	} else if hasVoiceover {
		farewellFrames = 180
	}
	totalVideoFrames := sceneTotalFrames + baseFrames + newsTotalFrames + conclusionFrames + farewellFrames

	props.BaseAnimationFrames = baseFrames
	props.NewsPages = newsPages
	props.NewsAudioFiles = newsAudioFiles
	props.NewsAudioFrames = newsAudioFrames
	props.NewsNarrationTexts = newsNarrationTexts
	props.TotalFrames = totalVideoFrames

	if hasVoiceover {
		logger.Info("Tick 语音合成完成",
			zap.Int("scene1Frames", props.Scene1Frames),
			zap.Int("scene2Frames", props.Scene2Frames),
			zap.Int("scene3Frames", props.Scene3Frames),
			zap.Int("scene4Frames", props.Scene4Frames),
			zap.Int("scene5Frames", props.Scene5Frames),
			zap.Int("baseFrames", baseFrames),
			zap.Int("newsTotalFrames", newsTotalFrames),
			zap.Int("mainStructureFrames", conclusionFrames),
			zap.Int("farewellFrames", farewellFrames),
			zap.Int("totalFrames", totalVideoFrames))
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("marshal props: %w", err)
	}

	rendererDir := config.GetRendererDir()
	entry := filepath.Join(rendererDir, "src", "renderer", "index.ts")

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	args := []string{
		"remotion", "render",
		entry,
		compID,
		outputPath,
		"--props", string(propsJSON),
		"--overwrite",
		"--fps", fmt.Sprintf("%d", FPS),
		"--frames", fmt.Sprintf("0-%d", props.TotalFrames-1),
		"--bitrate", "8M",
	}

	logger.Info("tick 渲染参数",
		zap.Int("propsSize", len(propsJSON)),
		zap.Int("sectors", len(sectorTicks)),
		zap.Int("totalFrames", props.TotalFrames),
		zap.Int("fps", FPS),
		zap.Int("width", w),
		zap.Int("height", h),
		zap.String("compID", compID),
		zap.String("output", outputPath))

	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Dir = rendererDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// 如果是上下文取消，返回明确的取消原因，避免上层把 ctx 取消当成普通渲染错误
		if ctx.Err() != nil {
			return "", fmt.Errorf("渲染已取消: %w", ctx.Err())
		}
		return "", fmt.Errorf("Remotion tick render failed: %w", err)
	}

	var fileInfo string
	if fi, err := os.Stat(outputPath); err == nil {
		fileInfo = fmt.Sprintf("%.1fMB", float64(fi.Size())/1024/1024)
	}

	logger.Info("tick 渲染完成",
		zap.String("output", outputPath),
		zap.String("fileSize", fileInfo))
	return outputPath, nil
}

func newTickVoiceoverWorkspace(rendererDir string) (string, string, error) {
	jobID := fmt.Sprintf("tick-%d", time.Now().UnixNano())
	publicPrefix := filepath.Join("voiceover", jobID)
	workspaceDir := filepath.Join(rendererDir, "public", publicPrefix)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", "", err
	}
	return workspaceDir, publicPrefix, nil
}

func buildSectorTicks(points []TickPoint) ([]SectorTick, []string) {
	timeOrder := uniqueTimes(points)

	sectorData := make(map[string][]float64)
	sectorPrev := make(map[string]float64)
	sectorLatest := make(map[string]TickPoint)

	for _, p := range points {
		prev := sectorPrev[p.Name]
		delta := p.Net - prev
		sectorData[p.Name] = append(sectorData[p.Name], delta)
		sectorPrev[p.Name] = p.Net
		sectorLatest[p.Name] = p
	}

	var result []SectorTick
	for name, data := range sectorData {
		if len(data) < len(timeOrder) {
			padded := make([]float64, len(timeOrder))
			copy(padded, data)
			data = padded
		}
		latest := sectorLatest[name]
		result = append(result, SectorTick{
			Name:                 name,
			Data:                 data,
			Rate:                 latest.Rate,
			ChangePct:            latest.ChangePct,
			SuperNet:             latest.SuperNet,
			SuperRate:            latest.SuperRate,
			BigNet:               latest.BigNet,
			BigRate:              latest.BigRate,
			MainRate:             latest.MainRate,
			Volume:               latest.Volume,
			Turnover:             latest.Turnover,
			TurnoverRate:         latest.TurnoverRate,
			LeadStockName:        latest.LeadStockName,
			LeadStockChangePct:   latest.LeadStockChangePct,
			TotalMarketCap:       latest.TotalMarketCap,
			CirculatingMarketCap: latest.CirculatingMarketCap,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		sumI := sumAbs(result[i].Data)
		sumJ := sumAbs(result[j].Data)
		return sumI > sumJ
	})

	return result, timeOrder
}

func BuildSectorTicks(points []TickPoint) ([]SectorTick, []string) {
	return buildSectorTicks(points)
}

func applyDailyToSectorTicks(ticks []SectorTick, _ []string, dateStr string) []SectorTick {
	db, err := storage.Get()
	if err != nil {
		return ticks
	}
	daily, err := db.LoadSectorsAll(dateStr)
	if err != nil || len(daily) == 0 {
		return ticks
	}
	byName := make(map[string]storage.SectorAll, len(daily))
	for _, d := range daily {
		byName[d.Name] = d
	}

	for i := range ticks {
		t := &ticks[i]
		if d, ok := byName[t.Name]; ok {
			if d.ChangePct != 0 {
				t.ChangePct = d.ChangePct
			}
			if d.Turnover != 0 {
				t.Turnover = d.Turnover
			}
			if d.TurnoverRate != 0 {
				t.TurnoverRate = d.TurnoverRate
			}
			if d.LeadStockName != "" {
				t.LeadStockName = d.LeadStockName
			}
			if d.LeadStockChangePct != 0 {
				t.LeadStockChangePct = d.LeadStockChangePct
			}
			if d.TotalMarketCap != 0 {
				t.TotalMarketCap = d.TotalMarketCap
			}
			if d.CirculatingMarketCap != 0 {
				t.CirculatingMarketCap = d.CirculatingMarketCap
			}
			if d.Rate != 0 {
				t.Rate = d.Rate
			}
			if d.SuperNet != 0 {
				t.SuperNet = d.SuperNet
			}
			if d.BigNet != 0 {
				t.BigNet = d.BigNet
			}
			if d.SuperRate != 0 {
				t.SuperRate = d.SuperRate
			}
			if d.BigRate != 0 {
				t.BigRate = d.BigRate
			}
			if d.Volume != 0 {
				t.Volume = d.Volume
			}
		}
	}

	sort.SliceStable(ticks, func(i, j int) bool {
		si := 0.5*math.Abs(ticks[i].ChangePct) + 0.2*math.Abs(ticks[i].Rate) + 0.3*ticks[i].Turnover
		sj := 0.5*math.Abs(ticks[j].ChangePct) + 0.2*math.Abs(ticks[j].Rate) + 0.3*ticks[j].Turnover
		if math.Abs(si-sj) > 1e-9 {
			return si > sj
		}
		return sumAbs(ticks[i].Data) > sumAbs(ticks[j].Data)
	})

	return ticks
}

func uniqueTimes(points []TickPoint) []string {
	seen := make(map[string]bool)
	var times []string
	for _, p := range points {
		if !seen[p.Time] {
			seen[p.Time] = true
			times = append(times, p.Time)
		}
	}
	sort.Strings(times)
	return times
}

func timeMinutes(t string) int {
	var h, m int
	fmt.Sscanf(t, "%d:%d", &h, &m)
	base := 9*60 + 30
	val := h*60 + m - base
	if val < 0 {
		val = 0
	}
	if h >= 13 {
		val -= 90
	}
	return val
}

func sumAbs(data []float64) float64 {
	var s float64
	for _, v := range data {
		if v < 0 {
			s -= v
		} else {
			s += v
		}
	}
	return s
}

func reorderNewsPagesByFlow(pages []hotnews.NewsPage, sectorTicks []SectorTick, format string) []hotnews.NewsPage {
	if len(pages) == 0 || len(sectorTicks) == 0 {
		return pages
	}

	flowMap := make(map[string]float64, len(sectorTicks))
	for _, st := range sectorTicks {
		cum := 0.0
		for _, v := range st.Data {
			cum += v
		}
		flowMap[st.Name] = cum
	}

	var allSectors []hotnews.SectorNews
	for _, page := range pages {
		allSectors = append(allSectors, page.Sectors...)
	}

	sort.Slice(allSectors, func(i, j int) bool {
		fi := math.Abs(flowMap[allSectors[i].Sector])
		fj := math.Abs(flowMap[allSectors[j].Sector])
		return fi > fj
	})

	perPage := 2
	if format == "tv" {
		perPage = 3
	}

	var result []hotnews.NewsPage
	for i := 0; i < len(allSectors); i += perPage {
		end := i + perPage
		if end > len(allSectors) {
			end = len(allSectors)
		}
		page := hotnews.NewsPage{Sectors: allSectors[i:end]}
		pageTitles := make(map[string]bool)
		for si := range page.Sectors {
			var deduped []hotnews.NewsItem
			for _, item := range page.Sectors[si].News {
				if pageTitles[item.Title] {
					continue
				}
				pageTitles[item.Title] = true
				deduped = append(deduped, item)
			}
			page.Sectors[si].News = deduped
		}
		result = append(result, page)
	}
	const totalPageCap = 1
	if len(result) > totalPageCap {
		result = result[:totalPageCap]
	}

	return result
}

func structToMap(v interface{}) map[string]interface{} {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// aiCallThrottle LLM 连续调用的间隔节流，默认 250ms；可通过 AI_CALL_THROTTLE_MS 环境变量覆盖。
// macOS 在短时间大量 443 connect 时容易触发 TIME_WAIT 端口耗尽（can't assign requested address），
// 节流可以把并发连接压下来。设置为 0 则关闭节流。
func aiCallThrottle() time.Duration {
	const defaultMs = 250
	if v := os.Getenv("AI_CALL_THROTTLE_MS"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return time.Duration(n) * time.Millisecond
		}
	}
	return defaultMs * time.Millisecond
}
