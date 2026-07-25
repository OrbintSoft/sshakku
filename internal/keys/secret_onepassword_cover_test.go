package keys

import (
	"errors"
	"testing"
)

func TestOnePasswordStoreDeleteErrorPropagates(t *testing.T) {
	boom := errors.New("op exec boom")
	r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
		"item get": fails(boom),
	}))
	b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
	if err := b.Store("svc", "label", "pass"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v (Store must surface Delete's failure)", err, boom)
	}
}

func TestOnePasswordStoreMarshalError(t *testing.T) {
	saveJSONMarshal(t)
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
		"item get": stdout("", 1), // nothing to delete
	}))
	b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
	if err := b.Store("svc", "label", "pass"); err == nil {
		t.Fatal("expected an error when the item template cannot be marshaled")
	}
}

func TestOnePasswordStoreCreateRunError(t *testing.T) {
	boom := errors.New("op exec boom")
	r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
		"item get":    stdout("", 1), // nothing to delete
		"item create": fails(boom),
	}))
	b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
	if err := b.Store("svc", "label", "pass"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}

func TestOnePasswordDeleteItemDeleteRunError(t *testing.T) {
	boom := errors.New("op exec boom")
	r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
		"item get":    stdout(`{"id":"abc"}`, 0), // found
		"item delete": fails(boom),
	}))
	b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
	if err := b.Delete("svc"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}

func TestOnePasswordListErrorBranches(t *testing.T) {
	t.Run("item list fails to run", func(t *testing.T) {
		boom := errors.New("op exec boom")
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item list": fails(boom),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		if _, err := b.List(); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("item list returns unparseable JSON", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item list": stdout("not json", 0),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		if _, err := b.List(); err == nil {
			t.Fatal("expected an error when item list returns unparseable JSON")
		}
	})
}
