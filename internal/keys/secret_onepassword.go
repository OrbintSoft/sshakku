package keys

import (
	"encoding/json"
	"fmt"
	"strings"
)

// onePasswordBin is the 1Password CLI. It authenticates out of band — either
// via the desktop app integration (system auth unlocks the app once; op then
// talks to it over a local socket with no per-command prompt) or a service
// account token in OP_SERVICE_ACCOUNT_TOKEN — so OnePasswordBackend itself
// never handles an account credential.
const onePasswordBin = "op"

// onePasswordTag marks every item OnePasswordBackend creates, mirroring the
// dedicated-collection assumption of SecretServiceBackend: Vault is expected
// to hold nothing but sshakku's own items, so the tag is defense in depth
// for List rather than the only thing distinguishing sshakku's items from a
// user's own.
const onePasswordTag = "sshakku"

// onePasswordField is one entry in an item JSON template's "fields" array
// (the shape `op item create -`/`op item template get` use).
type onePasswordField struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Purpose string `json:"purpose,omitempty"`
	Label   string `json:"label"`
	Value   string `json:"value"`
}

// onePasswordItemTemplate is the minimal item JSON template OnePasswordBackend
// sends to `op item create -` on stdin — never as assignment-statement
// arguments, which op's own docs warn are visible to other processes.
type onePasswordItemTemplate struct {
	Title    string             `json:"title"`
	Category string             `json:"category"`
	Tags     []string           `json:"tags"`
	Fields   []onePasswordField `json:"fields"`
}

// OnePasswordBackend stores and retrieves passphrases as items in a
// dedicated 1Password vault via the op CLI. Like SecretToolBackend it shells
// out rather than speaking a native protocol.
//
// op has no way to edit an existing item's concealed field without either
// putting the new value in argv (an assignment statement) or writing it to a
// template file on disk (the --template flag takes a filepath, not stdin) —
// both worse than what SecretToolBackend already avoids. Store instead
// deletes any existing item for service and creates a fresh one from a JSON
// template on stdin, so the passphrase only ever travels on stdin (Store) or
// is read back via a secret reference (Lookup), never in argv or a file.
//
// Vault must be a vault dedicated to sshakku (name or ID) — every item in it
// is titled with the service string sshakku itself generates
// (Loader.servicePrefix), so Lookup/Delete/List address and enumerate items
// by title with no separate attribute search, the same simplification the
// dedicated Secret Service collection design made.
type OnePasswordBackend struct {
	Runner Runner
	Vault  string
}

// Lookup reads the item's password field via a secret reference
// (op://<vault>/<service>/password). A non-zero exit is treated as a miss,
// not an error — op does not distinguish "item not found" from other
// failures by exit code alone, the same ambiguity SecretToolBackend accepts.
func (b *OnePasswordBackend) Lookup(service string) (string, bool, error) {
	ref := fmt.Sprintf("op://%s/%s/password", b.Vault, service)
	res, err := b.Runner.Run(Cmd{Name: onePasswordBin, Args: []string{"read", ref, "--no-newline"}})
	if err != nil {
		return "", false, err
	}
	if res.Code != 0 {
		return "", false, nil
	}
	return string(res.Stdout), true, nil
}

// Store deletes any existing item for service (see the type doc for why an
// in-place edit isn't used) and creates a fresh one holding label and
// passphrase.
func (b *OnePasswordBackend) Store(service, label, passphrase string) error {
	if err := b.Delete(service); err != nil {
		return err
	}

	payload, err := json.Marshal(onePasswordItemTemplate{
		Title:    service,
		Category: "PASSWORD",
		Tags:     []string{onePasswordTag},
		Fields: []onePasswordField{
			{ID: "label", Type: "STRING", Label: "label", Value: label},
			{ID: "password", Type: "CONCEALED", Purpose: "PASSWORD", Label: "password", Value: passphrase},
		},
	})
	if err != nil {
		return err
	}

	res, err := b.Runner.Run(Cmd{
		Name:  onePasswordBin,
		Args:  []string{"item", "create", "--vault", b.Vault, "-"},
		Stdin: string(payload),
	})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("op item create exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// Delete removes the item for service. It looks the item up first so a miss
// — nothing to delete — can be reported as success rather than conflated
// with a real deletion failure, the same shape SecretServiceBackend.Delete
// uses (search, then delete only what search found).
func (b *OnePasswordBackend) Delete(service string) error {
	res, err := b.Runner.Run(Cmd{Name: onePasswordBin, Args: []string{"item", "get", service, "--vault", b.Vault, "--format", "json"}})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return nil
	}

	res, err = b.Runner.Run(Cmd{Name: onePasswordBin, Args: []string{"item", "delete", service, "--vault", b.Vault}})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("op item delete exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// List enumerates every sshakku-tagged item's title in Vault. Since Vault is
// dedicated to sshakku (see the type doc), every title is a service string.
func (b *OnePasswordBackend) List() ([]string, error) {
	res, err := b.Runner.Run(Cmd{Name: onePasswordBin, Args: []string{"item", "list", "--vault", b.Vault, "--tags", onePasswordTag, "--format", "json"}})
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("op item list exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}

	var items []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(res.Stdout, &items); err != nil {
		return nil, err
	}
	services := make([]string, 0, len(items))
	for _, it := range items {
		services = append(services, it.Title)
	}
	return services, nil
}

var _ SecretBackend = (*OnePasswordBackend)(nil)
