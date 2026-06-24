package fetcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// MarginSummary 两市融资融券余额汇总。
type MarginSummary struct {
	Date      string  `json:"date"`      // 交易日
	RZYE      float64 `json:"rzye"`      // 融资余额（亿）
	RQYE      float64 `json:"rqye"`      // 融券余额（亿）
	RZMRE     float64 `json:"rzmre"`     // 融资买入额（亿）
	RZCHE     float64 `json:"rzche"`     // 融资偿还额（亿）
	RZNetBuy  float64 `json:"rzNetBuy"`  // 融资净买入（亿）= RZMRE - RZCHE
	RQYL      float64 `json:"rqyl"`      // 融券余量（万股）
	RQMCL     float64 `json:"rqmcl"`     // 融券卖出量（万股）
	RQCHL     float64 `json:"rqchl"`     // 融券偿还量（万股）
}

// MarginStock 个股融资融券明细。
type MarginStock struct {
	Date       string  `json:"date"`
	StockCode  string  `json:"stockCode"`
	StockName  string  `json:"stockName"`
	RZYE       float64 `json:"rzye"`       // 融资余额（亿）
	RZMRE      float64 `json:"rzmre"`      // 融资买入（亿）
	RZCHE      float64 `json:"rzche"`      // 融资偿还（亿）
	RZNet      float64 `json:"rzNet"`      // 融资净买入（亿）
	RQYL       float64 `json:"rqyl"`       // 融券余量（万股）
	RQYE       float64 `json:"rqye"`       // 融券余额（亿）
	RQMCL      float64 `json:"rqmcl"`      // 融券卖出（万股）
	RQCHL      float64 `json:"rqchl"`      // 融券偿还（万股）
}

type marginResponse struct {
	Result struct {
		Data []map[string]any `json:"data"`
	} `json:"result"`
	Success bool `json:"success"`
}

// FetchMarginSummary 获取最近 days 天的融资融券余额汇总。
func FetchMarginSummary(days int) ([]MarginSummary, error) {
	apiURL := fmt.Sprintf(
		"https://datacenter-web.eastmoney.com/api/data/v1/get?reportName=RPT_MARGIN_TRADE_SUMMARY&columns=ALL&pageSize=%d&pageNumber=1&sortColumns=TRADE_DATE&sortTypes=-1",
		days,
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://data.eastmoney.com/rzrq/total.html")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request margin summary: %w", err)
	}
	defer resp.Body.Close()

	var result marginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode margin summary: %w", err)
	}
	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("no margin data")
	}

	var summaries []MarginSummary
	for _, d := range result.Result.Data {
		dateStr := getString(d, "TRADE_DATE")
		if dateStr == "" {
			continue
		}
		s := MarginSummary{
			Date:  dateStr,
			RZYE:  roundTo2(getMarginFloat(d, "RZYE", 1e8)),
			RQYE:  roundTo2(getMarginFloat(d, "RQYE", 1e8)),
			RZMRE: roundTo2(getMarginFloat(d, "RZMRE", 1e8)),
			RZCHE: roundTo2(getMarginFloat(d, "RZCHE", 1e8)),
			RQYL:  roundTo2(getMarginFloat(d, "RQYL", 1e4)),
			RQMCL: roundTo2(getMarginFloat(d, "RQMCL", 1e4)),
			RQCHL: roundTo2(getMarginFloat(d, "RQCHL", 1e4)),
		}
		s.RZNetBuy = roundTo2(s.RZMRE - s.RZCHE)
		summaries = append(summaries, s)
	}

	return summaries, nil
}

