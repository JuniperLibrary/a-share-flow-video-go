package clsnews

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// DumpNews 打印新闻摘要到日志。
func DumpNews(news []CLSNews) {
	for _, n := range news {
		logger.Info("📰 财联社电报",
			zap.Int64("id", n.ID),
			zap.String("level", n.Level),
			zap.Time("ctime", n.CTime),
			zap.String("title", n.Title),
			zap.Strings("sectors", n.Sectors),
		)
	}
}

// API 请求参数
const (
	baseURL    = "https://www.cls.cn/v1/roll/get_roll_list"
	appName    = "CailianpressWeb"
	osName     = "web"
	svVersion  = "8.4.6"
	pageSize   = 50
	userAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

func generateSign(params string) string {
	sha1Hash := sha1.Sum([]byte(params))
	sha1Hex := hex.EncodeToString(sha1Hash[:])
	md5Hash := md5.Sum([]byte(sha1Hex))
	return hex.EncodeToString(md5Hash[:])
}

// telegraphResponse 财联社电报列表 API 返回结构。
type telegraphResponse struct {
	ErrNo int `json:"errno"`
	Data  struct {
		RollData []telegraphItem `json:"roll_data"`
	} `json:"data"`
}

// telegraphItem 单条电报原始字段。
type telegraphItem struct {
	ID         json.Number `json:"id"`
	Title      string      `json:"title"`
	Content    string      `json:"content"`
	Brief      string      `json:"brief"`
	CTime      int64       `json:"ctime"` // unix 秒
	Level      string      `json:"level"` // A/B/C
	ReadingNum int64       `json:"reading_num"`
	ShareURL   string      `json:"shareurl"`
}

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
	},
}

// FetchTelegraphList 拉取财联社电报列表。
// lastTime: 上次最新时间戳（unix 秒），用于增量获取；0 = 获取最新。
func FetchTelegraphList(lastTime int64) ([]CLSNews, error) {
	params := fmt.Sprintf("app=%s&os=%s&rn=%d&sv=%s", appName, osName, pageSize, svVersion)
	if lastTime > 0 {
		params = fmt.Sprintf("app=%s&last_time=%d&os=%s&rn=%d&sv=%s",
			appName, lastTime, osName, pageSize, svVersion)
	}
	sign := generateSign(params)
	url := fmt.Sprintf("%s?%s&sign=%s", baseURL, params, sign)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.cls.cn/telegraph")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result telegraphResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.ErrNo != 0 {
		return nil, fmt.Errorf("api error code: %d", result.ErrNo)
	}

	newsList := make([]CLSNews, 0, len(result.Data.RollData))
	for _, item := range result.Data.RollData {
		id, _ := item.ID.Int64()
		if id == 0 {
			continue
		}

		ctime := time.Unix(item.CTime, 0)
		title := strings.TrimSpace(item.Title)
		content := strings.TrimSpace(item.Content)
		if title == "" && content != "" {
			runes := []rune(content)
			if len(runes) > 80 {
				title = string(runes[:80]) + "…"
			} else {
				title = content
			}
		}

		newsList = append(newsList, CLSNews{
			ID:         id,
			Title:      title,
			Content:    content,
			Brief:      strings.TrimSpace(item.Brief),
			Level:      item.Level,
			ReadingNum: item.ReadingNum,
			CTime:      ctime,
			ShareURL:   item.ShareURL,
		})
	}

	return newsList, nil
}

// FetchArticleDetail 获取文章详情（备选方案，财联社 share/article API）。
func FetchArticleDetail(id int64) (*CLSNews, error) {
	url := fmt.Sprintf("https://api3.cls.cn/share/article/%d", id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.cls.cn/telegraph")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Title      string `json:"title"`
		Content    string `json:"content"`
		Author     string `json:"author"`
		CTime      int64  `json:"ctime"`
		ReadingNum int64  `json:"reading_num"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &CLSNews{
		ID:         id,
		Title:      result.Title,
		Content:    result.Content,
		CTime:      time.Unix(result.CTime, 0),
		ReadingNum: result.ReadingNum,
	}, nil
}

// ExtractMaxCTime 从新闻列表中提取最大时间戳（用于增量轮询）。
func ExtractMaxCTime(news []CLSNews) int64 {
	if len(news) == 0 {
		return 0
	}
	sort.Slice(news, func(i, j int) bool {
		return news[i].CTime.After(news[j].CTime)
	})
	return news[0].CTime.Unix()
}

// IsTradingTime 判断当前是否在交易时段（09:00-15:00），用于决定轮询频率。
func IsTradingTime() bool {
	now := time.Now()
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return false
	}
	hour, min, _ := now.Clock()
	totalMin := hour*60 + min
	return totalMin >= 9*60 && totalMin < 15*60
}

func GetPollInterval() time.Duration {
	if IsTradingTime() {
		return 30 * time.Second
	}
	return 5 * time.Minute
}
