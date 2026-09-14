# Dependencies

What has to be present on Linux, macOS or Windows to *run* SSHakku, versus
what's needed only to *build* it from source — for end users and for anyone
packaging it.

## To build

- **Go 1.26.0 or newer**, which is what `go.mod` requires. The only build-time
  requirement, on any of the three;
  `go build ./...` (or `make build`) fetches the Go module dependencies itself
  (`github.com/godbus/dbus/v5`, `github.com/BurntSushi/toml`,
  `golang.org/x/sys`, `github.com/ebitengine/purego`,
  `github.com/gofrs/flock`). All are pure Go: nothing
  here needs a C compiler, an Apple SDK, or cgo, so a build for any of the
  three can be produced on any of them. The macOS Keychain backend reaches
  Security.framework by loading it at run time rather than by linking against it
  at build time.

## To run

Always required, regardless of configuration:

- **OpenSSH client tools**: `ssh-add`, `ssh-agent`, `ssh-keygen`. SSHakku
  starts and manages its own `ssh-agent` process and drives `ssh-add`/
  `ssh-keygen` for every key it loads or fingerprints — there is no bundled
  reimplementation of any of these. Present by default on both Linux and
  macOS; on Windows it is an optional capability that has to be added first,
  see below.
- **OpenSSH 8.4 or newer**, on every platform. Below that release there is no
  way to tell `ssh` to ask a passphrase helper it would not have reached on its
  own — and on a session that sets no `DISPLAY` it would not have reached one,
  which is an ordinary Windows console, a Mac without an X server, and any
  machine you are on over SSH. On such a build your keys still load, but the
  wallet is never consulted for them and you are asked on the terminal instead.
  `ssh -V` says which build you have; `sshakku doctor` says so too, naming the
  version it found, so you do not have to know this number to find out.
- **A login shell that sources `/etc/profile.d`** on Linux, or `/etc/zprofile`
  on macOS (or, for a per-user install, `~/.bash_profile`/
  `~/.bash_profile.d/` on Linux, `~/.zprofile`/`~/.zprofile.d/` on macOS) —
  see the [Requirements](../README.md#requirements) section of the README. A
  per-user install can optionally also wire `~/.bashrc`/`~/.bashrc.d/`
  (Linux) or `~/.zshrc`/`~/.zshrc.d/` (macOS) via `make install-user
  WIRE_BASHRC=1`/`WIRE_ZSHRC=1` to additionally cover non-login shells.

Required only for the Secret Service backend, which is what a Linux install
uses when no `secret_backend` is set. It is a freedesktop mechanism and exists
on Linux alone: a macOS install defaults to the Keychain instead and needs
none of this, nor any configuration to get it (see
[Choosing the secret backend](CONFIGURATION.md#choosing-the-secret-backend)):

- **A reachable D-Bus session bus** with a Secret Service implementation
  behind it — KDE Wallet, GNOME Keyring, or KeePassXC (via its Secret Service
  integration), whichever the desktop environment already runs. SSHakku talks
  to `org.freedesktop.secrets` directly over D-Bus (no external CLI needed for
  this path).
- **`secret-tool`** (from `libsecret`'s command-line tools), as a fallback
  used only when the D-Bus session bus can't be reached at all (e.g. a
  non-interactive or non-desktop login) — see
  [Where passphrases are stored](CONFIGURATION.md#where-passphrases-are-stored).

On macOS the Keychain — the default there — needs nothing beyond what is
already on every Mac: SSHakku talks to Security.framework directly, never
shelling out to the `security` CLI.

Required only when a graphical passphrase prompt is used (a GUI session is
detected — Wayland or X11 with a live `$DISPLAY` confirmed via `xset` on
Linux, always on macOS):

- **`kdialog`** on Linux, for the passphrase dialog itself. Without it, or
  without a GUI session, SSHakku falls back to a terminal prompt instead (via
  `ssh-add` for an SSH key's own passphrase, or a plain terminal prompt for
  the Bitwarden master password). macOS needs no extra package for its
  graphical prompt.

Required only when `secret_backend` selects that backend in `config.toml` (see
[Choosing the secret backend](CONFIGURATION.md#choosing-the-secret-backend)):

- **`op`** (the 1Password CLI), for `secret_backend = "1password"`. Must
  already be signed in — SSHakku only runs `op read`/`op item ...`, never `op
  signin`.
- **`bw`** (the Bitwarden CLI), for `secret_backend = "bitwarden"`. No
  sign-in is assumed ahead of time; SSHakku drives `bw login`/`bw unlock`
  itself, prompting for the master password each time it needs the vault.

### On Windows

Windows is the one platform where the SSH tools themselves are not there until
somebody puts them there:

- **The OpenSSH client capability.** From Windows 10 build 1809 and Windows
  Server 2019 it ships as an optional feature that is *not* installed; an
  administrator adds it with `Add-WindowsCapability -Online -Name
  OpenSSH.Client~~~~0.0.1.0`, which is the command SSHakku names for you when
  it finds the tools missing. The `ssh-agent` service comes with it, and that
  service is what SSHakku drives here — it never starts an agent process of its
  own on this platform. Which OpenSSH release a given Windows carries is not
  something Microsoft publishes, and versions below the 8.4 floor above have
  shipped in the box; `sshakku doctor` names the one you have.
- **Nothing for the wallet.** Passphrases go into the Windows Credential
  Manager, which SSHakku reaches through the OS directly: no CLI to install, no
  daemon to run, no configuration to get it. It also has no lock of its own —
  see [Hardening](HARDENING.md#dont-leave-the-wallet-unlocked) for what that
  means and what to do instead.
- **Nothing for the passphrase dialog.** SSHakku draws it with `user32`, so
  there is no `kdialog` equivalent to install, nothing to package, and no
  script execution policy in the way of it.
- **`keepassxc-cli`**, and only if you configure `secret_backend =
  "keepassxc"`. It must be on `PATH`: SSHakku runs it by name.
- **Git Bash, GNU Make and the Go toolchain**, to install SSHakku at all. The
  install path described in [Installation](INSTALLATION.md) is a `make` target
  building from source, so these are needed on the machine being installed to —
  which is a requirement of that path rather than of the program, and one a
  packaged installer would remove.

## For packagers

A distribution package should declare:

- A build-time dependency on the Go toolchain (`>= 1.26.0`, the version
  `go.mod` requires — not the newer `toolchain` line beside it, which says
  which toolchain this repository builds with and not what the module needs),
  and nothing else — no C toolchain is involved.
- A runtime dependency on `openssh` (for `ssh-add`/`ssh-agent`/`ssh-keygen`).
- On Linux: `libsecret`'s tools (for `secret-tool`) and `kdialog` as
  recommended, not mandatory, runtime dependencies — both are optional
  fallbacks, not needed for every configuration. macOS needs no equivalent
  package; the Keychain backend and graphical prompt use only what ships with
  the OS.
- No runtime dependency on `op` or `bw` — they're only needed by whoever
  opts into `secret_backend = "1password"` or `"bitwarden"`, and are
  typically packaged and installed separately by the user in that case.

SSHakku's own Gentoo packaging lives in a separate overlay repository (see the
[Installation](../README.md#installation) section of the README), kept in
sync with these dependencies independently of this document.
