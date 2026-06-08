package hotnews

import "testing"

func TestIsNoise(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"财联社6月3日电,某某板块大涨", true},
		{"财联社", true},
		{"投资日历:周三资本市场大事提醒", true},
		{"三大指数开盘涨跌不一", true},
		{"竞价看龙头:早盘风向", true},
		{"今日申购指南及新股定位分析", true},
		{"南向资金持续净流入", true},
		{"CPO概念大幅高开 天孚通信涨近10%创历史新高", false},
		{"人形机器人概念表现活跃 浙江荣泰逼近涨停", false},
		{"", false},
	}
	for _, c := range cases {
		got := isNoise(c.title)
		if got != c.want {
			t.Errorf("isNoise(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}
