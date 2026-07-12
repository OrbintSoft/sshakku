// Package secretservice is a native client for the freedesktop Secret Service
// D-Bus API (org.freedesktop.secrets), implemented identically by KDE
// Wallet's ksecretd and GNOME Keyring. Unlike shelling out to secret-tool —
// which only ever targets the desktop's default collection and has no
// lock/unlock verbs — this client can create a dedicated collection and
// unlock/lock it explicitly around a single lookup or store.
package secretservice

import (
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	busName  = "org.freedesktop.secrets"
	rootPath = dbus.ObjectPath("/org/freedesktop/secrets")

	serviceIface    = "org.freedesktop.Secret.Service"
	collectionIface = "org.freedesktop.Secret.Collection"
	itemIface       = "org.freedesktop.Secret.Item"
	promptIface     = "org.freedesktop.Secret.Prompt"
	sessionIface    = "org.freedesktop.Secret.Session"

	serviceCollectionsProp = "org.freedesktop.Secret.Service.Collections"
	collectionLabelProp    = "org.freedesktop.Secret.Collection.Label"
	collectionItemsProp    = "org.freedesktop.Secret.Collection.Items"
	itemLabelProp          = "org.freedesktop.Secret.Item.Label"
	itemAttributesProp     = "org.freedesktop.Secret.Item.Attributes"

	// errNotSupported is the D-Bus error name a Secret Service implementation
	// returns for a CreateCollection call it structurally cannot satisfy —
	// observed live against GNOME Keyring 48 ("Only the 'default' alias is
	// supported") for any alias other than "" or "default", unlike KDE's
	// ksecretd, which accepts an arbitrary alias. Collection falls back to
	// Label-based lookup/unaliased creation only for this specific error.
	errNotSupported = "org.freedesktop.DBus.Error.NotSupported"

	// noPrompt is the sentinel object path the Secret Service returns in
	// place of a real prompt path when no interactive confirmation is needed.
	noPrompt = dbus.ObjectPath("/")
)

// promptTimeout bounds how long Unlock/Lock/CreateCollection/CreateItem wait
// for an interactive prompt to complete before dismissing it and treating the
// operation as failed. A var, not a const, so tests can shorten it.
var promptTimeout = 30 * time.Second

// Secret is the D-Bus Secret Service "Secret" struct. sshakku only ever
// negotiates the "plain" session algorithm, so Parameters is always empty and
// Value is the passphrase bytes verbatim (no decryption step).
type Secret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// Client is a connection to the Secret Service over the D-Bus session bus.
type Client struct {
	conn    *dbus.Conn
	service dbus.BusObject
	session dbus.ObjectPath
}

// NewClient connects to the session bus and negotiates a "plain" Secret
// Service session — no transport encryption, matching the trust boundary
// secret-tool already relied on (the session bus is restricted to this user).
func NewClient() (*Client, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("secret service: connect session bus: %w", err)
	}

	service := conn.Object(busName, rootPath)
	var (
		output  dbus.Variant
		session dbus.ObjectPath
	)
	call := service.Call(serviceIface+".OpenSession", 0, "plain", dbus.MakeVariant(""))
	if err := call.Store(&output, &session); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("secret service: open session: %w", err)
	}

	return &Client{conn: conn, service: service, session: session}, nil
}

// Close ends the Secret Service session and closes the bus connection.
func (c *Client) Close() error {
	_ = c.conn.Object(busName, c.session).Call(sessionIface+".Close", 0).Err
	return c.conn.Close()
}

// Collection resolves the object path of the named collection (matched by
// alias), creating it — with a matching alias — if it does not already
// exist. Some Secret Service implementations only ever accept the "default"
// alias (GNOME Keyring, confirmed live: CreateCollection with any other alias
// fails with errNotSupported before even offering a prompt); on that specific
// error, Collection falls back to finding an already-created collection by
// its Label property instead, and to creating a new, unaliased one
// (alias "") if none exists yet — the alias fast path above still applies
// unchanged for a backend like KDE's ksecretd that supports it.
func (c *Client) Collection(alias, label string) (dbus.ObjectPath, error) {
	var existing dbus.ObjectPath
	if err := c.service.Call(serviceIface+".ReadAlias", 0, alias).Store(&existing); err != nil {
		return "", fmt.Errorf("secret service: read alias %q: %w", alias, err)
	}
	if existing != noPrompt {
		return existing, nil
	}

	collection, prompt, err := c.createCollection(alias, label)
	if isAliasNotSupported(err) {
		found, ferr := c.findCollectionByLabel(label)
		if ferr != nil {
			return "", ferr
		}
		if found != noPrompt {
			return found, nil
		}
		collection, prompt, err = c.createCollection("", label)
	}
	if err != nil {
		return "", fmt.Errorf("secret service: create collection %q: %w", alias, err)
	}
	if prompt == noPrompt {
		return collection, nil
	}

	result, err := c.completePrompt(prompt)
	if err != nil {
		return "", err
	}
	var created dbus.ObjectPath
	if err := result.Store(&created); err != nil {
		return "", fmt.Errorf("secret service: create collection %q: unexpected prompt result: %w", alias, err)
	}
	return created, nil
}

