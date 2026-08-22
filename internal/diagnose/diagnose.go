// Package diagnose builds a read-only picture of the ssh-agent situation for the
// `sshakku doctor` command: which ssh-agent processes are running, which one (if
// any) is ours, whether each answers, and whether the shell's SSH_AUTH_SOCK is
// wired to a healthy agent. It only reads state — it never starts, signals, or
// reaps anything.
package diagnose

import (
	"context"
	"fmt"
	"time"

	"github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck"
	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
	"github.com/OrbintSoft/sshakku/internal/keystate"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"
)

// askpassNotWiredMsg is the finding text for the askpass-wiring check.
// loginShellHint (OS-specific: paths and the login-shell command differ
// between bash/profile.d on Linux and zsh/zprofile on macOS) is shared with
// the plain "no agent" finding below, since both stem from the same cause.
var askpassNotWiredMsg = "SSH_ASKPASS is not wired into this shell — once a key expires from the " +
	"agent, ssh will ask for its passphrase on the terminal instead of taking it from the wallet; " +
	loginShellHint

// envUnreadableMsg replaces every finding that would otherwise have been drawn
// from an environment variable, when the session being reported on is not this
// process's own. Those findings all read an empty string as a fact about the
// other user's shell; this says plainly that nobody looked.
const envUnreadableMsg = "this report describes another user's session, whose environment cannot be " +
	"read from here — nothing is claimed about their SSH_AUTH_SOCK or their askpass wiring, " +
	"and running sshakku doctor as that user is what would answer it"

// logTailLines is how many trailing session-log lines the report shows.
const logTailLines = 10

// now is the clock Format uses to render a loaded key's remaining time; a
// var, not a hard dependency, so tests can pin it.
var now = time.Now

// AgentSource enumerates the ssh-agent processes currently visible.
// inspect.Inspector satisfies it; tests supply a fake.
type AgentSource interface {
	Agents() ([]inspect.AgentProc, error)
}

// KeyLister lists the private-key files to consider; keys.Enumerator
// satisfies it.
type KeyLister interface {
	Keys() ([]string, error)
}

// KeyFingerprinter resolves a key file's fingerprint and the set currently
// loaded in the agent; keys.RunnerFingerprinter satisfies it.
type KeyFingerprinter interface {
	FileFingerprint(ctx context.Context, path string) (string, error)
	AgentFingerprints(ctx context.Context) (map[string]bool, error)
}

// KeyStateSource looks up the lifetime sshakku recorded for a key it added;
// keystate.Store satisfies it.
type KeyStateSource interface {
	Load(key string) (keystate.Record, bool)
}

// KeySource bundles the collaborators needed to inspect the user's keys and their
// agent/TTL state. A nil KeySource (the Gather parameter) skips the keys
// section entirely; a nil Lister field does the same.
type KeySource struct {
	// Dir is the directory Lister reads, for the report to name. The report
	// is how a user checks which directory SSHakku was told to look in, so it
	// must name that one and not the one it would otherwise have assumed.
	Dir         string
	Lister      KeyLister
	Fingerprint KeyFingerprinter
	State       KeyStateSource
}

// Inputs are the facts Gather reasons over, injected so it stays pure and
// testable — nothing here is read from the ambient process.
type Inputs struct {
	FixedSock string // the endpoint sessions are pointed at, as this system writes it
	// FixedSockPosix is that same endpoint in the writing a POSIX-emulating
	// shell can carry, where a system has two of them — empty where an
	// endpoint has only one writing, which is every system whose agent
	// listens on a socket. A session holding either is holding ours, and a
	// report that knew only one of them would call the other one somebody
	// else's agent.
	FixedSockPosix string
	LegacyDir      string // ~/.ssh/agent, for spotting a pre-sshakku agent
	StatePath      string // agent.state, holding the pid of the agent we started
	EnvSock        string // SSH_AUTH_SOCK as this shell sees it
	LogFile        string // session log to tail
	OurUID         int    // the invoking user's uid, to tell same-user agents apart

	// EnvAskpass and EnvAskpassRequire describe whether this shell's ssh
	// passphrase prompts are routed through sshakku's wallet-aware askpass
	// broker, which is what refills a key that has expired from the agent
	// without prompting.
	EnvAskpass        string // SSH_ASKPASS as this shell sees it
	EnvAskpassRequire string // SSH_ASKPASS_REQUIRE as this shell sees it

	// Env and SecretEnv are the variables SSHakku reads, as the report
	// presents them; the caller collects them, since which variables those
	// are belongs to the code that reads them.
	Env       []EnvVar
	SecretEnv []SecretEnvVar

	// EnvUnreadable says the environment described here is not this process's
	// own and could not be read — reporting on another user's session. The
	// zero value is the ordinary case, a report about the shell it runs in.
	// When it is set, every conclusion drawn from an environment variable is
	// withheld rather than drawn from an empty string: not knowing what a
	// shell exports is not evidence that it exports nothing.
	EnvUnreadable bool

	// NoAgentMechanism says this build has no way to keep an ssh-agent on the
	// system being reported on, which the caller knows and this does not go
	// and read. The zero value is the ordinary case, a system with one.
	NoAgentMechanism bool

	// LifetimeNotEnforceable says a key lifetime is configured that the agent
	// on the system being reported on cannot hold to. Both halves of that —
	// what is configured, and what this agent can do — are the caller's to
	// know, so it arrives as the one answer rather than as two facts to
	// combine here. The zero value is the ordinary case: either no lifetime
	// was asked for, or the agent honours the one that was.
	LifetimeNotEnforceable bool
}

