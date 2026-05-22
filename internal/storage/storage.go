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
)

type DB struct {
	mu sync.RWMutex
	db *sql.DB
}

type Sector struct {
	Datetime   string  `json:"datetime"`   // "2026-05-19 09:30" (tick) or "2026-05-19" (full)
	Name       string  `json:"name"`
	Net        float64 `json:"net"`        // 主力净流入（亿）
	Rate       float64 `json:"rate"`       // 主力净占比（%）
	InputDate  string  `json:"input_date"` // 录入时间 "2026-05-22 13:14:19"
}

type SectorAll struct {
	Date string  `json:"date"` // "2026-05-19"
	Code string  `json:"code"` // 板块代码 BKxxxx
	Name string  `json:"name"` // 板块名称
	Net  float64 `json:"net"`  // 主力资金净流入（亿）
	Rate float64 `json:"rate"` // 主力净占比（%）
}

type Copywriting struct {
	Date    string `json:"date"`
	Session string `json:"session"`
	Type    string `json:"type"`
	Content string `json:"content"`
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
		datetime   TEXT    NOT NULL,  -- 时间 "2026-05-19 09:30" (tick) 或 "2026-05-19" (全量)
		name       TEXT    NOT NULL,  -- 板块名称
		net        REAL    NOT NULL,  -- 主力资金净流入（亿）
		rate       REAL    NOT NULL DEFAULT 0,  -- 主力净占比（%）
		input_date TEXT    NOT NULL DEFAULT '',  -- 录入时间 "2026-05-22 13:14:19"
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
		date TEXT    NOT NULL,  -- 日期 "2026-05-19"
		code TEXT    NOT NULL,  -- 板块代码 BKxxxx
		name TEXT    NOT NULL,  -- 板块名称
		net  REAL    NOT NULL,  -- 主力资金净流入（亿）
		rate REAL    NOT NULL DEFAULT 0,  -- 主力净占比（%）
		PRIMARY KEY (date, name)
	);

	CREATE TABLE IF NOT EXISTS tick_events (
		date    TEXT    NOT NULL,  -- 日期 "2026-05-19"
		session TEXT    NOT NULL,  -- 时段 "full" / "morning"
		type    TEXT    NOT NULL,  -- 类型 "timeline" / "market" / "ticker"
		payload TEXT    NOT NULL,  -- JSON 数据
		PRIMARY KEY (date, session, type)
	);

	CREATE INDEX IF NOT EXISTS idx_sectors_date ON sectors(datetime);
	CREATE INDEX IF NOT EXISTS idx_copywriting_date ON copywriting(date);
	CREATE INDEX IF NOT EXISTS idx_sectors_all_date ON sectors_all(date);
	CREATE INDEX IF NOT EXISTS idx_tick_events_date ON tick_events(date);
	`
	if _, err := db.db.Exec(schema); err != nil {
		return err
	}

	// Migrations for existing databases
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN input_date TEXT NOT NULL DEFAULT ''")
	_, _ = db.db.Exec("ALTER TABLE sectors ADD COLUMN rate REAL NOT NULL DEFAULT 0")
	_, _ = db.db.Exec("ALTER TABLE sectors_all ADD COLUMN rate REAL NOT NULL DEFAULT 0")

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

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO sectors (datetime, name, net, rate, input_date) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range sectors {
		if _, err := stmt.Exec(s.Datetime, s.Name, s.Net, s.Rate, s.InputDate); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) LoadFullSectors(date string) ([]Sector, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT datetime, name, net, rate, input_date FROM sectors WHERE datetime = ? ORDER BY ABS(net) DESC", date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []Sector
	for rows.Next() {
		var s Sector
		if err := rows.Scan(&s.Datetime, &s.Name, &s.Net, &s.Rate, &s.InputDate); err != nil {
			return nil, err
		}
		sectors = append(sectors, s)
	}
	return sectors, rows.Err()
}

func (db *DB) LoadTickSectors(date string) ([]Sector, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT datetime, name, net, rate, input_date FROM sectors WHERE datetime LIKE ? AND datetime != ? ORDER BY datetime, ABS(net) DESC", date+" %", date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []Sector
	for rows.Next() {
		var s Sector
		if err := rows.Scan(&s.Datetime, &s.Name, &s.Net, &s.Rate, &s.InputDate); err != nil {
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

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO sectors_all (date, code, name, net, rate) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range sectors {
		if _, err := stmt.Exec(s.Date, s.Code, s.Name, s.Net, s.Rate); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) LoadSectorsAll(date string) ([]SectorAll, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT date, code, name, net, rate FROM sectors_all WHERE date = ? ORDER BY ABS(net) DESC", date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []SectorAll
	for rows.Next() {
		var s SectorAll
		if err := rows.Scan(&s.Date, &s.Code, &s.Name, &s.Net, &s.Rate); err != nil {
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

	rows, err := db.db.Query("SELECT date, code, name, net, rate FROM sectors_all WHERE date >= ? AND date <= ? ORDER BY date, ABS(net) DESC", startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []SectorAll
	for rows.Next() {
		var s SectorAll
		if err := rows.Scan(&s.Date, &s.Code, &s.Name, &s.Net, &s.Rate); err != nil {
			return nil, err
		}
		sectors = append(sectors, s)
	}
	return sectors, rows.Err()
}

func (db *DB) LoadSectorTrend(name, startDate, endDate string) ([]SectorAll, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query("SELECT date, code, name, net, rate FROM sectors_all WHERE name = ? AND date >= ? AND date <= ? ORDER BY date", name, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sectors []SectorAll
	for rows.Next() {
		var s SectorAll
		if err := rows.Scan(&s.Date, &s.Code, &s.Name, &s.Net, &s.Rate); err != nil {
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
