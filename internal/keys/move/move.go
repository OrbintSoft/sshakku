// Package move puts a user's ssh keys in a directory of their choosing, and
// leaves nothing half done.
//
// Moving a private key is three things at once — the file itself, the
// permissions the new place has to give it, and the configuration that says
// where the keys now are — and any two of them without the third leaves
// somebody worse off than before they started. So everything that can be
// refused is refused before the first file moves, and a failure part way
// through puts back what it already moved.
//
// What a system expects of a key file is the one thing that belongs to a
// platform, and it arrives here as a function. Everything that decides what to
// move, and in what order, is neutral.
package move

import (
	"fmt"
	"path/filepath"
	"slices"
)

// authorizedKeys is the one file never moved. It belongs to the SSH server
// rather than to the user: moving it is what stops logins into this machine,
// and it holds public keys, so there is nothing in it worth relocating.
const authorizedKeys = "authorized_keys"

// publicSuffix is what a private key's public half is called, beside it.
const publicSuffix = ".pub"

// Kind says what a path is, so that each system can give it what it expects.
type Kind int

const (
	// Directory is the directory the keys are going into.
	Directory Kind = iota
	// PrivateKey is a key file, which no other account may read.
	PrivateKey
	// PublicKey is a key's public half.
	PublicKey
)

// Move is one file going from where it is to where it is wanted.
type Move struct {
	From string
	To   string
	Kind Kind
}

// Plan is every file a run would move, worked out before anything moves.
type Plan struct {
	// Dir is the directory everything is going into.
	Dir string
	// Moves are the files, each private key followed by its public half, so a
	// run interrupted between the two has moved the halves of one key rather
	// than the private halves of all of them.
	Moves []Move
}

// PlanFor works out what moving keys into dir would move: each private key and,
// where one is beside it, its public half.
//
// there answers whether a path exists, which is a question for a filesystem;
// this decides what follows from the answer rather than asking it.
func PlanFor(dir string, keys []string, there func(path string) bool) Plan {
	p := Plan{Dir: dir}
	for _, key := range keys {
		if filepath.Base(key) == authorizedKeys {
			continue
		}
		p.Moves = append(p.Moves, Move{From: key, To: filepath.Join(dir, filepath.Base(key)), Kind: PrivateKey})
		if public := key + publicSuffix; there(public) {
			p.Moves = append(p.Moves, Move{
				From: public,
				To:   filepath.Join(dir, filepath.Base(public)),
				Kind: PublicKey,
			})
		}
	}
	return p
}

// Blocked names the destinations already taken, in the order they were planned.
//
// A move onto an existing file would overwrite it, and the file it would
// overwrite is somebody's key. Nothing in this package overwrites anything: a
// destination that is taken stops the whole run before it starts.
func (p Plan) Blocked(there func(path string) bool) []string {
	var taken []string
	for _, m := range p.Moves {
		if there(m.To) {
			taken = append(taken, m.To)
		}
	}
	return taken
}

// Result is what a run achieved.
type Result struct {
	// Moved are the files now in the new directory, in the order they went.
	Moved []string
	// Stranded are files a failed run could not put back where they were. It is
	// empty even for most failures, and when it is not, it is the only part of
	// this that somebody has to act on by hand.
	Stranded []string
}

// Apply moves every file in the plan and gives each one what this system
// expects of it, putting back what it already moved if one of them fails.
//
// The permissions are set after the move rather than before, because a file's
// permissions are set on the file and the file is not there yet. The directory
// is the caller's to make and to permit: it has to exist before anything can be
// moved into it, and it has to be refused before anything is.
func Apply(p Plan, rename func(from, to string) error, permit func(path string, kind Kind) error) (Result, error) {
	var res Result
	var done []Move

	for _, m := range p.Moves {
		if err := rename(m.From, m.To); err != nil {
			res.Stranded = putBack(done, rename)
			return res, fmt.Errorf("move %s: %w", m.From, err)
		}
		done = append(done, m)
		res.Moved = append(res.Moved, m.To)

		if err := permit(m.To, m.Kind); err != nil {
			res.Stranded = putBack(done, rename)
			return Result{Stranded: res.Stranded}, fmt.Errorf("set the permissions of %s: %w", m.To, err)
		}
	}
	return res, nil
}

// putBack returns the moved files to where they came from, and names the ones
// it could not. It works backwards, so the most recent move — the one most
// likely to be the reason anything is wrong — is undone first.
func putBack(done []Move, rename func(from, to string) error) []string {
	var stranded []string
	for _, m := range slices.Backward(done) {
		if err := rename(m.To, m.From); err != nil {
			stranded = append(stranded, m.To)
		}
	}
	return stranded
}
