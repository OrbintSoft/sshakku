package keystate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveMkdirAllError(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A component of Dir is a regular file, so creating the store directory fails.
	s := Store{Dir: filepath.Join(file, "sub")}
	if err := s.Save("id_rsa", time.Hour); err == nil {
		t.Fatal("want an error when the store directory cannot be created")
	}
}

func TestLoadWrongLineCountMisses(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if err := os.WriteFile(filepath.Join(dir, "id_rsa"), []byte("only-one-line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Load("id_rsa"); ok {
		t.Fatal("a record without exactly two lines must miss")
	}
}

func TestLoadNonNumericLifetimeMisses(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	// A valid timestamp so parsing reaches the lifetime, which is not a number.
	body := "2026-07-08T12:00:00Z\nnot-a-number\n"
	if err := os.WriteFile(filepath.Join(dir, "id_rsa"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Load("id_rsa"); ok {
		t.Fatal("a record with a non-numeric lifetime must miss")
	}
}

func TestClearRemoveError(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	// The record path is a non-empty directory, so os.Remove fails with an error
	// other than "not exist", which Clear must propagate.
	recDir := filepath.Join(dir, "id_rsa")
	if err := os.Mkdir(recDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recDir, "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear("id_rsa"); err == nil {
		t.Fatal("want an error when the record cannot be removed")
	}
}

func TestUnusableKeyIsIgnored(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	// Keys whose base name is unusable must be rejected before any file I/O.
	for _, key := range []string{".", "..", string(filepath.Separator), "   "} {
		if err := s.Save(key, time.Hour); err != nil {
			t.Fatalf("Save(%q): %v", key, err)
		}
		if _, ok := s.Load(key); ok {
			t.Fatalf("Load(%q) must miss for an unusable key", key)
		}
		if err := s.Clear(key); err != nil {
			t.Fatalf("Clear(%q): %v", key, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("no record should be written for unusable keys, got %v", entries)
	}
}
