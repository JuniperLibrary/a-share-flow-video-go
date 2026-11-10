package debate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/httpclient"
)

// fetchStatementTool 拉取三大报表的原始科目。
//
// 为什么必须有这个工具:fetch_metric 只有估值字段,核实"经营现金流/销售收现/
// 合同负债"这类报表科目时它给不出任何东西,辩论就变成"凭结构化摘要猜"。
// 实跑里 7 次 FactCheck 全部落空,正是因为缺这个能力。
type fetchStatementTool struct{}

func (s *fetchStatementTool) Name() string { return "fetch_statement" }

func (s *fetchStatementTool) Description() string {
	return "拉取三大报表原始科目(最近 1-4 期,金额单位亿元):statement=cashflow|balance|income。" +
		"periods 默认 2 期(本期+上期),要核对上一年年报口径必须显式传 4——" +
		"否则拿不到年报数,只能凭记忆补数,而凭记忆补的数不该进结论。" +
		"口径提示:cashflow 的\"经营活动现金流入合计\"含财务公司等金融业务科目" +
		"(客户存款和同业存放款项净增加额、向央行借款净增加额等,可为负数)," +
		"因此它可以小于\"销售商品提供劳务收到的现金\"。两者之差不是数据矛盾," +
		"也不能用来否定销售收现。"
}

type statementField struct {
	Key    string
	Label  string
	IsRate bool // 比率字段不做亿元换算
}

type statementSpec struct {
	ReportName string
	Fields     []statementField
}

// 字段顺序即输出顺序,方便模型按固定顺序引用。
var statementSpecs = map[string]statementSpec{
	"cashflow": {
		ReportName: "RPT_F10_FINANCE_GCASHFLOW",
		Fields: []statementField{
			{"NETCASH_OPERATE", "经营活动现金流净额", false},
			{"SALES_SERVICES", "销售商品提供劳务收到的现金", false},
			{"TOTAL_OPERATE_INFLOW", "经营活动现金流入合计", false},
			{"TOTAL_OPERATE_OUTFLOW", "经营活动现金流出合计", false},
			{"PAY_ALL_TAX", "支付的各项税费", false},
			{"CONSTRUCT_LONG_ASSET", "购建长期资产支付的现金", false},
			{"NETCASH_INVEST", "投资活动现金流净额", false},
			{"NETCASH_FINANCE", "筹资活动现金流净额", false},
			{"ASSIGN_DIVIDEND_PORFIT", "分配股利利润或偿付利息支付的现金", false},
			{"CCE_ADD", "现金及现金等价物净增加额", false},
			{"BEGIN_CCE", "期初现金及现金等价物", false},
			{"END_CCE", "期末现金及现金等价物", false},
		},
	},
	"balance": {
		ReportName: "RPT_F10_FINANCE_GBALANCE",
		Fields: []statementField{
			{"TOTAL_ASSETS", "总资产", false},
			{"TOTAL_LIABILITIES", "总负债", false},
			{"TOTAL_EQUITY", "归母净资产", false},
			{"MONETARYFUNDS", "货币资金", false},
			{"ACCOUNTS_RECE", "应收账款", false},
			{"NOTE_ACCOUNTS_RECE", "应收票据及应收账款", false},
			{"INVENTORY", "存货", false},
			{"CONTRACT_LIAB", "合同负债", false},
			{"CONTRACT_ASSET", "合同资产", false},
			{"ACCOUNTS_PAYABLE", "应付账款", false},
			// 核实阶段最常被问到的三个科目:现金缺口的常见解释项。
			// 2026-11-06 的实跑里 Bear 明确要求「应交税费」,
			// 但工具当时拿不出来,只能写"无科目对应",整条反证落空。
			{"TAX_PAYABLE", "应交税费", false},
			{"STAFF_SALARY_PAYABLE", "应付职工薪酬", false},
			{"TOTAL_OTHER_PAYABLE", "其他应付款", false},
			{"PREPAYMENT", "预付款项", false},
			{"CIP", "在建工程", false},
			{"FIXED_ASSET", "固定资产", false},
			{"INTANGIBLE_ASSET", "无形资产", false},
			{"DEVELOP_EXPENSE", "开发支出", false},
			{"GOODWILL", "商誉", false},
			{"LEASE_LIAB", "租赁负债", false},
			{"MINORITY_EQUITY", "少数股东权益", false},
		},
	},
	"income": {
		ReportName: "RPT_F10_FINANCE_GINCOME",
		Fields: []statementField{
			{"TOTAL_OPERATE_INCOME", "营业总收入", false},
			{"OPERATE_COST", "营业成本", false},
			{"TOTAL_OPERATE_COST", "营业总成本", false},
			{"OPERATE_TAX_ADD", "税金及附加", false},
			{"SALE_EXPENSE", "销售费用", false},
			{"MANAGE_EXPENSE", "管理费用", false},
			{"FINANCE_EXPENSE", "财务费用", false},
			{"OPERATE_PROFIT", "营业利润", false},
			{"TOTAL_PROFIT", "利润总额", false},
			{"INCOME_TAX", "所得税费用", false},
			{"PARENT_NETPROFIT", "归母净利润", false},
			{"DEDUCT_PARENT_NETPROFIT", "扣非归母净利润", false},
			{"INTEREST_NI", "利息净收入", false},
		},
	},
}

