package fetcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type FinanceReport struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	ReportDate string  `json:"reportDate"`
	ReportType string  `json:"reportType"`

	Revenue        float64 `json:"revenue"`
	RevenuePrev    float64 `json:"revenuePrev"`
	RevenueYoY     float64 `json:"revenueYoY"`
	NetProfit      float64 `json:"netProfit"`
	NetProfitPrev  float64 `json:"netProfitPrev"`
	NetProfitYoY   float64 `json:"netProfitYoY"`
	DeductedProfit float64 `json:"deductedProfit"`
	OperatingProfit float64 `json:"operatingProfit"`
	TotalProfit    float64 `json:"totalProfit"`

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

	OperatingCashFlow float64 `json:"operatingCashFlow"`
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

	report := buildReport(code, incomeData, balanceData)
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

	client := &http.Client{Timeout: 30 * time.Second}
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

	client := &http.Client{Timeout: 30 * time.Second}
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

func buildReport(code string, incomeData []map[string]any, balanceData map[string]any) *FinanceReport {
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
	if report.Revenue > 0 {
		report.GrossMargin = (report.Revenue - operatingCost) / report.Revenue * 100
		report.NetMargin = report.NetProfit / report.Revenue * 100
	}

	report.TotalAssets = getFloat64(balanceData, "TOTAL_ASSETS")
	report.TotalLiabilities = getFloat64(balanceData, "TOTAL_LIABILITIES")
	report.TotalEquity = getFloat64(balanceData, "TOTAL_EQUITY")
	report.ContractLiability = getFloat64(balanceData, "CONTRACT_LIABILITY")
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

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		QuotationCodeTable struct {
			Data []struct {
				Code   string `json:"Code"`
				Name   string `json:"Name"`
				JYS    string `json:"JYS"`
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

	sb.WriteString("\n⚕️ 财务健康\n")
	sb.WriteString(fmt.Sprintf("  资产负债率: %.1f%%\n", report.DebtAssetRatio))
	if report.CurrentRatio > 0 {
		sb.WriteString(fmt.Sprintf("  流动比率: %.1f%%\n", report.CurrentRatio))
	}
	if report.Revenue > 0 {
		invTurnover := report.Revenue / report.Inventory
		sb.WriteString(fmt.Sprintf("  存货周转: %.1f 天\n", 365/invTurnover))
	}

	return sb.String()
}
