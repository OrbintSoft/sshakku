package keys

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found || pass != "hunter2" {
			t.Fatalf("Lookup = (%q, %v), want (hunter2, true)", pass, found)
		}
		want := []string{"read", "op://sshakku/sshakku-id_rsa/password", "--no-newline"}
		if got := r.calls[0].Args; !equalStrings(got, want) {
			t.Fatalf("args = %v, want %v", got, want)
		}
	})

	t.Run("miss is found=false, no error", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, stdout("", 1))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		_, found, err := b.Lookup("sshakku-id_rsa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("found = true, want false for a miss")
		}
	})

	t.Run("a failure to start op is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		b := &OnePasswordBackend{Runner: newFakeRunner().on(onePasswordBin, fails(wantErr)), Vault: "sshakku"}
		if _, _, err := b.Lookup("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
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
		if err := b.Store("sshakku-id_rsa", "SSH Passphrase for id_rsa", passphrase); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(r.calls) != 2 {
			t.Fatalf("expected 2 op calls (get, create), got %d: %+v", len(r.calls), r.calls)
		}
		create := r.calls[1]
		for _, a := range create.Args {
			if strings.Contains(a, passphrase) {
				t.Fatalf("passphrase leaked into argv: %q", a)
			}
		}
		if !strings.Contains(create.Stdin, passphrase) {
			t.Fatalf("stdin = %q, want it to contain the passphrase", create.Stdin)
		}

		var tmpl onePasswordItemTemplate
		if err := json.Unmarshal([]byte(create.Stdin), &tmpl); err != nil {
			t.Fatalf("stdin is not valid item JSON: %v", err)
		}
		if tmpl.Title != "sshakku-id_rsa" || tmpl.Category != "PASSWORD" {
			t.Fatalf("template = %+v, want title=sshakku-id_rsa category=PASSWORD", tmpl)
		}
		var gotPass, gotLabel string
		for _, f := range tmpl.Fields {
			switch f.ID {
			case "password":
				gotPass = f.Value
			case "label":
				gotLabel = f.Value
			}
		}
		if gotPass != passphrase {
			t.Fatalf("password field = %q, want %q", gotPass, passphrase)
		}
		if gotLabel != "SSH Passphrase for id_rsa" {
			t.Fatalf("label field = %q, want %q", gotLabel, "SSH Passphrase for id_rsa")
		}
	})

	t.Run("existing item is deleted before recreating", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get":    stdout(`{"id":"abc123"}`, 0), // found
			"item delete": stdout("", 0),
			"item create": stdout("", 0),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		if err := b.Store("sshakku-id_rsa", "label", passphrase); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var verbs []string
		for _, c := range r.calls {
			verbs = append(verbs, c.Args[0]+" "+c.Args[1])
		}
		want := []string{"item get", "item delete", "item create"}
		if !equalStrings(verbs, want) {
			t.Fatalf("call sequence = %v, want %v", verbs, want)
		}
	})

	t.Run("a non-zero exit from create is an error", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get": stdout("", 1),
			"item create": func(Cmd) (Result, error) {
				return Result{Stderr: []byte("vault not found"), Code: 1}, nil
			},
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		if err := b.Store("x", "y", passphrase); err == nil {
			t.Fatal("expected an error for a non-zero exit")
		}
	})
}

func TestOnePasswordDelete(t *testing.T) {
	t.Run("existing item: looks up then deletes", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get":    stdout(`{"id":"abc123"}`, 0),
			"item delete": stdout("", 0),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		if err := b.Delete("sshakku-id_rsa"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 2 {
			t.Fatalf("expected 2 op calls, got %d: %+v", len(r.calls), r.calls)
		}
	})

	t.Run("missing item is success, no delete call made", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get": stdout("", 1),
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		if err := b.Delete("sshakku-id_rsa"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 1 {
			t.Fatalf("expected only the lookup call, got %d: %+v", len(r.calls), r.calls)
		}
	})

	t.Run("a non-zero exit from delete is an error", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, opCall(map[string]func(Cmd) (Result, error){
			"item get": stdout(`{"id":"abc123"}`, 0),
			"item delete": func(Cmd) (Result, error) {
				return Result{Stderr: []byte("permission denied"), Code: 1}, nil
			},
		}))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		if err := b.Delete("x"); err == nil {
			t.Fatal("expected an error for a non-zero exit")
		}
	})
}

func TestOnePasswordList(t *testing.T) {
	t.Run("returns each item's title", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, stdout(`[{"title":"sshakku-id_rsa"},{"title":"sshakku-id_ed25519"}]`, 0))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		got, err := b.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"sshakku-id_rsa", "sshakku-id_ed25519"}
		if !equalStrings(got, want) {
			t.Fatalf("List = %v, want %v", got, want)
		}
		wantArgs := []string{"item", "list", "--vault", "sshakku", "--tags", "sshakku", "--format", "json"}
		if gotArgs := r.calls[0].Args; !equalStrings(gotArgs, wantArgs) {
			t.Fatalf("args = %v, want %v", gotArgs, wantArgs)
		}
	})

	t.Run("empty vault returns an empty, non-nil slice", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, stdout(`[]`, 0))
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		got, err := b.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("List = %v, want empty", got)
		}
	})

	t.Run("a non-zero exit is an error", func(t *testing.T) {
		r := newFakeRunner().on(onePasswordBin, func(Cmd) (Result, error) {
			return Result{Stderr: []byte("not signed in"), Code: 1}, nil
		})
		b := &OnePasswordBackend{Runner: r, Vault: "sshakku"}
		if _, err := b.List(); err == nil {
			t.Fatal("expected an error for a non-zero exit")
		}
	})
}
