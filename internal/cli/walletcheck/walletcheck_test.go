package walletcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/config"
)

// TestViewDescribesTheConfiguredWalletOnThisMachine covers what View itself
// decides — which is only that the look is taken of *this* machine. The
// judgements are walletView's, and are checked against probes a test writes;
// what is checked here is that the real probe reaches them, because a report
// assembled from a probe of nowhere would describe every machine identically
// and still look like a report.
//
// The wallet named is one reached through a command-line tool, so taking this
// look asks nothing of a session bus and raises nothing on a screen.
func TestViewDescribesTheConfiguredWalletOnThisMachine(t *testing.T) {
	view := View(t.Context(), config.Settings{SecretBackend: config.SecretBackendOnePassword})

	assert.Equal(t, config.SecretBackendOnePassword, view.Backend,
		"the wallet described must be the one configured")
	require.Len(t, view.Requirements, 1, "that wallet needs one thing present: its own command-line tool")
	assert.Equal(t, "op", view.Requirements[0].Name, "and it must be named, so a reader knows what to install")
	assert.NotEmpty(t, view.Requirements[0].Detail,
		"a requirement with nothing said about it tells the reader neither where it is nor why it is missing")
}
