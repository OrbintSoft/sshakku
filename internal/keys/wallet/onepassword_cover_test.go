package wallet

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// errOpExecBoom is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errOpExecBoom = errors.New("op exec boom")

func TestOnePasswordStoreDeleteErrorPropagates(t *testing.T) {
	boom := errOpExecBoom
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get": runtest.Fails(boom),
	}))
	b := &OnePassword{Runner: r, Vault: "sshakku"}
	assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "pass"), boom,
		"a vault that could not be asked what is already under that name must stop the write: "+
			"the answer decides between replacing SSHakku's own entry and writing over somebody else's")
}

func TestOnePasswordStoreDeleteOfOwnItemFails(t *testing.T) {
	boom := errOpExecBoom
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get":    runtest.Stdout(`{"title":"svc","tags":["`+onePasswordTag+`"]}`, 0), // SSHakku's own.
		"item delete": runtest.Fails(boom),
	}))
	b := &OnePassword{Runner: r, Vault: "sshakku"}
	assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "pass"), boom,
		"op cannot edit a passphrase in place, so an entry that would not go must stop the write: "+
			"creating the new one anyway would leave two under the same name")
}

func TestOnePasswordItemGetUnreadableAnswer(t *testing.T) {
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get": runtest.Stdout("not json", 0),
	}))
	b := &OnePassword{Runner: r, Vault: "sshakku"}
	_, _, err := b.Lookup(t.Context(), "svc")
	assert.Error(t, err,
		"an answer that cannot be read says nothing about whose item it is, and guessing would be the one mistake that matters")
}

func TestOnePasswordLookupItemWithoutPasswordField(t *testing.T) {
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get": runtest.Stdout(`{"title":"svc","tags":["`+onePasswordTag+`"],"fields":[]}`, 0),
	}))
	b := &OnePassword{Runner: r, Vault: "sshakku"}
	_, found, err := b.Lookup(t.Context(), "svc")
	require.NoError(t, err, "an item holding no passphrase is not an error")
	assert.False(t, found, "but there is no passphrase to report found")
}

func TestOnePasswordStoreMarshalError(t *testing.T) {
	saveJSONMarshal(t)
	jsonMarshal = func(any) ([]byte, error) { return nil, errMarshalBoom }
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get": runtest.Stdout("", 1), // nothing to delete.
	}))
	b := &OnePassword{Runner: r, Vault: "sshakku"}
	assert.Error(t, b.Store(t.Context(), "svc", "label", "pass"),
		"an item that could not be built must be reported, not sent as whatever it came out as")
}

func TestOnePasswordStoreCreateRunError(t *testing.T) {
	boom := errOpExecBoom
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get":    runtest.Stdout("", 1), // nothing to delete.
		"item create": runtest.Fails(boom),
	}))
	b := &OnePassword{Runner: r, Vault: "sshakku"}
	assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "pass"), boom,
		"a write that could not start must be reported, not read as a passphrase saved")
}

func TestOnePasswordDeleteItemDeleteRunError(t *testing.T) {
	boom := errOpExecBoom
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get":    runtest.Stdout(`{"title":"svc","tags":["`+onePasswordTag+`"]}`, 0), // found, and SSHakku's own.
		"item delete": runtest.Fails(boom),
	}))
	b := &OnePassword{Runner: r, Vault: "sshakku"}
	assert.ErrorIs(t, b.Delete(t.Context(), "svc"), boom,
		"a removal that could not start must be reported, not read as a passphrase forgotten")
}

func TestOnePasswordListErrorBranches(t *testing.T) {
	t.Run("item list fails to run", func(t *testing.T) {
		boom := errOpExecBoom
		r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
			"item list": runtest.Fails(boom),
		}))
		b := &OnePassword{Runner: r, Vault: "sshakku"}
		_, err := b.List(t.Context())
		assert.ErrorIs(t, err, boom, "an op command that would not run must be reported, not read as an empty vault")
	})

	t.Run("item list returns unparseable JSON", func(t *testing.T) {
		r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
			"item list": runtest.Stdout("not json", 0),
		}))
		b := &OnePassword{Runner: r, Vault: "sshakku"}
		_, err := b.List(t.Context())
		assert.Error(t, err, "an answer that could not be read must be reported, not read as an empty vault")
	})
}
