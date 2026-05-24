package fetcher

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

// MultiDaySectorData 同一板块在多日的资金流向。
type MultiDaySectorData struct {
	Name  string
	Nets  []float64 // 按日期顺序
	Dates []string
	Total float64
	Trend string // "连续流入" / "连续流出" / "加速流入" / "加速流出" / "波动"
}

// BarSnapshot 某一日某一时间点的排名快照。
type BarSnapshot struct {
	Date string
	Time string // "09:30" — tick 时间点，空 = 日度数据
	Bars []BarEntry
}

// BarEntry 条形图单个条目。
type BarEntry struct {
	Name  string
	Net   float64
	Rate  float64
	Rank  int
	Color string
}

// IsTradingDay 判断是否为交易日（跳过周末）。
func IsTradingDay(t time.Time) bool {
	w := t.Weekday()
	return w != time.Saturday && w != time.Sunday
}

// GetTradingDays 获取截至 endDate 的最近 n 个交易日。
func GetTradingDays(endDate string, n int) ([]string, error) {
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("parse date: %w", err)
	}

	var days []string
	current := end
	// 最多往前找 30 天（足够覆盖节假日）
	for i := 0; i < 30 && len(days) < n; i++ {
		if IsTradingDay(current) {
			days = append(days, current.Format("2006-01-02"))
		}
		current = current.AddDate(0, 0, -1)
	}

	// 反转，使其按时间升序
	for i, j := 0, len(days)-1; i < j; i, j = i+1, j-1 {
		days[i], days[j] = days[j], days[i]
	}

	if len(days) < n {
		logger.Warn("交易日不足", zap.Int("found", len(days)), zap.Int("need", n))
	}

	return days, nil
}

// LoadMultiDaySectors 从 SQLite sectors 表加载多日板块数据（datetime = 日期精确匹配）。
func LoadMultiDaySectors(dates []string) (map[string][]Sector, error) {
	result := make(map[string][]Sector)

	db, err := storage.Get()
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	for _, date := range dates {
		dbSectors, err := db.LoadFullSectors(date)
		if err != nil {
			logger.Warn("数据库加载失败", zap.String("date", date), zap.Error(err))
			continue
		}
		if len(dbSectors) == 0 {
			logger.Warn("sectors 表中无该日期记录", zap.String("date", date))
			continue
		}

		sectors := make([]Sector, 0, len(dbSectors))
		for _, s := range dbSectors {
			sectors = append(sectors, Sector{Name: s.Name, Net: s.Net, Rate: s.Rate})
		}
		logger.Info("从 sectors 表加载", zap.String("date", date), zap.Int("sectors", len(sectors)))
		result[date] = sectors
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("sectors 表中没有对应日期的板块数据，请确认已保存当日板块数据")
	}

	return result, nil
}

// LoadFullSectorCSV 从 板块全量_*.csv 加载全量板块数据。
// 格式: 板块名称,主力资金净流入(亿),时间,趋势
func LoadFullSectorCSV(path string) ([]Sector, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("数据为空")
	}

	// 去重：按板块名称保留第一条
	seen := make(map[string]bool)
	var sectors []Sector
	for _, row := range records[1:] {
		if len(row) < 2 {
			continue
		}
		name := row[0]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true

		net, _ := strconv.ParseFloat(row[1], 64)
		if net == 0 {
			continue
		}

		rate := 0.0
		if len(row) > 2 {
			rate, _ = strconv.ParseFloat(row[2], 64)
		}

		sectors = append(sectors, Sector{
			Name: name,
			Net:  roundTo2(net),
			Rate: roundTo2(rate),
		})
	}

	// 按绝对值排序
	sort.Slice(sectors, func(i, j int) bool {
		return absF(sectors[i].Net) > absF(sectors[j].Net)
	})

	return sectors, nil
}

