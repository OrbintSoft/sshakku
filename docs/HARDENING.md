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

## Encrypt the disk

Everything above assumes the wallet database itself is out of reach at rest.
If the disk isn't encrypted, anyone with the drive — lost, stolen, or
discarded — can read it directly, bypassing the wallet's own lock entirely.
Full-disk encryption (LUKS on Linux) closes that gap; most distribution
installers offer to set it up during installation. `sshakku doctor` reports
whether it detected encryption on the disk backing your home directory — see
[Environment hardening checks](DIAGNOSTICS.md#environment-hardening-checks).

If your machine has a TPM, it can also back a stronger unlock than a
passphrase-typed-at-boot LUKS setup (for example, `systemd-cryptenroll`'s TPM2
support). `doctor` reports whether a TPM is present, as a hint of what's
available.

## Configure `/tmp`

Temporary files from other tools can end up on disk if `/tmp` isn't
memory-backed. Most modern distributions already mount `/tmp` as `tmpfs`; if
yours doesn't, systemd's `tmp.mount` or a `tmpfs` line in `/etc/fstab` fixes
it. `sshakku doctor` reports whether `/tmp` is tmpfs-backed — see
[Environment hardening checks](DIAGNOSTICS.md#environment-hardening-checks).

## Checking all of the above at once

`sshakku doctor` reports disk encryption, `/tmp`, and TPM presence together
under "environment", and `sshakku doctor --test-backend` proves your
configured secret backend actually works end to end — see
[Diagnostics](DIAGNOSTICS.md).
