//go:build linux

package secretservice

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
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
		if err := client.Unlock(col); err == nil {
			t.Fatal("Unlock returned no error against a prompt that never completes")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("Unlock waited %v against a prompt budget of 200ms; the configured wait is not the one in force", elapsed)
		}
	})

	t.Run("with none configured, the wait is a person's", func(t *testing.T) {
		// Not a round number picked here: a wait on someone typing a password
		// is counted in minutes, and anything a caller who chose nothing gets
		// must be long enough for that. Thirty seconds is not.
		if wait := (&Client{}).promptWait(); wait < 2*time.Minute {
			t.Errorf("an unconfigured prompt wait is %v, want at least the couple of minutes a person needs to answer a dialog", wait)
		}
	})
}