func (s *fetchStatementTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	code, _ := args["stock"].(string)
	if code == "" {
		return nil, fmt.Errorf("args.stock 必填")
	}
	statement := strings.ToLower(strings.TrimSpace(fmt.Sprint(args["statement"])))
	if statement == "" {
		statement = "cashflow"
	}
	spec, ok := statementSpecs[statement]
	if !ok {
		return nil, fmt.Errorf("args.statement 只支持 cashflow/balance/income,收到 %q", statement)
	}

	periods := argInt(args, "periods", 2)
	if periods > 4 {
		periods = 4
	}

	rows, err := fetchStatementRows(ctx, spec.ReportName, code, periods)
	if err != nil {
		return nil, err
	}
	result := buildStatementResult(spec, rows)
	if statement == "cashflow" {
		annotateCashflowScope(result)
	}
	return map[string]any{
		"statement": statement,
		"unit":      "亿元",
		"periods":   result,
	}, nil
}

// annotateCashflowScope 在"销售收现 > 经营流入合计"的那一期补一条口径说明。
//
// 合并报表里带财务公司业务的(如茅台),经营流入合计包含"客户存款和同业存放
// 款项净增加额""向中央银行借款净增加额"等可为负的金融科目,合计小于销售收现
// 是正常现象。不加说明,模型会把它读成"数据自相矛盾"——第四场实跑里 Bear
// 就是拿这个伪矛盾当最后一条反证,直接污染了 verdict。
func annotateCashflowScope(periods []map[string]any) {
	const (
		salesKey  = "销售商品提供劳务收到的现金"
		inflowKey = "经营活动现金流入合计"
	)
	for _, p := range periods {
		sales, okSales := p[salesKey].(float64)
		inflow, okInflow := p[inflowKey].(float64)
		if !okSales || !okInflow || sales <= inflow {
			continue
		}
		p["口径说明"] = "该期销售收现高于经营流入合计:流入合计含财务公司等金融业务科目(净额可为负),属正常口径,不是数据矛盾,不可据此否定销售收现"
	}
}

// buildStatementResult 是纯函数,便于单测:输入原始行,输出按科目标签组织的亿元数。
func buildStatementResult(spec statementSpec, rows []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{}
		if v, ok := row["REPORT_DATE"].(string); ok {
			item["报告期"] = strings.ReplaceAll(v, " 00:00:00", "")
		}
		if v, ok := row["REPORT_TYPE"].(string); ok && v != "" {
			item["报告类型"] = v
		}
		for _, f := range spec.Fields {
			v, ok := row[f.Key]
			if !ok || v == nil {
				continue
			}
			num, ok := toFloat(v)
			if !ok {
				continue
			}
			if f.IsRate {
				item[f.Label] = num
			} else {
				// 亿元,保留两位;0 也保留——"科目为 0"和"科目缺失"是两件事。
				item[f.Label] = round2(num / 1e8)
			}
		}
		out = append(out, item)
	}
	return out
}

// argInt 宽容解析工具参数里的整数。
//
// 模型的 tool call 参数经 JSON 反序列化后是 float64,但不同协议/客户端
// 会给出 json.Number 甚至字符串 "4"。以前只认 float64,传 "4" 时静默
// 回落到默认 2 期,于是"核对上一年年报"永远拿不到年报数——
// 实测 2026-11-06 的一场辩论就是这样逼着模型凭记忆补出 80.07 亿。
func argInt(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int(n)
		}
	case int:
		if n > 0 {
			return n
		}
	case json.Number:
		if i, err := n.Int64(); err == nil && i > 0 {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil && i > 0 {
			return i
		}
	}
	return def
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(n, "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func round2(f float64) float64 {
	if f >= 0 {
		return float64(int64(f*100+0.5)) / 100
	}
	return float64(int64(f*100-0.5)) / 100
}

func fetchStatementRows(ctx context.Context, reportName, code string, periods int) ([]map[string]any, error) {
	filter := fmt.Sprintf(`(SECUCODE="%s")`, statementSecucode(code))
	apiURL := fmt.Sprintf(
		"https://datacenter.eastmoney.com/securities/api/data/v1/get?sortColumns=REPORT_DATE&sortTypes=-1&pageSize=%d&pageNumber=1&reportName=%s&columns=ALL&filter=%s",
		periods, reportName, url.QueryEscape(filter),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://emweb.securities.eastmoney.com/")

	client := httpclient.New(20 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result struct {
			Data []map[string]any `json:"data"`
		} `json:"result"`
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析报表响应失败: %w", err)
	}
	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("未取到 %s 的 %s", code, reportName)
	}
	return result.Result.Data, nil
}

// statementSecucode 给纯数字代码补交易所后缀(F10 接口按 SECUCODE 过滤)。
func statementSecucode(code string) string {
	code = strings.TrimSpace(strings.ToUpper(code))
	if strings.Contains(code, ".") {
		return code
	}
	switch {
	case strings.HasPrefix(code, "6"):
		return code + ".SH"
	case strings.HasPrefix(code, "0"), strings.HasPrefix(code, "3"):
		return code + ".SZ"
	case strings.HasPrefix(code, "4"), strings.HasPrefix(code, "8"):
		return code + ".BJ"
	default:
		return code
	}
}
