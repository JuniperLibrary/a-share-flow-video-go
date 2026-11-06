package fetcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/httpclient"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

type FinanceReport struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	ReportDate string `json:"reportDate"`
	ReportType string `json:"reportType"`

	Revenue         float64 `json:"revenue"`
	RevenuePrev     float64 `json:"revenuePrev"`
	RevenueYoY      float64 `json:"revenueYoY"`
	NetProfit       float64 `json:"netProfit"`
	NetProfitPrev   float64 `json:"netProfitPrev"`
	NetProfitYoY    float64 `json:"netProfitYoY"`
	DeductedProfit  float64 `json:"deductedProfit"`
	OperatingProfit float64 `json:"operatingProfit"`
	TotalProfit     float64 `json:"totalProfit"`

	GrossMargin float64 `json:"grossMargin"`
	NetMargin   float64 `json:"netMargin"`
	ROE         float64 `json:"roe"`
	EPS         float64 `json:"eps"`

	SaleExpense    float64 `json:"saleExpense"`
	ManageExpense  float64 `json:"manageExpense"`
	FinanceExpense float64 `json:"financeExpense"`
	TotalCost      float64 `json:"totalCost"`

	TotalAssets        float64 `json:"totalAssets"`
	TotalLiabilities   float64 `json:"totalLiabilities"`
	TotalEquity        float64 `json:"totalEquity"`
	ContractLiability  float64 `json:"contractLiability"`
	MonetaryFunds      float64 `json:"monetaryFunds"`
	Inventory          float64 `json:"inventory"`
	AccountsReceivable float64 `json:"accountsReceivable"`
	FixedAsset         float64 `json:"fixedAsset"`

	DebtAssetRatio float64 `json:"debtAssetRatio"`
	CurrentRatio   float64 `json:"currentRatio"`

	OperatingCashFlow  float64 `json:"operatingCashFlow"`
	SalesCashReceived  float64 `json:"salesCashReceived"`
	PurchaseFixedAsset float64 `json:"purchaseFixedAsset"`
	OperatingCost      float64 `json:"operatingCost"`
}

type datacenterResponse struct {
	Result struct {
		Data []map[string]any `json:"data"`
	} `json:"result"`
	Success bool `json:"success"`
}

func FetchFinanceReport(code string) (*FinanceReport, error) {
	if code == "" {
		return nil, fmt.Errorf("股票代码不能为空")
	}

	code = strings.TrimSpace(code)
	code = strings.TrimPrefix(code, "SH")
	code = strings.TrimPrefix(code, "SZ")
	code = strings.TrimPrefix(code, "sh")
	code = strings.TrimPrefix(code, "sz")

	incomeData, err := fetchIncomeStatement(code)
	if err != nil {
		return nil, fmt.Errorf("获取利润表失败: %w", err)
	}

	balanceData, err := fetchBalanceSheet(code)
	if err != nil {
		return nil, fmt.Errorf("获取资产负债表失败: %w", err)
	}

	// 现金流量表与完整资产负债表用于补上摘要接口缺失的科目(经营现金流、销售收现、
	// 合同负债、购建长期资产)。失败不阻断:只是这些科目留空,辩论会标注"缺科目"。
	cashData, err := fetchCashFlowStatement(code)
	if err != nil {
		logger.Warn("获取现金流量表失败,相关科目将缺失", zap.String("code", code), zap.Error(err))
	}
	fullBalance, err := fetchFullBalanceSheet(code)
	if err != nil {
		logger.Warn("获取完整资产负债表失败,合同负债等科目将缺失", zap.String("code", code), zap.Error(err))
	}

	report := buildReport(code, incomeData, balanceData, cashData, fullBalance)
	return report, nil
}

