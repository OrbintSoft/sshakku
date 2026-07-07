package keystate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	s := Store{Dir: dir, Now: func() time.Time { return now }}

	if err := s.Save("id_rsa", 8*time.Hour); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rec, ok := s.Load("id_rsa")
	if !ok {
		t.Fatal("Load after Save must find a record")
	}
	if !rec.AddedAt.Equal(now) {
		t.Fatalf("AddedAt = %v, want %v", rec.AddedAt, now)
	}
	if rec.Lifetime != 8*time.Hour {
		t.Fatalf("Lifetime = %v, want 8h", rec.Lifetime)
	}
	expiresAt, ok := rec.ExpiresAt()
	if !ok {
		t.Fatal("ExpiresAt must report ok for a non-zero lifetime")
	}
	if want := now.Add(8 * time.Hour); !expiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", expiresAt, want)
	}

	info, err := os.Stat(filepath.Join(dir, "id_rsa"))
	if err != nil {
		t.Fatalf("stat record: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("record perm = %o, want 600", perm)
	}
}

func TestZeroLifetimeNeverExpires(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if err := s.Save("id_rsa", 0); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rec, ok := s.Load("id_rsa")
	if !ok {
		t.Fatal("Load after Save must find a record")
	}
	if _, ok := rec.ExpiresAt(); ok {
		t.Fatal("ExpiresAt must report false for a zero lifetime")
	}
}

func TestLoadMissReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if _, ok := s.Load("absent"); ok {
		t.Fatal("Load of an absent record must report false")
	}
}

func TestSaveOverwritesPreviousRecord(t *testing.T) {
	dir := t.TempDir()
	first := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	second := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	now := first
	s := Store{Dir: dir, Now: func() time.Time { return now }}

	if err := s.Save("id_rsa", time.Hour); err != nil {
		t.Fatalf("Save: %v", err)
	}
	now = second
	if err := s.Save("id_rsa", 2*time.Hour); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rec, ok := s.Load("id_rsa")
	if !ok {
		t.Fatal("Load after re-Save must find a record")
	}
	if !rec.AddedAt.Equal(second) {
		t.Fatalf("AddedAt = %v, want %v (the re-add time)", rec.AddedAt, second)
	}
	if rec.Lifetime != 2*time.Hour {
		t.Fatalf("Lifetime = %v, want 2h", rec.Lifetime)
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if err := s.Save("id_rsa", time.Hour); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Clear("id_rsa"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, ok := s.Load("id_rsa"); ok {
		t.Fatal("after Clear, Load must miss")
	}
	if err := s.Clear("absent"); err != nil {
		t.Fatalf("Clear of an absent record must not error: %v", err)
	}
}

func TestEmptyDirDisables(t *testing.T) {
	s := Store{}
	if err := s.Save("id_rsa", time.Hour); err != nil {
		t.Fatalf("Save on a disabled store: %v", err)
	}
	if _, ok := s.Load("id_rsa"); ok {
		t.Fatal("a disabled store must never find a record")
	}
	if err := s.Clear("id_rsa"); err != nil {
		t.Fatalf("Clear on a disabled store: %v", err)
	}
}

func TestMalformedRecordMisses(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	p := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(p, []byte("not-a-timestamp\nnot-a-number\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, ok := s.Load("id_rsa"); ok {
		t.Fatal("a malformed record must miss, not report a zero-value record")
	}
}

func TestPathEscapeContained(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if err := s.Save("../escape", time.Hour); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "escape" {
		t.Fatalf("expected exactly one file named 'escape' (base of '../escape') inside Dir, got %v", entries)
	}
}
