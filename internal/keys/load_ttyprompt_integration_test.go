//go:build unix

package keys

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/giveup"
	"github.com/OrbintSoft/sshakku/internal/keyring"
)

// Environment the parent hands its re-executed self, which runs the loader
// against the pseudo-terminal instead of running the test.
const (
	envTTYHelper    = "SSHAKKU_TTY_PROMPT_HELPER"
	envTTYKeyfile   = "SSHAKKU_TTY_PROMPT_KEYFILE"
	envTTYAskpass   = "SSHAKKU_TTY_PROMPT_ASKPASS"
	envTTYGiveupDir = "SSHAKKU_TTY_PROMPT_GIVEUP_DIR"
)

// vaultStoresMarker prefixes the one line the helper prints when it is done,
// reporting how many passphrases reached the secret store — the one outcome the
// parent cannot observe from outside the child. It carries a count, never a
// passphrase.
const vaultStoresMarker = "sshakku-test: vault-stores="

// ttyPromptTimeout bounds every read from the terminal, so a regression that
// stops prompting (or stops answering) fails the test instead of hanging CI.
// ttyHelperTimeout bounds the child as a whole, which walks through one real
// ssh-add per attempt and so needs room for several of these in a row.
const (
	ttyPromptTimeout = 30 * time.Second
	ttyHelperTimeout = 3 * time.Minute
)

// ttyPromptLine is what TTYPrompter writes to the terminal for the test key;
// the tests count these to see how many times the user was asked.
const ttyPromptLine = "Enter passphrase for id_test: "

// buildAskpassHelper builds sshakku into dir and returns the path to hand ssh
// as its SSH_ASKPASS program: the helper's own name, linked to the binary
// beside it, the way an install lays the two down. The name is what tells the
// binary that its argument is a prompt to answer rather than a command to run,
// so handing over the binary's own path would exercise a layout no user has and
// get a passphrase prompt answered with a usage error.
func buildAskpassHelper(t *testing.T, dir string) string {
	t.Helper()
	binary := filepath.Join(dir, "sshakku")
	build := exec.Command("go", "build", "-o", binary, "github.com/OrbintSoft/sshakku/cmd/sshakku")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sshakku: %v: %s", err, out)
	}
	helper := filepath.Join(dir, "sshakku-askpass")
	if err := os.Symlink(binary, helper); err != nil {
		t.Fatalf("link the askpass helper beside sshakku: %v", err)
	}
	return helper
}

// TestLoadKeysFirstTimePromptRealTerminal drives the first-time passphrase
// prompt over a real pseudo-terminal: an empty vault, no graphical prompter, a
// real ssh-agent, and the real TTYPrompter reading from a genuine controlling
// terminal rather than the socketpair the unit tests substitute for one.
//
// The child is this test binary re-executed into its own session with the pty
// slave as its controlling terminal, which is the only way a process can obtain
// one; the parent then plays the user on the master end.
//
// Three things only a live terminal can show: the prompt really reaches the
// terminal, the typed passphrase is never echoed back onto it, and the key
// really lands in the agent afterwards.
func TestLoadKeysFirstTimePromptRealTerminal(t *testing.T) {
	if os.Getenv(envTTYHelper) == "1" {
		runTTYPromptHelper()
	}
	requireRealSSHBinaries(t)
	requireUsableHandoff(t)

	const passphrase = "sshakku-live-terminal-test-passphrase"
	env := setupTTYPromptTest(t, passphrase)

	master, slave := openPTY(t)
	child := startTTYPromptHelper(t, env, slave)

	// Type only once the prompt has arrived, the way a user would: a terminal's
	// line discipline echoes what it receives as it receives it, so this is
	// exactly where a passphrase would become visible on screen.
	seen := readUntil(t, master, ttyPromptLine)
	if _, err := master.WriteString(passphrase + "\n"); err != nil {
		t.Fatalf("type the passphrase on the terminal: %v", err)
	}
	seen += drain(t, master)

	if err := child.Wait(); err != nil {
		t.Fatalf("helper: %v\nterminal output:\n%q", err, seen)
	}
	if strings.Contains(seen, passphrase) {
		t.Errorf("the passphrase was echoed back onto the terminal; output:\n%q", seen)
	}
	if want := vaultStoresMarker + "1"; !strings.Contains(seen, want) {
		t.Errorf("a freshly prompted passphrase was not stored for next time (want %q); output:\n%q", want, seen)
	}
	if entries, err := os.ReadDir(env.giveupDir); err == nil && len(entries) > 0 {
		t.Errorf("a successful first-time prompt recorded a give-up: %v", entries)
	}

	assertKeyInAgent(t, env.keyfile, true)
}

