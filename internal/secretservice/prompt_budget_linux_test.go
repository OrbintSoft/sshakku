//go:build linux

package secretservice

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPromptIsGivenTheBudgetForAWaitOnAPerson verifies F21 on the wallet Linux
// desktops provide: a prompt raised by the Secret Service is the desktop asking
// the user to unlock their wallet, so what is being waited for is a person
// typing a password — after a reboot, in front of a dialog that has just
// appeared.
//
// F21 promises two things about such a wait, and this covers both: that how
// long to wait is configurable, and that the budget is the one meant for a wait
// on a person rather than the short one meant for something answering on its
// own. Giving up while someone is still typing loses the wallet for that login:
// what follows is a passphrase prompt for a key whose passphrase was saved.
func TestAPromptIsGivenTheBudgetForAWaitOnAPerson(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/x")

	t.Run("the budget the caller configured is the one waited", func(t *testing.T) {
		client, _ := newTestClient(t, "hang")
		client.PromptTimeout = 200 * time.Millisecond

		start := time.Now()
		err := client.Unlock(t.Context(), col)
		require.Error(t, err, "a prompt that never completes must not be reported as an unlock")
		// The budget is in the sentence because whether it was too short is a
		// question about the configuration, and the reader cannot ask it
		// without being told what the budget was.
		assert.ErrorContains(t, err, "timed out after 200ms",
			"the refusal must name the budget the wait ran out of")
		assert.Less(t, time.Since(start), 5*time.Second,
			"the configured 200ms prompt budget is not the one in force")
	})

	t.Run("with none configured, the wait is a person's", func(t *testing.T) {
		// Not a round number picked here: a wait on someone typing a password
		// is counted in minutes, and anything a caller who chose nothing gets
		// must be long enough for that. Thirty seconds is not.
		assert.GreaterOrEqual(t, (&Client{}).promptWait(), 2*time.Minute,
			"an unconfigured prompt wait must allow the couple of minutes a person needs to answer a dialog")
	})
}
