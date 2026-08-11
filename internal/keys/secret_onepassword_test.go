package keys

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// opCall dispatches a fakeRunner "op" handler by its first two arguments
// (e.g. "item get", "item create", "read"), since OnePasswordBackend issues
// several different op subcommands and the shared fakeRunner keys handlers
// by binary name alone.
func opCall(handlers map[string]func(Cmd) (Result, error)) func(Cmd) (Result, error) {
	return func(c Cmd) (Result, error) {
		verb := c.Args[0]
		if len(c.Args) > 1 && (verb == "item" || verb == "vault") {
			verb += " " + c.Args[1]
		}
		h, ok := handlers[verb]
		if !ok {
			return Result{}, errors.New("unexpected op verb " + verb)
		}
		return h(c)
	}
}

func TestOnePasswordLookup(t *testing.T) {
	t.Run("hit reads the secret reference", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, stdout("hunter2", 0))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		pass, found, err := b.Lookup("sshakku-id_rsa")
		require.NoError(t, err, "a stored passphrase must come back")
		assert.True(t, found, "the item is in the vault, so it must be reported found")
		assert.Equal(t, "hunter2", pass, "and the passphrase read out must be the one that was stored")
		require.NotEmpty(t, r.calls, "the vault must actually be asked")
		assert.Equal(t, []string{"read", "op://sshakku/sshakku-id_rsa/password", "--no-newline"}, r.calls[0].Args,
			"the reference must name the vault, the entry and its password field, "+
				"and --no-newline keeps op from appending one to the passphrase")
	})

	t.Run("miss is found=false, no error", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, stdout("", 1))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		_, found, err := b.Lookup("sshakku-id_rsa")
		require.NoError(t, err, "a passphrase that was never stored is not an error")
		assert.False(t, found, "and nothing may be reported found")
	})

	t.Run("a failure to start op is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		b := &OnePasswordBackend{Runner: newFakeRunner().on(onePasswordBin, fails(wantErr)), Vault: "sshakku"}
		_, _, err := b.Lookup("x")
		assert.ErrorIs(t, err, wantErr, "a vault tool that would not run must be reported, not read as a miss")
	})
}

func TestOnePasswordStore(t *testing.T) {
	const passphrase = "s3cr3t-pass"

	t.Run("no existing item: deletes nothing, creates via stdin", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get":    stdout("", 1), // not found
			"item create": stdout("", 0),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		require.NoError(t, b.Store("sshakku-id_rsa", "SSH Passphrase for id_rsa", passphrase),
			"saving a passphrase must succeed")

		require.Lenf(t, r.calls, 2, "an entry that is not there is looked up, then created: %+v", r.calls)
		create := r.calls[1]
		for _, a := range create.Args {
			assert.NotContains(t, a, passphrase,
				"argv is world-readable on this machine: a passphrase there is readable by every other user")
		}
		assert.Contains(t, create.Stdin, passphrase, "the passphrase must reach op out of sight, on standard input")

		var tmpl onePasswordItemTemplate
		require.NoError(t, json.Unmarshal([]byte(create.Stdin), &tmpl),
			"op reads the item as JSON, so what is written must be an item op can read")
		assert.Equal(t, "sshakku-id_rsa", tmpl.Title, "the entry must be named after the key it belongs to")
		assert.Equal(t, "PASSWORD", tmpl.Category, "and be the kind of entry a password lives in")
		fields := map[string]string{}
		for _, f := range tmpl.Fields {
			fields[f.ID] = f.Value
		}
		assert.Equal(t, passphrase, fields["password"], "the passphrase must be what is saved")
		assert.Equal(t, "SSH Passphrase for id_rsa", fields["label"],
			"and the label must be what a person sees in their vault")
	})

	t.Run("existing item is deleted before recreating", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get":    stdout(`{"id":"abc123"}`, 0), // found
			"item delete": stdout("", 0),
			"item create": stdout("", 0),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		require.NoError(t, b.Store("sshakku-id_rsa", "label", passphrase), "replacing a passphrase must succeed")
		var verbs []string
		for _, c := range r.calls {
			require.GreaterOrEqualf(t, len(c.Args), 2, "every op call names a subcommand: %+v", c.Args)
			verbs = append(verbs, c.Args[0]+" "+c.Args[1])
		}
		assert.Equal(t, []string{"item get", "item delete", "item create"}, verbs,
			"op cannot edit an item's password in place, so the old entry must go before the new one lands: "+
				"leaving both would put two passphrases in the vault under the same name")
	})

	t.Run("a non-zero exit from create is an error", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get": stdout("", 1),
			"item create": func(Cmd) (Result, error) {
				return Result{Stderr: []byte("vault not found"), Code: 1}, nil
			},
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		assert.Error(t, b.Store("x", "y", passphrase),
			"a passphrase the vault refused to write must not be reported as saved")
	})
}

func TestOnePasswordDelete(t *testing.T) {
	t.Run("existing item: looks up then deletes", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get":    stdout(`{"id":"abc123"}`, 0),
			"item delete": stdout("", 0),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		require.NoError(t, b.Delete("sshakku-id_rsa"), "forgetting a passphrase must succeed")
		require.Lenf(t, r.calls, 2, "an entry that is there is looked up, then deleted: %+v", r.calls)
		assert.Equal(t, "delete", r.calls[1].Args[1], "and it must be the deletion")
	})

	t.Run("missing item is success, no delete call made", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get": stdout("", 1),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		require.NoError(t, b.Delete("sshakku-id_rsa"),
			"a passphrase that is already not there is the outcome that was asked for")
		assert.Lenf(t, r.calls, 1, "and nothing may be deleted when nothing matched: %+v", r.calls)
	})

	t.Run("a non-zero exit from delete is an error", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get": stdout(`{"id":"abc123"}`, 0),
			"item delete": func(Cmd) (Result, error) {
				return Result{Stderr: []byte("permission denied"), Code: 1}, nil
			},
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		assert.Error(t, b.Delete("x"), "a passphrase the vault refused to remove must not be reported as forgotten")
	})
}

func TestOnePasswordList(t *testing.T) {
	t.Run("returns each item's title", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, stdout(`[{"title":"sshakku-id_rsa"},{"title":"sshakku-id_ed25519"}]`, 0))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		got, err := b.List()
		require.NoError(t, err, "listing what the vault holds must succeed")
		assert.Equal(t, []string{"sshakku-id_rsa", "sshakku-id_ed25519"}, got,
			"every entry must be named, by the key it belongs to")
		require.NotEmpty(t, r.calls, "the vault must actually be asked")
		assert.Equal(t, []string{"item", "list", "--vault", "sshakku", "--tags", "sshakku", "--format", "json"},
			r.calls[0].Args,
			"the listing must be narrowed to SSHakku's own vault and tag: the rest of the account belongs to its owner, "+
				"and whatever is listed here is what forget --all goes on to delete")
	})

	t.Run("empty vault returns an empty, non-nil slice", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, stdout(`[]`, 0))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		got, err := b.List()
		require.NoError(t, err, "a vault holding nothing is not an error")
		assert.Empty(t, got, "and nothing may be listed")
	})

	t.Run("a non-zero exit is an error", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, func(Cmd) (Result, error) {
			return Result{Stderr: []byte("not signed in"), Code: 1}, nil
		})
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		_, err := b.List()
		assert.Error(t, err, "a vault that could not be read must not be listed as empty")
	})
}