// createCollection is the raw CreateCollection call, factored out so
// Collection can retry it with a different alias on isAliasNotSupported.
func (c *Client) createCollection(alias, label string) (collection, prompt dbus.ObjectPath, err error) {
	props := map[string]dbus.Variant{collectionLabelProp: dbus.MakeVariant(label)}
	err = c.service.Call(serviceIface+".CreateCollection", 0, props, alias).Store(&collection, &prompt)
	return collection, prompt, err
}

// isAliasNotSupported reports whether err is the D-Bus error a Secret
// Service implementation returns when it rejects a CreateCollection alias
// outright, rather than one of the many other reasons the call can fail
// (session bus unreachable, permission denied, …) that must still surface as
// a real error instead of triggering the Label-based fallback.
func isAliasNotSupported(err error) bool {
	var dbusErr dbus.Error
	return errors.As(err, &dbusErr) && dbusErr.Name == errNotSupported
}

// findCollectionByLabel searches every collection the service currently
// knows about for one whose Label property matches label, for backends where
// Collection's alias could not be set (isAliasNotSupported) so ReadAlias can
// never find it on a later call. Returns noPrompt, not an error, when
// nothing matches.
func (c *Client) findCollectionByLabel(label string) (dbus.ObjectPath, error) {
	v, err := c.service.GetProperty(serviceCollectionsProp)
	if err != nil {
		return "", fmt.Errorf("secret service: list collections: %w", err)
	}
	collections, ok := v.Value().([]dbus.ObjectPath)
	if !ok {
		return "", fmt.Errorf("secret service: list collections: unexpected property type %T", v.Value())
	}
	for _, collection := range collections {
		v, err := c.conn.Object(busName, collection).GetProperty(collectionLabelProp)
		if err != nil {
			continue
		}
		if l, ok := v.Value().(string); ok && l == label {
			return collection, nil
		}
	}
	return noPrompt, nil
}

// Unlock unlocks the given objects (typically a single collection),
// completing an interactive prompt if the Secret Service requires one.
func (c *Client) Unlock(objects ...dbus.ObjectPath) error {
	return c.unlockOrLock(serviceIface+".Unlock", objects)
}

// Lock locks the given objects, completing an interactive prompt if the
// Secret Service requires one.
func (c *Client) Lock(objects ...dbus.ObjectPath) error {
	return c.unlockOrLock(serviceIface+".Lock", objects)
}

func (c *Client) unlockOrLock(method string, objects []dbus.ObjectPath) error {
	var (
		done   []dbus.ObjectPath
		prompt dbus.ObjectPath
	)
	if err := c.service.Call(method, 0, objects).Store(&done, &prompt); err != nil {
		return fmt.Errorf("secret service: %s: %w", method, err)
	}
	if prompt == noPrompt {
		return nil
	}
	_, err := c.completePrompt(prompt)
	return err
}

// SearchItems returns the items in collection whose attributes match attrs
// exactly (all given attributes present with equal values).
func (c *Client) SearchItems(collection dbus.ObjectPath, attrs map[string]string) ([]dbus.ObjectPath, error) {
	var items []dbus.ObjectPath
	obj := c.conn.Object(busName, collection)
	if err := obj.Call(collectionIface+".SearchItems", 0, attrs).Store(&items); err != nil {
		return nil, fmt.Errorf("secret service: search items: %w", err)
	}
	return items, nil
}

// GetSecret returns the plaintext value of item, decoded via the client's
// plain session (no decryption needed).
func (c *Client) GetSecret(item dbus.ObjectPath) (string, error) {
	var secret Secret
	obj := c.conn.Object(busName, item)
	if err := obj.Call(itemIface+".GetSecret", 0, c.session).Store(&secret); err != nil {
		return "", fmt.Errorf("secret service: get secret: %w", err)
	}
	return string(secret.Value), nil
}

