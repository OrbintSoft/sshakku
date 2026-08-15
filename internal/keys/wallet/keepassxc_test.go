package wallet

import (
	"errors"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryAssociations is an association store held in memory. It stands in for
// the file on disk, which is upstream of every decision these tests make.
type memoryAssociations struct {
	stored  wire.Association
	present bool
	loadErr error
	saveErr error
	saved   int
}

func (m *memoryAssociations) Load() (wire.Association, bool, error) {
	return m.stored, m.present, m.loadErr
}

func (m *memoryAssociations) Save(a wire.Association) error {
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
	kp := &fakeKeePassXC{entries: map[string][]wire.Entry{
		"sshakku://id_ed25519": {
			{Login: "id_ED25519", Password: "the-other-key"},
			{Login: "id_ed25519", Password: "the-right-one"},
		},
	}}
	b := kp.backendFor(&memoryAssociations{stored: wire.Association{ID: "db", IDKey: "k"}, present: true})

	got, found, err := b.Lookup(t.Context(), "id_ed25519")
	require.NoError(t, err, "a stored passphrase must come back")
	require.True(t, found, "the entry is in the database, so it must be reported found")
	assert.Equal(t, "the-right-one", got,
		"the entry whose name matches exactly is the right one: any other hands this key another key's passphrase")
}

func TestKeePassXCLookupOfAnAbsentKeyIsAMiss(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := kp.backendFor(&memoryAssociations{stored: wire.Association{ID: "db", IDKey: "k"}, present: true})

	_, found, err := b.Lookup(t.Context(), "id_absent")
	require.NoError(t, err, "a passphrase that was never stored is not an error")
	assert.False(t, found, "and nothing may be reported found")
}

// TestKeePassXCLookupNeverAsksForApproval is the assertion that keeps F5 and F6
// silent. Associating raises a dialog in KeePassXC; a lookup happens with
// nobody watching, so it must report that it is not associated rather than
// trying to become so.
func TestKeePassXCLookupNeverAsksForApproval(t *testing.T) {
	kp := &fakeKeePassXC{}
	assoc := &memoryAssociations{} // nothing stored: never approved
	b := kp.backendFor(assoc)

	_, _, err := b.Lookup(t.Context(), "id_ed25519")
	assert.ErrorIs(t, err, wire.ErrNotAssociated,
		"a database SSHakku was never approved for must say so, rather than fail as an empty one")
	assert.Zero(t, kp.associateCalls,
		"associating raises a dialog in KeePassXC, and a lookup happens with nobody there to answer it")
	assert.Zero(t, assoc.saved, "so there is no approval to write down either")
}

// TestKeePassXCStoreAssociatesOnFirstUse is the counterpart: storing happens
// right after the user typed the passphrase, so they are present to approve.
func TestKeePassXCStoreAssociatesOnFirstUse(t *testing.T) {
	kp := &fakeKeePassXC{}
	assoc := &memoryAssociations{}
	b := kp.backendFor(assoc)

	// The label is deliberately not the key's name: they arrive as two separate
	// arguments, and a case where they read alike cannot tell which one was sent.
	require.NoError(t, b.Store(t.Context(), "id_ed25519", "SSH Passphrase for id_ed25519", "secret"),
		"saving a passphrase must succeed")
	assert.Equal(t, 1, kp.associateCalls,
		"storing happens right after the user typed the passphrase, so they are there to approve — once")
	assert.Equal(t, 1, assoc.saved, "and the approval must be written down, or they are asked again every run")
	assert.Equal(t, "sshakku://id_ed25519", kp.lastSet.url, "the entry must live under the key's own URL")
	assert.Equal(t, "secret", kp.lastSet.password, "and hold the passphrase that was typed")
	assert.Equal(t, "id_ed25519", kp.lastSet.login,
		"under the exact key name, which is what a lookup matches on")
}

func TestKeePassXCStoreReusesAnExistingAssociation(t *testing.T) {
	kp := &fakeKeePassXC{}
	assoc := &memoryAssociations{stored: wire.Association{ID: "db", IDKey: "k"}, present: true}
	b := kp.backendFor(assoc)

	require.NoError(t, b.Store(t.Context(), "id_ed25519", "", "secret"), "saving a passphrase must succeed")
	assert.Zero(t, kp.associateCalls, "an approval the user already granted must not be asked for again")
}

// TestKeePassXCStoreReplacesInPlace proves a re-stored passphrase updates the
// entry rather than leaving a second copy of the secret in the database.
func TestKeePassXCStoreReplacesInPlace(t *testing.T) {
	kp := &fakeKeePassXC{entries: map[string][]wire.Entry{
		"sshakku://id_ed25519": {{Login: "id_ed25519", Password: "old", UUID: "u-1"}},
	}}
	b := kp.backendFor(&memoryAssociations{stored: wire.Association{ID: "db", IDKey: "k"}, present: true})

	require.NoError(t, b.Store(t.Context(), "id_ed25519", "", "new"), "replacing a passphrase must succeed")
	assert.Equal(t, "u-1", kp.lastSet.uuid,
		"the write must carry the existing entry's identity, or it leaves a second copy of the secret in the database")
}

func TestKeePassXCDeleteSaysItCannot(t *testing.T) {
	b := KeePassXC{}
	err := b.Delete(t.Context(), "id_ed25519")
	require.ErrorIs(t, err, ErrDeleteUnsupported,
		"KeePassXC's local protocol has no verb for removing an entry, and that must be said, not faked")
	// The user has to do it themselves, so the message has to say where.
	assert.Contains(t, err.Error(), "sshakku://id_ed25519",
		"and it must name the entry to remove, since the user is the one who has to remove it")
}

func TestKeePassXCListIsUnsupported(t *testing.T) {
	_, err := (KeePassXC{}).List(t.Context())
	assert.ErrorIs(t, err, ErrListUnsupported,
		"KeePassXC's local protocol cannot enumerate a database, and that must be said, not answered as empty")
}

func TestKeePassXCWithNoAssociationStoreIsAnError(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := KeePassXC{NewSession: func() (KeePassXCSession, error) { return kp, nil }}
	_, _, err := b.Lookup(t.Context(), "id_ed25519")
	require.Error(t, err, "a route with nowhere to keep its approval cannot work, and must say so")
	assert.NotErrorIs(t, err, wire.ErrNotAssociated,
		"and it must not be reported as an approval the user has yet to give: approving it again would change nothing, "+
			"and the dialog would come back every run")
}

func TestKeePassXCReportsAnUnreachableKeePassXC(t *testing.T) {
	b := KeePassXC{
		NewSession:   func() (KeePassXCSession, error) { return nil, errNoKeePassXC },
		Associations: &memoryAssociations{},
	}
	_, _, err := b.Lookup(t.Context(), "id_ed25519")
	assert.Error(t, err, "a KeePassXC that is not running cannot answer, and must not be read as an empty database")
	assert.Error(t, b.Store(t.Context(), "id_ed25519", "", "p"),
		"nor may a passphrase be reported as saved into a database nothing reached")
}

func TestKeePassXCReportsAnUnreadableAssociation(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := kp.backendFor(&memoryAssociations{loadErr: errors.New("unreadable")})
	_, _, err := b.Lookup(t.Context(), "id_ed25519")
	assert.Error(t, err, "an approval that could not be read must be reported, not treated as never granted")
}

func TestKeePassXCReportsAnUnwritableAssociation(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := kp.backendFor(&memoryAssociations{saveErr: errors.New("read-only")})
	assert.Error(t, b.Store(t.Context(), "id_ed25519", "", "p"),
		"an approval that could not be written down must be reported: the next run would raise the dialog again")
}
