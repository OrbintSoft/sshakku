//go:build unix

package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnsureLeavesItsOwnDirectoriesPrivate covers the permissions Ensure forces
// on the layout it creates — 0700 for its own leaf directories, 0600 for the
// log — which is a question about a directory only a system with these bits can
// be asked.
func TestEnsureLeavesItsOwnDirectoriesPrivate(t *testing.T) {
	root := t.TempDir()
	runtime := filepath.Join(root, "run", "sshakku")
	config := filepath.Join(root, "cfg", "sshakku")
	state := filepath.Join(root, "state", "sshakku")
	l := Layout{
		ConfigDir:  config,
		StateDir:   state,
		RuntimeDir: runtime,
		SocketDir:  runtime,
		AgentSock:  filepath.Join(runtime, "agent.sock"),
		AgentLock:  filepath.Join(runtime, ".start.lock"),
		LogFile:    filepath.Join(state, "sessions.log"),
	}
	require.NoError(t, Ensure(l), "Ensure")
	for _, dir := range []string{config, state, runtime} {
		fi, err := os.Stat(dir)
		require.NoErrorf(t, err, "stat %s", dir)
		assert.Equalf(t, os.FileMode(0o700), fi.Mode().Perm(), "%s permissions", dir)
	}
	fi, err := os.Stat(l.LogFile)
	require.NoError(t, err, "stat log")
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "log permissions")
}

func TestEnsureRejectsSymlinkDir(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "elsewhere")
	require.NoError(t, os.Mkdir(target, 0o700))
	link := filepath.Join(root, "cfg")
	require.NoError(t, os.Symlink(target, link))
	// Every other path is filled in and creatable, so the symlinked ConfigDir is
	// the only thing Ensure can object to.
	state := filepath.Join(root, "state")
	l := Layout{
		ConfigDir:  link,
		StateDir:   state,
		RuntimeDir: filepath.Join(root, "run"),
		SocketDir:  filepath.Join(root, "run"),
		LogFile:    filepath.Join(state, "sessions.log"),
	}
	assert.Error(t, Ensure(l), "Ensure must reject a symlinked leaf directory")
}

// TestFromOS covers reading the layout inputs from the environment, including
// the reported UID and the fallback when HOME is unset.
func TestFromOS(t *testing.T) {
	t.Run("reads the environment", func(t *testing.T) {
		t.Setenv("HOME", "/home/alice")
		t.Setenv("XDG_CONFIG_HOME", "/home/alice/.config")
		t.Setenv("XDG_STATE_HOME", "/home/alice/.local/state")
		t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
		t.Setenv("XDG_CACHE_HOME", "/home/alice/.cache")

		env := FromOS()
		assert.Equal(t, "/home/alice", env.Home, "Home")
		assert.Equal(t, "/home/alice/.config", env.ConfigHome, "ConfigHome")
		assert.Equal(t, "/home/alice/.local/state", env.StateHome, "StateHome")
		assert.Equal(t, "/run/user/1000", env.RuntimeDir, "RuntimeDir")
		assert.Equal(t, "/home/alice/.cache", env.CacheHome, "CacheHome")
		assert.Equal(t, os.Getuid(), env.UID, "UID")
	})

	t.Run("empty HOME takes the fallback path", func(t *testing.T) {
		t.Setenv("HOME", "")
		// os.UserHomeDir also reads $HOME on unix, so with it empty the fallback
		// yields no home either; the point is that the guarded branch runs
		// without panicking and leaves Home defined by whatever it resolves to.
		env := FromOS()
		assert.Equal(t, os.Getuid(), env.UID, "UID")
	})
}

// TestProbeDir covers the probe both questions go through: a real directory
// passes, a plain file and a missing path fail, and requirePrivate asks not
// only who owns the directory but whether anybody else was let into it. The
// second half is not decoration — renaming and unlinking an entry are governed
// by the directory, so a directory its owner opened to the group or to everyone
// is one somebody else can swap our socket out of, whatever the socket's own
// mode says.
func TestProbeDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700), "a directory we have to ourselves") //nolint:gosec // G302: the mode is what is being asked about

	assert.True(t, ProbeDir(dir, false), "a real directory passes")
	assert.True(t, ProbeDir(dir, true), "a real directory we have to ourselves passes")

	file := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	assert.False(t, ProbeDir(file, false), "a plain file is not a directory")
	assert.False(t, ProbeDir(filepath.Join(dir, "missing"), false), "a missing path fails")

	// A uid that cannot own the temp dir must fail the ownership check.
	assert.False(t, ProbeDirAs(os.Getuid()+99999)(dir, true), "another uid does not own it")
	// Without requirePrivate the same probe asks only whether it is there.
	assert.True(t, ProbeDirAs(os.Getuid()+99999)(dir, false), "without requirePrivate only existence is asked")

	// Ours, and open to everybody: owning it is not having it to ourselves.
	for _, mode := range []os.FileMode{0o750, 0o770, 0o755, 0o777} {
		require.NoError(t, os.Chmod(dir, mode), "reopen the directory to others")
		assert.Falsef(t, ProbeDir(dir, true), "a %o directory of ours is not one we have to ourselves", mode)
		assert.Truef(t, ProbeDir(dir, false), "a %o directory is still plainly there", mode)
	}
}

// TestPrivateDir covers the question asked of a directory somebody else named:
// is it ours alone? Everything that would let another user in — a mode that
// grants them anything, a symlink pointing who knows where, something that is
// not a directory at all — has to answer no, since what goes in such a
// directory is a passphrase waiting to be collected.
func TestPrivateDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700)) //nolint:gosec // G302 flags the mode this test hands PrivateDir to judge; the subject is the judging
	assert.True(t, PrivateDir(dir), "a 0700 directory of ours is private")

	for _, mode := range []os.FileMode{0o770, 0o707, 0o750, 0o705, 0o777} {
		require.NoError(t, os.Chmod(dir, mode))
		assert.Falsef(t, PrivateDir(dir), "a %o directory is not private", mode)
	}
	require.NoError(t, os.Chmod(dir, 0o700)) //nolint:gosec // G302 cannot tell a directory from a file: 0o700 is the tightest a directory entered by its owner can be

	file := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	assert.False(t, PrivateDir(file), "a plain file is not a private directory")
	assert.False(t, PrivateDir(filepath.Join(dir, "missing")), "a missing path is not a private directory")

	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(dir, link))
	assert.False(t, PrivateDir(link), "a symlink to a private directory is not private")
}