// AgentView is one ssh-agent process as the report presents it.
type AgentView struct {
	PID       int
	UID       int // owning uid, or -1 when it could not be read
	Kind      inspect.ProcKind
	Socket    string
	Reachable bool
	Ancestry  []launcher.ProcInfo // the process chain that launched it, agent first
	Cgroup    string              // systemd unit the agent's cgroup names, or "" if none/unknown
}

// KeyView is one key file as the report presents it.
type KeyView struct {
	Name        string // base filename, e.g. "id_ed25519"
	Fingerprint string // "" when ssh-keygen could not read the file
	Loaded      bool   // whether Fingerprint is currently in the agent
	Tracked     bool   // whether sshakku recorded adding this key itself
	NoExpiry    bool   // Tracked, but recorded with no expiry (lifetime 0)
	ExpiresAt   time.Time
}

// Requirement is one thing the configured wallet needs in order to work, and
// whether it is here. Detail says where it was found, or what to do about it
// not being.
type Requirement struct {
	Name    string
	Detail  string
	Present bool
	// Undetermined marks a requirement that could not be settled by looking —
	// answering it would have taken an act, and the plain report performs
	// none. It is neither here nor missing, and Detail says what it would
	// take to find out.
	Undetermined bool
	// Fixable marks a piece this session could provide itself, which is what
	// makes it something `--fix` may go and provide. It is separate from
	// Present because the two answer different questions: a piece that is not
	// there yet and would appear by itself when first needed is nothing to
	// report as wrong, and is still something a user may reasonably ask for
	// now rather than later.
	Fixable bool
}

// WalletView is the configured secret backend as the report presents it: which
// wallet SSHakku would use, how it would be reached, and whether what that
// wallet needs is here.
//
// It is filled in by the caller rather than gathered here, because knowing
// which wallets exist and what each one needs belongs to the code that builds
// them; this package presents the answer, it does not work it out. A zero
// value prints nothing, which is what an invocation that never resolved a
// backend should say.
type WalletView struct {
	Backend      string
	Route        string // how the backend is reached, for the ones that offer a choice
	Requirements []Requirement
}

// Missing returns the requirements that are not satisfied and would stop the
// wallet working, in the order they were given.
func (w WalletView) Missing() []Requirement {
	var missing []Requirement
	for _, req := range w.Requirements {
		// An undetermined requirement is not a missing one: reporting it as a
		// problem would be stating as fact something nobody established. Nor is
		// one this session can provide itself: a piece that would appear the
		// first time it is needed is not something wrong with the machine, and
		// the report says separately that it is not there and how to have it
		// now.
		if !req.Present && !req.Undetermined && !req.Fixable {
			missing = append(missing, req)
		}
	}
	return missing
}

// walletBackendLine names the wallet, and the way it is reached when the
// backend offers more than one. A backend with a single way to be reached names
// no route, because there is nothing there for a user to have chosen wrongly.
func walletBackendLine(w WalletView) string {
	if w.Route == "" {
		return w.Backend
	}
	return fmt.Sprintf("%s  (route: %s)", w.Backend, w.Route)
}

// WalletFindings states each unmet requirement as a finding, so a wallet that
// cannot work shows up where a user looks for problems and not only in the
// section describing the wallet.
func WalletFindings(w WalletView) []string {
	var out []string
	for _, req := range w.Missing() {
		out = append(out, fmt.Sprintf(
			"the configured wallet (%s) needs %s: %s", w.Backend, req.Name, req.Detail))
	}
	return out
}

// Report is the read-only picture the doctor presents.
type Report struct {
	Wallet    WalletView
	FixedSock string
	// FixedReachable says whether an agent answers on the fixed endpoint. It
	// is asked of the endpoint itself rather than worked out from the process
	// list, because a system whose agent is a service has no process to find
	// and would otherwise be reported as having no agent while one answers.
	FixedReachable bool
	EnvSock        string
	EnvReachable   bool
	OurUID         int
	RecordedPID    int // pid from agent.state, 0 when absent or unreadable
	Agents         []AgentView
	State          State
	Findings       []string
	LogTail        []string
	InspectErr     error // enumeration failed; the report is partial
	Keys           []KeyView
	KeysDir        string // the directory Keys were read from, as the report names it
	KeysErr        error  // key enumeration failed; Keys is empty
	Host           hostcheck.Checks

	Env           []EnvVar
	SecretEnv     []SecretEnvVar
	EnvUnreadable bool // the environment shown is not this process's own (see Inputs)

	// NoAgentMechanism says the build being reported on has no way to keep an
	// ssh-agent here, which changes what there is to do about every state but
	// none of what is observed. Stated by the caller rather than read from the
	// machine, so both answers stay checkable from either one.
	NoAgentMechanism bool
}

// Gather inspects the agent situation described by in and returns the report,
// reading everything through src, prober, anc, and cg so it never touches the
// real /proc or sockets in a test. A nil anc skips ancestry attribution; a nil
// cg skips the cgroup fallback used when ancestry dead-ends at init. A nil
// keys skips the key/TTL section entirely. A nil host skips the
// environment-hardening section entirely (Report.Host stays its zero value,
// which Format and findings both already treat as "nothing to say"). It
// mutates nothing.
