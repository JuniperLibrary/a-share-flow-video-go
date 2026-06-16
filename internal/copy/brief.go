package copy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/storage"
)

// NarrativeBrief 是多源数据聚合后的叙事素材包，供 LLM 二次写作。
type NarrativeBrief struct {
	Date           string
	Session        string
	MarketSummary  string
	InflowLeaders  string
	OutflowLeaders string
	Structure      string
	IntradayPath   string
	Continuity     string
	NewsContext    string
	KeyMoments     string
	Angles         []string
	PrevPrediction string
}

func (b *NarrativeBrief) Format() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 日期 %s · %s\n\n", b.Date, sessionLabel(b.Session)))
	sb.WriteString("### 市场概览\n")
	sb.WriteString(b.MarketSummary + "\n\n")
	sb.WriteString("### 流入龙头\n")
	sb.WriteString(b.InflowLeaders + "\n\n")
	sb.WriteString("### 流出龙头\n")
	sb.WriteString(b.OutflowLeaders + "\n\n")
	sb.WriteString("### 资金结构\n")
	sb.WriteString(b.Structure + "\n\n")
	if b.IntradayPath != "" {
		sb.WriteString("### 分时路径\n")
		sb.WriteString(b.IntradayPath + "\n\n")
	}
	if b.Continuity != "" {
		sb.WriteString("### 连续性（近几个交易日）\n")
		sb.WriteString(b.Continuity + "\n\n")
	}
	if b.NewsContext != "" {
		sb.WriteString("### 关联资讯\n")
		sb.WriteString(b.NewsContext + "\n\n")
	}
	if b.KeyMoments != "" {
		sb.WriteString("### 盘中关键节点\n")
		sb.WriteString(b.KeyMoments + "\n\n")
	}
	if len(b.Angles) > 0 {
		sb.WriteString("### 可用叙事角度\n")
		for _, a := range b.Angles {
			sb.WriteString("- " + a + "\n")
		}
		sb.WriteString("\n")
	}
	if b.PrevPrediction != "" {
		sb.WriteString("### 昨日预判\n")
		sb.WriteString(b.PrevPrediction + "\n")
	}
	return sb.String()
}

func sessionLabel(session string) string {
	if cfg, ok := configSessionTitle(session); ok {
		return cfg
	}
	return session
}

func configSessionTitle(session string) (string, bool) {
	titles := map[string]string{"morning": "早盘", "full": "全天"}
	t, ok := titles[session]
	return t, ok
}

// BuildNarrativeBrief 从板块快照 + DB 多源数据构建叙事素材包。
func BuildNarrativeBrief(sectors []fetcher.Sector, dateStr, session, prevPrediction string) (*NarrativeBrief, error) {
	inflows, outflows, netTotal, totalSuper, totalBig := splitSectorFlows(sectors)

	brief := &NarrativeBrief{
		Date:           dateStr,
		Session:        session,
		MarketSummary:  fmt.Sprintf("主力净流向 %+.0f 亿，%d 个板块净流入、%d 个净流出。", netTotal, len(inflows), len(outflows)),
		InflowLeaders:  formatTopFlows(inflows, true, 5),
		OutflowLeaders: formatTopFlows(outflows, false, 3),
		Structure:      buildStructureBlock(netTotal, totalSuper, totalBig, inflows, sectors),
		PrevPrediction: strings.TrimSpace(prevPrediction),
	}

	if len(inflows) > 0 {
		brief.IntradayPath = buildIntradayPath(dateStr, session, inflows[0].Name)
		brief.Continuity = buildContinuity(dateStr, inflows)
		brief.Angles = deriveAngles(inflows, outflows, netTotal, totalSuper)
	}

	brief.NewsContext = loadNewsContext(dateStr, inflows)
	brief.KeyMoments = loadKeyMoments(dateStr, session)

	return brief, nil
}

func buildIntradayPath(dateStr, session, leader string) string {
	db, err := storage.Get()
	if err != nil {
		return ""
	}
	snapshots, err := db.LoadDaySnapshots(dateStr)
	if err != nil || len(snapshots) < 2 {
		return ""
	}

	type point struct {
		time string
		net  float64
	}
	var series []point
	for _, snap := range snapshots {
		t := storage.ExtractTime(snap.Datetime)
		if t == "" || !inSessionTime(t, session) {
			continue
		}
		for _, s := range snap.Sectors {
			if s.Name == leader {
				series = append(series, point{t, s.Net})
				break
			}
		}
	}
	if len(series) < 2 {
		return ""
	}

	open, close := series[0], series[len(series)-1]
	maxStep := 0.0
	maxStepTime := ""
	for i := 1; i < len(series); i++ {
		d := series[i].net - series[i-1].net
		if absF(d) > absF(maxStep) {
			maxStep = d
			maxStepTime = series[i].time
		}
	}

	lines := []string{
		fmt.Sprintf("%s 分时：%s 累计 %+.0f 亿 → %s 累计 %+.0f 亿（日内变化 %+.0f 亿）",
			leader, open.time, open.net, close.time, close.net, close.net-open.net),
	}
	if absF(maxStep) >= 5 {
		lines = append(lines, fmt.Sprintf("最大单段波动 %s %+.0f 亿", maxStepTime, maxStep))
	}
	if close.net > open.net*1.5 && open.net > 0 {
		lines = append(lines, "午后/尾盘加速吸金")
	} else if close.net < open.net*0.5 && open.net > 50 {
		lines = append(lines, "高位资金有所兑现")
	}
	return strings.Join(lines, "\n")
}

