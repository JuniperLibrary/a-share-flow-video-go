// Package oral 汇总「书面语转口播语」的替换规则。
//
// 文案生成（internal/copy）与语音合成（internal/tts）都要把书面表达改成
// 适合口播的说法。两处原本各存一份表，并且已经分叉：tts 侧多了全角分号转
// 逗号，copy 侧把「整体来看」重复注册了两次。规则收敛到这里单点维护，
// 避免以后再出现改一处漏一处。
package oral

import "strings"

// writtenToOralPairs 是「书面语 → 口播语」的键值对，奇数位是原词，偶数位是替换词。
//
// 全角分号转逗号也在这里：口播稿最终是要念出来的，分号属于书面标点。
var writtenToOralPairs = []string{
	"综上所述", "说白了",
	"不难看出", "你再往深看",
	"显而易见", "关键",
	"值得注意的是", "更关键的来了",
	"整体而言", "盘面看",
	"整体来看", "盘面看",
	"综合来看", "盘面看",
	"客观来说", "说句实在话",
	"坦率地讲", "说句实在话",
	"达到了", "到了",
	"取得了", "干到了",
	"占比达到了", "占到了",
	"的一个", "的",
	"非常明显的", "明显的",
	"进行了", "做了",
	"实现了", "做到了",
	"；", "，",
}

// buzzwordPairs 是「商业黑话 → 日常表达」的键值对，排列方式同 writtenToOralPairs。
var buzzwordPairs = []string{
	"闭环", "完整流程",
	"抓手", "切入点",
	"颗粒度", "细致程度",
	"对齐", "同步",
	"拉齐", "同步",
	"赋能", "帮助",
	"赛道", "领域",
	"弯道超车", "后发追上",
	"占领心智", "形成印象",
	"心智", "印象",
}

// WrittenToOral 把书面化表达换成口播更自然的说法。
var WrittenToOral = strings.NewReplacer(writtenToOralPairs...)

// Buzzwords 把商业黑话换成日常表达。
var Buzzwords = strings.NewReplacer(buzzwordPairs...)
