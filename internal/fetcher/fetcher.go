package fetcher

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
)

// Sector 表示一个板块的资金流向数据。
type Sector struct {
	Name  string  `json:"name"`
	Net   float64 `json:"net"`
	Color string  `json:"color"` // 运行时由前端/渲染层分配，不持久化到 CSV
}

// Top18HotSectors 当前市场最热门的板块。
var Top18HotSectors = []string{
	"半导体", "AI应用", "CPO概念", "有色金属", "锂矿概念",
	"商业航天", "电池", "机器人", "创新药", "白酒",
	"消费电子", "银行", "人工智能", "云计算", "低空经济",
	"电网设备", "通信设备", "传媒", "国产芯片",
}

type emResponse struct {
	Data struct {
		Diff []map[string]any `json:"diff"`
	} `json:"data"`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

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
		url := fmt.Sprintf("https://emdatah5.eastmoney.com/dc/ZJLX/getZDYLBData?fields=f12,f14,f62&pn=%d&pz=500&fid=f62&po=1&fs=%s&ut=b2884a393a59ad64002292a3e90d46a5", pn, fs)

		req, err := newRequest("GET", url)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		var result emResponse
		if err := decodeJSON(resp.Body, &result); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

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
		sectors = append(sectors, Sector{
			Name: name,
			Net:  roundTo2(netFloat / 1e8),
		})
	}

	fmt.Printf("  [东方财富 H5] 板块获取成功: %d 条\n", len(sectors))
	return sectors, nil
}

// FetchTop18HotSectors 获取 Top18HotSectors 的实时资金流数据。
func FetchTop18HotSectors() ([]Sector, error) {
	all, err := fetchPrimaryData()
	if err != nil {
		return nil, err
	}

	targetSet := make(map[string]bool, len(Top18HotSectors))
	for _, t := range Top18HotSectors {
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

	fmt.Printf("  [fetch] 热门板块: 匹配 %d 个 | 流入 1st=%s %+.1f亿\n",
		len(results), results[0].Name, results[0].Net)

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

	fmt.Printf("  [历史] 获取到 %d 个板块 BK 代码\n", len(mapping))
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
				return &Sector{
					Name: sectorName,
					Net:  roundTo2(netYuan / 1e8),
				}, false
			}
		}
		return nil, false
	}
	return nil, true
}

// FetchHistoricalSectors 获取指定日期的历史板块数据。
func FetchHistoricalSectors(dateStr string) ([]Sector, error) {
	fmt.Printf("  [历史] 正在获取 %s 热门板块历史数据...\n", dateStr)

	bkMapping, err := fetchBKCodes()
	if err != nil || len(bkMapping) == 0 {
		fmt.Println("  [历史] 无法获取板块 BK 代码")
		return nil, nil
	}

	var targets []string
	for _, name := range Top18HotSectors {
		if _, ok := bkMapping[name]; ok {
			targets = append(targets, name)
		}
	}

	if len(targets) == 0 {
		fmt.Println("  [历史] 无匹配的历史板块")
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
				fmt.Printf("  [历史] 连续 %d 次请求失败，IP 可能被临时封锁\n", maxConsecutiveErrors)
				break
			}
		} else {
			consecutiveErrors = 0
		}
	}
	if len(results) == 0 {
		fmt.Printf("  [历史] %s 未找到匹配的交易日数据（可能非交易日）\n", dateStr)
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
	fmt.Printf("  [历史] 热门板块 | 净流入 %d + 净流出 %d = %d 个板块\n", len(inflow), len(outflow), len(result))
	if len(result) > 0 {
		fmt.Printf("  [历史] 流入 1st=%s %+.1f亿\n", result[0].Name, result[0].Net)
		if len(inflow) < len(result) {
			fmt.Printf("  [历史] 流出 1st=%s %+.1f亿\n", result[len(inflow)].Name, result[len(inflow)].Net)
		}
	}
	return result, nil
}

func SaveDailyData(sectors []Sector, dateStr string) error {
	return SaveSessionData(sectors, dateStr, "full")
}

// SaveSessionData 按 session 保存板块数据。
// morning → data/YYYY-MM-DD/sectors_morning.csv
// full    → data/YYYY-MM-DD/sectors.csv
func SaveSessionData(sectors []Sector, dateStr, session string) error {
	dateDir := filepath.Join(config.GetDataDir(), dateStr)
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return err
	}

	filename := "sectors.csv"
	if session == "morning" {
		filename = "sectors_morning.csv"
	}
	filename = filepath.Join(dateDir, filename)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{"name", "net"})
	for _, s := range sectors {
		w.Write([]string{
			s.Name,
			strconv.FormatFloat(s.Net, 'f', 2, 64),
		})
	}

	fmt.Printf("  数据已保存到 %s\n", filename)
	return nil
}

func LoadCachedData(dateStr string) ([]Sector, error) {
	return LoadSessionData(dateStr, "full")
}

// LoadSessionData 按 session 加载板块数据。
func LoadSessionData(dateStr, session string) ([]Sector, error) {
	filename := "sectors.csv"
	if session == "morning" {
		filename = "sectors_morning.csv"
	}
	return loadCSV(filename, dateStr)
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

		sectors = append(sectors, Sector{
			Name:  getField(record, colIdx, "name"),
			Net:   netVal,
			Color: getField(record, colIdx, "color"),
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
