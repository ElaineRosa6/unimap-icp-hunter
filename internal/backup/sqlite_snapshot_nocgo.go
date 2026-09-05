//go:build !cgo

package backup

import (
	"context"
	"database/sql"
	"fmt"
)

// SnapshotSQLite requires the CGO SQLite driver, like application persistence.
func SnapshotSQLite(ctx context.Context, source *sql.DB, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("SQLite snapshots require CGO")
}
