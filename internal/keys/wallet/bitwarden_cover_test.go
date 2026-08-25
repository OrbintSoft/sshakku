package wallet

import (
	"errors"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
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
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"login --check": runtest.Fails(boom),
		}))
		b := &Bitwarden{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		assert.ErrorIs(t, b.Unlock(t.Context()), boom,
			"a bw command that would not run must be reported as it failed, not read as a vault that opened")
	})

	t.Run("config server fails to run", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"login --check": runtest.Stdout("", 1),
			"config server": runtest.Fails(boom),
		}))
		b := &Bitwarden{Runner: r, Prompter: &fakePrompter{pass: "m"}, Server: "https://vault.invalid"}
		assert.ErrorIs(t, b.Unlock(t.Context()), boom,
			"a bw command that would not run must be reported as it failed, not read as a vault that opened")
	})

	t.Run("config server exits non-zero", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"login --check": runtest.Stdout("", 1),
			"config server": func(run.Cmd) (run.Result, error) { return run.Result{Stderr: []byte("nope"), Code: 1}, nil },
		}))
		b := &Bitwarden{Runner: r, Prompter: &fakePrompter{pass: "m"}, Server: "https://vault.invalid"}
		assert.Error(t, b.Unlock(t.Context()),
			"a server the CLI would not accept must be reported: logging in would reach the wrong vault")
	})

	t.Run("login fails to run", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"login --check": runtest.Stdout("", 1),
			"login":         runtest.Fails(boom),
		}))
		b := &Bitwarden{Runner: r, Prompter: &fakePrompter{pass: "m"}, Email: "u@invalid"}
		assert.ErrorIs(t, b.Unlock(t.Context()), boom,
			"a bw command that would not run must be reported as it failed, not read as a vault that opened")
	})

	t.Run("login exits non-zero", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"login --check": runtest.Stdout("", 1),
			"login":         func(run.Cmd) (run.Result, error) { return run.Result{Stderr: []byte("bad creds"), Code: 1}, nil },
		}))
		b := &Bitwarden{Runner: r, Prompter: &fakePrompter{pass: "m"}, Email: "u@invalid"}
		assert.Error(t, b.Unlock(t.Context()), "an account that could not be logged into must be reported, not unlocked past")
	})

	t.Run("unlock fails to run", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"login --check":        runtest.Stdout("", 0),
			"unlock --passwordenv": runtest.Fails(boom),
		}))
		b := &Bitwarden{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		assert.ErrorIs(t, b.Unlock(t.Context()), boom,
			"a bw command that would not run must be reported as it failed, not read as a vault that opened")
	})
}

func TestBitwardenLockErrorBranches(t *testing.T) {
	t.Run("lock fails to run", func(t *testing.T) {
		boom := errors.New("bw exec boom")
		r := runtest.NewRunner().On(bitwardenBin, runtest.Fails(boom))
		b := &Bitwarden{Runner: r, Session: "sess", held: true}
		assert.ErrorIs(t, b.Lock(t.Context()), boom, "a lock command that would not run must be reported")
		assert.Empty(t, b.Session,
			"and the session must be forgotten even so: keeping a key to a vault that may still be open is the worse half")
		assert.False(t, b.held, "nor may the vault be believed open afterwards")
	})

	t.Run("lock exits non-zero", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, func(run.Cmd) (run.Result, error) {
			return run.Result{Stderr: []byte("cannot lock"), Code: 1}, nil
		})
		b := &Bitwarden{Runner: r, Session: "sess", held: true}
		assert.Error(t, b.Lock(t.Context()), "a vault that refused to lock must be reported, not left believed closed")
	})
}

func TestBitwardenFindItemIDErrorBranches(t *testing.T) {
	t.Run("get item fails to run", func(t *testing.T) {
		boom := errors.New("bw exec boom")
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"get item": runtest.Fails(boom),
		}))
		b := &Bitwarden{Runner: r, Session: "sess", held: true}
		assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "pass"), boom,
			"a bw command that would not run must be reported, not read as a passphrase saved")
	})

	t.Run("get item returns unparseable JSON", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"get item": runtest.Stdout("this is not json", 0),
		}))
		b := &Bitwarden{Runner: r, Session: "sess", held: true}
		assert.Error(t, b.Store(t.Context(), "svc", "label", "pass"),
			"an answer that could not be read must be reported: writing on top of it could overwrite the wrong entry")
	})
}

func TestBitwardenStoreMarshalError(t *testing.T) {
	saveJSONMarshal(t)
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
		"get item": runtest.Stdout("Not found.", 1),
	}))
	b := &Bitwarden{Runner: r, Session: "sess", held: true}
	assert.Error(t, b.Store(t.Context(), "svc", "label", "pass"),
		"an item that could not be built must be reported, not sent as whatever it came out as")
}

func TestBitwardenStoreCreateRunError(t *testing.T) {
	boom := errors.New("bw exec boom")
	r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
		"get item":    runtest.Stdout("Not found.", 1),
		"create item": runtest.Fails(boom),
	}))
	b := &Bitwarden{Runner: r, Session: "sess", held: true}
	assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "pass"), boom,
		"a write that could not start must be reported, not read as a passphrase saved")
}

