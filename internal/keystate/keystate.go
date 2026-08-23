// Package keystate records, per key, when sshakku added it to the agent, how
// long it is meant to live there and which file it came from — so `sshakku
// doctor` can report a key's remaining time in the agent without relying on
// ssh-agent to expose it (the ssh-agent protocol has no query for a key's
// remaining lifetime), and so a system whose agent expires nothing can be told
// which key to take out of it and where that key is. Each record is a small
// file under the per-login runtime directory (wiped on logout/reboot on the
// systems that have one, never written to disk otherwise); it holds no secret,
// only a timestamp, a duration and a path.
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
	// Lifetime is how long the key is meant to stay in the agent, whichever
	// side ends up enforcing it; zero means no expiry.
	Lifetime time.Duration
	// KeyFile is the file the key was added from, which is what names it to
	// `ssh-add` again. It is empty for a record written before records said
	// so, and such a record is one nothing can act on.
	KeyFile string
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

// Save records that the key in keyfile was just added to the agent with the
// given lifetime (zero for no expiry), stamped with the current time. The
// record is named after keyfile's base name, and keeps keyfile itself so a
// later caller can name the same key to ssh-add.
func (s Store) Save(keyfile string, lifetime time.Duration) error {
	p, ok := s.path(keyfile)
	if !ok {
		return nil
	}
	if err := os.MkdirAll(s.Dir, dirPerm); err != nil {
		return err
	}
	body := fmt.Sprintf("%s\n%d\n%s\n", s.now().UTC().Format(time.RFC3339), int64(lifetime/time.Second), keyfile)
	return os.WriteFile(p, []byte(body), filePerm)
}

// Records lists every well-formed record in Dir, in no particular order. A
// store that has never been written to has none, which is not an error; a file
// that is not a record is left out, exactly as Load leaves it out.
func (s Store) Records() ([]Record, error) {
	if s.Dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	recs := make([]Record, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if rec, ok := s.Load(e.Name()); ok {
			recs = append(recs, rec)
		}
	}
	return recs, nil
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
	// Three fields, of which the path is the last: it is taken as the whole of
	// what follows, so a path is never cut short by whatever it happens to
	// contain. Two fields is the older form, and reads as a record whose key
	// file is not known.
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 3)
	if len(lines) < 2 {
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
	rec := Record{AddedAt: addedAt, Lifetime: time.Duration(secs) * time.Second}
	if len(lines) == 3 {
		rec.KeyFile = strings.TrimSpace(lines[2])
	}
	return rec, true
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
