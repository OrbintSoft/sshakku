package giveup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRecordMkdirError covers Record's failure to create Dir: when a parent path
// component is a regular file, MkdirAll cannot make the directory and the error
// is returned rather than swallowed.
func TestRecordMkdirError(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s := Store{Dir: filepath.Join(file, "sub"), TTL: time.Hour}
	if err := s.Record("id_rsa"); err == nil {
		t.Fatal("want an error when Dir cannot be created")
	}
}

// TestClearRemoveError covers Clear's error return for a removal failure that is
// not "already absent": a sentinel that is a non-empty directory cannot be
// unlinked, so the error propagates instead of being treated as a missing record.
func TestClearRemoveError(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "id_rsa")
	if err := os.Mkdir(sentinel, 0o700); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sentinel, "child"), nil, 0o600); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	s := Store{Dir: dir, TTL: time.Hour}
	if err := s.Clear("id_rsa"); err == nil {
		t.Fatal("want an error when the sentinel cannot be removed")
	}
}

// TestUnusableKeyNameDisables covers path's rejection of key names that reduce to
// an unusable base ("..", "/", or all-whitespace): every operation becomes a
// no-op rather than letting a crafted name escape Dir.
func TestUnusableKeyNameDisables(t *testing.T) {
	s := Store{Dir: t.TempDir(), TTL: time.Hour}
	for _, key := range []string{"..", "/", "   "} {
		if s.GivenUp(key) {
			t.Errorf("GivenUp(%q) = true, want false for an unusable key", key)
		}
		if err := s.Record(key); err != nil {
			t.Errorf("Record(%q): %v", key, err)
		}
		if err := s.Clear(key); err != nil {
			t.Errorf("Clear(%q): %v", key, err)
		}
	}
}