// FetchMarginStocks 获取指定日期的个股融资融券明细。
func FetchMarginStocks(dateStr string) ([]MarginStock, error) {
	filter := fmt.Sprintf("(TRADE_DATE='%s')", dateStr)
	apiURL := fmt.Sprintf(
		"https://datacenter-web.eastmoney.com/api/data/v1/get?reportName=RPT_MARGIN_TRADE_DETAIL&columns=ALL&filter=%s&pageSize=200&pageNumber=1&sortColumns=RZYE&sortTypes=-1",
		url.QueryEscape(filter),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://data.eastmoney.com/rzrq/detail.html")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request margin stocks: %w", err)
	}
	defer resp.Body.Close()

	var result marginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode margin stocks: %w", err)
	}
	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("no margin stock data for %s", dateStr)
	}

	var stocks []MarginStock
	for _, d := range result.Result.Data {
		code := getString(d, "SECURITY_CODE")
		name := getString(d, "SECURITY_NAME_ABBR")
		if code == "" || name == "" {
			continue
		}
		stock := MarginStock{
			Date:      dateStr,
			StockCode: code,
			StockName: name,
			RZYE:      roundTo2(getMarginFloat(d, "RZYE", 1e8)),
			RZMRE:     roundTo2(getMarginFloat(d, "RZMRE", 1e8)),
			RZCHE:     roundTo2(getMarginFloat(d, "RZCHE", 1e8)),
			RQYL:      roundTo2(getMarginFloat(d, "RQYL", 1e4)),
			RQYE:      roundTo2(getMarginFloat(d, "RQYE", 1e8)),
			RQMCL:     roundTo2(getMarginFloat(d, "RQMCL", 1e4)),
			RQCHL:     roundTo2(getMarginFloat(d, "RQCHL", 1e4)),
		}
		stock.RZNet = roundTo2(stock.RZMRE - stock.RZCHE)
		// 只保留有成交的
		if stock.RZMRE > 0 || stock.RZCHE > 0 {
			stocks = append(stocks, stock)
		}
	}

	sort.Slice(stocks, func(i, j int) bool {
		return stocks[i].RZMRE > stocks[j].RZMRE
	})

	logger.Info("融资融券个股数据获取成功",
		zap.String("date", dateStr),
		zap.Int("count", len(stocks)),
	)
	return stocks, nil
}

// FetchMarginStockHistory 获取个股历史融资融券数据。
func FetchMarginStockHistory(code string, days int) ([]MarginStock, error) {
	filter := fmt.Sprintf("(SECURITY_CODE=\"%s\")", code)
	apiURL := fmt.Sprintf(
		"https://datacenter-web.eastmoney.com/api/data/v1/get?reportName=RPT_MARGIN_TRADE_DETAIL&columns=ALL&filter=%s&pageSize=%d&pageNumber=1&sortColumns=TRADE_DATE&sortTypes=-1",
		url.QueryEscape(filter), days,
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://data.eastmoney.com/rzrq/detail.html")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request margin stock history: %w", err)
	}
	defer resp.Body.Close()

	var result marginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode margin stock history: %w", err)
	}
	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("no margin history for %s", code)
	}

	var stocks []MarginStock
	for _, d := range result.Result.Data {
		dateStr := getString(d, "TRADE_DATE")
		name := getString(d, "SECURITY_NAME_ABBR")
		if dateStr == "" {
			continue
		}
		stock := MarginStock{
			Date:      dateStr,
			StockCode: code,
			StockName: name,
			RZYE:      roundTo2(getMarginFloat(d, "RZYE", 1e8)),
			RZMRE:     roundTo2(getMarginFloat(d, "RZMRE", 1e8)),
			RZCHE:     roundTo2(getMarginFloat(d, "RZCHE", 1e8)),
			RQYL:      roundTo2(getMarginFloat(d, "RQYL", 1e4)),
			RQYE:      roundTo2(getMarginFloat(d, "RQYE", 1e8)),
			RQMCL:     roundTo2(getMarginFloat(d, "RQMCL", 1e4)),
			RQCHL:     roundTo2(getMarginFloat(d, "RQCHL", 1e4)),
		}
		stock.RZNet = roundTo2(stock.RZMRE - stock.RZCHE)
		stocks = append(stocks, stock)
	}
	return stocks, nil
}

// getMarginFloat 从 datacenter API 返回值中解析浮点数，并除以 divisor。
func getMarginFloat(data map[string]any, key string, divisor float64) float64 {
	val, ok := data[key]
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v / divisor
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0
		}
		return f / divisor
	case json.Number:
		f, _ := v.Float64()
		return f / divisor
	}
	return 0
}
