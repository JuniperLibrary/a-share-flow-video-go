package fetcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// DragonTigerStock 龙虎榜个股数据。
type DragonTigerStock struct {
	Date         string  `json:"date"`
	StockCode    string  `json:"stockCode"`
	StockName    string  `json:"stockName"`
	ClosePrice   float64 `json:"closePrice"`   // 收盘价（元）
	ChangePct    float64 `json:"changePct"`    // 涨跌幅（%）
	NetBuyAmt    float64 `json:"netBuyAmt"`    // 净买入额（亿）
	BuyAmt       float64 `json:"buyAmt"`       // 买入额（亿）
	SellAmt      float64 `json:"sellAmt"`      // 卖出额（亿）
	DealerNetBuy float64 `json:"dealerNetBuy"` // 营业部净买入（亿）
	Reason       string  `json:"reason"`       // 上榜原因
	DealerName   string  `json:"dealerName"`   // 营业部/席位名称
	IsOrg        bool    `json:"isOrg"`         // 是否为机构席位
}

type billboardResponse struct {
	Result struct {
		Data []map[string]any `json:"data"`
	} `json:"result"`
	Success bool `json:"success"`
}

// FetchDragonTiger 获取指定日期的龙虎榜数据。
func FetchDragonTiger(dateStr string) ([]DragonTigerStock, error) {
	filter := fmt.Sprintf("(TRADE_DATE='%s')", dateStr)
	apiURL := fmt.Sprintf(
		"https://datacenter-web.eastmoney.com/api/data/v1/get?reportName=RPT_BILLBOARD_DAILY&columns=ALL&filter=%s&pageSize=100&pageNumber=1&sortColumns=NET_BUY_AMT&sortTypes=-1",
		url.QueryEscape(filter),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://data.eastmoney.com/stock/tradedetail.html")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request dragon tiger: %w", err)
	}
	defer resp.Body.Close()

	var result billboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode dragon tiger: %w", err)
	}
	if !result.Success || len(result.Result.Data) == 0 {
		return nil, fmt.Errorf("no dragon tiger data for %s", dateStr)
	}

	var stocks []DragonTigerStock
	for _, d := range result.Result.Data {
		code := getString(d, "SECURITY_CODE")
		name := getString(d, "SECURITY_NAME_ABBR")
		if code == "" || name == "" {
			continue
		}
		netBuy, _ := toFloat64(d["NET_BUY_AMT"])
		if netBuy == 0 {
			continue
		}
		stock := DragonTigerStock{
			Date:         dateStr,
			StockCode:    code,
			StockName:    name,
			ClosePrice:   roundTo2(getFloat64(d, "CLOSE_PRICE")),
			ChangePct:    roundTo2(getFloat64(d, "CHANGE_RATE")),
			NetBuyAmt:    roundTo2(netBuy / 1e8),
			BuyAmt:       roundTo2(getFloat64(d, "BUY_AMT") / 1e8),
			SellAmt:      roundTo2(getFloat64(d, "SELL_AMT") / 1e8),
			DealerNetBuy: roundTo2(getFloat64(d, "OPERATEDEPT_NET_BUY") / 1e8),
			Reason:       getString(d, "BILLBOARD_REASON"),
			DealerName:   getString(d, "OPERATEDEPT_NAME"),
			IsOrg:        strings.Contains(getString(d, "OPERATEDEPT_NAME"), "机构"),
		}
		stocks = append(stocks, stock)
	}

	if len(stocks) == 0 {
		return nil, fmt.Errorf("no dragon tiger records for %s", dateStr)
	}
	// 按净买入金额降序
	sort.Slice(stocks, func(i, j int) bool {
		return stocks[i].NetBuyAmt > stocks[j].NetBuyAmt
	})

	logger.Info("龙虎榜数据获取成功",
		zap.String("date", dateStr),
		zap.Int("count", len(stocks)),
		zap.String("top", stocks[0].StockName),
	)
	return stocks, nil
}

// FetchDragonTigerRecent 获取最近 N 个交易日的龙虎榜汇总数据。
func FetchDragonTigerRecent(days int) ([]DragonTigerStock, error) {
	// 获取最近 days 天的数据（跳过周末，最多尝试 days*2 次）
	var all []DragonTigerStock
	t := time.Now()
	attempted := 0

	for i := 0; i < days*2 && attempted < days; i++ {
		dateStr := t.AddDate(0, 0, -i).Format("2006-01-02")
		w := t.AddDate(0, 0, -i).Weekday()
		if w == time.Saturday || w == time.Sunday {
			continue
		}
		stocks, err := FetchDragonTiger(dateStr)
		if err == nil {
			all = append(all, stocks...)
			attempted++
		}
	}
	return all, nil
}

// AggregateDragonTigerBySector 将龙虎榜个股按板块汇聚。
func AggregateDragonTigerBySector(stocks []DragonTigerStock) map[string]struct {
	StockCount int
	TotalBuy   float64
	TopReason  string
} {
	result := make(map[string]struct {
		StockCount int
		TotalBuy   float64
		TopReason  string
	})
	// 简化的板块映射（根据股票简称判断）
	for _, s := range stocks {
		// 这里只是占位，实际需要更准确的板块标签
		sector := "其他"
		if strings.Contains(s.StockName, "半导体") || strings.Contains(s.StockName, "芯片") {
			sector = "半导体"
		} else if strings.Contains(s.StockName, "AI") || strings.Contains(s.StockName, "人工智能") {
			sector = "AI"
		} else if strings.Contains(s.StockName, "新能源") || strings.Contains(s.StockName, "锂电") {
			sector = "新能源"
		} else if strings.Contains(s.StockName, "医") {
			sector = "医药"
		} else if strings.Contains(s.StockName, "银行") || strings.Contains(s.StockName, "券商") || strings.Contains(s.StockName, "保险") {
			sector = "金融"
		}
		entry := result[sector]
		entry.StockCount++
		entry.TotalBuy += s.NetBuyAmt
		if entry.TopReason == "" {
			entry.TopReason = s.Reason
		}
		result[sector] = entry
	}
	return result
}

// getFloat64 helper for datacenter API returns (map[string]any).
func getFloat64(data map[string]any, key string) float64 {
	val, ok := data[key]
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}

// getString helper for datacenter API returns (map[string]any).
func getString(data map[string]any, key string) string {
	val, ok := data[key]
	if !ok || val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}
