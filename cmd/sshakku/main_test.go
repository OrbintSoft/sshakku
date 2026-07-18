package main

import (
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
	"time"

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
			if got := run(io.Discard, io.Discard, tc.args); got != tc.want {
				t.Errorf("run(%q) = %d, want %d", tc.args, got, tc.want)
			}
		})
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

func TestAskpassExports(t *testing.T) {
	got := askpassExports("/usr/local/bin/sshakku")
	want := "export SSH_ASKPASS='/usr/local/bin/sshakku'\n" +
		"export SSH_ASKPASS_REQUIRE=prefer\n" +
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
