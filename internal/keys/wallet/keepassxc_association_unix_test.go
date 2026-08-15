//go:build unix

package wallet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileAssociationIsNotWorldReadable checks the permissions rather than
// trusting the constant: anyone who could read this file could present
// themselves to KeePassXC as SSHakku.
//
// It asks the question where the answer means something. A permission bit is
// how a Unix system says who may open a file; a system that grants access by
// access-control list reports mode bits it synthesised, and the 0077 those
// bits carry there says nothing about who may read the file.
func TestFileAssociationIsNotWorldReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assoc.json")
	store := FileAssociationStore{Path: path}
	require.NoError(t, store.Save(wire.Association{ID: "db", IDKey: "k"}), "saving the approval must succeed")
	info, err := os.Stat(path)
	require.NoError(t, err, "and the file must be there")
	assert.Zero(t, info.Mode().Perm()&0o077,
		"readable by this user alone: anyone else who could read it could present themselves to KeePassXC as SSHakku")
}