// TestLoadKeysWrongPassphraseRealTerminal answers the live prompt with the
// wrong passphrase every time, over the same real pseudo-terminal, and checks
// how the loader gives up: the user is asked exactly MaxAttempts times and no
// more, is then told on the terminal that the key could not be loaded, and the
// key is marked as abandoned so later shells stop re-prompting for it.
//
// A rejected passphrase must also leave no trace — not echoed onto the
// terminal, and never written to the vault, which would poison later silent
// loads with a passphrase already known to be wrong.
func TestLoadKeysWrongPassphraseRealTerminal(t *testing.T) {
	if os.Getenv(envTTYHelper) == "1" {
		runTTYPromptHelper()
	}
	requireRealSSHBinaries(t)
	requireUsableHandoff(t)

	const wrong = "not-the-passphrase-for-this-key"
	env := setupTTYPromptTest(t, "sshakku-live-terminal-test-passphrase")

	master, slave := openPTY(t)
	child := startTTYPromptHelper(t, env, slave)

	var seen string
	for attempt := 1; attempt <= defaultMaxAttempts; attempt++ {
		seen += readUntil(t, master, ttyPromptLine)
		if _, err := master.WriteString(wrong + "\n"); err != nil {
			t.Fatalf("type the wrong passphrase (attempt %d): %v", attempt, err)
		}
	}
	seen += drain(t, master)

	if err := child.Wait(); err != nil {
		t.Fatalf("helper: %v\nterminal output:\n%q", err, seen)
	}
	if got := strings.Count(seen, ttyPromptLine); got != defaultMaxAttempts {
		t.Errorf("the user was asked %d times, want %d; output:\n%q", got, defaultMaxAttempts, seen)
	}
	if strings.Contains(seen, wrong) {
		t.Errorf("the rejected passphrase was echoed back onto the terminal; output:\n%q", seen)
	}
	if want := fmt.Sprintf("sshakku: could not load key id_test after %d attempts", defaultMaxAttempts); !strings.Contains(seen, want) {
		t.Errorf("the user was never told the key could not be loaded (want %q); output:\n%q", want, seen)
	}
	if want := vaultStoresMarker + "0"; !strings.Contains(seen, want) {
		t.Errorf("a rejected passphrase reached the vault (want %q); output:\n%q", want, seen)
	}
	if _, err := os.Stat(filepath.Join(env.giveupDir, "id_test")); err != nil {
		t.Errorf("no give-up recorded after the attempts ran out, so every later shell would prompt again: %v", err)
	}

	assertKeyInAgent(t, env.keyfile, false)
}

