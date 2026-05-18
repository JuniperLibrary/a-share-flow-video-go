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
)

type TickPoint struct {
	Time string
	Name string
	Net  float64
}

type TickFetcher struct {
	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	dateStr   string
	tickCount int
	errCount  int
	lastTick  string
}

func New() *TickFetcher {
	return &TickFetcher{}
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
	return map[string]any{
		"running":   tf.running,
		"date":      tf.dateStr,
		"tickCount": tf.tickCount,
		"errCount":  tf.errCount,
		"lastTick":  tf.lastTick,
	}
}

func (tf *TickFetcher) run() {
	tf.mu.Lock()
	dateStr := tf.dateStr
	stopCh := tf.stopCh
	tf.mu.Unlock()

	allRanges := []tradingRange{{0, 120}, {120, 240}}
	currentMinute := nowTradingMinute()

	fmt.Printf("  [tick] 采集启动 | 当前交易分钟=%d | 交易时段: 09:30-11:30, 13:00-15:00\n", currentMinute)

	for _, rng := range allRanges {
		if currentMinute >= 0 && rng.end < currentMinute {
			continue
		}

		startMinute := rng.start
		if currentMinute >= 0 && currentMinute > rng.start {
			startMinute = ((currentMinute - rng.start) / 10) * 10 + rng.start
		}

		for minute := startMinute; minute <= rng.end; minute += 10 {
			select {
			case <-stopCh:
				fmt.Println("  [tick] 采集已停止")
				return
			default:
			}

			timeStr := minutesToTime(minute)
			tf.collectTick(dateStr, timeStr, minute)

			if minute < rng.end {
				select {
				case <-stopCh:
					fmt.Println("  [tick] 采集已停止")
					return
				case <-time.After(10 * time.Minute):
				}
			}
		}
	}

	tf.mu.Lock()
	tf.running = false
	tf.mu.Unlock()
	fmt.Printf("  [tick] 采集完成 | 共 %d 个点\n", tf.tickCount)
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
	fmt.Printf("  [tick] 采集 %s\n", timeStr)

	sectors, err := fetcher.FetchTop18HotSectors()
	if err != nil {
		tf.mu.Lock()
		tf.errCount++
		tf.mu.Unlock()
		fmt.Printf("  [tick] 采集失败 %s: %v\n", timeStr, err)
		return
	}

	if err := appendTickCSV(dateStr, timeStr, sectors); err != nil {
		tf.mu.Lock()
		tf.errCount++
		tf.mu.Unlock()
		fmt.Printf("  [tick] 保存失败 %s: %v\n", timeStr, err)
		return
	}

	tf.mu.Lock()
	tf.tickCount++
	tf.lastTick = timeStr
	tf.mu.Unlock()
	fmt.Printf("  [tick] %s 已保存 | %d 个板块\n", timeStr, len(sectors))
}

func appendTickCSV(dateStr, timeStr string, sectors []fetcher.Sector) error {
	fpath := filepath.Join(config.GetDataDir(), dateStr, "ticks.csv")

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
