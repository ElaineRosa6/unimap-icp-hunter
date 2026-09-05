package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"
)

// SQLiteConnection binds a trusted application-owned pool to its startup file
// identity. The application retains lifetime ownership of DB and must not replace
// its database path while the pool is open.
type SQLiteConnection struct {
	DB       *sql.DB
	Identity os.FileInfo
}

// BindSQLiteConnection runs during owner initialization, not on backup paths.
func BindSQLiteConnection(ctx context.Context, db *sql.DB) (SQLiteConnection, error) {
	if db == nil {
		return SQLiteConnection{}, fmt.Errorf("nil SQLite owner")
	}
	rows, err := db.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		return SQLiteConnection{}, err
	}
	defer rows.Close()
	var path string
	for rows.Next() {
		var seq int
		var schema, file string
		if err = rows.Scan(&seq, &schema, &file); err != nil {
			return SQLiteConnection{}, err
		}
		if schema == "main" {
			path = file
		}
	}
	if err = rows.Err(); err != nil {
		return SQLiteConnection{}, err
	}
	if path == "" {
		return SQLiteConnection{}, fmt.Errorf("SQLite owner has no persistent main database")
	}
	info, err := os.Stat(path)
	if err != nil {
		return SQLiteConnection{}, err
	}
	return SQLiteConnection{DB: db, Identity: info}, nil
}

// SQLiteSnapshotterFor rejects unbound SQLite files. Each snapshot has a two
// minute ceiling, further bounded by the caller's context. It opens no source
// path; metadata from BackupContext is matched to an already-owned pool.
func SQLiteSnapshotterFor(connections ...SQLiteConnection) SQLiteSnapshotter {
	owned := append([]SQLiteConnection(nil), connections...)
	return func(ctx context.Context, info os.FileInfo, destination string) error {
		for _, source := range owned {
			if source.DB != nil && source.Identity != nil && os.SameFile(source.Identity, info) {
				bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
				defer cancel()
				return SnapshotSQLite(bounded, source.DB, destination)
			}
		}
		return fmt.Errorf("SQLite backup source has no bound application connection")
	}
}
