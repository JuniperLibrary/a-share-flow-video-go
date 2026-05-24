package clsnews

import "strings"

// hotSectors 热门板块关键词列表，与 fetcher.Top21HotSectors 保持一致。
var hotSectors = []string{
	"半导体", "AI应用", "CPO概念", "有色金属", "锂矿概念",
	"商业航天", "电池", "机器人", "创新药", "白酒",
	"消费电子", "银行", "人工智能", "云计算", "低空经济",
	"电网设备", "通信设备", "传媒", "国产芯片", "元件", "通信服务",
}

// MatchSectors 从新闻的标题+正文中匹配关联的板块名称。
// 返回匹配到的板块列表（去重，按出现顺序）。
func MatchSectors(title, content string) []string {
	text := title + " " + content
	seen := make(map[string]bool, len(hotSectors))
	var matched []string

	for _, sector := range hotSectors {
		if seen[sector] {
			continue
		}
		if strings.Contains(text, sector) {
			seen[sector] = true
			matched = append(matched, sector)
		}
	}
	return matched
}

// MatchSectorsToNews 批量给新闻列表匹配板块标签。
func MatchSectorsToNews(news []CLSNews) {
	for i := range news {
		if len(news[i].Sectors) == 0 {
			news[i].Sectors = MatchSectors(news[i].Title, news[i].Content)
		}
	}
}
