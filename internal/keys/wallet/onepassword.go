package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// onePasswordBin is the 1Password CLI. It authenticates out of band — either
// via the desktop app integration (system auth unlocks the app once; op then
// talks to it over a local socket with no per-command prompt) or a service
// account token in OP_SERVICE_ACCOUNT_TOKEN — so OnePassword itself
// never handles an account credential.
const onePasswordBin = "op"

// onePasswordTag marks every item OnePassword creates, and is the only thing
// that tells one of those from an item the user put in the same vault. A title
// cannot: the vault may hold anything its owner keeps there, including a name
// that looks like one SSHakku generates. So every operation that acts on an
// item — reading it, removing it, writing over it — asks for the mark first,
// and an item without it is somebody else's.
const onePasswordTag = "sshakku"

// onePasswordItemCommand is op's command group for items: the first word of
// each command line below that creates, gets, deletes or lists one.
const onePasswordItemCommand = "item"

// onePasswordVaultFlag confines every op command to Vault. An item created
// in one vault and looked up without naming it is one op resolves against
// whatever the account's default happens to be.
const onePasswordVaultFlag = "--vault"

// onePasswordPasswordField is the id and the label of the concealed field the
// passphrase is written to, which is also the field a secret reference reads
// back — the two have to spell it the same way.
const onePasswordPasswordField = "password"

// onePasswordField is one entry in an item JSON template's "fields" array
// (the shape `op item create -`/`op item template get` use).
type onePasswordField struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Purpose string `json:"purpose,omitempty"`
	Label   string `json:"label"`
	Value   string `json:"value"`
}

// onePasswordItemTemplate is the minimal item JSON template OnePassword
// sends to `op item create -` on stdin — never as assignment-statement
// arguments, which op's own docs warn are visible to other processes.
type onePasswordItemTemplate struct {
	Title    string             `json:"title"`
	Category string             `json:"category"`
	Tags     []string           `json:"tags"`
	Fields   []onePasswordField `json:"fields"`
}

// onePasswordItem is an item as `op item get` answers with one: what it is
// called, what marks it carries, and its fields when they were asked for.
type onePasswordItem struct {
	Title  string             `json:"title"`
	Tags   []string           `json:"tags"`
	Fields []onePasswordField `json:"fields"`
}

// ours reports whether SSHakku is what put this item in the vault. Everything
// else in the vault belongs to whoever put it there and is left exactly as it
// is — not read, not listed, not removed, not written over.
func (i onePasswordItem) ours() bool {
	return slices.Contains(i.Tags, onePasswordTag)
}

// password returns the item's password field, which is where Store puts a
// passphrase and where a Password-category item keeps one.
func (i onePasswordItem) password() (string, bool) {
	for _, f := range i.Fields {
		if f.ID == onePasswordPasswordField {
			return f.Value, true
		}
	}
	return "", false
}

// itemNotOursError is an item in Vault that SSHakku did not create, standing
// under the name SSHakku would store one of its own under. It is not removed
// and not written over — the vault may be one its owner keeps other things in,
// and an item of theirs stays theirs — so the passphrase is not saved and the
// clash is reported instead.
type itemNotOursError struct {
	vault string
	title string
}

func (e itemNotOursError) Error() string {
	return fmt.Sprintf("1Password vault %q already holds an item called %q that SSHakku did not create; "+
		"it has been left untouched and the passphrase was not saved — rename that item, "+
		"or point onepassword_vault at a different vault", e.vault, e.title)
}

// OnePassword stores and retrieves passphrases as items in a
// dedicated 1Password vault via the op CLI. Like SecretTool it shells
// out rather than speaking a native protocol.
//
// op has no way to edit an existing item's concealed field without either
// putting the new value in argv (an assignment statement) or writing it to a
// template file on disk (the --template flag takes a filepath, not stdin) —
// both worse than what SecretTool already avoids. Store instead
// deletes any existing item for service and creates a fresh one from a JSON
// template on stdin, so the passphrase only ever travels on stdin (Store) or
// comes back in op's own answer (Lookup), never in argv or a file.
//
// Vault (name or ID) does not have to be a vault kept for nothing else. Items
// are addressed by the service string sshakku generates (Loader.servicePrefix),
// which says where to look but never who an item belongs to: a vault its owner
// keeps other things in can hold that same title. Every item SSHakku creates is
// therefore marked (onePasswordTag), and every operation that would act on one
// checks the mark before it acts. What is not marked is not read, not listed,
// not removed, and not written over.
type OnePassword struct {
	Runner run.Runner
	Vault  string
	// Timeout bounds each op call. op can defer to the 1Password desktop app
	// for approval, so the budget is a person's, not a machine's; zero selects
	// run.DefaultInteractiveTimeout.
	Timeout time.Duration
}

// run bounds every op call, so an approval nobody grants ends as an error
// rather than as a shell that never comes back.
func (b *OnePassword) run(ctx context.Context, c run.Cmd) (run.Result, error) {
	if c.Timeout <= 0 {
		c.Timeout = b.Timeout
	}
	if c.Timeout <= 0 {
		c.Timeout = run.DefaultInteractiveTimeout
	}
	return b.Runner.Run(ctx, c)
}

