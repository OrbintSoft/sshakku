// Command sshakku tends the SSH agent: it computes the per-user runtime
// paths, keeps the agent healthy, and loads keys with passphrases pulled from
// the OS secret store. The login shell wires it in by evaluating its output:
//
//	eval "$(sshakku shell-init)"
package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/giveup"
	"github.com/OrbintSoft/sshakku/internal/keyring"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keystate"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/secretservice"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// agentLockWait bounds how long a login blocks for the start lock before it
// proceeds without it, so a stuck holder slows the login but never hangs it.
const agentLockWait = 5 * time.Second

// internalReadSocketTokenCmd is not a user-facing command: `doctor` execs the
// binary under this name as a child running with another user's credentials,
// to read that user's per-login socket token from their own kernel keyring (a
// keyring is only visible to the uid that owns it, unlike files, which root can
// read regardless of owner). It is deliberately absent from usage/--help.
const internalReadSocketTokenCmd = "__read-socket-token"

// knownSubcommands lists every argv[1] value run dispatches below. main checks
// it before trusting EnvAskpassMode: ssh always execs the SSH_ASKPASS helper
// with just the prompt text as its one argument, never one of these exact
// names, but askpass-env exports EnvAskpassMode into the whole login shell
// (nn-ssh-init.sh), so it stays set for anything the user later types by
// hand in that same shell — a real subcommand must win over it. Keep this in
// sync with the switch in run.
var knownSubcommands = map[string]bool{
	"shell-init":               true,
	"ensure-agent":             true,
	"load-keys":                true,
	"askpass-env":              true,
	"doctor":                   true,
	"forget":                   true,
	"help":                     true,
	"-h":                       true,
	"--help":                   true,
	internalReadSocketTokenCmd: true,
}

const usage = `sshakku — SSH agent and key shepherd

usage: sshakku <command>

commands:
  shell-init     drive the agent healthy and print shell assignments to eval
  ensure-agent   drive the agent to a healthy state and print agent_sock
  load-keys      add the user's ssh keys to the agent (interactive sessions)
  askpass-env    print exports routing ssh's askpass through sshakku (GUI only)
  doctor         report the ssh-agent situation; --fix applies the self-heal;
                 --user <name|uid> reports on another user's session (root only,
                 read-only; auto-detected from SUDO_UID under sudo);
                 --test-backend [name] stores/looks up/deletes a throwaway
                 probe entry in the named (or configured) secret backend
  forget         delete stored passphrases: <keyname>... or --all
  help           show this help
`

func main() {
	// ssh-add execs this binary as its SSH_ASKPASS program, passing only the
	// prompt as an argument and marking the call via the environment. Handle that
	// before subcommand dispatch and return the passphrase from the keyring —
	// unless args actually name one of our own subcommands, which always wins
	// (see wantsAskpass).
	args := os.Args[1:]
	if wantsAskpass(os.Getenv(keys.EnvAskpassMode) != "", args) {
		os.Exit(askpass(os.Stdout, args))
	}
	os.Exit(run(os.Stdout, os.Stderr, args))
}

// wantsAskpass reports whether main should treat this invocation as ssh's
// SSH_ASKPASS helper rather than dispatch args as a subcommand. askpassEnvSet
// is EnvAskpassMode's presence in the environment; it alone is not enough,
// because askpass-env exports it into the whole login shell
// (nn-ssh-init.sh) so it stays set for anything typed by hand afterward
// too — a real ssh invocation always execs the helper with just the prompt
// text as its one argument, never one of our own subcommand names, so a known
// subcommand always takes priority over the env marker.
func wantsAskpass(askpassEnvSet bool, args []string) bool {
	return askpassEnvSet && (len(args) == 0 || !knownSubcommands[args[0]])
}

