package cli

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/keys/protect"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// protectKeysCmdName is what a user types to reach this, and what the command
// prints when it has to tell them how to reach it again with different
// arguments.
const protectKeysCmdName = "protect-keys"

// protectKeysRequest is what protect-keys was asked to do, once its arguments
// have been read.
type protectKeysRequest struct {
	// dryRun says to name what would be protected and change nothing.
	dryRun bool
	// directory says to cover the directory the keys are in as well, which is
	// what makes the next key generated there born protected — and what can
	// cost something, where that directory is one an SSH server reads.
	directory bool
}

// parseProtectKeysArgs reads the command's flags and writes its own refusals.
// The bool reports whether the arguments were usable.
func parseProtectKeysArgs(stderr io.Writer, args []string) (protectKeysRequest, bool) {
	var asked protectKeysRequest
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			asked.dryRun = true
		case "--directory":
			asked.directory = true
		default:
			_, _ = fmt.Fprintf(stderr, "sshakku: protect-keys: unknown argument %q\n", arg)
			return protectKeysRequest{}, false
		}
	}
	return asked, true
}

// protectKeys encrypts the private keys SSHakku is configured to load to the
// account running it, so that no other account on the machine — administrator
// or not, acting as itself — can read them, and neither can anyone holding the
// disk while that account is not signed in.
//
// It acts on the keys and, only when asked, on the directory holding them. That
// asymmetry is the whole design: covering a key file changes what can be read
// today, while covering a directory changes what every file made in it
// afterwards is born as, and where the directory is one an SSH server reads,
// one of those files is the authorized_keys that decides whether anybody can
// log in to this machine by key at all.
func (d deps) protectKeys(stdout, stderr io.Writer, args []string) int {
	asked, ok := parseProtectKeysArgs(stderr, args)
	if !ok {
		return 2
	}
	if d.keyProtection.scheme == "" {
		_, _ = fmt.Fprintln(stderr,
			"sshakku: protect-keys: on this system SSHakku can't encrypt a key file to a single "+
				"account, so nothing was changed")
		return 1
	}

	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir)
	log := sessionlog.New(layout.LogFile)
	enumerator := loadSettings(layout, protectKeysCmdName, log).KeyEnumerator(layout.Home)
	keyFiles, err := enumerator.Keys()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: protect-keys: %v\n", err)
		return 1
	}
	if len(keyFiles) == 0 && !asked.directory {
		_, _ = fmt.Fprintf(stdout, "No keys to protect in %s\n", enumerator.Dir)
		return 0
	}

	plan := protect.PlanFor(enumerator.Dir, keyFiles, asked.directory, authorizedKeysIn(enumerator.Dir))
	survey := protect.Take(d.keyProtection.scheme, plan.Targets, d.keyProtection.look)
	pending := survey.Pending()
	protectKeysListing(stdout, survey, asked.dryRun)

	// What covering the directory costs is only ahead of this run where the
	// directory is still to be covered. One that is covered already has had
	// whatever effect it has, and asking about it now would be asking about
	// something that has happened.
	cost := plan.Cost
	if !slices.Contains(pending, cost.Dir) {
		cost = protect.Cost{}
	}
	if cost.NeedsWarning() {
		protectKeysCost(stdout, cost)
	}
	if asked.dryRun {
		if cost.NeedsConfirmation() {
			_, _ = fmt.Fprintln(stdout, "\nWithout --dry-run, this stops here and asks you to confirm.")
		}
		return 0
	}
	if len(pending) == 0 {
		return 0
	}
	if cost.NeedsConfirmation() && !protectKeysConfirmed(d.stdin, stdout) {
		_, _ = fmt.Fprintf(stdout,
			"Nothing was changed. If you only want the key files encrypted, run:\n  sshakku %s\n",
			protectKeysCmdName)
		return 1
	}
	return protectKeysOutcome(stdout, stderr, log,
		protect.Apply(pending, d.keyProtection.apply, d.keyProtection.look))
}

// authorizedKeysIn returns the path of the authorized_keys sitting in dir, or
// "" where there is none.
//
// A path that cannot be looked at for any other reason counts as there. What
// follows from its presence is a question rather than an action, and being
// asked one question too many is not the outcome worth guarding against here.
func authorizedKeysIn(dir string) string {
	path := protect.AuthorizedKeysIn(dir)
	if _, err := os.Lstat(path); err != nil && os.IsNotExist(err) {
		return ""
	}
	return path
}

