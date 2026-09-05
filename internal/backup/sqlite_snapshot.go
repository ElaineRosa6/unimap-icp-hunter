//go:build cgo

package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
)

// SnapshotSQLite creates a new standalone database from an existing connection.
// The caller owns the destination directory and must supply a bounded context
// for potentially busy databases. An existing destination is never replaced.
// This primitive does not change Backup's source selection or archive behavior.
func SnapshotSQLite(ctx context.Context, source *sql.DB, destination string) (resultErr error) {
	createdDestination := false
	defer func() {
		if createdDestination && resultErr != nil {
			if removeErr := os.Remove(destination); removeErr != nil && !os.IsNotExist(removeErr) {
				resultErr = errors.Join(resultErr, fmt.Errorf("remove failed snapshot: %w", removeErr))
			}
		}
	}()
	if source == nil {
		return fmt.Errorf("snapshot source database is nil")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	sourceConn, err := source.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire snapshot source: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, sourceConn.Close()) }()

	created, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("create snapshot destination: %w", err)
	}
	createdDestination = true
	if closeErr := created.Close(); closeErr != nil {
		return closeErr
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	path := filepath.ToSlash(absolute)
	if filepath.VolumeName(absolute) != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	uri := url.URL{Scheme: "file", Path: path}
	uri.RawQuery = url.Values{"mode": {"rw"}, "_journal_mode": {"DELETE"}, "_busy_timeout": {"50"}}.Encode()
	target, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, target.Close()) }()
	target.SetMaxOpenConns(1)
	targetConn, err := target.Conn(ctx)
	if err != nil {
		return err
	}
	copyErr := sourceConn.Raw(func(rawSource any) error {
		src, ok := rawSource.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("snapshot requires a SQLite source connection")
		}
		return targetConn.Raw(func(rawTarget any) error {
			dst, ok := rawTarget.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("snapshot requires a SQLite destination connection")
			}
			return copySQLitePages(ctx, src, dst)
		})
	})
	if copyErr = errors.Join(copyErr, targetConn.Close()); copyErr != nil {
		return copyErr
	}
	var check string
	if checkErr := target.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); checkErr != nil {
		return checkErr
	}
	if check != "ok" {
		return fmt.Errorf("snapshot failed integrity check")
	}
	if closeErr := target.Close(); closeErr != nil {
		return closeErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	file, err := os.OpenFile(destination, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

func copySQLitePages(ctx context.Context, source, target *sqlite3.SQLiteConn) (resultErr error) {
	handle, err := target.Backup("main", source, "main")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, handle.Finish()) }()
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		done, stepErr := handle.Step(128)
		if stepErr != nil {
			return stepErr
		}
		if done {
			return ctx.Err()
		}
		// Step reports SQLITE_BUSY/LOCKED as not-done with nil error. Yield rather
		// than spinning; a caller deadline also bounds sustained concurrent writes.
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
