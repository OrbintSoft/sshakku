package keepassxc

import (
	"errors"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire"
)

// fakeKeePassXC stands in for a running KeePassXC at the protocol boundary. It
// replaces the transport and the wire format — both upstream of everything the
// backend decides — and leaves the decisions themselves alone: which entry
// matches a key name, and whether an approval may be asked for at all.
//
// The protocol it stands in for is verified in its own package, against a
// server that speaks it with real keys and real encryption.
type fakeKeePassXC struct {
	// entries maps a URL to what get-logins returns for it.
	entries map[string][]wire.Entry

	// associateCalls counts approval requests, which is what a lookup must
	// never trigger.
	associateCalls int
	// testAssociateErr, when set, stands for KeePassXC no longer honouring the
	// stored approval.
	testAssociateErr error
	// associateErr, when set, stands for the user refusing the dialog.
	associateErr error
	// getLoginsErr, when set, fails the lookup.
	getLoginsErr error
	// setLoginErr, when set, fails the store.
	setLoginErr error

	// lastSet records the arguments of the last set-login.
	lastSet struct {
		url, login, password, uuid, group string
		called                            bool
	}
	closed bool
}

func (f *fakeKeePassXC) TestAssociate(wire.Association) error { return f.testAssociateErr }

func (f *fakeKeePassXC) Associate() (wire.Association, error) {
	f.associateCalls++
	if f.associateErr != nil {
		return wire.Association{}, f.associateErr
	}
	return wire.Association{ID: "granted", IDKey: "granted-key"}, nil
}

func (f *fakeKeePassXC) GetLogins(url string, _ wire.Association) ([]wire.Entry, error) {
	if f.getLoginsErr != nil {
		return nil, f.getLoginsErr
	}
	return f.entries[url], nil
}

func (f *fakeKeePassXC) SetLogin(url, login, password, uuid, group string, _ wire.Association) error {
	if f.setLoginErr != nil {
		return f.setLoginErr
	}
	f.lastSet.url = url
	f.lastSet.login = login
	f.lastSet.password = password
	f.lastSet.uuid = uuid
	f.lastSet.group = group
	f.lastSet.called = true
	if f.entries == nil {
		f.entries = map[string][]wire.Entry{}
	}
	f.entries[url] = []wire.Entry{{Login: login, Password: password, UUID: uuid}}
	return nil
}

func (f *fakeKeePassXC) Close() error {
	f.closed = true
	return nil
}

// backendFor wires a backend to this fake and the given association store.
func (f *fakeKeePassXC) backendFor(store AssociationStore) Native {
	return Native{
		NewSession:   func() (Session, error) { return f, nil },
		Associations: store,
	}
}

// errNoKeePassXC stands for nothing listening on any candidate socket.
var errNoKeePassXC = errors.New("no KeePassXC is running")