func inSessionTime(t, session string) bool {
	if session == "morning" {
		return t >= "09:30" && t <= "11:30"
	}
	return (t >= "09:30" && t <= "11:30") || (t >= "13:00" && t <= "15:00")
}

func buildContinuity(dateStr string, inflows []sectorFlow) string {
	days, err := fetcher.GetTradingDays(dateStr, 5)
	if err != nil || len(days) < 2 {
		return ""
	}

	db, err := storage.Get()
	if err != nil {
		return ""
	}

	limit := 2
	if len(inflows) < limit {
		limit = len(inflows)
	}

	var lines []string
	for i := 0; i < limit; i++ {
		name := inflows[i].Name
		var nets []string
		for _, d := range days {
			sectors, err := db.LoadFullSectors(d)
			if err != nil {
				continue
			}
			net := 0.0
			for _, s := range sectors {
				if s.Name == name {
					net = s.Net
					break
				}
			}
			short := d[5:]
			nets = append(nets, fmt.Sprintf("%s %+.0f", short, net))
		}
		if len(nets) >= 2 {
			trend := describeTrend(days, name, db)
			lines = append(lines, fmt.Sprintf("%s：%s（%s）", name, strings.Join(nets, " → "), trend))
		}
	}
	return strings.Join(lines, "\n")
}

func describeTrend(days []string, name string, db *storage.DB) string {
	if len(days) < 2 {
		return ""
	}
	prev, last := 0.0, 0.0
	for _, d := range []string{days[len(days)-2], days[len(days)-1]} {
		sectors, _ := db.LoadFullSectors(d)
		for _, s := range sectors {
			if s.Name == name {
				if d == days[len(days)-2] {
					prev = s.Net
				} else {
					last = s.Net
				}
				break
			}
		}
	}
	switch {
	case prev <= 0 && last > 50:
		return "由弱转强"
	case prev > 50 && last > prev*1.3:
		return "加速流入"
	case prev > 50 && last < prev*0.5:
		return "流入放缓"
	case prev > 0 && last < 0:
		return "由入转出"
	case prev < 0 && last < prev:
		return "持续失血"
	default:
		return "趋势延续"
	}
}

func loadNewsContext(dateStr string, inflows []sectorFlow) string {
	db, err := storage.Get()
	if err != nil {
		return ""
	}
	records, _, err := db.LoadNewsByDate(dateStr, 30, 0)
	if err != nil || len(records) == 0 {
		return ""
	}

	hotNames := make(map[string]bool)
	for i, v := range inflows {
		if i >= 3 {
			break
		}
		hotNames[v.Name] = true
	}

	var lines []string
	for _, r := range records {
		if len(lines) >= 3 {
			break
		}
		var sectors []string
		if json.Unmarshal([]byte(r.Sectors), &sectors) != nil {
			continue
		}
		matched := false
		for _, s := range sectors {
			if hotNames[s] {
				matched = true
				break
			}
		}
		if !matched && r.Level != "A" {
			continue
		}
		title := strings.TrimSpace(r.Title)
		if title == "" && r.Content != "" {
			title = truncateRunes(r.Content, 40)
		}
		if title == "" {
			continue
		}
		title = truncateRunes(title, 40)
		lines = append(lines, fmt.Sprintf("- [%s] %s", r.Level, title))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func loadKeyMoments(dateStr, session string) string {
	db, err := storage.Get()
	if err != nil {
		return ""
	}
	raw, err := db.LoadTickEvents(dateStr, session)
	if err != nil || len(raw) == 0 {
		return ""
	}
	var payload struct {
		Timeline []analyzer.TimelineEvent `json:"timeline"`
	}
	if json.Unmarshal(raw, &payload) != nil || len(payload.Timeline) == 0 {
		return ""
	}

	var lines []string
	for i, ev := range payload.Timeline {
		if i >= 4 {
			break
		}
		lines = append(lines, fmt.Sprintf("- %s %s：%s", ev.Time, ev.Sector, ev.Title))
	}
	return strings.Join(lines, "\n")
}

func deriveAngles(inflows, outflows []sectorFlow, netTotal, totalSuper float64) []string {
	var angles []string

	techNet := 0.0
	techN := 0
	for i, v := range inflows {
		if i >= 3 {
			break
		}
		if techSectorNames[v.Name] {
			techNet += v.Net
			techN++
		}
	}
	if techN >= 2 && techNet >= 200 {
		angles = append(angles, fmt.Sprintf("科技链共振（TOP 合计 %.0f 亿）", techNet))
	}

	if len(inflows) > 0 && absF(inflows[0].SuperNet) >= 80 {
		angles = append(angles, fmt.Sprintf("%s 超大单 %.0f 亿，主力级承接", inflows[0].Name, inflows[0].SuperNet))
	}

	if len(outflows) > 0 && len(inflows) > 0 {
		angles = append(angles, fmt.Sprintf("高低切：%s 吸金 vs %s 失血", inflows[0].Name, outflows[0].Name))
	}

	if absF(netTotal) < 200 && len(inflows) > 0 {
		angles = append(angles, "存量博弈，结构重于总量")
	} else if netTotal > 500 {
		angles = append(angles, "增量资金入场，看共识能否扩散")
	}

	maxSuper := 0.0
	if len(inflows) > 0 {
		maxSuper = inflows[0].SuperNet
	}
	if absF(maxSuper) >= 20 && absF(totalSuper) < absF(maxSuper)*0.5 {
		angles = append(angles, "全市场超大单对冲，但龙头有独立定价逻辑")
	}

	return angles
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "…"
}