// run dispatches a subcommand and returns the process exit code. Output goes to
// the supplied writers so the command is testable without touching real stdio.
func run(stdout, stderr io.Writer, args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "shell-init":
		return shellInit(stdout, stderr)
	case "ensure-agent":
		return ensureAgent(stdout, stderr)
	case "load-keys":
		return loadKeys(stderr)
	case "askpass-env":
		return askpassEnv(stdout, stderr)
	case "doctor":
		return doctor(stdout, stderr, args[1:])
	case "forget":
		return forget(stdout, stderr, args[1:])
	case internalReadSocketTokenCmd:
		return readSocketTokenInternal(stdout)
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "sshakku: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

// shellInit resolves and creates the per-user runtime layout, drives the fixed
// socket to a healthy ssh-agent, then prints the result as shell assignments for
// the login entrypoint to eval:
//
//	agent_sock='…'
//	agent_lock='…'
//	log_file='…'
//
// agent_sock is the live socket EnsureAgent settled on, which may be an adopted
// agent rather than the fixed path. Only these assignments go to stdout;
// diagnostics and anomalies go to stderr and the session log.
func shellInit(stdout, stderr io.Writer) int {
	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())
	if err := paths.Ensure(layout); err != nil {
		// Best-effort: the log dir may be the very thing we failed to create.
		_ = sessionlog.New(layout.LogFile).Log("ERROR", fmt.Sprintf("shell-init: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	paths.CleanupLegacyAgentDir(env.Home)

	liveSock, code := runEnsure(stderr, env, layout)
	if code != 0 {
		return code
	}

	assignments := []struct{ name, value string }{
		{"agent_sock", liveSock},
		{"agent_lock", layout.AgentLock},
		{"log_file", layout.LogFile},
	}
	for _, a := range assignments {
		if _, err := fmt.Fprintf(stdout, "%s=%s\n", a.name, shellSingleQuote(a.value)); err != nil {
			_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
			return 1
		}
	}
	return 0
}

// ensureAgent resolves the runtime layout, drives the fixed socket to a healthy
// ssh-agent (starting, reaping, or adopting as needed), and prints the live
// socket as a shell assignment:
//
//	agent_sock='…'
//
// It is a standalone entry point for exercising the lifecycle; the login path
// reaches the same logic through shell-init, which adds the other assignments.
func ensureAgent(stdout, stderr io.Writer) int {
	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())
	if err := paths.Ensure(layout); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}

	liveSock, code := runEnsure(stderr, env, layout)
	if code != 0 {
		return code
	}
	if _, err := fmt.Fprintf(stdout, "agent_sock=%s\n", shellSingleQuote(liveSock)); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	return 0
}

// askpass answers an SSH_ASKPASS request. The proactive key-loading path stashes
// the passphrase in the @u keyring and points us at it via $SSHAKKU_KEYCTL_SERIAL;
// with a serial we serve that one-shot stash. Without one we are the reactive
// broker for an interactive ssh whose key has expired, and answer the prompt in
// args from the wallet (or the terminal).
func askpass(stdout io.Writer, args []string) int {
	if os.Getenv(keys.EnvKeyctlSerial) != "" {
		return askpassFromKeyring(stdout)
	}
	return askpassBroker(stdout, args)
}

// askpassBroker answers ssh's passphrase or confirmation prompt: a key passphrase
// comes from the wallet (or the terminal on a miss), other prompts pass through to
// the terminal. Only the reply goes to stdout; diagnostics go to the session log.
// It reads the config file for the wallet-store policy, so a miss-then-store
// refill honours the same include/exclude rules as load-keys.
func askpassBroker(stdout io.Writer, args []string) int {
	layout := paths.Resolve(paths.FromOS(), paths.ProbeDir)
	log := sessionlog.New(layout.LogFile)
	settings := loadSettings(layout, "askpass", log)
	secret, closeSecret := newSecretBackend(currentUser(), log, settings)
	defer closeSecret()
	broker := keys.Broker{
		Secret: secret,
		TTY:    ttyPrompter{},
		Log:    log,
		Config: keys.Config{WalletStore: settings.StoresWallet},
	}
	reply, ok := broker.Answer(strings.Join(args, " "))
	if !ok {
		return 1
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", reply); err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("askpass: write reply: %v", err))
		return 1
	}
	return 0
}

// askpassFromKeyring reads the passphrase the loader stashed in the @u keyring,
// identified by the serial in $SSHAKKU_KEYCTL_SERIAL, prints it on stdout for
// ssh-add, and unlinks the one-shot entry. The passphrase never touches stderr or
// argv; only the keyring serial crosses the environment. Diagnostics go to the
// session log alone, so the success path stays silent.
func askpassFromKeyring(stdout io.Writer) int {
	log := sessionlog.New(paths.Resolve(paths.FromOS(), paths.ProbeDir).LogFile)

	raw := os.Getenv(keys.EnvKeyctlSerial)
	serial, err := strconv.Atoi(raw)
	if err != nil {
		_ = log.Log("ERROR", "askpass: missing or malformed keyctl serial")
		return 1
	}

	pass, readErr := keyring.Read(keyring.Serial(serial))
	// One-shot: drop the entry whether or not the read succeeded, so a leaked
	// passphrase cannot linger in the keyring.
	_ = keyring.Unlink(keyring.Serial(serial))
	if readErr != nil {
		_ = log.Log("ERROR", fmt.Sprintf("askpass: read keyring serial …%s: %v", tail(raw, 3), readErr))
		return 1
	}
	if len(pass) == 0 {
		_ = log.Log("ERROR", fmt.Sprintf("askpass: empty passphrase for serial …%s", tail(raw, 3)))
		return 1
	}

	// ssh-add reads the passphrase from stdout and strips the trailing newline.
	if _, err := fmt.Fprintf(stdout, "%s\n", pass); err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("askpass: write passphrase: %v", err))
		return 1
	}
	_ = log.Log("INFO", fmt.Sprintf("askpass: provided passphrase for serial …%s", tail(raw, 3)))
	return 0
}

