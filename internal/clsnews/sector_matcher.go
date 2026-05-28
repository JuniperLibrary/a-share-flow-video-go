package clsnews

import (
	"sort"
	"unicode/utf8"
)

type sectorDef struct {
	Name     string
	Keywords []string
}

// sectorDB 板块关键词库（名称 + 同义词/术语），标题命中权重 > 正文命中。
var sectorDB = []sectorDef{
	{
		Name: "半导体",
		Keywords: []string{
			"半导体", "芯片", "晶圆", "光刻", "光刻机",
			"集成电路", "IC设计", "封测", "先进封装",
			"半导体设备", "半导体材料", "第三代半导体",
			"碳化硅", "氮化镓", "SiC", "GaN",
			"台积电", "中芯国际", "华虹",
			"EDA", "IP授权",
		},
	},
	{
		Name: "AI应用",
		Keywords: []string{
			"AI应用", "AI+", "AI赋能",
			"生成式AI", "AIGC", "AIGC应用",
			"AI+医疗", "AI+教育", "AI+金融", "AI+制造",
			"多模态", "AI终端", "AI落地",
			"AI智能体", "AI Agent",
			"应用落地", "AI商业化",
		},
	},
	{
		Name: "CPO概念",
		Keywords: []string{
			"CPO", "CPO概念",
			"共封装光学", "硅光", "硅光子",
			"相干光学", "光互联",
			"光模块", "800G光模块", "1.6T光模块",
			"LPO", "线性驱动",
		},
	},
	{
		Name: "有色金属",
		Keywords: []string{
			"有色金属", "工业金属",
			"铜", "铝", "锌", "镍", "锡", "铅",
			"稀土", "永磁",
			"钨", "钼", "钛", "锑",
			"黄金", "贵金属",
			"氧化铝", "电解铝", "电解铜",
			"小金属", "战略金属",
		},
	},
	{
		Name: "锂矿概念",
		Keywords: []string{
			"锂矿", "锂资源",
			"碳酸锂", "氢氧化锂",
			"盐湖提锂", "锂辉石", "锂云母",
			"锂", "锂电上游",
			"锂盐", "电池级锂",
			"西藏矿业", "赣锋锂业", "天齐锂业",
		},
	},
	{
		Name: "商业航天",
		Keywords: []string{
			"商业航天", "商业火箭", "商业卫星",
			"卫星", "卫星互联网",
			"火箭", "可回收火箭", "运载火箭",
			"星链", "低轨卫星", "LEO",
			"航天", "太空", "卫星通信",
			"卫星制造", "卫星运营",
		},
	},
	{
		Name: "电池",
		Keywords: []string{
			"电池", "锂电池",
			"固态电池", "全固态电池", "半固态电池",
			"磷酸铁锂", "LFP", "三元锂",
			"钠离子电池", "钠电池",
			"动力电池", "储能电池",
			"电池片", "电池 pack",
			"正极", "负极", "电解液", "隔膜",
			"电池回收", "电池材料",
			"宁德时代", "比亚迪电池",
			"刀片电池", "麒麟电池",
		},
	},
	{
		Name: "机器人",
		Keywords: []string{
			"机器人", "机器人概念",
			"人形机器人", "仿生机器人",
			"机器视觉", "具身智能",
			"减速器", "RV减速器", "谐波减速器",
			"伺服电机", "伺服系统",
			"关节模组", "灵巧手",
			"执行器", "力矩传感器",
			"工业机器人", "协作机器人",
			"优必选", "特斯拉机器人", "擎天柱",
			"滚柱丝杠", "空心杯电机", "无框电机",
		},
	},
	{
		Name: "创新药",
		Keywords: []string{
			"创新药", "原研药",
			"生物药", "生物类似药",
			"靶向药", "靶向治疗",
			"抗体", "单抗", "双抗", "ADC",
			"CAR-T", "细胞治疗", "基因治疗",
			"临床试验", "临床三期",
			"FDA批准", "FDA", "NMPA",
			"PD-1", "GLP-1", "CXO",
			"新药上市", "药物研发",
		},
	},
	{
		Name: "白酒",
		Keywords: []string{
			"白酒", "酱酒", "浓香",
			"茅台", "五粮液", "泸州老窖",
			"山西汾酒", "洋河", "古井贡酒",
			"酿酒", "高端白酒",
			"白酒消费", "白酒库存",
			"宴席", "白酒动销",
		},
	},
	{
		Name: "消费电子",
		Keywords: []string{
			"消费电子",
			"手机", "智能手机", "折叠屏",
			"AR", "VR", "MR", "XR",
			"可穿戴", "智能穿戴", "智能手表", "耳机", "TWS",
			"笔电", "笔记本电脑", "平板",
			"面板", "OLED", "MiniLED", "MicroLED", "显示面板",
			"果链", "苹果产业链",
			"华为产业链", "华为手机",
			"AI PC", "AI眼镜",
			"消费电子", "3C",
		},
	},
	{
		Name: "银行",
		Keywords: []string{
			"银行", "商业银行",
			"国有大行", "股份行", "城商行", "农商行",
			"净息差", "息差", "息差收窄",
			"存款", "贷款", "信贷",
			"不良率", "不良贷款",
			"拨备", "资本充足率",
			"工商银行", "建设银行", "招商银行",
			"银行股", "银行板块",
		},
	},
	{
		Name: "人工智能",
		Keywords: []string{
			"人工智能", "AI",
			"大模型", "语言模型", "LLM",
			"机器学习", "深度学习",
			"GPT", "ChatGPT", "OpenAI",
			"自然语言处理", "NLP",
			"计算机视觉", "CV",
			"算力", "智算", "智能计算",
			"AI算力", "AI芯片",
			"训练", "推理", "模型训练",
			"深度学习框架",
			"国产大模型", "文心一言", "通义千问",
		},
	},
	{
		Name: "云计算",
		Keywords: []string{
			"云计算", "云服务",
			"IaaS", "PaaS", "SaaS",
			"公有云", "私有云", "混合云",
			"云原生", "容器", "K8s",
			"数据中心", "IDC",
			"算力", "算力租赁",
			"边缘计算",
			"阿里云", "腾讯云", "华为云",
			"云基础设施",
		},
	},
	{
		Name: "低空经济",
		Keywords: []string{
			"低空经济", "低空",
			"eVTOL", "飞行汽车",
			"无人机", "工业无人机", "eVTOL整机",
			"空管", "低空空管",
			"低空管控", "低空基础设施",
			"通航", "通用航空",
			"低空飞行", "低空通信",
		},
	},
	{
		Name: "电网设备",
		Keywords: []string{
			"电网设备", "电力设备",
			"变压器", "开关设备", "断路器",
			"特高压", "高压输电",
			"智能电网", "配电网", "配网",
			"电力", "电气设备",
			"充电桩", "充电基础设施",
			"输变电", "变电站",
			"电力物联网", "能源互联网",
			"国网", "南网", "电网投资",
		},
	},
	{
		Name: "通信设备",
		Keywords: []string{
			"通信设备",
			"5G", "5G-A", "5.5G", "6G",
			"基站", "小基站",
			"光通信", "光纤", "光缆",
			"交换机", "路由器", "网关",
			"通信设备", "网络设备",
			"光纤光缆", "数据中心网络",
			"华为通信", "中兴通讯",
			"卫星通信", "卫星导航",
		},
	},
	{
		Name: "传媒",
		Keywords: []string{
			"传媒", "媒体",
			"影视", "电影", "电视剧", "院线",
			"游戏", "网游", "手游", "游戏版号",
			"出版", "图书",
			"广告", "营销", "数字营销",
			"新媒体", "短视频", "直播",
			"短剧", "微短剧",
			"IP", "文化", "内容",
			"流媒体", "长视频",
			"文娱", "娱乐", "互动娱乐",
			"体育", "体育赛事",
		},
	},
	{
		Name: "国产芯片",
		Keywords: []string{
			"国产芯片", "国产替代",
			"自主可控", "信创", "国产化",
			"CPU", "GPU", "NPU", "AI芯片", "AI推理芯片",
			"龙芯", "飞腾", "鲲鹏", "申威",
			"寒武纪", "海光信息",
			"处理器", "算力芯片",
			"操作系统", "国产软件",
			"国产EDA", "半导体国产",
			"华为芯片", "麒麟芯片",
			"存算一体", "类脑芯片",
		},
	},
	{
		Name: "元件",
		Keywords: []string{
			"元件", "电子元件",
			"MLCC", "电容", "电阻", "电感",
			"连接器", "接插件",
			"功率器件", "IGBT", "MOSFET", "SiC器件",
			"传感器", "MEMS",
			"晶振", "滤波器",
			"PCB", "电路板",
			"被动元件", "分立器件",
			"汽车电子元件",
		},
	},
	{
		Name: "通信服务",
		Keywords: []string{
			"通信服务",
			"运营商", "电信运营商",
			"移动", "中国移动",
			"电信", "中国电信",
			"联通", "中国联通",
			"5G套餐", "宽带", "千兆宽带",
			"通信服务", "通信运营",
			"云通信", "增值电信",
		},
	},
}

