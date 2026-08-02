package keys

import (
	"os"
	"path/filepath"
	"strings"
)

// Enumerator lists the candidate private-key files in a directory (~/.ssh).
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

// Keys returns the absolute paths of regular files named id_* but not *.pub,
// non-recursively, in directory order. Symlinks are skipped (matching the bash
// `find -type f`), so only real key files are considered. A missing directory
// yields no keys and no error, so a host with no ~/.ssh exits cleanly with
// nothing to load.
func (e Enumerator) Keys() ([]string, error) {
	entries, err := os.ReadDir(e.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasPrefix(name, "id_") || strings.HasSuffix(name, ".pub") {
			continue
		}
		if !ent.Type().IsRegular() {
			continue
		}
		keys = append(keys, filepath.Join(e.Dir, name))
	}
	return keys, nil
}
