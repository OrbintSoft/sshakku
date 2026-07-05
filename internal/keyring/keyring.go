// Package keyring wraps the Linux kernel keyring (@u user keyring) operations
// sshakku needs: storing a short-lived secret, reading it back by serial, setting
// an expiry, and unlinking it. The payload travels in the syscall buffer, never
// in argv, so secrets handed through it cannot leak via `ps` or
// /proc/<pid>/cmdline. On platforms without a kernel keyring the operations
// degrade to ErrUnavailable.
package keyring

// Serial identifies a key within the kernel keyring.
type Serial int32

// Available reports whether the kernel keyring supports a full add-then-read
// round trip in this process's environment. Add can succeed while a later
// Read of that very same key is denied — e.g. in a container, CI runner, or
// systemd service whose process was never linked to a session keyring by a
// PAM login (`pam_keyinit`) — so a bare non-error return from Add is not
// sufficient evidence the keyring is actually usable; this exercises the
// round trip callers depend on.
func Available() bool {
	s, err := Add("sshakku-keyring-probe", []byte("probe"))
	if err != nil {
		return false
	}
	defer func() { _ = Unlink(s) }()
	_, err = Read(s)
	return err == nil
}
