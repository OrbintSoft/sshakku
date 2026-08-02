package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keystate"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

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

// userLookupID and userLookup resolve a uid or username via the OS user
// database; seams so lookupUser's uid/gid parse-failure branches are testable
// with a fabricated entry the real database would never return.
var (
	userLookupID = user.LookupId
	userLookup   = user.Lookup
)

// lookupUser resolves a username or uid string via the OS user database.
func lookupUser(nameOrUID string) (targetUser, error) {
	var u *user.User
	var err error
	if _, convErr := strconv.Atoi(nameOrUID); convErr == nil {
		u, err = userLookupID(nameOrUID)
	} else {
		u, err = userLookup(nameOrUID)
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
func (d deps) doctor(stdout, stderr io.Writer, args []string) int {
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
	if testBackendName != "" && !config.SecretBackendAvailable(testBackendName) {
		// The wallets are listed from the one place that knows them, so this
		// never offers the user a name this system has not got.
		_, _ = fmt.Fprintf(stderr,
			"sshakku: doctor --test-backend: %q is not a wallet this system has (want %s)\n",
			testBackendName, strings.Join(config.SecretBackends(), ", "))
		return 2
	}

	env := paths.FromOS()
	target, err := resolveTargetUser(userArg, env)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: doctor: %v\n", err)
		return 2
	}
	if msg := crossUserGuard(target, fix, testBackend, d.geteuid()); msg != "" {
		_, _ = fmt.Fprintf(stderr, "sshakku: doctor: %s\n", msg)
		return 2
	}

	if target.Source != "" {
		return d.doctorCrossUser(stdout, stderr, env, target)
	}

	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())

	// The wallet is described after the agent report is gathered, not inside
	// it: which wallets exist and what each one needs is this package's
	// knowledge, not the diagnose package's.
	report := d.gather(env, layout)
	report.Wallet = walletView(loadSettings(layout, "doctor", sessionlog.New(layout.LogFile)), realWalletProbe())
	report.Findings = append(report.Findings, diagnose.WalletFindings(report.Wallet)...)
	diagnose.Format(stdout, report)

	exitCode := 0
	if testBackend {
		_, _ = io.WriteString(stdout, "\n── testing secret backend ──\n")
		log := sessionlog.New(layout.LogFile)
		exitCode = d.testSecretBackend(stdout, stderr, layout, log, testBackendName)
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
	liveSock, code := d.runEnsure(stderr, env, layout)
	if code != 0 {
		return code
	}

	_, _ = io.WriteString(stdout, "\nafter:\n\n")
	after := d.gather(env, layout)
	diagnose.Format(stdout, after)
	if after.EnvSock != liveSock {
		_, _ = fmt.Fprintf(stdout,
			"\nthis shell still points elsewhere; open a new shell or run:\n  export SSH_AUTH_SOCK=%s\n",
			shellSingleQuote(liveSock))
	}
	return exitCode
}

// doctorProbeService is the throwaway entry testSecretBackend stores, looks up,
// and deletes. It is a fixed name rather than one built from the configured
// service prefix, so that probing a backend cannot address, and then delete, the
// entry of a key someone actually has: a probe named after the prefix would
// collide with a key called "doctor-probe", and a stored passphrase is not
// something a diagnostic may overwrite to find out whether the wallet works.
//
// The cost of the fixed name is that a probe left behind by a process killed
// between the store and the delete is not named by the prefix, so `forget --all`
// does not see it in a store where that prefix is what marks sshakku's entries.
const doctorProbeService = "sshakku-doctor-probe"

// testSecretBackend exercises name (or, when empty, settings.SecretBackend)
// end to end: store a throwaway probe entry, look it up back, and delete it,
// reporting a clear pass/fail for each step instead of a silent
// misconfiguration that only shows up as a broken ssh prompt later. The
// probe entry is always deleted before returning, even after a failure, so
// no leftover test data survives in the wallet. Returns 0 on a clean pass, 1
// on any failed step.
func (d deps) testSecretBackend(stdout, stderr io.Writer, layout paths.Layout, log keys.Logger, name string) int {
	settings := loadSettings(layout, "doctor", log)
	if name == "" {
		name = settings.SecretBackend
	}
	settings.SecretBackend = name
	_, _ = fmt.Fprintf(stdout, "backend: %s\n", name)

	secret, closeSecret := d.newSecret(currentUser(), log, settings)
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

// randRead is the probe RNG, a seam so randomProbeValue's read-failure branch
// (which crypto/rand practically never takes) can be exercised.
var randRead = rand.Read

// randomProbeValue returns a fresh random hex string for testSecretBackend's
// probe entry, so a lookup can only match what this run itself stored, never
// a leftover from an earlier one.
func randomProbeValue() (string, error) {
	b := make([]byte, 16)
	if _, err := randRead(b); err != nil {
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
func (d deps) doctorCrossUser(stdout, stderr io.Writer, invoking paths.Env, target targetUser) int {
	token, err := d.tokenSource.ReadToken(target.UID, target.GID)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: doctor: %v\n", err)
		return 1
	}
	targetEnv := paths.Env{Home: target.Home, UID: target.UID}
	layout := paths.Resolve(targetEnv, paths.ProbeDirAs(target.UID)).WithSocketToken(token)

	_, _ = fmt.Fprintf(stdout,
		"diagnosing uid %d (%s) — chosen via %s; invoked as uid %d (root)\n"+
			"note: the agent and \"started by\" below describe %s's own session. Their environment "+
			"is not readable from here, so no variable of theirs is shown and nothing is concluded "+
			"from one.\n\n",
		target.UID, target.Username, target.Source, invoking.UID, target.Username)

	// UIDGatedProber: root can dial any socket regardless of ownership, but that
	// isn't what "reachable" should mean for a report about target's session —
	// it must reflect what target could reach, not what this elevated caller
	// can bypass into.
	shownEnv, secretEnv := environmentNames()
	diagnose.Format(stdout, diagnose.Gather(diagnose.Inputs{
		FixedSock:     layout.AgentSock,
		LegacyDir:     filepath.Join(targetEnv.Home, ".ssh", "agent"),
		StatePath:     filepath.Join(filepath.Dir(layout.AgentSock), "agent.state"),
		LogFile:       layout.LogFile,
		OurUID:        target.UID,
		Env:           shownEnv,
		SecretEnv:     secretEnv,
		EnvUnreadable: true,
	}, agent.Inspector{}, agent.UIDGatedProber{UID: target.UID, Prober: agent.SocketProber{}}, newAncestrySource(), newCgroupSource(),
		nil, // the keys section only covers the invoking user's own ~/.ssh (see gatherReport)
		newHostSource(targetEnv.Home),
	))
	return 0
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
	shownEnv, secretEnv := environmentReport()
	return diagnose.Gather(diagnose.Inputs{
		FixedSock:         layout.AgentSock,
		LegacyDir:         filepath.Join(env.Home, ".ssh", "agent"),
		StatePath:         filepath.Join(filepath.Dir(layout.AgentSock), "agent.state"),
		EnvSock:           os.Getenv("SSH_AUTH_SOCK"),
		LogFile:           layout.LogFile,
		OurUID:            env.UID,
		EnvAskpass:        os.Getenv("SSH_ASKPASS"),
		EnvAskpassRequire: os.Getenv("SSH_ASKPASS_REQUIRE"),
		Env:               shownEnv,
		SecretEnv:         secretEnv,
	}, agent.Inspector{}, agent.SocketProber{}, newAncestrySource(), newCgroupSource(), keySource,
		newHostSource(env.Home))
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
