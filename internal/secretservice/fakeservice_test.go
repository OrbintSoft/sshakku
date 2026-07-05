package secretservice

import (
	"fmt"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
)

// propsIface is the standard D-Bus interface GetProperty calls land on;
// fakeCollection and fakeItem answer it alongside their own interface.
const propsIface = "org.freedesktop.DBus.Properties"

// fakeService is a minimal in-memory Secret Service, exported over a private
// D-Bus session bus, exercising Client against the real wire protocol rather
// than a mocked Go interface. behavior controls how CreateCollection, Unlock,
// Lock and CreateItem complete:
//
//   - ""        — reply immediately, no interactive prompt.
//   - "ok"      — return a prompt whose Prompt() emits Completed(false, …).
//   - "dismiss" — return a prompt whose Prompt() emits Completed(true, …).
//   - "hang"    — return a prompt whose Prompt() never completes, exercising
//     the client's timeout path.
type fakeService struct {
	conn     *dbus.Conn
	behavior string

	mu          sync.Mutex
	aliases     map[string]dbus.ObjectPath
	collections map[dbus.ObjectPath]*fakeCollection
	nextID      int
	lastPrompt  *fakePrompt
}

// startFakeSecretService exports svc as org.freedesktop.secrets on conn and
// claims the well-known bus name, so a Client connecting to the same bus
// finds it exactly as it would find ksecretd or GNOME Keyring.
func startFakeSecretService(t *testing.T, conn *dbus.Conn, behavior string) *fakeService {
	t.Helper()

	svc := &fakeService{
		conn:        conn,
		behavior:    behavior,
		aliases:     map[string]dbus.ObjectPath{},
		collections: map[dbus.ObjectPath]*fakeCollection{},
	}
	if err := conn.Export(svc, rootPath, serviceIface); err != nil {
		t.Fatalf("export fake service: %v", err)
	}
	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		t.Fatalf("request name %s: %v", busName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request name %s: reply = %v, want PrimaryOwner", busName, reply)
	}
	return svc
}

// getBehavior reads behavior under the lock, since tests may change it after
// setup (e.g. to make only a later call dismiss/hang while earlier calls in
// the same test complete cleanly).
func (s *fakeService) getBehavior() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.behavior
}

func (s *fakeService) nextPath(prefix string) dbus.ObjectPath {
	s.nextID++
	return dbus.ObjectPath(fmt.Sprintf("%s/%d", prefix, s.nextID))
}

// newPrompt exports a fakePrompt that, once Prompt() is invoked, resolves
// according to s.behavior; resultFn computes the Completed signal's payload
// for the "ok" case (e.g. the newly created collection or item path).
func (s *fakeService) newPrompt(resultFn func() dbus.Variant) dbus.ObjectPath {
	path := s.nextPath("/org/freedesktop/secrets/prompt")
	p := &fakePrompt{conn: s.conn, path: path, behavior: s.getBehavior(), resultFn: resultFn}
	if err := s.conn.Export(p, path, promptIface); err != nil {
		panic(fmt.Sprintf("export fake prompt: %v", err))
	}
	s.mu.Lock()
	s.lastPrompt = p
	s.mu.Unlock()
	return path
}

// OpenSession ignores algorithm/input — the fake only ever serves "plain"
// clients — and hands back an arbitrary session object path.
func (s *fakeService) OpenSession(string, dbus.Variant) (dbus.Variant, dbus.ObjectPath, *dbus.Error) {
	return dbus.MakeVariant(""), s.nextPath("/org/freedesktop/secrets/session"), nil
}

func (s *fakeService) ReadAlias(name string) (dbus.ObjectPath, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.aliases[name]; ok {
		return p, nil
	}
	return noPrompt, nil
}

func (s *fakeService) CreateCollection(props map[string]dbus.Variant, alias string) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	label, _ := props[collectionLabelProp].Value().(string)

	create := func() dbus.ObjectPath {
		path := s.nextPath("/org/freedesktop/secrets/collection")
		col := &fakeCollection{svc: s, path: path, label: label, items: map[dbus.ObjectPath]*fakeItem{}}

		s.mu.Lock()
		s.collections[path] = col
		if alias != "" {
			s.aliases[alias] = path
		}
		s.mu.Unlock()

		if err := s.conn.Export(col, path, collectionIface); err != nil {
			panic(fmt.Sprintf("export fake collection: %v", err))
		}
		if err := s.conn.Export(col, path, propsIface); err != nil {
			panic(fmt.Sprintf("export fake collection properties: %v", err))
		}
		return path
	}

	if s.getBehavior() == "" {
		return create(), noPrompt, nil
	}
	return noPrompt, s.newPrompt(func() dbus.Variant { return dbus.MakeVariant(create()) }), nil
}

func (s *fakeService) Unlock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	return s.unlockOrLock(objects)
}

func (s *fakeService) Lock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	return s.unlockOrLock(objects)
}