// tail returns the last n characters of s, for logging a key serial without
// recording it in full.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// loadKeys adds the user's ~/.ssh keys to the agent: it skips keys already loaded
// and, for the rest, pulls each passphrase from the secret store (or prompts) and
// hands it to ssh-add out of band. The login entrypoint calls it only in
// interactive shells. SSH_ASKPASS points at this very binary, which ssh-add re-execs
// to fetch the passphrase from the keyring. The success path is silent; problems go
// to the session log (and stderr for a hard failure).
func loadKeys(stderr io.Writer) int {
	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())
	log := sessionlog.New(layout.LogFile)

	self, err := os.Executable()
	if err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("load-keys: locate self: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}

	settings := loadSettings(layout, "load-keys", log)

	var giveupStore keys.GiveupStore
	if !settings.NoGiveup {
		giveupStore = giveup.Store{
			Dir: filepath.Join(filepath.Dir(layout.AgentSock), "giveup"),
			TTL: settings.GiveupTTL,
		}
	}
	keyStateStore := keystate.Store{Dir: keystateDir(layout)}

	var notifier keys.Notifier
	if !settings.Quiet {
		notifier = stderrNotifier{w: stderr}
	}

	runner := keys.ExecRunner{}
	guiEnv := keys.GUIEnv{
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		Display:        os.Getenv("DISPLAY"),
	}
	kdialog := keys.KDialogPrompter{Runner: runner}
	// The vault is always consulted first regardless of which of these is
	// picked (see Loader.loadViaVaultThenPrompt); this only chooses how to
	// ask when it misses — kdialog when a graphical session is usable,
	// otherwise the terminal, which needs no external binary.
	var prompter keys.Prompter = keys.TTYPrompter{}
	if keys.GUIAvailable(guiEnv, runner, kdialog) {
		prompter = kdialog
	}

	secret, closeSecret := newSecretBackend(currentUser(), log, settings)
	defer closeSecret()

	loader := keys.Loader{
		Keys:     keys.Enumerator{Dir: filepath.Join(env.Home, ".ssh")},
		Runner:   runner,
		Secret:   secret,
		Prompt:   prompter,
		Adder:    keys.ExecKeyAdder{AskpassProg: self, KeyLifetime: settings.KeyLifetime},
		Log:      log,
		Notify:   notifier,
		Giveup:   giveupStore,
		KeyState: keyStateStore,
		Config: keys.Config{
			MaxAttempts: settings.MaxAttempts,
			WalletStore: settings.StoresWallet,
			AutoLoad:    settings.AutoLoads,
			KeyLifetime: settings.KeyLifetime,
		},
	}
	if err := loader.LoadKeys(); err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("load-keys: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	return 0
}

// loadSettings reads the TOML config under layout's config dir, resolving it
// against the environment and built-in defaults (environment variable > file >
// default, per setting). config.toml loads first as the base; every *.toml
// file directly under config.d/, in filename order, is then merged on top of
// it, so a drop-in overrides a key config.toml set (see docs/CONFIGURATION.md).
// A missing file or dir is fine; a path, load, or parse problem — including
// one isolated to a single config.d file — is logged under tag and the
// affected setting falls back to its default.
func loadSettings(layout paths.Layout, tag string, log keys.Logger) config.Settings {
	configPath := filepath.Join(layout.ConfigDir, "config.toml")
	file, err := config.Load(configPath)
	if err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("%s: config %s: %v", tag, configPath, err))
	}

	confDDir := filepath.Join(layout.ConfigDir, "config.d")
	dirFile, dirErrs := config.LoadDir(confDDir)
	for _, e := range dirErrs {
		_ = log.Log("ERROR", fmt.Sprintf("%s: config %s: %v", tag, confDDir, e))
	}
	file = file.Merge(dirFile)

	settings, errs := config.Resolve(file, os.LookupEnv)
	for _, e := range errs {
		_ = log.Log("ERROR", e.Error())
	}
	return settings
}

