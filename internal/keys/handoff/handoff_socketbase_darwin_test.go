//go:build darwin

package handoff

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errNoHome is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errNoHome = errors.New("no home")

// TestChooseSocketBase covers which directory a passphrase rendezvous is
// allowed to be made in: the short per-user one when it really is private,
// and the fallback whenever anything about that is not so.
//
// It lives with the system that asks the question. The other two do not: one
// hands nothing over a socket, and the other has a single answer.
func TestChooseSocketBase(t *testing.T) {
	cache := func() (string, error) { return "/the/cache", nil }

	t.Run("a private temporary directory is preferred", func(t *testing.T) {
		got, err := chooseSocketBase("/the/tmp", func(string) bool { return true }, cache)
		require.NoError(t, err, "choosing where the rendezvous goes must succeed")
		assert.Equal(t, "/the/tmp", got, "a short private directory keeps the socket address inside the kernel's limit")
	})

	t.Run("a shared temporary directory is refused", func(t *testing.T) {
		got, err := chooseSocketBase("/the/tmp", func(string) bool { return false }, cache)
		require.NoError(t, err, "choosing where the rendezvous goes must succeed")
		assert.Equal(t, "/the/cache", got,
			"a directory anyone else can reach is no place for a passphrase, however short its name")
	})

	t.Run("no temporary directory named at all", func(t *testing.T) {
		got, err := chooseSocketBase("", func(string) bool {
			assert.Fail(t, "a directory nobody named was inspected", "there is nothing there to ask about")
			return true
		}, cache)
		require.NoError(t, err, "choosing where the rendezvous goes must succeed")
		assert.Equal(t, "/the/cache", got, "with no temporary directory named, the user's own cache is where it goes")
	})

	t.Run("and no cache directory either", func(t *testing.T) {
		_, err := chooseSocketBase("", func(string) bool { return true }, func() (string, error) {
			return "", errNoHome
		})
		assert.Error(t, err, "with nowhere private to put it, the handoff must not happen at all")
	})
}
