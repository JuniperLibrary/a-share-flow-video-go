package web

import (
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
)

var webShanghaiTZ = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

var defaultWatchlistCache = struct {
	mu    sync.Mutex
	at    time.Time
	items []config.SectorWatchItem
}{}

func registerSectorWatchlistRoutes(r *gin.Engine) {
	r.GET("/api/sectors/catalog", handleSectorCatalog)
	r.GET("/api/sectors/watchlist", handleSectorWatchlist)
	r.POST("/api/sectors/watchlist/set", handleSectorWatchlistSet)
	r.POST("/api/sectors/watchlist/remove", handleSectorWatchlistRemove)
}

func handleSectorCatalog(c *gin.Context) {
	category := c.Query("category")
	refresh := c.Query("refresh") == "1"

	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "初始化数据库失败", err)
		return
	}

	var stored []storage.SectorCatalogItem
	if !refresh {
		stored, _ = db.LoadSectorCatalog(category)
	}

	if refresh || len(stored) == 0 {
		items, err := fetcher.FetchSectorCatalog(category)
		if err != nil {
			logger.BadRequest(c, err.Error())
			return
		}

		now := time.Now().In(webShanghaiTZ).Format("2006-01-02 15:04:05")
		toSave := make([]storage.SectorCatalogItem, 0, len(items))
		for _, it := range items {
			if it.BKCode == "" || it.Name == "" {
				continue
			}
			toSave = append(toSave, storage.SectorCatalogItem{
				BKCode:    it.BKCode,
				Name:      it.Name,
				Category:  it.Category,
				UpdatedAt: now,
			})
		}
		if err := db.UpsertSectorCatalog(toSave); err != nil {
			logger.InternalError(c, "保存板块库失败", err)
			return
		}

		stored, _ = db.LoadSectorCatalog(category)
	}

	sort.Slice(stored, func(i, j int) bool { return stored[i].Name < stored[j].Name })

	resp := make([]fetcher.SectorCatalogItem, 0, len(stored))
	for _, it := range stored {
		resp = append(resp, fetcher.SectorCatalogItem{
			BKCode:   it.BKCode,
			Name:     it.Name,
			Category: it.Category,
		})
	}
	c.JSON(200, gin.H{"items": resp})
}

func handleSectorWatchlist(c *gin.Context) {
	wl := config.LoadSectorWatchlist()
	defaultItems, _ := getDefaultWatchlistItems()
	merged, changed := mergeWatchlistWithDefaults(wl.Items, defaultItems)

	if changed && !isSectorEditForbiddenNow() {
		wl.Items = merged
		_ = config.SaveSectorWatchlist(wl)
	}

	c.JSON(200, gin.H{"items": merged})
}

func handleSectorWatchlistSet(c *gin.Context) {
	if isSectorEditForbiddenNow() {
		logger.BadRequest(c, "开盘时间禁止操作（09:30-15:00）")
		return
	}

	var body struct {
		BKCode   string `json:"bk_code"`
		Name     string `json:"name"`
		Category string `json:"category"`
		Enabled  *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, "请求体需为 JSON")
		return
	}
	if body.BKCode == "" {
		logger.BadRequest(c, "缺少 bk_code")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	wl := config.LoadSectorWatchlist()
	found := false
	for i := range wl.Items {
		if wl.Items[i].BKCode == body.BKCode {
			if body.Name != "" {
				wl.Items[i].Name = body.Name
			}
			if body.Category != "" {
				wl.Items[i].Category = body.Category
			}
			wl.Items[i].Enabled = enabled
			found = true
			break
		}
	}
	if !found {
		wl.Items = append(wl.Items, config.SectorWatchItem{
			BKCode:   body.BKCode,
			Name:     body.Name,
			Category: body.Category,
			Enabled:  enabled,
		})
	}
	if err := config.SaveSectorWatchlist(wl); err != nil {
		logger.InternalError(c, "保存失败", err)
		return
	}
	c.JSON(200, gin.H{"ok": true, "items": wl.Items})
}

