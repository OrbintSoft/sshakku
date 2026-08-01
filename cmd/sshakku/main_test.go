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

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := realDeps().run(io.Discard, io.Discard, tc.args); got != tc.want {
				t.Errorf("run(%q) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}

// Every name a user may put in secret_backend has to be a name the diagnostics
// accept too: a wallet you can choose but cannot ask about is one you cannot
// diagnose when it stops working. The names come from the one list rather than
// being written out again here, which is what stopped them agreeing before.
func TestEveryChoosableWalletCanBeDiagnosed(t *testing.T) {
	names := config.SecretBackends()
	if len(names) == 0 {
		t.Fatal("this system offers no wallet at all")
	}
	for _, name := range names {
		if !config.SecretBackendAvailable(name) {
			t.Errorf("SecretBackendAvailable(%q) = false, want true for a wallet that can be chosen", name)
		}
	}
	if config.SecretBackendAvailable("bogus") {
		t.Error(`SecretBackendAvailable("bogus") = true, want false`)
	}
	if !config.SecretBackendAvailable(config.DefaultSecretBackend()) {
		t.Errorf("the default wallet %q is not among the ones this system offers", config.DefaultSecretBackend())
	}
}

func TestResolveTargetUser(t *testing.T) {
	self, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	selfUID := os.Getuid()

	t.Run("no --user, not root: self, no lookup needed", func(t *testing.T) {
		t.Setenv("SUDO_UID", "")
		got, err := resolveTargetUser("", paths.Env{UID: selfUID})
		if err != nil {
			t.Fatalf("resolveTargetUser: %v", err)
		}
		if got.UID != selfUID || got.Source != "" {
			t.Errorf("got %+v, want UID=%d Source=\"\"", got, selfUID)
		}
	})

	t.Run("--user names the invoking user: still self", func(t *testing.T) {
		got, err := resolveTargetUser(self.Username, paths.Env{UID: selfUID})
		if err != nil {
			t.Fatalf("resolveTargetUser: %v", err)
		}
		if got.UID != selfUID || got.Source != "" {
			t.Errorf("got %+v, want UID=%d Source=\"\"", got, selfUID)
		}
	})

	t.Run("--user names someone else: cross-user, regardless of who's actually invoking", func(t *testing.T) {
		// selfEnv.UID is deliberately a uid nothing resolves to, so this exercises
		// the "different from invoker" branch without depending on whether the test
		// process happens to be root.
		got, err := resolveTargetUser(self.Username, paths.Env{UID: -1})
		if err != nil {
			t.Fatalf("resolveTargetUser: %v", err)
		}
		if got.UID != selfUID || got.Source == "" {
			t.Errorf("got %+v, want UID=%d and a non-empty Source", got, selfUID)
		}
	})

	t.Run("unknown --user value errors", func(t *testing.T) {
		if _, err := resolveTargetUser("sshakku-test-no-such-user", paths.Env{UID: selfUID}); err == nil {
			t.Error("resolveTargetUser: got nil error for an unknown user")
		}
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
		if err != nil {
			t.Fatalf("resolveTargetUser: %v", err)
		}
		if got.UID != selfUID || got.Source == "" {
			t.Errorf("got %+v, want UID=%d and a non-empty Source", got, selfUID)
		}
	})

	t.Run("SUDO_UID ignored when not invoking as root", func(t *testing.T) {
		if selfUID == 0 {
			t.Skip("test process is already root: can't fake a distinct non-root SUDO_UID")
		}
		t.Setenv("SUDO_UID", strconv.Itoa(selfUID))
		got, err := resolveTargetUser("", paths.Env{UID: selfUID})
		if err != nil {
			t.Fatalf("resolveTargetUser: %v", err)
		}
		if got.Source != "" {
			t.Errorf("got %+v, want Source=\"\" (SUDO_UID should be ignored)", got)
		}
	})

	t.Run("malformed SUDO_UID as root errors", func(t *testing.T) {
		// As root with a non-numeric SUDO_UID, the auto-detect lookup fails and
		// resolveTargetUser reports it rather than silently falling through.
		t.Setenv("SUDO_UID", "not-a-uid-xyzzy")
		if _, err := resolveTargetUser("", paths.Env{UID: 0}); err == nil {
			t.Error("resolveTargetUser(bad SUDO_UID) = nil error, want a lookup failure")
		}
	})
}

func TestCrossUserGuard(t *testing.T) {
	self := targetUser{Source: ""}
	other := targetUser{Source: "the --user flag", UID: 1000, Username: "alice"}

	if got := crossUserGuard(self, true, false, 0); got != "" {
		t.Errorf("self, --fix: got %q, want \"\" (nothing cross-user applies)", got)
	}
	if got := crossUserGuard(self, false, false, 1000); got != "" {
		t.Errorf("self, non-root: got %q, want \"\"", got)
	}
	if got := crossUserGuard(other, true, false, 0); got == "" {
		t.Error("other user, --fix, root: want a refusal, got \"\"")
	}
	if got := crossUserGuard(other, false, false, 1000); got == "" {
		t.Error("other user, no --fix, non-root: want a refusal (requires root), got \"\"")
	}
	if got := crossUserGuard(other, false, false, 0); got != "" {
		t.Errorf("other user, no --fix, root: got %q, want \"\" (read-only cross-user is allowed)", got)
	}
	if got := crossUserGuard(other, false, true, 0); got == "" {
		t.Error("other user, --test-backend, root: want a refusal, got \"\"")
	}
}

// TestAskpassExports pins the exported environment verbatim. All three lines are
// load-bearing, and losing one breaks the wallet refill without breaking
// anything a coarser assertion would notice: SSH_ASKPASS names the helper,
// REQUIRE=force is what makes ssh consult it at all in a session with no
// DISPLAY, and the marker is how the helper knows its argv is a prompt to answer.
func TestAskpassExports(t *testing.T) {
	got := askpassExports("/usr/local/bin/sshakku")
	want := "export SSH_ASKPASS='/usr/local/bin/sshakku'\n" +
		"export SSH_ASKPASS_REQUIRE=force\n" +
		"export SSHAKKU_ASKPASS=1\n"
	if got != want {
		t.Errorf("askpassExports = %q, want %q", got, want)
	}
}

// TestWantsAskpass guards against askpass-env's shell-wide export of
// EnvAskpassMode shadowing every later subcommand typed by hand in that same
// shell (e.g. `sshakku doctor` silently turning into an askpass prompt).
func TestWantsAskpass(t *testing.T) {
	tests := []struct {
		name          string
		askpassEnvSet bool
		args          []string
		want          bool
	}{
		{"env unset, real prompt text", false, []string{"Enter passphrase for key '/home/u/.ssh/id_ed25519': "}, false},
		{"env set, real prompt text", true, []string{"Enter passphrase for key '/home/u/.ssh/id_ed25519': "}, true},
		{"env set, no args", true, nil, true},
		{"env set, doctor", true, []string{"doctor"}, false},
		{"env set, doctor with flags", true, []string{"doctor", "--user", "stefano"}, false},
		{"env set, forget", true, []string{"forget", "--all"}, false},
		{"env set, help", true, []string{"help"}, false},
		{"env set, unknown command", true, []string{"bogus"}, true},
		{"env unset, doctor", false, []string{"doctor"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := wantsAskpass(tc.askpassEnvSet, tc.args); got != tc.want {
				t.Errorf("wantsAskpass(%v, %q) = %v, want %v", tc.askpassEnvSet, tc.args, got, tc.want)
			}
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
	if err := os.MkdirAll(confD, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(confD, "10-override.toml"), "key_lifetime = \"2h\"\n")

	settings := loadSettings(paths.Layout{ConfigDir: dir}, "test", fakeLogger{})

	if settings.KeyLifetime != 2*time.Hour {
		t.Errorf("KeyLifetime = %v, want 2h (config.d/10-override.toml must win over config.toml)", settings.KeyLifetime)
	}
	if !settings.Quiet {
		t.Errorf("Quiet = %v, want true (config.toml's own value, untouched by config.d/)", settings.Quiet)
	}
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
	if err := os.Mkdir(filepath.Join(dir, "config.toml"), 0o755); err != nil {
		t.Fatalf("mkdir config.toml: %v", err)
	}
	// A malformed drop-in makes config.LoadDir report an error.
	confD := filepath.Join(dir, "config.d")
	if err := os.MkdirAll(confD, 0o755); err != nil {
		t.Fatalf("mkdir config.d: %v", err)
	}
	writeFile(t, filepath.Join(confD, "10-bad.toml"), "this is not = valid = toml")
	// An unparseable env value makes config.Resolve report an error.
	t.Setenv("SSHAKKU_KEY_LIFETIME", "notaduration")

	log := &countingLogger{}
	settings := loadSettings(paths.Layout{ConfigDir: dir}, "test", log)

	if log.n < 3 {
		t.Errorf("logged %d errors, want at least 3 (config load, config.d, resolve)", log.n)
	}
	// A setting untouched by the errors still resolves to its default.
	if settings.SecretBackend == "" {
		t.Error("SecretBackend is empty, want the built-in default despite the load errors")
	}
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
		if got := tail(tc.s, tc.n); got != tc.want {
			t.Errorf("tail(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
		}
	}
}

func TestRandomProbeValue(t *testing.T) {
	a, err := randomProbeValue()
	if err != nil {
		t.Fatalf("randomProbeValue: %v", err)
	}
	if len(a) != 32 {
		t.Errorf("len(randomProbeValue()) = %d, want 32 (16 bytes hex-encoded)", len(a))
	}
	if _, err := hex.DecodeString(a); err != nil {
		t.Errorf("randomProbeValue() = %q, not valid hex: %v", a, err)
	}
	b, err := randomProbeValue()
	if err != nil {
		t.Fatalf("randomProbeValue: %v", err)
	}
	if a == b {
		t.Errorf("two calls both returned %q, want distinct random values", a)
	}
}

func TestDetectGUIAvailableHeadless(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	if detectGUIAvailable() {
		t.Error("detectGUIAvailable() = true with no display server, want false")
	}
}

// TestAskpassEnvHeadless confirms a session with no display server is wired the
// same as a graphical one: the broker reads the wallet, which needs no display,
// and a key that has expired from the agent must be refilled there too. Unlike
// the case in cover_remaining_test.go this one leaves the GUI probe real, so it
// also covers the detector actually reporting a headless session.
func TestAskpassEnvHeadless(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	d := realDeps()
	d.self = func() (string, error) { return "/opt/sshakku/bin/sshakku", nil }
	var out, errOut bytes.Buffer
	if got := d.askpassEnv(&out, &errOut); got != 0 {
		t.Errorf("askpassEnv (headless) = %d, want 0", got)
	}
	if got, want := out.String(), askpassExports("/opt/sshakku/bin/sshakku"); got != want {
		t.Errorf("headless askpassEnv wrote %q to stdout, want %q", got, want)
	}
}

func TestStderrNotifier(t *testing.T) {
	var buf bytes.Buffer
	stderrNotifier{w: &buf}.Notify("hello world")
	if got, want := buf.String(), "sshakku: hello world\n"; got != want {
		t.Errorf("Notify wrote %q, want %q", got, want)
	}
}

// TestDispatchRoutesToRun covers dispatch's non-askpass branch: with the askpass
// env unset, it must fall through to normal subcommand dispatch. The askpass
// branch is exercised via TestAskpassHandoff.
func TestDispatchRoutesToRun(t *testing.T) {
	if got := dispatch(realDeps(), io.Discard, io.Discard, []string{"help"}, false); got != 0 {
		t.Errorf("dispatch(help) = %d, want 0", got)
	}
	if got := dispatch(realDeps(), io.Discard, io.Discard, nil, false); got != 2 {
		t.Errorf("dispatch(no args) = %d, want 2 (usage)", got)
	}
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
		if got := realDeps().askpassFromHandoff(io.Discard); got != 1 {
			t.Errorf("askpassFromHandoff (no token) = %d, want 1", got)
		}
	})

	t.Run("unresolvable token routed via askpass", func(t *testing.T) {
		t.Setenv(keys.EnvPassHandoffToken, "sshakku-test-nonexistent-token")
		if got := realDeps().askpass(io.Discard, nil); got != 1 {
			t.Errorf("askpass (bogus handoff token) = %d, want 1", got)
		}
	})
}

func TestKeystateDir(t *testing.T) {
	got := keystateDir(paths.Layout{AgentSock: "/run/user/1000/sshakku/agent.sock"})
	if want := "/run/user/1000/sshakku/keystate"; got != want {
		t.Errorf("keystateDir = %q, want %q", got, want)
	}
}

func TestCurrentUser(t *testing.T) {
	t.Run("USER set", func(t *testing.T) {
		t.Setenv("USER", "sshakku-test-user")
		if got := currentUser(); got != "sshakku-test-user" {
			t.Errorf("currentUser = %q, want the $USER value", got)
		}
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
		if got := currentUser(); got != self.Username {
			t.Errorf("currentUser = %q, want %q from user.Current()", got, self.Username)
		}
	})
}

func TestNewHostSource(t *testing.T) {
	if newHostSource("") == nil {
		t.Error("newHostSource returned nil, want a diagnose.HostSource for this OS")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
