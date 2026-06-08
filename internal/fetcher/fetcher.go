package fetcher

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

// Sector 表示一个板块的资金流向数据。
type Sector struct {
	Name       string  `json:"name"`
	Net        float64 `json:"net"`        // 主力净流入（亿）
	Rate       float64 `json:"rate"`       // 主力净占比（%），如 3.93
	ChangePct  float64 `json:"change_pct"` // 涨跌幅（%），如 1.23（f3 字段）
	SuperNet   float64 `json:"super_net"`  // 超大单净流入（亿）（f66 字段）
	SuperRate  float64 `json:"super_rate"` // 超大单净占比（%）（f69 字段）
	BigNet     float64 `json:"big_net"`    // 大单净流入（亿）（f72 字段）
	BigRate    float64 `json:"big_rate"`   // 大单净占比（%）（f75 字段）
	Volume     float64 `json:"volume"`     // 成交量（手，f5 字段），tick 数据用
	Turnover   float64 `json:"turnover"`   // 成交额（亿，f6 字段 ÷1e8），tick 数据用
	Color      string  `json:"color"`      // 运行时由前端/渲染层分配，不持久化到 CSV
	Category   string  `json:"category"`   // "industry" 行业 / "concept" 概念 / "" 未知
}

// Top21HotSectors 当前市场最热门的 21 个板块。
var Top21HotSectors = []string{
	"半导体", "AI应用", "CPO概念", "有色金属", "锂矿概念",
	"商业航天", "电池", "机器人", "创新药", "白酒",
	"消费电子", "银行", "人工智能", "云计算", "低空经济",
	"电网设备", "通信设备", "传媒", "国产芯片", "元件", "通信服务",
}

type emResponse struct {
	Data struct {
		Diff []map[string]any `json:"diff"`
	} `json:"data"`
}

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	},
}

func newRequest(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://emdatah5.eastmoney.com/dc/zjlx/index")
	return req, nil
}

func fetchEMRaw(fs string) ([]map[string]any, error) {
	var allDiff []map[string]any
	pn := 1

	for {
		var result emResponse
		var err error

		for attempt := 1; attempt <= 3; attempt++ {
			url := fmt.Sprintf("https://emdatah5.eastmoney.com/dc/ZJLX/getZDYLBData?fields=f12,f14,f3,f5,f6,f62,f66,f69,f72,f75,f184&pn=%d&pz=500&fid=f62&po=1&fs=%s&ut=b2884a393a59ad64002292a3e90d46a5", pn, fs)

			req, reqErr := newRequest("GET", url)
			if reqErr != nil {
				return nil, reqErr
			}
			resp, reqErr := httpClient.Do(req)
			if reqErr != nil {
				if attempt < 3 {
					time.Sleep(time.Duration(attempt) * 2 * time.Second)
					continue
				}
				return nil, reqErr
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				if attempt < 3 {
					time.Sleep(time.Duration(attempt) * 2 * time.Second)
					continue
				}
				return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
			}

			if reqErr = decodeJSON(resp.Body, &result); reqErr != nil {
				resp.Body.Close()
				if attempt < 3 {
					time.Sleep(time.Duration(attempt) * 2 * time.Second)
					continue
				}
				return nil, reqErr
			}
			resp.Body.Close()
			break
		}
		if err != nil {
			return nil, err
		}

		if result.Data.Diff == nil {
			break
		}
		allDiff = append(allDiff, result.Data.Diff...)

		if len(result.Data.Diff) < 100 {
			break
		}
		pn++
	}

	if len(allDiff) == 0 {
		return nil, fmt.Errorf("empty response")
	}
	return allDiff, nil
}

