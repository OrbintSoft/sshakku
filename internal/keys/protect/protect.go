// Package protect answers whether a private key on disk is readable only by
// the account that owns it, and turns that on where a system offers it.
//
// Nothing here is about a passphrase. A key file protected this way is still
// the same key with the same passphrase; what changes is that another account
// on the machine — including one with administrative rights, acting as itself —
// cannot read the bytes at all, and neither can anyone holding the disk while
// the owner is not logged in.
//
// Which systems can do this, and how, is the one thing that belongs to a
// platform: it arrives here as a function that answers about a path. Everything
// that reasons over those answers is neutral and takes them as arguments, so
// both a system that has a scheme and one that does not stay checkable from
// either machine.
package protect

import (
	"errors"
	"path/filepath"
)

// ErrNoSchemeHere is what a look or an attempt answers on a system this build
// has no way to protect a key on. It is not a failure to be reported as one:
// the caller says so plainly and changes nothing.
var ErrNoSchemeHere = errors.New("this build has no way to protect a key file on this system")

// Look answers whether one path is protected, or nil where that could not be
// told. It is the seam a survey is taken through.
type Look func(path string) (*bool, error)

// authorizedKeys is the one file in a key directory that is deliberately left
// alone. It holds public keys, so there is nothing in it to protect, and it is
// read at login by a service running as the system rather than as the account —
// which has no way to read it protected. Nothing would be gained and a working
// login is what would be risked.
const authorizedKeys = "authorized_keys"

// serverKeyDirName is the directory under a home that an SSH server reads a
// user's authorized_keys from unless it has been told otherwise. It is a fact
// about the server rather than about where SSHakku looks for keys — the two
// coincide by default and are free to diverge, which is the whole point of
// being able to move the keys elsewhere.
const serverKeyDirName = ".ssh"

// AuthorizedKeysIn names the authorized_keys file of dir, whether or not one is
// there. Whether it is there is a question for a filesystem; what the name is,
// is this package's to know, so that nobody deciding what covering a directory
// would cost has to spell it out again.
func AuthorizedKeysIn(dir string) string { return filepath.Join(dir, authorizedKeys) }

// Survey is what one look at a set of paths found.
type Survey struct {
	// Scheme names what this system protects a path with, empty where it has
	// none this build can use.
	Scheme string
	// Paths is each path's answer, in the order they were given.
	Paths []PathProtection
}

// PathProtection is one path's answer.
type PathProtection struct {
	Path      string
	Protected *bool
}

// Result is what an attempt actually achieved, read back off the filesystem
// rather than taken from what the attempt returned.
type Result struct {
	// Protected are the paths that came back protected afterwards.
	Protected []string
	// Failed are the paths that did not, whether the attempt said so or not.
	Failed []string
	// Skipped are the paths deliberately left alone.
	Skipped []string
	// Reasons carries what went wrong, by path, for the paths that have one.
	Reasons map[string]string
}

// Take surveys paths through look. A path that could not be told about is
// carried as nil rather than as a no: the difference between a path nobody
// could ask about and one found unprotected is the difference between a report
// worth acting on and one worth ignoring.
func Take(scheme string, paths []string, look Look) Survey {
	s := Survey{Scheme: scheme}
	for _, path := range paths {
		s.Paths = append(s.Paths, PathProtection{Path: path, Protected: answerFor(look, path)})
	}
	return s
}

// answerFor reduces a look to the answer or to not knowing. Why it could not be
// told does not change what a survey does with it.
func answerFor(look Look, path string) *bool {
	answer, err := look(path)
	if err != nil {
		return nil
	}
	return answer
}

// Already names the paths that came back protected before anything was done,
// in the order they were given.
func (s Survey) Already() []string {
	var done []string
	for _, p := range s.Paths {
		if p.Protected != nil && *p.Protected {
			done = append(done, p.Path)
		}
	}
	return done
}

