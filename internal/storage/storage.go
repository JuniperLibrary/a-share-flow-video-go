package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

type DB struct {
	mu sync.RWMutex
	db *sql.DB
}

type Sector struct {
	Datetime             string  `json:"datetime"` // "2026-05-19 09:30" (tick) or "2026-05-19" (full)
	Name                 string  `json:"name"`
	Net                  float64 `json:"net"`                    // 主力净流入（亿）
	Rate                 float64 `json:"rate"`                   // 主力净占比（%）
	ChangePct            float64 `json:"change_pct"`             // 涨跌幅（%）
	SuperNet             float64 `json:"super_net"`              // 超大单净流入（亿）
	SuperRate            float64 `json:"super_rate"`             // 超大单净占比（%）
	BigNet               float64 `json:"big_net"`                // 大单净流入（亿）
	BigRate              float64 `json:"big_rate"`               // 大单净占比（%）
	Volume               float64 `json:"volume"`                 // 成交量（手，tick 数据用）
	Turnover             float64 `json:"turnover"`               // 成交额（亿，tick 数据用）
	BKCode               string  `json:"bk_code"`                // 板块代码 BKxxxx
	TurnoverRate         float64 `json:"turnover_rate"`          // 换手率（%）
	LeadStockName        string  `json:"lead_stock_name"`        // 领涨股名称
	LeadStockChangePct   float64 `json:"lead_stock_change_pct"`  // 领涨股涨跌幅（%）
	TotalMarketCap       float64 `json:"total_market_cap"`       // 总市值（亿）
	CirculatingMarketCap float64 `json:"circulating_market_cap"` // 流通市值（亿）
	InputDate            string  `json:"input_date"`             // 录入时间 "2026-05-22 13:14:19"
}

type SectorAll struct {
	Date                 string  `json:"date"`                   // "2026-05-19"
	Code                 string  `json:"code"`                   // 板块代码 BKxxxx
	Name                 string  `json:"name"`                   // 板块名称
	Net                  float64 `json:"net"`                    // 主力资金净流入（亿）
	Rate                 float64 `json:"rate"`                   // 主力净占比（%）
	ChangePct            float64 `json:"change_pct"`             // 涨跌幅（%）
	SuperNet             float64 `json:"super_net"`              // 超大单净流入（亿）
	SuperRate            float64 `json:"super_rate"`             // 超大单净占比（%）
	BigNet               float64 `json:"big_net"`                // 大单净流入（亿）
	BigRate              float64 `json:"big_rate"`               // 大单净占比（%）
	Volume               float64 `json:"volume"`                 // 成交量（手）
	Turnover             float64 `json:"turnover"`               // 成交额（亿）
	TurnoverRate         float64 `json:"turnover_rate"`          // 换手率（%）
	LeadStockName        string  `json:"lead_stock_name"`        // 领涨股名称
	LeadStockChangePct   float64 `json:"lead_stock_change_pct"`  // 领涨股涨跌幅（%）
	TotalMarketCap       float64 `json:"total_market_cap"`       // 总市值（亿）
	CirculatingMarketCap float64 `json:"circulating_market_cap"` // 流通市值（亿）
	Category             string  `json:"category"`               // "industry" 行业板块 / "concept" 概念板块
}

