package sessionlog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errCloseFailed      = errors.New("close failed")
	errDiskFull         = errors.New("disk full")
	errPermissionDenied = errors.New("permission denied")
)

func TestLogAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.log")
	lg := New(path)
	require.NoError(t, lg.Log("INFO", "first"))
	require.NoError(t, lg.Log("ERROR", "second"))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	got := string(data)
	assert.Contains(t, got, "[INFO] first")
	assert.Contains(t, got, "[ERROR] second")
	assert.Equal(t, 2, strings.Count(got, "\n"), "one newline per entry")
}

func TestLogTrims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.log")
	lg := New(path)
	lg.maxLines = 3
	for i := range 10 {
		require.NoError(t, lg.Log("INFO", fmt.Sprintf("line-%d", i)))
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	require.Len(t, lines, 3, "kept lines")
	assert.Contains(t, lines[0], "line-7", "first kept line")
	assert.Contains(t, lines[2], "line-9", "last kept line")
}

// TestLogLockOpenError covers the branch where the lock cannot even be attempted:
// a parent directory that does not exist is one the lock file cannot be created
// in either, and that is the first thing Log reaches for.
func TestLogLockOpenError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "sessions.log")
	assert.Error(t, New(path).Log("INFO", "x"), "Log into a missing directory must fail")
}

// TestLogAppendOpenError covers the branch where the lock was taken and the log
// itself is what will not open. It needs the injected opener: a directory the
// lock file could be created in is one the log file could be created in too, so
// nothing about a real path puts the failure on this side of the lock.
func TestLogAppendOpenError(t *testing.T) {
	lg := New(filepath.Join(t.TempDir(), "sessions.log"))
	lg.open = func(string, int, os.FileMode) (io.WriteCloser, error) {
		return nil, errPermissionDenied
	}
	assert.Error(t, lg.Log("INFO", "x"), "Log must fail when the log file will not open")
}

// errWriteCloser is an injected log file whose Write and/or Close fail on demand,
// so Log's write- and close-failure branches can be driven.
type errWriteCloser struct {
	writeErr error
	closeErr error
	closed   bool
}

func (w *errWriteCloser) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return len(p), nil
}

func (w *errWriteCloser) Close() error {
	w.closed = true
	return w.closeErr
}

func TestLogWriteError(t *testing.T) {
	wc := &errWriteCloser{writeErr: errDiskFull}
	lg := New(filepath.Join(t.TempDir(), "sessions.log"))
	lg.open = func(string, int, os.FileMode) (io.WriteCloser, error) { return wc, nil }
	assert.Error(t, lg.Log("INFO", "x"), "Log with a failing Write must fail")
	assert.True(t, wc.closed, "Log must close the file after a write failure")
}

func TestLogCloseError(t *testing.T) {
	wc := &errWriteCloser{closeErr: errCloseFailed}
	lg := New(filepath.Join(t.TempDir(), "sessions.log"))
	lg.open = func(string, int, os.FileMode) (io.WriteCloser, error) { return wc, nil }
	assert.Error(t, lg.Log("INFO", "x"), "Log with a failing Close must fail")
}

// TestTrimReadError covers trim's read-failure branch: a path that is a
// directory cannot be read as a file.
func TestTrimReadError(t *testing.T) {
	lg := &Logger{path: t.TempDir(), maxLines: 3}
	assert.Error(t, lg.trim(), "trim on a directory path must fail")
}