func TestBitwardenDeleteRunErrors(t *testing.T) {
	boom := errors.New("bw exec boom")

	t.Run("get item fails to run", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"get item": runtest.Fails(boom),
		}))
		b := &Bitwarden{Runner: r, Session: "sess", held: true}
		assert.ErrorIs(t, b.Delete(t.Context(), "svc"), boom,
			"a bw command that would not run must be reported, not read as a passphrase forgotten")
	})

	t.Run("delete item fails to run", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(map[string]func(run.Cmd) (run.Result, error){
			"get item":    runtest.Stdout(`{"id":"abc"}`, 0),
			"delete item": runtest.Fails(boom),
		}))
		b := &Bitwarden{Runner: r, Session: "sess", held: true}
		assert.ErrorIs(t, b.Delete(t.Context(), "svc"), boom,
			"a bw command that would not run must be reported, not read as a passphrase forgotten")
	})
}

func TestBitwardenListErrorBranches(t *testing.T) {
	t.Run("list items fails to run", func(t *testing.T) {
		boom := errors.New("bw exec boom")
		r := runtest.NewRunner().On(bitwardenBin, runtest.Fails(boom))
		b := &Bitwarden{Runner: r, Session: "sess", held: true}
		_, err := b.List(t.Context())
		assert.ErrorIs(t, err, boom, "a bw command that would not run must be reported, not read as an empty vault")
	})

	t.Run("list items returns unparseable JSON", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, runtest.Stdout("not json", 0))
		b := &Bitwarden{Runner: r, Session: "sess", held: true}
		_, err := b.List(t.Context())
		assert.Error(t, err, "an answer that could not be read must be reported, not read as an empty vault")
	})
}

// TestBitwardenStandaloneBracketAllMethods covers the held=false unlock/lock
// bracket of Store, Delete, and List (Lookup's is covered elsewhere): each
// prompts, unlocks, runs, and locks when not already held open, and short-
// circuits when that unlock fails.
func TestBitwardenStandaloneBracketAllMethods(t *testing.T) {
	unlockOK := map[string]func(run.Cmd) (run.Result, error){
		"login --check":        runtest.Stdout("", 0),
		"unlock --passwordenv": runtest.Stdout("sess", 0),
		"lock":                 runtest.Stdout("", 0),
	}
	with := func(base map[string]func(run.Cmd) (run.Result, error), extra map[string]func(run.Cmd) (run.Result, error)) map[string]func(run.Cmd) (run.Result, error) {
		m := map[string]func(run.Cmd) (run.Result, error){}
		maps.Copy(m, base)
		maps.Copy(m, extra)
		return m
	}

	t.Run("Store unlocks and locks", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(with(unlockOK, map[string]func(run.Cmd) (run.Result, error){
			"get item":    runtest.Stdout("Not found.", 1),
			"create item": runtest.Stdout(`{"id":"x"}`, 0),
		})))
		b := &Bitwarden{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		require.NoError(t, b.Store(t.Context(), "svc", "label", "pass"), "a store on a locked vault must open it and save")
		assert.False(t, b.held, "a vault opened for one call must not be left open")
		assert.Empty(t, b.Session, "nor its session key kept, which would reopen it without asking anyone")
	})

	t.Run("Delete unlocks and locks", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(with(unlockOK, map[string]func(run.Cmd) (run.Result, error){
			"get item": runtest.Stdout("Not found.", 1),
		})))
		b := &Bitwarden{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		require.NoError(t, b.Delete(t.Context(), "svc"), "a delete on a locked vault must open it and act")
		assert.False(t, b.held, "and a vault opened for one call must not be left open")
	})

	t.Run("List unlocks and locks", func(t *testing.T) {
		r := runtest.NewRunner().On(bitwardenBin, bwCall(with(unlockOK, map[string]func(run.Cmd) (run.Result, error){
			"list items": runtest.Stdout("[]", 0),
		})))
		b := &Bitwarden{Runner: r, Prompter: &fakePrompter{pass: "m"}}
		_, err := b.List(t.Context())
		require.NoError(t, err, "a listing on a locked vault must open it and answer")
		assert.False(t, b.held, "and a vault opened for one call must not be left open")
	})

	t.Run("a failed Unlock short-circuits each method", func(t *testing.T) {
		for _, name := range []string{"store", "delete", "list"} {
			p := &fakePrompter{err: errors.New("prompt boom")}
			b := &Bitwarden{Runner: runtest.NewRunner(), Prompter: p}
			var err error
			switch name {
			case "store":
				err = b.Store(t.Context(), "svc", "label", "pass")
			case "delete":
				err = b.Delete(t.Context(), "svc")
			case "list":
				_, err = b.List(t.Context())
			}
			assert.Errorf(t, err, "%s on a vault that would not open must be reported, not carried out blind", name)
		}
	})
}