func decodeJSON(r io.Reader, v any) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func fetchPrimaryData() ([]Sector, error) {
	raw, err := fetchEMRaw("m:90+t:2")
	if err != nil {
		return nil, err
	}

	var sectors []Sector
	for _, item := range raw {
		name, _ := item["f14"].(string)
		netVal := item["f62"]
		if name == "" || netVal == nil {
			continue
		}
		netFloat, ok := toFloat64(netVal)
		if !ok || netFloat == 0 {
			continue
		}
		rateVal := item["f184"]
		rateFloat, _ := toFloat64(rateVal)
		changePctVal := item["f3"]
		changePctFloat, _ := toFloat64(changePctVal)
		superNetVal := item["f66"]
		superNetFloat, _ := toFloat64(superNetVal)
		superRateVal := item["f69"]
		superRateFloat, _ := toFloat64(superRateVal)
		bigNetVal := item["f72"]
		bigNetFloat, _ := toFloat64(bigNetVal)
		bigRateVal := item["f75"]
		bigRateFloat, _ := toFloat64(bigRateVal)
		volumeVal := item["f5"]
		volumeFloat, _ := toFloat64(volumeVal)
		turnoverVal := item["f6"]
		turnoverFloat, _ := toFloat64(turnoverVal)
		sectors = append(sectors, Sector{
			Name:      name,
			Net:       roundTo2(netFloat / 1e8),
			Rate:      roundTo2(rateFloat),
			ChangePct: roundTo2(changePctFloat),
			SuperNet:  roundTo2(superNetFloat / 1e8),
			SuperRate: roundTo2(superRateFloat),
			BigNet:    roundTo2(bigNetFloat / 1e8),
			BigRate:   roundTo2(bigRateFloat),
			Volume:    roundTo2(volumeFloat),
			Turnover:  roundTo2(turnoverFloat / 1e8),
			Category:  "industry",
		})
	}

	logger.Info("东方财富 H5 板块获取成功", zap.Int("count", len(sectors)))
	return sectors, nil
}

// FetchTop21HotSectors 获取 Top21HotSectors 的实时资金流数据。
func FetchTop21HotSectors() ([]Sector, error) {
	all, err := fetchPrimaryData()
	if err != nil {
		return nil, err
	}

	targetSet := make(map[string]bool, len(Top21HotSectors))
	for _, t := range Top21HotSectors {
		targetSet[t] = true
	}

	var results []Sector
	for _, s := range all {
		if targetSet[s.Name] {
			results = append(results, s)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return absF(results[i].Net) > absF(results[j].Net)
	})

	if len(results) == 0 {
		logger.Warn("热门板块匹配结果为空")
		return results, nil
	}

	logger.Info("热门板块匹配",
		zap.Int("count", len(results)),
		zap.String("tick_symbol", results[0].Name),
		zap.String("session", fmt.Sprintf("%+.1f亿", results[0].Net)))

	return results, nil
}

func fetchBKCodes() (map[string]string, error) {
	url := "https://82.push2.eastmoney.com/api/qt/clist/get?pn=1&pz=500&po=1&np=1&fltt=2&invt=2&fid=f62&fs=m:90+t:2&fields=f12,f14&ut=b2884a393a59ad64002292a3e90d46a5"

	req, err := newRequest("GET", url)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://data.eastmoney.com/bkzj/hy.html")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result emResponse
	if err := decodeJSON(resp.Body, &result); err != nil {
		return nil, err
	}

	mapping := make(map[string]string)
	for _, item := range result.Data.Diff {
		code, _ := item["f12"].(string)
		name, _ := item["f14"].(string)
		if code != "" && name != "" {
			mapping[name] = code
		}
	}

	logger.Info("获取板块 BK 代码", zap.Int("count", len(mapping)))
	return mapping, nil
}

func fetchSingleSectorHistorical(bkCode, sectorName, dateStr string) (*Sector, bool) {
	secid := fmt.Sprintf("90.%s", bkCode)
	url := fmt.Sprintf("https://push2his.eastmoney.com/api/qt/stock/fflow/daykline/get?secid=%s&lmt=30&klt=101&fields1=f1,f2,f3,f7&fields2=f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61,f62,f63,f64,f65&ut=b2884a393a59ad64002292a3e90d46a5", secid)

	req, err := newRequest("GET", url)
	if err != nil {
		return nil, true
	}
	req.Header.Set("Referer", "https://data.eastmoney.com/bkzj/hy.html")

	for attempt := 0; attempt < 2; attempt++ {
		resp, err := httpClient.Do(req)
		if err != nil {
			if attempt == 0 {
				time.Sleep(time.Second)
				continue
			}
			return nil, true
		}

		var data struct {
			Data struct {
				Klines []string `json:"klines"`
			} `json:"data"`
		}
		if err := decodeJSON(resp.Body, &data); err != nil {
			resp.Body.Close()
			return nil, false
		}
		resp.Body.Close()

		for _, kline := range data.Data.Klines {
			parts := strings.Split(kline, ",")
			if len(parts) < 2 {
				continue
			}
			if parts[0] == dateStr {
				netYuan, err := strconv.ParseFloat(parts[1], 64)
				if err != nil {
					return nil, false
				}
				sector := &Sector{
					Name: sectorName,
					Net:  roundTo2(netYuan / 1e8),
				}
				// Parse additional fields if present (f53=小单 f54=中单 f55=大单 f56=超大单 f57-61=占比 f63=涨跌幅)
				if len(parts) > 5 {
					if superNet, err := strconv.ParseFloat(parts[5], 64); err == nil {
						sector.SuperNet = roundTo2(superNet / 1e8)
					}
				}
				if len(parts) > 4 {
					if bigNet, err := strconv.ParseFloat(parts[4], 64); err == nil {
						sector.BigNet = roundTo2(bigNet / 1e8)
					}
				}
				if len(parts) > 12 {
					if changePct, err := strconv.ParseFloat(parts[12], 64); err == nil {
						sector.ChangePct = roundTo2(changePct)
					}
				}
				return sector, false
			}
		}
		return nil, false
	}
	return nil, true
}

