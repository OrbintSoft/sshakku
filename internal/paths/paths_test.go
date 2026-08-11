package paths

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRuntimeDir(t *testing.T) {
	const home = "/home/u"
	tests := []struct {
		name     string
		env      Env
		probe    func(string, bool) bool
		wantBase string
	}{
		{
			name:     "XDG_RUNTIME_DIR present",
			env:      Env{Home: home, RuntimeDir: "/run/user/1000", UID: 1000},
			probe:    func(p string, _ bool) bool { return p == "/run/user/1000" },
			wantBase: "/run/user/1000/sshakku",
		},
		{
			name:     "fallback to /run/user/UID when owned",
			env:      Env{Home: home, UID: 1000},
			probe:    func(p string, owner bool) bool { return p == "/run/user/1000" && owner },
			wantBase: "/run/user/1000/sshakku",
		},
		{
			name:     "/run/user ignored when not owned by us",
			env:      Env{Home: home, UID: 1000},
			probe:    func(p string, owner bool) bool { return p == "/run/user/1000" && !owner },
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
			env:      Env{Home: home, TempDir: "/tmp/private", UID: 1000},
			probe:    func(string, bool) bool { return false },
			wantBase: "/tmp/private/sshakku",
		},
		{
			name:     "a logind directory still wins over the temporary one",
			env:      Env{Home: home, RuntimeDir: "/run/user/1000", TempDir: "/tmp/private", UID: 1000},
			probe:    func(p string, _ bool) bool { return p == "/run/user/1000" },
			wantBase: "/run/user/1000/sshakku",
		},
		{
			name:     "XDG_CACHE_HOME honoured in cache fallback",
			env:      Env{Home: home, CacheHome: "/cache", UID: 1000},
			probe:    func(string, bool) bool { return false },
			wantBase: "/cache/sshakku",
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
	base := Resolve(Env{Home: "/h", RuntimeDir: "/run/user/1", UID: 1},
		func(p string, _ bool) bool { return p == "/run/user/1" })
	require.Equal(t, "/run/user/1/sshakku", base.SocketDir, "base SocketDir")

	got := base.WithSocketToken("deadbeef")
	assert.Equal(t, "/run/user/1/sshakku/deadbeef", got.SocketDir, "SocketDir")
	assert.Equal(t, "/run/user/1/sshakku/deadbeef/agent.sock", got.AgentSock, "AgentSock")
	assert.Equal(t, "/run/user/1/sshakku/deadbeef/.start.lock", got.AgentLock, "AgentLock")
	assert.Equal(t, base.RuntimeDir, got.RuntimeDir, "RuntimeDir must not change")
	assert.Equal(t, base, base.WithSocketToken(""), "an empty token leaves the layout unchanged")
}

func TestResolveConfigDir(t *testing.T) {
	noProbe := func(string, bool) bool { return false }

	got := Resolve(Env{Home: "/home/u", UID: 1}, noProbe)
	assert.Equal(t, "/home/u/.config/sshakku", got.ConfigDir, "ConfigDir")

	got = Resolve(Env{Home: "/home/u", ConfigHome: "/cfg", UID: 1}, noProbe)
	assert.Equal(t, "/cfg/sshakku", got.ConfigDir, "ConfigDir with XDG_CONFIG_HOME")
}

func TestResolveStateDir(t *testing.T) {
	noProbe := func(string, bool) bool { return false }

	got := Resolve(Env{Home: "/home/u", UID: 1}, noProbe)
	assert.Equal(t, "/home/u/.local/state/sshakku", got.StateDir, "StateDir")
	assert.Equal(t, "/home/u/.local/state/sshakku/sessions.log", got.LogFile, "LogFile")

	got = Resolve(Env{Home: "/home/u", StateHome: "/state", UID: 1}, noProbe)
	assert.Equal(t, "/state/sshakku", got.StateDir, "StateDir with XDG_STATE_HOME")
	assert.Equal(t, "/state/sshakku/sessions.log", got.LogFile, "LogFile with XDG_STATE_HOME")
}
