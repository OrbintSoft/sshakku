package install

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// queryHostScript asks one interpreter about itself. It is a real file rather
// than a string in this source so that it can be read, diffed and linted as
// the language it is written in.
//
//go:embed query-host.ps1
var queryHostScript []byte

// queryScriptMode keeps the borrowed copy readable only by the account running
// the query, which is also the account that will run it.
const queryScriptMode fs.FileMode = 0o600

// Profiles are the startup files one PowerShell interpreter reads, as that
// interpreter reports them. They are asked for rather than assembled: a
// Documents folder redirected into OneDrive, a Store or portable installation,
// and two versions installed side by side each put them somewhere a template
// would get wrong.
type Profiles struct {
	// Default is what `$PROFILE` alone means to this host, which is the
	// current-host one — worth having because it is what a person following
	// most instructions on the internet will have edited.
	Default                string `json:"default"`
	CurrentUserAllHosts    string `json:"currentUserAllHosts"`
	CurrentUserCurrentHost string `json:"currentUserCurrentHost"`
	AllUsersAllHosts       string `json:"allUsersAllHosts"`
	AllUsersCurrentHost    string `json:"allUsersCurrentHost"`
}

// Host is what one PowerShell interpreter said about itself. Every field is
// that interpreter's answer and no other's: the two Windows editions disagree
// about the execution policy of the same account, from separate registry keys,
// so each is asked for itself.
type Host struct {
	// Edition is "Desktop" for Windows PowerShell 5.x and "Core" for
	// PowerShell 6 and later.
	Edition string `json:"edition"`
	Version string `json:"version"`
	PSHome  string `json:"psHome"`
	// LanguageMode is "FullLanguage" on an ordinary machine. Anything else
	// can stop a dot-sourced hook from running.
	LanguageMode string `json:"languageMode"`
	// EffectiveExecutionPolicy is the one actually in force, which is the
	// first of the scopes below that is not "Undefined".
	EffectiveExecutionPolicy string            `json:"effectiveExecutionPolicy"`
	ExecutionPolicyByScope   map[string]string `json:"executionPolicyByScope"`
	Profiles                 Profiles          `json:"profiles"`
}

// AskHost runs the PowerShell interpreter at exe and returns what it reported
// about itself.
//
// The interpreter is started without its profile, so that a hook this program
// wired earlier is not running while it answers, and only its standard output
// is read. Windows PowerShell writes a CLIXML progress payload to standard
// error, which is not JSON and has no business in the answer.
func AskHost(ctx context.Context, exe string) (Host, error) {
	script, cleanup, err := queryScriptOnDisk()
	if err != nil {
		return Host{}, fmt.Errorf("laying out the query for %s: %w", exe, err)
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, exe, queryArguments(script)...)
	cmd.Env = childEnvironment(os.Environ())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		return Host{}, fmt.Errorf("asking %s about itself: %w%s", exe, err, explain(stderr.String()))
	}

	host, err := parseHost(stdout)
	if err != nil {
		return Host{}, fmt.Errorf("reading what %s said about itself: %w%s", exe, err, explain(stderr.String()))
	}
	return host, nil
}

// queryArguments is how the query is handed over: as the path of a readable
// script file, named on the command line in the clear.
//
// Do not replace this with -EncodedCommand, which is the obvious way to avoid
// laying a file down. What it hands the system is base64: unreadable to the
// administrator inspecting a process list, to the audit log, and to the
// endpoint protection deciding whether to allow it, whose behavioural
// heuristics flag that shape on sight and terminate the host mid-query. It is
// also the one form that cannot carry an Authenticode signature, and a signed
// script is what the strictest machines require before they will run one.
//
// A script file is governed by the execution policy where an encoded command is
// not. That is a feature here rather than a cost, because a policy refusing
// this file refuses the profile too — see explain.
func queryArguments(script string) []string {
	return []string{"-NoProfile", "-NonInteractive", "-File", script}
}

// queryScriptOnDisk puts the embedded query where an interpreter can read it,
// byte for byte as it is in the tree, and returns the path with the means to
// take it back. The byte-order mark travels with it: Windows PowerShell reads a
// script file without one in the machine's ANSI code page, which corrupts
// exactly the non-ASCII paths the query exists to report.
func queryScriptOnDisk() (string, func(), error) {
	dir, err := os.MkdirTemp("", "sshakku-query-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	path := filepath.Join(dir, "query-host.ps1")
	if err := os.WriteFile(path, queryHostScript, queryScriptMode); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

// parseHost reads the one line of JSON the query prints.
//
// Nothing at all is an error and not an empty answer. A host that ran the
// query and had nothing to say is not a thing that happens; a host that was
// handed something it could not run, on the other hand, exits successfully
// having printed nothing, and reporting that as a machine with no profiles
// would be worse than reporting it as a failure.
func parseHost(stdout []byte) (Host, error) {
	if len(bytes.TrimSpace(stdout)) == 0 {
		return Host{}, errors.New("it printed nothing")
	}
	var host Host
	if err := json.Unmarshal(stdout, &host); err != nil {
		return Host{}, fmt.Errorf("it did not answer with JSON: %w", err)
	}
	if host.Profiles.CurrentUserAllHosts == "" {
		return Host{}, errors.New("it named none of its profiles")
	}
	return host, nil
}

// inheritedFromOurOwnShell are the environment variables that describe the
// PowerShell which started this program, and which must not be handed to the
// PowerShell this program starts.
//
// PSModulePath says where one edition keeps its modules. Handed PowerShell
// 7's list, Windows PowerShell 5.x cannot resolve a module-qualified cmdlet at
// all: it reports that the module could not be auto-loaded and exits having
// printed nothing — the same shape of failure as every other way of getting
// the query wrong here.
//
// PSExecutionPolicyPreference is the process-scope execution policy this
// program was given. Passed on, the host reports it as its own, and the answer
// describes the session doing the asking instead of the sessions a user will
// start later, which are the ones that have to read the hook.
var inheritedFromOurOwnShell = []string{"PSModulePath", "PSExecutionPolicyPreference"}

// childEnvironment returns environ without the variables above. Names are
// matched without regard to case, because that is how the system this runs on
// treats them.
func childEnvironment(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if found && slices.ContainsFunc(inheritedFromOurOwnShell, func(drop string) bool {
			return strings.EqualFold(name, drop)
		}) {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// refusedByPolicy is the identifier PowerShell prints when the execution policy
// forbids a script file. Identifiers are not translated, unlike the message
// beside them, so this recognises the refusal on a machine in any language.
const refusedByPolicy = "UnauthorizedAccess"

// changeThePolicy is what it takes to make a host run its profile, said once so
// that both ways of finding out it will not — asking it and being refused, and
// asking it and being told — end in the same instruction.
const changeThePolicy = "Change it with `Set-ExecutionPolicy RemoteSigned -Scope CurrentUser`" +
	" and install again, or wire a different shell"

// explain turns what a host wrote to its standard error into something worth
// reading, for a message that would otherwise say only that something went
// wrong. Empty when it said nothing, so the message does not end in a dangling
// colon.
//
// One failure is named rather than merely quoted. A host whose execution policy
// refuses a script file will refuse its profile for the same reason, so a hook
// written into that profile would never run — which is the part a user has to
// know, and is not what the raw error appears to be about.
func explain(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}
	if strings.Contains(stderr, refusedByPolicy) {
		return ": its execution policy refuses to run a script file, which means it will not run" +
			" its profile either, so a hook written there would never start. " + changeThePolicy +
			". What it said: " + stderr
	}
	return ": " + stderr
}
