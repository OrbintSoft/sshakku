//go:build windows

package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFromOS covers reading the layout inputs from the environment here: the
// XDG variables are read as they are everywhere, the owner is the one this
// system cannot name, and a temporary directory that was named is dropped —
// nothing here can say whose it is, and a socket is not put somewhere the
// answer is unknown.
func TestFromOS(t *testing.T) {
	t.Run("reads the environment", func(t *testing.T) {
		t.Setenv("HOME", `C:\Users\alice`)
		t.Setenv("XDG_CONFIG_HOME", `C:\Users\alice\.config`)
		t.Setenv("XDG_STATE_HOME", `C:\Users\alice\.local\state`)
		t.Setenv("XDG_RUNTIME_DIR", `C:\Users\alice\.run`)
		t.Setenv("XDG_CACHE_HOME", `C:\Users\alice\.cache`)

		env := FromOS()
		assert.Equal(t, `C:\Users\alice`, env.Home, "Home")
		assert.Equal(t, `C:\Users\alice\.config`, env.ConfigHome, "ConfigHome")
		assert.Equal(t, `C:\Users\alice\.local\state`, env.StateHome, "StateHome")
		assert.Equal(t, `C:\Users\alice\.run`, env.RuntimeDir, "RuntimeDir")
		assert.Equal(t, `C:\Users\alice\.cache`, env.CacheHome, "CacheHome")
		assert.Equal(t, -1, env.UID,
			"this system names an owner by SID, so there is no uid to report and the rest of the code reads -1 as an unknown owner")
	})

	t.Run("empty HOME takes the fallback path", func(t *testing.T) {
		t.Setenv("HOME", "")
		// os.UserHomeDir reads the account's own profile directory here rather
		// than $HOME, so the fallback yields a home even with HOME empty.
		env := FromOS()
		assert.NotEmpty(t, env.Home, "the account's own profile directory answers when HOME does not")
	})

	t.Run("a temporary directory that was named is still dropped", func(t *testing.T) {
		t.Setenv("TMPDIR", t.TempDir())
		assert.Empty(t, FromOS().TempDir,
			"nothing here can tell whether a directory is this user's alone, so none is carried forward")
	})
}

// TestProbeDir covers the directory probe: a real directory passes and a file or
// a missing path does not. The ownership question is refused rather than
// guessed at — an owner is a SID and access is an ACL, neither of which a uid
// can name — so requireOwner answers no for a directory that is plainly there.
func TestProbeDir(t *testing.T) {
	dir := t.TempDir()

	assert.True(t, ProbeDir(dir, false), "a real directory passes")
	assert.False(t, ProbeDir(dir, true), "the ownership question has no answer here, so it is not answered yes")

	file := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	assert.False(t, ProbeDir(file, false), "a plain file is not a directory")
	assert.False(t, ProbeDir(filepath.Join(dir, "missing"), false), "a missing path fails")
}

// TestProbeDirAs covers the same probe asked on another account's behalf: the
// uid changes nothing, since it names nobody on this system.
func TestProbeDirAs(t *testing.T) {
	dir := t.TempDir()

	assert.True(t, ProbeDirAs(1000)(dir, false), "a real directory passes whatever uid was asked about")
	assert.False(t, ProbeDirAs(1000)(dir, true), "and the ownership question is still refused")
	assert.False(t, ProbeDirAs(1000)(filepath.Join(dir, "missing"), false), "a missing path fails")
}

// TestPrivateDir covers the question asked of a directory before anything of
// ours goes in it. Here it is a question about an ACL and an owner's SID, which
// this build does not read, so the answer is no even for a directory this test
// has just made — the caller then leaves it out rather than putting a socket
// somewhere somebody else may be able to wait at.
func TestPrivateDir(t *testing.T) {
	assert.False(t, PrivateDir(t.TempDir()),
		"a directory nobody read the ACL of is not known to be private, so it is not treated as private")
}
