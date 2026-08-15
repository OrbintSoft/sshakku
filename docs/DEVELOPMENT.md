# Development

Architecture, code layout, and how to build, test, and lint SSHakku — for
anyone working on the code itself. For licensing, sign-off, and how to send a
change, see [CONTRIBUTING.md](../CONTRIBUTING.md); for what SSHakku does and
how to install it, see the [README](../README.md).

## Architecture

The binary is `cmd/sshakku`; everything else lives under `internal/` and is
composed by it. One line each:

| Package | Responsibility |
| --- | --- |
| `cmd/sshakku` | The binary's entry point, and nothing else: the process's single `os.Exit` around `cli.Main`. |
| `internal/cli` | The command itself: subcommand dispatch, the askpass re-entry path when the binary is run under its askpass name, and the wiring that hands each command the system seams it needs. A package rather than `main` so all of it can be exercised from tests. |
| `internal/cli/backend` | Opens the wallet the settings select, per platform, and picks KeePassXC's route (its local protocol, the Secret Service, or its CLI). |
| `internal/cli/crossuser` | Reads another user's per-login socket token for `doctor --user`, by re-executing the binary under that user's credentials — a kernel keyring is only visible to the uid that owns it. |
| `internal/cli/dialog` | Decides where this machine can ask a person for a passphrase: the graphical dialog this platform can raise, or the controlling terminal. |
| `internal/cli/shell` | The shell forms the eval-able commands print in: the Bourne form every POSIX shell reads, and PowerShell's. |
| `internal/cli/walletcheck` | Describes the configured wallet as this machine actually is — which one would be opened, how it would be reached, what of that is present — for `doctor` to report and `--fix` to act on. |
| `internal/agent` | Tends the user's `ssh-agent`: starts one on the fixed socket, reaps dead agents/sockets, adopts one already running. Never reimplements `ssh-agent` itself. |
| `internal/agent/reach` | Answers whether an ssh-agent answers, by speaking its wire protocol on its socket the way `ssh-add -l` does — a socket file being there says nothing about what is still behind it. |
| `internal/agent/inspect` | Finds the `ssh-agent` processes running on this machine and says which is which: the one on the socket SSHakku pins, one left over from before SSHakku, or someone else's. Only the process list is platform-specific — Linux reads procfs, macOS asks sysctl. |
| `internal/agent/inspect/inspecttest` | Builds the procfs-shaped trees `inspect` reads, so a test decides what was running instead of asking the machine it runs on — and runs on a system that has no `/proc`. |
| `internal/config` | Resolves settings: environment variable, then the TOML config file, then a built-in default, per setting. Reads `config.toml` and the `config.d/` drop-ins as an ordered list of sources, so what resolved a value can be reported alongside the value. |
| `internal/diagnose` | Builds the read-only picture `sshakku doctor` reports: which agents are running, which is ours, whether it answers, whether the shell's `SSH_AUTH_SOCK` is wired up. Never starts, signals, or reaps anything. |
| `internal/diagnose/hostcheck` | Reads what the machine does for a passphrase that SSHakku cannot: whether the disk a wallet is written to is encrypted, whether `/tmp` is in memory, whether there is a TPM or Secure Enclave. Best-effort and read-only; "could not tell" is one of the answers. |
| `internal/diagnose/launcher` | Works out what started an ssh-agent — a desktop session, an SSH login, a service unit — by walking the process tree upward, falling back to the control group when a double-forked daemon has left no ancestor to walk to. |
| `internal/giveup` | Records, per key, that loading was abandoned after the bounded retries, so later shells skip it instead of re-prompting every time, until a TTL expires. |
| `internal/keyring` | Wraps the Linux kernel keyring (`@u` user keyring), used on Linux for handing a passphrase from `load-keys` to the askpass re-entry without it touching argv or a file; Darwin uses a private Unix socket instead (`internal/keys/handoff`). |
| `internal/keys` | Loads SSH keys into the agent: enumerates the key directory, skips keys already loaded, pulls each passphrase from the configured wallet, and drives `ssh-add` out of band. The askpass broker, which answers `ssh` when a key has expired from the agent, lives here too. |
| `internal/keys/handoff` | Carries one passphrase from the loader to the SSH_ASKPASS helper `ssh-add` runs, without it passing through argv, the environment or a file: only a token crosses, and the passphrase itself stays in a kernel keyring (Linux) or a socket buffer (Darwin). |
| `internal/keys/prompt` | Asks the user for a passphrase, and works out where it can be asked at all: the pinentry/zenity/kdialog/osascript dialogs, the terminal fallback, and the graphical-session detection that decides between them. |
| `internal/keys/wallet` | Stores and retrieves a key's passphrase: the pluggable backends (Secret Service, `secret-tool`, the macOS Keychain, 1Password, Bitwarden) behind one `wallet.Backend`, plus how SSHakku names its own entries in a store it shares with everything else. |
| `internal/keys/wallet/keepassxc` | The KeePassXC backend, in the two shapes it comes in: `Native`, which talks to a running KeePassXC over its local protocol, and `CLI`, which works on the database file through `keepassxc-cli`. Holds the association store nothing else needs — the identity KeePassXC recognises this client by. |
| `internal/keys/wallet/keepassxc/wire` | Speaks the local protocol KeePassXC exposes for its browser extension — JSON over a unix socket, every message sealed with NaCl box — and knows where that socket is on each platform. Talks only to a KeePassXC already running and unlocked; it asks the user for nothing. |
| `internal/keystate` | Records when a key was added to the agent and for how long, so `doctor` can report remaining lifetime without relying on the ssh-agent protocol (which has no such query). |
| `internal/paths` | Computes and creates the per-user runtime layout: config under the XDG config dir, the session log under the XDG state dir, the agent socket in per-user tmpfs — always outside `~/.ssh`. |
| `internal/run` | Runs the external programs SSHakku drives — `ssh-add`, `ssh-keygen`, a wallet's CLI, a passphrase dialog — and bounds how long anything outside the process may keep a caller waiting, so a tool that neither answers nor fails becomes an error to fall back from. `internal/run/runtest` holds the stand-ins that let a component driving one of those be tested without it. |
| `internal/secretservice` | A native client for the freedesktop Secret Service D-Bus API (`org.freedesktop.secrets`), used instead of shelling out to `secret-tool` so a dedicated collection can be created and locked/unlocked around a single lookup or store. |
| `internal/testtmp` | Hands a test a temporary directory short enough to bind a unix socket in — what `t.TempDir()` cannot give, since a socket address is capped at barely a hundred bytes and macOS's temp root spends most of them before the socket is named. |
| `internal/sessionlog` | Appends timestamped, level-tagged lines to the owner-only session log, bounded to a fixed number of recent lines. |

