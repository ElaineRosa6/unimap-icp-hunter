package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupContextCancelledBeforeStart(t *testing.T) {
	output := filepath.Join(t.TempDir(), "not-created")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := BackupContext(ctx, BackupConfig{Sources: []string{"absent"}, OutputDir: output})
	if result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("result=%v error=%v", result, err)
	}
	if _, err = os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("cancelled backup created output: %v", err)
	}
	if _, _, err = collectFilesContext(ctx, "absent"); !errors.Is(err, context.Canceled) {
		t.Fatalf("collection ignored cancellation: %v", err)
	}
}

type cancelHeaderWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelHeaderWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.cancel()
	return n, err
}

func TestBackupContextCancellationAfterTarHeader(t *testing.T) {
	source := t.TempDir()
	path := filepath.Join(source, "payload")
	payload := bytes.Repeat([]byte("content"), 10000)
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := &cancelHeaderWriter{cancel: cancel}
	tw := tar.NewWriter(sink)
	if err := addFileToTarContext(ctx, tw, path, source); !errors.Is(err, context.Canceled) {
		t.Fatalf("copy ignored cancellation: %v", err)
	}
	if sink.Len() != 512 {
		t.Fatalf("copied payload after header cancellation: %d bytes", sink.Len())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, payload) {
		t.Fatalf("source changed: %v", err)
	}
}

type cancellingRead struct {
	cancel context.CancelFunc
	calls  int
}

func (r *cancellingRead) Read(p []byte) (int, error) {
	r.calls++
	copy(p, "data")
	r.cancel()
	return 4, io.EOF
}

func TestBackupContextReaderObservesCancellationDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &cancellingRead{cancel: cancel}
	reader := contextReader{ctx: ctx, reader: source}
	n, err := reader.Read(make([]byte, 32))
	if n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("returned cancelled data: %d %v", n, err)
	}
	n, err = reader.Read(make([]byte, 32))
	if n != 0 || !errors.Is(err, context.Canceled) || source.calls != 1 {
		t.Fatalf("read after cancellation: %d %v calls=%d", n, err, source.calls)
	}
}

func TestBackupContextNormalArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload")
	if err := os.WriteFile(path, []byte("complete"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := BackupContext(context.Background(), BackupConfig{Sources: []string{path}, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := readBoundaryArchive(t, result.Path)["payload"]; got != "complete" {
		t.Fatalf("archive payload=%q", got)
	}
}
