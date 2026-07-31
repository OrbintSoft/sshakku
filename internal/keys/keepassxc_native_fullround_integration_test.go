//go:build unix

package keys

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keepassxc"
)

// This is the user's own scenario against a running KeePassXC, reached over its
// local protocol, with nothing faked anywhere in the path: the real sshakku
// binary, a dedicated ssh-agent of this test's own, a real passphrase-protected
// key, a real terminal, and a real wallet.
//
// It verifies, in the order a user meets them (docs/FEATURES.md):
//
//	F4  the first use asks for the passphrase once, and it is saved
//	F5  every later login shell loads that key with nothing typed
//	F6  after the key expires from the agent, an already-open shell gets it
//	    back from the wallet, still with nothing typed
//	F9  forgetting a passphrase the wallet gives no way to delete says so and
//	    names the entry, rather than claiming it is gone
//	F23 the route the user pinned is the one used
//
// The scenario is written from those promises. The one thing it must never do
// is make silence easy: F5 and F6 are checked with a real terminal attached, so
// a regression that starts asking has somewhere to ask and is caught, rather
// than failing for want of a tty.
//
// Opt-in, never on reachability: a running KeePassXC is somebody's real wallet,
// and this test writes to it.
func TestKeePassXCNativeFullRound(t *testing.T) {
	requireRealKeePassXCNative(t)
	requireRealSSHTools(t)
	requireUsableHandoff(t)

	const passphrase = "sshakku-native-full-round-passphrase"
	env := setupNativeFullRound(t, passphrase)

	// F4 — the first use. The wallet holds nothing for this key yet, so the
	// only place the passphrase can come from is the person at the terminal.
	master, slave := openPTY(t)
	child := startSSHakkuOnTerminal(t, env, slave, "load-keys")

	seen := readUntil(t, master, ttyPromptLine)
	if _, err := master.WriteString(passphrase + "\n"); err != nil {
		t.Fatalf("type the passphrase on the terminal: %v", err)
	}
	seen += drain(t, master)
	if err := child.Wait(); err != nil {
		t.Fatalf("first load-keys: %v\nterminal output:\n%q", err, seen)
	}
	if got := strings.Count(seen, ttyPromptLine); got != 1 {
		t.Errorf("the user was asked %d times on first use, want exactly 1; output:\n%q", got, seen)
	}
	if strings.Contains(seen, passphrase) {
		t.Errorf("the passphrase was echoed back onto the terminal; output:\n%q", seen)
	}
	assertKeyInAgent(t, env.keyfile, true)

	// F5 — a later login shell. Emptying the agent is what logging out and back
	// in looks like from here; the passphrase must now come from the wallet.
	// A terminal is attached on purpose: asking is possible, and must not happen.
	clearAgent(t)
	master, slave = openPTY(t)
	child = startSSHakkuOnTerminal(t, env, slave, "load-keys")
	seen = drain(t, master)
	if err := child.Wait(); err != nil {
		t.Fatalf("second load-keys: %v\nterminal output:\n%q", err, seen)
	}
	if strings.Contains(seen, ttyPromptLine) {
		t.Errorf("a later login shell asked for the passphrase again, so the first one was never saved; output:\n%q", seen)
	}
	if seen != "" {
		t.Errorf("a silent load wrote to the terminal: %q", seen)
	}
	assertKeyInAgent(t, env.keyfile, true)

	// F6 — the key expires while the shell stays open. Waiting for the agent to
	// really drop it, rather than for the configured lifetime to elapse, is what
	// makes this the agent's behaviour and not a clock's.
	waitForAgentToDropKey(t, env.keyfile)

	master, slave = openPTY(t)
	child = startSSHakkuOnTerminal(t, env, slave, "load-keys")
	seen = drain(t, master)
	if err := child.Wait(); err != nil {
		t.Fatalf("load-keys after expiry: %v\nterminal output:\n%q", err, seen)
	}
	if strings.Contains(seen, ttyPromptLine) {
		t.Errorf("an expired key was asked about again instead of being refilled from the wallet; output:\n%q", seen)
	}
	assertKeyInAgent(t, env.keyfile, true)

	// F9 — forgetting what this wallet gives no way to delete. The promise is
	// not that it succeeds; it is that it never says the passphrase is gone
	// while it is still there, and that it tells the user where to find it.
	out, err := runSSHakku(t, env, "forget", "id_test")
	if err == nil {
		t.Error("forget reported success on a wallet that cannot delete, so the passphrase would still be there unannounced")
	}
	for _, want := range []string{"id_test", "KeePassXC"} {
		if !strings.Contains(out, want) {
			t.Errorf("forget did not name %q, so the user cannot find the entry to remove; output:\n%q", want, out)
		}
	}

	// And it really is still there: the next load is still silent.
	clearAgent(t)
	master, slave = openPTY(t)
	child = startSSHakkuOnTerminal(t, env, slave, "load-keys")
	seen = drain(t, master)
	if err := child.Wait(); err != nil {
		t.Fatalf("load-keys after forget: %v\nterminal output:\n%q", err, seen)
	}
	if strings.Contains(seen, ttyPromptLine) {
		t.Error("forget left the wallet without the passphrase after reporting it could not delete it")
	}
}

