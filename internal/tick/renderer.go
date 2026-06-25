package tick

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
)

type SectorTick struct {
	Name                 string    `json:"name"`
	Color                string    `json:"color"`
	Data                 []float64 `json:"data"`
	Times                []string  `json:"times"`
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
	ChartNarrationSegments []int                    `json:"chartNarrationSegments,omitempty"`
	ChartNarrationTexts    []string                 `json:"chartNarrationTexts,omitempty"`

	// LLM 增强字段（可选，为空时前端 fallback 到模板逻辑）
	CatalysisResult    *CatalysisResult    `json:"catalysisResult,omitempty"`
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

	topLimit := 3
	if len(sectorTicks) < topLimit {
		topLimit = len(sectorTicks)
	}

	var points []inflection
	minGap := int(math.Max(2, math.Floor(float64(numPoints)*0.1)))
	for si := 0; si < topLimit; si++ {
		st := sectorTicks[si]
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
	if len(points) > 0 {
		for _, p := range points {
			action := ChartNarrationActionAccelerate
			direction := ChartNarrationDirectionInflow
			if p.delta < 0 {
				action = ChartNarrationActionWeaken
				direction = ChartNarrationDirectionNetOutflow
			}
			timePrefix := ""
			if p.time != "" {
				timePrefix = p.time + "，"
			}
			texts = append(texts, fmt.Sprintf(ChartNarrationTmplInflection, timePrefix, p.name, action, direction, math.Abs(p.delta), p.cum))
			startFrame := int(math.Floor(float64(p.idx) / float64(numPoints) * float64(totalFrames)))
			startFrame -= FPS
			if startFrame < 0 {
				startFrame = 0
			}
			startFrames = append(startFrames, startFrame)
		}
		return texts, startFrames
	}

	type segSum struct {
		name string
		net  float64
	}

	contentPoints := []float64{0.07, 0.40, 0.70}
	playPoints := []float64{0, 0.40, 0.70}

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
		for j := 0; j < 2 && j < len(sums); j++ {
			if math.Abs(sums[j].net) < 0.5 {
				continue
			}
			sign := ChartNarrationDirectionInflow
			if sums[j].net < 0 {
				sign = ChartNarrationDirectionNetOutflow
			}
			top2 = append(top2, fmt.Sprintf(ChartNarrationSectorFmt, sums[j].name, sign, math.Abs(sums[j].net)))
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

		texts = append(texts, text)
		startFrames = append(startFrames, int(math.Floor(playPoints[i]*float64(totalFrames))))
	}

	return texts, startFrames
}

func RenderTickVideo(dateStr, outputPath, format, session string, events []analyzer.MarketEvent, timeline []analyzer.TimelineEvent, ticker []analyzer.TickerItem, copywriteText string, newsPages []hotnews.NewsPage) (string, error) {
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

	sectorTicks := buildSectorTicks(points)
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
			zap.Int("timePoints", len(sectorTicks[0].Times)),
			zap.Strings("top3", topNames),
			zap.Int("totalDataPoints", len(sectorTicks)*len(sectorTicks[0].Times)))
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
	catalysisResult := GenerateCatalysis(sectorTicks, newsPages)
	if catalysisResult != nil {
		logger.Info("资金催化 LLM 分析完成",
			zap.Int("sectors", len(catalysisResult.Sectors)))
	}

	// LLM 主线结构收尾分析
	mainStructureResult := GenerateMainStructure(sectorTicks)
	if mainStructureResult != nil {
		logger.Info("主线结构收尾 LLM 分析完成",
			zap.String("conclusion", mainStructureResult.Conclusion))
	}

	newsPages = reorderNewsPagesByFlow(newsPages, sectorTicks, format)

	props := TickRenderProps{
		DateStr:             dateStr,
		DisplayDate:         displayDate,
		TotalFrames:         TotalFrames,
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
		voiceoverDir := filepath.Join(config.GetRendererDir(), "public", "voiceover")
		if err := os.MkdirAll(voiceoverDir, 0755); err != nil {
			return "", fmt.Errorf("create voiceover dir: %w", err)
		}

		if copywriteText != "" {
			scenes := tts.ParseCopywriting(copywriteText)
			sceneNames := []string{"hook1", "suspense", "twist", "answer", "hook2"}
			sceneTexts := []string{props.Scene1Text, props.Scene2Text, props.Scene3Text, props.Scene4Text, props.Scene5Text}

			for i := 0; i < 5; i++ {
				if scenes[i] == "" {
					continue
				}
				audioPath := filepath.Join(voiceoverDir, fmt.Sprintf("%s.mp3", sceneNames[i]))
				if err := tts.TextToSpeechCommentator(scenes[i], audioPath); err != nil {
					logger.Warn("Tick 场景 TTS 合成失败", zap.Int("scene", i+1), zap.Error(err))
					continue
				}
				dur := 0.0
				if _, err := os.Stat(audioPath); err == nil {
					if d, err := tts.GetAudioDuration(audioPath); err == nil {
						dur = d
					}
				}
				frames := int(math.Ceil(dur * FPS))
				const audioPadding = 10
				if frames > 0 {
					frames += audioPadding
				}
				sceneTexts[i] = scenes[i]
				switch i {
				case 0:
					props.Scene1Text = scenes[i]
					props.Scene1Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene1Frames = frames
				case 1:
					props.Scene2Text = scenes[i]
					props.Scene2Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene2Frames = frames
				case 2:
					props.Scene3Text = scenes[i]
					props.Scene3Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene3Frames = frames
				case 3:
					props.Scene4Text = scenes[i]
					props.Scene4Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene4Frames = frames
				case 4:
					props.Scene5Text = scenes[i]
					props.Scene5Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene5Frames = frames
				}
			}

			logger.Info("Tick 文案语音合成完成",
				zap.Int("scenes", 5))
		}

		if len(newsPages) > 0 {
			ttsTexts := hotnews.GenerateTTSText(newsPages)
			newsNarrationTexts = ttsTexts
			for i, text := range ttsTexts {
				newsPath := filepath.Join(voiceoverDir, fmt.Sprintf("news_%d.mp3", i))
				if err := tts.TextToSpeechCommentator(text, newsPath); err != nil {
					logger.Warn("Tick 新闻 TTS 合成失败，跳过", zap.Int("page", i), zap.Error(err))
					continue
				}
				dur := 0.0
				if d, err := tts.GetAudioDuration(newsPath); err == nil {
					dur = d
				}
				frames := int(math.Ceil(dur * FPS))
				if frames > 0 {
					frames += 10
				}
				newsAudioFiles = append(newsAudioFiles, fmt.Sprintf("voiceover/news_%d.mp3", i))
				newsAudioFrames = append(newsAudioFrames, frames)
				newsTotalFrames += frames
			}
			logger.Info("Tick 新闻语音合成完成",
				zap.Int("pages", len(newsAudioFiles)),
				zap.Int("newsTotalFrames", newsTotalFrames))
		}

		props.ChartNarrationSegments = chartNarrationSegments
		props.ChartNarrationTexts = chartNarrationTexts
	}

	sceneTotalFrames := props.Scene1Frames + props.Scene2Frames + props.Scene3Frames + props.Scene4Frames + props.Scene5Frames
	conclusionFrames := 0
	if len(sectorTicks) > 0 {
		conclusionFrames = 90
	}
	totalVideoFrames := sceneTotalFrames + baseFrames + newsTotalFrames + conclusionFrames

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

	cmd := exec.Command("npx", args...)
	cmd.Dir = rendererDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
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

func buildSectorTicks(points []TickPoint) []SectorTick {
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
			Times:                timeOrder,
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

	return result
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
