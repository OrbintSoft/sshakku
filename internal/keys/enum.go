package keys

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Enumerator lists the candidate private-key files in a directory.
type Enumerator struct {
	Dir string

	// Patterns are shell globs matched against the file name; a file matching
	// any one of them is a key. Empty means the convention SSHakku ships with.
	Patterns []string

	// MustExist makes a missing Dir an error rather than an empty result. An
	// account with no ~/.ssh has no keys and that is ordinary; a directory
	// somebody asked for by name and that is not there is a mistake, and the
	// two are indistinguishable from inside this directory.
	MustExist bool
}

// DefaultKeyDirName is the directory under a user's home that OpenSSH keeps
// keys in, and where SSHakku looks when no other is named.
const DefaultKeyDirName = ".ssh"

// defaultKeyPatterns is the naming convention ssh-keygen follows when it is not
// told otherwise, and the rule that applies when no patterns are given.
var defaultKeyPatterns = []string{"id_*"}

// DefaultKeyPatterns returns the naming rule that applies when no patterns are
// given, for a caller that has to state it rather than apply it — a report of
// what is in force cannot show "nothing" where a rule is at work.
func DefaultKeyPatterns() []string {
	return slices.Clone(defaultKeyPatterns)
}

// notKeys are the files OpenSSH keeps in that directory for its own use. They
// are never private keys, so they are skipped however wide the patterns are:
// "every key in here" is what a user means by "*", and being asked for a
// passphrase to known_hosts is not something typing one can fix.
var notKeys = []string{
	"config",
	"known_hosts",
	"known_hosts.old",
	"authorized_keys",
	"authorized_keys2",
	"environment",
	"rc",
}

// Keys returns the absolute paths of the regular files in Dir whose names match
// Patterns, non-recursively, in directory order. Public halves (*.pub) and the
// files OpenSSH keeps there for itself are never returned. Symlinks are skipped
// (matching the bash `find -type f`), so only real key files are considered.
//
// A missing directory yields no keys and no error unless MustExist is set, so a
// host with no ~/.ssh exits cleanly with nothing to load.
func (e Enumerator) Keys() ([]string, error) {
	entries, err := os.ReadDir(e.Dir)
	if err != nil {
		if !e.MustExist && absent(e.Dir) {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for _, ent := range entries {
		name := ent.Name()
		if !e.isKeyName(name) {
			continue
		}
		if !ent.Type().IsRegular() {
			continue
		}
		keys = append(keys, filepath.Join(e.Dir, name))
	}
	return keys, nil
}

// absent reports whether there is nothing at path at all — asked of the
// filesystem rather than read off the error the read failed with.
//
// The two answers a caller has to tell apart are "you have no key directory",
// which is how most accounts start and is no error, and "what you named is not
// a directory", which is a mistake in the configuration and has to be said.
// Systems do not agree on which error is which: reading a regular file as a
// directory fails with ENOTDIR on unix, but on Windows with the code for a
// path that is not there, which Go reports as both ENOTDIR *and* fs.ErrNotExist
// — so a key_dir pointing at a file was read there as an account with no keys.
// Whether something is there is a question the filesystem answers the same way
// everywhere.
func absent(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// isKeyName reports whether a file name in Dir is one of the user's keys. A
// malformed pattern simply matches nothing here: the configuration layer
// refuses one and says so, and this type is also used with patterns nobody
// wrote.
func (e Enumerator) isKeyName(name string) bool {
	if strings.HasSuffix(name, ".pub") || slices.Contains(notKeys, name) {
		return false
	}
	for _, pattern := range e.patterns() {
		if matched, err := filepath.Match(pattern, name); err == nil && matched {
			return true
		}
	}
	return false
}

func (e Enumerator) patterns() []string {
	if len(e.Patterns) == 0 {
		return defaultKeyPatterns
	}
	return e.Patterns
}
