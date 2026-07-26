//go:build linux

package diagnose

import (
	"path/filepath"
	"testing"
)

// TestParseMountsSkipsShortLine covers parseMounts dropping a line with fewer
// than three whitespace-separated fields.
func TestParseMountsSkipsShortLine(t *testing.T) {
	entries := parseMounts([]byte("only two\n/dev/sda1 / ext4 rw 0 0\n"))
	if len(entries) != 1 || entries[0].mountPoint != "/" {
		t.Errorf("parseMounts = %+v, want just the well-formed entry", entries)
	}
}

// TestResolveDevBaseAbsoluteSymlink covers resolveDevBase following an absolute
// symlink target that stays under /dev, and stopping when an absolute target
// escapes /dev.
func TestResolveDevBaseAbsoluteSymlink(t *testing.T) {
	dev := t.TempDir()
	// mapper/root → absolute /dev/dm-3 (stays under /dev): base resolves to dm-3.
	symlink(t, "/dev/dm-3", filepath.Join(dev, "mapper", "root"))
	if got := resolveDevBase(dev, "/dev/mapper/root"); got != "dm-3" {
		t.Errorf("resolveDevBase = %q, want dm-3", got)
	}
	// mapper/escape → absolute path outside /dev: the walk stops at the symlink.
	symlink(t, "/elsewhere/x", filepath.Join(dev, "mapper", "escape"))
	if got := resolveDevBase(dev, "/dev/mapper/escape"); got != "escape" {
		t.Errorf("resolveDevBase = %q, want escape (walk stops when leaving /dev)", got)
	}
}

// TestDeviceEncryptedNonLUKSNoSlaves covers deviceEncrypted's final false return
// when the dm/uuid exists but is not LUKS and there are no slave devices to chase.
func TestDeviceEncryptedNonLUKSNoSlaves(t *testing.T) {
	sys := t.TempDir()
	writeFile(t, filepath.Join(sys, "class", "block", "dm-9", "dm", "uuid"), "LVM-plainvolume\n")
	if got := deviceEncrypted(sys, "dm-9", 1); got == nil || *got {
		t.Errorf("deviceEncrypted = %v, want a definite false", got)
	}
}

// TestRealTmpfsSizeError covers realTmpfsSize returning 0 when statfs fails on a
// path that does not exist.
func TestRealTmpfsSizeError(t *testing.T) {
	if got := realTmpfsSize(filepath.Join(t.TempDir(), "does-not-exist")); got != 0 {
		t.Errorf("realTmpfsSize of a missing path = %d, want 0", got)
	}
}