type Copywriting struct {
	Date    string `json:"date"`
	Session string `json:"session"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type Note struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`    // "completed" 已完成 / "planned" 待计划
	Content   string `json:"content"` // 笔记内容
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// DebateHistory 财报辩论历史记录。
type DebateHistory struct {
	TaskID        string `json:"task_id"`
	StockCode     string `json:"stock_code"`
	StockName     string `json:"stock_name"`
	ReportSummary string `json:"report_summary"`
	TurnCount     int    `json:"turn_count"`
	Format        string `json:"format"`
	VideoPath     string `json:"video_path"`
	CreatedAt     string `json:"created_at"`
}

func New(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	d, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	if _, err := d.Exec("PRAGMA journal_mode=WAL"); err != nil {
		d.Close()
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	db := &DB{db: d}
	if err := db.initSchema(); err != nil {
		d.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.db.Close()
}

func (db *DB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sectors (
		datetime                TEXT    NOT NULL,  -- 时间 "2026-05-19 09:30" (tick) 或 "2026-05-19" (全量)
		name                    TEXT    NOT NULL,  -- 板块名称
		net                     REAL    NOT NULL,  -- 主力资金净流入（亿）
		rate                    REAL    NOT NULL DEFAULT 0,  -- 主力净占比（%）
		change_pct              REAL    NOT NULL DEFAULT 0,  -- 涨跌幅（%）
		super_net               REAL    NOT NULL DEFAULT 0,  -- 超大单净流入（亿）
		super_rate              REAL    NOT NULL DEFAULT 0,  -- 超大单净占比（%）
		big_net                 REAL    NOT NULL DEFAULT 0,  -- 大单净流入（亿）
		big_rate                REAL    NOT NULL DEFAULT 0,  -- 大单净占比（%）
		volume                  REAL    NOT NULL DEFAULT 0,  -- 成交量（手，tick 数据用）
		turnover                REAL    NOT NULL DEFAULT 0,  -- 成交额（亿，tick 数据用）
		bk_code                 TEXT    NOT NULL DEFAULT '',  -- 板块代码 BKxxxx
		turnover_rate           REAL    NOT NULL DEFAULT 0,  -- 换手率（%）
		lead_stock_name         TEXT    NOT NULL DEFAULT '',  -- 领涨股名称
		lead_stock_change_pct   REAL    NOT NULL DEFAULT 0,  -- 领涨股涨跌幅（%）
		total_market_cap        REAL    NOT NULL DEFAULT 0,  -- 总市值（亿）
		circulating_market_cap  REAL    NOT NULL DEFAULT 0,  -- 流通市值（亿）
		input_date              TEXT    NOT NULL DEFAULT '',  -- 录入时间 "2026-05-22 13:14:19"
		PRIMARY KEY (datetime, name)
	);

	CREATE TABLE IF NOT EXISTS copywriting (
		date    TEXT    NOT NULL,  -- 日期 "2026-05-19"
		session TEXT    NOT NULL,  -- 时段 "full" / "morning"
		type    TEXT    NOT NULL,  -- 类型 "template" / "ai" / "template_tick" / "ai_tick"
		content TEXT    NOT NULL,  -- 文案内容
		PRIMARY KEY (date, session, type)
	);

	CREATE TABLE IF NOT EXISTS sectors_all (
		date                    TEXT    NOT NULL,  -- 日期 "2026-05-19"
		code                    TEXT    NOT NULL,  -- 板块代码 BKxxxx
		name                    TEXT    NOT NULL,  -- 板块名称
		net                     REAL    NOT NULL,  -- 主力资金净流入（亿）
		rate                    REAL    NOT NULL DEFAULT 0,  -- 主力净占比（%）
		change_pct              REAL    NOT NULL DEFAULT 0,  -- 涨跌幅（%）
		super_net               REAL    NOT NULL DEFAULT 0,  -- 超大单净流入（亿）
		super_rate              REAL    NOT NULL DEFAULT 0,  -- 超大单净占比（%）
		big_net                 REAL    NOT NULL DEFAULT 0,  -- 大单净流入（亿）
		big_rate                REAL    NOT NULL DEFAULT 0,  -- 大单净占比（%）
		volume                  REAL    NOT NULL DEFAULT 0,  -- 成交量（手）
		turnover                REAL    NOT NULL DEFAULT 0,  -- 成交额（亿）
		turnover_rate           REAL    NOT NULL DEFAULT 0,  -- 换手率（%）
		lead_stock_name         TEXT    NOT NULL DEFAULT '',  -- 领涨股名称
		lead_stock_change_pct   REAL    NOT NULL DEFAULT 0,  -- 领涨股涨跌幅（%）
		total_market_cap        REAL    NOT NULL DEFAULT 0,  -- 总市值（亿）
		circulating_market_cap  REAL    NOT NULL DEFAULT 0,  -- 流通市值（亿）
		category                TEXT    NOT NULL DEFAULT '',  -- "industry" 行业 / "concept" 概念
		PRIMARY KEY (date, name)
	);

	CREATE TABLE IF NOT EXISTS tick_events (
		date    TEXT    NOT NULL,  -- 日期 "2026-05-19"
		session TEXT    NOT NULL,  -- 时段 "full" / "morning"
		type    TEXT    NOT NULL,  -- 类型 "timeline" / "market" / "ticker"
		payload TEXT    NOT NULL,  -- JSON 数据
		PRIMARY KEY (date, session, type)
	);

	CREATE TABLE IF NOT EXISTS cls_news (
		id          BIGINT PRIMARY KEY,
		title       TEXT    NOT NULL,
		content     TEXT    NOT NULL DEFAULT '',
		brief       TEXT    NOT NULL DEFAULT '',
		level       TEXT    NOT NULL DEFAULT 'C',
		reading_num BIGINT  NOT NULL DEFAULT 0,
		ctime       DATETIME NOT NULL,
		shareurl    TEXT    NOT NULL DEFAULT '',
		sectors     TEXT    NOT NULL DEFAULT '',
		created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_sectors_date ON sectors(datetime);
	CREATE INDEX IF NOT EXISTS idx_copywriting_date ON copywriting(date);
	CREATE INDEX IF NOT EXISTS idx_sectors_all_date ON sectors_all(date);
	CREATE INDEX IF NOT EXISTS idx_tick_events_date ON tick_events(date);
	CREATE INDEX IF NOT EXISTS idx_cls_news_ctime ON cls_news(ctime);

	CREATE TABLE IF NOT EXISTS notes (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		type       TEXT    NOT NULL DEFAULT 'planned',
		content    TEXT    NOT NULL,
		created_at TEXT    NOT NULL DEFAULT (datetime('now','localtime')),
		updated_at TEXT    NOT NULL DEFAULT (datetime('now','localtime'))
	);

	CREATE TABLE IF NOT EXISTS daily_reports (
		date        TEXT    PRIMARY KEY,  -- "2026-06-09"
		session     TEXT    NOT NULL DEFAULT 'full',
		summary     TEXT    NOT NULL DEFAULT '',
		outlook     TEXT    NOT NULL DEFAULT '',
		report_json TEXT    NOT NULL DEFAULT '{}',  -- 完整日报 JSON
		created_at  TEXT    NOT NULL DEFAULT (datetime('now','localtime'))
	);

	CREATE TABLE IF NOT EXISTS debate_history (
		task_id        TEXT PRIMARY KEY,
		stock_code     TEXT NOT NULL DEFAULT '',
		stock_name     TEXT NOT NULL DEFAULT '',
		report_summary TEXT NOT NULL DEFAULT '',
		turn_count     INTEGER NOT NULL DEFAULT 0,
		format         TEXT NOT NULL DEFAULT 'mobile',
		video_path     TEXT NOT NULL DEFAULT '',
		created_at     TEXT NOT NULL DEFAULT (datetime('now','localtime'))
	);
	`
	if _, err := db.db.Exec(schema); err != nil {
		return err
	}

	// Migrations for existing databases (idempotent — ADD COLUMN with default)
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN input_date TEXT NOT NULL DEFAULT ''")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN rate REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN change_pct REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN super_net REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN super_rate REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN big_net REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN big_rate REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN volume REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN turnover REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN bk_code TEXT NOT NULL DEFAULT ''")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN turnover_rate REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN lead_stock_name TEXT NOT NULL DEFAULT ''")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN lead_stock_change_pct REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN total_market_cap REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN circulating_market_cap REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN rate REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN category TEXT NOT NULL DEFAULT ''")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN change_pct REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN super_net REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN super_rate REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN big_net REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN big_rate REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN volume REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN turnover REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN turnover_rate REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN lead_stock_name TEXT NOT NULL DEFAULT ''")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN lead_stock_change_pct REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN total_market_cap REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN circulating_market_cap REAL NOT NULL DEFAULT 0")

	return nil
}

func (db *DB) SaveSectors(sectors []Sector) error {
	if len(sectors) == 0 {
		return nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO sectors (datetime, name, net, rate, change_pct, super_net, super_rate, big_net, big_rate, volume, turnover, bk_code, turnover_rate, lead_stock_name, lead_stock_change_pct, total_market_cap, circulating_market_cap, input_date) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range sectors {
		if _, err := stmt.Exec(s.Datetime, s.Name, s.Net, s.Rate, s.ChangePct, s.SuperNet, s.SuperRate, s.BigNet, s.BigRate, s.Volume, s.Turnover, s.BKCode, s.TurnoverRate, s.LeadStockName, s.LeadStockChangePct, s.TotalMarketCap, s.CirculatingMarketCap, s.InputDate); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) LoadFullSectors(date string) ([]Sector, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	prefix := date + "%"
	rows, err := db.db.Query(`
		SELECT s.datetime, s.name, s.net, s.rate, s.change_pct, s.super_net, s.super_rate, s.big_net, s.big_rate, s.volume, s.turnover, s.input_date
		FROM sectors s
		INNER JOIN (
			SELECT name, MAX(datetime) AS max_dt
			FROM sectors
			WHERE datetime LIKE ?
			GROUP BY name
		) latest ON s.name = latest.name AND s.datetime = latest.max_dt
		ORDER BY ABS(s.net) DESC
	`, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []Sector
	for rows.Next() {
		var s Sector
		if err := rows.Scan(&s.Datetime, &s.Name, &s.Net, &s.Rate, &s.ChangePct, &s.SuperNet, &s.SuperRate, &s.BigNet, &s.BigRate, &s.Volume, &s.Turnover, &s.BKCode, &s.TurnoverRate, &s.LeadStockName, &s.LeadStockChangePct, &s.TotalMarketCap, &s.CirculatingMarketCap, &s.InputDate); err != nil {
			return nil, err
		}
		sectors = append(sectors, s)
	}
	return sectors, rows.Err()
}

// TimeSnapshot 一个时间点的所有板块数据快照。
type TimeSnapshot struct {
	Datetime string // "2026-05-20 09:30"
	Sectors  []Sector
}

// LoadDaySnapshots 加载指定日期所有 tick 时间点的板块快照（按时间升序）。
func (db *DB) LoadDaySnapshots(date string) ([]TimeSnapshot, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	prefix := date + "%"
	rows, err := db.db.Query(`
		SELECT datetime, name, net, rate, change_pct, super_net, super_rate, big_net, big_rate, volume, turnover, bk_code, turnover_rate, lead_stock_name, lead_stock_change_pct, total_market_cap, circulating_market_cap, input_date
		FROM sectors
		WHERE datetime LIKE ?
		ORDER BY datetime ASC
	`, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []TimeSnapshot
	var currentDT string
	var currentSectors []Sector

	for rows.Next() {
		var s Sector
		if err := rows.Scan(&s.Datetime, &s.Name, &s.Net, &s.Rate, &s.ChangePct, &s.SuperNet, &s.SuperRate, &s.BigNet, &s.BigRate, &s.Volume, &s.Turnover, &s.BKCode, &s.TurnoverRate, &s.LeadStockName, &s.LeadStockChangePct, &s.TotalMarketCap, &s.CirculatingMarketCap, &s.InputDate); err != nil {
			return nil, err
		}
		if s.Datetime != currentDT {
			if currentDT != "" {
				snapshots = append(snapshots, TimeSnapshot{Datetime: currentDT, Sectors: currentSectors})
			}
			currentDT = s.Datetime
			currentSectors = nil
		}
		currentSectors = append(currentSectors, s)
	}
	if currentDT != "" {
		snapshots = append(snapshots, TimeSnapshot{Datetime: currentDT, Sectors: currentSectors})
	}

	return snapshots, rows.Err()
}

func (db *DB) LoadTickSectors(date string) ([]Sector, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT datetime, name, net, rate, change_pct, super_net, super_rate, big_net, big_rate, volume, turnover, bk_code, turnover_rate, lead_stock_name, lead_stock_change_pct, total_market_cap, circulating_market_cap, input_date FROM sectors WHERE datetime LIKE ? AND datetime != ? ORDER BY datetime, ABS(net) DESC", date+" %", date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []Sector
	for rows.Next() {
		var s Sector
		if err := rows.Scan(&s.Datetime, &s.Name, &s.Net, &s.Rate, &s.ChangePct, &s.SuperNet, &s.SuperRate, &s.BigNet, &s.BigRate, &s.Volume, &s.Turnover, &s.BKCode, &s.TurnoverRate, &s.LeadStockName, &s.LeadStockChangePct, &s.TotalMarketCap, &s.CirculatingMarketCap, &s.InputDate); err != nil {
			return nil, err
		}
		sectors = append(sectors, s)
	}
	return sectors, rows.Err()
}

func (db *DB) HasTickData(datetime string) (bool, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var count int
	err := db.db.QueryRow("SELECT COUNT(*) FROM sectors WHERE datetime = ?", datetime).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (db *DB) DeleteSectorsByDate(date string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.Exec("DELETE FROM sectors WHERE datetime = ? OR datetime LIKE ?", date, date+" %")
	return err
}

func (db *DB) SaveSectorsAll(sectors []SectorAll) error {
	if len(sectors) == 0 {
		return nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO sectors_all (date, code, name, net, rate, change_pct, super_net, super_rate, big_net, big_rate, volume, turnover, turnover_rate, lead_stock_name, lead_stock_change_pct, total_market_cap, circulating_market_cap, category) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range sectors {
		if _, err := stmt.Exec(s.Date, s.Code, s.Name, s.Net, s.Rate, s.ChangePct, s.SuperNet, s.SuperRate, s.BigNet, s.BigRate, s.Volume, s.Turnover, s.TurnoverRate, s.LeadStockName, s.LeadStockChangePct, s.TotalMarketCap, s.CirculatingMarketCap, s.Category); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) LoadSectorsAll(date string) ([]SectorAll, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT date, code, name, net, rate, change_pct, super_net, super_rate, big_net, big_rate, volume, turnover, turnover_rate, lead_stock_name, lead_stock_change_pct, total_market_cap, circulating_market_cap, category FROM sectors_all WHERE date = ? ORDER BY ABS(net) DESC", date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []SectorAll
	for rows.Next() {
		var s SectorAll
		if err := rows.Scan(&s.Date, &s.Code, &s.Name, &s.Net, &s.Rate, &s.ChangePct, &s.SuperNet, &s.SuperRate, &s.BigNet, &s.BigRate, &s.Volume, &s.Turnover, &s.TurnoverRate, &s.LeadStockName, &s.LeadStockChangePct, &s.TotalMarketCap, &s.CirculatingMarketCap, &s.Category); err != nil {
			return nil, err
		}
		sectors = append(sectors, s)
	}
	return sectors, rows.Err()
}

func (db *DB) ListSectorsAllDates() ([]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT DISTINCT date FROM sectors_all ORDER BY date DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

func (db *DB) LoadSectorsAllRange(startDate, endDate string) ([]SectorAll, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT date, code, name, net, rate, change_pct, super_net, super_rate, big_net, big_rate, volume, turnover, turnover_rate, lead_stock_name, lead_stock_change_pct, total_market_cap, circulating_market_cap, category FROM sectors_all WHERE date >= ? AND date <= ? ORDER BY date, ABS(net) DESC", startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []SectorAll
	for rows.Next() {
		var s SectorAll
		if err := rows.Scan(&s.Date, &s.Code, &s.Name, &s.Net, &s.Rate, &s.ChangePct, &s.SuperNet, &s.SuperRate, &s.BigNet, &s.BigRate, &s.Volume, &s.Turnover, &s.TurnoverRate, &s.LeadStockName, &s.LeadStockChangePct, &s.TotalMarketCap, &s.CirculatingMarketCap, &s.Category); err != nil {
			return nil, err
		}
		sectors = append(sectors, s)
	}
	return sectors, rows.Err()
}

func (db *DB) LoadSectorTrend(name, startDate, endDate string) ([]SectorAll, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT date, code, name, net, rate, change_pct, super_net, super_rate, big_net, big_rate, volume, turnover, turnover_rate, lead_stock_name, lead_stock_change_pct, total_market_cap, circulating_market_cap, category FROM sectors_all WHERE name = ? AND date >= ? AND date <= ? ORDER BY date", name, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []SectorAll
	for rows.Next() {
		var s SectorAll
		if err := rows.Scan(&s.Date, &s.Code, &s.Name, &s.Net, &s.Rate, &s.ChangePct, &s.SuperNet, &s.SuperRate, &s.BigNet, &s.BigRate, &s.Volume, &s.Turnover, &s.TurnoverRate, &s.LeadStockName, &s.LeadStockChangePct, &s.TotalMarketCap, &s.CirculatingMarketCap, &s.Category); err != nil {
			return nil, err
		}
		sectors = append(sectors, s)
	}
	return sectors, rows.Err()
}

func (db *DB) ListSectorsAllNames() ([]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT DISTINCT name FROM sectors_all ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

func (db *DB) SaveCopywriting(cw Copywriting) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.Exec(`INSERT OR REPLACE INTO copywriting (date, session, type, content) VALUES (?, ?, ?, ?)`,
		cw.Date, cw.Session, cw.Type, cw.Content)
	return err
}

func (db *DB) LoadCopywriting(date string) ([]Copywriting, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT date, session, type, content FROM copywriting WHERE date = ?", date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Copywriting
	for rows.Next() {
		var cw Copywriting
		if err := rows.Scan(&cw.Date, &cw.Session, &cw.Type, &cw.Content); err != nil {
			return nil, err
		}
		result = append(result, cw)
	}
	return result, rows.Err()
}

func (db *DB) LoadCopywritingBySession(date, session string) (map[string]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT type, content FROM copywriting WHERE date = ? AND session = ?", date, session)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var typ, content string
		if err := rows.Scan(&typ, &content); err != nil {
			return nil, err
		}
		result[typ] = content
	}
	return result, rows.Err()
}

func (db *DB) DeleteCopywriting(date, session string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if session == "" {
		_, err := db.db.Exec("DELETE FROM copywriting WHERE date = ?", date)
		return err
	}
	_, err := db.db.Exec("DELETE FROM copywriting WHERE date = ? AND session = ?", date, session)
	return err
}

func (db *DB) ListTickDates() ([]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT DISTINCT SUBSTR(datetime, 1, 10) FROM sectors WHERE datetime LIKE '____-__-__ %'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

// CLSNewsRecord 财联社新闻数据库记录。
type CLSNewsRecord struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Brief      string `json:"brief"`
	Level      string `json:"level"`
	ReadingNum int64  `json:"reading_num"`
	CTime      string `json:"ctime"` // "2026-05-24 16:34:00"
	ShareURL   string `json:"shareurl"`
	Sectors    string `json:"sectors"` // JSON 数组字符串
	CreatedAt  string `json:"created_at"`
}

// SaveCLSNews 批量保存财联社新闻（INSERT OR IGNORE 按 id 去重）。
func (db *DB) SaveCLSNews(records []CLSNewsRecord) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO cls_news (id, title, content, brief, level, reading_num, ctime, shareurl, sectors) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	saved := 0
	for _, r := range records {
		result, err := stmt.Exec(r.ID, r.Title, r.Content, r.Brief, r.Level, r.ReadingNum, r.CTime, r.ShareURL, r.Sectors)
		if err != nil {
			return saved, err
		}
		if n, _ := result.RowsAffected(); n > 0 {
			saved++
		}
	}

	return saved, tx.Commit()
}

// LoadLatestNews 加载最新新闻（分页，按 ctime 降序）。
func (db *DB) LoadLatestNews(limit, offset int) ([]CLSNewsRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query(`SELECT id, title, content, brief, level, reading_num, ctime, shareurl, sectors, created_at FROM cls_news ORDER BY ctime DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CLSNewsRecord
	for rows.Next() {
		var r CLSNewsRecord
		if err := rows.Scan(&r.ID, &r.Title, &r.Content, &r.Brief, &r.Level, &r.ReadingNum, &r.CTime, &r.ShareURL, &r.Sectors, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// LoadNewsByDate 按日期加载新闻（按 ctime 降序）。
func (db *DB) LoadNewsByDate(date string, limit, offset int) ([]CLSNewsRecord, int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	prefix := date + "%"

	var total int
	err := db.db.QueryRow("SELECT COUNT(*) FROM cls_news WHERE ctime LIKE ?", prefix).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.db.Query(`SELECT id, title, content, brief, level, reading_num, ctime, shareurl, sectors, created_at FROM cls_news WHERE ctime LIKE ? ORDER BY ctime DESC LIMIT ? OFFSET ?`, prefix, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []CLSNewsRecord
	for rows.Next() {
		var r CLSNewsRecord
		if err := rows.Scan(&r.ID, &r.Title, &r.Content, &r.Brief, &r.Level, &r.ReadingNum, &r.CTime, &r.ShareURL, &r.Sectors, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		records = append(records, r)
	}
	return records, total, rows.Err()
}

// SearchCLSNews 搜索新闻（按标题或正文模糊匹配）。
func (db *DB) SearchCLSNews(keyword string, limit, offset int) ([]CLSNewsRecord, int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	like := "%" + keyword + "%"

	var total int
	err := db.db.QueryRow("SELECT COUNT(*) FROM cls_news WHERE title LIKE ? OR content LIKE ?", like, like).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.db.Query(`SELECT id, title, content, brief, level, reading_num, ctime, shareurl, sectors, created_at FROM cls_news WHERE title LIKE ? OR content LIKE ? ORDER BY ctime DESC LIMIT ? OFFSET ?`, like, like, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []CLSNewsRecord
	for rows.Next() {
		var r CLSNewsRecord
		if err := rows.Scan(&r.ID, &r.Title, &r.Content, &r.Brief, &r.Level, &r.ReadingNum, &r.CTime, &r.ShareURL, &r.Sectors, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		records = append(records, r)
	}
	return records, total, rows.Err()
}

// GetCLSNewsCount 返回新闻总数。
func (db *DB) GetCLSNewsCount() (int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var count int
	err := db.db.QueryRow("SELECT COUNT(*) FROM cls_news").Scan(&count)
	return count, err
}

func DateToDatetime(date string) string {
	return date
}

func DateToDatetimeTick(date, time string) string {
	return date + " " + time
}

func ExtractDate(datetime string) string {
	if idx := strings.Index(datetime, " "); idx > 0 {
		return datetime[:idx]
	}
	return datetime
}

func ExtractTime(datetime string) string {
	if idx := strings.Index(datetime, " "); idx > 0 && idx+1 < len(datetime) {
		return datetime[idx+1:]
	}
	return ""
}

func (db *DB) SaveTickEvents(date, session string, payload json.RawMessage) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.Exec(`INSERT OR REPLACE INTO tick_events (date, session, type, payload) VALUES (?, ?, 'combined', ?)`,
		date, session, string(payload))
	return err
}

func (db *DB) LoadTickEvents(date, session string) (json.RawMessage, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var payloadStr string
	err := db.db.QueryRow("SELECT payload FROM tick_events WHERE date = ? AND session = ? AND type = 'combined'",
		date, session).Scan(&payloadStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payloadStr), nil
}

// RawRows executes a query and returns all rows as []map[string]any.
func (db *DB) RawRows(query string, args ...any) ([]map[string]any, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		valPtrs := make([]any, len(cols))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}

		if err := rows.Scan(valPtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any)
		for i, col := range cols {
			val := vals[i]
			switch v := val.(type) {
			case []byte:
				row[col] = string(v)
			default:
				row[col] = v
			}
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// RawExec executes a statement without returning rows.
func (db *DB) RawExec(query string, args ...any) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.Exec(query, args...)
	return err
}

func (db *DB) SaveDebateHistory(h DebateHistory) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.Exec(`INSERT OR REPLACE INTO debate_history (task_id, stock_code, stock_name, report_summary, turn_count, format, video_path, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, COALESCE(?, datetime('now','localtime')))`,
		h.TaskID, h.StockCode, h.StockName, h.ReportSummary, h.TurnCount, h.Format, h.VideoPath, h.CreatedAt)
	return err
}

func (db *DB) ListDebateHistory(limit, offset int) ([]DebateHistory, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT task_id, stock_code, stock_name, report_summary, turn_count, format, video_path, created_at FROM debate_history ORDER BY created_at DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []DebateHistory
	for rows.Next() {
		var h DebateHistory
		if err := rows.Scan(&h.TaskID, &h.StockCode, &h.StockName, &h.ReportSummary, &h.TurnCount, &h.Format, &h.VideoPath, &h.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

func (db *DB) GetDebateHistory(taskID string) (*DebateHistory, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	row := db.db.QueryRow("SELECT task_id, stock_code, stock_name, report_summary, turn_count, format, video_path, created_at FROM debate_history WHERE task_id = ?", taskID)
	var h DebateHistory
	if err := row.Scan(&h.TaskID, &h.StockCode, &h.StockName, &h.ReportSummary, &h.TurnCount, &h.Format, &h.VideoPath, &h.CreatedAt); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return &h, nil
}

func (db *DB) DeleteDebateHistory(taskID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.Exec("DELETE FROM debate_history WHERE task_id = ?", taskID)
	return err
}

// SaveDailyReport 保存日报（INSERT OR REPLACE）。
func (db *DB) SaveDailyReport(date, session, summary, outlook, reportJSON string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.Exec(
		`INSERT OR REPLACE INTO daily_reports (date, session, summary, outlook, report_json) VALUES (?, ?, ?, ?, ?)`,
		date, session, summary, outlook, reportJSON)
	return err
}

// LoadDailyReport 加载指定日期的日报。
// 返回 (reportJSON, session, summary, outlook, error)。
func (db *DB) LoadDailyReport(date string) (reportJSON, session, summary, outlook string, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	row := db.db.QueryRow("SELECT report_json, session, summary, outlook FROM daily_reports WHERE date = ?", date)
	if e := row.Scan(&reportJSON, &session, &summary, &outlook); e == sql.ErrNoRows {
		return "", "", "", "", nil
	} else if e != nil {
		return "", "", "", "", e
	}
	return reportJSON, session, summary, outlook, nil
}

// ListDailyReportDates 列出所有已有日报的日期（按日期降序）。
func (db *DB) ListDailyReportDates() ([]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT date FROM daily_reports ORDER BY date DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

var exportQueries = []struct {
	File  string
	Query string
}{
	{"sectors.json", "SELECT datetime, name, net, rate, change_pct, super_net, super_rate, big_net, big_rate, volume, turnover, bk_code, turnover_rate, lead_stock_name, lead_stock_change_pct, total_market_cap, circulating_market_cap, input_date FROM sectors ORDER BY datetime, name"},
	{"sectors_all.json", "SELECT date, code, name, net, rate, change_pct, super_net, super_rate, big_net, big_rate, volume, turnover, turnover_rate, lead_stock_name, lead_stock_change_pct, total_market_cap, circulating_market_cap FROM sectors_all ORDER BY date, name"},
	{"copywriting.json", "SELECT date, session, type, content FROM copywriting ORDER BY date, session, type"},
	{"tick_events.json", "SELECT date, session, type, payload FROM tick_events ORDER BY date, session, type"},
	{"cls_news.json", "SELECT id, title, content, brief, level, reading_num, ctime, shareurl, sectors, created_at FROM cls_news ORDER BY ctime DESC"},
}

// ListNotes 列出所有笔记（按创建时间倒序）。
func (db *DB) ListNotes() ([]Note, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT id, type, content, created_at, updated_at FROM notes ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Type, &n.Content, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// SaveNote 创建一条笔记。
func (db *DB) SaveNote(note Note) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.db.Exec("INSERT INTO notes (type, content) VALUES (?, ?)", note.Type, note.Content)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateNote 更新笔记的 type 和/或 content。
func (db *DB) UpdateNote(id int64, noteType, content string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.Exec("UPDATE notes SET type = ?, content = ?, updated_at = datetime('now','localtime') WHERE id = ?", noteType, content, id)
	return err
}

// DeleteNote 删除一条笔记。
func (db *DB) DeleteNote(id int64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.Exec("DELETE FROM notes WHERE id = ?", id)
	return err
}

// ExportJSON 将数据库全部表导出为 JSON 文件到 data/ 目录。
// 仅在 DATA_MODE=json 时有效，sqlite 模式不产生 JSON 文件。
func (db *DB) ExportJSON() error {
	dir := config.GetDataDir()
	os.MkdirAll(dir, 0755)

	for _, t := range exportQueries {
		rows, err := db.RawRows(t.Query)
		if err != nil {
			logger.Warn("导出失败", zap.String("table", t.File), zap.Error(err))
			continue
		}
		path := filepath.Join(dir, t.File)
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			logger.Warn("JSON 序列化失败", zap.String("table", t.File), zap.Error(err))
			continue
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			logger.Warn("写入失败", zap.String("path", path), zap.Error(err))
			continue
		}
		logger.Info("JSON 已导出", zap.String("table", t.File), zap.Int("rows", len(rows)))
	}
	return nil
}
