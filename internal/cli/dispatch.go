// Package cli is the sshakku command: subcommand dispatch, the askpass
// re-entry path when the binary is run under its askpass name, and the wiring
// that hands each command the system seams it needs.
//
// It is a package rather than the main package so that all of it can be
// exercised from tests — cmd/sshakku is the entry point and holds nothing but
// the process's single os.Exit.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/cli/backend"
	"github.com/OrbintSoft/sshakku/internal/cli/crossuser"
	"github.com/OrbintSoft/sshakku/internal/cli/dialog"
	"github.com/OrbintSoft/sshakku/internal/cli/walletcheck"
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keys/handoff"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/run"
)

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
  install        wire the login hook into one shell; uninstall takes it out.
                 With nothing else, the shell is the one you ran it from.
                 --shell <name> looks one up, --shell-exe <path> names the
                 interpreter to ask about itself, --profile <file> names the
                 file outright; --scope <user|machine>, --hosts <all|current>
                 for a PowerShell, --no-path to skip the environment step
  uninstall      take that wiring back out, with the same selectors
  help           show this help

shell-init, ensure-agent and askpass-env print lines for a shell to run, and
take --shell <posix|powershell> to say which shell is asking. Without it they
print the Bourne form every POSIX shell reads.
`

// Main runs one invocation and returns the process exit code. It is the whole
// program: the entry point does nothing but hand it the process's arguments and
// exit on what it returns.
func Main(ctx context.Context, stdout, stderr io.Writer, invokedAs string, args []string) int {
	return dispatch(ctx, realDeps(), stdout, stderr, invokedAs, args)
}

// deps carries the process's injectable system seams so the command bodies can
// be exercised against fakes in tests. Only dependencies that reach outside the
// process — the secret backend, whose default opens the D-Bus session bus, and
// the agent ensurer, which spawns/reaps real ssh-agents — live here; anything a
// command builds purely from its arguments stays inline. main wires it from the
// real implementations via realDeps.
type deps struct {
	// newSecret opens the secret backend settings.SecretBackend selects, with a
	// cleanup func that releases whatever it opened (see backend.Open).
	newSecret func(ctx context.Context, user string, log keys.Logger, settings config.Settings) (wallet.Backend, func())
	// ensurer drives the fixed socket to a healthy ssh-agent (see runEnsure). The
	// production value is the concrete agent.Manager; tests substitute a fake so
	// the shell-init/ensure-agent bodies run without spawning a real agent.
	ensurer agentEnsurer
	// gather builds the diagnostic report for a resolved layout (see
	// gatherReport), reading the real procfs, sockets, and process tree. Injected
	// so doctor's report-printing and self-heal paths run against a synthetic
	// report instead of this host's live agent state.
	gather func(ctx context.Context, env paths.Env, layout paths.Layout, settings config.Settings) diagnose.Report
	// tokenSource reads another user's per-login socket token for a cross-user
	// doctor run (see crossuser.Exec). The production value re-executes this
	// binary under the target's credentials; a fake lets doctorCrossUser run
	// without root or a second uid.
	tokenSource crossuser.Source
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
	// in production (dialog.Graphical): what counts as a usable graphical
	// session, and what draws the dialog, are each platform's own answers.
	graphicalPrompter func(context.Context, config.Settings, keys.Logger) prompt.Prompter
	// fetchHandoff redeems a one-shot passphrase-handoff token (handoff.Fetch).
	// Injected so askpass's handoff path is testable without a live kernel
	// keyring, which many containers and CI runners lack.
	fetchHandoff func(ctx context.Context, token string) (string, error)
	// tty is the reactive broker's terminal fallback when a passphrase prompt
	// misses the wallet (see askpassBroker). The production value reads
	// /dev/tty; a fake lets the broker's decline path run without a controlling
	// terminal.
	tty prompt.TTY
	// wallet describes the configured wallet as this machine actually is: which
	// one would be opened, how it would be reached, and what of that is here.
	// Injected so a test decides what the machine has instead of asking the one
	// it runs on — which for --fix matters twice over, since what the report
	// finds is what decides whether anything is created. Taking the finished
	// view rather than the probe behind it also keeps that decision checkable
	// from a machine whose wallet has no compartment at all.
	wallet func(ctx context.Context, settings config.Settings) diagnose.WalletView
	// runner runs the external programs a command drives directly rather than
	// through a component that brings its own — today the ssh-add that takes a
	// key whose lifetime has elapsed back out of the agent. Injected so a
	// session's expiry runs on a machine with neither an agent nor an ssh-add.
	runner run.Runner
	// makeCompartment creates the compartment the configured wallet keeps
	// SSHakku's entries in, and reports what it made. Nil where this system's
	// wallet has no such thing to make. Only --fix ever calls it.
	makeCompartment func(ctx context.Context, settings config.Settings) (string, error)
	// enableAgentService lets the agent's service be started again where
	// somebody has disabled it. Only --fix ever calls it, and only where the
	// report said the service is disabled — this writes, and what it writes
	// belongs to the machine rather than to one account. Injected so both
	// outcomes run on a machine where the real one would refuse, or where
	// there is no service at all.
	enableAgentService func(ctx context.Context) error
	// agentKeepsLifetimes is whether the agent on this system holds a key for a
	// stated time and drops it at that deadline itself. It decides two things a
	// session does: what lifetime a key is added with, and whether taking the
	// expired ones back out is this session's job at all. Injected so both
	// answers run from either machine, since which one a system gives is the
	// system's own (agent.KeepsLifetimes).
	agentKeepsLifetimes bool
}

// realDeps wires deps to the production implementations.
func realDeps() deps {
	return deps{
		newSecret:           backend.Open,
		ensurer:             realEnsurer(),
		gather:              gatherReport,
		tokenSource:         crossuser.Exec{},
		geteuid:             os.Geteuid,
		self:                os.Executable,
		graphicalPrompter:   dialog.Graphical,
		fetchHandoff:        handoff.Fetch,
		tty:                 dialog.TTY{},
		wallet:              walletcheck.View,
		runner:              run.ExecRunner{},
		makeCompartment:     walletcheck.MakeCompartment,
		enableAgentService:  agent.EnableAgentService,
		agentKeepsLifetimes: agent.KeepsLifetimes(),
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
	if invokedAsAskpass(invokedAs) {
		return d.askpass(ctx, stdout, args)
	}
	return d.run(ctx, stdout, stderr, args)
}

// askpassProg returns the path to hand ssh as its SSH_ASKPASS helper: the name
// above, carrying whatever this system puts at the end of a program's name, in
// the directory self was installed into. Derived from the running binary rather
// than fixed at build time, so a copy run from anywhere reaches the helper next
// to it and not one somewhere else on the system.
func askpassProg(self string) string {
	return filepath.Join(filepath.Dir(self), askpassProgName+programSuffix)
}

// invokedAsAskpass reports whether this process was run under the helper's
// name, which is what says the arguments are a prompt to answer rather than a
// command to run. The suffix is part of that name wherever this system gives
// programs one: the helper installed beside `sshakku.exe` is called
// `sshakku-askpass.exe`, and a comparison blind to that reads ssh's invocation
// of the helper as somebody running the binary itself — which answers a
// passphrase prompt with the usage text.
func invokedAsAskpass(invokedAs string) bool {
	return filepath.Base(invokedAs) == askpassProgName+programSuffix
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
	case installCmdName:
		return d.install(ctx, stdout, stderr, args[1:])
	case uninstallCmdName:
		return d.uninstall(ctx, stdout, stderr, args[1:])
	case crossuser.ReadSocketTokenCmd:
		return readSocketTokenInternal(stdout)
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "sshakku: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
