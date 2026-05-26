package tickfetcher

import (
	"fmt"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

// shanghaiTZ is the Asia/Shanghai timezone used for all trading time calculations.
var shanghaiTZ = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// Fallback to UTC+8 if location data is unavailable.
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

// TickSnapshot is the full state pushed to SSE subscribers on each tick.
type TickSnapshot struct {
	Points   []TickPoint `json:"points"`
	Date     string      `json:"date"`
	Running  bool        `json:"running"`
	Count    int         `json:"count"`
	LastTime string      `json:"lastTime"`
}

type TickPoint struct {
	Time string
	Name string
	Net  float64
	Rate float64
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

	// SSE observer: subscribers receive TickSnapshot on each successful tick.
	subsMu      sync.RWMutex
	subscribers map[chan TickSnapshot]bool
}

func New() *TickFetcher {
	cfg := config.LoadTickConfig()
	return &TickFetcher{
		intervalMinutes: cfg.IntervalMinutes,
		subscribers:     make(map[chan TickSnapshot]bool),
	}
}

// Subscribe registers a channel to receive TickSnapshot updates.
// Returns the channel and an unsubscribe function.
func (tf *TickFetcher) Subscribe() (chan TickSnapshot, func()) {
	ch := make(chan TickSnapshot, 1)
	tf.subsMu.Lock()
	tf.subscribers[ch] = true
	tf.subsMu.Unlock()

	return ch, func() {
		tf.subsMu.Lock()
		delete(tf.subscribers, ch)
		close(ch)
		tf.subsMu.Unlock()
	}
}

// GetSnapshot returns the current accumulated tick data.
func (tf *TickFetcher) IsRunning() bool {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	return tf.running
}

func (tf *TickFetcher) GetSnapshot() TickSnapshot {
	tf.mu.Lock()
	dateStr := tf.dateStr
	running := tf.running
	lastTime := tf.lastTick
	tf.mu.Unlock()

	points, _ := LoadTickCSV(dateStr, "full")
	if points == nil {
		points = []TickPoint{}
	}

	timeSet := make(map[string]struct{})
	for _, p := range points {
		timeSet[p.Time] = struct{}{}
	}
	count := len(timeSet)

	tf.mu.Lock()
	if tf.tickCount > count {
		count = tf.tickCount
	}
	tf.mu.Unlock()

	return TickSnapshot{
		Points:   points,
		Date:     dateStr,
		Running:  running,
		Count:    count,
		LastTime: lastTime,
	}
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
	tf.dateStr = time.Now().In(shanghaiTZ).Format("2006-01-02")
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

	allRanges := []tradingRange{{0, 119}, {120, 240}}

	for _, rng := range allRanges {
		if rng.end < currentMinute {
			continue
		}

		startMinute := rng.start
		if currentMinute > rng.start {
			startMinute = ((currentMinute-rng.start)/interval)*interval + rng.start
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
	now := time.Now().In(shanghaiTZ)
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

	datetime := storage.DateToDatetimeTick(dateStr, timeStr)
	db, err := storage.Get()
	if err == nil {
		if exists, _ := db.HasTickData(datetime); exists {
			l.Info("tick 已存在，跳过")
			return
		}
	}

	l.Info("tick 采集")

	sectors, err := fetcher.FetchTop21HotSectors()
	if err != nil {
		tf.mu.Lock()
		tf.errCount++
		tf.mu.Unlock()
		l.Error("tick 采集失败", zap.Error(err))
		return
	}

	if err := saveTickToDB(dateStr, timeStr, sectors); err != nil {
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

	tf.broadcast()
}

func (tf *TickFetcher) broadcast() {
	snapshot := tf.GetSnapshot()

	tf.subsMu.RLock()
	for ch := range tf.subscribers {
		select {
		case ch <- snapshot:
		default:
		}
	}
	tf.subsMu.RUnlock()
}

func saveTickToDB(dateStr, timeStr string, sectors []fetcher.Sector) error {
	db, err := storage.Get()
	if err != nil {
		return err
	}
	inputDate := time.Now().In(shanghaiTZ).Format("2006-01-02 15:04:05")
	records := make([]storage.Sector, 0, len(sectors))
	for _, s := range sectors {
		records = append(records, storage.Sector{
			Datetime:  storage.DateToDatetimeTick(dateStr, timeStr),
			Name:      s.Name,
			Net:       s.Net,
			Rate:      s.Rate,
			InputDate: inputDate,
		})
	}
	return db.SaveSectors(records)
}

type tradingRange struct {
	start, end int
}

// tickSchedule 计算从 currentMinute 开始，以 interval 为步长，
// 在 allRanges 范围内所有待采集的 (tradingMinute, timeStr) 列表。
// 纯数学计算，不依赖时钟或 IO，用于测试验证调度逻辑。
func tickSchedule(currentMinute, interval int, allRanges []tradingRange) []struct {
	Minute int
	Time   string
} {
	var result []struct {
		Minute int
		Time   string
	}
	for _, rng := range allRanges {
		if rng.end < currentMinute {
			continue
		}
		startMinute := rng.start
		if currentMinute > rng.start {
			startMinute = ((currentMinute-rng.start)/interval)*interval + rng.start
		}
		for minute := startMinute; minute <= rng.end; minute += interval {
			result = append(result, struct {
				Minute int
				Time   string
			}{Minute: minute, Time: minutesToTime(minute)})
		}
	}
	return result
}

func minutesToTime(minutes int) string {
	if minutes < 120 {
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
			Time: timeStr,
			Name: s.Name,
			Net:  s.Net,
			Rate: s.Rate,
		})
	}
	return points, nil
}

func isMorningTime(t string) bool {
	return t >= "09:30" && t <= "11:30"
}
