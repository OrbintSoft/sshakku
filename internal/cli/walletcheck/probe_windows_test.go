//go:build windows

package walletcheck

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/config"
)

// unconfiguredSettings resolves the settings a machine with no config file of
// its own gets — the case these tests describe. It goes through the
// configuration layer rather than naming a backend here, because naming one
// would answer a question the user never asked.
func unconfiguredSettings(t *testing.T) config.Settings {
	t.Helper()
	sources := config.LoadSources(t.TempDir())
	settings, _ := config.Resolve(config.Merged(sources), os.LookupEnv)
	return settings
}

// TestTheReportNamesTheStoreThisSystemKeeps verifies F25 here: the report names
// the wallet the passphrases actually go into, which on this system is the one
// the system provides itself.
func TestTheReportNamesTheStoreThisSystemKeeps(t *testing.T) {
	view := walletView(t.Context(), unconfiguredSettings(t), probeWith(runtime.GOOS, nil, nil, "", nil))

	assert.Equal(t, config.SecretBackendCredentialManager, view.Backend,
		"the report must name the wallet the passphrases actually go into")
	for _, req := range view.Requirements {
		assert.NotEqualf(t, "session bus", req.Name,
			"%s has no D-Bus session bus, so asking for one sends the user after a piece that cannot exist (%q)",
			runtime.GOOS, req.Detail)
	}
}

// TestTheReportSaysWhatGuardsAWalletThatNeverAsks is F54's second half. A user
// reading "your passphrases are in the system's wallet" has every reason to
// assume the guarantees of the wallets on the other two platforms, and this one
// has neither: no lock, and no per-program permission. Being asked for nothing
// is exactly what makes that invisible, so the report is where it gets said.
func TestTheReportSaysWhatGuardsAWalletThatNeverAsks(t *testing.T) {
	view := walletView(t.Context(), unconfiguredSettings(t), probeWith(runtime.GOOS, nil, nil, "", nil))

	assert.Contains(t, view.Guard, "any program running as you",
		"the report must say who can read what is stored here, not merely that something guards it")
}

// TestAWalletThisSystemHasNotGotIsNotDescribedAsGuarded: the sentence belongs
// to the store this system keeps, not to every wallet a configuration might
// name here.
func TestAWalletThisSystemHasNotGotIsNotDescribedAsGuarded(t *testing.T) {
	settings := config.Settings{
		SecretBackend:  config.SecretBackendKeePassXC,
		KeePassXCRoute: config.KeePassXCRouteCLI,
	}

	view := walletView(t.Context(), settings, probeWith(runtime.GOOS, nil, nil, "", nil))

	assert.Empty(t, view.Guard, "another wallet's guarantees are not this one's to describe")
}

// F23 and F48: a route the user pinned is answered under the name they wrote,
// and what this system has not got is named as absent rather than reported as
// something to go and install.
//
// The Secret Service is the one way in that cannot exist here — there is no
// session bus for it to be on — so the report has two things to get right at
// once. It must not quietly answer about a different route, which would leave
// the user reading about a way in they did not choose and cannot tell apart
// from the one they did; and it must not send them after a piece to install,
// because there is no version of this system where that piece arrives. Naming
// the route that does reach the same database here is the part they can act on.
func TestARouteThisSystemHasNoWayInForIsNamedRatherThanSwapped(t *testing.T) {
	settings := config.Settings{
		SecretBackend:  config.SecretBackendKeePassXC,
		KeePassXCRoute: config.KeePassXCRouteSecretService,
	}

	view := walletView(t.Context(), settings, probeWith(runtime.GOOS, nil, nil, "", nil))

	assert.Equal(t, config.KeePassXCRouteSecretService, view.Route,
		"the route reported is the one that was written down, not one substituted for it")
	require.Len(t, view.Requirements, 1,
		"there is one thing to say about a way in that does not exist here")
	assert.Equal(t, "secret service", view.Requirements[0].Name)
	assert.Contains(t, view.Requirements[0].Detail, runtime.GOOS,
		"the reason is this operating system, and saying so is what stops the reader looking for a package")
	assert.Contains(t, view.Requirements[0].Detail, config.KeePassXCRouteNative,
		"and a route that does reach the same database here is what the reader can act on")
}
