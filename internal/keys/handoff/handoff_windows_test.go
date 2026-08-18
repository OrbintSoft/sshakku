//go:build windows

package handoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F7: the passphrase crosses to the helper without passing through argv, the
// environment, or a file anyone could read. This is the whole of it on this
// system, through the entry points the product really calls — the rendezvous
// is made where the account's own profile is what keeps it private.
func TestAPassphraseCrossesToTheHelperAndIsGoneAfterwards(t *testing.T) {
	token, err := Stash("s3cr3t", 5*time.Second)
	require.NoError(t, err, "putting a passphrase aside for the helper to collect must succeed")

	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(token, cache),
		"the rendezvous is inside this account's own directory, which is what makes it the account's")

	got, err := Fetch(t.Context(), token)
	require.NoError(t, err, "collecting it must succeed")
	assert.Equal(t, "s3cr3t", got, "and hand back exactly what was put aside")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(token); os.IsNotExist(err) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, err = os.Stat(token)
	assert.Truef(t, os.IsNotExist(err),
		"a passphrase that has been collected leaves nothing behind to collect again: %s", token)

	_, err = Fetch(t.Context(), token)
	assert.Error(t, err, "and a second collector gets nothing rather than the passphrase again")
}

// A stash nobody comes for does not sit there waiting: it is served once or it
// expires, and either way the rendezvous is gone.
func TestAStashNobodyCollectsExpiresOnItsOwn(t *testing.T) {
	token, err := Stash("s3cr3t", 200*time.Millisecond)
	require.NoError(t, err)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(token); os.IsNotExist(err) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, err = os.Stat(token)
	assert.Truef(t, os.IsNotExist(err), "an uncollected passphrase is not left lying about: %s", token)
}

// The address limit is this system's, and a path over it is refused with the
// length said out loud — the alternative is the kernel's "invalid argument",
// which sends whoever reads it looking for a permission or a missing directory.
func TestAnAddressTooLongIsRefusedInWordsThatSayWhy(t *testing.T) {
	deep := filepath.Join(t.TempDir(), strings.Repeat("d", 120))
	require.NoError(t, os.MkdirAll(deep, 0o700))

	_, err := socketHandoffStash("s3cr3t", time.Second, func() (string, error) { return deep, nil }, maxSocketAddr)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "socket address", "the reason is the address, and the message says so")
}
