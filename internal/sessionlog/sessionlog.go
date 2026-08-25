// Package sessionlog appends timestamped, level-tagged lines to sshakku's
// owner-only session log and keeps it bounded to a fixed number of recent lines.
package sessionlog

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// DefaultMaxLines bounds the session log; older lines are dropped on write.
const DefaultMaxLines = 100

const (
	filePerm   = 0o600
	timeLayout = "2006-01-02 15:04:05"
	// lockSuffix names the lock file, which sits beside the log rather than
	// being the log. Windows enforces a lock against every handle but the one
	// holding it, so a writer that locked the log itself would then be refused
	// by its own lock when it reopened the file to trim it.
	lockSuffix = ".lock"
	// defaultLockWait bounds how long a writer queues for the lock before giving
	// up on it. What it queues behind is one append and one rewrite of a file of
	// at most maxLines lines, so a wait anywhere near this long means something
	// other than contention is wrong — and somebody's login shell is not the
	// place to discover that by stopping.
	defaultLockWait = 2 * time.Second
	// defaultLockPoll is the retry interval while queuing.
	defaultLockPoll = 2 * time.Millisecond
)

// Logger appends to a single log file, trimming it to maxLines after each write.
type Logger struct {
	path     string
	maxLines int
	// lockWait and lockPoll are how long to queue for the log's lock and how
	// often to retry while queuing. A test shortens them to reach the giving-up
	// path without spending the real wait on it.
	lockWait time.Duration
	lockPoll time.Duration
	// open creates the log file for appending; a nil value uses os.OpenFile. It
	// is injectable so the write- and close-failure paths can be exercised.
	open func(path string, flag int, perm os.FileMode) (io.WriteCloser, error)
}

// New returns a Logger writing to path, bounded to DefaultMaxLines.
func New(path string) *Logger {
	return &Logger{
		path:     path,
		maxLines: DefaultMaxLines,
		lockWait: defaultLockWait,
		lockPoll: defaultLockPoll,
	}
}

// openAppend is the production opener: the log file opened for create/append.
func openAppend(path string, flag int, perm os.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(path, flag, perm)
}

// Log appends one "TIMESTAMP | [LEVEL] message" line, then trims the file to the
// most recent maxLines lines.
//
// Both halves happen under one lock, and the append needs it as much as the trim
// does. Appending on its own is safe — O_APPEND puts the line at whatever the end
// is at that moment, however many other writers there are — but trimming reads
// the whole file and writes it back, so an append that lands between that read
// and that write is one the trim overwrites. Several shells opening at once is
// exactly when that happens, and it is what the lock is here to stop.
//
// A writer that cannot get the lock in time appends anyway and leaves the file
// untrimmed. It is the last resort rather than the plan: it risks the line the
// way the unlocked version risked every line, but it keeps a login shell from
// stopping on a lock that something else has wedged, and it never makes the
// clobbering worse by trimming on top of it.
func (l *Logger) Log(level, message string) error {
	line := fmt.Sprintf("%s | [%s] %s\n", time.Now().Format(timeLayout), level, message)

	release, held, err := l.lock()
	if err != nil {
		return err
	}
	defer release()

	open := l.open
	if open == nil {
		open = openAppend
	}
	f, err := open(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePerm)
	if err != nil {
		return err
	}
	if _, err := f.Write([]byte(line)); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if !held {
		return nil
	}
	return l.trim()
}

// lock queues for the log's lock and reports whether it got it, along with the
// function that releases it — which is safe to call either way, so a caller that
// went ahead without the lock needs no separate path for putting it back.
//
// An error is the lock file itself refusing to be opened, which is a log that
// cannot be written rather than a lock somebody else holds.
func (l *Logger) lock() (release func(), held bool, err error) {
	lock := flock.New(l.path + lockSuffix)
	deadline := time.Now().Add(l.lockWait)
	for {
		got, err := lock.TryLock()
		if err != nil {
			return nil, false, err
		}
		if got {
			return func() { _ = lock.Unlock() }, true, nil
		}
		if !time.Now().Before(deadline) {
			return func() {}, false, nil
		}
		time.Sleep(l.lockPoll)
	}
}

// trim rewrites the file keeping only its last maxLines lines.
func (l *Logger) trim() error {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) <= l.maxLines {
		return nil
	}
	kept := lines[len(lines)-l.maxLines:]
	return os.WriteFile(l.path, []byte(strings.Join(kept, "\n")+"\n"), filePerm)
}
