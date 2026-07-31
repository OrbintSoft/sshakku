package keys

import (
	"errors"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keepassxc"
)

// memoryAssociations is an association store held in memory. It stands in for
// the file on disk, which is upstream of every decision these tests make.
type memoryAssociations struct {
	stored  keepassxc.Association
	present bool
	loadErr error
	saveErr error
	saved   int
}

func (m *memoryAssociations) Load() (keepassxc.Association, bool, error) {
	return m.stored, m.present, m.loadErr
}

func (m *memoryAssociations) Save(a keepassxc.Association) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.stored = a
	m.present = true
	m.saved++
	return nil
}

// The tests below drive the backend against a stand-in KeePassXC at the
// protocol boundary. What is replaced is the transport and the wire format,
// both upstream of what the backend decides; the decisions themselves — which
// entry matches a key name, and whether an approval may be asked for — are left
// to the code under test. The protocol is verified in its own package, against
// a server that speaks it with real keys and real encryption.

func TestKeePassXCLookupMatchesTheExactKeyName(t *testing.T) {
	// KeePassXC's URL host is case-insensitive, so two key names differing only
	// in case arrive at the same URL and it returns both. The backend must pick
	// by exact name, or one key would hand back the other's passphrase.
	kp := &fakeKeePassXC{entries: map[string][]keepassxc.Entry{
		"sshakku://id_ed25519": {
			{Login: "id_ED25519", Password: "the-other-key"},
			{Login: "id_ed25519", Password: "the-right-one"},
		},
	}}
	b := kp.backendFor(&memoryAssociations{stored: keepassxc.Association{ID: "db", IDKey: "k"}, present: true})

	got, found, err := b.Lookup("id_ed25519")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !found {
		t.Fatal("the stored passphrase must be found")
	}
	if got != "the-right-one" {
		t.Errorf("passphrase = %q, want the entry whose name matches exactly", got)
	}
}

func TestKeePassXCLookupOfAnAbsentKeyIsAMiss(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := kp.backendFor(&memoryAssociations{stored: keepassxc.Association{ID: "db", IDKey: "k"}, present: true})

	_, found, err := b.Lookup("id_absent")
	if err != nil {
		t.Fatalf("a miss must not be an error: %v", err)
	}
	if found {
		t.Error("nothing was stored, so nothing must be found")
	}
}

// TestKeePassXCLookupNeverAsksForApproval is the assertion that keeps F5 and F6
// silent. Associating raises a dialog in KeePassXC; a lookup happens with
// nobody watching, so it must report that it is not associated rather than
// trying to become so.
func TestKeePassXCLookupNeverAsksForApproval(t *testing.T) {
	kp := &fakeKeePassXC{}
	assoc := &memoryAssociations{} // nothing stored: never approved
	b := kp.backendFor(assoc)

	_, _, err := b.Lookup("id_ed25519")
	if !errors.Is(err, keepassxc.ErrNotAssociated) {
		t.Fatalf("err = %v, want ErrNotAssociated", err)
	}
	if kp.associateCalls != 0 {
		t.Fatal("a lookup must never associate — that raises a dialog nobody is there to answer")
	}
	if assoc.saved != 0 {
		t.Error("a lookup must not write an association")
	}
}