// FetchHistoricalSectors 获取指定日期的历史板块数据。
func FetchHistoricalSectors(dateStr string) ([]Sector, error) {
	logger.Info("获取历史板块数据", zap.String("date", dateStr))

	bkMapping, err := fetchBKCodes()
	if err != nil || len(bkMapping) == 0 {
		logger.Warn("无法获取板块 BK 代码")
		return nil, nil
	}

	var targets []string
	for _, name := range Top21HotSectors {
		if _, ok := bkMapping[name]; ok {
			targets = append(targets, name)
		}
	}

	if len(targets) == 0 {
		logger.Warn("无匹配的历史板块")
		return nil, nil
	}

	var results []Sector
	consecutiveErrors := 0
	maxConsecutiveErrors := 5

	for _, name := range targets {
		bkCode := bkMapping[name]
		time.Sleep(500 * time.Millisecond)

		result, isErr := fetchSingleSectorHistorical(bkCode, name, dateStr)
		if result != nil {
			results = append(results, *result)
			consecutiveErrors = 0
		} else if isErr {
			consecutiveErrors++
			if consecutiveErrors >= maxConsecutiveErrors {
				logger.Error("连续请求失败，IP 可能被临时封锁", zap.Int("errors", maxConsecutiveErrors))
				break
			}
		} else {
			consecutiveErrors = 0
		}
	}
	if len(results) == 0 {
		logger.Warn("未找到匹配的历史数据", zap.String("date", dateStr))
		return nil, nil
	}

	sort.Slice(results, func(i, j int) bool {
		return absF(results[i].Net) > absF(results[j].Net)
	})

	var inflow, outflow []Sector
	for _, s := range results {
		if s.Net >= 0 {
			inflow = append(inflow, s)
		} else {
			outflow = append(outflow, s)
		}
	}
	if len(inflow) > 10 {
		inflow = inflow[:10]
	}
	if len(outflow) > 10 {
		outflow = outflow[:10]
	}

	result := append(inflow, outflow...)
	logger.Info("历史热门板块",
		zap.Int("inflow", len(inflow)),
		zap.Int("outflow", len(outflow)),
		zap.Int("total", len(result)))
	if len(result) > 0 {
		logger.Info("流入榜首", zap.String("tick_symbol", result[0].Name), zap.String("session", fmt.Sprintf("%+.1f亿", result[0].Net)))
		if len(inflow) < len(result) {
			logger.Info("流出榜首", zap.String("tick_symbol", result[len(inflow)].Name), zap.String("session", fmt.Sprintf("%+.1f亿", result[len(inflow)].Net)))
		}
	}
	return result, nil
}

func SaveDailyData(sectors []Sector, dateStr string) error {
	dateDir := filepath.Join(config.GetDataDir(), dateStr)
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return err
	}

	fpath := filepath.Join(dateDir, "sectors.csv")
	f, err := os.Create(fpath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{"name", "net", "rate", "change_pct", "super_net", "super_rate", "big_net", "big_rate", "volume", "turnover"})
	for _, s := range sectors {
		w.Write([]string{
			s.Name,
			strconv.FormatFloat(s.Net, 'f', 2, 64),
			strconv.FormatFloat(s.Rate, 'f', 2, 64),
			strconv.FormatFloat(s.ChangePct, 'f', 2, 64),
			strconv.FormatFloat(s.SuperNet, 'f', 2, 64),
			strconv.FormatFloat(s.SuperRate, 'f', 2, 64),
			strconv.FormatFloat(s.BigNet, 'f', 2, 64),
			strconv.FormatFloat(s.BigRate, 'f', 2, 64),
			strconv.FormatFloat(s.Volume, 'f', 2, 64),
			strconv.FormatFloat(s.Turnover, 'f', 2, 64),
		})
	}

	// Also persist to SQLite
	if db, err := storage.Get(); err == nil {
		inputDate := time.Now().Format("2006-01-02 15:04:05")
		records := make([]storage.Sector, 0, len(sectors))
		for _, s := range sectors {
		records = append(records, storage.Sector{
			Datetime:  storage.DateToDatetime(dateStr),
			Name:      s.Name,
			Net:       s.Net,
			Rate:      s.Rate,
			ChangePct: s.ChangePct,
			SuperNet:  s.SuperNet,
			SuperRate: s.SuperRate,
			BigNet:    s.BigNet,
			BigRate:   s.BigRate,
			Volume:    s.Volume,
			Turnover:  s.Turnover,
			InputDate: inputDate,
		})
		}
		_ = db.SaveSectors(records)
	}

	logger.Info("数据已保存", zap.String("path", fpath))
	return nil
}

