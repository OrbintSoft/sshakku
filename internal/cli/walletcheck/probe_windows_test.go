//go:build windows

package walletcheck

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

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