// protectKeysListing prints what the run found and what it is about to do. It
// is the same listing either way, so that a dry run is the real run with its
// last step left out rather than a different report of a different thing.
func protectKeysListing(stdout io.Writer, survey protect.Survey, dryRun bool) {
	if already := survey.Already(); len(already) > 0 {
		_, _ = fmt.Fprintf(stdout, "already protected with %s:\n", survey.Scheme)
		protectKeysPaths(stdout, already)
	}
	pending := survey.Pending()
	if len(pending) == 0 {
		_, _ = fmt.Fprintln(stdout, "Nothing left to protect.")
		return
	}
	// The heading states an intention rather than progress, because a run that
	// has to stop and ask has not protected anything yet when this is printed,
	// and must not read as though it had.
	heading := "to be protected with"
	if dryRun {
		heading = "would be protected with"
	}
	_, _ = fmt.Fprintf(stdout, "%s %s:\n", heading, survey.Scheme)
	protectKeysPaths(stdout, pending)
}

func protectKeysPaths(stdout io.Writer, paths []string) {
	for _, path := range paths {
		_, _ = fmt.Fprintf(stdout, "  %s\n", path)
	}
}

// The two warnings, in files of their own because they are prose: one line per
// paragraph, wrapped by whatever terminal they land in rather than at a width
// chosen here, and editable without touching any code.
//
// They say different things because the situations differ in what is actually
// at risk. Encrypting a directory changes what files *created* in it afterwards
// are born as, and nothing else: an authorized_keys already sitting there goes
// on being readable, through appends and through edits made in place. So where
// there is no such file yet, the first one added is born encrypted; where there
// is one, it keeps working until the day something deletes and recreates it.
var (
	//go:embed protect-keys-warning.txt
	protectKeysWarning string
	//go:embed protect-keys-confirm.txt
	protectKeysConfirmWarning string
)

// protectKeysCost prints the warning for whichever situation this is, under a
// headline naming it.
func protectKeysCost(stdout io.Writer, cost protect.Cost) {
	if cost.AuthorizedKeys != "" {
		_, _ = fmt.Fprintf(stdout, "\nWARNING: your SSH server reads this directory.\n"+
			"  There's an authorized_keys in it: %s\n\n%s", cost.AuthorizedKeys, protectKeysConfirmWarning)
		return
	}
	_, _ = fmt.Fprintf(stdout, "\nWARNING: %s is where an SSH server looks for authorized_keys.\n\n%s",
		cost.Dir, protectKeysWarning)
}

// protectKeysConfirmed asks, in as many words, and reports whether the answer
// was yes.
//
// Anything else is no: another word, an empty line, a standard input with
// nobody behind it. The question is asked because going on wrongly stops key
// logins into the machine, and a default of yes would be the same as never
// having asked.
func protectKeysConfirmed(stdin io.Reader, stdout io.Writer) bool {
	_, _ = fmt.Fprint(stdout, "\nType \"yes\" to encrypt the directory anyway: ")
	if stdin == nil {
		_, _ = fmt.Fprintln(stdout)
		return false
	}
	answer, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && answer == "" {
		_, _ = fmt.Fprintln(stdout)
		return false
	}
	_, _ = fmt.Fprintln(stdout)
	return strings.EqualFold(strings.TrimSpace(answer), "yes")
}

// protectKeysOutcome reports what the run achieved, as the filesystem answered
// afterwards rather than as the calls reported, and returns the exit code.
func protectKeysOutcome(stdout, stderr io.Writer, log *sessionlog.Logger, res protect.Result) int {
	for _, path := range res.Protected {
		_ = log.Log("INFO", "protect-keys: protected "+path)
	}
	_, _ = fmt.Fprintf(stdout, "\nProtected %d of %d.\n", len(res.Protected), len(res.Protected)+len(res.Failed))
	if len(res.Failed) == 0 {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "Could not protect:")
	for _, path := range res.Failed {
		_, _ = fmt.Fprintf(stderr, "  %s: %s\n", path, res.Reasons[path])
		_ = log.Log("ERROR", fmt.Sprintf("protect-keys: %s: %s", path, res.Reasons[path]))
	}
	return 1
}
