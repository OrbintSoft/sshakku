//go:build linux

package hostcheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settled fails the test when a check was left undetermined, and returns the
// answer it reached. nil and false are different answers — "nobody could tell"
// and "no" — and the report says different things about them, so a test that
// let one stand in for the other would agree with the wrong one.
func settled(t *testing.T, got *bool, what string) bool {
	t.Helper()
	require.NotNilf(t, got, "%s must be settled, not left undetermined", what)
	return *got
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755), "lay out the directory for the fake file")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644), "write the fake file")
}

func symlink(t *testing.T, oldname, newname string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(newname), 0o755), "lay out the directory for the fake symlink")
	require.NoError(t, os.Symlink(oldname, newname), "create the fake symlink")
}

func TestChecksDiskEncryptedPlainPartition(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	writeFile(t, filepath.Join(proc, "mounts"), "/dev/sda2 / ext4 rw,relatime 0 0\n")

	got := Procfs{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks(t.Context())
	assert.False(t, settled(t, got.DiskEncrypted, "disk encryption"), "a plain partition is not encrypted")
}

func TestChecksDiskEncryptedDirectLUKS(t *testing.T) {
	root := t.TempDir()
	proc, sys, dev := filepath.Join(root, "proc"), filepath.Join(root, "sys"), filepath.Join(root, "dev")
	writeFile(t, filepath.Join(proc, "mounts"), "/dev/mapper/luks-root / ext4 rw,relatime 0 0\n")
	symlink(t, "../dm-1", filepath.Join(dev, "mapper", "luks-root"))
	writeFile(t, filepath.Join(sys, "class", "block", "dm-1", "dm", "uuid"), "CRYPT-LUKS2-abcdef-luks-root\n")

	got := Procfs{ProcRoot: proc, SysRoot: sys, DevRoot: dev, Target: "/"}.Checks(t.Context())
	assert.True(t, settled(t, got.DiskEncrypted, "disk encryption"), "a LUKS volume is encrypted")
}

func TestChecksDiskEncryptedLUKSUnderLVM(t *testing.T) {
	root := t.TempDir()
	proc, sys, dev := filepath.Join(root, "proc"), filepath.Join(root, "sys"), filepath.Join(root, "dev")
	writeFile(t, filepath.Join(proc, "mounts"), "/dev/mapper/vg-root / ext4 rw,relatime 0 0\n")
	symlink(t, "../dm-2", filepath.Join(dev, "mapper", "vg-root"))
	writeFile(t, filepath.Join(sys, "class", "block", "dm-2", "dm", "uuid"), "LVM-abcdef-vg-root\n")
	require.NoError(t, os.MkdirAll(filepath.Join(sys, "class", "block", "dm-2", "slaves", "dm-1"), 0o755), "lay out the fake slave device")
	writeFile(t, filepath.Join(sys, "class", "block", "dm-1", "dm", "uuid"), "CRYPT-LUKS2-abcdef-luks-vg-root\n")

	got := Procfs{ProcRoot: proc, SysRoot: sys, DevRoot: dev, Target: "/"}.Checks(t.Context())
	assert.True(t, settled(t, got.DiskEncrypted, "disk encryption"), "LVM on top of LUKS is still encrypted underneath")
}

func TestChecksDiskEncryptedUnresolvable(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	writeFile(t, filepath.Join(proc, "mounts"), "overlay / overlay rw 0 0\n")

	got := Procfs{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks(t.Context())
	assert.Nil(t, got.DiskEncrypted, "a device nothing could be traced to leaves the question open")
}

func TestChecksDiskEncryptedNoMountsFile(t *testing.T) {
	root := t.TempDir()
	got := Procfs{ProcRoot: filepath.Join(root, "proc"), SysRoot: filepath.Join(root, "sys"), Target: "/"}.Checks(t.Context())
	assert.Nil(t, got.DiskEncrypted, "with no mount table to read, the question stays open")
}

// encryptedHomeOnPlainRoot lays out a machine mounted in two places: a plain
// root partition and a LUKS /home. Which of the two an answer comes from is
// the whole question, so the caller sets Target and nothing else.
func encryptedHomeOnPlainRoot(t *testing.T) Procfs {
	t.Helper()
	root := t.TempDir()
	proc, sys, dev := filepath.Join(root, "proc"), filepath.Join(root, "sys"), filepath.Join(root, "dev")
	writeFile(t, filepath.Join(proc, "mounts"),
		"/dev/sda1 / ext4 rw 0 0\n"+
			"/dev/mapper/luks-home /home ext4 rw 0 0\n")
	symlink(t, "../dm-3", filepath.Join(dev, "mapper", "luks-home"))
	writeFile(t, filepath.Join(sys, "class", "block", "dm-3", "dm", "uuid"), "CRYPT-LUKS2-abcdef-luks-home\n")

	return Procfs{ProcRoot: proc, SysRoot: sys, DevRoot: dev}
}

func TestChecksDiskEncryptedPicksLongestMount(t *testing.T) {
	h := encryptedHomeOnPlainRoot(t)
	h.Target = "/home/alice"

	got := h.Checks(t.Context())
	assert.True(t, settled(t, got.DiskEncrypted, "disk encryption"),
		"the answer must come from the mount the target is actually on, /home rather than /")
}

// TestChecksDiskEncryptedDoesNotMatchAPrefixOfAMountPoint pins that a mount
// point is matched at a path boundary and not as a bare string prefix: /home
// and /homework are different places, and answering about the wrong one would
// report somebody's unencrypted directory as encrypted.
func TestChecksDiskEncryptedDoesNotMatchAPrefixOfAMountPoint(t *testing.T) {
	h := encryptedHomeOnPlainRoot(t)
	h.Target = "/homework"

	got := h.Checks(t.Context())
	assert.False(t, settled(t, got.DiskEncrypted, "disk encryption"),
		"/homework is not under /home, so the answer must come from the root mount")
}

func TestChecksTmpTmpfs(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	writeFile(t, filepath.Join(proc, "mounts"),
		"/dev/sda1 / ext4 rw 0 0\n"+
			"tmpfs /tmp tmpfs rw,size=1048576k 0 0\n")

	orig := tmpfsSize
	tmpfsSize = func(string) int64 { return 512 * 1024 * 1024 }
	t.Cleanup(func() { tmpfsSize = orig })

	got := Procfs{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks(t.Context())
	assert.True(t, settled(t, got.TmpTmpfs, "whether /tmp is a tmpfs"), "a tmpfs mounted on /tmp is one")
	assert.Equal(t, int64(512*1024*1024), got.TmpSizeBytes, "the size must be the measured one, not the one in the mount options")
}

func TestChecksTmpNotTmpfs(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	writeFile(t, filepath.Join(proc, "mounts"), "/dev/sda1 / ext4 rw 0 0\n")

	got := Procfs{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks(t.Context())
	assert.False(t, settled(t, got.TmpTmpfs, "whether /tmp is a tmpfs"), "with no /tmp mount of its own, /tmp is not a tmpfs")
	assert.Zero(t, got.TmpSizeBytes, "there is no tmpfs to have a size")
}

func TestChecksTmpShadowedByLaterMount(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	writeFile(t, filepath.Join(proc, "mounts"),
		"/dev/sda1 / ext4 rw 0 0\n"+
			"/dev/sda2 /tmp ext4 rw 0 0\n"+ // stale bind mount info, shadowed below
			"tmpfs /tmp tmpfs rw 0 0\n")

	got := Procfs{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks(t.Context())
	assert.True(t, settled(t, got.TmpTmpfs, "whether /tmp is a tmpfs"),
		"where a path is mounted twice, the mount in force is the later one")
}

func TestChecksTPMPresent2_0(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "sys")
	writeFile(t, filepath.Join(sys, "class", "tpm", "tpm0", "tpm_version_major"), "2\n")

	got := Procfs{ProcRoot: filepath.Join(root, "proc"), SysRoot: sys, DevRoot: filepath.Join(root, "dev")}.Checks(t.Context())
	assert.True(t, settled(t, got.SecureHardwarePresent, "secure hardware"), "a TPM device entry is secure hardware")
	assert.Equal(t, "TPM 2.0", got.SecureHardwareKind, "the version the device reports")
}

func TestChecksTPMPresent1_2(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "sys")
	require.NoError(t, os.MkdirAll(filepath.Join(sys, "class", "tpm", "tpm0"), 0o755), "lay out the fake TPM entry")

	got := Procfs{ProcRoot: filepath.Join(root, "proc"), SysRoot: sys, DevRoot: filepath.Join(root, "dev")}.Checks(t.Context())
	assert.True(t, settled(t, got.SecureHardwarePresent, "secure hardware"), "a TPM device entry is secure hardware")
	assert.Equal(t, "TPM 1.2", got.SecureHardwareKind, "a device that reports no version is the older one")
}

func TestChecksTPMAbsent(t *testing.T) {
	root := t.TempDir()
	got := Procfs{ProcRoot: filepath.Join(root, "proc"), SysRoot: filepath.Join(root, "sys"), DevRoot: filepath.Join(root, "dev")}.Checks(t.Context())
	assert.False(t, settled(t, got.SecureHardwarePresent, "secure hardware"), "a machine with no TPM entry has none")
	assert.Empty(t, got.SecureHardwareKind, "there is no device to name a version for")
}

func TestChecksTPMIgnoresResourceManagerEntry(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "sys")
	require.NoError(t, os.MkdirAll(filepath.Join(sys, "class", "tpm", "tpmrm0"), 0o755), "lay out the fake resource-manager entry")

	got := Procfs{ProcRoot: filepath.Join(root, "proc"), SysRoot: sys, DevRoot: filepath.Join(root, "dev")}.Checks(t.Context())
	assert.False(t, settled(t, got.SecureHardwarePresent, "secure hardware"),
		"tpmrm0 is the kernel's resource manager, not a TPM device")
}

func TestUnescapeMount(t *testing.T) {
	cases := map[string]string{
		`/mnt/my\040drive`: "/mnt/my drive",
		`/tmp`:             "/tmp",
		`back\134slash`:    `back\slash`,
	}
	for in, want := range cases {
		assert.Equalf(t, want, unescapeMount(in), "the mount point behind %s", in)
	}
}
