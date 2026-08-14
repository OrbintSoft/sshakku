package main

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRun exercises argument dispatch only. shell-init and ensure-agent are
// omitted: both now drive the real agent lifecycle (start, reap, adopt), so
// invoking them here would spawn and reap agents on the test host. doctor is
// omitted for a milder version of the same reason — it reads the host's real
// /proc and probes live sockets. That logic is covered by the agent and diagnose
// package tests.
func TestRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no args", nil, 2},
		{"help", []string{"help"}, 0},
		{"help flag", []string{"--help"}, 0},
		{"unknown command", []string{"bogus"}, 2},
		{"doctor unknown flag", []string{"doctor", "--bogus"}, 2},
		{"forget no args", []string{"forget"}, 2},
		{"forget --all with names", []string{"forget", "--all", "id_rsa"}, 2},
		{"internal read socket token", []string{internalReadSocketTokenCmd}, 0},
		{"doctor --user missing value", []string{"doctor", "--user"}, 2},
		{"doctor --user unknown", []string{"doctor", "--user", "sshakku-test-no-such-user"}, 2},
		{"doctor --test-backend unknown name", []string{"doctor", "--test-backend", "bogus"}, 2},
		{"askpass-env unknown dialect", []string{"askpass-env", "--shell=fish"}, 2},
		{"askpass-env unknown argument", []string{"askpass-env", "--posix"}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equalf(t, tc.want, realDeps().run(t.Context(), io.Discard, io.Discard, tc.args),
				"the exit status for %q", tc.args)
		})
	}
}

// TestEveryChoosableWalletCanBeDiagnosed needs this system to have a wallet at
// all, so it is in walletcheck_unix_test.go beside the other wallet tests.

func TestResolveTargetUser(t *testing.T) {
	self, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	selfUID := os.Getuid()
	// resolveTargetUser answers two things: whose files to look at, and whether
	// that is somebody other than the caller — Source is empty for the caller
	// and names how the other user was arrived at otherwise.

	t.Run("no --user, not root: self, no lookup needed", func(t *testing.T) {
		t.Setenv("SUDO_UID", "")
		got, err := resolveTargetUser("", paths.Env{UID: selfUID})
		require.NoError(t, err, "resolveTargetUser")
		assert.Equal(t, selfUID, got.UID, "the caller's own uid")
		assert.Empty(t, got.Source, "nothing cross-user happened")
	})

	t.Run("--user names the invoking user: still self", func(t *testing.T) {
		got, err := resolveTargetUser(self.Username, paths.Env{UID: selfUID})
		require.NoError(t, err, "resolveTargetUser")
		assert.Equal(t, selfUID, got.UID, "the caller's own uid")
		assert.Empty(t, got.Source, "naming yourself is not going cross-user")
	})

	t.Run("--user names someone else: cross-user, regardless of who's actually invoking", func(t *testing.T) {
		// selfEnv.UID is deliberately a uid nothing resolves to, so this exercises
		// the "different from invoker" branch without depending on whether the test
		// process happens to be root.
		got, err := resolveTargetUser(self.Username, paths.Env{UID: -1})
		require.NoError(t, err, "resolveTargetUser")
		assert.Equal(t, selfUID, got.UID, "the uid of the user named")
		assert.NotEmpty(t, got.Source, "a target that is not the caller must say how it was arrived at")
	})

	t.Run("unknown --user value errors", func(t *testing.T) {
		_, err := resolveTargetUser("sshakku-test-no-such-user", paths.Env{UID: selfUID})
		assert.Error(t, err, "a user nobody can resolve must be reported, not silently taken for the caller")
	})

	t.Run("SUDO_UID auto-detected only when invoking as root", func(t *testing.T) {
		if selfUID == 0 {
			// The test process itself is root (e.g. a container test run), so
			// there's no non-root uid left to fake as SUDO_UID: a real sudo
			// invocation never sets SUDO_UID=0, and resolveTargetUser correctly
			// treats a lookup that resolves back to uid 0 as "no cross-user
			// target", the very thing this subtest exists to rule out.
			t.Skip("test process is already root: can't fake a distinct non-root SUDO_UID")
		}
		t.Setenv("SUDO_UID", strconv.Itoa(selfUID))
		got, err := resolveTargetUser("", paths.Env{UID: 0})
		require.NoError(t, err, "resolveTargetUser")
		assert.Equal(t, selfUID, got.UID, "the uid sudo recorded")
		assert.NotEmpty(t, got.Source, "a target arrived at through SUDO_UID must say so")
	})

	t.Run("SUDO_UID ignored when not invoking as root", func(t *testing.T) {
		if selfUID == 0 {
			t.Skip("test process is already root: can't fake a distinct non-root SUDO_UID")
		}
		// A SUDO_UID naming somebody else, so honouring it and ignoring it lead
		// to different answers. Set to the caller's own uid the two are the
		// same, and the check passes whichever the code does. Root is the one
		// uid that resolves on every system this runs on.
		t.Setenv("SUDO_UID", "0")
		got, err := resolveTargetUser("", paths.Env{UID: selfUID})
		require.NoError(t, err, "resolveTargetUser")
		assert.Equal(t, selfUID, got.UID, "the caller stays the target when they did not come through sudo")
		assert.Empty(t, got.Source, "SUDO_UID means nothing when the caller is not root")
	})

	t.Run("malformed SUDO_UID as root errors", func(t *testing.T) {
		// As root with a non-numeric SUDO_UID, the auto-detect lookup fails and
		// resolveTargetUser reports it rather than silently falling through.
		t.Setenv("SUDO_UID", "not-a-uid-xyzzy")
		_, err := resolveTargetUser("", paths.Env{UID: 0})
		assert.Error(t, err, "a SUDO_UID that resolves to nobody must be reported, not fallen through")
	})
}

