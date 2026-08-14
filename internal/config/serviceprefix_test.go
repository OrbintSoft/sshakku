package config

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
)

// TestResolveServicePrefix covers the setting that names SSHakku's entries in
// the store: a chosen name is kept, an absent one takes the default, and a name
// no store can be relied on to hold verbatim is refused loudly and replaced by
// the default rather than being written into entry names nobody can find again.
func TestResolveServicePrefix(t *testing.T) {
	t.Run("a chosen name is used as given", func(t *testing.T) {
		s, errs := Resolve(File{ServicePrefix: ptr("wallet-of-mine")}, lookupFrom(nil))
		require.Empty(t, errs, "unexpected errors")
		assert.Equal(t, "wallet-of-mine", s.ServicePrefix, "ServicePrefix must be the file's own value")
	})

	t.Run("absent or empty takes the default", func(t *testing.T) {
		for _, file := range []File{{}, {ServicePrefix: ptr("")}} {
			s, errs := Resolve(file, lookupFrom(nil))
			require.Emptyf(t, errs, "unexpected errors for %+v", file)
			assert.Equalf(t, wallet.DefaultServicePrefix, s.ServicePrefix, "ServicePrefix for %+v", file)
		}
	})

	t.Run("whitespace or a slash is refused, and said so", func(t *testing.T) {
		for _, bad := range []string{"my wallet", "sshakku/keys", "tab\there", "trailing "} {
			s, errs := Resolve(File{ServicePrefix: ptr(bad)}, lookupFrom(nil))
			assert.Equalf(t, wallet.DefaultServicePrefix, s.ServicePrefix, "ServicePrefix for %q must be the default", bad)
			// The value must be named in the report: a login shell writes this
			// to the session log and nowhere else, so an error that does not
			// quote what was rejected leaves nothing to search the config for.
			// Quoted, since that is how a name holding a tab stays readable —
			// and a rejected name is exactly the kind that holds one.
			var named bool
			for _, err := range errs {
				if strings.Contains(err.Error(), strconv.Quote(bad)) {
					named = true
				}
			}
			assert.Truef(t, named, "errors for %q = %v, want one quoting the rejected value", bad, errs)
		}
	})
}
