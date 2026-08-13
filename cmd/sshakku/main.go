// Command sshakku tends the SSH agent: it computes the per-user runtime
// paths, keeps the agent healthy, and loads keys with passphrases pulled from
// the OS secret store. The login shell wires it in by evaluating its output:
//
//	eval "$(sshakku shell-init)"
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// internalReadSocketTokenCmd is not a user-facing command: `doctor` runs the
// binary with this as its argument, as a child holding another user's credentials,
// to read that user's per-login socket token from their own kernel keyring (a
// keyring is only visible to the uid that owns it, unlike files, which root can
// read regardless of owner). It is deliberately absent from usage/--help.
const internalReadSocketTokenCmd = "__read-socket-token"

// askpassProgName is the name this binary answers ssh's passphrase prompts
// under, installed beside it as a link. ssh execs its SSH_ASKPASS helper as a
// single program with only the prompt as an argument, so a flag or a subcommand
// cannot be passed to say what the invocation is for; the name is what says it,
// and it is set by whoever runs the program rather than inferred from what they
// ran it with.
const askpassProgName = "sshakku-askpass"

const usage = `sshakku — SSH agent and key shepherd

usage: sshakku <command>

commands:
  shell-init     drive the agent healthy and print shell assignments to eval
  ensure-agent   drive the agent to a healthy state and print agent_sock
  load-keys      add the user's ssh keys to the agent (interactive sessions)
  askpass-env    print exports routing ssh's askpass through sshakku
  config         print the configuration in force and where each value came from;
                 --edit opens your own config.toml in $EDITOR
  doctor         report the ssh-agent situation; --fix applies the self-heal;
                 --user <name|uid> reports on another user's session (root only,
                 read-only; auto-detected from SUDO_UID under sudo);
                 --test-backend [name] stores/looks up/deletes a throwaway
                 probe entry in the named (or configured) secret backend
  forget         delete stored passphrases: <keyname>... or --all
  help           show this help

shell-init, ensure-agent and askpass-env print lines for a shell to run, and
take --shell <posix|powershell> to say which shell is asking. Without it they
print the Bourne form every POSIX shell reads.
`

func main() {
	// The process entry point: a single os.Exit around the testable dispatch,
	// with nothing here to unit-test (a test can't observe os.Exit).
	//
	// The context created here is the program's only root. Everything that
	// waits on something outside this process — the session bus, ssh-add, a
	// wallet's CLI, a socket — derives its own from the one passed down, so a
	// deadline or a cancellation set above a wait reaches the wait itself. A
	// second root anywhere below would be where that stops being true.
	//coverage:ignore
	os.Exit(dispatch(context.Background(), realDeps(), os.Stdout, os.Stderr, os.Args[0], os.Args[1:]))
}