`tools/` holds CI-only helpers built and run by workflows, never by
`make build` or `cmd/sshakku` — e.g. `tools/testreport`, which turns a `go
test -json` stream into the coverage/timing/failure summary behind the
per-PR test-health comment.

## How the pieces fit together

`internal/cli`'s `run()` dispatches on `args[0]`: `shell-init`,
`ensure-agent`, `load-keys`, `askpass-env`, `doctor`, `forget`, `help`. See
[docs/CLI.md](CLI.md) for the full command reference.

The end-to-end flow, as wired up by `nn-ssh-init.sh` (installed to
`/etc/profile.d` on Linux, sourced from `/etc/zprofile` on macOS) in every
login shell:

1. `eval "$(sshakku shell-init)"` resolves the runtime paths and gets a
   healthy `ssh-agent` on a fixed socket (starting, reaping, or adopting one
   as needed), then prints `agent_sock`/`log_file` shell assignments. The
   shell exports `SSH_AUTH_SOCK` to that fixed socket, so it never goes
   stale even if the agent restarts.
2. `eval "$(sshakku askpass-env)"` runs in every login shell, interactive or
   not: it exports `SSH_ASKPASS`/`SSH_ASKPASS_REQUIRE` pointed at the
   `sshakku` binary itself, so any later `ssh`/`git`/`scp` that needs a
   passphrase is routed through it instead of prompting on the terminal
   directly.