func (s *fakeService) unlockOrLock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	if s.getBehavior() == "" {
		return objects, noPrompt, nil
	}
	return nil, s.newPrompt(func() dbus.Variant { return dbus.MakeVariant(objects) }), nil
}

// fakeCollection is a Secret Service collection: a set of attribute-tagged
// items. Its behaviour for CreateItem mirrors fakeService's for
// CreateCollection/Unlock/Lock (immediate vs. prompt-mediated completion).
type fakeCollection struct {
	svc   *fakeService
	path  dbus.ObjectPath
	label string

	mu    sync.Mutex
	items map[dbus.ObjectPath]*fakeItem
}

// Get answers org.freedesktop.DBus.Properties.Get for the Items property, the
// only one Client reads from a collection.
func (c *fakeCollection) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	if iface == collectionIface && prop == "Items" {
		c.mu.Lock()
		defer c.mu.Unlock()
		items := make([]dbus.ObjectPath, 0, len(c.items))
		for path := range c.items {
			items = append(items, path)
		}
		return dbus.MakeVariant(items), nil
	}
	return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("unknown property %s.%s", iface, prop))
}

func (c *fakeCollection) SearchItems(attrs map[string]string) ([]dbus.ObjectPath, *dbus.Error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var matches []dbus.ObjectPath
	for path, it := range c.items {
		if attrsMatch(it.attrs, attrs) {
			matches = append(matches, path)
		}
	}
	return matches, nil
}

func (c *fakeCollection) CreateItem(props map[string]dbus.Variant, secret Secret, replace bool) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	label, _ := props[itemLabelProp].Value().(string)
	attrs, _ := props[itemAttributesProp].Value().(map[string]string)

	create := func() dbus.ObjectPath {
		c.mu.Lock()
		defer c.mu.Unlock()

		if replace {
			for path, it := range c.items {
				if attrsMatch(it.attrs, attrs) {
					it.label = label
					it.secret = append([]byte(nil), secret.Value...)
					return path
				}
			}
		}

		path := c.svc.nextPath(string(c.path) + "/item")
		item := &fakeItem{path: path, label: label, attrs: attrs, secret: append([]byte(nil), secret.Value...), col: c}
		c.items[path] = item
		if err := c.svc.conn.Export(item, path, itemIface); err != nil {
			panic(fmt.Sprintf("export fake item: %v", err))
		}
		if err := c.svc.conn.Export(item, path, propsIface); err != nil {
			panic(fmt.Sprintf("export fake item properties: %v", err))
		}
		return path
	}

	if c.svc.getBehavior() == "" {
		return create(), noPrompt, nil
	}
	return noPrompt, c.svc.newPrompt(func() dbus.Variant { return dbus.MakeVariant(create()) }), nil
}

// fakeItem is a single stored secret with its search attributes.
type fakeItem struct {
	path   dbus.ObjectPath
	label  string
	attrs  map[string]string
	secret []byte
	col    *fakeCollection
}

func (it *fakeItem) GetSecret(session dbus.ObjectPath) (Secret, *dbus.Error) {
	return Secret{Session: session, Value: append([]byte(nil), it.secret...), ContentType: "text/plain"}, nil
}

// Get answers org.freedesktop.DBus.Properties.Get for the Attributes
// property, the only one Client reads from an item.
func (it *fakeItem) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	if iface == itemIface && prop == "Attributes" {
		return dbus.MakeVariant(it.attrs), nil
	}
	return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("unknown property %s.%s", iface, prop))
}

// Delete removes the item from its collection, following the same
// immediate-vs-prompt-mediated completion rule as CreateItem: a dismissed or
// hung prompt leaves the item in place, since deletion was never confirmed.
func (it *fakeItem) Delete() (dbus.ObjectPath, *dbus.Error) {
	remove := func() dbus.Variant {
		it.col.mu.Lock()
		delete(it.col.items, it.path)
		it.col.mu.Unlock()
		return dbus.MakeVariant("")
	}
	if it.col.svc.getBehavior() == "" {
		remove()
		return noPrompt, nil
	}
	return it.col.svc.newPrompt(remove), nil
}

// fakePrompt drives the async Prompt/Completed dance the client must
// complete when Unlock/Lock/CreateCollection/CreateItem can't finish
// synchronously.
type fakePrompt struct {
	conn     *dbus.Conn
	path     dbus.ObjectPath
	behavior string
	resultFn func() dbus.Variant

	mu             sync.Mutex
	dismissedCalls int
}

func (p *fakePrompt) Prompt(string) *dbus.Error {
	switch p.behavior {
	case "dismiss":
		_ = p.conn.Emit(p.path, promptIface+".Completed", true, dbus.MakeVariant(""))
	case "hang":
		// Never completes: the client is expected to time out and Dismiss.
	default: // "ok"
		_ = p.conn.Emit(p.path, promptIface+".Completed", false, p.resultFn())
	}
	return nil
}

func (p *fakePrompt) Dismiss() *dbus.Error {
	p.mu.Lock()
	p.dismissedCalls++
	p.mu.Unlock()
	return nil
}

func attrsMatch(have, want map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}
