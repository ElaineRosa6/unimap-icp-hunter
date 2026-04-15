package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database 数据库连接管理
type Database struct {
	db *sql.DB
}

// NewDatabase 创建数据库连接
func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Database{db: db}, nil
}

// Close 关闭数据库连接
func (d *Database) Close() error {
	return d.db.Close()
}

// InitSchema 初始化数据库表结构
func (d *Database) InitSchema() error {
	// 创建规则表
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			pattern TEXT NOT NULL,
			description TEXT,
			severity INTEGER DEFAULT 1,
			enabled BOOLEAN DEFAULT TRUE,
			priority INTEGER DEFAULT 100,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create rules table: %w", err)
	}

	// 创建白名单表
	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS whitelist (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			value TEXT NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create whitelist table: %w", err)
	}

	// 创建规则版本表
	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS rule_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			rule_id INTEGER NOT NULL,
			version INTEGER NOT NULL,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			pattern TEXT NOT NULL,
			description TEXT,
			severity INTEGER DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (rule_id) REFERENCES rules(id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create rule_versions table: %w", err)
	}

	// 创建索引
	_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_rules_type ON rules(type)`)
	if err != nil {
		return fmt.Errorf("failed to create index on rules.type: %w", err)
	}

	_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_rules_enabled ON rules(enabled)`)
	if err != nil {
		return fmt.Errorf("failed to create index on rules.enabled: %w", err)
	}

	_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_whitelist_type ON whitelist(type)`)
	if err != nil {
		return fmt.Errorf("failed to create index on whitelist.type: %w", err)
	}

	_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_rule_versions_rule_id ON rule_versions(rule_id)`)
	if err != nil {
		return fmt.Errorf("failed to create index on rule_versions.rule_id: %w", err)
	}

	return nil
}

// Rule 规则结构
type Rule struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Pattern     string    `json:"pattern"`
	Description string    `json:"description"`
	Severity    int       `json:"severity"`
	Enabled     bool      `json:"enabled"`
	Priority    int       `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Whitelist 白名单结构
type Whitelist struct {
	ID          int       `json:"id"`
	Type        string    `json:"type"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// RuleVersion 规则版本结构
type RuleVersion struct {
	ID          int       `json:"id"`
	RuleID      int       `json:"rule_id"`
	Version     int       `json:"version"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Pattern     string    `json:"pattern"`
	Description string    `json:"description"`
	Severity    int       `json:"severity"`
	CreatedAt   time.Time `json:"created_at"`
}