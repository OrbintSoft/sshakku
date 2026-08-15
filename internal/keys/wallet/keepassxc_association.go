package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire"
)

// FileAssociationStore keeps the KeePassXC association in a file under the
// user's state directory.
//
// What it holds is an identifier and a public key — enough for KeePassXC to
// recognise this client, not enough to read anything out of the database. It is
// still written 0600: anyone who could read it could present themselves to
// KeePassXC as SSHakku, and the answer to "who may talk to the wallet" should
// not be "whoever can read a file".
type FileAssociationStore struct {
	// Path is the file the association lives in.
	Path string
}

// storedAssociation is the on-disk form. It is versioned so a later format can
// be told apart from this one instead of being misread as it.
type storedAssociation struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	IDKey   string `json:"idKey"`
}

// associationVersion is the current on-disk format.
const associationVersion = 1

// Load returns the stored association. A missing file is reported as "none
// yet", not as an error: it is the state every user starts in.
func (s FileAssociationStore) Load() (wire.Association, bool, error) {
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return wire.Association{}, false, nil
	}
	if err != nil {
		return wire.Association{}, false, fmt.Errorf("reading the KeePassXC association: %w", err)
	}
	var stored storedAssociation
	if err := json.Unmarshal(raw, &stored); err != nil {
		return wire.Association{}, false, fmt.Errorf("reading the KeePassXC association: %w", err)
	}
	if stored.Version != associationVersion {
		return wire.Association{}, false, fmt.Errorf("the KeePassXC association is version %d, which this build does not understand", stored.Version)
	}
	if stored.ID == "" || stored.IDKey == "" {
		return wire.Association{}, false, errors.New("the KeePassXC association is incomplete")
	}
	return wire.Association{ID: stored.ID, IDKey: stored.IDKey}, true, nil
}

// Save writes the association, creating the directory if needed.
func (s FileAssociationStore) Save(a wire.Association) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("creating the directory for the KeePassXC association: %w", err)
	}
	raw, err := json.Marshal(storedAssociation{
		Version: associationVersion,
		ID:      a.ID,
		IDKey:   a.IDKey,
	})
	if err != nil {
		// The stored form is two strings and an int, so encoding it cannot
		// fail. The error is still checked in case the type grows a field
		// that can.
		//coverage:ignore
		return fmt.Errorf("writing the KeePassXC association: %w", err)
	}
	if err := os.WriteFile(s.Path, raw, 0o600); err != nil {
		return fmt.Errorf("writing the KeePassXC association: %w", err)
	}
	return nil
}
