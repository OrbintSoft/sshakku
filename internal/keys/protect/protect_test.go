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

// TestASurveyNamesWhatIsDoneAndWhatIsLeft is the whole of what a run needs
// before it changes anything: which paths it has something to do about.
func TestASurveyNamesWhatIsDoneAndWhatIsLeft(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	safe := filepath.Join(dir, "id_safe")
	bare := filepath.Join(dir, "id_bare")

	s := Take("EFS", []string{safe, bare}, lookOf(t, map[string]*bool{safe: &yes, bare: &no}))

	assert.Equal(t, "EFS", s.Scheme)
	assert.Equal(t, []string{safe}, s.Already(), "the key there is nothing to do about")
	assert.Equal(t, []string{bare}, s.Pending(), "and the one there is")
}

// TestAPathNobodyCouldAnswerForIsStillAttempted. A look that failed is not an
// answer, and the two ways of reading it are not equally safe: attempting again
// changes nothing where the path is already covered, while leaving a key alone
// because a look failed is how a key stays in the clear with a run behind it
// that reported success.
func TestAPathNobodyCouldAnswerForIsStillAttempted(t *testing.T) {
	unknown := filepath.FromSlash("/home/alice/keys/id_unknown")

	s := Take("EFS", []string{unknown}, lookOf(t, map[string]*bool{unknown: nil}))

	assert.Empty(t, s.Already(), "nothing may be reported done that nobody established")
	assert.Equal(t, []string{unknown}, s.Pending())
}

// TestASurveyOfAPlatformWithNoSchemeClaimsNothing. Where this build has no way
// to protect a key, the survey must not report the keys as unprotected: nobody
// looked, and a reader sent to turn on something that is not there is worse off
// than one told nothing.
func TestASurveyOfAPlatformWithNoSchemeClaimsNothing(t *testing.T) {
	key := filepath.FromSlash("/home/alice/.ssh/id_key")

	s := Take("", []string{key}, func(string) (*bool, error) {
		return nil, ErrNoSchemeHere
	})

	assert.Empty(t, s.Scheme)
	assert.Empty(t, s.Already(), "nothing was established about the key")
	require.Len(t, s.Paths, 1)
	assert.Nil(t, s.Paths[0].Protected, "and the absence is carried as an absence")
}

// TestAPlanForTheKeysAloneCostsNothing is the ordinary run, and the reason it is
// the default: the files that exist are protected, and nothing that does not
// exist yet is affected either way.
func TestAPlanForTheKeysAloneCostsNothing(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	key := filepath.Join(dir, "id_key")

	p := PlanFor(dir, []string{key}, false, AuthorizedKeysIn(dir))

	assert.Equal(t, []string{key}, p.Targets, "the directory is not among them")
	assert.False(t, p.Cost.NeedsWarning(), "and there is nothing to warn about")
	assert.False(t, p.Cost.NeedsConfirmation())
}

// TestAPlanThatCoversTheDirectoryDoesItFirst. A key generated between this run
// and the next is born protected even where a key below will not budge.
func TestAPlanThatCoversTheDirectoryDoesItFirst(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/keys")
	key := filepath.Join(dir, "id_key")

	p := PlanFor(dir, []string{key}, true, "")

	assert.Equal(t, []string{dir, key}, p.Targets)
	assert.False(t, p.Cost.NeedsWarning(),
		"a directory no ssh server reads, with no authorized_keys in it, costs nothing to cover")
}

// TestCoveringTheDirectoryAnSSHServerReadsIsWarnedAbout. A file created there
// afterwards is born encrypted, and the server reads authorized_keys as the
// system, before there is a session of the account's to unlock anything with.
func TestCoveringTheDirectoryAnSSHServerReadsIsWarnedAbout(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")

	p := PlanFor(dir, nil, true, "")

	assert.True(t, p.Cost.ServerReads, "this is the directory a server reads unless told otherwise")
	assert.True(t, p.Cost.NeedsWarning(), "so what it costs is said before it happens")
	assert.False(t, p.Cost.NeedsConfirmation(),
		"and with no authorized_keys actually there, being told is enough")
}

// TestAnAuthorizedKeysActuallyThereIsNotAWarning is the sharp end of it. The
// file itself is in no danger — covering a directory does nothing to what is
// already in it — but its presence is what says a server really does read this
// directory, and the one it writes next is born encrypted.
func TestAnAuthorizedKeysActuallyThereIsNotAWarning(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/keys")
	authorized := AuthorizedKeysIn(dir)

	p := PlanFor(dir, nil, true, authorized)

	assert.False(t, p.Cost.ServerReads, "not by its name, wherever this directory is")
	assert.Equal(t, authorized, p.Cost.AuthorizedKeys, "but by what is sitting in it")
	assert.True(t, p.Cost.NeedsConfirmation(), "which is asked about, not warned about")
}

// TestAPlanNeverTargetsAuthorizedKeys. That file holds public keys, so there is
// nothing in it to protect; and it is read at login by a service running as the
// system rather than as the account, which has no way to read it encrypted.
func TestAPlanNeverTargetsAuthorizedKeys(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	authorized := AuthorizedKeysIn(dir)
	key := filepath.Join(dir, "id_key")

	p := PlanFor(dir, []string{key, authorized}, false, authorized)

	assert.Equal(t, []string{key}, p.Targets, "handed it as a key, it is still not a target")
}

// TestApplyProtectsEveryTargetItIsGiven covers the ordinary run.
func TestApplyProtectsEveryTargetItIsGiven(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	key := filepath.Join(dir, "id_key")

	done := map[string]bool{}
	answers := map[string]*bool{dir: &no, key: &no}
	res := Apply([]string{dir, key}, func(path string) error {
		done[path] = true
		answers[path] = &yes // What the system would say afterwards.
		return nil
	}, lookOf(t, answers))

	assert.True(t, done[dir], "the directory is covered, so the next key is born protected")
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

	res := Apply([]string{dir}, func(string) error {
		return nil // Reports success.
	}, lookOf(t, map[string]*bool{dir: &no})) // And yet nothing changed.

	assert.Empty(t, res.Protected, "nothing may be reported protected that is not")
	assert.Equal(t, []string{dir}, res.Failed,
		"a call that succeeded and changed nothing is a failure to protect")
}

// TestApplyNeverTouchesAuthorizedKeys, whatever it is handed. A plan does not
// put that file among the targets; this is the guard that holds however the
// targets were arrived at.
func TestApplyNeverTouchesAuthorizedKeys(t *testing.T) {
	dir := filepath.FromSlash("/home/alice/.ssh")
	authorized := AuthorizedKeysIn(dir)
	key := filepath.Join(dir, "id_key")

	answers := map[string]*bool{key: &yes}
	res := Apply([]string{key, authorized}, func(path string) error {
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

	answers := map[string]*bool{stuck: &no, fine: &yes}
	res := Apply([]string{stuck, fine}, func(path string) error {
		if path == stuck {
			return errStuck
		}
		return nil
	}, lookOf(t, answers))

	assert.Contains(t, res.Protected, fine, "the rest is still done")
	assert.Contains(t, res.Failed, stuck)
	assert.NotEmpty(t, res.Reasons[stuck], "and why, since the reader has to act on it")
}
