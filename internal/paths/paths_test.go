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
		name        string
		env         Env
		probe       func(string, bool) bool
		wantBase    string
		wantRefused []Refusal
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
			// The directory the environment named is there, and is not this
			// account's alone. It is not used, and it is named: a session whose
			// endpoint quietly moved is one nobody can work back from, and
			// where it would have gone is a directory another account can
			// rename our socket out of.
			name:        "a runtime directory that is there but not ours alone is refused, and named",
			env:         Env{Home: home, RuntimeDir: runUser, UID: 1000},
			probe:       func(p string, private bool) bool { return p == runUser && !private },
			wantBase:    filepath.Join(home, ".cache", "sshakku"),
			wantRefused: []Refusal{{Var: "XDG_RUNTIME_DIR", Path: runUser}},
		},
		{
			// Absent is not refused. A stale variable left over from a session
			// that ended names a directory nobody turned down, and reporting it
			// as somebody else's would send a person looking for an intruder
			// where there is only a path that no longer exists.
			name:     "a runtime directory that is merely absent is not refused",
			env:      Env{Home: home, RuntimeDir: filepath.FromSlash("/run/user/9999"), UID: 1000},
			probe:    func(string, bool) bool { return false },
			wantBase: filepath.Join(home, ".cache", "sshakku"),
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
			assert.Equal(t, tc.wantRefused, got.Refused, "Refused")
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

// TestResolveRefusesDirectoriesThatAreNotThisAccountsOwn covers every directory
// the environment can name. A variable is inherited by any shell that becomes
// another account without opening a session of its own, so each of these can
// arrive pointing at somebody else's home — and what is at stake differs by
// directory rather than being the same worry repeated. The configuration
// decides where passphrases are filed and which command is run to edit it; the
// state directory takes the record of what was done with the keys; the cache
// directory is where the endpoint goes when there is no runtime directory to
// use, and a parent another account owns is one our directory can be renamed
// out of and answered in place of.
func TestResolveRefusesDirectoriesThatAreNotThisAccountsOwn(t *testing.T) {
	home := filepath.FromSlash("/home/u")
	theirs := filepath.FromSlash("/home/them/.config")

	// there reports the directory as present and as not this account's alone,
	// which is the one answer that must change what Resolve picks.
	there := func(p string, private bool) bool { return p == theirs && !private }

	tests := []struct {
		name    string
		env     Env
		got     func(Layout) string
		want    string
		wantVar string
	}{
		{
			name:    "a configuration directory belonging to another account",
			env:     Env{Home: home, ConfigHome: theirs, UID: 1000},
			got:     func(l Layout) string { return l.ConfigDir },
			want:    filepath.Join(home, ".config", "sshakku"),
			wantVar: "XDG_CONFIG_HOME",
		},
		{
			name:    "a state directory belonging to another account",
			env:     Env{Home: home, StateHome: theirs, UID: 1000},
			got:     func(l Layout) string { return l.StateDir },
			want:    filepath.Join(home, ".local", "state", "sshakku"),
			wantVar: "XDG_STATE_HOME",
		},
		{
			name:    "a cache directory belonging to another account",
			env:     Env{Home: home, CacheHome: theirs, UID: 1000},
			got:     func(l Layout) string { return l.RuntimeDir },
			want:    filepath.Join(home, ".cache", "sshakku"),
			wantVar: "XDG_CACHE_HOME",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			layout := Resolve(tc.env, there)
			assert.Equal(t, tc.want, tc.got(layout),
				"nothing of this account's goes in a directory another account may write")
			// Naming it is the other half. A session whose files quietly moved
			// is one nobody can work back from, and the variable has to travel
			// with the path: it is the only part a reader can go and change.
			assert.Equal(t, []Refusal{{Var: tc.wantVar, Path: theirs}}, layout.Refused,
				"what was turned down, and which variable named it")
		})
	}
}

// TestResolveDoesNotRefuseADirectoryThatIsMerelyAbsent keeps a stale variable
// from being reported as an intruder. A path left over from a session that
// ended was turned down by nobody, and a report calling it somebody else's
// would send a person looking for an attacker where there is only a path that
// no longer resolves.
func TestResolveDoesNotRefuseADirectoryThatIsMerelyAbsent(t *testing.T) {
	home := filepath.FromSlash("/home/u")
	gone := filepath.FromSlash("/home/them/.config")

	layout := Resolve(Env{Home: home, ConfigHome: gone, UID: 1000},
		func(string, bool) bool { return false })

	assert.Equal(t, filepath.Join(gone, "sshakku"), layout.ConfigDir,
		"a directory that is not there yet is this session's to create")
	assert.Empty(t, layout.Refused, "nothing was turned down, so there is nothing to report")
}

// TestResolveKeepsADirectoryItCannotAttribute is the half that keeps the
// promise a build can keep. Where ownership cannot be established at all, a
// directory the environment named is used exactly as it was before: an
// unanswered question is not a refusal, and discarding a user's own
// configuration for want of an answer would be the worse failure — and one
// they could do nothing about.
//
// The socket is deliberately not covered by this, and the difference is the
// point: it has somewhere else to go, so doubt costs it a different path and
// nothing more.
func TestResolveKeepsADirectoryItCannotAttribute(t *testing.T) {
	home := filepath.FromSlash("/home/u")
	named := filepath.FromSlash("/cfg")

	// The directory is there, and the ownership question comes back no because
	// this build cannot answer it rather than because the answer is no.
	cannotTell := func(p string, private bool) bool { return p == named && !private }

	layout := Resolve(Env{Home: home, ConfigHome: named, UID: 1000, OwnerUnknowable: true}, cannotTell)

	assert.Equal(t, filepath.Join(named, "sshakku"), layout.ConfigDir,
		"a question this build cannot answer must leave the configuration where it was")
	assert.Empty(t, layout.Refused, "nothing may be reported as a stranger's on a build that cannot tell")
}
