package protect

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies F69. Everything here runs on every platform: what belongs to one
// system is the answer about a path, which arrives as a function, and the
// reasoning over those answers is the same reasoning everywhere.

// errStuck is a refusal handed to the seam below, standing for the real ones a
// filesystem gives: a file another process is holding open, one marked
// read-only. What the caller does with it is the same either way.
var errStuck = errors.New("in use by another process")

// yes and no are what a look comes back with about one path.
var (
	yes = true
	no  = false
)

// lookOf answers from a table, and refuses anything not in it — a path nobody
// said anything about is a bug in the caller, not a path that is unprotected.
func lookOf(t *testing.T, answers map[string]*bool) Look {
	t.Helper()
	return func(path string) (*bool, error) {
		answer, named := answers[path]
		if !named {
			t.Fatalf("looked at %q, which this test said nothing about", path)
		}
		return answer, nil
	}
}

// TestASurveyNamesTheKeysThatAreNotProtected is the whole of what the report
// needs: which keys a reader has to do something about.
func TestASurveyNamesTheKeysThatAreNotProtected(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	safe := filepath.Join(dir, "id_safe")
	bare := filepath.Join(dir, "id_bare")

	s := Take("EFS", dir, []string{safe, bare}, lookOf(t, map[string]*bool{
		dir: &yes, safe: &yes, bare: &no,
	}))

	assert.Equal(t, "EFS", s.Scheme)
	assert.Equal(t, []string{bare}, s.Unprotected(), "only the key that is not protected")
	assert.False(t, s.Complete(), "a directory with one bare key in it is not done")
}

// TestAMarkedDirectoryOverBareKeysIsNotProtected is the state this exists to
// catch. Marking a directory covers the files made in it afterwards and does
// nothing for the ones already there, so "the directory is encrypted" over keys
// that are not is exactly the report a reader would act on wrongly.
func TestAMarkedDirectoryOverBareKeysIsNotProtected(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	bare := filepath.Join(dir, "id_bare")

	s := Take("EFS", dir, []string{bare}, lookOf(t, map[string]*bool{dir: &yes, bare: &no}))

	require.NotNil(t, s.DirProtected)
	assert.True(t, *s.DirProtected, "the directory really is marked")
	assert.False(t, s.Complete(), "and that is not the same as the keys being protected")
}

// TestADirectoryThatIsBareUnderProtectedKeysIsNotDoneEither: the other half of
// the same question. Every key is covered today, and the next one generated
// there will not be, with nothing to say so at the time.
func TestADirectoryThatIsBareUnderProtectedKeysIsNotDoneEither(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	safe := filepath.Join(dir, "id_safe")

	s := Take("EFS", dir, []string{safe}, lookOf(t, map[string]*bool{dir: &no, safe: &yes}))

	assert.Empty(t, s.Unprotected(), "no key needs anything done to it")
	assert.False(t, s.Complete(), "but the directory still does, for the key that does not exist yet")
}

// TestASurveyOfAPlatformWithNoSchemeClaimsNothing. Where this build has no way
// to protect a key, the report must not say the keys are unprotected: nobody
// looked, and a reader sent to turn on something that is not there is worse off
// than one told nothing.
func TestASurveyOfAPlatformWithNoSchemeClaimsNothing(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	key := filepath.Join(dir, "id_key")

	s := Take("", dir, []string{key}, func(string) (*bool, error) {
		return nil, ErrNoSchemeHere
	})

	assert.Empty(t, s.Scheme)
	assert.Nil(t, s.DirProtected, "nothing was established about the directory")
	assert.Empty(t, s.Unprotected(), "and nothing may be demanded of the keys")
	assert.True(t, s.Nothing(), "the whole survey is an absence, and says so")
}

// TestApplyProtectsTheKeysAndTheDirectory covers the ordinary run.
func TestApplyProtectsTheKeysAndTheDirectory(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	key := filepath.Join(dir, "id_key")

	done := map[string]bool{}
	answers := map[string]*bool{dir: &no, key: &no}
	res := Apply(dir, []string{key}, func(path string) error {
		done[path] = true
		answers[path] = &yes // What the system would say afterwards.
		return nil
	}, lookOf(t, answers))

	assert.True(t, done[dir], "the directory is marked, so the next key is born protected")
	assert.True(t, done[key], "and the key that is already there is protected now")
	assert.ElementsMatch(t, []string{dir, key}, res.Protected)
	assert.Empty(t, res.Failed)
}

// TestApplyBelievesTheFilesystemRatherThanTheCall is the sharp edge. The call
// that turns this on can report success and change nothing — a directory
// holding a read-only file is documented as doing exactly that — so what the
// result says happened is read back off the path, never taken from the return
// value.
func TestApplyBelievesTheFilesystemRatherThanTheCall(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")

	res := Apply(dir, nil, func(string) error {
		return nil // Reports success.
	}, lookOf(t, map[string]*bool{dir: &no})) // And yet nothing changed.

	assert.Empty(t, res.Protected, "nothing may be reported protected that is not")
	assert.Equal(t, []string{dir}, res.Failed,
		"a call that succeeded and changed nothing is a failure to protect")
}

// TestApplyNeverTouchesAuthorizedKeys. That file holds public keys, so there is
// nothing in it to protect; and it is read at login by a service running as the
// system rather than as the account, which has no way to read it encrypted.
// Nothing is gained and a working login is what would be risked.
func TestApplyNeverTouchesAuthorizedKeys(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	authorized := filepath.Join(dir, "authorized_keys")
	key := filepath.Join(dir, "id_key")

	answers := map[string]*bool{dir: &yes, key: &yes}
	res := Apply(dir, []string{key, authorized}, func(path string) error {
		if path == authorized {
			t.Fatalf("authorized_keys must never be handed to the protecting call")
		}
		return nil
	}, lookOf(t, answers))

	assert.NotContains(t, res.Protected, authorized)
	assert.NotContains(t, res.Failed, authorized, "nor is leaving it alone a failure")
	assert.Contains(t, res.Skipped, authorized, "it is named as left alone, rather than passed over in silence")
}

// TestApplyReportsWhatWouldNotBudge: a key held open by something else, a
// read-only file. The rest still gets done, and what did not is named.
func TestApplyReportsWhatWouldNotBudge(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	stuck := filepath.Join(dir, "id_stuck")
	fine := filepath.Join(dir, "id_fine")

	answers := map[string]*bool{dir: &yes, stuck: &no, fine: &yes}
	res := Apply(dir, []string{stuck, fine}, func(path string) error {
		if path == stuck {
			return errStuck
		}
		return nil
	}, lookOf(t, answers))

	assert.Contains(t, res.Protected, fine, "the rest is still done")
	assert.Contains(t, res.Failed, stuck)
	assert.NotEmpty(t, res.Reasons[stuck], "and why, since the reader has to act on it")
}
