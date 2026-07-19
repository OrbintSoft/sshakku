package keys

import (
	"errors"
	"sort"
	"testing"
)

// fakeKeychainClient is an in-memory KeychainClient for KeychainBackend's
// unit tests, keyed by service (Account is recorded but not used to key
// storage — every test uses a single fixed account).
type fakeKeychainClient struct {
	items                                             map[string]string
	addErr, updateErr, findErr, deleteErr, listErr    error
	addCalls, updateCalls                             int
	lastAccount, lastService, lastLabel, lastPassword string
}

func (f *fakeKeychainClient) Add(account, service, label, passphrase string) error {
	f.addCalls++
	f.lastAccount, f.lastService, f.lastLabel, f.lastPassword = account, service, label, passphrase
	if f.addErr != nil {
		return f.addErr
	}
	if f.items == nil {
		f.items = map[string]string{}
	}
	f.items[service] = passphrase
	return nil
}

func (f *fakeKeychainClient) Update(account, service, passphrase string) error {
	f.updateCalls++
	f.lastAccount, f.lastService, f.lastPassword = account, service, passphrase
	if f.updateErr != nil {
		return f.updateErr
	}
	f.items[service] = passphrase
	return nil
}

func (f *fakeKeychainClient) Find(account, service string) (string, bool, error) {
	if f.findErr != nil {
		return "", false, f.findErr
	}
	p, ok := f.items[service]
	return p, ok, nil
}

func (f *fakeKeychainClient) Delete(account, service string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.items, service)
	return nil
}

func (f *fakeKeychainClient) List(account string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	services := make([]string, 0, len(f.items))
	for s := range f.items {
		services = append(services, s)
	}
	sort.Strings(services)
	return services, nil
}

var _ KeychainClient = (*fakeKeychainClient)(nil)

func TestKeychainBackendLookup(t *testing.T) {
	t.Run("hit", func(t *testing.T) {
		c := &fakeKeychainClient{items: map[string]string{"svc": "hunter2"}}
		b := &KeychainBackend{Client: c, Account: "alice"}
		got, found, err := b.Lookup("svc")
		if err != nil || !found || got != "hunter2" {
			t.Fatalf("Lookup = %q, %v, %v; want hunter2, true, nil", got, found, err)
		}
	})
	t.Run("miss", func(t *testing.T) {
		c := &fakeKeychainClient{}
		b := &KeychainBackend{Client: c, Account: "alice"}
		_, found, err := b.Lookup("svc")
		if err != nil || found {
			t.Fatalf("Lookup = found=%v, err=%v; want found=false, nil", found, err)
		}
	})
	t.Run("client error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{findErr: wantErr}
		b := &KeychainBackend{Client: c, Account: "alice"}
		_, _, err := b.Lookup("svc")
		if !errors.Is(err, wantErr) {
			t.Fatalf("Lookup err = %v, want %v", err, wantErr)
		}
	})
}

func TestKeychainBackendStore(t *testing.T) {
	t.Run("new item calls Add, not Update", func(t *testing.T) {
		c := &fakeKeychainClient{}
		b := &KeychainBackend{Client: c, Account: "alice"}
		if err := b.Store("svc", "label", "hunter2"); err != nil {
			t.Fatalf("Store: %v", err)
		}
		if c.addCalls != 1 || c.updateCalls != 0 {
			t.Fatalf("addCalls=%d updateCalls=%d, want 1, 0", c.addCalls, c.updateCalls)
		}
		if c.lastAccount != "alice" || c.lastService != "svc" || c.lastLabel != "label" || c.lastPassword != "hunter2" {
			t.Fatalf("Add called with (%q, %q, %q, %q)", c.lastAccount, c.lastService, c.lastLabel, c.lastPassword)
		}
	})
	t.Run("existing item calls Update, not Add", func(t *testing.T) {
		c := &fakeKeychainClient{items: map[string]string{"svc": "old"}}
		b := &KeychainBackend{Client: c, Account: "alice"}
		if err := b.Store("svc", "label", "new"); err != nil {
			t.Fatalf("Store: %v", err)
		}
		if c.addCalls != 0 || c.updateCalls != 1 {
			t.Fatalf("addCalls=%d updateCalls=%d, want 0, 1", c.addCalls, c.updateCalls)
		}
		if c.items["svc"] != "new" {
			t.Fatalf("items[svc] = %q, want new", c.items["svc"])
		}
	})
	t.Run("find error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{findErr: wantErr}
		b := &KeychainBackend{Client: c, Account: "alice"}
		if err := b.Store("svc", "label", "x"); !errors.Is(err, wantErr) {
			t.Fatalf("Store err = %v, want %v", err, wantErr)
		}
	})
	t.Run("add error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{addErr: wantErr}
		b := &KeychainBackend{Client: c, Account: "alice"}
		if err := b.Store("svc", "label", "x"); !errors.Is(err, wantErr) {
			t.Fatalf("Store err = %v, want %v", err, wantErr)
		}
	})
	t.Run("update error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{items: map[string]string{"svc": "old"}, updateErr: wantErr}
		b := &KeychainBackend{Client: c, Account: "alice"}
		if err := b.Store("svc", "label", "x"); !errors.Is(err, wantErr) {
			t.Fatalf("Store err = %v, want %v", err, wantErr)
		}
	})
}

func TestKeychainBackendDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c := &fakeKeychainClient{items: map[string]string{"svc": "x"}}
		b := &KeychainBackend{Client: c, Account: "alice"}
		if err := b.Delete("svc"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, ok := c.items["svc"]; ok {
			t.Fatal("item should be gone")
		}
	})
	t.Run("missing entry is not an error", func(t *testing.T) {
		c := &fakeKeychainClient{}
		b := &KeychainBackend{Client: c, Account: "alice"}
		if err := b.Delete("svc"); err != nil {
			t.Fatalf("Delete of a missing entry should succeed, got %v", err)
		}
	})
	t.Run("client error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{deleteErr: wantErr}
		b := &KeychainBackend{Client: c, Account: "alice"}
		if err := b.Delete("svc"); !errors.Is(err, wantErr) {
			t.Fatalf("Delete err = %v, want %v", err, wantErr)
		}
	})
}

func TestKeychainBackendList(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c := &fakeKeychainClient{items: map[string]string{"b": "x", "a": "y"}}
		b := &KeychainBackend{Client: c, Account: "alice"}
		got, err := b.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		want := []string{"a", "b"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("List = %v, want %v", got, want)
		}
	})
	t.Run("client error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{listErr: wantErr}
		b := &KeychainBackend{Client: c, Account: "alice"}
		if _, err := b.List(); !errors.Is(err, wantErr) {
			t.Fatalf("List err = %v, want %v", err, wantErr)
		}
	})
}
