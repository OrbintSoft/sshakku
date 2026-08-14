package wallet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// TestKeePassXCCLIUsesTheConfiguredContainer verifies the KeePassXC half of the
// promise that the compartment SSHakku keeps its entries in is the one the
// configuration names (F33): the group it creates, the group each entry is
// filed under, and the group it enumerates are all that one.
//
// It reads the argv that crossed the process boundary rather than the backend's
// intent, because a group name that is only half applied — created here, read
// back there — is exactly the shape of failure that looks like an empty wallet.
func TestKeePassXCCLIUsesTheConfiguredContainer(t *testing.T) {
	const group = "my-own-compartment"
	const service = "SSHakku-Key-id_rsa"

	t.Run("a new entry creates and fills the configured group", func(t *testing.T) {
		// The first call is Store's own lookup; a non-zero exit is the miss that
		// makes it create the group rather than edit an entry.
		runner := &runtest.Recorder{Results: []run.Result{{Code: 1}}}
		b := cliBackend(runner, &countingPrompter{password: "db"})
		b.Group = group

		require.NoError(t, b.Store(t.Context(), service, "label", "hunter2"), "saving into the configured group must succeed")
		require.Len(t, runner.Calls, 3, "a key with no entry yet is looked up, then its group made, then written")
		assert.Equal(t, group, lastArg(runner.Calls[1].Args), "the group that is made must be the one configured")
		assert.Equal(t, group+"/"+service, lastArg(runner.Calls[2].Args),
			"and the entry must be filed inside it, not in the group SSHakku would have picked")
	})

	t.Run("a lookup reads from the configured group", func(t *testing.T) {
		runner := &runtest.Recorder{Results: []run.Result{{Code: 0, Stdout: []byte("hunter2\n")}}}
		b := cliBackend(runner, &countingPrompter{password: "db"})
		b.Group = group

		_, _, err := b.Lookup(t.Context(), service)
		require.NoError(t, err, "reading from the configured group must succeed")
		require.NotEmpty(t, runner.Calls, "the database must actually be asked")
		assert.Equal(t, group+"/"+service, lastArg(runner.Calls[0].Args),
			"a lookup that read another group would report an empty wallet with the passphrases sitting in the configured one")
	})

	t.Run("a sweep enumerates the configured group and reports plain names", func(t *testing.T) {
		runner := &runtest.Recorder{Results: []run.Result{{Code: 0, Stdout: []byte(group + "/" + service + "\n")}}}
		b := cliBackend(runner, &countingPrompter{password: "db"})
		b.Group = group

		services, err := b.List(t.Context())
		require.NoError(t, err, "enumerating the configured group must succeed")
		require.NotEmpty(t, runner.Calls, "the database must actually be asked")
		assert.Equal(t, group, lastArg(runner.Calls[0].Args), "the group enumerated must be the one configured")
		// The group name is where the entry lives, not part of what it is
		// called: a sweep that left it on would hand `forget --all` names no
		// lookup could ever match.
		assert.Equal(t, []string{service}, services,
			"the names reported must be the ones a lookup goes by, without the group they happen to live in")
	})

	t.Run("an unset name keeps the group SSHakku has always used", func(t *testing.T) {
		runner := &runtest.Recorder{Results: []run.Result{{Code: 0, Stdout: []byte("hunter2\n")}}}
		b := cliBackend(runner, &countingPrompter{password: "db"})

		_, _, err := b.Lookup(t.Context(), service)
		require.NoError(t, err, "reading from SSHakku's own group must succeed")
		require.NotEmpty(t, runner.Calls, "the database must actually be asked")
		assert.Equal(t, keepassxcCLIGroup+"/"+service, lastArg(runner.Calls[0].Args),
			"a user who configured nothing must keep the group their entries are already in")
	})
}

// TestKeePassXCNativeSendsTheConfiguredGroup covers the group name the local
// protocol carries. KeePassXC files an entry into a group of its own choosing
// and ignores the name it is handed, so this changes nothing the user can
// observe today — which is why F33 promises them nothing here. It is still the
// configured name that goes on the wire rather than a fixed one, so that a
// KeePassXC which ever starts honouring it honours what was asked for.
func TestKeePassXCNativeSendsTheConfiguredGroup(t *testing.T) {
	t.Run("the configured name", func(t *testing.T) {
		kp := &fakeKeePassXC{}
		b := kp.backendFor(&memoryAssociations{})
		b.Group = "my-own-compartment"

		require.NoError(t, b.Store(t.Context(), "id_ed25519", "", "secret"), "saving a passphrase must succeed")
		assert.Equal(t, "my-own-compartment", kp.lastSet.group,
			"the name that goes on the wire must be the one configured, so a KeePassXC that starts honouring it honours that")
	})

	t.Run("SSHakku's own when none is configured", func(t *testing.T) {
		kp := &fakeKeePassXC{}
		b := kp.backendFor(&memoryAssociations{})

		require.NoError(t, b.Store(t.Context(), "id_ed25519", "", "secret"), "saving a passphrase must succeed")
		assert.Equal(t, keepassxcGroup, kp.lastSet.group, "and SSHakku's own name when the user configured none")
	})
}

// lastArg is the argument keepassxc-cli takes as the path to act on, which is
// always the final one.
func lastArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[len(args)-1]
}