// TestLoadKeysEmptyAnswerRealTerminal answers the live prompt with nothing at
// all — Enter on its own, the answer a person gives by accident — and expects
// F8 to hold exactly as it does for a wrong passphrase: asked MaxAttempts
// times, told once on the terminal, the key marked as abandoned so later shells
// stay quiet, nothing written to the vault, and the key left out of the agent.
//
// It takes a real terminal to ask this. A prompt read from anything else cannot
// distinguish "the user pressed Enter" from "there was nothing to read", and
// what the loader then does with that empty answer used to depend on which
// platform's handoff it was carried through.
func TestLoadKeysEmptyAnswerRealTerminal(t *testing.T) {
	if os.Getenv(envTTYHelper) == "1" {
		runTTYPromptHelper()
	}
	requireRealSSHBinaries(t)
	requireUsableHandoff(t)

	env := setupTTYPromptTest(t, "sshakku-live-terminal-test-passphrase")

	master, slave := openPTY(t)
	child := startTTYPromptHelper(t, env, slave)

	var seen string
	for attempt := 1; attempt <= defaultMaxAttempts; attempt++ {
		seen += readUntil(t, master, ttyPromptLine)
		if _, err := master.WriteString("\n"); err != nil {
			t.Fatalf("press Enter (attempt %d): %v", attempt, err)
		}
	}
	seen += drain(t, master)

	if err := child.Wait(); err != nil {
		t.Fatalf("helper: %v\nterminal output:\n%q", err, seen)
	}
	if got := strings.Count(seen, ttyPromptLine); got != defaultMaxAttempts {
		t.Errorf("the user was asked %d times, want %d; output:\n%q", got, defaultMaxAttempts, seen)
	}
	if want := fmt.Sprintf("sshakku: could not load key id_test after %d attempts", defaultMaxAttempts); !strings.Contains(seen, want) {
		t.Errorf("the user was never told the key could not be loaded (want %q); output:\n%q", want, seen)
	}
	if want := vaultStoresMarker + "0"; !strings.Contains(seen, want) {
		t.Errorf("an empty passphrase reached the vault (want %q); output:\n%q", want, seen)
	}
	if _, err := os.Stat(filepath.Join(env.giveupDir, "id_test")); err != nil {
		t.Errorf("no give-up recorded after the attempts ran out, so every later shell would prompt again: %v", err)
	}

	assertKeyInAgent(t, env.keyfile, false)
}

// ttyPromptEnv is the staged world a live-terminal prompt test runs against.
type ttyPromptEnv struct {
	keyfile   string
	askpass   string
	agentSock string
	giveupDir string
}

// setupTTYPromptTest stages a passphrase-protected key, a real ssh-agent, and
// the real sshakku binary as the SSH_ASKPASS helper — the same out-of-band
// handoff production uses, so the passphrase never reaches ssh-add's argv.
func setupTTYPromptTest(t *testing.T, passphrase string) ttyPromptEnv {
	t.Helper()

	// The Darwin handoff names an AF_UNIX socket under $HOME, whose path the
	// kernel caps well below what a default temp dir costs.
	dir := shortDir(t)
	t.Setenv("HOME", dir)

	keyfile := filepath.Join(dir, "id_test")
	if out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", passphrase, "-f", keyfile, "-q").CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v: %s", err, out)
	}

	askpass := buildAskpassHelper(t, dir)

	sock := filepath.Join(dir, "agent.sock")
	agentCmd := exec.Command("ssh-agent", "-D", "-a", sock)
	if err := agentCmd.Start(); err != nil {
		t.Fatalf("start ssh-agent: %v", err)
	}
	t.Cleanup(func() {
		_ = agentCmd.Process.Kill()
		_ = agentCmd.Wait()
	})
	waitForSocket(t, sock)
	t.Setenv("SSH_AUTH_SOCK", sock)

	return ttyPromptEnv{
		keyfile:   keyfile,
		askpass:   askpass,
		agentSock: sock,
		giveupDir: filepath.Join(dir, "giveup"),
	}
}

// startTTYPromptHelper re-executes this test binary as the helper child, with
// slave as its stdio and its controlling terminal. Setsid puts it in a new
// session (a process may only claim a controlling terminal there) and Setctty
// with Ctty=0 makes the slave — the child's stdin — that terminal, which is
// what the loader's /dev/tty then resolves to.
func startTTYPromptHelper(t *testing.T, env ttyPromptEnv, slave *os.File) *exec.Cmd {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), ttyHelperTimeout)
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = []string{
		envTTYHelper + "=1",
		envTTYKeyfile + "=" + env.keyfile,
		envTTYAskpass + "=" + env.askpass,
		envTTYGiveupDir + "=" + env.giveupDir,
		"SSH_AUTH_SOCK=" + env.agentSock,
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start the helper on the pty: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})
	// The child owns the slave now; dropping the parent's copy is what makes a
	// read on the master report end-of-file once the child exits.
	_ = slave.Close()
	return cmd
}

