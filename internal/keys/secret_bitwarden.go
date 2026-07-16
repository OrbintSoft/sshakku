package keys

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// bitwardenBin is the Bitwarden CLI. Unlike SecretServiceBackend (PAM-linked
// wallet unlock) and OnePasswordBackend (the 1Password desktop app's own
// system-auth integration), bw has no non-interactive unlock path today — a
// biometric/system-auth CLI integration is only a community wrapper, not an
// official bw feature — so BitwardenBackend prompts for the master password
// itself via Prompter every time it unlocks. It is never cached or written
// anywhere: only the resulting, short-lived bw session key is held in
// memory (Session), the same way the passphrases this backend stores never
// touch argv or disk.
const bitwardenBin = "bw"

// bitwardenPasswordEnv carries the master password to `bw login`/`bw unlock`
// via --passwordenv, so it travels through the child's environment rather
// than argv or a temp file.
const bitwardenPasswordEnv = "SSHAKKU_BW_PASSWORD"

// bitwardenLoginItemType is Bitwarden's numeric item category for a Login
// item (as opposed to a secure note, card, or identity) — the shape that
// has a built-in password field.
const bitwardenLoginItemType = 1

// bitwardenLogin is the "login" section of a Bitwarden Login item.
type bitwardenLogin struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	URIs     []string `json:"uris"`
}

// bitwardenItem is the JSON template `bw create item` / `bw edit item`
// expect, base64-encoded, on stdin — never as a command-line argument, which
// `bw`'s own docs warn is visible to other processes.
type bitwardenItem struct {
	Type     int            `json:"type"`
	Name     string         `json:"name"`
	Notes    string         `json:"notes"`
	Favorite bool           `json:"favorite"`
	Reprompt int            `json:"reprompt"`
	Fields   []any          `json:"fields"`
	Login    bitwardenLogin `json:"login"`
}

// bitwardenItemRef is the subset of `bw get item`'s JSON output BitwardenBackend
// needs: the item's globally unique id, required by `bw edit item` and
// `bw delete item` (unlike `bw get`, neither accepts a name/search term).
type bitwardenItemRef struct {
	ID string `json:"id"`
}

// BitwardenBackend stores and retrieves passphrases as Login items in a
// Bitwarden (or self-hosted Vaultwarden) account via the bw CLI, the same
// shell-out pattern SecretToolBackend and OnePasswordBackend use.
//
// The account is assumed dedicated to sshakku, the same simplification the
// dedicated Secret Service collection design made: `bw list items` returns
// only sshakku's own items, and each is addressed by Name (the service
// string sshakku itself generates) with no separate attribute search.
//
// Unlike op, bw supports a true in-place edit of an existing item via stdin
// (no argv-only limitation), so Store edits when an item already exists
// instead of deleting and recreating it.
//
// Every call locks again when it unlocked itself (see held below) — there
// is deliberately no session cache: a Bitwarden-backed key never re-prompts
// for the *SSH* key's own passphrase (that still comes from Bitwarden
// automatically), but each fresh sshakku invocation that actually needs
// Bitwarden re-prompts for the *master* password. The alternative — sshakku
// caching or storing the master password to avoid that — was deliberately
// rejected: it would let a single stolen secret unlock every credential
// this backend holds, well beyond this project's threat model for a single
// SSH key passphrase.
type BitwardenBackend struct {
	Runner   Runner
	Prompter Prompter
	// Email identifies the Bitwarden account to log into.
	Email string
	// Server, if set, points bw at a self-hosted Vaultwarden instance via
	// `bw config server` instead of the default bitwarden.com.
	Server string

	// Session is the bw unlock session key. Unlock sets it; the other
	// methods use it via BW_SESSION. It is set directly only by a caller
	// batching several calls under one Unlock/Lock (see SecretSession).
	Session string
	// held is true between an external Unlock and its matching Lock (see
	// SecretSession): Lookup/Store/Delete/List skip their own prompt/unlock/
	// lock bracket and reuse the held-open session instead.
	held bool
}

func (b *BitwardenBackend) env() []string {
	// The session key decrypts the whole account, so — like the passphrases
	// this backend stores — it travels only via the environment, never argv.
	return []string{"BW_SESSION=" + b.Session}
}