func TestCrossUserGuard(t *testing.T) {
	self := targetUser{Source: ""}
	other := targetUser{Source: "the --user flag", UID: 1000, Username: "alice"}

	// Six independent verdicts; assert throughout so one run names every one
	// that went the wrong way.
	assert.Empty(t, crossUserGuard(self, true, false, 0), "acting on your own machine needs no permission")
	assert.Empty(t, crossUserGuard(self, false, false, 1000), "nor does looking at it")
	assert.NotEmpty(t, crossUserGuard(other, true, false, 0), "changing another user's setup must be refused")
	assert.NotEmpty(t, crossUserGuard(other, false, false, 1000), "reading another user's setup takes root")
	assert.Empty(t, crossUserGuard(other, false, false, 0), "root may look at another user's setup")
	assert.NotEmpty(t, crossUserGuard(other, false, true, 0), "probing another user's wallet is not looking, it is acting")
}

// TestAskpassExports pins the exported environment verbatim. Both lines are
// load-bearing, and losing one breaks the wallet refill without breaking
// anything a coarser assertion would notice: SSH_ASKPASS names the helper
// beside the binary rather than the binary itself, and REQUIRE=force is what
// makes ssh consult it at all in a session with no DISPLAY.
func TestAskpassExports(t *testing.T) {
	// The helper's path is derived from the binary's with filepath, so it comes
	// back in this system's own spelling — what is pinned verbatim is the pair
	// of lines, not one system's separator.
	bindir := filepath.FromSlash("/usr/local/bin")
	want := "export SSH_ASKPASS='" + filepath.Join(bindir, "sshakku-askpass") + "'\n" +
		"export SSH_ASKPASS_REQUIRE='force'\n"
	assert.Equal(t, want, askpassExports(dialect(t, shellPosix), filepath.Join(bindir, "sshakku")),
		"the two lines the shell must export, verbatim")
}