// CreateItem stores passphrase under attrs in collection, labelled label.
// replace controls whether an existing item with the same attrs is
// overwritten in place rather than duplicated.
func (c *Client) CreateItem(collection dbus.ObjectPath, label string, attrs map[string]string, passphrase string, replace bool) error {
	props := map[string]dbus.Variant{
		itemLabelProp:      dbus.MakeVariant(label),
		itemAttributesProp: dbus.MakeVariant(attrs),
	}
	secret := Secret{Session: c.session, Value: []byte(passphrase), ContentType: "text/plain"}

	var (
		item   dbus.ObjectPath
		prompt dbus.ObjectPath
	)
	obj := c.conn.Object(busName, collection)
	call := obj.Call(collectionIface+".CreateItem", 0, props, secret, replace)
	if err := call.Store(&item, &prompt); err != nil {
		return fmt.Errorf("secret service: create item %q: %w", label, err)
	}
	if prompt == noPrompt {
		return nil
	}
	_, err := c.completePrompt(prompt)
	return err
}

// Items returns every item currently in collection, regardless of attributes
// — unlike SearchItems, which only returns items matching a given filter.
func (c *Client) Items(collection dbus.ObjectPath) ([]dbus.ObjectPath, error) {
	obj := c.conn.Object(busName, collection)
	v, err := obj.GetProperty(collectionItemsProp)
	if err != nil {
		return nil, fmt.Errorf("secret service: collection items: %w", err)
	}
	items, ok := v.Value().([]dbus.ObjectPath)
	if !ok {
		return nil, fmt.Errorf("secret service: collection items: unexpected property type %T", v.Value())
	}
	return items, nil
}

// ItemAttributes returns the lookup attributes (e.g. "service", "username")
// item was stored under.
func (c *Client) ItemAttributes(item dbus.ObjectPath) (map[string]string, error) {
	obj := c.conn.Object(busName, item)
	v, err := obj.GetProperty(itemAttributesProp)
	if err != nil {
		return nil, fmt.Errorf("secret service: item attributes: %w", err)
	}
	attrs, ok := v.Value().(map[string]string)
	if !ok {
		return nil, fmt.Errorf("secret service: item attributes: unexpected property type %T", v.Value())
	}
	return attrs, nil
}

// DeleteItem removes item from its collection, completing an interactive
// prompt if the Secret Service requires one.
func (c *Client) DeleteItem(item dbus.ObjectPath) error {
	var prompt dbus.ObjectPath
	obj := c.conn.Object(busName, item)
	if err := obj.Call(itemIface+".Delete", 0).Store(&prompt); err != nil {
		return fmt.Errorf("secret service: delete item: %w", err)
	}
	if prompt == noPrompt {
		return nil
	}
	_, err := c.completePrompt(prompt)
	return err
}

// completePrompt drives a Secret Service Prompt object to completion: it
// subscribes to its Completed signal, invokes Prompt, and waits — bounded by
// promptTimeout, after which the prompt is dismissed and an error returned.
// A user-dismissed prompt is also reported as an error, never as a silent
// zero value, since callers must not confuse "dismissed" with "not found".
func (c *Client) completePrompt(path dbus.ObjectPath) (dbus.Variant, error) {
	ch := make(chan *dbus.Signal, 1)
	c.conn.Signal(ch)
	defer c.conn.RemoveSignal(ch)

	matchOpts := []dbus.MatchOption{
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(promptIface),
		dbus.WithMatchMember("Completed"),
	}
	if err := c.conn.AddMatchSignal(matchOpts...); err != nil {
		return dbus.Variant{}, fmt.Errorf("secret service: watch prompt %s: %w", path, err)
	}
	defer func() { _ = c.conn.RemoveMatchSignal(matchOpts...) }()

	prompt := c.conn.Object(busName, path)
	if err := prompt.Call(promptIface+".Prompt", 0, "").Err; err != nil {
		return dbus.Variant{}, fmt.Errorf("secret service: prompt %s: %w", path, err)
	}

	select {
	case sig := <-ch:
		if sig.Path != path || sig.Name != promptIface+".Completed" || len(sig.Body) != 2 {
			return dbus.Variant{}, fmt.Errorf("secret service: unexpected signal from prompt %s", path)
		}
		dismissed, ok := sig.Body[0].(bool)
		result, ok2 := sig.Body[1].(dbus.Variant)
		if !ok || !ok2 {
			return dbus.Variant{}, fmt.Errorf("secret service: malformed Completed signal from prompt %s", path)
		}
		if dismissed {
			return dbus.Variant{}, fmt.Errorf("secret service: prompt %s dismissed", path)
		}
		return result, nil
	case <-time.After(promptTimeout):
		_ = prompt.Call(promptIface+".Dismiss", 0)
		return dbus.Variant{}, fmt.Errorf("secret service: prompt %s timed out after %s", path, promptTimeout)
	}
}