// newSecretBackend opens the secret backend settings.SecretBackend selects
// (config.toml's secret_backend key; secret-service is the default). For
// secret-service it opens the native Secret Service client and wraps it in a
// SecretServiceBackend, which unlocks its own dedicated collection only for
// the duration of each lookup/store rather than relying on the desktop's
// idle timeout. If the session bus is unreachable (e.g. a headless session
// with no D-Bus user session) it logs the failure and falls back to
// SecretToolBackend, so a key can still be looked up or stored via the
// desktop's default collection rather than aborting the caller outright.
// 1password and bitwarden shell out to their own CLI instead, so neither
// touches D-Bus. The returned func releases any resource newSecretBackend
// opened (only the Secret Service client today) and must always be called.
func newSecretBackend(user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func()) {
	switch settings.SecretBackend {
	case config.SecretBackendKeychain:
		return newKeychainBackend(user), func() {}
	case config.SecretBackendOnePassword:
		return &keys.OnePasswordBackend{Runner: keys.ExecRunner{}, Vault: settings.OnePasswordVault}, func() {}
	case config.SecretBackendBitwarden:
		return &keys.BitwardenBackend{
			Runner:   keys.ExecRunner{},
			Prompter: newBitwardenPrompter(),
			Email:    settings.BitwardenEmail,
			Server:   settings.BitwardenServer,
		}, func() {}
	default:
		client, err := secretservice.NewClient()
		if err != nil {
			_ = log.Log("ERROR", fmt.Sprintf("secret service: %v; falling back to secret-tool", err))
			return keys.SecretToolBackend{Runner: keys.ExecRunner{}, User: user}, func() {}
		}
		return &keys.SecretServiceBackend{Client: client, User: user}, func() { _ = client.Close() }
	}
}

// bitwardenMasterPrompter asks for BitwardenBackend's master password: via the
// same graphical dialog as an SSH key's own passphrase when a display is
// available, otherwise on the controlling terminal. This is a separate prompt
// from the SSH key passphrase prompt — it never touches the wallet, and
// unlike a wallet-backed key's passphrase it cannot be silently skipped, so
// it needs a terminal fallback even where the SSH key prompt would just add
// the key on the terminal directly instead.
type bitwardenMasterPrompter struct {
	kdialog keys.KDialogPrompter
	gui     bool
}

func (p bitwardenMasterPrompter) Prompt(keyname string) (string, error) {
	if p.gui {
		return p.kdialog.Prompt(keyname)
	}
	return ttyPrompter{}.Prompt("Enter "+keyname, true)
}

func (bitwardenMasterPrompter) Available() bool { return true }

func newBitwardenPrompter() keys.Prompter {
	runner := keys.ExecRunner{}
	return bitwardenMasterPrompter{kdialog: keys.KDialogPrompter{Runner: runner}, gui: detectGUIAvailable()}
}

// stderrNotifier surfaces a user-facing notice to the terminal of the
// interactive shell that ran load-keys; $SSHAKKU_QUIET suppresses it.
type stderrNotifier struct{ w io.Writer }

func (n stderrNotifier) Notify(message string) {
	_, _ = fmt.Fprintf(n.w, "sshakku: %s\n", message)
}

// detectGUIAvailable reports whether a graphical passphrase prompt can be shown
// in this environment, by the same test askpassEnv and doctor's askpass finding
// both rely on: a reachable display server and an installed prompter.
func detectGUIAvailable() bool {
	runner := keys.ExecRunner{}
	guiEnv := keys.GUIEnv{
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		Display:        os.Getenv("DISPLAY"),
	}
	return keys.GUIAvailable(guiEnv, runner, keys.KDialogPrompter{Runner: runner})
}

// askpassEnv prints the export lines that route this shell's ssh passphrase
// prompts through sshakku's wallet-aware broker, so a key that expires from the
// agent is refilled from the wallet without a terminal prompt. It emits them
// only when a graphical prompter is available — a headless session keeps ssh's
// own terminal prompting — and the login entrypoint evals it in every login
// shell, interactive or not, since it only ever prints two export lines.
func askpassEnv(stdout, stderr io.Writer) int {
	if !detectGUIAvailable() {
		return 0
	}
	self, err := os.Executable()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	if _, err := io.WriteString(stdout, askpassExports(self)); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	return 0
}

// askpassExports returns the shell `export` lines pointing ssh's SSH_ASKPASS at
// self's wallet-aware broker; REQUIRE=prefer makes ssh consult it even with a tty.
func askpassExports(self string) string {
	return fmt.Sprintf(
		"export SSH_ASKPASS=%s\nexport SSH_ASKPASS_REQUIRE=prefer\nexport %s=1\n",
		shellSingleQuote(self), keys.EnvAskpassMode,
	)
}

// readSocketTokenInternal prints the calling process's own per-login socket
// token (see paths.ReadSocketToken) and nothing else, so a parent process that
// exec'd this as a child under another uid's credentials can capture that uid's
// token from stdout. It never creates a token: an unavailable or empty keyring
// prints an empty line, not an error, since "no token yet" is a valid, expected
// state (a tokenless layout) rather than a failure.
func readSocketTokenInternal(stdout io.Writer) int {
	_, _ = fmt.Fprintln(stdout, paths.ReadSocketToken())
	return 0
}

// targetUser identifies whose ssh-agent session `doctor` should report on.
// Source is "" for the invoking user themselves; otherwise it names how a
// different target was chosen, for the report header.
type targetUser struct {
	UID      int
	GID      int
	Username string
	Home     string
	Source   string
}

