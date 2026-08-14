package keystate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	s := Store{Dir: dir, Now: func() time.Time { return now }}

	require.NoError(t, s.Save("id_rsa", 8*time.Hour), "Save")
	rec, ok := s.Load("id_rsa")
	require.True(t, ok, "Load after Save must find a record")
	assert.Truef(t, rec.AddedAt.Equal(now), "AddedAt = %v, want %v", rec.AddedAt, now)
	assert.Equal(t, 8*time.Hour, rec.Lifetime, "Lifetime")
	expiresAt, ok := rec.ExpiresAt()
	require.True(t, ok, "ExpiresAt must report ok for a non-zero lifetime")
	want := now.Add(8 * time.Hour)
	assert.Truef(t, expiresAt.Equal(want), "ExpiresAt = %v, want %v", expiresAt, want)
}

func TestZeroLifetimeNeverExpires(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	require.NoError(t, s.Save("id_rsa", 0), "Save")
	rec, ok := s.Load("id_rsa")
	require.True(t, ok, "Load after Save must find a record")
	_, ok = rec.ExpiresAt()
	assert.False(t, ok, "ExpiresAt must report false for a zero lifetime")
}

func TestLoadMissReturnsFalse(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	_, ok := s.Load("absent")
	assert.False(t, ok, "Load of an absent record must report false")
}

func TestSaveOverwritesPreviousRecord(t *testing.T) {
	dir := t.TempDir()
	first := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	second := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	now := first
	s := Store{Dir: dir, Now: func() time.Time { return now }}

	require.NoError(t, s.Save("id_rsa", time.Hour), "Save")
	now = second
	require.NoError(t, s.Save("id_rsa", 2*time.Hour), "re-Save")
	rec, ok := s.Load("id_rsa")
	require.True(t, ok, "Load after re-Save must find a record")
	assert.Truef(t, rec.AddedAt.Equal(second), "AddedAt = %v, want %v (the re-add time)", rec.AddedAt, second)
	assert.Equal(t, 2*time.Hour, rec.Lifetime, "Lifetime")
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	require.NoError(t, s.Save("id_rsa", time.Hour), "Save")
	require.NoError(t, s.Clear("id_rsa"), "Clear")
	_, ok := s.Load("id_rsa")
	assert.False(t, ok, "after Clear, Load must miss")
	assert.NoError(t, s.Clear("absent"), "Clear of an absent record must not error")
}

func TestEmptyDirDisables(t *testing.T) {
	s := Store{}
	assert.NoError(t, s.Save("id_rsa", time.Hour), "Save on a disabled store")
	_, ok := s.Load("id_rsa")
	assert.False(t, ok, "a disabled store must never find a record")
	assert.NoError(t, s.Clear("id_rsa"), "Clear on a disabled store")
}

func TestMalformedRecordMisses(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	p := filepath.Join(dir, "id_rsa")
	require.NoError(t, os.WriteFile(p, []byte("not-a-timestamp\nnot-a-number\n"), 0o600), "seed")
	_, ok := s.Load("id_rsa")
	assert.False(t, ok, "a malformed record must miss, not report a zero-value record")
}

func TestPathEscapeContained(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	require.NoError(t, s.Save("../escape", time.Hour), "Save")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "ReadDir")
	require.Len(t, entries, 1, "exactly one file must be written inside Dir")
	assert.Equal(t, "escape", entries[0].Name(), "the record is named after the base of '../escape'")
}