// TestKeePassXCStoreAssociatesOnFirstUse is the counterpart: storing happens
// right after the user typed the passphrase, so they are present to approve.
func TestKeePassXCStoreAssociatesOnFirstUse(t *testing.T) {
	kp := &fakeKeePassXC{}
	assoc := &memoryAssociations{}
	b := kp.backendFor(assoc)

	if err := b.Store("id_ed25519", "id_ed25519", "secret"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if kp.associateCalls != 1 {
		t.Fatalf("associate was called %d times, want once", kp.associateCalls)
	}
	if assoc.saved != 1 {
		t.Fatal("the approval must be persisted, or the user is asked again every run")
	}
	if kp.lastSet.url != "sshakku://id_ed25519" || kp.lastSet.password != "secret" {
		t.Errorf("set-login = %+v, want the passphrase under the key's own URL", kp.lastSet)
	}
	if kp.lastSet.login != "id_ed25519" {
		t.Errorf("username = %q, want the exact key name — it is what a lookup matches on", kp.lastSet.login)
	}
}

func TestKeePassXCStoreReusesAnExistingAssociation(t *testing.T) {
	kp := &fakeKeePassXC{}
	assoc := &memoryAssociations{stored: keepassxc.Association{ID: "db", IDKey: "k"}, present: true}
	b := kp.backendFor(assoc)

	if err := b.Store("id_ed25519", "", "secret"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if kp.associateCalls != 0 {
		t.Error("an approval already granted must not be asked for again")
	}
}

// TestKeePassXCStoreReplacesInPlace proves a re-stored passphrase updates the
// entry rather than leaving a second copy of the secret in the database.
func TestKeePassXCStoreReplacesInPlace(t *testing.T) {
	kp := &fakeKeePassXC{entries: map[string][]keepassxc.Entry{
		"sshakku://id_ed25519": {{Login: "id_ed25519", Password: "old", UUID: "u-1"}},
	}}
	b := kp.backendFor(&memoryAssociations{stored: keepassxc.Association{ID: "db", IDKey: "k"}, present: true})

	if err := b.Store("id_ed25519", "", "new"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if got := kp.lastSet.uuid; got != "u-1" {
		t.Errorf("set-login carried uuid %q, want the existing entry's — otherwise it creates a duplicate", got)
	}
}

func TestKeePassXCDeleteSaysItCannot(t *testing.T) {
	b := KeePassXCBackend{}
	err := b.Delete("id_ed25519")
	if !errors.Is(err, ErrDeleteUnsupported) {
		t.Fatalf("err = %v, want ErrDeleteUnsupported", err)
	}
	// The user has to do it themselves, so the message has to say where.
	if !strings.Contains(err.Error(), "sshakku://id_ed25519") {
		t.Errorf("err = %v, want it to name the entry to remove", err)
	}
}

func TestKeePassXCListIsUnsupported(t *testing.T) {
	if _, err := (KeePassXCBackend{}).List(); !errors.Is(err, ErrListUnsupported) {
		t.Fatalf("err = %v, want ErrListUnsupported", err)
	}
}

func TestKeePassXCWithNoAssociationStoreIsAnError(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := KeePassXCBackend{NewSession: func() (KeePassXCSession, error) { return kp, nil }}
	if _, _, err := b.Lookup("id_ed25519"); err == nil {
		t.Fatal("a backend with nowhere to keep its approval must say so")
	}
}

func TestKeePassXCReportsAnUnreachableKeePassXC(t *testing.T) {
	b := KeePassXCBackend{
		NewSession:   func() (KeePassXCSession, error) { return nil, errNoKeePassXC },
		Associations: &memoryAssociations{},
	}
	if _, _, err := b.Lookup("id_ed25519"); err == nil {
		t.Fatal("an unreachable KeePassXC must be reported")
	}
	if err := b.Store("id_ed25519", "", "p"); err == nil {
		t.Fatal("an unreachable KeePassXC must be reported on store too")
	}
}

func TestKeePassXCReportsAnUnreadableAssociation(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := kp.backendFor(&memoryAssociations{loadErr: errors.New("unreadable")})
	if _, _, err := b.Lookup("id_ed25519"); err == nil {
		t.Fatal("an association that cannot be read must be reported, not treated as absent")
	}
}

func TestKeePassXCReportsAnUnwritableAssociation(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := kp.backendFor(&memoryAssociations{saveErr: errors.New("read-only")})
	if err := b.Store("id_ed25519", "", "p"); err == nil {
		t.Fatal("an approval that could not be saved must be reported — the next run would ask again")
	}
}