// nativeFullRoundEnv is the staged world the scenario runs in: one throwaway
// account's worth of directories, one key, and one agent.
type nativeFullRoundEnv struct {
	binary    string
	home      string
	keyfile   string
	agentSock string
	configDir string
	stateDir  string
}

// nativeKeyLifetime is short enough that the expiry step is not the slow part
// of the run, and long enough that the two loads before it are not racing it.
const nativeKeyLifetime = 5 * time.Second

// setupNativeFullRound builds the real binary, gives it a config that pins the
// native route, and starts an ssh-agent of this test's own — never the one the
// surrounding session may already have, whose keys are not ours to touch.
func setupNativeFullRound(t *testing.T, passphrase string) nativeFullRoundEnv {
	t.Helper()

	// The Darwin handoff names an AF_UNIX socket under $HOME, whose path the
	// kernel caps well below what a default temp dir costs.
	root := shortDir(t)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatalf("make the test home: %v", err)
	}

	keyfile := filepath.Join(home, ".ssh", "id_test")
	if out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", passphrase, "-f", keyfile, "-q").CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v: %s", err, out)
	}

	binary := filepath.Join(root, "sshakku")
	if out, err := exec.Command("go", "build", "-o", binary, "github.com/OrbintSoft/sshakku/cmd/sshakku").CombinedOutput(); err != nil {
		t.Fatalf("build sshakku: %v: %s", err, out)
	}

	configDir := filepath.Join(root, "config", "sshakku")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("make the config dir: %v", err)
	}
	config := "secret_backend = \"keepassxc\"\n" +
		"keepassxc_route = \"native\"\n" +
		"key_lifetime = \"" + nativeKeyLifetime.String() + "\"\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	stateDir := filepath.Join(root, "state")
	// Where nothing else provides a running KeePassXC, this test provides one.
	// The check that follows is what decides either way: with the opt-in given,
	// an app that is not answering is a broken environment, not a reason to pass.
	if app := os.Getenv("SSHAKKU_TEST_KEEPASSXC_APP"); app != "" {
		stageKeePassXC(t, app, root, filepath.Join(stateDir, "sshakku"))
	}
	requireKeePassXCListening(t)

	sock := filepath.Join(root, "agent.sock")
	agentCmd := exec.Command("ssh-agent", "-D", "-a", sock)
	if err := agentCmd.Start(); err != nil {
		t.Fatalf("start the test's own ssh-agent: %v", err)
	}
	t.Cleanup(func() {
		_ = agentCmd.Process.Kill()
		_ = agentCmd.Wait()
	})
	waitForSocket(t, sock)
	// The test's own ssh-add calls — emptying the agent, listing what is in it —
	// have to reach this agent and no other.
	t.Setenv("SSH_AUTH_SOCK", sock)

	return nativeFullRoundEnv{
		binary:    binary,
		home:      home,
		keyfile:   keyfile,
		agentSock: sock,
		configDir: filepath.Dir(configDir),
		stateDir:  stateDir,
	}
}

