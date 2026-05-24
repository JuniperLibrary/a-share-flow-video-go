package clsnews

import "time"

// CLSNews 财联社新闻记录。
type CLSNews struct {
	ID         int64     `json:"id"`          // 新闻唯一 ID（财联社）
	Title      string    `json:"title"`       // 标题（C 级快讯可能为空，用 content 前 80 字补）
	Content    string    `json:"content"`     // 正文（快讯即完整内容）
	Brief      string    `json:"brief"`       // 摘要 / 导语
	Level      string    `json:"level"`       // 重要等级 A=重点红字 B=红字 C=普通
	ReadingNum int64     `json:"reading_num"` // 阅读数
	CTime      time.Time `json:"ctime"`       // 发布时间（财联社服务端时间）
	ShareURL   string    `json:"shareurl"`    // 原文链接
	Sectors    []string  `json:"sectors"`     // 匹配到的关联板块名称列表
	CreatedAt  time.Time `json:"created_at"`  // 入库时间（本地时间）
}
