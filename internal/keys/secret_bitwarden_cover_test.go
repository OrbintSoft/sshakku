package keys

import (
	"errors"
	"testing"
)

// saveJSONMarshal snapshots the jsonMarshal seam and restores it when the
// (sub)test ends, so a test can force the otherwise-unreachable marshal-failure
// branch of a Store without leaking into its siblings.
func saveJSONMarshal(t *testing.T) {
	t.Helper()
	orig := jsonMarshal
	t.Cleanup(func() { jsonMarshal = orig })
}

func TestBitwardenUnlockErrorBranches(t *testing.T) {
	boom := errors.New("bw exec boom")

	t.Run("login --check fails to run", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check": fails(boom),
		}))
		b := &BitwardenBackend{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		if err := b.Unlock(); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("config server fails to run", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check": stdout("", 1),
			"config server": fails(boom),
		}))
		b := &BitwardenBackend{Runner: r, Prompter: &fakePrompter{pass: "m"}, Server: "https://vault.invalid"}
		if err := b.Unlock(); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("config server exits non-zero", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check": stdout("", 1),
			"config server": func(Cmd) (Result, error) { return Result{Stderr: []byte("nope"), Code: 1}, nil },
		}))
		b := &BitwardenBackend{Runner: r, Prompter: &fakePrompter{pass: "m"}, Server: "https://vault.invalid"}
		if err := b.Unlock(); err == nil {
			t.Fatal("expected an error on a non-zero config-server exit")
		}
	})

	t.Run("login fails to run", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check": stdout("", 1),
			"login":         fails(boom),
		}))
		b := &BitwardenBackend{Runner: r, Prompter: &fakePrompter{pass: "m"}, Email: "u@invalid"}
		if err := b.Unlock(); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("login exits non-zero", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check": stdout("", 1),
			"login":         func(Cmd) (Result, error) { return Result{Stderr: []byte("bad creds"), Code: 1}, nil },
		}))
		b := &BitwardenBackend{Runner: r, Prompter: &fakePrompter{pass: "m"}, Email: "u@invalid"}
		if err := b.Unlock(); err == nil {
			t.Fatal("expected an error on a non-zero login exit")
		}
	})

	t.Run("unlock fails to run", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check":        stdout("", 0),
			"unlock --passwordenv": fails(boom),
		}))
		b := &BitwardenBackend{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		if err := b.Unlock(); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})
}

func TestBitwardenLockErrorBranches(t *testing.T) {
	t.Run("lock fails to run", func(t *testing.T) {
		boom := errors.New("bw exec boom")
		r := newFakeRunner().on(bitwardenBin, fails(boom))
		b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
		if err := b.Lock(); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
		if b.Session != "" || b.held {
			t.Fatal("Lock must forget the session even when the lock command fails")
		}
	})

	t.Run("lock exits non-zero", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, func(Cmd) (Result, error) {
			return Result{Stderr: []byte("cannot lock"), Code: 1}, nil
		})
		b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
		if err := b.Lock(); err == nil {
			t.Fatal("expected an error on a non-zero lock exit")
		}
	})
}

func TestBitwardenFindItemIDErrorBranches(t *testing.T) {
	t.Run("get item fails to run", func(t *testing.T) {
		boom := errors.New("bw exec boom")
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item": fails(boom),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
		if err := b.Store("svc", "label", "pass"); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("get item returns unparseable JSON", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item": stdout("this is not json", 0),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
		if err := b.Store("svc", "label", "pass"); err == nil {
			t.Fatal("expected an error when get item returns unparseable JSON")
		}
	})
}

func TestBitwardenStoreMarshalError(t *testing.T) {
	saveJSONMarshal(t)
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
		"get item": stdout("Not found.", 1),
	}))
	b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
	if err := b.Store("svc", "label", "pass"); err == nil {
		t.Fatal("expected an error when the item template cannot be marshaled")
	}
}

func TestBitwardenStoreCreateRunError(t *testing.T) {
	boom := errors.New("bw exec boom")
	r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
		"get item":    stdout("Not found.", 1),
		"create item": fails(boom),
	}))
	b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
	if err := b.Store("svc", "label", "pass"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}

func TestBitwardenDeleteRunErrors(t *testing.T) {
	boom := errors.New("bw exec boom")

	t.Run("get item fails to run", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item": fails(boom),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
		if err := b.Delete("svc"); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("delete item fails to run", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item":    stdout(`{"id":"abc"}`, 0),
			"delete item": fails(boom),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
		if err := b.Delete("svc"); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})
}

func TestBitwardenListErrorBranches(t *testing.T) {
	t.Run("list items fails to run", func(t *testing.T) {
		boom := errors.New("bw exec boom")
		r := newFakeRunner().on(bitwardenBin, fails(boom))
		b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
		if _, err := b.List(); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("list items returns unparseable JSON", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, stdout("not json", 0))
		b := &BitwardenBackend{Runner: r, Session: "sess", held: true}
		if _, err := b.List(); err == nil {
			t.Fatal("expected an error when list items returns unparseable JSON")
		}
	})
}

// TestBitwardenStandaloneBracketAllMethods covers the held=false unlock/lock
// bracket of Store, Delete, and List (Lookup's is covered elsewhere): each
// prompts, unlocks, runs, and locks when not already held open, and short-
// circuits when that unlock fails.
func TestBitwardenStandaloneBracketAllMethods(t *testing.T) {
	unlockOK := map[string]func(Cmd) (Result, error){
		"login --check":        stdout("", 0),
		"unlock --passwordenv": stdout("sess", 0),
		"lock":                 stdout("", 0),
	}
	with := func(base map[string]func(Cmd) (Result, error), extra map[string]func(Cmd) (Result, error)) map[string]func(Cmd) (Result, error) {
		m := map[string]func(Cmd) (Result, error){}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	t.Run("Store unlocks and locks", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(with(unlockOK, map[string]func(Cmd) (Result, error){
			"get item":    stdout("Not found.", 1),
			"create item": stdout(`{"id":"x"}`, 0),
		})))
		b := &BitwardenBackend{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		if err := b.Store("svc", "label", "pass"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.held || b.Session != "" {
			t.Fatal("expected the session locked and forgotten after a standalone Store")
		}
	})

	t.Run("Delete unlocks and locks", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(with(unlockOK, map[string]func(Cmd) (Result, error){
			"get item": stdout("Not found.", 1),
		})))
		b := &BitwardenBackend{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		if err := b.Delete("svc"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.held {
			t.Fatal("expected the session locked after a standalone Delete")
		}
	})

	t.Run("List unlocks and locks", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(with(unlockOK, map[string]func(Cmd) (Result, error){
			"list items": stdout("[]", 0),
		})))
		b := &BitwardenBackend{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		if _, err := b.List(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.held {
			t.Fatal("expected the session locked after a standalone List")
		}
	})

	t.Run("a failed Unlock short-circuits each method", func(t *testing.T) {
		for _, name := range []string{"store", "delete", "list"} {
			p := &fakePrompter{err: errors.New("prompt boom")}
			b := &BitwardenBackend{Runner: newFakeRunner(), Prompter: p}
			var err error
			switch name {
			case "store":
				err = b.Store("svc", "label", "pass")
			case "delete":
				err = b.Delete("svc")
			case "list":
				_, err = b.List()
			}
			if err == nil {
				t.Fatalf("%s: expected an error when Unlock fails", name)
			}
		}
	})
}