// TestDispatchRoutesOnTheNameItWasRunAs covers the one thing that decides
// whether args are a prompt to answer or a command to run. ssh execs the helper
// by the path it was given, which is why a bare name and an absolute path must
// both be recognised; everything else is somebody running the binary itself,
// including the two names it is reached under for its own purposes.
func TestDispatchRoutesOnTheNameItWasRunAs(t *testing.T) {
	tests := []struct {
		invokedAs string
		want      bool
	}{
		{"sshakku-askpass", true},
		{"/usr/local/bin/sshakku-askpass", true},
		{"./sshakku-askpass", true},
		{"sshakku", false},
		{"/usr/local/bin/sshakku", false},
		{"/tmp/go-build123/b001/sshakku.test", false},
		{"sshakku-askpass-backup", false},
	}
	for _, tc := range tests {
		t.Run(tc.invokedAs, func(t *testing.T) {
			assert.Equalf(t, tc.want, filepath.Base(tc.invokedAs) == askpassProgName,
				"whether being run as %q means answering a prompt", tc.invokedAs)
		})
	}
}

// TestLoadSettingsMergesConfigD confirms config.d/*.toml files, in filename
// order, override config.toml — end to end through loadSettings, not just
// the config package's own Merge/LoadDir unit tests.
func TestLoadSettingsMergesConfigD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.toml"), "key_lifetime = \"1h\"\nquiet = true\n")
	confD := filepath.Join(dir, "config.d")
	require.NoError(t, os.MkdirAll(confD, 0o755), "create config.d")
	writeFile(t, filepath.Join(confD, "10-override.toml"), "key_lifetime = \"2h\"\n")

	settings := loadSettings(paths.Layout{ConfigDir: dir}, "test", fakeLogger{})

	assert.Equal(t, 2*time.Hour, settings.KeyLifetime, "a drop-in must win over config.toml")
	assert.True(t, settings.Quiet, "a setting no drop-in mentions keeps config.toml's value")
}

// countingLogger records how many lines were logged, so a test can assert that
// an error-handling branch actually ran.
type countingLogger struct{ n int }

func (c *countingLogger) Log(string, string) error { c.n++; return nil }

// TestLoadSettingsLogsErrors covers loadSettings' three error-logging branches
// at once — a config.toml that fails to load, a malformed config.d drop-in, and
// an unparseable env override — and confirms it still returns usable settings
// with defaults where a value could not be resolved.
func TestLoadSettingsLogsErrors(t *testing.T) {
	dir := t.TempDir()
	// A directory where config.toml should be a file makes config.Load fail.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "config.toml"), 0o755), "make config.toml unreadable as a file")
	// A malformed drop-in makes config.LoadDir report an error.
	confD := filepath.Join(dir, "config.d")
	require.NoError(t, os.MkdirAll(confD, 0o755), "create config.d")
	writeFile(t, filepath.Join(confD, "10-bad.toml"), "this is not = valid = toml")
	// An unparseable env value makes config.Resolve report an error.
	t.Setenv("SSHAKKU_KEY_LIFETIME", "notaduration")

	log := &countingLogger{}
	settings := loadSettings(paths.Layout{ConfigDir: dir}, "test", log)

	assert.GreaterOrEqual(t, log.n, 3, "each of the three failures must be logged, not swallowed")
	// A setting untouched by the errors still resolves to its default.
	// service_prefix rather than secret_backend, because every system has a
	// built-in name for SSHakku's wallet entries and not every system has a
	// wallet: on one with none the latter resolves to nothing, which says
	// something true about that platform and nothing about this branch.
	assert.NotEmpty(t, settings.ServicePrefix, "a run that could not read its config still gets working defaults")
}

func TestTail(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"", 3, ""},
		{"ab", 3, "ab"},
		{"abc", 3, "abc"},
		{"abcdef", 3, "def"},
		{"abcdef", 0, ""},
	}
	for _, tc := range tests {
		assert.Equalf(t, tc.want, tail(tc.s, tc.n), "the last %d characters of %q", tc.n, tc.s)
	}
}

func TestRandomProbeValue(t *testing.T) {
	a, err := randomProbeValue()
	require.NoError(t, err, "randomProbeValue")
	assert.Len(t, a, 32, "sixteen random bytes, hex-encoded")
	_, decodeErr := hex.DecodeString(a)
	assert.NoError(t, decodeErr, "the value must be hex all the way through")

	b, err := randomProbeValue()
	require.NoError(t, err, "randomProbeValue")
	assert.NotEqual(t, a, b, "two probes must not share a value, or one could be mistaken for the other")
}