// itemAt asks op for the item titled service in Vault. A non-zero exit is
// treated as a miss, not an error — op does not distinguish "item not found"
// from other failures by exit code alone, the same ambiguity SecretTool
// accepts.
//
// reveal fills in concealed field values. It is asked for only where such a
// value is what the caller came for, so a call that only needs to know whose
// item this is does not put that item's passphrase on a pipe.
func (b *OnePassword) itemAt(ctx context.Context, service string, reveal bool) (onePasswordItem, bool, error) {
	args := []string{onePasswordItemCommand, "get", service, onePasswordVaultFlag, b.Vault, "--format", "json"}
	if reveal {
		args = append(args, "--reveal")
	}
	res, err := b.run(ctx, run.Cmd{Name: onePasswordBin, Args: args})
	if err != nil {
		return onePasswordItem{}, false, err
	}
	if res.Code != 0 {
		return onePasswordItem{}, false, nil
	}
	var item onePasswordItem
	if err := json.Unmarshal(res.Stdout, &item); err != nil {
		return onePasswordItem{}, false, err
	}
	return item, true, nil
}

// Lookup reads the passphrase SSHakku stored under service. An item standing
// under that name that SSHakku did not create is not one of its own, so it is
// a miss rather than a passphrase: reading it would hand the caller somebody
// else's secret because the two happen to share a title.
func (b *OnePassword) Lookup(ctx context.Context, service string) (string, bool, error) {
	item, found, err := b.itemAt(ctx, service, true)
	if err != nil || !found || !item.ours() {
		return "", false, err
	}
	pass, ok := item.password()
	return pass, ok, nil
}

// Store replaces what SSHakku has stored under service with label and
// passphrase (see the type doc for why an in-place edit isn't used).
//
// An item there that SSHakku did not create is neither removed nor written
// over, and there is nowhere else to put this one: two items in a vault
// sharing a title cannot afterwards be told apart by the name that addresses
// them. So the clash is reported and the passphrase is not saved.
func (b *OnePassword) Store(ctx context.Context, service, label, passphrase string) error {
	item, found, err := b.itemAt(ctx, service, false)
	if err != nil {
		return err
	}
	if found && !item.ours() {
		return itemNotOursError{vault: b.Vault, title: service}
	}
	if found {
		if err := b.deleteItem(ctx, service); err != nil {
			return err
		}
	}

	payload, err := jsonMarshal(onePasswordItemTemplate{
		Title:    service,
		Category: "PASSWORD",
		Tags:     []string{onePasswordTag},
		Fields: []onePasswordField{
			{ID: "label", Type: "STRING", Label: "label", Value: label},
			{ID: onePasswordPasswordField, Type: "CONCEALED", Purpose: "PASSWORD", Label: onePasswordPasswordField, Value: passphrase},
		},
	})
	if err != nil {
		return err
	}

	res, err := b.run(ctx, run.Cmd{
		Name:  onePasswordBin,
		Args:  []string{onePasswordItemCommand, "create", onePasswordVaultFlag, b.Vault, "-"},
		Stdin: string(payload),
	})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return exitError{command: "op item create", code: res.Code, stderr: strings.TrimSpace(string(res.Stderr))}
	}
	return nil
}

// Delete removes what SSHakku stored under service. It looks the item up first
// so a miss — nothing to delete — can be reported as success rather than
// conflated with a real deletion failure, the same shape SecretService.Delete
// uses (search, then delete only what search found).
//
// An item there that SSHakku did not create is the same answer as no item at
// all: there is nothing of SSHakku's under that name to forget, and what is
// under it belongs to whoever put it there.
func (b *OnePassword) Delete(ctx context.Context, service string) error {
	item, found, err := b.itemAt(ctx, service, false)
	if err != nil || !found || !item.ours() {
		return err
	}
	return b.deleteItem(ctx, service)
}

// deleteItem removes the item titled service, whoever it belongs to. Every
// caller has established that already; this one only runs the command.
func (b *OnePassword) deleteItem(ctx context.Context, service string) error {
	res, err := b.run(ctx, run.Cmd{Name: onePasswordBin, Args: []string{onePasswordItemCommand, "delete", service, onePasswordVaultFlag, b.Vault}})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return exitError{command: "op item delete", code: res.Code, stderr: strings.TrimSpace(string(res.Stderr))}
	}
	return nil
}

// List enumerates every sshakku-tagged item's title in Vault. Since Vault is
// dedicated to sshakku (see the type doc), every title is a service string.
func (b *OnePassword) List(ctx context.Context) ([]string, error) {
	res, err := b.run(ctx, run.Cmd{Name: onePasswordBin, Args: []string{onePasswordItemCommand, "list", onePasswordVaultFlag, b.Vault, "--tags", onePasswordTag, "--format", "json"}})
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, exitError{command: "op item list", code: res.Code, stderr: strings.TrimSpace(string(res.Stderr))}
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

var _ Backend = (*OnePassword)(nil)
