# Hardening

SSHakku protects the passphrase itself: it never sits in an environment
variable, a log, or a file on disk. But a passphrase pulled silently from the
wallet is only as safe as the wallet, the disk it lives on, and the machine
around it — none of which SSHakku controls. This page covers what's worth
doing on top, and what `sshakku doctor` can check for you along the way.

## A short key lifetime

The agent forgets a key after its lifetime elapses (`key_lifetime` /
`SSHAKKU_KEY_LIFETIME`, default 8 hours) — and because SSHakku refills it
silently from the wallet on the next shell, a short lifetime costs you
nothing. It bounds how long an unlocked key sits in the agent, available to
anything running as you, without ever asking you to retype a passphrase from
memory. See [Settings](CONFIGURATION.md#settings) for how to change it.

**Where the agent keeps no lifetimes, this bounds less than it looks like it
does.** Windows' `ssh-agent` accepts no expiry, so the key is taken out by
SSHakku instead, as the next session opens — which means the window closes at
your next login rather than at the deadline you set, and on a machine nobody
logs into it does not close at all. The key also survives a reboot there, since
that agent keeps what it is given in your account's registry. A short lifetime
is still worth setting and still costs nothing; what bounds the window on that
platform is locking the session, and the shortest lifetime in the world does
not substitute for it. `sshakku doctor` says which of the two kinds of agent
you are on.

## Don't leave the wallet unlocked

SSHakku itself only unlocks your secret-store collection for the seconds
around each lookup or store, then locks it again — but that only bounds the
window *it* opens. If the wallet is also unlocked by something else (you
opened it manually, another app queried it) and your desktop has no idle-lock
timeout of its own, it can stay unlocked far longer than SSHakku ever needs.
Set one:

- **KDE Wallet** — System Settings → KDE Wallet: "Close when unused for"
  and "Close when screen is locked".
- **GNOME Keyring** — locks with the screen; keep the screen lock's idle
  timeout short (Settings → Privacy → Screen Lock).
- **KeePassXC** — Application Settings → Security: "Lock databases after
  inactivity".
- **Windows Credential Manager** — there is nothing to close. A credential
  stored there is decrypted for anything running as your account: no unlock
  step, and no idle timeout to set. What stands between a stored passphrase and
  another program running as you is the session lock itself — the same thing
  that bounds the agent's keys above, which is why on this platform the two
  recommendations collapse into one. Lock the screen.

## Encrypt the disk

Everything above assumes the wallet database itself is out of reach at rest.
If the disk isn't encrypted, anyone with the drive — lost, stolen, or
discarded — can read it directly, bypassing the wallet's own lock entirely.
Full-disk encryption (LUKS on Linux, FileVault on macOS) closes that gap;
most distribution installers offer to set it up during installation, and
macOS offers it during first setup or later under System Settings →
Privacy & Security → FileVault. `sshakku doctor` reports whether it detected
encryption on the disk backing your home directory — see
[Environment hardening checks](DIAGNOSTICS.md#environment-hardening-checks).

If your machine has a TPM (Linux) or a Secure Enclave (macOS), it can also
back a stronger unlock than a plain passphrase — for example,
`systemd-cryptenroll`'s TPM2 support for LUKS. `doctor` reports whether such
hardware is present, as a hint of what's available.

## Configure `/tmp`

Temporary files from other tools can end up on disk if `/tmp` isn't
memory-backed. Most modern distributions already mount `/tmp` as `tmpfs`; if
yours doesn't, systemd's `tmp.mount` or a `tmpfs` line in `/etc/fstab` fixes
it. `sshakku doctor` reports whether `/tmp` is tmpfs-backed — see
[Environment hardening checks](DIAGNOSTICS.md#environment-hardening-checks).

## Checking all of the above at once

`sshakku doctor` reports disk encryption, `/tmp`, and secure hardware
presence together under "environment", and `sshakku doctor --test-backend`
proves your
configured secret backend actually works end to end — see
[Diagnostics](DIAGNOSTICS.md).
