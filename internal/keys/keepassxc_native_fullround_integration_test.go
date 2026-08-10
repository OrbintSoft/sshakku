//go:build unix

package keys

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keepassxc"
	"github.com/OrbintSoft/sshakku/internal/keyring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	requireEverythingTheRoundDrives(t)

	const passphrase = "sshakku-native-full-round-passphrase"
	env := setupNativeFullRound(t, passphrase)

	// F4 — the first use. The wallet holds nothing for this key yet, so the
	// only place the passphrase can come from is the person at the terminal.
	master, slave := openPTY(t)
	child := startSSHakkuOnTerminal(t, env, slave, "load-keys")

	seen := readUntil(t, master, ttyPromptLine)
	_, err := master.WriteString(passphrase + "\n")
	require.NoError(t, err, "the user types their passphrase")
	seen += drain(t, master)
	require.NoErrorf(t, child.Wait(), "the first use must load the key; terminal output:\n%q", seen)
	assert.Equalf(t, 1, strings.Count(seen, ttyPromptLine),
		"they are asked once, and once only, the first time they use the key:\n%q", seen)
	assert.NotContainsf(t, seen, passphrase,
		"and it must never be echoed back: that puts it on the screen and in the scrollback:\n%q", seen)
	assertKeyInAgent(t, env.keyfile, true)

	// F5 — a later login shell. Emptying the agent is what logging out and back
	// in looks like from here; the passphrase must now come from the wallet.
	// A terminal is attached on purpose: asking is possible, and must not happen.
	clearAgent(t)
	master, slave = openPTY(t)
	child = startSSHakkuOnTerminal(t, env, slave, "load-keys")
	seen = drain(t, master)
	require.NoErrorf(t, child.Wait(), "a later login shell must load the key; terminal output:\n%q", seen)
	assert.NotContainsf(t, seen, ttyPromptLine,
		"and must not ask again: a terminal is attached on purpose, so asking is possible and must not happen:\n%q",
		seen)
	assert.Emptyf(t, seen, "the whole promise is that the user notices nothing at all:\n%q", seen)
	assertKeyInAgent(t, env.keyfile, true)

	// F6 — the key expires while the shell stays open. Waiting for the agent to
	// really drop it, rather than for the configured lifetime to elapse, is what
	// makes this the agent's behaviour and not a clock's.
	waitForAgentToDropKey(t, env.keyfile)

	master, slave = openPTY(t)
	child = startSSHakkuOnTerminal(t, env, slave, "load-keys")
	seen = drain(t, master)
	require.NoErrorf(t, child.Wait(), "a key that expired must be loaded again; terminal output:\n%q", seen)
	assert.NotContainsf(t, seen, ttyPromptLine,
		"out of the wallet, with nothing typed: the user is in the middle of their day and did not lose the key:\n%q",
		seen)
	assertKeyInAgent(t, env.keyfile, true)

	// F9 — forgetting what this wallet gives no way to delete. The promise is
	// not that it succeeds; it is that it never says the passphrase is gone
	// while it is still there, and that it tells the user where to find it.
	out, err := runSSHakku(t, env, "forget", "id_test")
	assert.Errorf(t, err,
		"this wallet gives no way to delete, and claiming success would tell the user a passphrase is gone "+
			"while it is still sitting in their database:\n%q", out)
	assert.Containsf(t, out, "id_test", "the entry to remove must be named:\n%q", out)
	assert.Containsf(t, out, "KeePassXC", "and where to remove it, since the user is the one who has to:\n%q", out)

	// And it really is still there: the next load is still silent.
	clearAgent(t)
	master, slave = openPTY(t)
	child = startSSHakkuOnTerminal(t, env, slave, "load-keys")
	seen = drain(t, master)
	require.NoErrorf(t, child.Wait(), "the key must still load; terminal output:\n%q", seen)
	assert.NotContainsf(t, seen, ttyPromptLine,
		"silently, because the passphrase really is still there: a forget that reported it could not delete "+
			"must not have deleted anything:\n%q", seen)
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
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0o700), "a throwaway account to run in")

	keyfile := filepath.Join(home, ".ssh", "id_test")
	out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", passphrase, "-f", keyfile, "-q").CombinedOutput()
	require.NoErrorf(t, err, "a real passphrase-protected key:\n%s", out)

	binary := filepath.Join(root, "sshakku")
	out, err = exec.Command("go", "build", "-o", binary, "github.com/OrbintSoft/sshakku/cmd/sshakku").CombinedOutput()
	require.NoErrorf(t, err, "the real sshakku binary is what this scenario drives:\n%s", out)
	// ssh is handed the helper beside the binary, not the binary itself, so a
	// build with nothing next to it is a layout no install produces: the key
	// would never open, however right the passphrase is.
	require.NoError(t, os.Symlink(binary, filepath.Join(root, "sshakku-askpass")),
		"laid down beside it under the name an install gives it")

	configDir := filepath.Join(root, "config", "sshakku")
	require.NoError(t, os.MkdirAll(configDir, 0o700), "a configuration directory of this account's own")
	// The terminal is where this scenario watches: F5 and F6 are checked with a
	// real one attached, so that a regression which starts asking has somewhere
	// to ask. On a machine with a window server the product would rightly raise
	// a dialog instead (F29), and a dialog nobody is sitting in front of is a
	// question that never comes back — so this session says it has no dialog,
	// the same thing a user writes when they want to be asked where they are.
	config := "secret_backend = \"keepassxc\"\n" +
		"keepassxc_route = \"native\"\n" +
		"gui_prompter = \"none\"\n" +
		"key_lifetime = \"" + nativeKeyLifetime.String() + "\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(config), 0o600),
		"a configuration pinning the route under test")

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
	require.NoError(t, agentCmd.Start(),
		"an ssh-agent of this test's own: the surrounding session's keys are not ours to touch")
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

	require.NoErrorf(t, cmd.Start(), "start %s %v on the terminal", env.binary, args)
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
	out, err := exec.Command("ssh-add", "-D").CombinedOutput()
	require.NoErrorf(t, err, "empty the agent, which is what a new login session looks like:\n%s", out)
}

