package keys

import (
	"errors"
	"testing"
)

// mapSecret answers per service. fakeSecret gives one answer to every lookup,
// which cannot express the case these tests are about: something stored under
// one name and nothing under the other.
type mapSecret struct {
	items     map[string]string
	lookupErr error
	storeErr  error
	deleteErr error
	stored    []string
	deleted   []string
}

func (s *mapSecret) Lookup(service string) (string, bool, error) {
	if s.lookupErr != nil {
		return "", false, s.lookupErr
	}
	pass, ok := s.items[service]
	return pass, ok, nil
}

func (s *mapSecret) Store(service, _, passphrase string) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	if s.items == nil {
		s.items = map[string]string{}
	}
	s.items[service] = passphrase
	s.stored = append(s.stored, service)
	return nil
}

func (s *mapSecret) Delete(service string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.items, service)
	s.deleted = append(s.deleted, service)
	return nil
}

func (s *mapSecret) List() ([]string, error) { return nil, ErrListUnsupported }

func discardLog(string, string, ...any) {}

const (
	currentName = defaultServicePrefix + "-id_rsa"
	oldName     = legacyServicePrefix + "-id_rsa"
)

func TestLookupWithLegacyReadsTheCurrentNameFirst(t *testing.T) {
	s := &mapSecret{items: map[string]string{currentName: "hunter2", oldName: "stale"}}

	pass, found, err := lookupWithLegacy(s, currentName, "id_rsa", discardLog)
	if err != nil || !found || pass != "hunter2" {
		t.Fatalf("lookup = (%q,%v,%v), want (hunter2,true,nil)", pass, found, err)
	}
	// Nothing to migrate, so nothing may be written or removed: an entry the
	// current name already answers for is not touched on the way past.
	if len(s.stored) != 0 || len(s.deleted) != 0 {
		t.Errorf("stored %v, deleted %v; want neither", s.stored, s.deleted)
	}
}

func TestLookupWithLegacyMovesWhatItFindsUnderTheOldName(t *testing.T) {
	s := &mapSecret{items: map[string]string{oldName: "hunter2"}}

	pass, found, err := lookupWithLegacy(s, currentName, "id_rsa", discardLog)
	if err != nil || !found || pass != "hunter2" {
		t.Fatalf("lookup = (%q,%v,%v), want (hunter2,true,nil)", pass, found, err)
	}
	if s.items[currentName] != "hunter2" {
		t.Errorf("%s = %q, want it to hold the passphrase now", currentName, s.items[currentName])
	}
	if _, still := s.items[oldName]; still {
		t.Errorf("%s is still there; an entry left under the old name is one forget no longer recognises", oldName)
	}
}

func TestLookupWithLegacyStillAnswersWhenTheMoveFails(t *testing.T) {
	t.Run("the store fails", func(t *testing.T) {
		s := &mapSecret{items: map[string]string{oldName: "hunter2"}, storeErr: errors.New("wallet is read-only")}
		pass, found, err := lookupWithLegacy(s, currentName, "id_rsa", discardLog)
		if err != nil || !found || pass != "hunter2" {
			t.Fatalf("lookup = (%q,%v,%v), want the passphrase anyway", pass, found, err)
		}
		if _, gone := s.items[oldName]; !gone {
			t.Error("the old entry was removed even though nothing was written in its place")
		}
	})

	t.Run("the delete fails", func(t *testing.T) {
		s := &mapSecret{items: map[string]string{oldName: "hunter2"}, deleteErr: errors.New("no permission")}
		pass, found, err := lookupWithLegacy(s, currentName, "id_rsa", discardLog)
		if err != nil || !found || pass != "hunter2" {
			t.Fatalf("lookup = (%q,%v,%v), want the passphrase anyway", pass, found, err)
		}
		if s.items[currentName] != "hunter2" {
			t.Error("the passphrase was not written under the current name")
		}
	})
}

func TestLookupWithLegacyMisses(t *testing.T) {
	t.Run("neither name holds anything", func(t *testing.T) {
		s := &mapSecret{items: map[string]string{}}
		if _, found, err := lookupWithLegacy(s, currentName, "id_rsa", discardLog); found || err != nil {
			t.Fatalf("found=%v err=%v, want a plain miss", found, err)
		}
	})

	t.Run("an old entry holding only blanks is not moved", func(t *testing.T) {
		s := &mapSecret{items: map[string]string{oldName: "   "}}
		if _, found, err := lookupWithLegacy(s, currentName, "id_rsa", discardLog); found || err != nil {
			t.Fatalf("found=%v err=%v, want a miss", found, err)
		}
		if len(s.stored) != 0 {
			t.Errorf("stored %v; whitespace is not a passphrase worth carrying forward", s.stored)
		}
	})

	t.Run("a configured prefix has no old name to fall back to", func(t *testing.T) {
		s := &mapSecret{items: map[string]string{oldName: "hunter2"}}
		if _, found, _ := lookupWithLegacy(s, "mine-id_rsa", "id_rsa", discardLog); found {
			t.Error("a service sshakku did not name must not be rewritten to the old prefix")
		}
	})

	t.Run("a lookup error is passed back, not swallowed", func(t *testing.T) {
		wantErr := errors.New("wallet unreachable")
		s := &mapSecret{lookupErr: wantErr}
		if _, _, err := lookupWithLegacy(s, currentName, "id_rsa", discardLog); !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want %v", err, wantErr)
		}
	})
}

// TestOwnServicesKeepsBothNames pins that an entry an older version wrote is
// still sshakku's to forget. Dropping the old prefix here would leave those
// entries where nothing this program offers could reach them.
func TestOwnServicesKeepsBothNames(t *testing.T) {
	got := ownServices([]string{"github.com", currentName, oldName, "Another App-credentials"})
	want := []string{currentName, oldName}
	if !equalStrings(got, want) {
		t.Fatalf("ownServices = %v, want %v", got, want)
	}
}
