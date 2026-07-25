package sessionlog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.log")
	lg := New(path)
	if err := lg.Log("INFO", "first"); err != nil {
		t.Fatal(err)
	}
	if err := lg.Log("ERROR", "second"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "[INFO] first") || !strings.Contains(got, "[ERROR] second") {
		t.Errorf("log missing entries:\n%s", got)
	}
	if n := strings.Count(got, "\n"); n != 2 {
		t.Errorf("newline count = %d, want 2", n)
	}
}

func TestLogPerm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.log")
	if err := New(path).Log("INFO", "x"); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
}

func TestLogTrims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.log")
	lg := &Logger{path: path, maxLines: 3}
	for i := 0; i < 10; i++ {
		if err := lg.Log("INFO", fmt.Sprintf("line-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("kept %d lines, want 3", len(lines))
	}
	if !strings.Contains(lines[0], "line-7") {
		t.Errorf("first kept line = %q, want line-7", lines[0])
	}
	if !strings.Contains(lines[2], "line-9") {
		t.Errorf("last line = %q, want line-9", lines[2])
	}
}

// TestLogOpenError covers the branch where the log file cannot be opened: a
// parent directory that does not exist makes os.OpenFile fail.
func TestLogOpenError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "sessions.log")
	if err := New(path).Log("INFO", "x"); err == nil {
		t.Error("Log into a missing directory returned nil, want error")
	}
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
	if err := lg.Log("INFO", "x"); err == nil {
		t.Error("Log with a failing Write returned nil, want error")
	}
	if !wc.closed {
		t.Error("Log did not close the file after a write failure")
	}
}

func TestLogCloseError(t *testing.T) {
	wc := &errWriteCloser{closeErr: errors.New("close failed")}
	lg := &Logger{
		path:     filepath.Join(t.TempDir(), "sessions.log"),
		maxLines: DefaultMaxLines,
		open:     func(string, int, os.FileMode) (io.WriteCloser, error) { return wc, nil },
	}
	if err := lg.Log("INFO", "x"); err == nil {
		t.Error("Log with a failing Close returned nil, want error")
	}
}

// TestTrimReadError covers trim's read-failure branch: a path that is a
// directory cannot be read as a file.
func TestTrimReadError(t *testing.T) {
	lg := &Logger{path: t.TempDir(), maxLines: 3}
	if err := lg.trim(); err == nil {
		t.Error("trim on a directory path returned nil, want error")
	}
}
