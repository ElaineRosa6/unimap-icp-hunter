package tamper

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type historyDatabaseOwner struct {
	mu       sync.Mutex
	db       *sql.DB
	info     os.FileInfo
	closed   bool
	closeErr error
}

// NewPersistentHashStorage eagerly opens a service-owned history pool. Unlike
// NewHashStorage, operations share this pool and the caller must call Close.
func NewPersistentHashStorage(baseDir string) (*HashStorage, error) {
	storage := NewHashStorage(baseDir)
	db, err := storage.openHistoryIndex()
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filepath.Join(storage.baseDir, "check_records.db"))
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	storage.historyOwner = &historyDatabaseOwner{db: db, info: info}
	return storage, nil
}

func (owner *historyDatabaseOwner) database() (*sql.DB, error) {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.closed {
		return nil, fmt.Errorf("history database is closed")
	}
	return owner.db, nil
}

func (s *HashStorage) releaseHistoryIndex(db *sql.DB) {
	if s.historyOwner == nil {
		_ = db.Close()
	}
}

// HistoryDatabase borrows the existing pool; callers must not close it. The
// identity is captured at construction, not by reopening a backup source path.
func (s *HashStorage) HistoryDatabase() (*sql.DB, os.FileInfo, error) {
	if s.historyOwner == nil {
		return nil, nil, fmt.Errorf("history database is not persistent")
	}
	db, err := s.historyOwner.database()
	if err != nil {
		return nil, nil, err
	}
	return db, s.historyOwner.info, nil
}

// Close is terminal and idempotent for a persistent store. Short-lived stores
// already close each operation's pool, so Close is a no-op for them.
func (s *HashStorage) Close() error {
	if s.historyOwner == nil {
		return nil
	}
	owner := s.historyOwner
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if !owner.closed {
		owner.closed = true
		owner.closeErr = owner.db.Close()
	}
	return owner.closeErr
}
