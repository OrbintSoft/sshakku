package keys

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bwCall dispatches a fakeRunner "bw" handler by its first two arguments
// (e.g. "get item", "get password", "create item"), since BitwardenBackend
// issues several different bw subcommands and the shared fakeRunner keys
// handlers by binary name alone.
func bwCall(handlers map[string]func(Cmd) (Result, error)) func(Cmd) (Result, error) {
	return func(c Cmd) (Result, error) {
		verb := c.Args[0]
		switch {
		case verb == "login" && len(c.Args) > 1 && c.Args[1] == "--check":
			verb = "login --check"
		case verb == "login":
			verb = "login" // Args[1] is the account email, not a fixed verb token
		case len(c.Args) > 1:
			verb += " " + c.Args[1]
		}
		h, ok := handlers[verb]
		if !ok {
			return Result{}, errors.New("unexpected bw verb " + verb)
		}
		return h(c)
	}
}

// bwVerbs is the sequence of bw subcommands that were run, which is what
// several cases below are about: which of login/unlock/lock happened, and in
// what order.
func bwVerbs(r *fakeRunner) []string {
	var verbs []string
	for _, c := range r.calls {
		verbs = append(verbs, c.Args[0])
	}
	return verbs
}

func hasSessionEnv(c Cmd, session string) bool {
	want := "BW_SESSION=" + session
	for _, e := range c.Env {
		if e == want {
			return true
		}
	}
	return false
}

func TestBitwardenLookup(t *testing.T) {
	t.Run("hit reads the password", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, stdout("hunter2", 0))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		pass, found, err := b.Lookup("sshakku-id_rsa")
		require.NoError(t, err, "a stored passphrase must come back")
		assert.True(t, found, "the item is in the vault, so it must be reported found")
		assert.Equal(t, "hunter2", pass, "and the passphrase read out must be the one that was stored")
		require.NotEmpty(t, r.calls, "the vault must actually be asked")
		call := r.calls[0]
		assert.Equal(t, []string{"get", "password", "sshakku-id_rsa"}, call.Args,
			"the vault must be asked for the password of exactly the entry named")
		assert.True(t, hasSessionEnv(call, "sess-token"),
			"the unlocked session must be handed over, or bw asks the user to unlock again")
		for _, a := range call.Args {
			assert.NotContains(t, a, "sess-token",
				"argv is world-readable on this machine: a session key there is readable by every other user")
		}
	})

	t.Run("miss is found=false, no error", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, stdout("Not found.", 1))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		_, found, err := b.Lookup("sshakku-id_rsa")
		require.NoError(t, err, "a passphrase that was never stored is not an error")
		assert.False(t, found, "and nothing may be reported found")
	})

	t.Run("a failure to start bw is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		b := &BitwardenBackend{Runner: newFakeRunner().on(bitwardenBin, fails(wantErr)), Session: "sess-token", held: true}
		_, _, err := b.Lookup("x")
		assert.ErrorIs(t, err, wantErr, "a vault tool that would not run must be reported, not read as a miss")
	})
}