// resolveTargetUser decides whose session to diagnose: an explicit --user
// value (userArg), else a uid auto-detected from SUDO_UID when the invoking
// user is root, else the invoking user themselves. A target that turns out to
// be the invoking user (however specified) always gets Source == "", since
// nothing cross-user actually applies.
func resolveTargetUser(userArg string, selfEnv paths.Env) (targetUser, error) {
	lookup := func(nameOrUID, source string) (targetUser, error) {
		u, err := lookupUser(nameOrUID)
		if err != nil {
			return targetUser{}, err
		}
		if u.UID != selfEnv.UID {
			u.Source = source
		}
		return u, nil
	}

	if userArg != "" {
		u, err := lookup(userArg, "the --user flag")
		if err != nil {
			return targetUser{}, fmt.Errorf("--user %q: %w", userArg, err)
		}
		return u, nil
	}
	if selfEnv.UID == 0 {
		if sudoUID := os.Getenv("SUDO_UID"); sudoUID != "" {
			u, err := lookup(sudoUID, "SUDO_UID (auto-detected)")
			if err != nil {
				return targetUser{}, fmt.Errorf("SUDO_UID=%s: %w", sudoUID, err)
			}
			return u, nil
		}
	}
	return targetUser{UID: selfEnv.UID, Home: selfEnv.Home}, nil
}

// lookupUser resolves a username or uid string via the OS user database.
func lookupUser(nameOrUID string) (targetUser, error) {
	var u *user.User
	var err error
	if _, convErr := strconv.Atoi(nameOrUID); convErr == nil {
		u, err = user.LookupId(nameOrUID)
	} else {
		u, err = user.Lookup(nameOrUID)
	}
	if err != nil {
		return targetUser{}, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return targetUser{}, fmt.Errorf("parse uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return targetUser{}, fmt.Errorf("parse gid %q: %w", u.Gid, err)
	}
	return targetUser{UID: uid, GID: gid, Username: u.Username, Home: u.HomeDir}, nil
}

// crossUserGuard returns the refusal message for an operation that would touch
// another user's session, or "" when it may proceed. --fix must never run
// cross-user (docs/THREAT-MODEL.md E1: elevation is for read-only inspection,
// never for writing as root); reading another user's session requires euid 0,
// since only root can assume another uid's identity to read their socket token.
func crossUserGuard(target targetUser, fix, testBackend bool, euid int) string {
	if target.Source == "" {
		return ""
	}
	if fix {
		return fmt.Sprintf(
			"doctor --fix cannot act on another user's session (uid %d); run as that user instead, e.g.:\n  sudo -u %s -H sshakku doctor --fix",
			target.UID, target.Username)
	}
	if testBackend {
		return fmt.Sprintf(
			"doctor --test-backend cannot act on another user's session (uid %d); run as that user instead, e.g.:\n  sudo -u %s -H sshakku doctor --test-backend",
			target.UID, target.Username)
	}
	if euid != 0 {
		return fmt.Sprintf("diagnosing uid %d requires root privileges (e.g. run via sudo)", target.UID)
	}
	return ""
}

// doctor reports the ssh-agent situation: which agents are running, which one is
// ours, whether each answers, and whether this shell's SSH_AUTH_SOCK is wired to
// a healthy agent. Plain `doctor` inspects only and changes nothing. With --fix
// it then applies the same self-heal the login path runs (reap dead agents,
// start on the fixed socket, or adopt a healthy foreign one) and re-reports.
//
// --user <name|uid> diagnoses a different user's session instead of the
// invoking one (auto-detected from SUDO_UID when invoked as root via sudo with
// no --user given). This requires root, is read-only regardless of --fix (see
// crossUserGuard), and confirms the target's own fixed socket by reading their
// per-login token from their own kernel keyring — reached by re-executing this
// binary under their credentials (execTokenSource), never by guessing.
//
// --test-backend [name] actively exercises the named secret backend (or, when
// no name is given, whichever config.toml's secret_backend resolves to) end
// to end: store, look up, and delete a throwaway probe entry, so a
// misconfigured backend surfaces here instead of as a broken ssh prompt
// later. Refused cross-user for the same reason as --fix (see
// crossUserGuard): it acts, it does not just read.
func doctor(stdout, stderr io.Writer, args []string) int {
	fix := false
	testBackend := false
	var userArg, testBackendName string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--fix":
			fix = true
		case "--user":
			i++
			if i >= len(args) {
				_, _ = fmt.Fprintln(stderr, "sshakku: doctor: --user requires a value")
				return 2
			}
			userArg = args[i]
		case "--test-backend":
			testBackend = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				testBackendName = args[i]
			}
		default:
			_, _ = fmt.Fprintf(stderr, "sshakku: doctor: unknown argument %q\n", args[i])
			return 2
		}
	}
	if testBackendName != "" && !validSecretBackendName(testBackendName) {
		_, _ = fmt.Fprintf(stderr,
			"sshakku: doctor --test-backend: unknown backend %q (want secret-service, 1password, or bitwarden)\n", testBackendName)
		return 2
	}

	env := paths.FromOS()
	target, err := resolveTargetUser(userArg, env)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: doctor: %v\n", err)
		return 2
	}
	if msg := crossUserGuard(target, fix, testBackend, os.Geteuid()); msg != "" {
		_, _ = fmt.Fprintf(stderr, "sshakku: doctor: %s\n", msg)
		return 2
	}

	if target.Source != "" {
		return doctorCrossUser(stdout, stderr, env, target)
	}

	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())

	diagnose.Format(stdout, gatherReport(env, layout))

	exitCode := 0
	if testBackend {
		_, _ = io.WriteString(stdout, "\n── testing secret backend ──\n")
		log := sessionlog.New(layout.LogFile)
		exitCode = testSecretBackend(stdout, stderr, layout, log, testBackendName)
	}
	if !fix {
		return exitCode
	}

	// --fix heals agent state, but a child process cannot rewrite this shell's
	// SSH_AUTH_SOCK, so the current shell may still need a new login or an export.
	_, _ = io.WriteString(stdout, "\n── applying self-heal ──\n")
	if err := paths.Ensure(layout); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	paths.CleanupLegacyAgentDir(env.Home)
	liveSock, code := runEnsure(stderr, env, layout)
	if code != 0 {
		return code
	}

	_, _ = io.WriteString(stdout, "\nafter:\n\n")
	after := gatherReport(env, layout)
	diagnose.Format(stdout, after)
	if after.EnvSock != liveSock {
		_, _ = fmt.Fprintf(stdout,
			"\nthis shell still points elsewhere; open a new shell or run:\n  export SSH_AUTH_SOCK=%s\n",
			shellSingleQuote(liveSock))
	}
	return exitCode
}

