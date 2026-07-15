package keys

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// bitwardenBin is the Bitwarden CLI. BitwardenBackend never runs `bw login` or
// `bw unlock` itself and never handles the account's master password — it
// only uses a session key that is already available (Session), the same
// separation of concerns SecretServiceBackend has from the desktop's own
// wallet unlock and OnePasswordBackend has from the 1Password desktop app.
const bitwardenBin = "bw"

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
type BitwardenBackend struct {
	Runner Runner
	// Session is a bw unlock session key, supplied already-unlocked — see
	// the type doc for why obtaining and refreshing it is out of scope here.
	Session string
}

func (b *BitwardenBackend) env() []string {
	// The session key decrypts the whole account, so — like the passphrases
	// this backend stores — it travels only via the environment, never argv.
	return []string{"BW_SESSION=" + b.Session}
}

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

// Lookup runs `bw get password <service>`. A non-zero exit is treated as a
// miss, not an error — bw does not distinguish "item not found" from other
// failures by exit code alone, the same ambiguity SecretToolBackend accepts.
func (b *BitwardenBackend) Lookup(service string) (string, bool, error) {
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
// vault listing) and passphrase.
func (b *BitwardenBackend) Store(service, label, passphrase string) error {
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
func (b *BitwardenBackend) Delete(service string) error {
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
func (b *BitwardenBackend) List() ([]string, error) {
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