func TestBitwardenStore(t *testing.T) {
	const passphrase = "s3cr3t-pass"

	t.Run("no existing item: creates via base64 stdin", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item":    stdout("Not found.", 1),
			"create item": stdout(`{"id":"new-id"}`, 0),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		require.NoError(t, b.Store("sshakku-id_rsa", "SSH Passphrase for id_rsa", passphrase),
			"saving a passphrase must succeed")

		require.Lenf(t, r.calls, 2, "an entry that is not there is looked up, then created: %+v", r.calls)
		create := r.calls[1]
		assert.Equal(t, []string{"create", "item"}, create.Args, "and it must be a creation, not an edit of something else")
		for _, a := range create.Args {
			assert.NotContains(t, a, passphrase,
				"argv is world-readable on this machine: a passphrase there is readable by every other user")
		}
		assert.NotContains(t, create.Stdin, passphrase,
			"bw reads the item as base64; a passphrase appearing verbatim means it was never encoded")

		decoded, err := base64.StdEncoding.DecodeString(create.Stdin)
		require.NoError(t, err, "bw reads the item as base64, so what is written must decode as base64")
		var item bitwardenItem
		require.NoError(t, json.Unmarshal(decoded, &item), "and what it decodes to must be an item bw can read")
		assert.Equal(t, "sshakku-id_rsa", item.Name, "the entry must be named after the key it belongs to")
		assert.Equal(t, bitwardenLoginItemType, item.Type, "and be the kind of entry a password lives in")
		assert.Equal(t, passphrase, item.Login.Password, "the passphrase must be what is saved")
		assert.Equal(t, "SSH Passphrase for id_rsa", item.Login.Username,
			"and the label must be what a person sees in their vault")
	})

	t.Run("existing item is edited in place, not deleted", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item":  stdout(`{"id":"abc123"}`, 0),
			"edit item": stdout(`{"id":"abc123"}`, 0),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		require.NoError(t, b.Store("sshakku-id_rsa", "label", passphrase), "replacing a passphrase must succeed")
		require.Lenf(t, r.calls, 2, "an entry that is there is looked up, then edited: %+v", r.calls)
		assert.Equal(t, []string{"edit", "item", "abc123"}, r.calls[1].Args,
			"the entry already in the vault must be edited in place; deleting and recreating it loses its history")
	})

	t.Run("a non-zero exit from create is an error", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item": stdout("Not found.", 1),
			"create item": func(Cmd) (Result, error) {
				return Result{Stderr: []byte("vault is locked"), Code: 1}, nil
			},
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		assert.Error(t, b.Store("x", "y", passphrase),
			"a passphrase the vault refused to write must not be reported as saved")
	})
}

func TestBitwardenDelete(t *testing.T) {
	t.Run("existing item: looks up id then deletes permanently", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item":    stdout(`{"id":"abc123"}`, 0),
			"delete item": stdout("", 0),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		require.NoError(t, b.Delete("sshakku-id_rsa"), "forgetting a passphrase must succeed")
		require.Lenf(t, r.calls, 2, "an entry that is there is looked up, then deleted: %+v", r.calls)
		assert.Equal(t, []string{"delete", "item", "abc123", "--permanent"}, r.calls[1].Args,
			"a passphrase the user asked to forget must leave the vault, not sit in its trash")
	})

	t.Run("missing item is success, no delete call made", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item": stdout("Not found.", 1),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		require.NoError(t, b.Delete("sshakku-id_rsa"),
			"a passphrase that is already not there is the outcome that was asked for")
		assert.Lenf(t, r.calls, 1, "and nothing may be deleted when nothing matched: %+v", r.calls)
	})

	t.Run("a non-zero exit from delete is an error", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item": stdout(`{"id":"abc123"}`, 0),
			"delete item": func(Cmd) (Result, error) {
				return Result{Stderr: []byte("permission denied"), Code: 1}, nil
			},
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		assert.Error(t, b.Delete("x"), "a passphrase the vault refused to remove must not be reported as forgotten")
	})
}

// TestBitwardenListLeavesOtherItemsAlone verifies F27 against a vault holding
// more than sshakku's own items, which is any vault a person actually uses:
// `bw list items` answers with the whole vault, and whatever List reports is
// what `forget --all` goes on to delete.
func TestBitwardenListLeavesOtherItemsAlone(t *testing.T) {
	r := newFakeRunner().on(bitwardenBin, stdout(`[{"name":"github.com"},{"name":"`+defaultServicePrefix+`-id_ed25519"},{"name":"Bank"},{"name":"`+defaultServicePrefix+`-id_rsa"},{"name":"Passport scan"}]`, 0))
	b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
	got, err := b.List()
	require.NoError(t, err, "listing what SSHakku keeps in the vault must succeed")
	assert.Equal(t, []string{defaultServicePrefix + "-id_ed25519", defaultServicePrefix + "-id_rsa"}, got,
		"only SSHakku's own entries may be reported: everything else in the vault belongs to someone else, "+
			"and whatever is listed here is what forget --all goes on to delete")
}