// validSecretBackendName reports whether name is one of the secret backends
// newSecretBackend knows how to construct.
func validSecretBackendName(name string) bool {
	switch name {
	case config.SecretBackendSecretService, config.SecretBackendOnePassword, config.SecretBackendBitwarden:
		return true
	default:
		return false
	}
}

// doctorProbeService is the throwaway entry testSecretBackend stores, looks
// up, and deletes; distinguished from a real key's service id (which is
// always keys.DefaultServicePrefix + "-" + a key filename) so it can never
// collide with one.
const doctorProbeService = "sshakku-doctor-probe"

// testSecretBackend exercises name (or, when empty, settings.SecretBackend)
// end to end: store a throwaway probe entry, look it up back, and delete it,
// reporting a clear pass/fail for each step instead of a silent
// misconfiguration that only shows up as a broken ssh prompt later. The
// probe entry is always deleted before returning, even after a failure, so
// no leftover test data survives in the wallet. Returns 0 on a clean pass, 1
// on any failed step.
func testSecretBackend(stdout, stderr io.Writer, layout paths.Layout, log keys.Logger, name string) int {
	settings := loadSettings(layout, "doctor", log)
	if name == "" {
		name = settings.SecretBackend
	}
	settings.SecretBackend = name
	_, _ = fmt.Fprintf(stdout, "backend: %s\n", name)

	secret, closeSecret := newSecretBackend(currentUser(), log, settings)
	defer closeSecret()

	probe, err := randomProbeValue()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: doctor --test-backend: %v\n", err)
		return 1
	}
	return probeSecretBackend(stdout, log, secret, probe)
}

// probeSecretBackend runs the unlock/store/lookup/delete probe against
// secret, reporting a clear pass/fail per step. Split out from
// testSecretBackend so the probe logic is testable against a fake
// keys.SecretBackend, independent of which real backend newSecretBackend
// would construct. The probe entry is always deleted before returning, even
// after an earlier step failed, so no leftover test data survives in the
// wallet.
func probeSecretBackend(stdout io.Writer, log keys.Logger, secret keys.SecretBackend, probe string) int {
	if sess, ok := secret.(keys.SecretSession); ok {
		if err := sess.Unlock(); err != nil {
			_, _ = fmt.Fprintf(stdout, "  unlock: FAILED: %v\n", err)
			_, _ = fmt.Fprintln(stdout, "backend test: FAIL")
			return 1
		}
		_, _ = fmt.Fprintln(stdout, "  unlock: ok")
		defer func() {
			if err := sess.Lock(); err != nil {
				_ = log.Log("ERROR", fmt.Sprintf("doctor --test-backend: lock: %v", err))
			}
		}()
	}

	ok := true
	if err := secret.Store(doctorProbeService, "sshakku doctor test probe", probe); err != nil {
		_, _ = fmt.Fprintf(stdout, "  store: FAILED: %v\n", err)
		ok = false
	} else {
		_, _ = fmt.Fprintln(stdout, "  store: ok")
	}

	if ok {
		switch got, found, err := secret.Lookup(doctorProbeService); {
		case err != nil:
			_, _ = fmt.Fprintf(stdout, "  lookup: FAILED: %v\n", err)
			ok = false
		case !found:
			_, _ = fmt.Fprintln(stdout, "  lookup: FAILED: probe entry not found after storing it")
			ok = false
		case got != probe:
			_, _ = fmt.Fprintln(stdout, "  lookup: FAILED: value read back does not match what was stored")
			ok = false
		default:
			_, _ = fmt.Fprintln(stdout, "  lookup: ok")
		}
	}

	if err := secret.Delete(doctorProbeService); err != nil {
		_, _ = fmt.Fprintf(stdout, "  delete: FAILED: %v\n", err)
		ok = false
	} else {
		_, _ = fmt.Fprintln(stdout, "  delete: ok")
	}

	if ok {
		_, _ = fmt.Fprintln(stdout, "backend test: PASS")
		return 0
	}
	_, _ = fmt.Fprintln(stdout, "backend test: FAIL")
	return 1
}

