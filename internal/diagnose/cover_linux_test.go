//go:build linux

package diagnose

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseMountsSkipsShortLine covers parseMounts dropping a line with fewer
// than three whitespace-separated fields.
func TestParseMountsSkipsShortLine(t *testing.T) {
	entries := parseMounts([]byte("only two\n/dev/sda1 / ext4 rw 0 0\n"))
	require.Len(t, entries, 1, "only the well-formed line describes a mount")
	assert.Equal(t, "/", entries[0].mountPoint, "the mount point of the line that was kept")
}

// TestResolveDevBaseAbsoluteSymlink covers resolveDevBase following an absolute
// symlink target that stays under /dev, and stopping when an absolute target
// escapes /dev.
func TestResolveDevBaseAbsoluteSymlink(t *testing.T) {
	dev := t.TempDir()
	// mapper/root → absolute /dev/dm-3 (stays under /dev): base resolves to dm-3.
	symlink(t, "/dev/dm-3", filepath.Join(dev, "mapper", "root"))
	assert.Equal(t, "dm-3", resolveDevBase(dev, "/dev/mapper/root"), "a target still under /dev is followed")
	// mapper/escape → absolute path outside /dev: the walk stops at the symlink.
	symlink(t, "/elsewhere/x", filepath.Join(dev, "mapper", "escape"))
	assert.Equal(t, "escape", resolveDevBase(dev, "/dev/mapper/escape"), "the walk stops where the target leaves /dev")
}

// TestDeviceEncryptedNonLUKSNoSlaves covers deviceEncrypted's final false return
// when the dm/uuid exists but is not LUKS and there are no slave devices to chase.
func TestDeviceEncryptedNonLUKSNoSlaves(t *testing.T) {
	sys := t.TempDir()
	writeFile(t, filepath.Join(sys, "class", "block", "dm-9", "dm", "uuid"), "LVM-plainvolume\n")
	got := deviceEncrypted(sys, "dm-9", 1)
	require.NotNil(t, got, "a device that was read must be answered for, not left undetermined")
	assert.False(t, *got, "a plain LVM volume with nothing under it is not encrypted")
}

// TestDeviceEncryptedVerityIsNotEncryption pins that the device-mapper check
// looks for LUKS and not merely for the CRYPT family: dm-verity authenticates
// a volume without encrypting it, and calling such a disk encrypted would tell
// a user their keys are protected at rest when they are not.
func TestDeviceEncryptedVerityIsNotEncryption(t *testing.T) {
	sys := t.TempDir()
	writeFile(t, filepath.Join(sys, "class", "block", "dm-7", "dm", "uuid"), "CRYPT-VERITY-abcdef-root\n")
	got := deviceEncrypted(sys, "dm-7", 1)
	require.NotNil(t, got, "a device that was read must be answered for")
	assert.False(t, *got, "an authenticated volume is not an encrypted one")
}

// TestRealTmpfsSizeError covers realTmpfsSize returning 0 when statfs fails on a
// path that does not exist.
func TestRealTmpfsSizeError(t *testing.T) {
	assert.Zero(t, realTmpfsSize(filepath.Join(t.TempDir(), "does-not-exist")),
		"a path that could not be measured has no size to report")
}
