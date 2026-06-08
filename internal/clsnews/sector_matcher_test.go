package clsnews

import (
	"testing"
)

func TestMatchSectors_NoMatch(t *testing.T) {
	sectors := MatchSectors("今日天气晴好", "北京地区晴转多云，气温适中")
	if len(sectors) != 0 {
		t.Errorf("expected no match, got %v", sectors)
	}
}

func TestMatchSectors_ExactName(t *testing.T) {
	sectors := MatchSectors("半导体板块持续走强", "半导体行业迎来新一轮增长周期")
	if len(sectors) != 1 || sectors[0] != "半导体" {
		t.Errorf("expected [半导体], got %v", sectors)
	}
}

func TestMatchSectors_SynonymMatch(t *testing.T) {
	sectors := MatchSectors("台积电宣布3nm工艺量产", "")
	// 台积电 → 半导体
	if len(sectors) != 1 || sectors[0] != "半导体" {
		t.Errorf("expected [半导体] via synonym, got %v", sectors)
	}
}

func TestMatchSectors_TitleWeighted(t *testing.T) {
	sectors := MatchSectors(
		"大模型训练成本下降",
		"今天天气很好，没有什么特别的消息。",
	)
	// "大模型" → 人工智能（标题命中，应排在前面）
	matched := false
	for _, s := range sectors {
		if s == "人工智能" {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("expected '人工智能' from '大模型' keyword, got %v", sectors)
	}
}

func TestMatchSectors_MultipleMatches(t *testing.T) {
	sectors := MatchSectors(
		"AI算力需求暴增",
		"台积电先进封装产能供不应求，光模块订单大幅增长",
	)
	// 标题: "AI算力" → 人工智能 (title +2)
	// 正文: "台积电" → 半导体 (content +1)
	// 正文: "先进封装" → 半导体 (content +1, cumulative)
	// 正文: "光模块" → CPO概念 (content +1)
	// 排序: 半导体得分可能更高（3次关键词命中）
	if len(sectors) < 2 {
		t.Errorf("expected at least 2 sectors, got %v", sectors)
	}
}

func TestMatchSectors_ShortInput(t *testing.T) {
	sectors := MatchSectors("", "")
	if len(sectors) != 0 {
		t.Errorf("expected empty for empty input, got %v", sectors)
	}

	sectors = MatchSectors("a", "")
	if len(sectors) != 0 {
		t.Errorf("expected empty for single-char input, got %v", sectors)
	}
}

func TestMatchSectors_CJKKeyword(t *testing.T) {
	sectors := MatchSectors("固态电池技术突破", "全固态电池能量密度提升50%")
	// 固态电池 → 电池
	// 标题 "固态电池" → 电池 +2
	// 正文 "全固态电池" → 电池 +1
	if len(sectors) == 0 || sectors[0] != "电池" {
		t.Errorf("expected [电池] at top, got %v", sectors)
	}
}

func TestMatchSectors_EnglishKeyword(t *testing.T) {
	sectors := MatchSectors("OpenAI发布GPT-5", "")
	if len(sectors) == 0 {
		t.Errorf("expected match for 'GPT', got empty")
	}
	matched := false
	for _, s := range sectors {
		if s == "人工智能" {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("expected '人工智能' from 'GPT' keyword, got %v", sectors)
	}
}

func TestMatchSectors_ContentFallback(t *testing.T) {
	// 正文中匹配到关键词（标题无关）
	sectors := MatchSectors("今日财经要闻", "宁德时代发布麒麟电池新一代产品")
	// "麒麟电池" → 电池
	if len(sectors) == 0 {
		t.Errorf("expected content match, got empty")
	}
	matched := false
	for _, s := range sectors {
		if s == "电池" {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("expected '电池' matched from content, got %v", sectors)
	}
}

func TestMatchSectors_OrderByScore(t *testing.T) {
	// 标题命中权重高于正文命中，标题匹配的板块应该排在前面
	sectors := MatchSectors(
		"芯片制裁升级",
		"碳酸锂价格继续下跌，银行板块表现稳健",
	)
	// "芯片" → 半导体（标题+2，正文0，总分2）
	// "碳酸锂" → 锂矿概念（标题0，正文+1，总分1）
	// "银行" → 银行（标题0，正文+1，总分1）
	// 限制返回 top 2 板块
	if len(sectors) > 2 {
		t.Errorf("expected at most 2 sectors, got %v", sectors)
	}
	if len(sectors) == 0 || sectors[0] != "半导体" {
		t.Errorf("expected semiconductor ranked #1 (title match), got %v", sectors)
	}
}

func TestMatchSectorsToNews(t *testing.T) {
	news := []CLSNews{
		{Title: "半导体大涨", Content: "芯片产业链全面爆发"},
		{Title: "天气新闻", Content: "今天晴转多云"},
		{Title: "", Content: ""},
	}
	MatchSectorsToNews(news)

	if len(news[0].Sectors) == 0 {
		t.Error("expected sectors on news[0]")
	}
	if len(news[1].Sectors) != 0 {
		t.Errorf("expected no sectors on news[1], got %v", news[1].Sectors)
	}
	if len(news[2].Sectors) != 0 {
		t.Errorf("expected no sectors on news[2], got %v", news[2].Sectors)
	}
}

func TestMatchSectors_SkipsIfAlreadyMatched(t *testing.T) {
	news := []CLSNews{
		{Title: "半导体大涨", Content: "", Sectors: []string{"半导体"}},
	}
	MatchSectorsToNews(news)
	if len(news[0].Sectors) != 1 || news[0].Sectors[0] != "半导体" {
		t.Errorf("expected existing sectors preserved, got %v", news[0].Sectors)
	}
}

func TestMatchSectors_MultiKeywordCumulative(t *testing.T) {
	// 多个同板块关键词命中累积得分
	sectors := MatchSectors("", "芯片 + 晶圆 + 光刻机 + 先进封装 全面突破")
	if len(sectors) != 1 || sectors[0] != "半导体" {
		t.Errorf("expected [半导体] from multi keyword hits, got %v", sectors)
	}
}

func TestMatchSectors_NonAShareMilitary_Blocked(t *testing.T) {
	title := "乌克兰称俄军向乌发射8枚高超音速巡航导弹"
	content := "财联社6月2日电，乌克兰空军2日说，1日晚至2日清晨，俄军再次对乌克兰首都基辅市及第聂伯罗彼得罗夫斯克州等多地发动导弹和无人机袭击。俄军共发射73枚导弹，其中包括8枚锆石高超音速巡航导弹。"
	sectors := MatchSectors(title, content)
	if len(sectors) != 0 {
		t.Errorf("expected nil for military news, got %v", sectors)
	}
}

func TestMatchSectors_NonAShareMidEast_Blocked(t *testing.T) {
	title := "以色列国防军空袭加沙地带"
	content := "哈马斯武装向以色列发射火箭弹，双方冲突持续升级"
	sectors := MatchSectors(title, content)
	if len(sectors) != 0 {
		t.Errorf("expected nil for middle-east news, got %v", sectors)
	}
}

func TestMatchSectors_AShareChipsMention_NotBlocked(t *testing.T) {
	title := "美国升级对华芯片出口管制"
	content := "国内半导体行业加快自主可控进程，光刻机国产化取得新突破"
	sectors := MatchSectors(title, content)
	matched := false
	for _, s := range sectors {
		if s == "半导体" {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("expected '半导体' matched for chip news, got %v", sectors)
	}
}

func TestContainsRune(t *testing.T) {
	tests := []struct {
		text string
		kw   string
		want bool
	}{
		{"固态电池技术", "固态电池", true},
		{"固态电池技术", "电池", true},
		{"hello world", "world", true},
		{"hello world", "xyz", false},
		{"GPU算力", "GPU", true},
		{"AI芯片", "NPU", false},
		{"", "AI", false},
		{"abc", "abcd", false},
		{"a", "a", true},
	}
	for _, tc := range tests {
		got := containsRune([]rune(tc.text), tc.kw)
		if got != tc.want {
			t.Errorf("containsRune(%q, %q) = %v, want %v", tc.text, tc.kw, got, tc.want)
		}
	}
}
