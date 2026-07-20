# SSHakku

Tends your SSH agent so every shell can use SSH without retyping the passphrase:
it starts and watches the agent (lifecycle, health checks, diagnostics, recovery)
and loads your keys, pulling each passphrase from the OS secret store — never
from an environment variable or a file on disk.

## How it works

Every login shell keeps one `ssh-agent` alive on a fixed socket, so
`SSH_AUTH_SOCK` never goes stale even if the agent is restarted. The first time
a key is used, SSHakku prompts for its passphrase and stores it in a dedicated
collection in your desktop's secret store (KDE Wallet, GNOME Keyring, or
KeePassXC — see [Requirements](#requirements)). Every time after that, the key
is loaded silently: open a login shell (a fresh login, or any terminal
configured to start one), and the key is already there. If something goes
wrong, `sshakku doctor` explains what and, with `--fix`, repairs it.

## Requirements

- **Linux**, with a login shell sourcing `/etc/profile.d` (the default on
  every mainstream distribution), **or macOS**, with a login shell sourcing
  `/etc/zprofile` (the default `zsh` login shell on every current macOS
  release).
- **A secret store**: on Linux, KDE Wallet, GNOME Keyring, or KeePassXC (any
  Secret Service implementation); on macOS, the Keychain. Either platform can
  instead use a password manager you already run, the 1Password or Bitwarden
  CLI — see
  [Choosing the secret backend](docs/CONFIGURATION.md#choosing-the-secret-backend).
- **Go 1.25+**, only to build from source (see Installation).

## Installation

Both modes build from source with the same `git clone` first:

```sh
git clone https://github.com/OrbintSoft/sshakku.git
cd sshakku
```

### System-wide

```sh
sudo make install
```

On Linux, installs the `sshakku` binary to `/usr/local/bin`, plus a login
hook to `/etc/profile.d` that wires it into every user's `bash`. On macOS,
the binary goes to the same path, and the hook is instead rendered to
`/usr/local/share/sshakku/` with a marker block added to `/etc/zprofile`
(macOS has no `/etc/profile.d`-style drop-in directory for `zsh`, the
default login shell). `sudo` is needed because these locations are
root-owned; `sshakku` itself never runs with elevated privileges — only the
one-time install does.

A login shell doesn't fire for every new terminal (see
[docs/DIAGNOSTICS.md](docs/DIAGNOSTICS.md)). Opt in with `sudo make install
WIRE_BASHRC=1` (Linux) or `sudo make install WIRE_ZSHRC=1` (macOS) to
additionally wire the hook into non-login interactive shells too: on Linux,
if this system's bash provides a non-login rc drop-in directory
(`/etc/bash/bashrc.d` by default), a file is dropped there, otherwise a
clearly delimited block is added to a single file (`/etc/bash.bashrc` by
default, created if it doesn't exist yet); on macOS, a marker block is added
to `/etc/zshrc` the same way. Additive to the login hook above, never a
replacement.

To remove it: `sudo make uninstall`.

Override `PREFIX`/`BINDIR`/`DESTDIR`/`ETC_PROFILE_D`/`BASH_BASHRC_D`/
`BASH_BASHRC_FILE` (Linux) or `PREFIX`/`BINDIR`/`DESTDIR`/`SHARE_DIR`/
`ETC_ZPROFILE`/`ETC_ZSHRC` (macOS) on the `make install` command line to
install elsewhere (e.g. packaging into a staging root).

### Per-user

```sh
make install-user
```

No `sudo` needed. Installs the binary to `$HOME/.local/bin/sshakku` and
wires the same login hook into your own shell only. On Linux: if
`$HOME/.bash_profile.d/` already exists, a file is dropped there; otherwise
a clearly delimited block is added to `$HOME/.bash_profile` (created if it
doesn't exist yet). On macOS: the same, but for `zsh` — `$HOME/.zprofile.d/`
if it exists, otherwise a block in `$HOME/.zprofile`. Either way the rest of
the file is left untouched. Make sure `$HOME/.local/bin` is on your `PATH`.

A login shell doesn't fire for every new terminal — a plain new tab or a
multiplexer pane often starts a non-login shell instead (see
[docs/DIAGNOSTICS.md](docs/DIAGNOSTICS.md)). To also wire the same hook into
`$HOME/.bashrc.d/`/`$HOME/.bashrc` (Linux) or `$HOME/.zshrc.d/`/
`$HOME/.zshrc` (macOS), so those pick it up too, opt in with:

```sh
make install-user WIRE_BASHRC=1   # Linux
make install-user WIRE_ZSHRC=1    # macOS
```

This is additive, never a replacement for the login hook above.

To remove it (both the login hook and, if it was wired, the non-login one):
`make uninstall-user`.

### Gentoo

The maintainer runs SSHakku from a personal ebuild overlay,
[`orbintsoft-ebuild`](https://github.com/OrbintSoft/orbintsoft-ebuild), kept in
sync with this repository.

## First run

Install, then start a new login shell — log out and back in, or run `bash -l`
(a plain new terminal tab isn't guaranteed to start one; see
[docs/DIAGNOSTICS.md](docs/DIAGNOSTICS.md) if a new terminal doesn't pick it
up). The first `ssh` to a key you haven't used yet prompts for its passphrase
once; every use after that — in this shell and every new login shell — is
silent. If a key ever stops refilling silently, run `sshakku doctor` to see
why.

## Documentation

| Guide | Covers |
| --- | --- |
| [docs/CLI.md](docs/CLI.md) | Every subcommand and flag, with exit codes. |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | Every setting: key lifetime, retries, which secret backend to use, which keys are stored/auto-loaded, and where passphrases live. |
| [docs/DEPENDENCIES.md](docs/DEPENDENCIES.md) | What must be installed to run SSHakku versus only to build it, including which pieces are backend- or feature-specific — for users and packagers. |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | Architecture, code layout, and how to build, test, and lint SSHakku — for contributors. |
| [docs/DIAGNOSTICS.md](docs/DIAGNOSTICS.md) | `sshakku doctor`: reading the report, `--fix`, `--user`, and `--test-backend`. |
| [docs/HARDENING.md](docs/HARDENING.md) | Practical steps outside SSHakku itself that keep your keys safer: a short key lifetime, not leaving the wallet unlocked, disk encryption, and `/tmp`. |
| [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) | What SSHakku protects against, what it doesn't, and why — for anyone evaluating it for a security-sensitive setup. |

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). You keep the
copyright in your work; contributions are released under the EUPL-1.2 and covered
by the [Contributor License Agreement](CLA.md).

## License

Copyright © 2026 Stefano Balzarotti (OrbintSoft) and contributors.
Licensed under the [European Union Public Licence v. 1.2](LICENSE) (`EUPL-1.2`).
The public release stays EUPL-1.2; the copyright holder may additionally offer the
project under other licences. See [COPYRIGHT.md](COPYRIGHT.md) and
[AUTHORS.md](AUTHORS.md).