func LoadCachedData(dateStr string) ([]Sector, error) {
	return LoadSessionData(dateStr, "full")
}

func LoadSessionData(dateStr, session string) ([]Sector, error) {
	if session == "morning" {
		return LoadMorningFromTick(dateStr)
	}
	return loadCSV("sectors.csv", dateStr)
}

func LoadMorningFromTick(dateStr string) ([]Sector, error) {
	points, err := loadTickCSV(dateStr, "morning")
	if err != nil || len(points) == 0 {
		return nil, fmt.Errorf("no morning tick data for %s", dateStr)
	}

	latest := make(map[string]float64)
	for _, p := range points {
		latest[p.Name] = p.Net
	}

	var sectors []Sector
	for name, net := range latest {
		sectors = append(sectors, Sector{Name: name, Net: net})
	}

	sort.Slice(sectors, func(i, j int) bool {
		return absF(sectors[i].Net) > absF(sectors[j].Net)
	})

	return sectors, nil
}

func loadTickCSV(dateStr, session string) ([]TickPoint, error) {
	db, err := storage.Get()
	if err != nil {
		return nil, err
	}

	sectors, err := db.LoadTickSectors(dateStr)
	if err != nil {
		return nil, err
	}

	var points []TickPoint
	for _, s := range sectors {
		timeStr := storage.ExtractTime(s.Datetime)
		if timeStr == "" {
			continue
		}
		if session == "morning" && !isMorningTime(timeStr) {
			continue
		}
		points = append(points, TickPoint{
			Time:     timeStr,
			Name:     s.Name,
			Net:      s.Net,
			Volume:   s.Volume,
			Turnover: s.Turnover,
		})
	}
	return points, nil
}

func isMorningTime(t string) bool {
	return t >= "09:30" && t <= "11:30"
}

type TickPoint struct {
	Time     string
	Name     string
	Net      float64
	Volume   float64
	Turnover float64
}

func loadCSV(filename, dateStr string) ([]Sector, error) {
	filepath := filepath.Join(config.GetDataDir(), dateStr, filename)

	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}

	colIdx := make(map[string]int)
	for i, h := range header {
		colIdx[h] = i
	}

	var sectors []Sector
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		netVal, _ := strconv.ParseFloat(getField(record, colIdx, "net"), 64)
		rateVal, _ := strconv.ParseFloat(getField(record, colIdx, "rate"), 64)
		changePctVal, _ := strconv.ParseFloat(getField(record, colIdx, "change_pct"), 64)
		superNetVal, _ := strconv.ParseFloat(getField(record, colIdx, "super_net"), 64)
		superRateVal, _ := strconv.ParseFloat(getField(record, colIdx, "super_rate"), 64)
		bigNetVal, _ := strconv.ParseFloat(getField(record, colIdx, "big_net"), 64)
		bigRateVal, _ := strconv.ParseFloat(getField(record, colIdx, "big_rate"), 64)
		volumeVal, _ := strconv.ParseFloat(getField(record, colIdx, "volume"), 64)
		turnoverVal, _ := strconv.ParseFloat(getField(record, colIdx, "turnover"), 64)

		sectors = append(sectors, Sector{
			Name:      getField(record, colIdx, "name"),
			Net:       netVal,
			Rate:      rateVal,
			ChangePct: changePctVal,
			SuperNet:  superNetVal,
			SuperRate: superRateVal,
			BigNet:    bigNetVal,
			BigRate:   bigRateVal,
			Volume:    volumeVal,
			Turnover:  turnoverVal,
			Color:     getField(record, colIdx, "color"),
		})
	}

	return sectors, nil
}

// FetchAllRaw 获取全量板块数据（不经过目标列表过滤）。
func FetchAllRaw() ([]Sector, error) {
	return fetchPrimaryData()
}

func getField(record []string, colIdx map[string]int, field string) string {
	idx, ok := colIdx[field]
	if !ok || idx >= len(record) {
		return ""
	}
	return record[idx]
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func roundTo2(x float64) float64 {
	return math.Round(x*100) / 100
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	case nil:
		return 0, false
	default:
		return 0, false
	}
}
