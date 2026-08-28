package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errChmodBoom is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errChmodBoom = errors.New("chmod boom")

// TestEnsureCreatesLayout covers what Ensure has to leave behind on whichever
// system is running it: every directory of the layout, and the log file inside
// one of them. What those are permissioned to is a question only some systems
// can be asked, and is asked there.
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
		assert.Truef(t, fi.IsDir(), "%s must be a directory", dir)
	}
	fi, err := os.Stat(l.LogFile)
	require.NoError(t, err, "stat log")
	assert.True(t, fi.Mode().IsRegular(), "the log must be a regular file")
}

// TestEnsureReportsADirectoryItCannotMake covers the failure Ensure stops at: a
// directory of the layout that cannot be created is reported rather than skipped,
// since what comes after it would then be writing into a layout that is not there.
func TestEnsureReportsADirectoryItCannotMake(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, nil, 0o600))

	state := filepath.Join(root, "state")
	l := Layout{
		// A plain file stands where this directory's parent would have to be, so
		// it is the first thing Ensure tries and the only thing it can object to.
		ConfigDir:  filepath.Join(blocked, "sshakku"),
		StateDir:   state,
		RuntimeDir: filepath.Join(root, "run"),
		SocketDir:  filepath.Join(root, "run"),
		LogFile:    filepath.Join(state, "sessions.log"),
	}
	assert.Error(t, Ensure(l), "Ensure must report a layout directory it could not make")
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

// TestEnsureDirErrors covers ensureDir's failure branches: a parent that is a
// plain file makes MkdirAll fail, and an injected chmod that fails makes the
// permission step fail even though the directory was created.
func TestEnsureDirErrors(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notadir")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	assert.Error(t, ensureDir(filepath.Join(file, "child"), os.Chmod), "ensureDir under a file must fail")

	failChmod := func(string, os.FileMode) error { return errChmodBoom }
	assert.Error(t, ensureDir(filepath.Join(root, "d"), failChmod), "ensureDir with a failing chmod must fail")
}

// TestEnsureFileErrors covers ensureFile's failure branches: opening a path that
// is an existing directory fails, and an injected chmod that fails makes the
// permission step fail even though the file was created.
func TestEnsureFileErrors(t *testing.T) {
	dir := t.TempDir()
	assert.Error(t, ensureFile(dir, 0o600, os.Chmod), "ensureFile on a directory must fail")

	failChmod := func(string, os.FileMode) error { return errChmodBoom }
	assert.Error(t, ensureFile(filepath.Join(dir, "f"), 0o600, failChmod), "ensureFile with a failing chmod must fail")
}