// BuildBarSnapshots 从多日数据构建 Bar Chart Race 快照（累计模式）。
// 每日数据 = 前面所有日期的累计值，形成资金流累积效果。
func BuildBarSnapshots(dayData map[string][]Sector, dates []string) []BarSnapshot {
	var snapshots []BarSnapshot

	// 累计资金流 map
	cumulativeNets := make(map[string]float64)
	// 跟踪每个板块最近一次出现的完整数据（用于获取 rate）
	latestSector := make(map[string]Sector)

	for _, date := range dates {
		sectors, ok := dayData[date]
		if !ok || len(sectors) == 0 {
			continue
		}

		for _, s := range sectors {
			cumulativeNets[s.Name] += s.Net
			latestSector[s.Name] = s
		}

		var allEntries []BarEntry
		for name, net := range cumulativeNets {
			ls := latestSector[name]
			allEntries = append(allEntries, BarEntry{
				Name:  name,
				Net:   roundTo2(net),
				Rate:  ls.Rate,
				Color: ls.Color,
			})
		}

		// 按绝对值排序
		sort.Slice(allEntries, func(i, j int) bool {
			return absF(allEntries[i].Net) > absF(allEntries[j].Net)
		})

		// 限制 TOP18
		if len(allEntries) > 18 {
			allEntries = allEntries[:18]
		}

		// 更新排名
		for i := range allEntries {
			allEntries[i].Rank = i + 1
		}

		topName := allEntries[0].Name
		topNet := allEntries[0].Net
		logger.Debug("快照构建",
			zap.String("date", date),
			zap.Int("totalCumulative", len(cumulativeNets)),
			zap.Int("bars", len(allEntries)),
			zap.String("top", topName),
			zap.Float64("topNet", topNet))

		snapshots = append(snapshots, BarSnapshot{
			Date: date,
			Bars: allEntries,
		})
	}

	logger.Info("快照构建完成",
		zap.Int("snapshots", len(snapshots)),
		zap.Int("uniqueSectors", len(cumulativeNets)))
	return snapshots
}

// LoadMultiDayTicks 从 SQLite sectors 表加载多日逐 tick 板块快照（按时间点分组）。
func LoadMultiDayTicks(dates []string) (map[string][]storage.TimeSnapshot, error) {
	result := make(map[string][]storage.TimeSnapshot)

	db, err := storage.Get()
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	for _, date := range dates {
		snapshots, err := db.LoadDaySnapshots(date)
		if err != nil {
			logger.Warn("数据库加载失败", zap.String("date", date), zap.Error(err))
			continue
		}
		if len(snapshots) == 0 {
			logger.Warn("sectors 表中无该日期 tick 记录", zap.String("date", date))
			continue
		}
		logger.Info("从 sectors 表加载 tick 快照",
			zap.String("date", date),
			zap.Int("snapshots", len(snapshots)))
		result[date] = snapshots
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("sectors 表中没有对应日期的 tick 数据")
	}

	return result, nil
}

