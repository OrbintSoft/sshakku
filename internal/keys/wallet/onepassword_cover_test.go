package wallet

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

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
		"the old entry has to go before the new one lands, so a removal that failed must stop the write, not be passed over")
}

func TestOnePasswordStoreMarshalError(t *testing.T) {
	saveJSONMarshal(t)
	jsonMarshal = func(any) ([]byte, error) { return nil, errMarshalBoom }
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get": runtest.Stdout("", 1), // nothing to delete
	}))
	b := &OnePassword{Runner: r, Vault: "sshakku"}
	assert.Error(t, b.Store(t.Context(), "svc", "label", "pass"),
		"an item that could not be built must be reported, not sent as whatever it came out as")
}

func TestOnePasswordStoreCreateRunError(t *testing.T) {
	boom := errOpExecBoom
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get":    runtest.Stdout("", 1), // nothing to delete
		"item create": runtest.Fails(boom),
	}))
	b := &OnePassword{Runner: r, Vault: "sshakku"}
	assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "pass"), boom,
		"a write that could not start must be reported, not read as a passphrase saved")
}

func TestOnePasswordDeleteItemDeleteRunError(t *testing.T) {
	boom := errOpExecBoom
	r := runtest.NewRunner().On(onePasswordBin, opCall(map[string]func(run.Cmd) (run.Result, error){
		"item get":    runtest.Stdout(`{"id":"abc"}`, 0), // found
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
