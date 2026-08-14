package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
)

// TestForgetOnAWalletThatCannotDeleteTellsTheUserWhatToDo verifies F9's second
// half through the command a user actually runs: when the wallet gives SSHakku
// no way to delete, `sshakku forget` must fail loudly and name the entry, never
// report the passphrase as forgotten.
func TestForgetOnAWalletThatCannotDeleteTellsTheUserWhatToDo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)

	var stdout, stderr bytes.Buffer
	d := depsReturning(wallet.KeePassXC{})

	code := d.forget(t.Context(), &stdout, &stderr, []string{"id_ed25519"})

	assert.NotZero(t, code, "nothing was removed, so this did not succeed")
	assert.NotContains(t, stdout.String(), "forgot", "and nothing may claim the passphrase is gone")
	assert.Contains(t, stderr.String(), "KeePassXC", "the user must be told where the entry still is")
	assert.Contains(t, stderr.String(), wallet.DefaultServicePrefix+"-id_ed25519",
		"and which entry to remove by hand")
}