// BuildBarSnapshotsFromTicks 从多日逐 tick 数据构建 Bar Chart Race 快照。
// 每日逐 tick 累积：前一日收盘总值 + 当日盘中累积值 = bar 当前值，形成竞争效果。
func BuildBarSnapshotsFromTicks(
	dayTicks map[string][]storage.TimeSnapshot,
	dates []string,
) []BarSnapshot {
	var snapshots []BarSnapshot

	// 已完成日期的累计资金流（前一日收盘总值）
	prevDayFinal := make(map[string]float64)
	// 跟踪每个板块最近一次出现的 rate（当日最新值）
	latestRate := make(map[string]float64)

	for _, date := range dates {
		tickSnapshots, ok := dayTicks[date]
		if !ok || len(tickSnapshots) == 0 {
			continue
		}

		for _, ts := range tickSnapshots {
			timeStr := storage.ExtractTime(ts.Datetime)
			if timeStr == "" {
				continue
			}

			// 更新 rate 缓存
			for _, s := range ts.Sectors {
				latestRate[s.Name] = s.Rate
			}

			// 前日累计 + 当日盘中累积值 = bar 当前值
			var allEntries []BarEntry
			for _, s := range ts.Sectors {
				net := prevDayFinal[s.Name] + s.Net
				allEntries = append(allEntries, BarEntry{
					Name: s.Name,
					Net:  roundTo2(net),
					Rate: s.Rate,
				})
			}

			// 补充 prevDayFinal 中有、但本 snapshot 中丢失的板块（值保持为前日累计）
			for name, net := range prevDayFinal {
				found := false
				for _, s := range ts.Sectors {
					if s.Name == name {
						found = true
						break
					}
				}
				if !found {
					allEntries = append(allEntries, BarEntry{
						Name: name,
						Net:  roundTo2(net),
						Rate: latestRate[name],
					})
				}
			}

			// 按绝对值排序
			sort.Slice(allEntries, func(i, j int) bool {
				return absF(allEntries[i].Net) > absF(allEntries[j].Net)
			})

			// 限制 TOP21
			if len(allEntries) > 21 {
				allEntries = allEntries[:21]
			}

			// 更新排名
			for i := range allEntries {
				allEntries[i].Rank = i + 1
			}

			snapshots = append(snapshots, BarSnapshot{
				Date: date,
				Time: timeStr,
				Bars: allEntries,
			})
		}

		// 该日所有 tick 处理完毕 = 该日收盘
		// 以当日最后一个快照的各板块 net 作为该日累计增量
		lastSnap := tickSnapshots[len(tickSnapshots)-1]
		for _, s := range lastSnap.Sectors {
			prevDayFinal[s.Name] += s.Net
		}

		logger.Debug("收盘累计更新",
			zap.String("date", date),
			zap.Int("tickSnapshots", len(tickSnapshots)),
			zap.Int("cumulativeSectors", len(prevDayFinal)))
	}

	logger.Info("tick 快照构建完成",
		zap.Int("snapshots", len(snapshots)),
		zap.Int("uniqueSectors", len(prevDayFinal)))
	return snapshots
}

// BuildMultiDaySectorData 构建跨日板块数据（用于趋势分析）。
func BuildMultiDaySectorData(dayData map[string][]Sector, dates []string) []MultiDaySectorData {
	// 收集所有板块
	sectorMap := make(map[string][]float64)
	sectorDates := make(map[string][]string)

	for _, date := range dates {
		sectors, ok := dayData[date]
		if !ok {
			continue
		}
		sectorNetMap := make(map[string]float64)
		for _, s := range sectors {
			sectorNetMap[s.Name] = s.Net
		}

		// 确保所有板块都有值（缺失的填 0）
		allNames := make(map[string]bool)
		for name := range sectorMap {
			allNames[name] = true
		}
		for name := range sectorNetMap {
			allNames[name] = true
		}

		for name := range allNames {
			if net, exists := sectorNetMap[name]; exists {
				sectorMap[name] = append(sectorMap[name], net)
				sectorDates[name] = append(sectorDates[name], date)
			} else {
				sectorMap[name] = append(sectorMap[name], 0)
				sectorDates[name] = append(sectorDates[name], date)
			}
		}
	}

	var results []MultiDaySectorData
	for name, nets := range sectorMap {
		if len(nets) != len(dates) {
			continue
		}

		total := 0.0
		for _, n := range nets {
			total += n
		}

		results = append(results, MultiDaySectorData{
			Name:  name,
			Nets:  nets,
			Dates: sectorDates[name],
			Total: roundTo2(total),
			Trend: classifyTrend(nets),
		})
	}

	// 按累计净流入排序
	sort.Slice(results, func(i, j int) bool {
		return absF(results[i].Total) > absF(results[j].Total)
	})

	return results
}

// classifyTrend 判断资金流向趋势。
func classifyTrend(nets []float64) string {
	if len(nets) == 0 {
		return "波动"
	}

	allPositive := true
	allNegative := true
	increasing := true
	decreasing := true

	for i, n := range nets {
		if n <= 0 {
			allPositive = false
		}
		if n >= 0 {
			allNegative = false
		}
		if i > 0 {
			if nets[i] <= nets[i-1] {
				increasing = false
			}
			if nets[i] >= nets[i-1] {
				decreasing = false
			}
		}
	}

	if allPositive && increasing {
		return "加速流入"
	}
	if allPositive {
		return "连续流入"
	}
	if allNegative && decreasing {
		return "加速流出"
	}
	if allNegative {
		return "连续流出"
	}
	return "波动"
}
