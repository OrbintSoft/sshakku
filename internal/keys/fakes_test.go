package keys

import (
	"context"
	"strings"
	"time"
)

// fakePrompter is a Prompter whose availability and answer are scripted.
type fakePrompter struct {
	avail bool
	pass  string
	err   error
	calls []string
}

func (p *fakePrompter) Available(context.Context) bool { return p.avail }

func (p *fakePrompter) Prompt(_ context.Context, keyname string) (string, error) {
	p.calls = append(p.calls, keyname)
	return p.pass, p.err
}

// fakeLister returns a fixed list of key paths (or an error).
type fakeLister struct {
	paths []string
	err   error
}

func (l fakeLister) Keys() ([]string, error) { return l.paths, l.err }

// fakeSecret is a scripted SecretBackend that records every Store. It also
// implements SecretSession (Unlock/Lock), recording call counts, so Loader
// batch-unlock tests can assert on them without a real Secret Service.
type fakeSecret struct {
	lookupPass   string
	lookupFound  bool
	lookupErr    error
	storeErr     error
	stored       []storeCall
	deleteErr    error
	deleted      []string
	listServices []string
	listErr      error

	unlockErr   error
	unlockCalls int
	lockErr     error
	lockCalls   int
}

type storeCall struct{ service, label, passphrase string }

func (s *fakeSecret) Lookup(context.Context, string) (string, bool, error) {
	return s.lookupPass, s.lookupFound, s.lookupErr
}

func (s *fakeSecret) Store(_ context.Context, service, label, passphrase string) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.stored = append(s.stored, storeCall{service, label, passphrase})
	return nil
}

func (s *fakeSecret) Delete(_ context.Context, service string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, service)
	return nil
}

func (s *fakeSecret) List(context.Context) ([]string, error) {
	return s.listServices, s.listErr
}

func (s *fakeSecret) Unlock(context.Context) error {
	s.unlockCalls++
	return s.unlockErr
}

func (s *fakeSecret) Lock(context.Context) error {
	s.lockCalls++
	return s.lockErr
}

// fakeKeyAdder records each add and returns scripted exit codes per call.
type fakeKeyAdder struct {
	withCodes []int // exit codes for successive AddWithAskpass calls
	err       error
	calls     []addCall
}

type addCall struct {
	keyfile    string
	passphrase string
}

func (a *fakeKeyAdder) AddWithAskpass(_ context.Context, keyfile, passphrase string) (int, error) {
	a.calls = append(a.calls, addCall{keyfile: keyfile, passphrase: passphrase})
	if a.err != nil {
		return 0, a.err
	}
	return popCode(&a.withCodes), nil
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

// fakeGiveup is an in-memory GiveupStore that scripts GivenUp and records the
// keys passed to Record and Clear.
type fakeGiveup struct {
	given     map[string]bool
	recorded  []string
	cleared   []string
	recordErr error
	clearErr  error
}

func newFakeGiveup() *fakeGiveup { return &fakeGiveup{given: map[string]bool{}} }

func (g *fakeGiveup) GivenUp(key string) bool { return g.given[key] }

func (g *fakeGiveup) Record(key string) error {
	if g.recordErr != nil {
		return g.recordErr
	}
	g.recorded = append(g.recorded, key)
	g.given[key] = true
	return nil
}

func (g *fakeGiveup) Clear(key string) error {
	if g.clearErr != nil {
		return g.clearErr
	}
	g.cleared = append(g.cleared, key)
	delete(g.given, key)
	return nil
}

// fakeKeyState is an in-memory KeyState that records every Save call.
type fakeKeyState struct {
	err   error
	saved []keyStateCall
}

type keyStateCall struct {
	key      string
	lifetime time.Duration
}

func (k *fakeKeyState) Save(key string, lifetime time.Duration) error {
	if k.err != nil {
		return k.err
	}
	k.saved = append(k.saved, keyStateCall{key, lifetime})
	return nil
}

// fakeNotifier records the user-facing notices a Loader emits.
type fakeNotifier struct{ msgs []string }

func (n *fakeNotifier) Notify(message string) { n.msgs = append(n.msgs, message) }