func handleSectorWatchlistRemove(c *gin.Context) {
	if isSectorEditForbiddenNow() {
		logger.BadRequest(c, "开盘时间禁止操作（09:30-15:00）")
		return
	}

	var body struct {
		BKCode string `json:"bk_code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, "请求体需为 JSON")
		return
	}
	if body.BKCode == "" {
		logger.BadRequest(c, "缺少 bk_code")
		return
	}

	wl := config.LoadSectorWatchlist()
	next := make([]config.SectorWatchItem, 0, len(wl.Items))
	for _, it := range wl.Items {
		if it.BKCode == body.BKCode {
			continue
		}
		next = append(next, it)
	}
	wl.Items = next
	if err := config.SaveSectorWatchlist(wl); err != nil {
		logger.InternalError(c, "保存失败", err)
		return
	}
	c.JSON(200, gin.H{"ok": true, "items": wl.Items})
}

func isSectorEditForbiddenAt(t time.Time) bool {
	local := t.In(webShanghaiTZ)
	w := local.Weekday()
	if w == time.Saturday || w == time.Sunday {
		return false
	}
	min := local.Hour()*60 + local.Minute()
	start := 9*60 + 30
	end := 15 * 60
	return min >= start && min <= end
}

func isSectorEditForbiddenNow() bool {
	return isSectorEditForbiddenAt(time.Now())
}

func getDefaultWatchlistItems() ([]config.SectorWatchItem, error) {
	defaultWatchlistCache.mu.Lock()
	defer defaultWatchlistCache.mu.Unlock()

	if time.Since(defaultWatchlistCache.at) < 5*time.Minute && len(defaultWatchlistCache.items) > 0 {
		out := make([]config.SectorWatchItem, len(defaultWatchlistCache.items))
		copy(out, defaultWatchlistCache.items)
		return out, nil
	}

	nameIndex := make(map[string]config.SectorWatchItem)
	categories := []string{"concept", "industry", "region"}
	for _, cat := range categories {
		sectors, err := fetcher.FetchSectorsByCategory(cat)
		if err != nil {
			return nil, err
		}
		for _, s := range sectors {
			if s.BKCode == "" || s.Name == "" {
				continue
			}
			if _, ok := nameIndex[s.Name]; ok {
				continue
			}
			nameIndex[s.Name] = config.SectorWatchItem{
				BKCode:   s.BKCode,
				Name:     s.Name,
				Category: s.Category,
				Enabled:  true,
			}
		}
	}

	items := make([]config.SectorWatchItem, 0, len(fetcher.Top21HotSectors))
	for _, name := range fetcher.Top21HotSectors {
		if it, ok := nameIndex[name]; ok {
			items = append(items, it)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].Name < items[j].Name
	})

	defaultWatchlistCache.at = time.Now()
	defaultWatchlistCache.items = items

	out := make([]config.SectorWatchItem, len(items))
	copy(out, items)
	return out, nil
}

func mergeWatchlistWithDefaults(existing []config.SectorWatchItem, defaults []config.SectorWatchItem) ([]config.SectorWatchItem, bool) {
	index := make(map[string]int)
	changed := false
	out := make([]config.SectorWatchItem, 0, len(existing)+len(defaults))
	for _, it := range existing {
		if it.BKCode == "" {
			changed = true
			continue
		}
		if it.Category == "" {
			it.Category = "industry"
			changed = true
		}
		index[it.BKCode] = len(out)
		out = append(out, it)
	}
	for _, d := range defaults {
		if d.BKCode == "" {
			changed = true
			continue
		}
		if d.Category == "" {
			d.Category = "industry"
			changed = true
		}
		if pos, ok := index[d.BKCode]; ok {
			if out[pos].Name == "" {
				out[pos].Name = d.Name
				changed = true
			}
			if out[pos].Category == "" {
				out[pos].Category = d.Category
				changed = true
			}
			continue
		}
		index[d.BKCode] = len(out)
		out = append(out, d)
		changed = true
	}
	return out, changed
}