// randomProbeValue returns a fresh random hex string for testSecretBackend's
// probe entry, so a lookup can only match what this run itself stored, never
// a leftover from an earlier one.
func randomProbeValue() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate probe value: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// doctorCrossUser reports on target's session instead of the invoking one.
// Read-only: crossUserGuard has already refused --fix and confirmed euid 0
// before this runs. It confirms target's own fixed socket by reading their
// per-login token from their own kernel keyring (execTokenSource), rather than
// guessing a path — an empty token is a valid "no agent started yet" state, not
// a failure.
func doctorCrossUser(stdout, stderr io.Writer, invoking paths.Env, target targetUser) int {
	token, err := (execTokenSource{}).ReadToken(target.UID, target.GID)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: doctor: %v\n", err)
		return 1
	}
	targetEnv := paths.Env{Home: target.Home, UID: target.UID}
	layout := paths.Resolve(targetEnv, paths.ProbeDirAs(target.UID)).WithSocketToken(token)

	_, _ = fmt.Fprintf(stdout,
		"diagnosing uid %d (%s) — chosen via %s; invoked as uid %d (root)\n"+
			"note: SSH_AUTH_SOCK and \"started by\" below describe %s's own session, not this shell's environment.\n\n",
		target.UID, target.Username, target.Source, invoking.UID, target.Username)

	// UIDGatedProber: root can dial any socket regardless of ownership, but that
	// isn't what "reachable" should mean for a report about target's session —
	// it must reflect what target could reach, not what this elevated caller
	// can bypass into.
	diagnose.Format(stdout, diagnose.Gather(diagnose.Inputs{
		FixedSock: layout.AgentSock,
		LegacyDir: filepath.Join(targetEnv.Home, ".ssh", "agent"),
		StatePath: filepath.Join(filepath.Dir(layout.AgentSock), "agent.state"),
		LogFile:   layout.LogFile,
		OurUID:    target.UID,
	}, agent.Inspector{}, agent.UIDGatedProber{UID: target.UID, Prober: agent.SocketProber{}}, diagnose.ProcfsAncestry{}, diagnose.ProcfsCgroup{},
		nil, // the keys section only covers the invoking user's own ~/.ssh (see gatherReport)
		newHostSource(targetEnv.Home),
	))
	return 0
}

// keystateDir is where load-keys records each key's added-at/lifetime state,
// so doctor can later report it; shared so both sides agree on the path. It
// sits alongside the giveup dir, under the per-login runtime directory
// (tmpfs, wiped on logout/reboot — see internal/keystate).
func keystateDir(layout paths.Layout) string {
	return filepath.Join(filepath.Dir(layout.AgentSock), "keystate")
}

// gatherReport builds the diagnostic report for the resolved layout, reading the
// real procfs, sockets, and process tree. Both the read-only and --fix paths use
// it so they present the situation identically.
func gatherReport(env paths.Env, layout paths.Layout) diagnose.Report {
	runner := keys.ExecRunner{}
	keySource := &diagnose.KeySource{
		Lister:      keys.Enumerator{Dir: filepath.Join(env.Home, ".ssh")},
		Fingerprint: keys.RunnerFingerprinter{Runner: runner},
		State:       keystate.Store{Dir: keystateDir(layout)},
	}
	return diagnose.Gather(diagnose.Inputs{
		FixedSock:         layout.AgentSock,
		LegacyDir:         filepath.Join(env.Home, ".ssh", "agent"),
		StatePath:         filepath.Join(filepath.Dir(layout.AgentSock), "agent.state"),
		EnvSock:           os.Getenv("SSH_AUTH_SOCK"),
		LogFile:           layout.LogFile,
		OurUID:            env.UID,
		GUIAvailable:      detectGUIAvailable(),
		EnvAskpass:        os.Getenv("SSH_ASKPASS"),
		EnvAskpassRequire: os.Getenv("SSH_ASKPASS_REQUIRE"),
	}, agent.Inspector{}, agent.SocketProber{}, diagnose.ProcfsAncestry{}, diagnose.ProcfsCgroup{}, keySource,
		newHostSource(env.Home))
}

