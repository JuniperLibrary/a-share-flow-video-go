package fetcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// NorthboundDaily 北向资金逐日汇总。
type NorthboundDaily struct {
	Date       string  `json:"date"`
	TotalBuy   float64 `json:"totalBuy"`   // 沪股通+深股通买入（亿）
	TotalSell  float64 `json:"totalSell"`  // 沪股通+深股通卖出（亿）
	TotalNet   float64 `json:"totalNet"`   // 净买入（亿）
	ShNetBuy   float64 `json:"shNetBuy"`   // 沪股通净买入（亿）
	SzNetBuy   float64 `json:"szNetBuy"`   // 深股通净买入（亿）
	TotalHold  float64 `json:"totalHold"`  // 累计持仓（亿）
}

// NorthboundStock 北向资金逐股持仓。
type NorthboundStock struct {
	Date        string  `json:"date"`
	StockCode   string  `json:"stockCode"`
	StockName   string  `json:"stockName"`
	Market      string  `json:"market"`      // SH / SZ
	HoldShares  float64 `json:"holdShares"`  // 持仓量（万股）
	HoldValue   float64 `json:"holdValue"`   // 持仓市值（亿）
	HoldRatio   float64 `json:"holdRatio"`   // 持股比例（%）
	ChangeShares float64 `json:"changeShares"` // 变动量（万股）
	ChangeValue  float64 `json:"changeValue"`  // 变动市值（亿）
}

type northboundSummaryResponse struct {
	Data []struct {
		Date   string  `json:"jyrq"`
		Code   string  `json:"scode"`
		Name   string  `json:"sname"`
		JRZJE  float64 `json:"jrzje"`  // 今日净买入额（元）
		LJCJE  float64 `json:"ljcje"`  // 累计成交额
		CCSZ   float64 `json:"ccsz"`   // 持仓市值（元）
		JSRZJE float64 `json:"jsrzje"` // 净买入
	} `json:"data"`
}

type northboundStockResponse struct {
	Result struct {
		Data []map[string]any `json:"data"`
	} `json:"result"`
	Success bool `json:"success"`
}

// FetchNorthboundDaily 获取最近 days 天北向资金汇总。
func FetchNorthboundDaily(days int) ([]NorthboundDaily, error) {
	apiURL := fmt.Sprintf(
		"https://datacenter-web.eastmoney.com/api/data/v1/get?reportName=RPT_NORTH_FLOW&columns=ALL&pageSize=%d&pageNumber=1&sortColumns=TRADE_DATE&sortTypes=-1",
		days,
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://data.eastmoney.com/hsgt/index.html")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request northbound daily: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Result struct {
			Data []map[string]any `json:"data"`
		} `json:"result"`
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode northbound daily: %w", err)
	}
	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("no northbound data")
	}

	var dailies []NorthboundDaily
	for _, d := range result.Result.Data {
		dateStr := getString(d, "TRADE_DATE")
		if dateStr == "" {
			continue
		}
		nd := NorthboundDaily{
			Date:      dateStr,
			TotalBuy:  roundTo2(getNbFloat(d, "BUY_AMT", 1e8)),
			TotalSell: roundTo2(getNbFloat(d, "SELL_AMT", 1e8)),
			TotalNet:  roundTo2(getNbFloat(d, "NET_BUY_AMT", 1e8)),
			ShNetBuy:  roundTo2(getNbFloat(d, "SH_NET_BUY", 1e8)),
			SzNetBuy:  roundTo2(getNbFloat(d, "SZ_NET_BUY", 1e8)),
			TotalHold: roundTo2(getNbFloat(d, "CCE_AMT", 1e8)),
		}
		dailies = append(dailies, nd)
	}
	return dailies, nil
}

// FetchNorthboundStocks 获取指定日期的北向资金持股明细。
func FetchNorthboundStocks(dateStr string) ([]NorthboundStock, error) {
	// 沪股通
	shStocks, err := fetchNorthboundByMarket(dateStr, "SH")
	if err != nil {
		logger.Warn("沪股通数据获取失败", zap.String("date", dateStr), zap.Error(err))
	}
	// 深股通
	szStocks, err2 := fetchNorthboundByMarket(dateStr, "SZ")
	if err2 != nil {
		logger.Warn("深股通数据获取失败", zap.String("date", dateStr), zap.Error(err2))
	}

	var all []NorthboundStock
	all = append(all, shStocks...)
	all = append(all, szStocks...)

	if len(all) == 0 {
		return nil, fmt.Errorf("no northbound stock data for %s", dateStr)
	}

	logger.Info("北向资金持股数据获取成功",
		zap.String("date", dateStr),
		zap.Int("count", len(all)),
	)
	return all, nil
}

func fetchNorthboundByMarket(dateStr, market string) ([]NorthboundStock, error) {
	// 港股通持股明细接口
	reportName := "RPT_HK_STOCK_HOLD"
	// 沪股通: SH / 深股通: SZ
	filter := fmt.Sprintf("(TRADE_DATE='%s')(MARKET_TYPE=\"%s\")", dateStr, market)
	apiURL := fmt.Sprintf(
		"https://datacenter-web.eastmoney.com/api/data/v1/get?reportName=%s&columns=ALL&filter=%s&pageSize=200&pageNumber=1&sortColumns=HOLD_SHARES&sortTypes=-1",
		reportName, url.QueryEscape(filter),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://data.eastmoney.com/hsgt/index.html")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request northbound stocks %s: %w", market, err)
	}
	defer resp.Body.Close()

	var result northboundStockResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode northbound stocks %s: %w", market, err)
	}
	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("no northbound stock data for %s %s", dateStr, market)
	}

	var stocks []NorthboundStock
	for _, d := range result.Result.Data {
		code := getString(d, "SECURITY_CODE")
		name := getString(d, "SECURITY_NAME_ABBR")
		if code == "" || name == "" {
			continue
		}
		holdShares := getNbFloat(d, "HOLD_SHARES", 1e4)   // 股→万股
		holdValue := getNbFloat(d, "HOLD_MARKET_CAP", 1e8) // 元→亿
		changeShares := getNbFloat(d, "CHANGE_SHARES", 1e4)
		changeValue := getNbFloat(d, "CHANGE_MARKET_CAP", 1e8)

		// 只保留有变动的
		if changeShares == 0 {
			continue
		}

		stock := NorthboundStock{
			Date:         dateStr,
			StockCode:    code,
			StockName:    name,
			Market:       market,
			HoldShares:   roundTo2(holdShares),
			HoldValue:    roundTo2(holdValue),
			HoldRatio:    roundTo2(getNbFloat(d, "HOLD_RATIO", 1)),
			ChangeShares: roundTo2(changeShares),
			ChangeValue:  roundTo2(changeValue),
		}
		stocks = append(stocks, stock)
	}
	return stocks, nil
}

// FetchNorthboundAccumulate 获取指定日期区间北向累计净买入（用于趋势分析）。
type NorthboundAccumulate struct {
	Date     string  `json:"date"`
	AccumNet float64 `json:"accumNet"` // 区间累计净买入
}

// getNbFloat 从北向 API 返回值中解析浮点数，并除以 divisor。
func getNbFloat(data map[string]any, key string, divisor float64) float64 {
	val, ok := data[key]
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v / divisor
	case string:
		f, _ := parseFloat64(v)
		return f / divisor
	case json.Number:
		f, _ := v.Float64()
		return f / divisor
	}
	return 0
}

func parseFloat64(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