func fetchIncomeStatement(code string) ([]map[string]any, error) {
	filter := fmt.Sprintf("(SECURITY_CODE=\"%s\")", code)
	// 获取最多 8 期数据，用于匹配同类型报告期做同比
	apiURL := fmt.Sprintf(
		"https://datacenter-web.eastmoney.com/api/data/v1/get?sortColumns=REPORT_DATE&sortTypes=-1&pageSize=8&pageNumber=1&reportName=RPT_DMSK_FN_INCOME&columns=ALL&filter=%s",
		url.QueryEscape(filter),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://data.eastmoney.com/")

	client := httpclient.New(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result datacenterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("未找到 %s 的利润表数据", code)
	}

	return result.Result.Data, nil
}

func fetchBalanceSheet(code string) (map[string]any, error) {
	filter := fmt.Sprintf("(SECURITY_CODE=\"%s\")", code)
	apiURL := fmt.Sprintf(
		"https://datacenter-web.eastmoney.com/api/data/v1/get?sortColumns=REPORT_DATE&sortTypes=-1&pageSize=1&pageNumber=1&reportName=RPT_DMSK_FN_BALANCE&columns=ALL&filter=%s",
		url.QueryEscape(filter),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://data.eastmoney.com/")

	client := httpclient.New(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result datacenterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("未找到 %s 的资产负债表数据", code)
	}

	return result.Result.Data[0], nil
}

// secucode 给纯数字代码补交易所后缀，完整报表接口(F10)按 SECUCODE 过滤。
func secucode(code string) string {
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

// fetchF10Statement 拉取完整报表(F10)。完整报表字段比摘要接口全，
// 现金流量表、合同负债、研发/无形资产等科目只在完整报表里有。
func fetchF10Statement(reportName, code string, pageSize int) (map[string]any, error) {
	filter := fmt.Sprintf(`(SECUCODE="%s")`, secucode(code))
	apiURL := fmt.Sprintf(
		"https://datacenter.eastmoney.com/securities/api/data/v1/get?sortColumns=REPORT_DATE&sortTypes=-1&pageSize=%d&pageNumber=1&reportName=%s&columns=ALL&filter=%s",
		pageSize, reportName, url.QueryEscape(filter),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://emweb.securities.eastmoney.com/")

	client := httpclient.New(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result datacenterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("未找到 %s 的 %s 数据", code, reportName)
	}
	return result.Result.Data[0], nil
}

func fetchCashFlowStatement(code string) (map[string]any, error) {
	return fetchF10Statement("RPT_F10_FINANCE_GCASHFLOW", code, 1)
}

func fetchFullBalanceSheet(code string) (map[string]any, error) {
	return fetchF10Statement("RPT_F10_FINANCE_GBALANCE", code, 1)
}

func buildReport(code string, incomeData []map[string]any, balanceData, cashData, fullBalance map[string]any) *FinanceReport {
	cur := incomeData[0]

	report := &FinanceReport{
		Code:       code,
		ReportDate: getString(cur, "REPORT_DATE"),
		Name:       getString(cur, "SECURITY_NAME_ABBR"),
	}

	rptType := getFloat64(cur, "DATE_TYPE_CODE")
	switch rptType {
	case 1:
		report.ReportType = "一季报"
	case 6:
		report.ReportType = "中报"
	case 9:
		report.ReportType = "三季报"
	case 12:
		report.ReportType = "年报"
	default:
		report.ReportType = "季报"
	}

	report.Revenue = getFloat64(cur, "TOTAL_OPERATE_INCOME")
	report.NetProfit = getFloat64(cur, "PARENT_NETPROFIT")
	report.OperatingProfit = getFloat64(cur, "OPERATE_PROFIT")
	report.TotalProfit = getFloat64(cur, "TOTAL_PROFIT")
	report.DeductedProfit = getFloat64(cur, "DEDUCT_PARENT_NETPROFIT")
	report.EPS = getFloat64(cur, "BASIC_EPS")
	report.ROE = getFloat64(cur, "WEIGHTAVG_ROE")

	report.SaleExpense = getFloat64(cur, "SALE_EXPENSE")
	report.ManageExpense = getFloat64(cur, "MANAGE_EXPENSE")
	report.FinanceExpense = getFloat64(cur, "FINANCE_EXPENSE")
	report.TotalCost = getFloat64(cur, "TOTAL_OPERATE_COST")

	operatingCost := getFloat64(cur, "OPERATE_COST")
	report.OperatingCost = operatingCost
	if report.Revenue > 0 {
		report.GrossMargin = (report.Revenue - operatingCost) / report.Revenue * 100
		report.NetMargin = report.NetProfit / report.Revenue * 100
	}

	report.TotalAssets = getFloat64(balanceData, "TOTAL_ASSETS")
	report.TotalLiabilities = getFloat64(balanceData, "TOTAL_LIABILITIES")
	report.TotalEquity = getFloat64(balanceData, "TOTAL_EQUITY")
	// 摘要接口没有 CONTRACT_LIABILITY,完整报表里叫 CONTRACT_LIAB。
	report.ContractLiability = getFloat64(fullBalance, "CONTRACT_LIAB")
	if report.ContractLiability == 0 {
		report.ContractLiability = getFloat64(balanceData, "CONTRACT_LIAB")
	}
	report.OperatingCashFlow = getFloat64(cashData, "NETCASH_OPERATE")
	report.SalesCashReceived = getFloat64(cashData, "SALES_SERVICES")
	report.PurchaseFixedAsset = getFloat64(cashData, "CONSTRUCT_LONG_ASSET")
	report.MonetaryFunds = getFloat64(balanceData, "MONETARYFUNDS")
	report.Inventory = getFloat64(balanceData, "INVENTORY")
	report.AccountsReceivable = getFloat64(balanceData, "ACCOUNTS_RECE")
	report.FixedAsset = getFloat64(balanceData, "FIXED_ASSET")
	report.DebtAssetRatio = getFloat64(balanceData, "DEBT_ASSET_RATIO")
	report.CurrentRatio = getFloat64(balanceData, "CURRENT_RATIO")

	var prev map[string]any = findSamePeriodLastYear(incomeData)
	if prev != nil {
		report.RevenuePrev = getFloat64(prev, "TOTAL_OPERATE_INCOME")
		report.NetProfitPrev = getFloat64(prev, "PARENT_NETPROFIT")

		if report.RevenuePrev > 0 {
			report.RevenueYoY = (report.Revenue - report.RevenuePrev) / report.RevenuePrev * 100
		}
		if report.NetProfitPrev > 0 {
			report.NetProfitYoY = (report.NetProfit - report.NetProfitPrev) / report.NetProfitPrev * 100
		}
	}

	return report
}

type StockSearchResult struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Market string `json:"market"`
}

func SearchStock(keyword string) ([]StockSearchResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("搜索关键词不能为空")
	}

	apiURL := fmt.Sprintf(
		"https://searchadapter.eastmoney.com/api/suggest/get?input=%s&type=14&token=D43BF722C8E33C5E18E082F7EBB5D41C",
		url.QueryEscape(keyword),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://www.eastmoney.com/")

	client := httpclient.New(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		QuotationCodeTable struct {
			Data []struct {
				Code string `json:"Code"`
				Name string `json:"Name"`
				JYS  string `json:"JYS"`
			} `json:"Data"`
			Message string `json:"Message"`
		} `json:"QuotationCodeTable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var stocks []StockSearchResult
	for _, d := range result.QuotationCodeTable.Data {
		market := "SH"
		if d.JYS == "0" {
			market = "SZ"
		}
		stocks = append(stocks, StockSearchResult{
			Code:   d.Code,
			Name:   d.Name,
			Market: market,
		})
	}
	return stocks, nil
}

func findSamePeriodLastYear(incomeData []map[string]any) map[string]any {
	if len(incomeData) < 2 {
		return nil
	}
	curTypeCode := getString(incomeData[0], "DATE_TYPE_CODE")
	for _, d := range incomeData[1:] {
		if getString(d, "DATE_TYPE_CODE") == curTypeCode {
			return d
		}
	}
	return nil
}

// PeriodDays 返回年初到报告期的天数。累计报表用它把"期内成本"还原成日均。
func PeriodDays(reportDate string) int {
	t, err := time.Parse("2006-01-02 15:04:05", reportDate)
	if err != nil {
		if t, err = time.Parse("2006-01-02", reportDate); err != nil {
			return 365
		}
	}
	days := t.YearDay()
	if days <= 0 || days > 366 {
		return 365
	}
	return days
}

func FormatFinanceReport(report *FinanceReport) string {
	if report == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s（%s）%s\n", report.Name, report.Code, report.ReportType))
	sb.WriteString(fmt.Sprintf("报告期: %s\n", report.ReportDate))
	sb.WriteString(strings.Repeat("─", 36))
	sb.WriteString("\n")

	sb.WriteString("\n📊 盈利能力\n")
	sb.WriteString(fmt.Sprintf("  营业收入: %.2f 亿", report.Revenue/1e8))
	if report.RevenuePrev > 0 {
		sb.WriteString(fmt.Sprintf("  (同比 %+.1f%%)", report.RevenueYoY))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  归母净利润: %.2f 亿", report.NetProfit/1e8))
	if report.NetProfitPrev > 0 {
		sb.WriteString(fmt.Sprintf("  (同比 %+.1f%%)", report.NetProfitYoY))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  扣非净利润: %.2f 亿\n", report.DeductedProfit/1e8))
	sb.WriteString(fmt.Sprintf("  营业利润: %.2f 亿\n", report.OperatingProfit/1e8))
	sb.WriteString(fmt.Sprintf("  毛利率: %.1f%%\n", report.GrossMargin))
	sb.WriteString(fmt.Sprintf("  净利率: %.1f%%\n", report.NetMargin))

	if report.ROE > 0 {
		sb.WriteString(fmt.Sprintf("  ROE: %.1f%%\n", report.ROE))
	}
	if report.EPS > 0 {
		sb.WriteString(fmt.Sprintf("  EPS: %.2f 元\n", report.EPS))
	}

	sb.WriteString("\n💰 费用结构\n")
	sb.WriteString(fmt.Sprintf("  总营业成本: %.2f 亿\n", report.TotalCost/1e8))
	if report.Revenue > 0 {
		sb.WriteString(fmt.Sprintf("  销售费用: %.2f 亿 (%.1f%%)\n", report.SaleExpense/1e8, report.SaleExpense/report.Revenue*100))
		sb.WriteString(fmt.Sprintf("  管理费用: %.2f 亿 (%.1f%%)\n", report.ManageExpense/1e8, report.ManageExpense/report.Revenue*100))
		sb.WriteString(fmt.Sprintf("  财务费用: %.2f 亿 (%.1f%%)\n", report.FinanceExpense/1e8, report.FinanceExpense/report.Revenue*100))
	}

	sb.WriteString("\n🏦 资产负债\n")
	sb.WriteString(fmt.Sprintf("  总资产: %.2f 亿\n", report.TotalAssets/1e8))
	sb.WriteString(fmt.Sprintf("  总负债: %.2f 亿 (%.1f%%)\n", report.TotalLiabilities/1e8, report.DebtAssetRatio))
	sb.WriteString(fmt.Sprintf("  净资产: %.2f 亿\n", report.TotalEquity/1e8))
	if report.ContractLiability > 0 {
		sb.WriteString(fmt.Sprintf("  合同负债: %.2f 亿\n", report.ContractLiability/1e8))
	}
	sb.WriteString(fmt.Sprintf("  货币资金: %.2f 亿\n", report.MonetaryFunds/1e8))
	sb.WriteString(fmt.Sprintf("  存货: %.2f 亿\n", report.Inventory/1e8))
	sb.WriteString(fmt.Sprintf("  应收账款: %.2f 亿\n", report.AccountsReceivable/1e8))

	sb.WriteString("\n💵 现金流\n")
	if report.OperatingCashFlow != 0 {
		sb.WriteString(fmt.Sprintf("  经营活动现金流净额: %.2f 亿\n", report.OperatingCashFlow/1e8))
	}
	if report.SalesCashReceived != 0 {
		sb.WriteString(fmt.Sprintf("  销售商品收到的现金: %.2f 亿\n", report.SalesCashReceived/1e8))
		if report.Revenue > 0 {
			sb.WriteString(fmt.Sprintf("  收现比(含税口径): %.3f\n", report.SalesCashReceived/report.Revenue))
		}
	}
	if report.PurchaseFixedAsset != 0 {
		sb.WriteString(fmt.Sprintf("  购建长期资产支付的现金: %.2f 亿\n", report.PurchaseFixedAsset/1e8))
	}
	if report.OperatingCashFlow == 0 && report.SalesCashReceived == 0 {
		sb.WriteString("  （现金流量表未取到,相关科目按缺失处理）\n")
	}

	sb.WriteString("\n⚕️ 财务健康\n")
	sb.WriteString(fmt.Sprintf("  资产负债率: %.1f%%\n", report.DebtAssetRatio))
	if report.CurrentRatio > 0 {
		sb.WriteString(fmt.Sprintf("  流动比率: %.1f%%\n", report.CurrentRatio))
	}
	// 存货周转必须用营业成本口径。用营收当分母会把天数压低一个量级
	// (茅台实测:营收口径 168 天 vs 成本口径 1228 天),辩论会据此误判"压货"。
	if report.Inventory > 0 && report.OperatingCost > 0 {
		// 利润表是年初至今的累计数,分母必须先按报告期天数还原成"日均成本",
		// 否则 Q3 会被当成全年,天数虚高(茅台:1642 天 vs 实际 1228 天)。
		days := PeriodDays(report.ReportDate)
		sb.WriteString(fmt.Sprintf("  存货周转(成本口径,按 %d 天): %.1f 天\n", days, float64(days)*report.Inventory/report.OperatingCost))
	}

	return sb.String()
}
