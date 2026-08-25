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
	lg := &Logger{path: path, maxLines: 3}
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

// TestLogOpenError covers the branch where the log file cannot be opened: a
// parent directory that does not exist makes os.OpenFile fail.
func TestLogOpenError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "sessions.log")
	assert.Error(t, New(path).Log("INFO", "x"), "Log into a missing directory must fail")
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
	wc := &errWriteCloser{writeErr: errors.New("disk full")}
	lg := &Logger{
		path:     filepath.Join(t.TempDir(), "sessions.log"),
		maxLines: DefaultMaxLines,
		open:     func(string, int, os.FileMode) (io.WriteCloser, error) { return wc, nil },
	}
	assert.Error(t, lg.Log("INFO", "x"), "Log with a failing Write must fail")
	assert.True(t, wc.closed, "Log must close the file after a write failure")
}

func TestLogCloseError(t *testing.T) {
	wc := &errWriteCloser{closeErr: errors.New("close failed")}
	lg := &Logger{
		path:     filepath.Join(t.TempDir(), "sessions.log"),
		maxLines: DefaultMaxLines,
		open:     func(string, int, os.FileMode) (io.WriteCloser, error) { return wc, nil },
	}
	assert.Error(t, lg.Log("INFO", "x"), "Log with a failing Close must fail")
}

// TestTrimReadError covers trim's read-failure branch: a path that is a
// directory cannot be read as a file.
func TestTrimReadError(t *testing.T) {
	lg := &Logger{path: t.TempDir(), maxLines: 3}
	assert.Error(t, lg.trim(), "trim on a directory path must fail")
}