// deps carries the process's injectable system seams so the command bodies can
// be exercised against fakes in tests. Only dependencies that reach outside the
// process — the secret backend, whose default opens the D-Bus session bus, and
// the agent ensurer, which spawns/reaps real ssh-agents — live here; anything a
// command builds purely from its arguments stays inline. main wires it from the
// real implementations via realDeps.
type deps struct {
	// newSecret opens the secret backend settings.SecretBackend selects, with a
	// cleanup func that releases whatever it opened (see newSecretBackend).
	newSecret func(ctx context.Context, user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func())
	// ensurer drives the fixed socket to a healthy ssh-agent (see runEnsure). The
	// production value is the concrete agent.Manager; tests substitute a fake so
	// the shell-init/ensure-agent bodies run without spawning a real agent.
	ensurer agentEnsurer
	// gather builds the diagnostic report for a resolved layout (see
	// gatherReport), reading the real procfs, sockets, and process tree. Injected
	// so doctor's report-printing and self-heal paths run against a synthetic
	// report instead of this host's live agent state.
	gather func(env paths.Env, layout paths.Layout, settings config.Settings) diagnose.Report
	// tokenSource reads another user's per-login socket token for a cross-user
	// doctor run (see execTokenSource). The production value re-executes this
	// binary under the target's credentials; a fake lets doctorCrossUser run
	// without root or a second uid.
	tokenSource TargetTokenSource
	// geteuid reports the process's effective uid, which doctor's cross-user
	// guard consults to decide whether elevation is present. Injected so a test
	// can drive the root-only cross-user path without actually being root.
	geteuid func() int
	// self reports this binary's own path (os.Executable), which askpass-env
	// bakes into the SSH_ASKPASS export lines. Injected so the lookup's
	// error branch is testable.
	self func() (string, error)
	// graphicalPrompter returns the dialog this platform can raise a passphrase
	// prompt with, or nil when none can be shown here — which is what decides
	// whether load-keys asks through a dialog or on the terminal. Injected so
	// both branches run regardless of what the test host has, and platform-bound
	// in production (newGraphicalPrompter): what counts as a usable graphical
	// session, and what draws the dialog, are each platform's own answers.
	graphicalPrompter func(context.Context, config.Settings, keys.Logger) keys.Prompter
	// fetchHandoff redeems a one-shot passphrase-handoff token (keys.FetchHandoff).
	// Injected so askpass's handoff path is testable without a live kernel
	// keyring, which many containers and CI runners lack.
	fetchHandoff func(ctx context.Context, token string) (string, error)
	// tty is the reactive broker's terminal fallback when a passphrase prompt
	// misses the wallet (see askpassBroker). The production value reads
	// /dev/tty; a fake lets the broker's decline path run without a controlling
	// terminal.
	tty keys.TTY
	// wallet describes the configured wallet as this machine actually is: which
	// one would be opened, how it would be reached, and what of that is here.
	// Injected so a test decides what the machine has instead of asking the one
	// it runs on — which for --fix matters twice over, since what the report
	// finds is what decides whether anything is created. Taking the finished
	// view rather than the probe behind it also keeps that decision checkable
	// from a machine whose wallet has no compartment at all.
	wallet func(ctx context.Context, settings config.Settings) diagnose.WalletView
	// makeCompartment creates the compartment the configured wallet keeps
	// SSHakku's entries in, and reports what it made. Nil where this system's
	// wallet has no such thing to make. Only --fix ever calls it.
	makeCompartment func(ctx context.Context, settings config.Settings) (string, error)
}

// realDeps wires deps to the production implementations.
func realDeps() deps {
	return deps{
		newSecret:         newSecretBackend,
		ensurer:           realEnsurer(),
		gather:            gatherReport,
		tokenSource:       execTokenSource{},
		geteuid:           os.Geteuid,
		self:              os.Executable,
		graphicalPrompter: newGraphicalPrompter,
		fetchHandoff:      keys.FetchHandoff,
		tty:               ttyPrompter{},
		wallet:            realWalletView,
		makeCompartment:   realMakeCompartment,
	}
}

// dispatch routes an invocation either to the SSH_ASKPASS helper path or to
// normal subcommand dispatch, returning the process exit code. It is the whole
// body of main, split out so main stays a single os.Exit call and everything
// here can be tested without spawning the process.
//
// invokedAs is the path the process was run under (argv[0]). ssh execs the
// helper by the path it was given, and a person types the binary's own name, so
// the two callers arrive under different names and there is nothing to work
// out: args are a prompt to answer, or a subcommand, and never both.
func dispatch(ctx context.Context, d deps, stdout, stderr io.Writer, invokedAs string, args []string) int {
	if filepath.Base(invokedAs) == askpassProgName {
		return d.askpass(ctx, stdout, args)
	}
	return d.run(ctx, stdout, stderr, args)
}

// askpassProg returns the path to hand ssh as its SSH_ASKPASS helper: the name
// above, in the directory self was installed into. Derived from the running
// binary rather than fixed at build time, so a copy run from anywhere reaches
// the helper next to it and not one somewhere else on the system.
func askpassProg(self string) string {
	return filepath.Join(filepath.Dir(self), askpassProgName)
}

// run dispatches a subcommand and returns the process exit code. Output goes to
// the supplied writers so the command is testable without touching real stdio.
func (d deps) run(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "shell-init":
		return d.shellInit(ctx, stdout, stderr, args[1:])
	case "ensure-agent":
		return d.ensureAgent(ctx, stdout, stderr, args[1:])
	case "load-keys":
		return d.loadKeys(ctx, stderr)
	case "askpass-env":
		return d.askpassEnv(stdout, stderr, args[1:])
	case "config":
		return d.config(ctx, stdout, stderr, args[1:])
	case "doctor":
		return d.doctor(ctx, stdout, stderr, args[1:])
	case "forget":
		return d.forget(ctx, stdout, stderr, args[1:])
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
