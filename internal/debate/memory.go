package debate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

type DebateMemory interface {
	GetRecentSessions(ctx context.Context, stockCode string, limit int) ([]SessionEntry, error)
	SaveSession(ctx context.Context, entry SessionEntry) error
}

type SessionEntry struct {
	SessionID  string
	StockCode  string
	StockName  string
	Date       string
	Verdict    string
	TurnsCount int
	Script     Script
}

type DebateMemoryNoop struct{}

func NewDebateMemoryNoop() *DebateMemoryNoop { return &DebateMemoryNoop{} }

func (m *DebateMemoryNoop) GetRecentSessions(ctx context.Context, stockCode string, limit int) ([]SessionEntry, error) {
	return nil, nil
}

func (m *DebateMemoryNoop) SaveSession(ctx context.Context, entry SessionEntry) error {
	logger.Debug("辩论记录(noop)",
		zap.String("sessionId", entry.SessionID),
		zap.String("stockCode", entry.StockCode),
		zap.Int("turns", entry.TurnsCount),
	)
	return nil
}

type DebateMemorySQLite struct {
	db *sql.DB
}

func NewDebateMemorySQLite(dbPath string) (*DebateMemorySQLite, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}
	db.SetMaxOpenConns(1)

	schema := `
	CREATE TABLE IF NOT EXISTS debate_sessions (
		session_id   TEXT PRIMARY KEY,
		stock_code   TEXT NOT NULL DEFAULT '',
		stock_name   TEXT NOT NULL DEFAULT '',
		report_date  TEXT NOT NULL DEFAULT '',
		verdict      TEXT NOT NULL DEFAULT '',
		turns_count  INTEGER NOT NULL DEFAULT 0,
		script_json  TEXT NOT NULL DEFAULT '',
		created_at   TEXT NOT NULL DEFAULT (datetime('now','localtime'))
	);
	CREATE INDEX IF NOT EXISTS idx_debate_sessions_stock_date
		ON debate_sessions(stock_code, created_at DESC);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("建表失败: %w", err)
	}
	return &DebateMemorySQLite{db: db}, nil
}

func (m *DebateMemorySQLite) Close() error {
	return m.db.Close()
}

func (m *DebateMemorySQLite) SaveSession(ctx context.Context, entry SessionEntry) error {
	scriptJSON, err := json.Marshal(entry.Script)
	if err != nil {
		return fmt.Errorf("script 序列化失败: %w", err)
	}
	_, err = m.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO debate_sessions
		(session_id, stock_code, stock_name, report_date, verdict, turns_count, script_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, COALESCE(?, datetime('now','localtime')))
	`,
		entry.SessionID,
		entry.StockCode,
		entry.StockName,
		entry.Date,
		entry.Verdict,
		entry.TurnsCount,
		string(scriptJSON),
		nil,
	)
	if err != nil {
		logger.Error("辩论记录保存失败",
			zap.String("sessionId", entry.SessionID),
			zap.String("stockCode", entry.StockCode),
			zap.Error(err),
		)
		return fmt.Errorf("保存 session 失败: %w", err)
	}
	logger.Debug("辩论记录已保存",
		zap.String("sessionId", entry.SessionID),
		zap.String("stockCode", entry.StockCode),
		zap.Int("turns", entry.TurnsCount),
	)
	return nil
}

func (m *DebateMemorySQLite) GetRecentSessions(ctx context.Context, stockCode string, limit int) ([]SessionEntry, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := m.db.QueryContext(ctx, `
		SELECT session_id, stock_code, stock_name, report_date, verdict, turns_count, script_json
		FROM debate_sessions
		WHERE stock_code = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, stockCode, limit)
	if err != nil {
		return nil, fmt.Errorf("查询历史 sessions 失败: %w", err)
	}
	defer rows.Close()

	out := make([]SessionEntry, 0)
	for rows.Next() {
		var e SessionEntry
		var scriptJSON string
		if err := rows.Scan(&e.SessionID, &e.StockCode, &e.StockName, &e.Date, &e.Verdict, &e.TurnsCount, &scriptJSON); err != nil {
			return nil, err
		}
		if scriptJSON != "" {
			_ = json.Unmarshal([]byte(scriptJSON), &e.Script)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type DebateMemoryAutoDate struct {
	delegate DebateMemory
	now     func() time.Time
}

func NewDebateMemoryAutoDate(d DebateMemory) *DebateMemoryAutoDate {
	return &DebateMemoryAutoDate{delegate: d, now: time.Now}
}

func (m *DebateMemoryAutoDate) GetRecentSessions(ctx context.Context, stockCode string, limit int) ([]SessionEntry, error) {
	return m.delegate.GetRecentSessions(ctx, stockCode, limit)
}

func (m *DebateMemoryAutoDate) SaveSession(ctx context.Context, entry SessionEntry) error {
	if entry.Date == "" {
		entry.Date = m.now().Format("2006-01-02")
	}
	return m.delegate.SaveSession(ctx, entry)
}
