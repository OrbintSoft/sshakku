// Package keystate records, per key, when sshakku added it to the agent and
// for how long it was asked to live there — so `sshakku doctor` can report a
// key's remaining time in the agent without relying on ssh-agent to expose it
// (the ssh-agent protocol has no query for a key's remaining lifetime). Each
// record is a small file under the per-login runtime directory (wiped on
// logout/reboot, never written to disk otherwise); it holds no secret, only a
// timestamp and a duration.
package keystate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
)

// Record is what Store persists for one key.
type Record struct {
	// AddedAt is when sshakku added the key to the agent.
	AddedAt time.Time
	// Lifetime is the value passed to `ssh-add -t`; zero means no expiry.
	Lifetime time.Duration
}

// ExpiresAt returns when the key expires, and false when Lifetime is zero
// (the key was added with no expiry).
func (r Record) ExpiresAt() (time.Time, bool) {
	if r.Lifetime <= 0 {
		return time.Time{}, false
	}
	return r.AddedAt.Add(r.Lifetime), true
}

// Store records key-lifetime state as one file per key under Dir.
type Store struct {
	// Dir holds the per-key records; it is created on the first Save. An
	// empty Dir disables the store: Save is a no-op and Load always misses.
	Dir string
	// Now is the clock, overridable in tests; nil uses time.Now.
	Now func() time.Time
}

// Save records that key was just added to the agent with the given lifetime
// (zero for no expiry), stamped with the current time.
func (s Store) Save(key string, lifetime time.Duration) error {
	p, ok := s.path(key)
	if !ok {
		return nil
	}
	if err := os.MkdirAll(s.Dir, dirPerm); err != nil {
		return err
	}
	body := fmt.Sprintf("%s\n%d\n", s.now().UTC().Format(time.RFC3339), int64(lifetime/time.Second))
	return os.WriteFile(p, []byte(body), filePerm)
}

// Load returns the recorded state for key, and whether a well-formed record
// was found. A missing or malformed record reports false — the caller treats
// the key's lifetime as unknown (e.g. added outside sshakku, or before a
// reboot wiped the runtime directory).
func (s Store) Load(key string) (Record, bool) {
	p, ok := s.path(key)
	if !ok {
		return Record{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Record{}, false
	}
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) != 2 {
		return Record{}, false
	}
	addedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[0]))
	if err != nil {
		return Record{}, false
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
	if err != nil {
		return Record{}, false
	}
	return Record{AddedAt: addedAt, Lifetime: time.Duration(secs) * time.Second}, true
}

// Clear removes any recorded state for key; a missing record is not an error.
func (s Store) Clear(key string) error {
	p, ok := s.path(key)
	if !ok {
		return nil
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// path maps a key to its record file, taking the base name so a path cannot
// escape Dir. It reports false when the store is disabled or the name is
// unusable.
func (s Store) path(key string) (string, bool) {
	if s.Dir == "" {
		return "", false
	}
	name := filepath.Base(key)
	if name == "." || name == ".." || name == string(filepath.Separator) || strings.TrimSpace(name) == "" {
		return "", false
	}
	return filepath.Join(s.Dir, name), true
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