// childEnv is what the binary runs with. DISPLAY is deliberately absent: this
// is a session with no graphical prompter, so a question has to reach the
// terminal, where the test can see it. XDG_RUNTIME_DIR and TMPDIR are the
// session's own, because between them they are where a running KeePassXC put
// its socket — which directory it picks depends on the platform.
func (e nativeFullRoundEnv) childEnv() []string {
	return []string{
		"HOME=" + e.home,
		"XDG_CONFIG_HOME=" + e.configDir,
		"XDG_STATE_HOME=" + e.stateDir,
		"XDG_RUNTIME_DIR=" + os.Getenv("XDG_RUNTIME_DIR"),
		"TMPDIR=" + os.Getenv("TMPDIR"),
		"SSH_AUTH_SOCK=" + e.agentSock,
		"PATH=" + os.Getenv("PATH"),
	}
}

// startSSHakkuOnTerminal runs the binary with slave as its stdio and its
// controlling terminal. Setsid puts it in a new session, the only place a
// process may claim one; Setctty with Ctty=0 makes the slave — its stdin — that
// terminal, which is what the prompter's /dev/tty then resolves to.
func startSSHakkuOnTerminal(t *testing.T, env nativeFullRoundEnv, slave *os.File, args ...string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(env.binary, args...)
	cmd.Env = env.childEnv()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s %v on the terminal: %v", env.binary, args, err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	// The child owns the slave now; dropping this copy is what makes a read on
	// the master report end-of-file once the child exits.
	_ = slave.Close()
	return cmd
}

// runSSHakku runs the binary without a terminal and returns everything it said,
// for the steps whose outcome is a message rather than a silence.
func runSSHakku(t *testing.T, env nativeFullRoundEnv, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command(env.binary, args...)
	cmd.Env = env.childEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// clearAgent empties the test's own agent, which is what a new login session
// looks like to everything downstream of it.
func clearAgent(t *testing.T) {
	t.Helper()
	if out, err := exec.Command("ssh-add", "-D").CombinedOutput(); err != nil {
		t.Fatalf("empty the agent: %v: %s", err, out)
	}
}

// waitForAgentToDropKey blocks until the agent itself has let the key go, so
// the step that follows is answering a real expiry rather than a timer.
func waitForAgentToDropKey(t *testing.T, keyfile string) {
	t.Helper()

	runner := ExecRunner{}
	fp, err := FileFingerprint(runner, keyfile)
	if err != nil {
		t.Fatalf("FileFingerprint: %v", err)
	}
	deadline := time.Now().Add(nativeKeyLifetime + 15*time.Second)
	for {
		loaded, err := AgentFingerprints(runner)
		if err != nil {
			t.Fatalf("AgentFingerprints while waiting for the key to expire: %v", err)
		}
		if !loaded[fp] {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatal("the key was still in the agent well after its lifetime elapsed, so nothing here would ever be a refill")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// requireRealKeePassXCNative refuses to run without an explicit opt-in: a
// running KeePassXC is somebody's real wallet, and this test writes to it.
func requireRealKeePassXCNative(t *testing.T) {
	t.Helper()

	if os.Getenv("SSHAKKU_TEST_ALLOW_REAL_KEEPASSXC_NATIVE") != "1" {
		t.Skip("skipping: set SSHAKKU_TEST_ALLOW_REAL_KEEPASSXC_NATIVE=1 to run against a running KeePassXC, which this test writes to")
	}
}

// requireKeePassXCListening insists the thing this test is supposed to talk to
// is actually there: with the opt-in given, an absent KeePassXC is a broken
// environment, not a reason to quietly pass.
func requireKeePassXCListening(t *testing.T) {
	t.Helper()

	candidates := keepassxc.SocketPaths()
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Fatalf("KeePassXC is not listening on any of %v", candidates)
}
