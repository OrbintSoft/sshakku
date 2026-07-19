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
| `cmd/sshakku` | The single binary: subcommand dispatch, and the askpass re-entry path when invoked as `SSH_ASKPASS`. |
| `internal/agent` | Tends the user's `ssh-agent`: probes a socket, starts one on the fixed socket, reaps dead agents/sockets, adopts one already running. Never reimplements `ssh-agent` itself. |
| `internal/config` | Resolves settings: environment variable, then the TOML config file, then a built-in default, per setting. |
| `internal/diagnose` | Builds the read-only picture `sshakku doctor` reports: which agents are running, which is ours, whether it answers, whether the shell's `SSH_AUTH_SOCK` is wired up. Never starts, signals, or reaps anything. |
| `internal/giveup` | Records, per key, that loading was abandoned after the bounded retries, so later shells skip it instead of re-prompting every time, until a TTL expires. |
| `internal/keyring` | Wraps the Linux kernel keyring (`@u` user keyring) for handing a passphrase from `load-keys` to the askpass re-entry without it touching argv or a file. |
| `internal/keys` | Loads SSH keys into the agent: enumerates `~/.ssh`, skips keys already loaded, pulls each passphrase from the configured secret backend, and drives `ssh-add` out of band. The pluggable `SecretBackend`s (Secret Service, `secret-tool`, 1Password, Bitwarden) and the askpass broker live here. |
| `internal/keystate` | Records when a key was added to the agent and for how long, so `doctor` can report remaining lifetime without relying on the ssh-agent protocol (which has no such query). |
| `internal/paths` | Computes and creates the per-user runtime layout: config under the XDG config dir, the session log under the XDG state dir, the agent socket in per-user tmpfs — always outside `~/.ssh`. |
| `internal/secretservice` | A native client for the freedesktop Secret Service D-Bus API (`org.freedesktop.secrets`), used instead of shelling out to `secret-tool` so a dedicated collection can be created and locked/unlocked around a single lookup or store. |
| `internal/sessionlog` | Appends timestamped, level-tagged lines to the owner-only session log, bounded to a fixed number of recent lines. |

## How the pieces fit together

`cmd/sshakku/main.go`'s `run()` dispatches on `args[0]`: `shell-init`,
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
   and write to the terminal): it enumerates `~/.ssh`, skips keys already in
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
(editorconfig-checker), `lint-go` (`gofmt`, `go vet`, golangci-lint), and
`lint-docker` (hadolint) — see the `Makefile` for the exact invocation of
each. Every tool must already be on `PATH`; `make lint` does not install
anything itself. `.github/workflows/linting.yml` installs pinned versions of
all eight before running the same target — check there for the versions this
project currently lints against.

## Recommended dev environment

- **Docker**, to run the container test suite above — the plain container
  image is the closest thing to what CI actually checks on every push.
- **The lint tools listed above**, so `make lint` catches what CI would
  before you push.
- **VS Code** is the recommended editor. The repository ships shared
  formatting rules in `.vscode/settings.json` (trailing whitespace, final
  newline, LF line endings) that any editor reading that file picks up
  automatically.
