//go:build unix

package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureCreatesLayout(t *testing.T) {
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

func TestCleanupLegacyAgentDir(t *testing.T) {
	home := t.TempDir()
	agent := filepath.Join(home, ".ssh", "agent")
	require.NoError(t, os.MkdirAll(agent, 0o700))
	for _, f := range []string{"ssh-agent.sock", ".start.lock"} {
		require.NoError(t, os.WriteFile(filepath.Join(agent, f), nil, 0o600))
	}
	CleanupLegacyAgentDir(home)
	_, err := os.Stat(agent)
	assert.ErrorIs(t, err, os.ErrNotExist, "the legacy agent dir must be gone")
}

func TestCleanupLegacyAgentDirLeavesForeignFiles(t *testing.T) {
	home := t.TempDir()
	agent := filepath.Join(home, ".ssh", "agent")
	require.NoError(t, os.MkdirAll(agent, 0o700))
	foreign := filepath.Join(agent, "keep-me")
	require.NoError(t, os.WriteFile(foreign, nil, 0o600))
	CleanupLegacyAgentDir(home)
	_, err := os.Stat(foreign)
	assert.NoError(t, err, "a file we did not put there must survive")
}

// TestCleanupLegacyAgentDirEarlyReturns covers the no-op guards: an empty home,
// a home with no ~/.ssh/agent at all, and one where that path is a plain file
// rather than a directory. None must panic or touch the filesystem.
func TestCleanupLegacyAgentDirEarlyReturns(t *testing.T) {
	t.Run("empty home", func(t *testing.T) {
		CleanupLegacyAgentDir("")
	})

	t.Run("no agent dir", func(t *testing.T) {
		CleanupLegacyAgentDir(t.TempDir())
	})

	t.Run("agent path is a file", func(t *testing.T) {
		home := t.TempDir()
		ssh := filepath.Join(home, ".ssh")
		require.NoError(t, os.Mkdir(ssh, 0o700))
		agent := filepath.Join(ssh, "agent")
		require.NoError(t, os.WriteFile(agent, nil, 0o600))
		CleanupLegacyAgentDir(home)
		_, err := os.Stat(agent)
		assert.NoError(t, err, "a plain file at that path must survive")
	})
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

// TestFromEnvHomeFallback drives the branch FromOS cannot reach through the real
// os functions: HOME unset, so the home directory comes from the injected
// homeDir lookup.
func TestFromEnvHomeFallback(t *testing.T) {
	getenv := func(key string) string {
		if key == "HOME" {
			return ""
		}
		return ""
	}
	homeDir := func() (string, error) { return "/fallback/home", nil }
	env := fromEnv(getenv, homeDir, func() int { return 4242 }, func(string) bool { return true })
	assert.Equal(t, "/fallback/home", env.Home, "Home comes from the homeDir fallback")
	assert.Equal(t, 4242, env.UID, "UID")
}

// TestFromEnvTempDir covers the one input that is inspected rather than merely
// read: a temporary directory this user does not have to themselves is not
// carried forward at all, so nothing downstream can put a socket in it by
// mistake.
func TestFromEnvTempDir(t *testing.T) {
	getenv := func(key string) string {
		if key == "TMPDIR" {
			return "/the/tmp"
		}
		return ""
	}
	homeDir := func() (string, error) { return "/home/alice", nil }
	uid := func() int { return 1000 }

	env := fromEnv(getenv, homeDir, uid, func(string) bool { return true })
	assert.Equal(t, "/the/tmp", env.TempDir, "a private temporary directory is kept")

	env = fromEnv(getenv, homeDir, uid, func(string) bool { return false })
	assert.Empty(t, env.TempDir, "a shared temporary directory is dropped")

	env = fromEnv(func(string) string { return "" }, homeDir, uid, func(string) bool {
		assert.Fail(t, "a temporary directory that was never named got inspected")
		return true
	})
	assert.Empty(t, env.TempDir, "no temporary directory was named")
}

// TestProbeDir covers the directory/ownership probe: a real directory passes,
// a plain file and a missing path fail, and requireOwner enforces the uid.
func TestProbeDir(t *testing.T) {
	dir := t.TempDir()

	assert.True(t, ProbeDir(dir, false), "a real directory passes")
	assert.True(t, ProbeDir(dir, true), "a real directory owned by us passes")

	file := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	assert.False(t, ProbeDir(file, false), "a plain file is not a directory")
	assert.False(t, ProbeDir(filepath.Join(dir, "missing"), false), "a missing path fails")

	// A uid that cannot own the temp dir must fail the ownership check.
	assert.False(t, ProbeDirAs(os.Getuid()+99999)(dir, true), "another uid does not own it")
	// Without requireOwner the same probe ignores ownership.
	assert.True(t, ProbeDirAs(os.Getuid()+99999)(dir, false), "without requireOwner ownership is ignored")
}

// TestPrivateDir covers the question asked of a directory somebody else named:
// is it ours alone? Everything that would let another user in — a mode that
// grants them anything, a symlink pointing who knows where, something that is
// not a directory at all — has to answer no, since what goes in such a
// directory is a passphrase waiting to be collected.
func TestPrivateDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	assert.True(t, PrivateDir(dir), "a 0700 directory of ours is private")

	for _, mode := range []os.FileMode{0o770, 0o707, 0o750, 0o705, 0o777} {
		require.NoError(t, os.Chmod(dir, mode))
		assert.Falsef(t, PrivateDir(dir), "a %o directory is not private", mode)
	}
	require.NoError(t, os.Chmod(dir, 0o700))

	file := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	assert.False(t, PrivateDir(file), "a plain file is not a private directory")
	assert.False(t, PrivateDir(filepath.Join(dir, "missing")), "a missing path is not a private directory")

	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(dir, link))
	assert.False(t, PrivateDir(link), "a symlink to a private directory is not private")
}

// TestEnsureDirErrors covers ensureDir's failure branches: a parent that is a
// plain file makes MkdirAll fail, and an injected chmod that fails makes the
// permission step fail even though the directory was created.
func TestEnsureDirErrors(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notadir")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	assert.Error(t, ensureDir(filepath.Join(file, "child"), os.Chmod), "ensureDir under a file must fail")

	failChmod := func(string, os.FileMode) error { return errors.New("chmod boom") }
	assert.Error(t, ensureDir(filepath.Join(root, "d"), failChmod), "ensureDir with a failing chmod must fail")
}

// TestEnsureFileErrors covers ensureFile's failure branches: opening a path that
// is an existing directory fails, and an injected chmod that fails makes the
// permission step fail even though the file was created.
func TestEnsureFileErrors(t *testing.T) {
	dir := t.TempDir()
	assert.Error(t, ensureFile(dir, 0o600, os.Chmod), "ensureFile on a directory must fail")

	failChmod := func(string, os.FileMode) error { return errors.New("chmod boom") }
	assert.Error(t, ensureFile(filepath.Join(dir, "f"), 0o600, failChmod), "ensureFile with a failing chmod must fail")
}
