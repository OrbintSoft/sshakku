package diagnose

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HostChecks are best-effort, read-only observations about the host
// environment: conditions outside sshakku's own control that materially
// affect its threat model (a leaked wallet on an unencrypted disk, a
// passphrase transiting a world-readable /tmp). A nil pointer means "could
// not determine" — these checks never guess, and doctor only ever reports
// them, never configures or refuses to run because of them.
type HostChecks struct {
	// DiskEncrypted reports whether the block device backing Target (see
	// ProcfsHostSource) is LUKS-encrypted, including one level of
	// LUKS-under-LVM. nil when the backing device could not be resolved
	// (network filesystem, overlay, tmpfs root, missing /proc/mounts).
	DiskEncrypted *bool

	// TmpTmpfs reports whether /tmp is its own tmpfs mount, as opposed to
	// living on the root filesystem. nil when /proc/mounts could not be read.
	TmpTmpfs *bool
	// TmpSizeBytes is /tmp's total size when TmpTmpfs is true; 0 when
	// TmpTmpfs is not true, or the size could not be determined.
	TmpSizeBytes int64

	// TPMPresent reports whether the kernel has a TPM device driver bound
	// (i.e. the firmware exposed and enabled a TPM the kernel could claim) —
	// never nil, since an absent /sys/class/tpm is itself a determination,
	// not an unknown.
	TPMPresent *bool
	// TPMVersion is "2.0" or "1.2" when TPMPresent is true, else "".
	TPMVersion string
}

// HostSource gathers HostChecks; ProcfsHostSource is the real implementation
// reading /proc, /sys, and /dev on Linux. Tests supply a fake.
type HostSource interface {
	Checks() HostChecks
}

// ProcfsHostSource reads /proc/mounts, /sys/class/block, /sys/class/tpm, and
// resolves device-mapper symlinks under /dev. ProcRoot/SysRoot/DevRoot are
// injectable for tests; empty means the real "/proc"/"/sys"/"/dev". Target is
// the path whose backing filesystem is checked for encryption; empty means
// "/".
type ProcfsHostSource struct {
	ProcRoot string
	SysRoot  string
	DevRoot  string
	Target   string
}

// Checks implements HostSource.
func (h ProcfsHostSource) Checks() HostChecks {
	procRoot := orDefault(h.ProcRoot, "/proc")
	sysRoot := orDefault(h.SysRoot, "/sys")
	devRoot := orDefault(h.DevRoot, "/dev")
	target := orDefault(h.Target, "/")

	var hc HostChecks
	if b, err := os.ReadFile(filepath.Join(procRoot, "mounts")); err == nil {
		mounts := parseMounts(b)
		if m, ok := findMountFor(mounts, target); ok && strings.HasPrefix(m.device, "/dev/") {
			devBase := resolveDevBase(devRoot, m.device)
			hc.DiskEncrypted = deviceEncrypted(sysRoot, devBase, 1)
		}
		if tm, ok := findExactMount(mounts, "/tmp"); ok {
			tmpfs := tm.fstype == "tmpfs"
			hc.TmpTmpfs = &tmpfs
			if tmpfs {
				hc.TmpSizeBytes = tmpfsSize(tm.mountPoint)
			}
		} else {
			// No dedicated /tmp mount at all means /tmp lives on whatever
			// filesystem contains it (almost always the root fs) — a real
			// determination, not an unknown, since the full mount table was
			// read successfully.
			notTmpfs := false
			hc.TmpTmpfs = &notTmpfs
		}
	}

	hc.TPMPresent, hc.TPMVersion = tpmInfo(sysRoot)
	return hc
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// tmpfsSize returns a tmpfs mount's total size in bytes, or 0 when it could
// not be determined. A var so tests can stub it without a real tmpfs mount;
// realTmpfsSize (OS-specific) is the production value.
var tmpfsSize = realTmpfsSize

// mountEntry is one parsed /proc/mounts line.
type mountEntry struct {
	device     string
	mountPoint string
	fstype     string
}

// parseMounts parses /proc/mounts content (the same whitespace-separated,
// octal-escaped format as /etc/fstab). Malformed lines are skipped.
func parseMounts(b []byte) []mountEntry {
	var entries []mountEntry
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		entries = append(entries, mountEntry{
			device:     unescapeMount(fields[0]),
			mountPoint: unescapeMount(fields[1]),
			fstype:     fields[2],
		})
	}
	return entries
}

var mountEscapes = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

// unescapeMount decodes the octal escapes /proc/mounts uses for spaces, tabs,
// newlines, and backslashes in device paths and mount points.
func unescapeMount(s string) string {
	return mountEscapes.Replace(s)
}

