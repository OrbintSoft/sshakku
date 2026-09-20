package move

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies F70. Everything here runs on every platform: what belongs to one
// system is what it expects of a key file, which arrives as a function.

// errFull stands for what a filesystem refuses a move with — a full disk, a
// file somebody has open, a directory that turned out not to be writable.
var errFull = errors.New("no space left on device")

// nothingThere is a filesystem with none of the paths asked about in it.
func nothingThere(string) bool { return false }

// theseExist answers for a fixed set and says no to everything else.
func theseExist(paths ...string) func(string) bool {
	return func(path string) bool { return slices.Contains(paths, path) }
}

// permitted records what each moved path was given, so a test can assert that
// a key and its public half are not treated the same.
type permitted map[string]Kind

func (p permitted) permit(path string, kind Kind) error {
	p[path] = kind
	return nil
}

// renamerOf moves paths in a map standing for a filesystem, and refuses the
// ones named. It is what lets a failure happen at a chosen point.
func renamerOf(files map[string]bool, refuse ...string) func(from, to string) error {
	return func(from, to string) error {
		if slices.Contains(refuse, from) {
			return errFull
		}
		delete(files, from)
		files[to] = true
		return nil
	}
}

func TestAKeyMovesWithItsPublicHalf(t *testing.T) {
	from := filepath.FromSlash("/home/alice/.ssh")
	to := filepath.FromSlash("/home/alice/keys")
	key := filepath.Join(from, "id_ed25519")

	p := PlanFor(to, []string{key}, theseExist(key+".pub"))

	require.Len(t, p.Moves, 2)
	assert.Equal(t, Move{From: key, To: filepath.Join(to, "id_ed25519"), Kind: PrivateKey}, p.Moves[0])
	assert.Equal(t, Move{
		From: key + ".pub", To: filepath.Join(to, "id_ed25519.pub"), Kind: PublicKey,
	}, p.Moves[1], "and it follows its own key, not all the others")
}

func TestAKeyWithNoPublicHalfMovesAlone(t *testing.T) {
	key := filepath.FromSlash("/home/alice/.ssh/id_ed25519")

	p := PlanFor(filepath.FromSlash("/home/alice/keys"), []string{key}, nothingThere)

	require.Len(t, p.Moves, 1, "nothing is invented for a public half that is not there")
	assert.Equal(t, key, p.Moves[0].From)
}

// TestAuthorizedKeysIsNeverMoved. That file belongs to the SSH server rather
// than to the user, and moving it is what stops logins into this machine.
func TestAuthorizedKeysIsNeverMoved(t *testing.T) {
	from := filepath.FromSlash("/home/alice/.ssh")
	authorized := filepath.Join(from, authorizedKeys)
	key := filepath.Join(from, "id_ed25519")

	p := PlanFor(filepath.FromSlash("/home/alice/keys"), []string{key, authorized}, nothingThere)

	require.Len(t, p.Moves, 1)
	assert.Equal(t, key, p.Moves[0].From, "handed it as a key, it is still not moved")
}

// TestADestinationAlreadyTakenIsNamedBeforeAnythingMoves. A move onto an
// existing file overwrites it, and the file it would overwrite is somebody's
// key.
func TestADestinationAlreadyTakenIsNamedBeforeAnythingMoves(t *testing.T) {
	to := filepath.FromSlash("/home/alice/keys")
	key := filepath.FromSlash("/home/alice/.ssh/id_ed25519")
	taken := filepath.Join(to, "id_ed25519")

	p := PlanFor(to, []string{key}, theseExist(taken))

	assert.Equal(t, []string{taken}, p.Blocked(theseExist(taken)))
}

func TestNothingIsBlockedByAnEmptyDestination(t *testing.T) {
	key := filepath.FromSlash("/home/alice/.ssh/id_ed25519")

	p := PlanFor(filepath.FromSlash("/home/alice/keys"), []string{key}, nothingThere)

	assert.Empty(t, p.Blocked(nothingThere))
}

func TestApplyMovesEverythingAndPermitsEachForWhatItIs(t *testing.T) {
	from := filepath.FromSlash("/home/alice/.ssh")
	to := filepath.FromSlash("/home/alice/keys")
	key := filepath.Join(from, "id_ed25519")
	files := map[string]bool{key: true, key + ".pub": true}

	p := PlanFor(to, []string{key}, theseExist(key+".pub"))
	given := permitted{}
	res, err := Apply(p, renamerOf(files), given.permit)

	require.NoError(t, err)
	assert.Empty(t, res.Stranded)
	assert.False(t, files[key], "the key is not in the old directory afterwards — it moved, it was not copied")
	assert.True(t, files[filepath.Join(to, "id_ed25519")], "and it is in the new one")
	assert.Equal(t, PrivateKey, given[filepath.Join(to, "id_ed25519")])
	assert.Equal(t, PublicKey, given[filepath.Join(to, "id_ed25519.pub")],
		"a public half is not given what a private key is given")
}

// TestAFailedMovePutsBackWhatItAlreadyMoved is the promise that nothing is done
// by halves. A run that moved two keys and could not move the third leaves a
// user with keys in two places and a configuration naming one of them.
func TestAFailedMovePutsBackWhatItAlreadyMoved(t *testing.T) {
	from := filepath.FromSlash("/home/alice/.ssh")
	to := filepath.FromSlash("/home/alice/keys")
	first := filepath.Join(from, "id_first")
	stuck := filepath.Join(from, "id_stuck")
	files := map[string]bool{first: true, stuck: true}

	p := PlanFor(to, []string{first, stuck}, nothingThere)
	res, err := Apply(p, renamerOf(files, stuck), permitted{}.permit)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "id_stuck", "what stopped it is named")
	assert.Empty(t, res.Stranded, "and everything went back")
	assert.True(t, files[first], "the key that had already moved is where it started")
	assert.False(t, files[filepath.Join(to, "id_first")], "and not in the new directory")
}

// TestWhatCouldNotBePutBackIsNamed. Putting things back can fail too, and a
// user left with a key somewhere they did not ask for has to be told which one
// and where — this is the one outcome that needs a person.
func TestWhatCouldNotBePutBackIsNamed(t *testing.T) {
	from := filepath.FromSlash("/home/alice/.ssh")
	to := filepath.FromSlash("/home/alice/keys")
	first := filepath.Join(from, "id_first")
	stuck := filepath.Join(from, "id_stuck")
	moved := filepath.Join(to, "id_first")

	p := PlanFor(to, []string{first, stuck}, nothingThere)
	// The move of id_stuck fails, and so does putting id_first back.
	res, err := Apply(p, renamerOf(map[string]bool{}, stuck, moved), permitted{}.permit)

	require.Error(t, err)
	assert.Equal(t, []string{moved}, res.Stranded)
}

// TestAPermissionThatWouldNotTakeUndoesTheMoveToo. A key sitting in a new
// directory with the wrong permissions is a key OpenSSH will refuse, so a move
// that could not be finished is not a move worth keeping.
func TestAPermissionThatWouldNotTakeUndoesTheMoveToo(t *testing.T) {
	from := filepath.FromSlash("/home/alice/.ssh")
	to := filepath.FromSlash("/home/alice/keys")
	key := filepath.Join(from, "id_ed25519")
	files := map[string]bool{key: true}

	p := PlanFor(to, []string{key}, nothingThere)
	res, err := Apply(p, renamerOf(files), func(string, Kind) error { return errFull })

	require.Error(t, err)
	assert.Empty(t, res.Stranded)
	assert.True(t, files[key], "the key is back where it was")
	assert.Empty(t, res.Moved, "and nothing is reported moved")
}
