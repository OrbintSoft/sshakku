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

// Survey is what one look at a key directory found.
type Survey struct {
	// Scheme names what this system protects a key with, empty where it has
	// none this build can use.
	Scheme string
	// DirProtected is the directory's own answer. A marked directory covers
	// the files created in it afterwards and does nothing for the ones already
	// there, which is why it is kept apart from the keys.
	DirProtected *bool
	// Keys is each key path's answer, in the order they were given.
	Keys []KeyProtection
}

// KeyProtection is one key file's answer.
type KeyProtection struct {
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

// Take surveys dir and keys through look. A path that could not be told about
// is carried as nil rather than as a no: the difference between a key nobody
// could ask about and a key found unprotected is the difference between a
// report worth acting on and one worth ignoring.
func Take(scheme, dir string, keys []string, look Look) Survey {
	s := Survey{Scheme: scheme, DirProtected: answerFor(look, dir)}
	for _, key := range keys {
		s.Keys = append(s.Keys, KeyProtection{Path: key, Protected: answerFor(look, key)})
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

// Unprotected names the keys that came back as definitely not protected. A key
// nobody could answer for is not among them.
func (s Survey) Unprotected() []string {
	var bare []string
	for _, key := range s.Keys {
		if key.Protected != nil && !*key.Protected {
			bare = append(bare, key.Path)
		}
	}
	return bare
}

// Complete says whether the directory and every key came back protected.
//
// Both halves are required, and each fails in its own way. Keys protected under
// an unmarked directory means the next key generated there will not be, with
// nothing said at the time. A marked directory over unprotected keys means the
// report reads as done while the keys that exist are readable.
func (s Survey) Complete() bool {
	if s.DirProtected == nil || !*s.DirProtected {
		return false
	}
	for _, key := range s.Keys {
		if key.Protected == nil || !*key.Protected {
			return false
		}
	}
	return true
}

// Nothing says whether this survey established anything at all, which is what a
// system with no scheme for this produces.
func (s Survey) Nothing() bool {
	if s.DirProtected != nil {
		return false
	}
	for _, key := range s.Keys {
		if key.Protected != nil {
			return false
		}
	}
	return true
}

// Apply protects dir and each key through do, then reads every path back
// through look to say what actually happened.
//
// The reading back is the point rather than a precaution. The call that turns
// this on can report success and change nothing — a directory holding a
// read-only file is documented as doing exactly that — so a result taken from
// the return value would tell a reader their keys are covered on the say-so of
// a call that did not cover them.
func Apply(dir string, keys []string, do func(path string) error, look Look) Result {
	res := Result{Reasons: map[string]string{}}

	// The directory first, so that a key generated between this run and the
	// next is born protected even if a key below refuses to budge.
	targets := []string{dir}
	for _, key := range keys {
		if filepath.Base(key) == authorizedKeys {
			res.Skipped = append(res.Skipped, key)
			continue
		}
		targets = append(targets, key)
	}

	for _, target := range targets {
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
