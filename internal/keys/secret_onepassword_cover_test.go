package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnePasswordStoreDeleteErrorPropagates(t *testing.T) {
	boom := errors.New("op exec boom")
	r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
		"item get": fails(boom),
	}))
	b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
	assert.ErrorIs(t, b.Store("svc", "label", "pass"), boom,
		"the old entry has to go before the new one lands, so a removal that failed must stop the write, not be passed over")
}

func TestOnePasswordStoreMarshalError(t *testing.T) {
	saveJSONMarshal(t)
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
		"item get": stdout("", 1), // nothing to delete
	}))
	b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
	assert.Error(t, b.Store("svc", "label", "pass"),
		"an item that could not be built must be reported, not sent as whatever it came out as")
}

func TestOnePasswordStoreCreateRunError(t *testing.T) {
	boom := errors.New("op exec boom")
	r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
		"item get":    stdout("", 1), // nothing to delete
		"item create": fails(boom),
	}))
	b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
	assert.ErrorIs(t, b.Store("svc", "label", "pass"), boom,
		"a write that could not start must be reported, not read as a passphrase saved")
}

func TestOnePasswordDeleteItemDeleteRunError(t *testing.T) {
	boom := errors.New("op exec boom")
	r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
		"item get":    stdout(`{"id":"abc"}`, 0), // found
		"item delete": fails(boom),
	}))
	b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
	assert.ErrorIs(t, b.Delete("svc"), boom,
		"a removal that could not start must be reported, not read as a passphrase forgotten")
}

func TestOnePasswordListErrorBranches(t *testing.T) {
	t.Run("item list fails to run", func(t *testing.T) {
		boom := errors.New("op exec boom")
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item list": fails(boom),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		_, err := b.List()
		assert.ErrorIs(t, err, boom, "an op command that would not run must be reported, not read as an empty vault")
	})

	t.Run("item list returns unparseable JSON", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item list": stdout("not json", 0),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		_, err := b.List()
		assert.Error(t, err, "an answer that could not be read must be reported, not read as an empty vault")
	})
}