3. `sshakku load-keys` runs only in interactive shells (loading may prompt
   and write to the terminal): it enumerates the key directory, skips keys already in
   the agent, and calls `ssh-add` for the rest with itself set as the
   askpass helper.
4. When `ssh-add` (or a later `ssh`) needs a passphrase, it execs the
   `sshakku` binary as `SSH_ASKPASS`. This re-enters `main()`, which
   recognizes the askpass invocation and answers either from the kernel
   keyring (a passphrase `load-keys` just fetched and stashed for this
   handoff) or from the configured secret backend, falling back to a
   terminal prompt only if both are unavailable.

See [docs/CONFIGURATION.md](CONFIGURATION.md) for every setting this flow
reads, and [docs/DEPENDENCIES.md](DEPENDENCIES.md) for what each backend
needs installed.

## Building and running the unit tests

```sh
make build   # go build -o bin/sshakku ./cmd/sshakku
make test    # go test -race ./...
```

`make test` is what CI runs on every push, on a plain Linux runner (with
`dbus-daemon` installed, since some `internal/secretservice`/`internal/keys`
tests talk to a real D-Bus session bus).

## Running the container test suite

There is no dedicated `make` target for this yet — the exact commands below
are what CI itself runs (`.github/workflows/test.yml` and
`desktop-stack.yml`), reproduced here for running locally. All of them need
plain `docker` (build and run); no `docker compose` is used anywhere in this
repository.

**The container suite** — headless, no desktop, runs automatically in CI on
every push:

```sh
docker build -f test/containers/debian.Dockerfile -t sshakku-test-debian .
docker run --init --rm -v "$PWD":/src:ro -w /src sshakku-test-debian make test
```

**The desktop-stack suite** — a real desktop secret store, one Dockerfile
per backend, run only on demand (`workflow_dispatch` in CI, not on every
push, since each one drives a full desktop stack headlessly and takes
noticeably longer):

```sh
docker build -f test/containers/kde.Dockerfile -t sshakku-test-desktop-stack-kde .
docker run --init --rm -v "$PWD":/src:ro sshakku-test-desktop-stack-kde make test
```

Swap `kde` for `gnome-keyring`, `keepassxc`, or `vaultwarden` for the other
backends. Each Dockerfile's header comment
explains why that particular distro/version was chosen (e.g. KDE's
`ksecretd` isn't packaged on Debian; Debian's KeePassXC 2.7.10 segfaults on a
backgrounded unlock where Fedora's 2.7.12 doesn't).

1Password's real-account coverage (`onepassword-real-account.yml`) is not
container-based: it runs `go test -run OnePasswordBackendRealAccount
./internal/keys/...` directly against a real 1Password service account
token, so it isn't part of `make test` and needs a token you provide
yourself to reproduce locally.

## Linting

```sh
make lint
```

runs `lint-sh` (shellcheck, shfmt), `lint-md` (markdownlint-cli2), `lint-toml`
(taplo), `lint-make` (checkmake), `lint-yaml` (actionlint), `lint-editorconfig`
(editorconfig-checker), `lint-go` (golangci-lint, for both formatting and
analysis, plus `go vet`), and
`lint-docker` (hadolint) — see the `Makefile` for the exact invocation of
each. Every tool must already be on `PATH`; `make lint` does not install
anything itself. `.github/workflows/linting.yml` installs pinned versions of
all eight before running the same target — check there for the versions this
project currently lints against.

`lint-go` analyses each build separately — Linux, macOS, and the two
failure-injection tags — because golangci-lint only ever looks at one. You can
run all of them from either machine: no macOS host is needed to lint the macOS
files, and a file that only compiles under a build tag is analysed only if that
build is in the list.

## Recommended dev environment

- **Docker**, to run the container test suite above — the plain container
  image is the closest thing to what CI actually checks on every push.
- **The lint tools listed above**, so `make lint` catches what CI would
  before you push.
- **VS Code** is the recommended editor. The repository ships shared
  formatting rules in `.vscode/settings.json` (trailing whitespace, final
  newline, LF line endings) that any editor reading that file picks up
  automatically.
