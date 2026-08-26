package install

import (
	"errors"
	"fmt"
	"slices"
)

// The refusals a profile choice can meet, each holding the invariant head of
// its sentence so the value that was asked for keeps its place in the line.
var (
	errNoProfileForHosts       = errors.New("no profile is selected by hosts")
	errNoProfileForCombination = errors.New("no profile is wired for scope")
)

// noProfileNamedError is a host that answered the query but left the profile
// for the scope and hosts asked empty. Writing to the empty path would create
// a file called "" wherever the install ran, and report it as the wiring.
type noProfileNamedError struct {
	hosts Hosts
	scope Scope
}

func (e noProfileNamedError) Error() string {
	return fmt.Sprintf("this host named no %s profile for %s sessions, so there is nothing to wire", e.hosts, e.scope)
}

// Scope says whose sessions a wiring is for.
type Scope string

const (
	// User wires the account running the install, and needs no privilege.
	User Scope = "user"
	// Machine wires every account on the system, and needs one.
	Machine Scope = "machine"
)

// Hosts says which PowerShell hosts read the wiring. A host is one program
// embedding the interpreter — the console, the ISE, an editor's terminal — and
// each keeps a profile of its own beside the one they all share.
type Hosts string

const (
	// AllHosts wires the profile every host of the interpreter reads. It is the
	// default because a person who installs from a console window and then
	// opens their editor's terminal means both.
	AllHosts Hosts = "all"
	// CurrentHost wires only the profile of the host that was asked, for
	// somebody who means exactly that one.
	CurrentHost Hosts = "current"
)

// ParseScope reads a `--scope` value, refusing one nobody serves rather than
// carrying an unusable value down to whatever would have written a file.
func ParseScope(name string) (Scope, error) {
	for _, scope := range []Scope{User, Machine} {
		if name == string(scope) {
			return scope, nil
		}
	}
	return "", unknownScope(Scope(name))
}

// ParseHosts reads a `--hosts` value.
func ParseHosts(name string) (Hosts, error) {
	for _, hosts := range []Hosts{AllHosts, CurrentHost} {
		if name == string(hosts) {
			return hosts, nil
		}
	}
	return "", fmt.Errorf("%w %q; hosts is %q or %q", errNoProfileForHosts, name, AllHosts, CurrentHost)
}

// ProfileFor names the one file a PowerShell install writes into, out of the
// five the interpreter reports.
//
// The file is the host's own answer rather than a path assembled here: a
// Documents folder redirected into OneDrive, a Store or portable installation,
// and two versions side by side each put it somewhere a template would get
// wrong.
func ProfileFor(host Host, scope Scope, hosts Hosts) (string, error) {
	var profile string
	switch {
	case scope == User && hosts == AllHosts:
		profile = host.Profiles.CurrentUserAllHosts
	case scope == User && hosts == CurrentHost:
		profile = host.Profiles.CurrentUserCurrentHost
	case scope == Machine && hosts == AllHosts:
		profile = host.Profiles.AllUsersAllHosts
	case scope == Machine && hosts == CurrentHost:
		profile = host.Profiles.AllUsersCurrentHost
	default:
		return "", fmt.Errorf("%w %q and hosts %q;"+
			" scope is %q or %q, and hosts is %q or %q",
			errNoProfileForCombination, scope, hosts, User, Machine, AllHosts, CurrentHost)
	}

	// An interpreter that named no profile for the combination asked has not
	// answered it. Writing to the empty path would create a file called "" in
	// whatever directory the install happened to be run from, and report it as
	// the wiring.
	if profile == "" {
		return "", noProfileNamedError{hosts: hosts, scope: scope}
	}
	return profile, nil
}

// EveryProfile lists every profile file this host reports, which is what an
// uninstall has to sweep.
//
// An install writes one file, but not always the same one: a person may have
// installed for the current host and be uninstalling with the default, or have
// installed twice with different flags. Removing only the file the current
// flags select would leave a hook behind that keeps running and is no longer
// mentioned by anything.
//
// The list is deduplicated, because these five are five names and not always
// five files: what `$PROFILE` alone means is the current-host one under most
// hosts, and reporting a file twice would have an uninstall say it removed a
// wiring it had already removed.
func EveryProfile(host Host) []string {
	var profiles []string
	for _, profile := range []string{
		host.Profiles.Default,
		host.Profiles.CurrentUserAllHosts,
		host.Profiles.CurrentUserCurrentHost,
		host.Profiles.AllUsersAllHosts,
		host.Profiles.AllUsersCurrentHost,
	} {
		if profile != "" && !slices.Contains(profiles, profile) {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}
