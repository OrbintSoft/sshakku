package keepassxc

import (
	"errors"
	"testing"
)

// These pin the parts of KeePassXC's wire format that cannot be inferred from
// this package's own code — they were measured against a real KeePassXC 2.7.12,
// and a stand-in built from the implementation instead would have agreed with
// whatever the implementation happened to assume.

// TestAcceptanceIsStatedInsideTheEncryptedMessage records where a reply says it
// succeeded. KeePassXC puts the flag inside the encrypted message and leaves it
// off the envelope entirely; only a failure is named outside. A client that
// reads the envelope instead rejects every reply that worked.
func TestAcceptanceIsStatedInsideTheEncryptedMessage(t *testing.T) {
	s := newFakeServer(t)
	s.reply = func(string, map[string]any) any {
		return map[string]any{"success": "true", "id": "db", "hash": "h"}
	}
	c := s.connect()

	assoc, err := c.Associate()
	if err != nil {
		t.Fatalf("Associate with acceptance stated only inside the message = %v, want it accepted", err)
	}
	if assoc.ID != "db" {
		t.Errorf("association id = %q, want %q", assoc.ID, "db")
	}
}

// TestARefusalInsideTheEncryptedMessageIsReported is the other half: the same
// place that says yes is what says no, and a refusal there must not be read as
// success just because the envelope named no error.
func TestARefusalInsideTheEncryptedMessageIsReported(t *testing.T) {
	s := newFakeServer(t)
	s.reply = func(string, map[string]any) any {
		return map[string]any{"success": "false"}
	}
	c := s.connect()

	if _, err := c.Associate(); err == nil {
		t.Fatal("a reply whose encrypted body refused the request must not be reported as success")
	}
}

// TestAURLThatMatchesNothingIsNotAFailure records that KeePassXC answers a
// lookup with no matches by naming an error rather than by returning an empty
// list. A key whose passphrase has never been saved is the ordinary first-use
// state, so the caller must see an empty answer, not a broken wallet.
func TestAURLThatMatchesNothingIsNotAFailure(t *testing.T) {
	s := newFakeServer(t)
	s.failWith[actionGetLogins] = envelope{Error: "No logins found", Code: errCodeNoLogins}
	c := s.connect()

	entries, err := c.GetLogins("sshakku://never-stored", Association{ID: "db", IDKey: "key"})
	if err != nil {
		t.Fatalf("GetLogins on a URL with no entry = %v, want no error", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %v, want none", entries)
	}
}

// TestAStaleAssociationIsReportedAsSuchHoweverItIsRefused covers the one
// question whose refusal has a single meaning: if KeePassXC will not confirm an
// association, the association does not hold, and the caller's answer is to ask
// for a new one. A locked database is not such an answer — nothing can be
// confirmed until it is unlocked — and is kept distinct.
func TestAStaleAssociationIsReportedAsSuchHoweverItIsRefused(t *testing.T) {
	t.Run("refused inside the message", func(t *testing.T) {
		s := newFakeServer(t)
		s.reply = func(string, map[string]any) any {
			return map[string]any{"success": "false"}
		}
		c := s.connect()
		if err := c.TestAssociate(Association{ID: "db", IDKey: "key"}); !errors.Is(err, ErrNotAssociated) {
			t.Errorf("TestAssociate = %v, want ErrNotAssociated", err)
		}
	})

	t.Run("locked database stays its own answer", func(t *testing.T) {
		s := newFakeServer(t)
		s.failWith[actionTestAssociate] = envelope{Error: "locked", Code: errCodeDatabaseNotOpened}
		c := s.connect()
		if err := c.TestAssociate(Association{ID: "db", IDKey: "key"}); !errors.Is(err, ErrDatabaseLocked) {
			t.Errorf("TestAssociate against a locked database = %v, want ErrDatabaseLocked", err)
		}
	})
}
