package keys

import (
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

// fakeRunner answers Run from a per-binary handler table, so a test can stub
// ssh-keygen, ssh-add, secret-tool, etc. independently and inspect the calls.
type fakeRunner struct {
	handlers map[string]func(Cmd) (Result, error)
	calls    []Cmd
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{handlers: make(map[string]func(Cmd) (Result, error))}
}

// on registers a handler for a command name.
func (f *fakeRunner) on(name string, h func(Cmd) (Result, error)) *fakeRunner {
	f.handlers[name] = h
	return f
}

func (f *fakeRunner) Run(c Cmd) (Result, error) {
	f.calls = append(f.calls, c)
	if h, ok := f.handlers[c.Name]; ok {
		return h(c)
	}
	return Result{}, fmt.Errorf("unexpected command %q", c.Name)
}

// stdout builds a handler that returns out on stdout with the given exit code.
func stdout(out string, code int) func(Cmd) (Result, error) {
	return func(Cmd) (Result, error) {
		return Result{Stdout: []byte(out), Code: code}, nil
	}
}

// fakePrompter is a Prompter whose availability and answer are scripted.
type fakePrompter struct {
	avail bool
	pass  string
	err   error
	calls []string
}

func (p *fakePrompter) Available() bool { return p.avail }

func (p *fakePrompter) Prompt(keyname string) (string, error) {
	p.calls = append(p.calls, keyname)
	return p.pass, p.err
}

// fakeLister returns a fixed list of key paths (or an error).
type fakeLister struct {
	paths []string
	err   error
}

func (l fakeLister) Keys() ([]string, error) { return l.paths, l.err }

// fakeSecret is a scripted SecretBackend that records every Store.
type fakeSecret struct {
	lookupPass  string
	lookupFound bool
	lookupErr   error
	storeErr    error
	stored      []storeCall
}

type storeCall struct{ service, label, passphrase string }

func (s *fakeSecret) Lookup(string) (string, bool, error) {
	return s.lookupPass, s.lookupFound, s.lookupErr
}

func (s *fakeSecret) Store(service, label, passphrase string) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.stored = append(s.stored, storeCall{service, label, passphrase})
	return nil
}

// fakeKeyAdder records each add and returns scripted exit codes per call.
type fakeKeyAdder struct {
	withCodes []int // exit codes for successive AddWithAskpass calls
	intCodes  []int // exit codes for successive AddInteractive calls
	err       error
	calls     []addCall
}

type addCall struct {
	keyfile     string
	passphrase  string
	interactive bool
}

func (a *fakeKeyAdder) AddWithAskpass(keyfile, passphrase string) (int, error) {
	a.calls = append(a.calls, addCall{keyfile: keyfile, passphrase: passphrase})
	if a.err != nil {
		return 0, a.err
	}
	return popCode(&a.withCodes), nil
}

func (a *fakeKeyAdder) AddInteractive(keyfile string) (int, error) {
	a.calls = append(a.calls, addCall{keyfile: keyfile, interactive: true})
	if a.err != nil {
		return 0, a.err
	}
	return popCode(&a.intCodes), nil
}

// popCode returns and removes the first code, defaulting to 0 when exhausted.
func popCode(codes *[]int) int {
	if len(*codes) == 0 {
		return 0
	}
	c := (*codes)[0]
	*codes = (*codes)[1:]
	return c
}

// fakeLogger records the level-tagged lines a Loader emits.
type fakeLogger struct{ lines []string }

func (f *fakeLogger) Log(level, message string) error {
	f.lines = append(f.lines, level+" "+message)
	return nil
}

func (f *fakeLogger) contains(sub string) bool {
	for _, l := range f.lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// fails builds a handler that reports a failure to start the process.
func fails(err error) func(Cmd) (Result, error) {
	return func(Cmd) (Result, error) { return Result{}, err }
}

// fakeGiveup is an in-memory GiveupStore that scripts GivenUp and records the
// keys passed to Record and Clear.
type fakeGiveup struct {
	given    map[string]bool
	recorded []string
	cleared  []string
}

func newFakeGiveup() *fakeGiveup { return &fakeGiveup{given: map[string]bool{}} }

func (g *fakeGiveup) GivenUp(key string) bool { return g.given[key] }

func (g *fakeGiveup) Record(key string) error {
	g.recorded = append(g.recorded, key)
	g.given[key] = true
	return nil
}

func (g *fakeGiveup) Clear(key string) error {
	g.cleared = append(g.cleared, key)
	delete(g.given, key)
	return nil
}

// fakeNotifier records the user-facing notices a Loader emits.
type fakeNotifier struct{ msgs []string }

func (n *fakeNotifier) Notify(message string) { n.msgs = append(n.msgs, message) }

// fakeSecretServiceClient scripts SecretServiceClient for SecretServiceBackend
// tests, recording the objects passed to Unlock/Lock so tests can assert the
// unlock/lock bracket around a Lookup/Store.
type fakeSecretServiceClient struct {
	collection      dbus.ObjectPath
	collectionErr   error
	collectionCalls int

	unlockErr error
	unlocked  []dbus.ObjectPath

	lockErr error
	locked  []dbus.ObjectPath

	items         []dbus.ObjectPath
	searchErr     error
	searchedAttrs map[string]string

	secretsByItem map[dbus.ObjectPath]string
	secretErr     error

	createErr    error
	createdItems []ssCreateCall
}

type ssCreateCall struct {
	collection dbus.ObjectPath
	label      string
	attrs      map[string]string
	passphrase string
	replace    bool
}

func (f *fakeSecretServiceClient) Collection(string, string) (dbus.ObjectPath, error) {
	f.collectionCalls++
	if f.collectionErr != nil {
		return "", f.collectionErr
	}
	return f.collection, nil
}

func (f *fakeSecretServiceClient) Unlock(objects ...dbus.ObjectPath) error {
	f.unlocked = append(f.unlocked, objects...)
	return f.unlockErr
}

func (f *fakeSecretServiceClient) Lock(objects ...dbus.ObjectPath) error {
	f.locked = append(f.locked, objects...)
	return f.lockErr
}

func (f *fakeSecretServiceClient) SearchItems(_ dbus.ObjectPath, attrs map[string]string) ([]dbus.ObjectPath, error) {
	f.searchedAttrs = attrs
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.items, nil
}

func (f *fakeSecretServiceClient) GetSecret(item dbus.ObjectPath) (string, error) {
	if f.secretErr != nil {
		return "", f.secretErr
	}
	return f.secretsByItem[item], nil
}

func (f *fakeSecretServiceClient) CreateItem(collection dbus.ObjectPath, label string, attrs map[string]string, passphrase string, replace bool) error {
	f.createdItems = append(f.createdItems, ssCreateCall{collection, label, attrs, passphrase, replace})
	return f.createErr
}