// findMountFor returns the mount entry covering target: the entry whose
// mount point is target itself or an ancestor directory, preferring the
// longest (most specific) match and, among equal-length matches, the last
// one listed (the most recently mounted, which shadows any earlier mount at
// the same point).
func findMountFor(mounts []mountEntry, target string) (mountEntry, bool) {
	target = filepath.Clean(target)
	var best mountEntry
	bestLen := -1
	for _, m := range mounts {
		mp := filepath.Clean(m.mountPoint)
		if mp != target && mp != "/" && !strings.HasPrefix(target, mp+"/") {
			continue
		}
		if len(mp) >= bestLen {
			best = m
			bestLen = len(mp)
		}
	}
	return best, bestLen >= 0
}

// findExactMount returns the mount entry at exactly mountPoint, preferring
// the last one listed (see findMountFor).
func findExactMount(mounts []mountEntry, mountPoint string) (mountEntry, bool) {
	mountPoint = filepath.Clean(mountPoint)
	var best mountEntry
	found := false
	for _, m := range mounts {
		if filepath.Clean(m.mountPoint) == mountPoint {
			best = m
			found = true
		}
	}
	return best, found
}

// resolveDevBase follows device's chain of symlinks (as /dev/mapper/* and
// /dev/disk/by-*/* both are, relative to /dev) under devRoot and returns the
// final basename — the name /sys/class/block entries use, e.g. "dm-1" or
// "sda1". device is expected to start with "/dev/"; callers check that
// before calling. A non-symlink, a symlink target outside /dev, or a chain
// longer than 8 hops (defensive bound, real chains are 1 hop) stops the walk
// and returns the basename reached so far.
func resolveDevBase(devRoot, device string) string {
	rel := strings.TrimPrefix(device, "/dev/")
	for range 8 {
		target, err := os.Readlink(filepath.Join(devRoot, rel))
		if err != nil {
			break
		}
		if filepath.IsAbs(target) {
			if !strings.HasPrefix(target, "/dev/") {
				break
			}
			rel = strings.TrimPrefix(target, "/dev/")
		} else {
			rel = filepath.Join(filepath.Dir(rel), target)
		}
	}
	return filepath.Base(rel)
}

// deviceEncrypted reports whether devBase (a /sys/class/block entry name) is
// a LUKS device-mapper target, following one level of "slaves" (the devices
// it is built on) when devBase is itself device-mapper but not LUKS — the
// LUKS-under-LVM shape, where the mounted logical volume's dm/uuid says
// "LVM-...", not "CRYPT-LUKS...", and the LUKS device is one of its slaves.
// No dm node at all (a plain partition) is a determination, not an unknown:
// it returns false, never nil. depth bounds the slave recursion (1 covers
// LUKS-under-LVM; deeper stacks are not chased).
func deviceEncrypted(sysRoot, devBase string, depth int) *bool {
	b, err := os.ReadFile(filepath.Join(sysRoot, "class", "block", devBase, "dm", "uuid"))
	if err != nil {
		f := false
		return &f
	}
	if strings.HasPrefix(strings.TrimSpace(string(b)), "CRYPT-LUKS") {
		t := true
		return &t
	}
	if depth > 0 {
		if entries, err := os.ReadDir(filepath.Join(sysRoot, "class", "block", devBase, "slaves")); err == nil {
			for _, e := range entries {
				if r := deviceEncrypted(sysRoot, e.Name(), depth-1); r != nil && *r {
					return r
				}
			}
		}
	}
	f := false
	return &f
}

// tpmNameRE matches a TPM device's /sys/class/tpm entry ("tpm0", "tpm1", …),
// excluding the kernel resource-manager entries ("tpmrm0", …).
var tpmNameRE = regexp.MustCompile(`^tpm[0-9]+$`)

// tpmInfo reports whether the kernel has a TPM device driver bound and, if
// so, its rough version. TPM 2.0's sysfs ABI exposes tpm_version_major;
// TPM 1.2's predates that file, so its presence without the file is taken as
// 1.2. An unreadable or empty /sys/class/tpm is a determination (no bound
// TPM driver), not an unknown — presence is never nil.
func tpmInfo(sysRoot string) (*bool, string) {
	entries, err := os.ReadDir(filepath.Join(sysRoot, "class", "tpm"))
	if err == nil {
		for _, e := range entries {
			if !tpmNameRE.MatchString(e.Name()) {
				continue
			}
			present := true
			version := "1.2"
			if b, err := os.ReadFile(filepath.Join(sysRoot, "class", "tpm", e.Name(), "tpm_version_major")); err == nil {
				if strings.TrimSpace(string(b)) == "2" {
					version = "2.0"
				}
			}
			return &present, version
		}
	}
	absent := false
	return &absent, ""
}