func TestBitwardenList(t *testing.T) {
	t.Run("returns each item's name", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, stdout(`[{"name":"`+defaultServicePrefix+`-id_rsa"},{"name":"`+defaultServicePrefix+`-id_ed25519"}]`, 0))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		got, err := b.List()
		require.NoError(t, err, "listing what the vault holds must succeed")
		assert.Equal(t, []string{defaultServicePrefix + "-id_rsa", defaultServicePrefix + "-id_ed25519"}, got,
			"every entry must be named, by the key it belongs to")
		require.NotEmpty(t, r.calls, "the vault must actually be asked")
		assert.Equal(t, []string{"list", "items"}, r.calls[0].Args, "and asked for its items")
	})

	t.Run("empty account returns an empty, non-nil slice", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, stdout(`[]`, 0))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		got, err := b.List()
		require.NoError(t, err, "a vault holding nothing is not an error")
		assert.Empty(t, got, "and nothing may be listed")
	})

	t.Run("a non-zero exit is an error", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, func(Cmd) (Result, error) {
			return Result{Stderr: []byte("vault is locked"), Code: 1}, nil
		})
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		_, err := b.List()
		assert.Error(t, err, "a vault that could not be read must not be listed as empty")
	})
}

func TestBitwardenUnlock(t *testing.T) {
	t.Run("already logged in: skips login, unlocks with the prompted password", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check":        stdout("", 0), // already logged in
			"unlock --passwordenv": stdout("fresh-session-key", 0),
		}))
		p := &fakePrompter{pass: "correct horse battery staple"}
		b := &BitwardenBackend{Runner: r, Prompter: p}

		require.NoError(t, b.Unlock(), "unlocking a vault the user is logged into must succeed")
		assert.Equal(t, "fresh-session-key", b.Session, "the session bw handed back is what later calls must use")
		assert.True(t, b.held, "and the vault must be known to be open")
		assert.Len(t, p.calls, 1, "the master password is asked for once, not once per call")

		require.NotEmpty(t, r.calls, "bw must actually be run")
		unlockCall := r.calls[len(r.calls)-1]
		for _, a := range unlockCall.Args {
			assert.NotContains(t, a, "correct horse battery staple",
				"argv is world-readable on this machine: a master password there is readable by every other user")
		}
		assert.Contains(t, unlockCall.Env, EnvBitwardenPassword+"=correct horse battery staple",
			"bw must be given the password out of sight, through the environment")
	})

	t.Run("not logged in: logs in first, then unlocks", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check":        stdout("", 1), // not logged in
			"login":                stdout("", 0),
			"unlock --passwordenv": stdout("fresh-session-key", 0),
		}))
		p := &fakePrompter{pass: "hunter2"}
		b := &BitwardenBackend{Runner: r, Prompter: p, Email: "sshakku-test@example.invalid"}

		require.NoError(t, b.Unlock(), "unlocking a vault the user is not yet logged into must succeed")

		assert.Equal(t, []string{"login", "login", "unlock"}, bwVerbs(r),
			"a vault nobody is logged into must be logged into before it can be unlocked")
		require.Greaterf(t, len(r.calls), 1, "the login call must have been made: %+v", r.calls)
		require.Greater(t, len(r.calls[1].Args), 1, "the login must name an account")
		assert.Equal(t, "sshakku-test@example.invalid", r.calls[1].Args[1],
			"and it must be the account the user configured, not whichever one bw remembers")
	})

	t.Run("Server set, not yet logged in: configures the server before logging in", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check":        stdout("", 1), // not logged in
			"config server":        stdout("", 0),
			"login":                stdout("", 0),
			"unlock --passwordenv": stdout("fresh-session-key", 0),
		}))
		p := &fakePrompter{pass: "hunter2"}
		b := &BitwardenBackend{Runner: r, Prompter: p, Server: "https://vault.example.invalid"}

		require.NoError(t, b.Unlock(), "unlocking a self-hosted vault must succeed")
		require.Greaterf(t, len(r.calls), 1, "the server must be configured before logging in: %+v", r.calls)
		assert.Equal(t, []string{"config", "server", "https://vault.example.invalid"}, r.calls[1].Args,
			"a user who named their own server must be logged into that one, not into Bitwarden's")
	})

	t.Run("Server set, already logged in: never calls config server", func(t *testing.T) {
		// bw refuses to change server config while logged in ("Logout
		// required before server config update") — a real failure this
		// fixture would catch if Unlock called config server unconditionally.
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check":        stdout("", 0), // already logged in
			"unlock --passwordenv": stdout("fresh-session-key", 0),
		}))
		p := &fakePrompter{pass: "hunter2"}
		b := &BitwardenBackend{Runner: r, Prompter: p, Server: "https://vault.example.invalid"}

		require.NoError(t, b.Unlock(),
			"bw refuses to change the server while logged in, so a login that is already there must be left alone")
	})

	t.Run("a canceled prompt is returned as-is, no bw call made", func(t *testing.T) {
		r := newFakeRunner()
		p := &fakePrompter{err: ErrPromptCanceled}
		b := &BitwardenBackend{Runner: r, Prompter: p}
		assert.ErrorIs(t, b.Unlock(), ErrPromptCanceled, "a user who dismissed the prompt has answered, and must be obeyed")
		assert.Emptyf(t, r.calls, "and nothing may be attempted on a vault they declined to open: %+v", r.calls)
	})

	t.Run("a non-zero exit from unlock is an error", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check": stdout("", 0),
			"unlock --passwordenv": func(Cmd) (Result, error) {
				return Result{Stderr: []byte("invalid master password"), Code: 1}, nil
			},
		}))
		p := &fakePrompter{pass: "wrong"}
		b := &BitwardenBackend{Runner: r, Prompter: p}
		assert.Error(t, b.Unlock(), "a wrong master password must be reported, not passed over")
		assert.False(t, b.held, "and the vault must not be believed open when it is not")
	})
}

