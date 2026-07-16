package keys

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found || pass != "hunter2" {
			t.Fatalf("Lookup = (%q, %v), want (hunter2, true)", pass, found)
		}
		call := r.calls[0]
		want := []string{"get", "password", "sshakku-id_rsa"}
		if !equalStrings(call.Args, want) {
			t.Fatalf("args = %v, want %v", call.Args, want)
		}
		if !hasSessionEnv(call, "sess-token") {
			t.Fatalf("env = %v, want BW_SESSION=sess-token", call.Env)
		}
		for _, a := range call.Args {
			if strings.Contains(a, "sess-token") {
				t.Fatalf("session key leaked into argv: %q", a)
			}
		}
	})

	t.Run("miss is found=false, no error", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, stdout("Not found.", 1))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		_, found, err := b.Lookup("sshakku-id_rsa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("found = true, want false for a miss")
		}
	})

	t.Run("a failure to start bw is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		b := &BitwardenBackend{Runner: newFakeRunner().on(bitwardenBin, fails(wantErr)), Session: "sess-token", held: true}
		if _, _, err := b.Lookup("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
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
		if err := b.Store("sshakku-id_rsa", "SSH Passphrase for id_rsa", passphrase); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(r.calls) != 2 {
			t.Fatalf("expected 2 bw calls (get, create), got %d: %+v", len(r.calls), r.calls)
		}
		create := r.calls[1]
		if !equalStrings(create.Args, []string{"create", "item"}) {
			t.Fatalf("args = %v, want [create item]", create.Args)
		}
		for _, a := range create.Args {
			if strings.Contains(a, passphrase) {
				t.Fatalf("passphrase leaked into argv: %q", a)
			}
		}
		if strings.Contains(create.Stdin, passphrase) {
			t.Fatal("stdin is not base64-encoded: passphrase appears verbatim")
		}

		decoded, err := base64.StdEncoding.DecodeString(create.Stdin)
		if err != nil {
			t.Fatalf("stdin is not valid base64: %v", err)
		}
		var item bitwardenItem
		if err := json.Unmarshal(decoded, &item); err != nil {
			t.Fatalf("decoded stdin is not valid item JSON: %v", err)
		}
		if item.Name != "sshakku-id_rsa" || item.Type != bitwardenLoginItemType {
			t.Fatalf("item = %+v, want name=sshakku-id_rsa type=%d", item, bitwardenLoginItemType)
		}
		if item.Login.Password != passphrase {
			t.Fatalf("login.password = %q, want %q", item.Login.Password, passphrase)
		}
		if item.Login.Username != "SSH Passphrase for id_rsa" {
			t.Fatalf("login.username = %q, want the label", item.Login.Username)
		}
	})

	t.Run("existing item is edited in place, not deleted", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item":  stdout(`{"id":"abc123"}`, 0),
			"edit item": stdout(`{"id":"abc123"}`, 0),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		if err := b.Store("sshakku-id_rsa", "label", passphrase); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 2 {
			t.Fatalf("expected 2 bw calls (get, edit), got %d: %+v", len(r.calls), r.calls)
		}
		edit := r.calls[1]
		if !equalStrings(edit.Args, []string{"edit", "item", "abc123"}) {
			t.Fatalf("args = %v, want [edit item abc123]", edit.Args)
		}
	})

	t.Run("a non-zero exit from create is an error", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item": stdout("Not found.", 1),
			"create item": func(Cmd) (Result, error) {
				return Result{Stderr: []byte("vault is locked"), Code: 1}, nil
			},
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		if err := b.Store("x", "y", passphrase); err == nil {
			t.Fatal("expected an error for a non-zero exit")
		}
	})
}

func TestBitwardenDelete(t *testing.T) {
	t.Run("existing item: looks up id then deletes permanently", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item":    stdout(`{"id":"abc123"}`, 0),
			"delete item": stdout("", 0),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		if err := b.Delete("sshakku-id_rsa"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		del := r.calls[1]
		if !equalStrings(del.Args, []string{"delete", "item", "abc123", "--permanent"}) {
			t.Fatalf("args = %v, want [delete item abc123 --permanent]", del.Args)
		}
	})

	t.Run("missing item is success, no delete call made", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item": stdout("Not found.", 1),
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		if err := b.Delete("sshakku-id_rsa"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 1 {
			t.Fatalf("expected only the lookup call, got %d: %+v", len(r.calls), r.calls)
		}
	})

	t.Run("a non-zero exit from delete is an error", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"get item": stdout(`{"id":"abc123"}`, 0),
			"delete item": func(Cmd) (Result, error) {
				return Result{Stderr: []byte("permission denied"), Code: 1}, nil
			},
		}))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		if err := b.Delete("x"); err == nil {
			t.Fatal("expected an error for a non-zero exit")
		}
	})
}

