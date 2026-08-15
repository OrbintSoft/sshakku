package wallet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileAssociationRoundTrip(t *testing.T) {
	store := FileAssociationStore{Path: filepath.Join(t.TempDir(), "nested", "assoc.json")}
	want := wire.Association{ID: "db-1", IDKey: "a-public-key"}

	require.NoError(t, store.Save(want),
		"the approval must be written down, including into a directory that is not there yet")
	got, found, err := store.Load()
	require.NoError(t, err, "and read back")
	require.True(t, found, "an approval that was saved must be found, or the user is asked to grant it again")
	assert.Equal(t, want, got, "and be the one that was granted: another would not be honoured by KeePassXC")
}

// TestFileAssociationMissingIsNotAnError covers the state every user starts in.
func TestFileAssociationMissingIsNotAnError(t *testing.T) {
	store := FileAssociationStore{Path: filepath.Join(t.TempDir(), "absent.json")}
	_, found, err := store.Load()
	require.NoError(t, err, "an approval nobody has granted yet is the state every user starts in, not an error")
	assert.False(t, found, "and there is none to find")
}

func TestFileAssociationRejectsUnreadableContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"not JSON at all", "{{{"},
		{"a version this build does not know", `{"version":99,"id":"db","idKey":"k"}`},
		{"no database id", `{"version":1,"idKey":"k"}`},
		{"no identification key", `{"version":1,"id":"db"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "assoc.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o600), "seed the stored approval")
			_, found, err := FileAssociationStore{Path: path}.Load()
			assert.Error(t, err,
				"an approval that cannot be understood must be reported, not read as one nobody ever granted")
			assert.False(t, found, "and nothing usable was read, so nothing may be reported as found")
		})
	}
}

func TestFileAssociationReportsAnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	// A directory where the file should be: readable as a path, not as a file.
	path := filepath.Join(dir, "assoc.json")
	require.NoError(t, os.Mkdir(path, 0o700), "seed a directory where the file should be")
	_, _, err := FileAssociationStore{Path: path}.Load()
	assert.Error(t, err, "an approval that could not be read must be reported, not treated as never granted")
}

func TestFileAssociationReportsAnUnwritableLocation(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600),
		"seed a file where the parent directory should be")
	// The parent of the target is a regular file, so the directory cannot be
	// created.
	store := FileAssociationStore{Path: filepath.Join(blocker, "assoc.json")}
	assert.Error(t, store.Save(wire.Association{ID: "db", IDKey: "k"}),
		"an approval that could not be written must be reported: the next run would raise the dialog again")
}

func TestFileAssociationReportsAnUnwritableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "assoc.json")
	// A directory at the target path: the directory creation succeeds, the
	// file write does not.
	require.NoError(t, os.Mkdir(path, 0o700), "seed a directory at the path the file should take")
	assert.Error(t, FileAssociationStore{Path: path}.Save(wire.Association{ID: "db", IDKey: "k"}),
		"an approval that could not be written must be reported: the next run would raise the dialog again")
}
