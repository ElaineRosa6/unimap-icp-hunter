package tamper

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// CheckRecordQuery selects a page of tamper check records. URL is optional;
// an empty URL selects records across all monitored targets.
type CheckRecordQuery struct {
	URL       string
	StartTime int64
	EndTime   int64
	Limit     int
	Offset    int
}

// CheckRecordPage is a timestamp-descending page of check records.
type CheckRecordPage struct {
	Records []*CheckRecord
	Total   int
	URLs    []string
}

func (s *HashStorage) historyIndex() (*sql.DB, error) {
	if s.historyOwner != nil {
		return s.historyOwner.database()
	}
	return s.openHistoryIndex()
}

func (s *HashStorage) openHistoryIndex() (*sql.DB, error) {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", filepath.Join(s.baseDir, "check_records.db")+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS check_records (id TEXT PRIMARY KEY, url TEXT NOT NULL, timestamp INTEGER NOT NULL, payload TEXT NOT NULL); CREATE INDEX IF NOT EXISTS idx_check_records_url_time ON check_records(url, timestamp DESC);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *HashStorage) indexCheckRecord(record *CheckRecord) error {
	db, err := s.historyIndex()
	if err != nil {
		return err
	}
	defer s.releaseHistoryIndex(db)
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT OR REPLACE INTO check_records(id,url,timestamp,payload) VALUES(?,?,?,?)`, record.ID, record.URL, record.Timestamp, string(payload))
	return err
}

func (s *HashStorage) listIndexedCheckRecords() (map[string][]*CheckRecord, error) {
	db, err := s.historyIndex()
	if err != nil {
		return nil, err
	}
	defer s.releaseHistoryIndex(db)
	rows, err := db.Query(`SELECT payload FROM check_records ORDER BY timestamp DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]*CheckRecord)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var record CheckRecord
		if err := json.Unmarshal([]byte(payload), &record); err != nil {
			continue
		}
		result[record.URL] = append(result[record.URL], &record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate check record index: %w", err)
	}
	return result, nil
}

// ListCheckRecords returns a page of records from the SQLite index. It keeps
// selection, ordering, counting, and pagination in the storage layer so
// callers do not need to load every payload before serving a history page.
func (s *HashStorage) ListCheckRecords(query CheckRecordQuery) (CheckRecordPage, error) {
	query = normalizeCheckRecordQuery(query)
	markerPath := filepath.Join(s.baseDir, ".check_records_indexed")
	if _, err := os.Stat(markerPath); err == nil {
		return s.listIndexedCheckRecordPage(query)
	}

	if _, err := s.ListAllCheckRecords(); err != nil {
		return CheckRecordPage{}, fmt.Errorf("initialize check record index: %w", err)
	}
	return s.listIndexedCheckRecordPage(query)
}

func normalizeCheckRecordQuery(query CheckRecordQuery) CheckRecordQuery {
	if query.Limit <= 0 {
		query.Limit = 200
	}
	if query.Limit > 10000 {
		query.Limit = 10000
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	return query
}

func (s *HashStorage) listIndexedCheckRecordPage(query CheckRecordQuery) (CheckRecordPage, error) {
	db, err := s.historyIndex()
	if err != nil {
		return CheckRecordPage{}, err
	}
	defer s.releaseHistoryIndex(db)

	where, args := checkRecordWhere(query, true)

	var total int
	if countErr := db.QueryRow("SELECT COUNT(*) FROM check_records"+where, args...).Scan(&total); countErr != nil {
		return CheckRecordPage{}, fmt.Errorf("count indexed check records: %w", countErr)
	}
	urls, err := listIndexedCheckRecordURLs(db, query, total)
	if err != nil {
		return CheckRecordPage{}, err
	}

	pageArgs := append(args, query.Limit, query.Offset)
	rows, err := db.Query("SELECT payload FROM check_records"+where+" ORDER BY timestamp DESC LIMIT ? OFFSET ?", pageArgs...)
	if err != nil {
		return CheckRecordPage{}, err
	}
	defer rows.Close()

	page := CheckRecordPage{Records: make([]*CheckRecord, 0, query.Limit), Total: total, URLs: urls}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return CheckRecordPage{}, err
		}
		var record CheckRecord
		if err := json.Unmarshal([]byte(payload), &record); err != nil {
			continue
		}
		page.Records = append(page.Records, &record)
	}
	if err := rows.Err(); err != nil {
		return CheckRecordPage{}, fmt.Errorf("iterate indexed check record page: %w", err)
	}
	return page, nil
}

func listIndexedCheckRecordURLs(db *sql.DB, query CheckRecordQuery, total int) ([]string, error) {
	if query.URL != "" {
		if total == 0 {
			return nil, nil
		}
		return []string{query.URL}, nil
	}

	where, args := checkRecordWhere(query, false)
	rows, err := db.Query(`SELECT DISTINCT url FROM check_records`+where+` ORDER BY url`, args...)
	if err != nil {
		return nil, fmt.Errorf("list indexed check record URLs: %w", err)
	}
	defer rows.Close()
	urls := make([]string, 0)
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed check record URLs: %w", err)
	}
	return urls, nil
}

func checkRecordWhere(query CheckRecordQuery, includeURL bool) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if includeURL && query.URL != "" {
		clauses = append(clauses, "url = ?")
		args = append(args, query.URL)
	}
	if query.StartTime > 0 {
		clauses = append(clauses, "timestamp >= ?")
		args = append(args, query.StartTime)
	}
	if query.EndTime > 0 {
		clauses = append(clauses, "timestamp <= ?")
		args = append(args, query.EndTime)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (s *HashStorage) deleteIndexedCheckRecords(url string) error {
	db, err := s.historyIndex()
	if err != nil {
		return err
	}
	defer s.releaseHistoryIndex(db)
	_, err = db.Exec(`DELETE FROM check_records WHERE url = ?`, url)
	return err
}