// MatchSectors matches sectors to news by keyword scoring:
// title hit = +2, content hit = +1; returns sectors sorted by score descending, min score 1.
func MatchSectors(title, content string) []string {
	type match struct {
		name  string
		score int
	}

	titleRunes := []rune(title)
	contentRunes := []rune(content)

	shortTitle := utf8.RuneCountInString(title) < 2
	shortContent := utf8.RuneCountInString(content) < 2

	var results []match

	for _, sector := range sectorDB {
		score := 0
		for _, kw := range sector.Keywords {
			if !shortTitle && containsRune(titleRunes, kw) {
				score += 5
			}
			if !shortContent && containsRune(contentRunes, kw) {
				score += 1
			}
		}
		if score > 0 {
			results = append(results, match{name: sector.Name, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	maxResults := 2
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.name
	}
	return out
}

// containsRune checks if kw exists within textRunes (rune-based for CJK support).
func containsRune(textRunes []rune, kw string) bool {
	if len(kw) == 0 {
		return false
	}
	kwRunes := []rune(kw)
	if len(kwRunes) > len(textRunes) {
		return false
	}
	if len(kwRunes) == 1 {
		for _, r := range textRunes {
			if r == kwRunes[0] {
				return true
			}
		}
		return false
	}
	maxStart := len(textRunes) - len(kwRunes)
outer:
	for i := 0; i <= maxStart; i++ {
		for j := 0; j < len(kwRunes); j++ {
			if textRunes[i+j] != kwRunes[j] {
				continue outer
			}
		}
		return true
	}
	return false
}

// MatchSectorsToNews 批量给新闻列表匹配板块标签。
func MatchSectorsToNews(news []CLSNews) {
	for i := range news {
		if len(news[i].Sectors) == 0 {
			news[i].Sectors = MatchSectors(news[i].Title, news[i].Content)
		}
	}
}
