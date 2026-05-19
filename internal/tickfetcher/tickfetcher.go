package tickfetcher

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

type TickPoint struct {
	Time string
	Name string
	Net  float64
}

type TickFetcher struct {
	mu              sync.Mutex
	running         bool
	stopCh          chan struct{}
	dateStr         string
	tickCount       int
	errCount        int
	lastTick        string
	intervalMinutes int
}

func New() *TickFetcher {
	cfg := config.LoadTickConfig()
	return &TickFetcher{intervalMinutes: cfg.IntervalMinutes}
}

func (tf *TickFetcher) SetIntervalMinutes(n int) {
	if n < 1 || n > 30 {
		n = 10
	}
	tf.mu.Lock()
	tf.intervalMinutes = n
	tf.mu.Unlock()
	config.SaveTickConfig(config.TickConfig{IntervalMinutes: n})
}

func (tf *TickFetcher) GetIntervalMinutes() int {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	if tf.intervalMinutes <= 0 {
		return 10
	}
	return tf.intervalMinutes
}

func (tf *TickFetcher) Start() error {
	tf.mu.Lock()
	if tf.running {
		tf.mu.Unlock()
		return fmt.Errorf("tick 采集中，请先停止")
	}
	tf.running = true
	tf.stopCh = make(chan struct{})
	tf.dateStr = time.Now().Format("2006-01-02")
	tf.tickCount = 0
	tf.errCount = 0
	tf.mu.Unlock()

	go tf.run()
	return nil
}

func (tf *TickFetcher) Stop() {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	if !tf.running {
		return
	}
	close(tf.stopCh)
	tf.running = false
}

func (tf *TickFetcher) GetStatus() map[string]any {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	interval := tf.intervalMinutes
	if interval <= 0 {
		interval = 10
	}
	return map[string]any{
		"running":         tf.running,
		"date":            tf.dateStr,
		"tickCount":       tf.tickCount,
		"errCount":        tf.errCount,
		"lastTick":        tf.lastTick,
		"intervalMinutes": interval,
	}
}

func (tf *TickFetcher) run() {
	tf.mu.Lock()
	dateStr := tf.dateStr
	stopCh := tf.stopCh
	interval := tf.intervalMinutes
	if interval <= 0 {
		interval = 10
	}
	tf.mu.Unlock()

	currentMinute := nowTradingMinute()
	if currentMinute < 0 {
		tf.mu.Lock()
		tf.running = false
		tf.mu.Unlock()
		return
	}

	allRanges := []tradingRange{{0, 120}, {120, 240}}

	for _, rng := range allRanges {
		if rng.end < currentMinute {
			continue
		}

		startMinute := rng.start
		if currentMinute > rng.start {
			startMinute = ((currentMinute - rng.start) / interval) * interval + rng.start
		}

		for minute := startMinute; minute <= rng.end; minute += interval {
			select {
			case <-stopCh:
				return
			default:
			}

			timeStr := minutesToTime(minute)
			tf.collectTick(dateStr, timeStr, minute)

			if minute < rng.end {
				select {
				case <-stopCh:
					return
				case <-time.After(time.Duration(interval) * time.Minute):
				}
			}
		}
	}

	tf.mu.Lock()
	tf.running = false
	tf.mu.Unlock()
}

func nowTradingMinute() int {
	now := time.Now()
	wallMin := now.Hour()*60 + now.Minute()

	morningStart := 9*60 + 30
	morningEnd := 11*60 + 30
	afternoonStart := 13 * 60
	afternoonEnd := 15 * 60

	if wallMin < morningStart || wallMin > afternoonEnd {
		return -1
	}
	if wallMin <= morningEnd {
		return wallMin - morningStart
	}
	if wallMin < afternoonStart {
		return -1
	}
	return wallMin - morningStart - 90
}

func (tf *TickFetcher) collectTick(dateStr, timeStr string, minute int) {
	l := logger.With(zap.String("date", dateStr), zap.String("time", timeStr))
	l.Info("tick 采集")

	sectors, err := fetcher.FetchTop18HotSectors()
	if err != nil {
		tf.mu.Lock()
		tf.errCount++
		tf.mu.Unlock()
		l.Error("tick 采集失败", zap.Error(err))
		return
	}

	if err := appendTickCSV(dateStr, timeStr, sectors); err != nil {
		tf.mu.Lock()
		tf.errCount++
		tf.mu.Unlock()
		l.Error("tick 保存失败", zap.Error(err))
		return
	}

	tf.mu.Lock()
	tf.tickCount++
	tf.lastTick = timeStr
	tf.mu.Unlock()
	l.Info("tick 已保存", zap.Int("sectors", len(sectors)))
}

func appendTickCSV(dateStr, timeStr string, sectors []fetcher.Sector) error {
	dir := filepath.Join(config.GetDataDir(), dateStr)
	os.MkdirAll(dir, 0755)
	fpath := filepath.Join(dir, "ticks.csv")
	os.MkdirAll(filepath.Dir(fpath), 0755)

	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, _ := f.Stat()
	w := csv.NewWriter(f)
	defer w.Flush()

	if stat.Size() == 0 {
		w.Write([]string{"time", "name", "net"})
	}

	for _, s := range sectors {
		w.Write([]string{
			timeStr,
			s.Name,
			strconv.FormatFloat(s.Net, 'f', 2, 64),
		})
	}

	return nil
}

type tradingRange struct {
	start, end int
}

func minutesToTime(minutes int) string {
	if minutes <= 120 {
		h := 9 + (30+minutes)/60
		m := (30 + minutes) % 60
		return fmt.Sprintf("%02d:%02d", h, m)
	}
	afternoonMin := minutes - 120
	h := 13 + afternoonMin/60
	m := afternoonMin % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func LoadTickCSV(dateStr, session string) ([]TickPoint, error) {
	fpath := filepath.Join(config.GetDataDir(), dateStr, "ticks.csv")

	f, err := os.Open(fpath)
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
		return nil, nil
	}

	var points []TickPoint
	for _, rec := range records[1:] {
		if len(rec) < 3 {
			continue
		}
		timeStr := rec[0]
		name := rec[1]
		net, _ := strconv.ParseFloat(rec[2], 64)

		if session == "morning" && !isMorningTime(timeStr) {
			continue
		}

		points = append(points, TickPoint{
			Time: timeStr,
			Name: name,
			Net:  net,
		})
	}

	return points, nil
}

func isMorningTime(t string) bool {
	return t >= "09:30" && t <= "11:30"
}
