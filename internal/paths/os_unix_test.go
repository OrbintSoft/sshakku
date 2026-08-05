//go:build unix

package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
	if err := Ensure(l); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	for _, dir := range []string{config, state, runtime} {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if perm := fi.Mode().Perm(); perm != 0o700 {
			t.Errorf("%s perm = %o, want 700", dir, perm)
		}
	}
	fi, err := os.Stat(l.LogFile)
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("log perm = %o, want 600", perm)
	}
}

func TestEnsureRejectsSymlinkDir(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "elsewhere")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "cfg")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	l := Layout{ConfigDir: link, RuntimeDir: filepath.Join(root, "run"), SocketDir: filepath.Join(root, "run")}
	if err := Ensure(l); err == nil {
		t.Error("Ensure accepted a symlinked leaf directory, want error")
	}
}

func TestCleanupLegacyAgentDir(t *testing.T) {
	home := t.TempDir()
	agent := filepath.Join(home, ".ssh", "agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ssh-agent.sock", ".start.lock"} {
		if err := os.WriteFile(filepath.Join(agent, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	CleanupLegacyAgentDir(home)
	if _, err := os.Stat(agent); !os.IsNotExist(err) {
		t.Errorf("legacy agent dir still present: %v", err)
	}
}

func TestCleanupLegacyAgentDirLeavesForeignFiles(t *testing.T) {
	home := t.TempDir()
	agent := filepath.Join(home, ".ssh", "agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(agent, "keep-me")
	if err := os.WriteFile(foreign, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	CleanupLegacyAgentDir(home)
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("foreign file was removed: %v", err)
	}
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
		if err := os.Mkdir(ssh, 0o700); err != nil {
			t.Fatal(err)
		}
		agent := filepath.Join(ssh, "agent")
		if err := os.WriteFile(agent, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		CleanupLegacyAgentDir(home)
		if _, err := os.Stat(agent); err != nil {
			t.Errorf("agent file was removed: %v", err)
		}
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
		if env.Home != "/home/alice" {
			t.Errorf("Home = %q, want /home/alice", env.Home)
		}
		if env.ConfigHome != "/home/alice/.config" {
			t.Errorf("ConfigHome = %q", env.ConfigHome)
		}
		if env.StateHome != "/home/alice/.local/state" {
			t.Errorf("StateHome = %q", env.StateHome)
		}
		if env.RuntimeDir != "/run/user/1000" {
			t.Errorf("RuntimeDir = %q", env.RuntimeDir)
		}
		if env.CacheHome != "/home/alice/.cache" {
			t.Errorf("CacheHome = %q", env.CacheHome)
		}
		if env.UID != os.Getuid() {
			t.Errorf("UID = %d, want %d", env.UID, os.Getuid())
		}
	})

	t.Run("empty HOME takes the fallback path", func(t *testing.T) {
		t.Setenv("HOME", "")
		// os.UserHomeDir also reads $HOME on unix, so with it empty the fallback
		// yields no home either; the point is that the guarded branch runs
		// without panicking and leaves Home defined by whatever it resolves to.
		env := FromOS()
		if env.UID != os.Getuid() {
			t.Errorf("UID = %d, want %d", env.UID, os.Getuid())
		}
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
	if env.Home != "/fallback/home" {
		t.Errorf("Home = %q, want /fallback/home (from the homeDir fallback)", env.Home)
	}
	if env.UID != 4242 {
		t.Errorf("UID = %d, want 4242", env.UID)
	}
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
	if env.TempDir != "/the/tmp" {
		t.Errorf("TempDir = %q, want /the/tmp (a private one is kept)", env.TempDir)
	}

	env = fromEnv(getenv, homeDir, uid, func(string) bool { return false })
	if env.TempDir != "" {
		t.Errorf("TempDir = %q, want empty (a shared one is dropped)", env.TempDir)
	}

	env = fromEnv(func(string) string { return "" }, homeDir, uid, func(string) bool {
		t.Error("a temporary directory that was never named got inspected")
		return true
	})
	if env.TempDir != "" {
		t.Errorf("TempDir = %q, want empty (none was named)", env.TempDir)
	}
}

// TestProbeDir covers the directory/ownership probe: a real directory passes,
// a plain file and a missing path fail, and requireOwner enforces the uid.
func TestProbeDir(t *testing.T) {
	dir := t.TempDir()

	if !ProbeDir(dir, false) {
		t.Error("ProbeDir(dir, false) = false, want true")
	}
	if !ProbeDir(dir, true) {
		t.Error("ProbeDir(dir, true) = false, want true (owned by us)")
	}

	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if ProbeDir(file, false) {
		t.Error("ProbeDir(file, false) = true, want false (not a directory)")
	}
	if ProbeDir(filepath.Join(dir, "missing"), false) {
		t.Error("ProbeDir(missing) = true, want false")
	}

	// A uid that cannot own the temp dir must fail the ownership check.
	if ProbeDirAs(os.Getuid()+99999)(dir, true) {
		t.Error("ProbeDirAs(other uid)(dir, true) = true, want false")
	}
	// Without requireOwner the same probe ignores ownership.
	if !ProbeDirAs(os.Getuid()+99999)(dir, false) {
		t.Error("ProbeDirAs(other uid)(dir, false) = false, want true")
	}
}

// TestPrivateDir covers the question asked of a directory somebody else named:
// is it ours alone? Everything that would let another user in — a mode that
// grants them anything, a symlink pointing who knows where, something that is
// not a directory at all — has to answer no, since what goes in such a
// directory is a passphrase waiting to be collected.
func TestPrivateDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if !PrivateDir(dir) {
		t.Error("PrivateDir(0700 dir of ours) = false, want true")
	}

	for _, mode := range []os.FileMode{0o770, 0o707, 0o750, 0o705, 0o777} {
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatal(err)
		}
		if PrivateDir(dir) {
			t.Errorf("PrivateDir(%o dir) = true, want false", mode)
		}
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if PrivateDir(file) {
		t.Error("PrivateDir(file) = true, want false (not a directory)")
	}
	if PrivateDir(filepath.Join(dir, "missing")) {
		t.Error("PrivateDir(missing) = true, want false")
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if PrivateDir(link) {
		t.Error("PrivateDir(symlink to a private dir) = true, want false")
	}
}

// TestEnsureDirErrors covers ensureDir's failure branches: a parent that is a
// plain file makes MkdirAll fail, and an injected chmod that fails makes the
// permission step fail even though the directory was created.
func TestEnsureDirErrors(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notadir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureDir(filepath.Join(file, "child"), os.Chmod); err == nil {
		t.Error("ensureDir under a file returned nil, want error")
	}

	failChmod := func(string, os.FileMode) error { return errors.New("chmod boom") }
	if err := ensureDir(filepath.Join(root, "d"), failChmod); err == nil {
		t.Error("ensureDir with a failing chmod returned nil, want error")
	}
}

// TestEnsureFileErrors covers ensureFile's failure branches: opening a path that
// is an existing directory fails, and an injected chmod that fails makes the
// permission step fail even though the file was created.
func TestEnsureFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := ensureFile(dir, 0o600, os.Chmod); err == nil {
		t.Error("ensureFile on a directory returned nil, want error")
	}

	failChmod := func(string, os.FileMode) error { return errors.New("chmod boom") }
	if err := ensureFile(filepath.Join(dir, "f"), 0o600, failChmod); err == nil {
		t.Error("ensureFile with a failing chmod returned nil, want error")
	}
}