// Unlock prompts for the master password via Prompter, logs into Email if
// not already logged in, and unlocks to obtain a fresh session key — never
// caching or storing the password itself. The password reaches bw only via
// --passwordenv, never argv.
func (b *BitwardenBackend) Unlock() error {
	password, err := b.Prompter.Prompt("your Bitwarden master password")
	if err != nil {
		return err
	}
	passwordEnv := []string{bitwardenPasswordEnv + "=" + password}

	check, err := b.Runner.Run(Cmd{Name: bitwardenBin, Args: []string{"login", "--check"}})
	if err != nil {
		return err
	}
	if check.Code != 0 {
		// bw refuses to change the server config while logged in ("Logout
		// required before server config update"), so this only ever runs
		// as part of the first login, not on every Unlock.
		if b.Server != "" {
			res, err := b.Runner.Run(Cmd{Name: bitwardenBin, Args: []string{"config", "server", b.Server}})
			if err != nil {
				return err
			}
			if res.Code != 0 {
				return fmt.Errorf("bw config server exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
			}
		}

		res, err := b.Runner.Run(Cmd{
			Name: bitwardenBin,
			Args: []string{"login", b.Email, "--passwordenv", bitwardenPasswordEnv},
			Env:  passwordEnv,
		})
		if err != nil {
			return err
		}
		if res.Code != 0 {
			return fmt.Errorf("bw login exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
		}
	}

	res, err := b.Runner.Run(Cmd{
		Name: bitwardenBin,
		Args: []string{"unlock", "--passwordenv", bitwardenPasswordEnv, "--raw"},
		Env:  passwordEnv,
	})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("bw unlock exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}

	b.Session = strings.TrimSpace(string(res.Stdout))
	b.held = true
	return nil
}

// Lock destroys the current session (`bw lock`) and forgets it, regardless
// of whether the lock command itself succeeds.
func (b *BitwardenBackend) Lock() error {
	res, err := b.Runner.Run(Cmd{Name: bitwardenBin, Args: []string{"lock"}, Env: b.env()})
	b.Session = ""
	b.held = false
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("bw lock exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

var _ SecretSession = (*BitwardenBackend)(nil)

// findItemID looks up service by name and returns its id. A miss is
// found=false, not an error.
func (b *BitwardenBackend) findItemID(service string) (string, bool, error) {
	res, err := b.Runner.Run(Cmd{Name: bitwardenBin, Args: []string{"get", "item", service}, Env: b.env()})
	if err != nil {
		return "", false, err
	}
	if res.Code != 0 {
		return "", false, nil
	}
	var ref bitwardenItemRef
	if err := json.Unmarshal(res.Stdout, &ref); err != nil {
		return "", false, err
	}
	return ref.ID, true, nil
}

// Lookup runs `bw get password <service>`, prompting for the master
// password and unlocking first unless already held open by a batching
// caller (see SecretSession). A non-zero exit is treated as a miss, not an
// error — bw does not distinguish "item not found" from other failures by
// exit code alone, the same ambiguity SecretToolBackend accepts.
func (b *BitwardenBackend) Lookup(service string) (string, bool, error) {
	if !b.held {
		if err := b.Unlock(); err != nil {
			return "", false, err
		}
		defer func() { _ = b.Lock() }()
	}

	res, err := b.Runner.Run(Cmd{Name: bitwardenBin, Args: []string{"get", "password", service}, Env: b.env()})
	if err != nil {
		return "", false, err
	}
	if res.Code != 0 {
		return "", false, nil
	}
	return string(res.Stdout), true, nil
}

// Store edits the existing item for service if one exists, or creates a new
// one otherwise, holding label (as the login username, for a human-readable
// vault listing) and passphrase. Prompts for the master password and
// unlocks first unless already held open by a batching caller (see
// SecretSession).
func (b *BitwardenBackend) Store(service, label, passphrase string) error {
	if !b.held {
		if err := b.Unlock(); err != nil {
			return err
		}
		defer func() { _ = b.Lock() }()
	}

	id, found, err := b.findItemID(service)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(bitwardenItem{
		Type:   bitwardenLoginItemType,
		Name:   service,
		Fields: []any{},
		Login:  bitwardenLogin{Username: label, Password: passphrase, URIs: []string{}},
	})
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(payload)

	verb, args := "create", []string{"create", "item"}
	if found {
		verb, args = "edit", []string{"edit", "item", id}
	}
	res, err := b.Runner.Run(Cmd{Name: bitwardenBin, Args: args, Stdin: encoded, Env: b.env()})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("bw %s item exited %d: %s", verb, res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// Delete removes the item for service. A miss (nothing to delete) is
// success, not an error — the same contract as SecretServiceBackend.Delete,
// via the same search-then-delete shape (bw delete needs an id, not a name).
// Prompts for the master password and unlocks first unless already held
// open by a batching caller (see SecretSession).
func (b *BitwardenBackend) Delete(service string) error {
	if !b.held {
		if err := b.Unlock(); err != nil {
			return err
		}
		defer func() { _ = b.Lock() }()
	}

	id, found, err := b.findItemID(service)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	res, err := b.Runner.Run(Cmd{Name: bitwardenBin, Args: []string{"delete", "item", id, "--permanent"}, Env: b.env()})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("bw delete item exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// List returns the name of every item in the account. Since the account is
// dedicated to sshakku (see the type doc), every name is a service string.
// Prompts for the master password and unlocks first unless already held
// open by a batching caller (see SecretSession).
func (b *BitwardenBackend) List() ([]string, error) {
	if !b.held {
		if err := b.Unlock(); err != nil {
			return nil, err
		}
		defer func() { _ = b.Lock() }()
	}

	res, err := b.Runner.Run(Cmd{Name: bitwardenBin, Args: []string{"list", "items"}, Env: b.env()})
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("bw list items exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}

	var items []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(res.Stdout, &items); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	return names, nil
}

var _ SecretBackend = (*BitwardenBackend)(nil)
