//go:build linux

package diagnose

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(newname), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatal(err)
	}
}

func TestChecksDiskEncryptedPlainPartition(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	writeFile(t, filepath.Join(proc, "mounts"), "/dev/sda2 / ext4 rw,relatime 0 0\n")

	got := ProcfsHostSource{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks()
	if got.DiskEncrypted == nil || *got.DiskEncrypted {
		t.Fatalf("DiskEncrypted = %v, want false", got.DiskEncrypted)
	}
}

func TestChecksDiskEncryptedDirectLUKS(t *testing.T) {
	root := t.TempDir()
	proc, sys, dev := filepath.Join(root, "proc"), filepath.Join(root, "sys"), filepath.Join(root, "dev")
	writeFile(t, filepath.Join(proc, "mounts"), "/dev/mapper/luks-root / ext4 rw,relatime 0 0\n")
	symlink(t, "../dm-1", filepath.Join(dev, "mapper", "luks-root"))
	writeFile(t, filepath.Join(sys, "class", "block", "dm-1", "dm", "uuid"), "CRYPT-LUKS2-abcdef-luks-root\n")

	got := ProcfsHostSource{ProcRoot: proc, SysRoot: sys, DevRoot: dev, Target: "/"}.Checks()
	if got.DiskEncrypted == nil || !*got.DiskEncrypted {
		t.Fatalf("DiskEncrypted = %v, want true", got.DiskEncrypted)
	}
}

func TestChecksDiskEncryptedLUKSUnderLVM(t *testing.T) {
	root := t.TempDir()
	proc, sys, dev := filepath.Join(root, "proc"), filepath.Join(root, "sys"), filepath.Join(root, "dev")
	writeFile(t, filepath.Join(proc, "mounts"), "/dev/mapper/vg-root / ext4 rw,relatime 0 0\n")
	symlink(t, "../dm-2", filepath.Join(dev, "mapper", "vg-root"))
	writeFile(t, filepath.Join(sys, "class", "block", "dm-2", "dm", "uuid"), "LVM-abcdef-vg-root\n")
	if err := os.MkdirAll(filepath.Join(sys, "class", "block", "dm-2", "slaves", "dm-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sys, "class", "block", "dm-1", "dm", "uuid"), "CRYPT-LUKS2-abcdef-luks-vg-root\n")

	got := ProcfsHostSource{ProcRoot: proc, SysRoot: sys, DevRoot: dev, Target: "/"}.Checks()
	if got.DiskEncrypted == nil || !*got.DiskEncrypted {
		t.Fatalf("DiskEncrypted = %v, want true", got.DiskEncrypted)
	}
}

func TestChecksDiskEncryptedUnresolvable(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	writeFile(t, filepath.Join(proc, "mounts"), "overlay / overlay rw 0 0\n")

	got := ProcfsHostSource{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks()
	if got.DiskEncrypted != nil {
		t.Fatalf("DiskEncrypted = %v, want nil (unresolvable device)", got.DiskEncrypted)
	}
}

func TestChecksDiskEncryptedNoMountsFile(t *testing.T) {
	root := t.TempDir()
	got := ProcfsHostSource{ProcRoot: filepath.Join(root, "proc"), SysRoot: filepath.Join(root, "sys"), Target: "/"}.Checks()
	if got.DiskEncrypted != nil {
		t.Fatalf("DiskEncrypted = %v, want nil (no /proc/mounts)", got.DiskEncrypted)
	}
}

func TestChecksDiskEncryptedPicksLongestMount(t *testing.T) {
	root := t.TempDir()
	proc, sys, dev := filepath.Join(root, "proc"), filepath.Join(root, "sys"), filepath.Join(root, "dev")
	writeFile(t, filepath.Join(proc, "mounts"),
		"/dev/sda1 / ext4 rw 0 0\n"+
			"/dev/mapper/luks-home /home ext4 rw 0 0\n")
	symlink(t, "../dm-3", filepath.Join(dev, "mapper", "luks-home"))
	writeFile(t, filepath.Join(sys, "class", "block", "dm-3", "dm", "uuid"), "CRYPT-LUKS2-abcdef-luks-home\n")

	got := ProcfsHostSource{ProcRoot: proc, SysRoot: sys, DevRoot: dev, Target: "/home/alice"}.Checks()
	if got.DiskEncrypted == nil || !*got.DiskEncrypted {
		t.Fatalf("DiskEncrypted = %v, want true (should match /home, not /)", got.DiskEncrypted)
	}
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

	got := ProcfsHostSource{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks()
	if got.TmpTmpfs == nil || !*got.TmpTmpfs {
		t.Fatalf("TmpTmpfs = %v, want true", got.TmpTmpfs)
	}
	if got.TmpSizeBytes != 512*1024*1024 {
		t.Fatalf("TmpSizeBytes = %d, want %d", got.TmpSizeBytes, 512*1024*1024)
	}
}

func TestChecksTmpNotTmpfs(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	writeFile(t, filepath.Join(proc, "mounts"), "/dev/sda1 / ext4 rw 0 0\n")

	got := ProcfsHostSource{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks()
	if got.TmpTmpfs == nil || *got.TmpTmpfs {
		t.Fatalf("TmpTmpfs = %v, want false (no separate /tmp mount)", got.TmpTmpfs)
	}
	if got.TmpSizeBytes != 0 {
		t.Fatalf("TmpSizeBytes = %d, want 0", got.TmpSizeBytes)
	}
}

func TestChecksTmpShadowedByLaterMount(t *testing.T) {
	root := t.TempDir()
	proc, sys := filepath.Join(root, "proc"), filepath.Join(root, "sys")
	writeFile(t, filepath.Join(proc, "mounts"),
		"/dev/sda1 / ext4 rw 0 0\n"+
			"/dev/sda2 /tmp ext4 rw 0 0\n"+ // stale bind mount info, shadowed below
			"tmpfs /tmp tmpfs rw 0 0\n")

	got := ProcfsHostSource{ProcRoot: proc, SysRoot: sys, DevRoot: filepath.Join(root, "dev"), Target: "/"}.Checks()
	if got.TmpTmpfs == nil || !*got.TmpTmpfs {
		t.Fatalf("TmpTmpfs = %v, want true (the later /tmp entry should win)", got.TmpTmpfs)
	}
}

func TestChecksTPMPresent2_0(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "sys")
	writeFile(t, filepath.Join(sys, "class", "tpm", "tpm0", "tpm_version_major"), "2\n")

	got := ProcfsHostSource{ProcRoot: filepath.Join(root, "proc"), SysRoot: sys, DevRoot: filepath.Join(root, "dev")}.Checks()
	if got.SecureHardwarePresent == nil || !*got.SecureHardwarePresent {
		t.Fatalf("SecureHardwarePresent = %v, want true", got.SecureHardwarePresent)
	}
	if got.SecureHardwareKind != "TPM 2.0" {
		t.Fatalf("SecureHardwareKind = %q, want %q", got.SecureHardwareKind, "TPM 2.0")
	}
}

func TestChecksTPMPresent1_2(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "sys")
	if err := os.MkdirAll(filepath.Join(sys, "class", "tpm", "tpm0"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := ProcfsHostSource{ProcRoot: filepath.Join(root, "proc"), SysRoot: sys, DevRoot: filepath.Join(root, "dev")}.Checks()
	if got.SecureHardwarePresent == nil || !*got.SecureHardwarePresent {
		t.Fatalf("SecureHardwarePresent = %v, want true", got.SecureHardwarePresent)
	}
	if got.SecureHardwareKind != "TPM 1.2" {
		t.Fatalf("SecureHardwareKind = %q, want %q", got.SecureHardwareKind, "TPM 1.2")
	}
}

func TestChecksTPMAbsent(t *testing.T) {
	root := t.TempDir()
	got := ProcfsHostSource{ProcRoot: filepath.Join(root, "proc"), SysRoot: filepath.Join(root, "sys"), DevRoot: filepath.Join(root, "dev")}.Checks()
	if got.SecureHardwarePresent == nil || *got.SecureHardwarePresent {
		t.Fatalf("SecureHardwarePresent = %v, want false", got.SecureHardwarePresent)
	}
	if got.SecureHardwareKind != "" {
		t.Fatalf("SecureHardwareKind = %q, want empty", got.SecureHardwareKind)
	}
}

func TestChecksTPMIgnoresResourceManagerEntry(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "sys")
	if err := os.MkdirAll(filepath.Join(sys, "class", "tpm", "tpmrm0"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := ProcfsHostSource{ProcRoot: filepath.Join(root, "proc"), SysRoot: sys, DevRoot: filepath.Join(root, "dev")}.Checks()
	if got.SecureHardwarePresent == nil || *got.SecureHardwarePresent {
		t.Fatalf("SecureHardwarePresent = %v, want false (tpmrm0 is not a TPM device entry)", got.SecureHardwarePresent)
	}
}

func TestUnescapeMount(t *testing.T) {
	cases := map[string]string{
		`/mnt/my\040drive`: "/mnt/my drive",
		`/tmp`:             "/tmp",
		`back\134slash`:    `back\slash`,
	}
	for in, want := range cases {
		if got := unescapeMount(in); got != want {
			t.Errorf("unescapeMount(%q) = %q, want %q", in, got, want)
		}
	}
}
