# Dependencies

What has to be present on a Linux system to *run* SSHakku, versus what's needed
only to *build* it from source — for end users and for anyone packaging it.

## To build

- **Go 1.25 or newer.** The only build-time requirement; `go build ./...` (or
  `make build`) fetches the Go module dependencies itself
  (`github.com/godbus/dbus/v5`, `github.com/BurntSushi/toml`, `golang.org/x/sys`
  — all pure Go, no C toolchain or cgo involved).

## To run

Always required, regardless of configuration:

- **OpenSSH client tools**: `ssh-add`, `ssh-agent`, `ssh-keygen`. SSHakku
  starts and manages its own `ssh-agent` process and drives `ssh-add`/
  `ssh-keygen` for every key it loads or fingerprints — there is no bundled
  reimplementation of any of these.
- **A login shell that sources `/etc/profile.d`** (or, for a per-user install,
  `~/.bash_profile`/`~/.bash_profile.d/`) — see the
  [Requirements](../README.md#requirements) section of the README.

Required only for the default secret backend (`secret_backend =
"secret-service"`, i.e. no `secret_backend` set at all):

- **A reachable D-Bus session bus** with a Secret Service implementation
  behind it — KDE Wallet, GNOME Keyring, or KeePassXC (via its Secret Service
  integration), whichever the desktop environment already runs. SSHakku talks
  to `org.freedesktop.secrets` directly over D-Bus (no external CLI needed for
  this path).
- **`secret-tool`** (from `libsecret`'s command-line tools), as a fallback
  used only when the D-Bus session bus can't be reached at all (e.g. a
  non-interactive or non-desktop login) — see
  [Where passphrases are stored](CONFIGURATION.md#where-passphrases-are-stored).

Required only when a graphical passphrase prompt is used (a GUI session is
detected — Wayland, or X11 with a live `$DISPLAY` confirmed via `xset`):

- **`kdialog`**, for the passphrase dialog itself. Without it, or without a
  GUI session, SSHakku falls back to a terminal prompt instead (via `ssh-add`
  for an SSH key's own passphrase, or a plain terminal prompt for the
  Bitwarden master password).

Required only when `secret_backend` selects that backend in `config.toml` (see
[Choosing the secret backend](CONFIGURATION.md#choosing-the-secret-backend)):

- **`op`** (the 1Password CLI), for `secret_backend = "1password"`. Must
  already be signed in — SSHakku only runs `op read`/`op item ...`, never `op
  signin`.
- **`bw`** (the Bitwarden CLI), for `secret_backend = "bitwarden"`. No
  sign-in is assumed ahead of time; SSHakku drives `bw login`/`bw unlock`
  itself, prompting for the master password each time it needs the vault.

## For packagers

A distribution package should declare:

- A build-time dependency on the Go toolchain (`>= 1.25`).
- A runtime dependency on `openssh` (for `ssh-add`/`ssh-agent`/`ssh-keygen`).
- `libsecret`'s tools (for `secret-tool`) and `kdialog` as recommended, not
  mandatory, runtime dependencies — both are optional fallbacks, not needed
  for every configuration.
- No runtime dependency on `op` or `bw` — they're only needed by whoever
  opts into `secret_backend = "1password"` or `"bitwarden"`, and are
  typically packaged and installed separately by the user in that case.

SSHakku's own Gentoo packaging lives in a separate overlay repository (see the
[Installation](../README.md#installation) section of the README), kept in
sync with these dependencies independently of this document.