// runTTYPromptHelper is the child half: a real Loader — real ssh-agent, real
// ssh-add through the real handoff, real TTYPrompter on the controlling
// terminal — with an empty vault, so every passphrase must come from the
// terminal. It exits non-zero only when LoadKeys itself fails; every other
// outcome is left for the parent to observe from the agent, the give-up
// directory, and the terminal.
func runTTYPromptHelper() {
	secret := &fakeSecret{lookupFound: false}
	loader := Loader{
		Keys:   fakeLister{paths: []string{os.Getenv(envTTYKeyfile)}},
		Runner: ExecRunner{},
		Secret: secret,
		Prompt: TTYPrompter{},
		Adder:  ExecKeyAdder{AskpassProg: os.Getenv(envTTYAskpass)},
		Log:    &fakeLogger{},
		Notify: stderrNotifier{},
		Giveup: giveup.Store{Dir: os.Getenv(envTTYGiveupDir)},
	}
	if err := loader.LoadKeys(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "LoadKeys: %v\n", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stderr, "\n%s%d\n", vaultStoresMarker, len(secret.stored))
	os.Exit(0)
}

// stderrNotifier writes the loader's user-facing notices to the terminal, the
// way the sshakku binary does for an interactive shell, so the parent can read
// them off the pty.
type stderrNotifier struct{}

func (stderrNotifier) Notify(message string) {
	_, _ = fmt.Fprintf(os.Stderr, "sshakku: %s\n", message)
}

// readUntil reads from the terminal until want appears, and fails the test if
// it never does. It returns everything read so far, so the caller can assert on
// what else the terminal did (or did not) show.
func readUntil(t *testing.T, master *os.File, want string) string {
	t.Helper()

	deadline := time.Now().Add(ttyPromptTimeout)
	var seen strings.Builder
	buf := make([]byte, 512)
	for {
		if err := master.SetReadDeadline(deadline); err != nil {
			t.Fatalf("bound the terminal read: %v", err)
		}
		n, err := master.Read(buf)
		seen.Write(buf[:n])
		if strings.Contains(seen.String(), want) {
			return seen.String()
		}
		if err != nil {
			t.Fatalf("waiting for %q on the terminal: %v; read so far:\n%q", want, err, seen.String())
		}
	}
}

// drain reads whatever the child still writes to the terminal until it exits
// and the terminal reports end-of-file. A read error is the expected end: the
// master reports one (EIO on Linux) once the last slave end is gone.
func drain(t *testing.T, master *os.File) string {
	t.Helper()

	deadline := time.Now().Add(ttyPromptTimeout)
	var seen strings.Builder
	buf := make([]byte, 512)
	for {
		if err := master.SetReadDeadline(deadline); err != nil {
			t.Fatalf("bound the terminal read: %v", err)
		}
		n, err := master.Read(buf)
		seen.Write(buf[:n])
		if err != nil {
			if os.IsTimeout(err) {
				t.Fatalf("the helper never finished writing to the terminal; read so far:\n%q", seen.String())
			}
			return seen.String()
		}
	}
}

// assertKeyInAgent checks whether keyfile's fingerprint is in the running
// agent, which is where a prompted passphrase must (or must not) have landed.
func assertKeyInAgent(t *testing.T, keyfile string, want bool) {
	t.Helper()

	runner := ExecRunner{}
	fp, err := FileFingerprint(runner, keyfile)
	if err != nil {
		t.Fatalf("FileFingerprint: %v", err)
	}
	loaded, err := AgentFingerprints(runner)
	if err != nil {
		t.Fatalf("AgentFingerprints: %v", err)
	}
	if got := loaded[fp]; got != want {
		t.Errorf("key present in the agent = %v, want %v", got, want)
	}
}

// requireUsableHandoff skips the test when this environment cannot carry a
// passphrase from the loader to ssh-add. On Linux that is the kernel keyring,
// which a container or CI runner whose process was never linked to a session
// keyring by a PAM login cannot use; Darwin's socket handoff always works.
func requireUsableHandoff(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "linux" && !keyring.Available() {
		t.Skip("kernel user keyring isn't usable for a round trip in this environment (e.g. no session-keyring link — common in CI/containers without a PAM login)")
	}
}
