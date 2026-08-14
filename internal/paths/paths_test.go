package paths

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRuntimeDir(t *testing.T) {
	// The directories handed to Resolve, and probed for, spelled the way the
	// system running this spells a path. What is asserted below is then the
	// path made of these components rather than one system's spelling of it.
	home := filepath.FromSlash("/home/u")
	runUser := filepath.FromSlash("/run/user/1000")
	tempDir := filepath.FromSlash("/tmp/private")
	cacheHome := filepath.FromSlash("/cache")
	tests := []struct {
		name     string
		env      Env
		probe    func(string, bool) bool
		wantBase string
	}{
		{
			name:     "XDG_RUNTIME_DIR present",
			env:      Env{Home: home, RuntimeDir: runUser, UID: 1000},
			probe:    func(p string, _ bool) bool { return p == runUser },
			wantBase: filepath.Join(runUser, "sshakku"),
		},
		{
			name:     "fallback to /run/user/UID when owned",
			env:      Env{Home: home, UID: 1000},
			probe:    func(p string, owner bool) bool { return p == runUser && owner },
			wantBase: filepath.Join(runUser, "sshakku"),
		},
		{
			name:     "/run/user ignored when not owned by us",
			env:      Env{Home: home, UID: 1000},
			probe:    func(p string, owner bool) bool { return p == runUser && !owner },
			wantBase: filepath.Join(home, ".cache", "sshakku"),
		},
		{
			name:     "cache fallback when no tmpfs",
			env:      Env{Home: home, UID: 1000},
			probe:    func(string, bool) bool { return false },
			wantBase: filepath.Join(home, ".cache", "sshakku"),
		},
		{
			// Where there is no logind directory the private temporary one is
			// taken before the home: a socket address is bounded and a home
			// directory is not, so a home deep enough leaves a session with no
			// agent it can reach at all.
			name:     "the private temporary directory comes before the home",
			env:      Env{Home: home, TempDir: tempDir, UID: 1000},
			probe:    func(string, bool) bool { return false },
			wantBase: filepath.Join(tempDir, "sshakku"),
		},
		{
			name:     "a logind directory still wins over the temporary one",
			env:      Env{Home: home, RuntimeDir: runUser, TempDir: tempDir, UID: 1000},
			probe:    func(p string, _ bool) bool { return p == runUser },
			wantBase: filepath.Join(runUser, "sshakku"),
		},
		{
			name:     "XDG_CACHE_HOME honoured in cache fallback",
			env:      Env{Home: home, CacheHome: cacheHome, UID: 1000},
			probe:    func(string, bool) bool { return false },
			wantBase: filepath.Join(cacheHome, "sshakku"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.env, tc.probe)
			assert.Equal(t, tc.wantBase, got.RuntimeDir, "RuntimeDir")
			assert.Equal(t, filepath.Join(tc.wantBase, "agent.sock"), got.AgentSock, "AgentSock")
			assert.Equal(t, filepath.Join(tc.wantBase, ".start.lock"), got.AgentLock, "AgentLock")
		})
	}
}

func TestWithSocketToken(t *testing.T) {
	runUser := filepath.FromSlash("/run/user/1")
	base := Resolve(Env{Home: filepath.FromSlash("/h"), RuntimeDir: runUser, UID: 1},
		func(p string, _ bool) bool { return p == runUser })
	require.Equal(t, filepath.Join(runUser, "sshakku"), base.SocketDir, "base SocketDir")

	got := base.WithSocketToken("deadbeef")
	socketDir := filepath.Join(runUser, "sshakku", "deadbeef")
	assert.Equal(t, socketDir, got.SocketDir, "SocketDir")
	assert.Equal(t, filepath.Join(socketDir, "agent.sock"), got.AgentSock, "AgentSock")
	assert.Equal(t, filepath.Join(socketDir, ".start.lock"), got.AgentLock, "AgentLock")
	assert.Equal(t, base.RuntimeDir, got.RuntimeDir, "RuntimeDir must not change")
	assert.Equal(t, base, base.WithSocketToken(""), "an empty token leaves the layout unchanged")
}

func TestResolveConfigDir(t *testing.T) {
	noProbe := func(string, bool) bool { return false }

	home := filepath.FromSlash("/home/u")
	configHome := filepath.FromSlash("/cfg")

	got := Resolve(Env{Home: home, UID: 1}, noProbe)
	assert.Equal(t, filepath.Join(home, ".config", "sshakku"), got.ConfigDir, "ConfigDir")

	got = Resolve(Env{Home: home, ConfigHome: configHome, UID: 1}, noProbe)
	assert.Equal(t, filepath.Join(configHome, "sshakku"), got.ConfigDir, "ConfigDir with XDG_CONFIG_HOME")
}

func TestResolveStateDir(t *testing.T) {
	noProbe := func(string, bool) bool { return false }

	home := filepath.FromSlash("/home/u")
	stateHome := filepath.FromSlash("/state")

	got := Resolve(Env{Home: home, UID: 1}, noProbe)
	stateDir := filepath.Join(home, ".local", "state", "sshakku")
	assert.Equal(t, stateDir, got.StateDir, "StateDir")
	assert.Equal(t, filepath.Join(stateDir, "sessions.log"), got.LogFile, "LogFile")

	got = Resolve(Env{Home: home, StateHome: stateHome, UID: 1}, noProbe)
	assert.Equal(t, filepath.Join(stateHome, "sshakku"), got.StateDir, "StateDir with XDG_STATE_HOME")
	assert.Equal(t, filepath.Join(stateHome, "sshakku", "sessions.log"), got.LogFile, "LogFile with XDG_STATE_HOME")
}
