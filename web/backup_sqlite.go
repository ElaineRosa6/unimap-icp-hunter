package web

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/unimap/project/internal/backup"
	"github.com/unimap/project/internal/logger"
)

func initBackupSnapshots(srv *Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var pools []*sql.DB
	if srv.historyDB != nil {
		pools = append(pools, srv.historyDB.DB())
	}
	if srv.userDB != nil {
		pools = append(pools, srv.userDB.DB())
	}
	if srv.batchDB != nil {
		pools = append(pools, srv.batchDB.DB())
	}
	if srv.icpDB != nil {
		pools = append(pools, srv.icpDB.DB())
	}
	var sources []backup.SQLiteConnection
	for _, pool := range pools {
		source, err := backup.BindSQLiteConnection(ctx, pool)
		if err != nil {
			logger.Warnf("backup SQLite owner binding failed: %v", err)
			continue
		}
		sources = append(sources, source)
	}
	if srv.tamperApp != nil {
		db, info, err := srv.tamperApp.HistoryDatabase()
		if err == nil {
			sources = append(sources, backup.SQLiteConnection{DB: db, Identity: info})
		} else {
			logger.Warnf("backup tamper owner binding failed: %v", err)
		}
	}
	srv.backupSnapshotter = backup.SQLiteSnapshotterFor(sources...)
}

func (s *Server) snapshotSQLite(ctx context.Context, info os.FileInfo, destination string) error {
	if s.backupSnapshotter == nil {
		return backup.SQLiteSnapshotterFor()(ctx, info, destination)
	}
	return s.backupSnapshotter(ctx, info, destination)
}
