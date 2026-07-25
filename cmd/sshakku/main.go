// Command sshakku tends the SSH agent: it computes the per-user runtime
// paths, keeps the agent healthy, and loads keys with passphrases pulled from
// the OS secret store. The login shell wires it in by evaluating its output:
//
//	eval "$(sshakku shell-init)"
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

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
	// The process entry point: a single os.Exit around the testable dispatch,
	// with nothing here to unit-test (a test can't observe os.Exit).
	//coverage:ignore
	os.Exit(dispatch(realDeps(), os.Stdout, os.Stderr, os.Args[1:], os.Getenv(keys.EnvAskpassMode) != ""))
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
	newSecret func(user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func())
	// ensurer drives the fixed socket to a healthy ssh-agent (see runEnsure). The
	// production value is the concrete agent.Manager; tests substitute a fake so
	// the shell-init/ensure-agent bodies run without spawning a real agent.
	ensurer agentEnsurer
}

// realDeps wires deps to the production implementations.
func realDeps() deps {
	return deps{newSecret: newSecretBackend, ensurer: realEnsurer()}
}

// dispatch routes an invocation either to the SSH_ASKPASS helper path or to
// normal subcommand dispatch, returning the process exit code. It is the whole
// body of main, split out so main stays a single os.Exit call and everything
// here can be tested without spawning the process.
//
// ssh-add execs this binary as its SSH_ASKPASS program, passing only the prompt
// as an argument and marking the call via the environment (askpassEnvSet).
// Handle that before subcommand dispatch and return the stashed passphrase —
// unless args actually name one of our own subcommands, which always wins (see
// wantsAskpass).
func dispatch(d deps, stdout, stderr io.Writer, args []string, askpassEnvSet bool) int {
	if wantsAskpass(askpassEnvSet, args) {
		return d.askpass(stdout, args)
	}
	return d.run(stdout, stderr, args)
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
func (d deps) run(stdout, stderr io.Writer, args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "shell-init":
		return d.shellInit(stdout, stderr)
	case "ensure-agent":
		return d.ensureAgent(stdout, stderr)
	case "load-keys":
		return d.loadKeys(stderr)
	case "askpass-env":
		return askpassEnv(stdout, stderr)
	case "doctor":
		return d.doctor(stdout, stderr, args[1:])
	case "forget":
		return d.forget(stdout, stderr, args[1:])
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