// forget deletes stored passphrases: either the named keys, or every entry
// sshakku manages with --all. Argument validation happens before any secret
// backend is opened, so a usage error never touches the D-Bus session bus.
func forget(stdout, stderr io.Writer, args []string) int {
	all := false
	var names []string
	for _, a := range args {
		if a == "--all" {
			all = true
			continue
		}
		names = append(names, a)
	}
	switch {
	case all && len(names) > 0:
		_, _ = fmt.Fprintln(stderr, "sshakku: forget: --all cannot be combined with key names")
		return 2
	case !all && len(names) == 0:
		_, _ = fmt.Fprintln(stderr, "sshakku: forget: specify one or more key names, or --all")
		return 2
	}

	layout := paths.Resolve(paths.FromOS(), paths.ProbeDir)
	log := sessionlog.New(layout.LogFile)
	settings := loadSettings(layout, "forget", log)
	secret, closeSecret := newSecretBackend(currentUser(), log, settings)
	defer closeSecret()

	// forget always touches the secret store (listing and/or deleting), so —
	// unlike load-keys, which unlocks lazily since some keys may need no wallet
	// access at all — it unlocks once up front for the whole operation instead
	// of once per List/Delete call.
	if sess, ok := secret.(keys.SecretSession); ok {
		if err := sess.Unlock(); err != nil {
			_ = log.Log("ERROR", fmt.Sprintf("forget: unlock secret store: %v", err))
		} else {
			defer func() {
				if err := sess.Lock(); err != nil {
					_ = log.Log("ERROR", fmt.Sprintf("forget: lock secret store: %v", err))
				}
			}()
		}
	}

	var services []string
	if all {
		list, err := secret.List()
		if err != nil {
			if errors.Is(err, keys.ErrListUnsupported) {
				_, _ = fmt.Fprintln(stderr, "sshakku: forget --all needs the native Secret Service backend; name keys explicitly instead")
			} else {
				_, _ = fmt.Fprintf(stderr, "sshakku: forget: %v\n", err)
			}
			return 1
		}
		services = list
	} else {
		services = make([]string, len(names))
		for i, name := range names {
			services[i] = keys.DefaultServicePrefix + "-" + name
		}
	}

	fail := false
	for _, service := range services {
		if err := secret.Delete(service); err != nil {
			_, _ = fmt.Fprintf(stderr, "sshakku: forget %s: %v\n", service, err)
			_ = log.Log("ERROR", fmt.Sprintf("forget %s: %v", service, err))
			fail = true
			continue
		}
		_, _ = fmt.Fprintf(stdout, "forgot %s\n", service)
		_ = log.Log("INFO", fmt.Sprintf("forgot %s", service))
	}
	if fail {
		return 1
	}
	return 0
}

// currentUser returns the login name for the secret-store "username" attribute,
// matching $USER so entries the earlier shell version stored are still found.
func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// runEnsure drives the fixed socket to a healthy ssh-agent for the resolved
// layout, serialising concurrent logins on the start lock and reporting
// anomalies and errors to stderr and the session log. It returns the live socket
// to expose and a process exit code (0 on success). shell-init and ensure-agent
// share it so the login path and the standalone command drive the agent
// identically; each caller prints the assignments it needs.
func runEnsure(stderr io.Writer, env paths.Env, layout paths.Layout) (string, int) {
	log := sessionlog.New(layout.LogFile)
	m := agent.Manager{
		Prober:    agent.SocketProber{},
		Inspector: agent.Inspector{},
		Runner:    agent.ExecRunner{},
		Signaler:  agent.SysSignaler{},
		Locker:    agent.FlockLocker{Wait: agentLockWait},
	}
	cfg := agent.EnsureConfig{
		FixedSock: layout.AgentSock,
		LegacyDir: filepath.Join(env.Home, ".ssh", "agent"),
		StatePath: filepath.Join(filepath.Dir(layout.AgentSock), "agent.state"),
		LockPath:  layout.AgentLock,
		OurUID:    env.UID,
	}

	res, err := m.EnsureAgent(cfg, log)
	if err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("ensure-agent: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return "", 1
	}
	if res.Anomaly != "" {
		_, _ = fmt.Fprintf(stderr, "sshakku: %s\n", res.Anomaly)
	}
	return res.LiveSock, 0
}

// shellSingleQuote wraps s in single quotes safe for POSIX shell eval, so paths
// containing spaces or metacharacters survive intact.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