// waitForAgentToDropKey blocks until the agent itself has let the key go, so
// the step that follows is answering a real expiry rather than a timer.
func waitForAgentToDropKey(t *testing.T, keyfile string) {
	t.Helper()

	runner := ExecRunner{}
	fp, err := FileFingerprint(runner, keyfile)
	require.NoError(t, err, "reading the key's fingerprint must succeed")
	deadline := time.Now().Add(nativeKeyLifetime + 15*time.Second)
	for {
		loaded, err := AgentFingerprints(runner)
		require.NoError(t, err, "asking the agent what it holds must keep succeeding")
		if !loaded[fp] {
			return
		}
		require.True(t, time.Now().Before(deadline),
			"the key is still in the agent well after its lifetime elapsed, so nothing after this would be a refill")
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

// requireEverythingTheRoundDrives insists on the pieces the scenario is made
// of. None of them is a reason to skip: the opt-in has already been given, so a
// missing piece is a broken environment, and skipping would report a green run
// for a round that never happened — which is exactly what a keyctl check
// belonging to another test did here on macOS, on a job that passed.
func requireEverythingTheRoundDrives(t *testing.T) {
	t.Helper()

	for _, bin := range []string{"ssh-agent", "ssh-add", "ssh-keygen"} {
		_, err := exec.LookPath(bin)
		require.NoErrorf(t, err, "%s is not on PATH, so the round cannot run", bin)
	}
	// The passphrase reaches a detached ssh-add through the kernel keyring on
	// Linux; the other platforms hand it over by their own means, which need
	// nothing arranged here.
	if runtime.GOOS == "linux" {
		require.True(t, keyring.Available(),
			"the kernel user keyring is not usable here, so nothing could hand a passphrase to ssh-add "+
				"(a session-keyring link is what a PAM login arranges)")
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
	require.FailNowf(t, "KeePassXC is not listening", "tried %v", candidates)
}