func TestBitwardenLock(t *testing.T) {
	r := newFakeRunner().on(bitwardenBin, stdout("", 0))
	b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
	require.NoError(t, b.Lock(), "closing the vault must succeed")
	assert.Empty(t, b.Session, "a session key kept past the lock would reopen the vault without asking anyone")
	assert.False(t, b.held, "and the vault must not be believed open once it is locked")
}

func TestBitwardenStandaloneBracket(t *testing.T) {
	t.Run("Lookup with held=false prompts, unlocks, and locks around the call", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check":        stdout("", 0),
			"unlock --passwordenv": stdout("fresh-session-key", 0),
			"get password":         stdout("hunter2", 0),
			"lock":                 stdout("", 0),
		}))
		p := &fakePrompter{pass: "hunter2"}
		b := &BitwardenBackend{Runner: r, Prompter: p}

		pass, found, err := b.Lookup("sshakku-id_rsa")
		require.NoError(t, err, "a lookup on a locked vault must open it and answer")
		assert.True(t, found, "the item is in the vault, so it must be reported found")
		assert.Equal(t, "hunter2", pass, "and the passphrase read out must be the one that was stored")
		assert.Len(t, p.calls, 1, "the master password is asked for once")
		assert.False(t, b.held, "a vault opened for one call must not be left open")
		assert.Empty(t, b.Session, "nor its session key kept, which would reopen it without asking anyone")

		assert.Equal(t, []string{"login", "unlock", "get", "lock"}, bwVerbs(r),
			"a call that opened the vault itself must close it again on its way out")
	})

	t.Run("a failed Unlock short-circuits Lookup with no get/lock call", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check": stdout("", 0),
			"unlock --passwordenv": func(Cmd) (Result, error) {
				return Result{Code: 1}, nil
			},
		}))
		p := &fakePrompter{pass: "wrong"}
		b := &BitwardenBackend{Runner: r, Prompter: p}
		_, _, err := b.Lookup("x")
		assert.Error(t, err, "a vault that would not open cannot answer, and must not be read as a miss")
	})
}
