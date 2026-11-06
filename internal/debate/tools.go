package debate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/httpclient"
)

type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args map[string]any) (any, error)
}

type ToolRegistry struct {
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{tools: make(map[string]Tool)}
	r.Register(&fetchMetricTool{})
	r.Register(&fetchStatementTool{})
	r.Register(&comparePeersTool{})
	r.Register(&searchNewsTool{})
	r.Register(&riskAlertsTool{})
	return r
}

func (r *ToolRegistry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *ToolRegistry) List() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

func eastmoneyGet(ctx context.Context, apiURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://quote.eastmoney.com/")

	client := httpclient.New(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func normalizeStockCode(code string) (secID string, err error) {
	code = strings.TrimSpace(code)
	code = strings.TrimPrefix(code, "SH")
	code = strings.TrimPrefix(code, "SZ")
	code = strings.TrimPrefix(code, "sh")
	code = strings.TrimPrefix(code, "sz")
	if code == "" {
		return "", fmt.Errorf("股票代码不能为空")
	}
	if strings.HasPrefix(code, "6") {
		return "1." + code, nil
	}
	if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
		return "0." + code, nil
	}
	return "", fmt.Errorf("无法识别股票代码 %s", code)
}

func jsonNumber(s string) float64 {
	var f float64
	_, _ = fmt.Sscanf(s, "%f", &f)
	return f
}

type fetchMetricTool struct{}

func (s *fetchMetricTool) Name() string { return "fetch_metric" }
func (s *fetchMetricTool) Description() string {
	return "拉取实时估值指标(PE-TTM/PB/换手率/涨跌幅/总市值/流通市值,市值单位亿元,带 as_of 快照时间)。" +
		"只覆盖估值:取不到的字段会列在\"未取到\"里(公司亏损或接口无值),不存在就写缺,不要把缺失当 0。"
}

// fetchMetricSupported 是本工具真实能查的字段。args.metric 目前不参与取数,
// 只做白名单校验:模型经常来查"经营现金流""销售收现"这类报表科目,
// 若不拒绝,它会拿到 PE/PB 却以为核实过了——假核实比不核实更危险。
var fetchMetricSupported = map[string]bool{
	"": true, "valuation": true, "price": true, "pe": true, "pe_ttm": true,
	"pb": true, "turnover": true, "mkt_cap": true, "circ_cap": true,
}

func (s *fetchMetricTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	code, _ := args["stock"].(string)
	if code == "" {
		return nil, fmt.Errorf("args.stock 必填")
	}
	metric, _ := args["metric"].(string)
	if !fetchMetricSupported[strings.ToLower(strings.TrimSpace(metric))] {
		return nil, fmt.Errorf("fetch_metric 只支持估值指标(price/pe/pb/turnover/mkt_cap/circ_cap),无法查询 %q；报表科目请用 fetch_statement,查不到就写「缺 XX 科目」", metric)
	}
	secID, err := normalizeStockCode(code)
	if err != nil {
		return nil, err
	}
	apiURL := fmt.Sprintf(
		"https://push2.eastmoney.com/api/qt/stock/get?secid=%s&fields=f43,f57,f58,f60,f116,f117,f162,f167,f168,f170",
		url.QueryEscape(secID),
	)
	body, err := eastmoneyGet(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("未拿到 %s 的指标", code)
	}
	return buildMetricResult(resp.Data, metric), nil
}

// buildMetricResult 把行情接口的裸字段整理成可引用的指标。
// 抽成纯函数是为了能单测"缺失字段不进结果"这条规则——
// 它和现金流那次是同一类错误:0 被模型读成事实。
func buildMetricResult(data map[string]any, metric string) map[string]any {
	getF := func(k string) float64 {
		v, ok := data[k]
		if !ok || v == nil {
			return 0
		}
		switch n := v.(type) {
		case float64:
			return n
		case string:
			return jsonNumber(n)
		}
		return 0
	}
	getS := func(k string) string {
		v, ok := data[k]
		if !ok || v == nil {
			return ""
		}
		s, _ := v.(string)
		return s
	}
	// 字段映射来自 2026-11-06 对 1.600519 的实测,不是猜的:
	//   f43=145300 -> 1453.00 元(分);f60=145200 -> 昨收 1452.00
	//   f116/f117 -> 总市值/流通市值(元);f162=2113 -> PE-TTM 21.13(×100)
	//   f167=670  -> PB 6.70;f168=34 -> 换手率 0.34%;f170=7 -> 涨跌幅 0.07%
	// 旧实现把 f167 当 PE-LYR、f168 当 PB、f191 当总市值,直接把 PB 6.70 说成
	// "PB 34"、把 -1604 当市值,模型据此写的估值论据全是错的。
	fields := []struct {
		label string
		code  string
		scale float64
	}{
		{"price", "f43", 0.01},
		{"prev_close", "f60", 0.01},
		{"pe_ttm", "f162", 0.01},
		{"pb", "f167", 0.01},
		{"turnover_pct", "f168", 0.01},
		{"change_pct", "f170", 0.01},
		{"mkt_cap_亿", "f116", 1e-8},
		{"circ_cap_亿", "f117", 1e-8},
	}

	out := map[string]any{
		"code":  getS("f57"),
		"name":  getS("f58"),
		"as_of": time.Now().Format("2006-01-02 15:04"),
		"口径":    "行情快照,与财报报告期无关;PE-TTM 为最近四个季度滚动,负值表示亏损;换手率/涨跌幅单位为%;市值为亿元",
	}
	if m := strings.ToLower(strings.TrimSpace(metric)); m != "" {
		out["requested_metric"] = m
	}

	// 0 在行情接口里既可能是停牌/亏损,也可能是字段缺失,
	// 一律不进结果——"没有"和"是 0"必须分开,否则模型会把缺失读成便宜。
	missing := make([]string, 0, len(fields))
	for _, f := range fields {
		v := getF(f.code) * f.scale
		if v == 0 {
			missing = append(missing, f.label)
			continue
		}
		out[f.label] = round2(v)
	}
	if len(missing) > 0 {
		out["未取到"] = missing
	}
	return out
}

type comparePeersTool struct{}

func (s *comparePeersTool) Name() string { return "compare_peers" }
func (s *comparePeersTool) Description() string {
	return "拉取行业头部 PE/PB 排序,用于行业对比"
}
func (s *comparePeersTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	sector, _ := args["sector"].(string)
	if sector == "" {
		return nil, fmt.Errorf("args.sector 必填")
	}
	apiURL := "https://push2.eastmoney.com/api/qt/clist/get?pn=1&pz=10&po=1&np=1&fltt=2&invt=2&fid=f3&fs=m:90+t:2&fields=f12,f14,f2,f3,f20,f162,f167,f168"
	if bk, ok := args["bk_code"].(string); ok && bk != "" {
		apiURL = fmt.Sprintf(
			"https://push2.eastmoney.com/api/qt/clist/get?pn=1&pz=10&po=1&np=1&fltt=2&invt=2&fid=f3&fs=b:%s&fields=f12,f14,f2,f3,f20,f162,f167,f168",
			url.QueryEscape(bk),
		)
	}
	body, err := eastmoneyGet(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Diff []map[string]any `json:"diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	getF := func(d map[string]any, k string) float64 {
		v, ok := d[k]
		if !ok || v == nil {
			return 0
		}
		if f, ok := v.(float64); ok {
			return f
		}
		return 0
	}
	getS := func(d map[string]any, k string) string {
		v, ok := d[k]
		if !ok || v == nil {
			return ""
		}
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}
	type peer struct {
		Code  string  `json:"code"`
		Name  string  `json:"name"`
		Price float64 `json:"price"`
		Pct   float64 `json:"pct"`
		PE    float64 `json:"pe_ttm"`
		PB    float64 `json:"pb"`
		MCap  float64 `json:"mcap"`
	}
	out := make([]peer, 0, len(resp.Data.Diff))
	for _, d := range resp.Data.Diff {
		out = append(out, peer{
			Code:  getS(d, "f12"),
			Name:  getS(d, "f14"),
			Price: getF(d, "f2") / 100,
			Pct:   getF(d, "f3") / 100,
			PE:    getF(d, "f162"),
			PB:    getF(d, "f168"),
			MCap:  getF(d, "f20"),
		})
	}
	return map[string]any{
		"sector": sector,
		"peers":  out,
		"count":  len(out),
	}, nil
}

type searchNewsTool struct{}

func (s *searchNewsTool) Name() string        { return "search_news" }
func (s *searchNewsTool) Description() string { return "搜索近期公告/新闻/监管问询" }
func (s *searchNewsTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	code, _ := args["stock"].(string)
	if code == "" {
		return nil, fmt.Errorf("args.stock 必填")
	}
	apiURL := fmt.Sprintf(
		"https://np-anotice-stock.eastmoney.com/api/security/ann?cb=&sr=-1&page_size=10&page_index=1&ann_type=A&client_source=web&stock_list=%s&f_node=0&s_node=0",
		url.QueryEscape(code),
	)
	body, err := eastmoneyGet(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			List []struct {
				Title   string `json:"title"`
				Date    string `json:"notice_date"`
				Code    string `json:"stock_code"`
				Columns string `json:"columns_name"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	type newsItem struct {
		Title   string `json:"title"`
		Date    string `json:"date"`
		Code    string `json:"code"`
		Columns string `json:"columns"`
	}
	out := make([]newsItem, 0, len(resp.Data.List))
	for _, n := range resp.Data.List {
		out = append(out, newsItem{Title: n.Title, Date: n.Date, Code: n.Code, Columns: n.Columns})
	}
	return map[string]any{
		"stock": code,
		"news":  out,
		"count": len(out),
	}, nil
}

type riskAlertsTool struct{}

func (s *riskAlertsTool) Name() string { return "risk_alerts" }
func (s *riskAlertsTool) Description() string {
	return "拉取财报风险指标 (应收/商誉/担保/关联交易占比)"
}
func (s *riskAlertsTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	code, _ := args["stock"].(string)
	if code == "" {
		return nil, fmt.Errorf("args.stock 必填")
	}
	filter := fmt.Sprintf("(SECURITY_CODE=\"%s\")", code)
	apiURL := fmt.Sprintf(
		"https://datacenter-web.eastmoney.com/api/data/v1/get?sortColumns=REPORT_DATE&sortTypes=-1&pageSize=1&pageNumber=1&reportName=RPT_DMSK_FN_BALANCE&columns=ALL&filter=%s",
		url.QueryEscape(filter),
	)
	body, err := eastmoneyGet(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Success bool `json:"success"`
		Result  struct {
			Data []map[string]any `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if !resp.Success || len(resp.Result.Data) == 0 {
		return nil, fmt.Errorf("未找到 %s 的资产负债表", code)
	}
	row := resp.Result.Data[0]
	getF := func(k string) float64 {
		v, ok := row[k]
		if !ok || v == nil {
			return 0
		}
		if f, ok := v.(float64); ok {
			return f
		}
		return 0
	}
	receivable := getF("ACCOUNT_RECEIVABLE")
	goodwill := getF("GOODWILL")
	totalAssets := getF("TOTAL_ASSETS")
	shortBorrow := getF("SHORT_BORROW")
	longBorrow := getF("LONG_BORROW")
	contractLiab := getF("CONTRACT_LIABILITY")
	guarantee := getF("GUARANTEE_AMT")
	relatedSales := getF("RELATED_PARTY_TRANS_SAL")
	relatedPurch := getF("RELATED_PARTY_TRANS_PUR")
	revenue := getF("TOTAL_OPERATE_INCOME")

	alerts := []string{}
	if totalAssets > 0 {
		ratio := goodwill / totalAssets
		if ratio > 0.10 {
			alerts = append(alerts, fmt.Sprintf("商誉占比 %.1f%% > 10%%", ratio*100))
		}
	}
	if totalAssets > 0 {
		ratio := receivable / totalAssets
		if ratio > 0.20 {
			alerts = append(alerts, fmt.Sprintf("应收占比 %.1f%% > 20%%", ratio*100))
		}
	}
	if revenue > 0 {
		ratio := (relatedSales + relatedPurch) / revenue
		if ratio > 0.05 {
			alerts = append(alerts, fmt.Sprintf("关联购销占比 %.1f%% > 5%%", ratio*100))
		}
	}
	if totalAssets > 0 {
		ratio := guarantee / totalAssets
		if ratio > 0.10 {
			alerts = append(alerts, fmt.Sprintf("对外担保占比 %.1f%% > 10%%", ratio*100))
		}
	}

	return map[string]any{
		"code":             code,
		"receivable":       receivable,
		"goodwill":         goodwill,
		"goodwill_ratio":   ifZero(totalAssets, goodwill/totalAssets*100),
		"short_borrow":     shortBorrow,
		"long_borrow":      longBorrow,
		"contract_liab":    contractLiab,
		"guarantee":        guarantee,
		"related_sales":    relatedSales,
		"related_purchase": relatedPurch,
		"total_assets":     totalAssets,
		"alerts":           alerts,
	}, nil
}

func ifZero(denom, val float64) float64 {
	if denom == 0 {
		return 0
	}
	return val
}