// TestAskpassEnvHeadless confirms a session with no display server is wired the
// same as a graphical one: the broker reads the wallet, which needs no display,
// and a key that has expired from the agent must be refilled there too. Unlike
// the case in cover_remaining_test.go this one leaves the platform's own
// prompter lookup real, so it also covers it reporting a headless session.
func TestAskpassEnvHeadless(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	d := realDeps()
	d.self = func() (string, error) { return "/opt/sshakku/bin/sshakku", nil }
	var out, errOut bytes.Buffer
	require.Zero(t, d.askpassEnv(&out, &errOut, nil), "a session with no display is still one the broker serves")
	assert.Equal(t, askpassExports(dialect(t, shellPosix), "/opt/sshakku/bin/sshakku"), out.String(),
		"the same exports a graphical session gets")
}

func TestStderrNotifier(t *testing.T) {
	var buf bytes.Buffer
	stderrNotifier{w: &buf}.Notify("hello world")
	assert.Equal(t, "sshakku: hello world\n", buf.String(), "a notice must name the tool it came from")
}

// TestDispatchRoutesToRun covers dispatch's non-askpass branch: run under its
// own name, it must fall through to normal subcommand dispatch. The askpass
// branch is exercised via TestAskpassHandoff.
func TestDispatchRoutesToRun(t *testing.T) {
	assert.Zero(t, dispatch(t.Context(), realDeps(), io.Discard, io.Discard, "/usr/local/bin/sshakku", []string{"help"}),
		"asking for help is not a failure")
	assert.Equal(t, 2, dispatch(t.Context(), realDeps(), io.Discard, io.Discard, "/usr/local/bin/sshakku", nil),
		"running the tool with nothing to do is a usage error")
}

// TestAskpassHandoff covers the SSH_ASKPASS handoff failure branches: a missing
// token returns 1, and askpass routes an unresolvable token to the handoff path,
// which also returns 1. HOME and XDG_STATE_HOME point at a temp dir so the
// session log never touches the real state dir.
func TestAskpassHandoff(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)

	t.Run("missing token", func(t *testing.T) {
		t.Setenv(keys.EnvPassHandoffToken, "")
		assert.Equal(t, 1, realDeps().askpassFromHandoff(t.Context(), io.Discard),
			"with no token there is no prompt to answer, and that must be reported")
	})

	t.Run("unresolvable token routed via askpass", func(t *testing.T) {
		t.Setenv(keys.EnvPassHandoffToken, "sshakku-test-nonexistent-token")
		assert.Equal(t, 1, realDeps().askpass(t.Context(), io.Discard, nil),
			"a token that resolves to nothing must be reported, not answered with an empty passphrase")
	})
}

func TestKeystateDir(t *testing.T) {
	socketDir := filepath.FromSlash("/run/user/1000/sshakku")
	got := keystateDir(paths.Layout{AgentSock: filepath.Join(socketDir, "agent.sock")})
	assert.Equal(t, filepath.Join(socketDir, "keystate"), got,
		"the key records sit beside the socket they describe")
}

func TestCurrentUser(t *testing.T) {
	t.Run("USER set", func(t *testing.T) {
		t.Setenv("USER", "sshakku-test-user")
		assert.Equal(t, "sshakku-test-user", currentUser(), "the shell's own answer is taken as given")
	})

	t.Run("USER empty falls back to a lookup", func(t *testing.T) {
		t.Setenv("USER", "")
		// The fallback resolves the real process owner; on a normal test host
		// that is a non-empty username. If the lookup itself fails (an
		// unresolvable uid), "" is the documented result, so accept either.
		self, err := user.Current()
		if err != nil {
			t.Skipf("user.Current: %v", err)
		}
		assert.Equal(t, self.Username, currentUser(), "with no $USER the process owner is looked up")
	})
}

func TestNewHostSource(t *testing.T) {
	assert.NotNil(t, newHostSource(""), "every OS this builds for must have host checks of its own")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoErrorf(t, os.WriteFile(path, []byte(content), 0o644), "write the fixture file %s", path)
}