// Pending names the paths an attempt still has to be made for: every one the
// survey did not come back certain was protected already.
//
// A path nobody could answer for is among them, which is the one judgement in
// here. Attempting is cheap and turning it on twice changes nothing, while
// leaving a key alone because a look failed is how a key stays in the clear
// with a run behind it that reported success.
func (s Survey) Pending() []string {
	var todo []string
	for _, p := range s.Paths {
		if p.Protected == nil || !*p.Protected {
			todo = append(todo, p.Path)
		}
	}
	return todo
}

// Cost is what covering a key directory itself would cost. The zero value is
// what an ordinary run costs: protecting the key files alone changes nothing
// about any file that does not exist yet, and so costs nothing.
type Cost struct {
	// Dir is the directory that would be covered, empty where none is.
	Dir string
	// ServerReads says whether that is the directory an SSH server reads a
	// user's authorized_keys from unless it has been told otherwise.
	ServerReads bool
	// AuthorizedKeys is the path of the authorized_keys actually sitting in it,
	// empty where there is none. The file itself is in no danger — covering a
	// directory does nothing to the files already in it — but its presence is
	// what says an SSH server really does read this directory, which turns the
	// cost from something that might apply into something that does.
	AuthorizedKeys string
}

// NeedsWarning reports whether covering this directory is something the user
// has to be told the cost of before it happens.
func (c Cost) NeedsWarning() bool {
	return c.Dir != "" && (c.ServerReads || c.AuthorizedKeys != "")
}

// NeedsConfirmation reports whether being told is not enough, and the run has
// to stop and be answered before it goes on.
func (c Cost) NeedsConfirmation() bool { return c.AuthorizedKeys != "" }

// Plan is what a run would change and what changing it would cost, worked out
// before anything is changed — so the run that says what it would do and the
// run that does it reach the same decision the same way.
type Plan struct {
	// Targets are the paths to protect, in the order they are to be done.
	Targets []string
	// Cost is what covering the directory costs, and is the zero value where
	// the directory is not among the targets.
	Cost Cost
}

// PlanFor works out what protecting keys — and, where coverDir says so, the
// directory holding them — would change.
//
// authorizedKeysThere is the path of the authorized_keys actually sitting in
// dir, empty where there is none: whether a file is there is a question for a
// filesystem, and this decides what follows from the answer rather than asking
// it.
func PlanFor(dir string, keys []string, coverDir bool, authorizedKeysThere string) Plan {
	var p Plan
	if coverDir {
		// The directory first, so that a key generated between this run and the
		// next is born protected even where a key below will not budge.
		p.Targets = append(p.Targets, dir)
		p.Cost = Cost{
			Dir:            dir,
			ServerReads:    filepath.Base(dir) == serverKeyDirName,
			AuthorizedKeys: authorizedKeysThere,
		}
	}
	for _, key := range keys {
		if filepath.Base(key) == authorizedKeys {
			continue
		}
		p.Targets = append(p.Targets, key)
	}
	return p
}

// Apply protects each target in order through do, then reads every path back
// through look to say what actually happened.
//
// The reading back is the point rather than a precaution. The call that turns
// this on can report success and change nothing — a directory holding a
// read-only file is documented as doing exactly that — so a result taken from
// the return value would tell a reader their keys are covered on the say-so of
// a call that did not cover them.
func Apply(targets []string, do func(path string) error, look Look) Result {
	res := Result{Reasons: map[string]string{}}

	for _, target := range targets {
		// A plan does not put this file among the targets; the guard is here so
		// that no caller can, whatever it worked out or was handed.
		if filepath.Base(target) == authorizedKeys {
			res.Skipped = append(res.Skipped, target)
			continue
		}
		if err := do(target); err != nil {
			res.Reasons[target] = err.Error()
		}
		switch answer, err := look(target); {
		case err == nil && answer != nil && *answer:
			res.Protected = append(res.Protected, target)
		default:
			res.Failed = append(res.Failed, target)
			if _, said := res.Reasons[target]; !said {
				res.Reasons[target] = "the attempt reported success and the path is still not protected"
			}
		}
	}
	return res
}
