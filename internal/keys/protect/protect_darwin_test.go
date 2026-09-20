//go:build darwin

package protect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What this system answers about a key at rest, which is that it has no answer
// to give (F68, F69). The distinction being asserted is the whole of why these
// return what they do: "cannot tell" is not "unprotected", and a build that
// collapsed the two would tell every user here that their keys are readable by
// anyone on the machine, and send them looking for a per-file setting this
// system has not got: FileVault answers for the volume and is reported by the
// disk-encryption check, and the nearest per-directory equivalent is an
// encrypted image somebody mounts rather than an attribute on a file.

func TestThisSystemNamesNoSchemeToProtectAKeyWith(t *testing.T) {
	assert.Empty(t, Scheme(), "no scheme is named, so nothing is claimed about a key at rest")
}

func TestWhetherAKeyIsProtectedHereIsUnanswered(t *testing.T) {
	key := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(key, []byte("not really a key"), 0o600))

	answer, err := Protected(key)

	require.ErrorIs(t, err, ErrNoSchemeHere, "the question is refused rather than guessed at")
	assert.Nil(t, answer, "and nothing is handed back that a report could print as an answer")
}

func TestProtectingAKeyHereIsRefusedAndChangesNothing(t *testing.T) {
	const body = "not really a key"
	key := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(key, []byte(body), 0o600))
	before, err := os.Stat(key)
	require.NoError(t, err)

	refused := Protect(key)

	require.ErrorIs(t, refused, ErrNoSchemeHere, "it says so rather than appearing to have worked")
	after, err := os.Stat(key)
	require.NoError(t, err)
	kept, err := os.ReadFile(key)
	require.NoError(t, err)
	assert.Equal(t, body, string(kept), "the key is exactly as it was")
	assert.Equal(t, before.Mode(), after.Mode(), "and so is what may be done with it")
}
