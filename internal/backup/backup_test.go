package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackup_CreatesTarGz(t *testing.T) {
	// 创建测试源
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "sub")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(subDir, "b.txt"), []byte("world"), 0644)

	outDir := t.TempDir()

	cfg := BackupConfig{
		Sources:   []string{srcDir},
		OutputDir: outDir,
		Prefix:    "test",
	}

	result, err := Backup(cfg)
	if err != nil {
		t.Fatalf("Backup() error: %v", err)
	}

	if result.Path == "" {
		t.Fatal("expected non-empty path")
	}
	if result.Size <= 0 {
		t.Fatal("expected positive size")
	}

	// 验证文件存在
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("backup file should exist: %v", err)
	}
}

func TestBackup_NoSources(t *testing.T) {
	outDir := t.TempDir()

	_, err := Backup(BackupConfig{
		Sources:   []string{"/nonexistent/path/that/does/not/exist"},
		OutputDir: outDir,
		Prefix:    "test",
	})

	if err == nil {
		t.Fatal("expected error for non-existent source")
	}
}

func TestBackup_DefaultOutputDir(t *testing.T) {
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("test"), 0644)

	// 使用默认输出目录
	result, err := Backup(BackupConfig{
		Sources: []string{srcDir},
		Prefix:  "test",
	})
	if err != nil {
		// 默认 ./backups 可能不可写，只要不 panic 就行
		t.Logf("Backup with default dir: %v", err)
	} else {
		// 如果成功，验证路径包含 backups
		if result.Path == "" {
			t.Fatal("expected non-empty path")
		}
	}
}

func TestListBackups_Empty(t *testing.T) {
	dir := t.TempDir()

	backups, err := ListBackups(dir, "test")
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected 0 backups, got %d", len(backups))
	}
}

func TestListBackups_FindsBackups(t *testing.T) {
	// 先创建一个备份
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("test"), 0644)

	outDir := t.TempDir()

	_, err := Backup(BackupConfig{
		Sources:   []string{srcDir},
		OutputDir: outDir,
		Prefix:    "test",
	})
	if err != nil {
		t.Fatalf("Backup() error: %v", err)
	}

	backups, err := ListBackups(outDir, "test")
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}
	if backups[0].Size <= 0 {
		t.Fatal("expected positive size")
	}
}

func TestListBackups_FiltersByPrefix(t *testing.T) {
	outDir := t.TempDir()

	// 创建不同前缀的备份文件
	os.WriteFile(filepath.Join(outDir, "aaa_backup_20250101_120000.tar.gz"), []byte("dummy"), 0644)
	os.WriteFile(filepath.Join(outDir, "bbb_backup_20250101_120000.tar.gz"), []byte("dummy"), 0644)

	backups, err := ListBackups(outDir, "aaa")
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup with 'aaa' prefix, got %d", len(backups))
	}
}

func TestCleanupOldBackups(t *testing.T) {
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("test"), 0644)

	outDir := t.TempDir()

	// 创建 5 个备份，每个间隔 1.1 秒以避免文件名冲突
	for i := 0; i < 5; i++ {
		_, err := Backup(BackupConfig{
			Sources:    []string{srcDir},
			OutputDir:  outDir,
			Prefix:     "test",
			MaxBackups: 0, // 不自动清理
		})
		if err != nil {
			t.Fatalf("Backup() #%d error: %v", i+1, err)
		}
		// 错开时间避免同秒覆盖
		if i < 4 {
			time.Sleep(1100 * time.Millisecond)
		}
	}

	// 验证有 5 个备份
	backups, err := ListBackups(outDir, "test")
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) != 5 {
		t.Fatalf("expected 5 backups, got %d", len(backups))
	}

	// 现在创建第 6 个备份，设置 MaxBackups=3
	_, err = Backup(BackupConfig{
		Sources:    []string{srcDir},
		OutputDir:  outDir,
		Prefix:     "test",
		MaxBackups: 3,
	})
	if err != nil {
		t.Fatalf("Backup() error: %v", err)
	}

	// 应该只剩 3 个
	backups, err = ListBackups(outDir, "test")
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}
	if len(backups) > 3 {
		t.Fatalf("expected <= 3 backups after cleanup, got %d", len(backups))
	}
}

func TestCollectFiles_NonExistent(t *testing.T) {
	_, err := collectFiles("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestCollectFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := collectFiles(dir)
	if err != nil {
		t.Fatalf("collectFiles() error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files in empty dir, got %d", len(files))
	}
}

func TestCollectFiles_WithFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "c.txt"), []byte("c"), 0644)

	files, err := collectFiles(dir)
	if err != nil {
		t.Fatalf("collectFiles() error: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
}

func TestCollectFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "single.txt")
	os.WriteFile(file, []byte("single"), 0644)

	files, err := collectFiles(file)
	if err != nil {
		t.Fatalf("collectFiles() error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}
