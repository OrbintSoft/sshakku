package diagnose

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies F69's reading half: whether the keys on disk are encrypted to this
// account alone is something the report says, per key and for the directory
// separately, and where they are not it names the command that changes it.
//
// What is stubbed here is the answer about a path, which belongs to the system
// and is driven against the real filesystem in the protect package. What is not
// stubbed is the decision under test — which of those answers reaches a reader,
// and as what.

const keyDir = "/home/u/.ssh"

// answers is a look that reads its answers from a table, and refuses to answer
// about anything it was not told about — which is how a path nobody asked about
// stays distinguishable from one that came back no.
func answers(table map[string]*bool) func(string) (*bool, error) {
	return func(path string) (*bool, error) {
		if answer, told := table[path]; told {
			return answer, nil
		}
		return nil, errBoom
	}
}

func protectedKeySource(scheme string, table map[string]*bool, keys ...string) *KeySource {
	return &KeySource{
		Dir:          keyDir,
		Lister:       fakeKeyLister{paths: keys},
		AtRestScheme: scheme,
		AtRest:       answers(table),
	}
}

func reportWithKeys(t *testing.T, ks *KeySource) Report {
	t.Helper()
	return Gather(t.Context(),
		Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000},
		fakeSource{}, fakeProber{}, nil, nil, ks, nil)
}

// TestTheKeysAndTheDirectoryAreReportedSeparately is the heart of the promise.
// A directory that says "encrypted" above keys that are not is the state worth
// catching, and a report that gave one answer for the lot could not show it.
func TestTheKeysAndTheDirectoryAreReportedSeparately(t *testing.T) {
	yes, no := true, false
	mine, theirs := filepath.Join(keyDir, "id_ed25519"), filepath.Join(keyDir, "id_rsa")

	r := reportWithKeys(t, protectedKeySource("EFS", map[string]*bool{
		keyDir: &yes,
		mine:   &no,
		theirs: &yes,
	}, mine, theirs))

	require.Len(t, r.Keys, 2)
	assert.Equal(t, &no, r.Keys[0].Protected, "the key that is not protected says so")
	assert.Equal(t, &yes, r.Keys[1].Protected, "and the one that is")
	assert.Equal(t, &yes, r.KeysDirProtected, "the directory's own answer is kept apart from theirs")
	assert.Equal(t, "EFS", r.KeyProtectionScheme, "and the report names what the answers are about")

	var b strings.Builder
	Format(&b, r)
	out := b.String()
	assert.Contains(t, out, "EFS", "the section names the scheme rather than leaving a reader to guess")
	assert.Contains(t, out, "not protected", "a key in the clear is visible in the report")
}

// TestNothingIsSaidAboutProtectionWhereThisSystemHasNoScheme. Reporting every
// key as unprotected on a system with nothing to protect it with sends a reader
// after a setting that does not exist there.
func TestNothingIsSaidAboutProtectionWhereThisSystemHasNoScheme(t *testing.T) {
	key := filepath.Join(keyDir, "id_ed25519")
	r := reportWithKeys(t, protectedKeySource("", nil, key))

	require.Len(t, r.Keys, 1)
	assert.Nil(t, r.Keys[0].Protected, "nobody looked, so nothing is claimed")

	var b strings.Builder
	Format(&b, r)
	out := b.String()
	assert.NotContains(t, out, "not protected", "a system with no scheme says nothing about protection")
	assert.NotContains(t, out, "protect-keys", "and sends nobody to a command that has nothing to do here")
	assert.Contains(t, out, "keys in "+keyDir, "while the keys themselves are listed as always")
}

// TestAnUnprotectedKeyNamesTheCommandThatProtectsIt. A report that states a
// problem and not what to do about it leaves the reader where it found them.
func TestAnUnprotectedKeyNamesTheCommandThatProtectsIt(t *testing.T) {
	yes, no := true, false
	key := filepath.Join(keyDir, "id_ed25519")

	r := reportWithKeys(t, protectedKeySource("EFS", map[string]*bool{
		keyDir: &yes,
		key:    &no,
	}, key))

	assert.Contains(t, strings.Join(r.Findings, "\n"), "sshakku protect-keys",
		"a key in the clear names the command that protects it")
	assert.Contains(t, strings.Join(r.Findings, "\n"), "id_ed25519",
		"and says which key, since a reader with several has to know which one")
}

// TestAKeyNobodyCouldAnswerForIsNotCalledUnprotected. The difference between a
// key found in the clear and a key nobody could ask about is the difference
// between a report worth acting on and one worth ignoring.
func TestAKeyNobodyCouldAnswerForIsNotCalledUnprotected(t *testing.T) {
	yes := true
	key := filepath.Join(keyDir, "id_ed25519")

	// The key is missing from the table, so the look refuses to answer about it.
	r := reportWithKeys(t, protectedKeySource("EFS", map[string]*bool{keyDir: &yes}, key))

	require.Len(t, r.Keys, 1)
	assert.Nil(t, r.Keys[0].Protected, "a look that could not answer leaves the answer open")
	assert.NotContains(t, strings.Join(r.Findings, "\n"), "protect-keys",
		"and nothing is reported as wrong on the strength of a question nobody answered")
}

// TestProtectedKeysUnderAnUnmarkedDirectoryAreTheirOwnFinding. Every key
// covered and the directory not means the next key generated there is born in
// the clear, with nothing said at the time — a state that reads as finished and
// is not.
func TestProtectedKeysUnderAnUnmarkedDirectoryAreTheirOwnFinding(t *testing.T) {
	yes, no := true, false
	key := filepath.Join(keyDir, "id_ed25519")

	r := reportWithKeys(t, protectedKeySource("EFS", map[string]*bool{
		keyDir: &no,
		key:    &yes,
	}, key))

	joined := strings.Join(r.Findings, "\n")
	assert.Contains(t, joined, "protect-keys", "the directory is worth a finding of its own")
	assert.NotContains(t, joined, "id_ed25519",
		"and it is not about the keys, which are all protected")
}

// TestEverythingProtectedIsNothingToReport. A report that keeps talking after
// the answer is yes teaches a reader to skim past it.
func TestEverythingProtectedIsNothingToReport(t *testing.T) {
	yes := true
	key := filepath.Join(keyDir, "id_ed25519")

	r := reportWithKeys(t, protectedKeySource("EFS", map[string]*bool{
		keyDir: &yes,
		key:    &yes,
	}, key))

	assert.NotContains(t, strings.Join(r.Findings, "\n"), "protect-keys",
		"nothing to do is nothing to say")
}
