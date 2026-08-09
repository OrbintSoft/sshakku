package giveup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordAndGivenUp(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir, TTL: time.Hour}
	assert.False(t, s.GivenUp("id_rsa"), "a fresh store must not report given up")
	require.NoError(t, s.Record("id_rsa"), "Record")
	assert.True(t, s.GivenUp("id_rsa"), "after Record the key must be given up")
	info, err := os.Stat(filepath.Join(dir, "id_rsa"))
	require.NoError(t, err, "stat sentinel")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "sentinel permissions")
}

func TestGivenUpExpiresAfterTTL(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	s := Store{Dir: dir, TTL: time.Hour, Now: func() time.Time { return now }}
	require.NoError(t, s.Record("id_rsa"), "Record")
	now = now.Add(2 * time.Hour)
	assert.False(t, s.GivenUp("id_rsa"), "an expired record must not report given up")
	_, err := os.Stat(filepath.Join(dir, "id_rsa"))
	assert.ErrorIs(t, err, os.ErrNotExist, "an expired sentinel must be removed")
}

func TestZeroTTLNeverExpires(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	s := Store{Dir: dir, TTL: 0, Now: func() time.Time { return now }}
	require.NoError(t, s.Record("id_rsa"), "Record")
	now = now.Add(1000 * time.Hour)
	assert.True(t, s.GivenUp("id_rsa"), "with TTL<=0 a record must never expire")
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir, TTL: time.Hour}
	require.NoError(t, s.Record("id_rsa"), "Record")
	require.NoError(t, s.Clear("id_rsa"), "Clear")
	assert.False(t, s.GivenUp("id_rsa"), "after Clear the key must not be given up")
	assert.NoError(t, s.Clear("absent"), "Clear of an absent record must not error")
}

func TestEmptyDirDisables(t *testing.T) {
	s := Store{}
	assert.NoError(t, s.Record("id_rsa"), "Record on a disabled store")
	assert.False(t, s.GivenUp("id_rsa"), "a disabled store must never report given up")
	assert.NoError(t, s.Clear("id_rsa"), "Clear on a disabled store")
}

func TestMalformedTimestampDropped(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir, TTL: time.Hour}
	p := filepath.Join(dir, "id_rsa")
	require.NoError(t, os.WriteFile(p, []byte("not-a-timestamp\n"), 0o600), "seed")
	assert.False(t, s.GivenUp("id_rsa"), "a malformed record must not report given up")
	_, err := os.Stat(p)
	assert.ErrorIs(t, err, os.ErrNotExist, "a malformed sentinel must be removed")
}
