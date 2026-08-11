package config

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveSecretContainer covers the setting that names the compartment
// SSHakku makes for itself in the wallet (F33).
//
// Unset resolves to the empty string rather than to a default name, because
// there is no single default to resolve to: the Secret Service collection and
// the KeePassXC group are called different things and each keeps the name it
// already had. Which one applies is the backend's to know; this layer's job is
// to decide whether the user named one at all, and whether the name is one
// SSHakku may take.
func TestResolveSecretContainer(t *testing.T) {
	t.Run("a chosen name is used as given", func(t *testing.T) {
		s, errs := Resolve(File{SecretContainer: ptr("my-own-compartment")}, lookupFrom(nil))
		require.Empty(t, errs, "unexpected errors")
		assert.Equal(t, "my-own-compartment", s.SecretContainer, "SecretContainer must be the file's own value")
	})

	t.Run("absent or empty leaves each backend its own default", func(t *testing.T) {
		for _, file := range []File{{}, {SecretContainer: ptr("")}} {
			s, errs := Resolve(file, lookupFrom(nil))
			require.Emptyf(t, errs, "unexpected errors for %+v", file)
			assert.Emptyf(t, s.SecretContainer, "SecretContainer for %+v must be left unset", file)
		}
	})

	t.Run("whitespace or a slash is refused, and said so", func(t *testing.T) {
		// A KeePassXC group is addressed by path, so a name holding a slash
		// would nest the compartment somewhere nobody asked for it to be.
		for _, bad := range []string{"my compartment", "wallets/sshakku", "tab\there", "trailing "} {
			assertRefused(t, bad)
		}
	})

	// The names a desktop uses for its own wallet are refused because SSHakku
	// would not merely write into such a collection — it would adopt it, and
	// then treat everything already in it as its own to enumerate and delete.
	// Casing is not a defence: a wallet answering to "login" answers to "Login".
	t.Run("a name the desktop keeps for itself is refused, in any casing", func(t *testing.T) {
		for _, bad := range []string{"default", "Default", "login", "LOGIN", "session", "Session", "kdewallet", "KDEWallet"} {
			assertRefused(t, bad)
		}
	})
}

// assertRefused checks that a name is rejected, that SSHakku falls back to the
// backend's own compartment rather than to the rejected one, and that the
// report names the value: a login shell writes this to the session log and
// nowhere else, so an error that does not quote what was rejected leaves
// nothing to search the config file for.
func assertRefused(t *testing.T, bad string) {
	t.Helper()
	s, errs := Resolve(File{SecretContainer: ptr(bad)}, lookupFrom(nil))
	assert.Emptyf(t, s.SecretContainer, "SecretContainer for %q must be left unset", bad)
	var named bool
	for _, err := range errs {
		if strings.Contains(err.Error(), strconv.Quote(bad)) {
			named = true
		}
	}
	assert.Truef(t, named, "errors for %q = %v, want one quoting the rejected value", bad, errs)
}