func TestBitwardenList(t *testing.T) {
	t.Run("returns each item's name", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, stdout(`[{"name":"sshakku-id_rsa"},{"name":"sshakku-id_ed25519"}]`, 0))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		got, err := b.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"sshakku-id_rsa", "sshakku-id_ed25519"}
		if !equalStrings(got, want) {
			t.Fatalf("List = %v, want %v", got, want)
		}
		if gotArgs := r.calls[0].Args; !equalStrings(gotArgs, []string{"list", "items"}) {
			t.Fatalf("args = %v, want [list items]", gotArgs)
		}
	})

	t.Run("empty account returns an empty, non-nil slice", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, stdout(`[]`, 0))
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		got, err := b.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("List = %v, want empty", got)
		}
	})

	t.Run("a non-zero exit is an error", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, func(Cmd) (Result, error) {
			return Result{Stderr: []byte("vault is locked"), Code: 1}, nil
		})
		b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
		if _, err := b.List(); err == nil {
			t.Fatal("expected an error for a non-zero exit")
		}
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

		if err := b.Unlock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.Session != "fresh-session-key" {
			t.Fatalf("Session = %q, want fresh-session-key", b.Session)
		}
		if !b.held {
			t.Fatal("held = false, want true after Unlock")
		}
		if len(p.calls) != 1 {
			t.Fatalf("expected exactly one prompt, got %d", len(p.calls))
		}

		unlockCall := r.calls[len(r.calls)-1]
		for _, a := range unlockCall.Args {
			if strings.Contains(a, "correct horse battery staple") {
				t.Fatalf("master password leaked into argv: %q", a)
			}
		}
		wantEnv := bitwardenPasswordEnv + "=correct horse battery staple"
		found := false
		for _, e := range unlockCall.Env {
			if e == wantEnv {
				found = true
			}
		}
		if !found {
			t.Fatalf("env = %v, want it to contain %q", unlockCall.Env, wantEnv)
		}
	})

	t.Run("not logged in: logs in first, then unlocks", func(t *testing.T) {
		r := newFakeRunner().on(bitwardenBin, bwCall(map[string]func(Cmd) (Result, error){
			"login --check":        stdout("", 1), // not logged in
			"login":                stdout("", 0),
			"unlock --passwordenv": stdout("fresh-session-key", 0),
		}))
		p := &fakePrompter{pass: "hunter2"}
		b := &BitwardenBackend{Runner: r, Prompter: p, Email: "sshakku-test@example.invalid"}

		if err := b.Unlock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var verbs []string
		for _, c := range r.calls {
			verbs = append(verbs, c.Args[0])
		}
		want := []string{"login", "login", "unlock"}
		if !equalStrings(verbs, want) {
			t.Fatalf("call sequence = %v, want %v", verbs, want)
		}
		loginCall := r.calls[1]
		if loginCall.Args[1] != "sshakku-test@example.invalid" {
			t.Fatalf("login email arg = %q, want the configured Email", loginCall.Args[1])
		}
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

		if err := b.Unlock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		configCall := r.calls[1]
		if !equalStrings(configCall.Args, []string{"config", "server", "https://vault.example.invalid"}) {
			t.Fatalf("args = %v, want [config server https://vault.example.invalid]", configCall.Args)
		}
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

		if err := b.Unlock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("a canceled prompt is returned as-is, no bw call made", func(t *testing.T) {
		r := newFakeRunner()
		p := &fakePrompter{err: ErrPromptCanceled}
		b := &BitwardenBackend{Runner: r, Prompter: p}
		if err := b.Unlock(); !errors.Is(err, ErrPromptCanceled) {
			t.Fatalf("error = %v, want ErrPromptCanceled", err)
		}
		if len(r.calls) != 0 {
			t.Fatalf("expected no bw calls, got %d: %+v", len(r.calls), r.calls)
		}
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
		if err := b.Unlock(); err == nil {
			t.Fatal("expected an error for a non-zero exit")
		}
		if b.held {
			t.Fatal("held = true, want false after a failed Unlock")
		}
	})
}

func TestBitwardenLock(t *testing.T) {
	r := newFakeRunner().on(bitwardenBin, stdout("", 0))
	b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true}
	if err := b.Lock(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Session != "" {
		t.Fatalf("Session = %q, want empty after Lock", b.Session)
	}
	if b.held {
		t.Fatal("held = true, want false after Lock")
	}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found || pass != "hunter2" {
			t.Fatalf("Lookup = (%q, %v), want (hunter2, true)", pass, found)
		}
		if len(p.calls) != 1 {
			t.Fatalf("expected exactly one prompt, got %d", len(p.calls))
		}
		if b.held || b.Session != "" {
			t.Fatal("expected the session to be locked and forgotten after a standalone call")
		}

		var verbs []string
		for _, c := range r.calls {
			verbs = append(verbs, c.Args[0])
		}
		want := []string{"login", "unlock", "get", "lock"}
		if !equalStrings(verbs, want) {
			t.Fatalf("call sequence = %v, want %v", verbs, want)
		}
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
		if _, _, err := b.Lookup("x"); err == nil {
			t.Fatal("expected an error when Unlock fails")
		}
	})
}
