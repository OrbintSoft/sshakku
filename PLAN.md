# SSHakku — PLAN

Roadmap for the rewrite. We fix the **goals** first; the **phases** come after the
goals are reviewed and agreed. See `CLAUDE.md` for the project rules and
`docs/THREAT-MODEL.md` for the threat model and the June 2026 incident that
motivated the rewrite.

Entries below are kept short by design (rule 2): full investigation detail for
anything marked done lives in `git log -p -- PLAN.md` and the commit that
introduced it, not here.

---

## Goals

Authoritative list of what the rewrite must achieve. "(open: …)" marks a decision
still to be made.

### Core behaviour

1. **SSH always ready on terminal open, without re-typing the passphrase.** The
   original reason the project exists. The stock approach (login init +
   `ssh-askpass`) is rejected as too fragile: it breaks often and each breakage
   costs time to diagnose. This project does not claim to be better in principle,
   but it performs explicit checks, reasons about the problem, and writes a
   detailed log.

2. **Security: the passphrase lives in a secure vault and never transits an
   environment variable** (where it could leak into a log or elsewhere). Only the
   key id is passed around; the passphrase is handed over out of band (today: a
   short-lived `keyctl` entry) and stored in a secret store (today: KDE Wallet).
   Planned extension: the loaded key **expires** after a configurable lifetime
   (e.g. 20 min / 1 h / 4 h), and simply opening a new shell silently re-activates
   it from the vault. (open: expire the *key in the agent* vs the *stored
   passphrase* — intended meaning is key-in-agent expiry, passphrase stays in the
   vault.)

3. **Silent on success.** When everything is fine the script prints nothing to
   stdout/stderr — no spam, no interference with other commands.

4. **Bounded retries, no loops.** It may retry, but after N attempts (say 3) it
   gives up and must not keep spamming in every shell. (open: also limit over time
   / reset at next login; ideally provide an opt-out.)

5. **No SSH keys → no breakage.** With nothing to load, the script must still exit
   cleanly.

6. **Best-effort recovery.** An SSH session already started by something else is
   fine — at most we load the keys that are missing. If a socket is up but the
   environment variables don't match, fix them as far as possible. A healthy agent
   we did not start is adopted (via the fixed-socket symlink) and the anomaly is
   reported — never killed; only dead sockets/agents are reaped. (Note the hard
   limit: a child process cannot rewrite the environment of an already-running
   parent such as the session/GUI; the fixed-socket approach is what makes this
   robust.) See open decision 15 for the full five-state policy.

7. **No database — plain text files only. No secrets or otherwise sensitive
   information in logs.**

### Diagnostics

8. **A diagnostic tool (currently missing).** Reports problems: who started the
   ssh socket, why it isn't working, which processes are involved, etc. It can be
   run with `sudo` to have the privileges needed to inspect the full picture.

### Portability

9. **Work without a graphical environment, and under Wayland** (not only X11).

10. **Primary target: Gentoo Linux with OpenRC and KDE.** It must work here first.

11. **Adaptable to other Linux systems:** Gentoo with systemd; other distributions
    with other desktops such as GNOME and its keyring. The passphrase store must be
    pluggable — beyond KDE Wallet and the GNOME equivalent, support e.g. 1Password.

12. **Secondary target: macOS** (zsh, Keychain or 1Password).

13. **Later: Windows.** First under bash, then PowerShell (open: module vs profile
    vs other). Credential Manager or 1Password.

### Engineering

14. **Move logic out of pure bash into a more maintainable, testable,
    cross-platform language, minimizing duplication.** A lot of shell glue will
    remain, but the core logic should not live in bash. Candidate: Go. (The
    login-shell entrypoint is necessarily a thin shell layer; keep it minimal.)

15. **Highly parametrizable and configurable.**

16. **Maximally testable:** unit tests, plus integration tests in containers at
    least on Linux. macOS/Windows to be decided — Windows containers exist, macOS
    is unclear; possibly Vagrant, otherwise CI runners, or best-effort on macOS.

### Installation & filesystem

17. **Two installation modes.** *System-wide* (requires `sudo`, as today:
    `/etc/profile.d`, `$BINDIR`) **or** *per-user* (no root, everything under
    `$HOME`). The same logic must work in both; only the paths and the bootstrap
    hook differ.

18. **Least-privilege execution.** Executables/scripts run with the privileges of
    the user who opens the terminal — never escalate. The only exception is the
    diagnostic tool (goal 8), which may be run with `sudo` on demand to inspect the
    full picture.

19. **Standard file locations, outside `~/.ssh`.** Config in `/etc/<name>/` (system)
    or `$XDG_CONFIG_HOME` (per-user); logs/state in `$XDG_STATE_HOME`; the agent
    socket in `$XDG_RUNTIME_DIR` (per-user, mode 0700) — all with correct
    ownership/permissions. Never store our own files under `~/.ssh`: it is reserved
    for OpenSSH and, as the June 2026 incident showed, creating `~/.ssh/agent/` is
    precisely what makes OpenSSH 10.x relocate the socket to a random path.

---

## Open decisions

Points raised during goal review that need a decision (or an explicit constraint
honoured) before or during the phases. Each notes the related goal. Entries marked
done are summarised; see the note at the top of this file for full detail.

1. **Threat model (goal 2, 7). Decided (Phase 0)** — see `docs/THREAT-MODEL.md`
   (source of truth). In two lines: *protects* the passphrase from logs, shell
   history, `argv` and plaintext on disk — at rest only in the OS secret store, in
   transit only via a short-lived `keyctl` entry / stdin. Same-user processes,
   root, swap/coredumps and physical access are deferred decisions, settled per
   threat rather than excluded by design.

2. **No passphrase in `argv` (goal 2).** Never pass the passphrase as a
   command-line argument. Feed it through stdin/env instead. Audit every tool
   invocation that touches a passphrase — an invariant every secret backend since
   has followed (`SecretToolBackend`, `SecretServiceBackend`, `OnePasswordBackend`,
   `BitwardenBackend`).

3. **"Silent" means zero stdout/stderr when non-interactive (goal 3).** Anything
   sourced from `profile.d` runs for non-interactive SSH sessions too; a single
   byte on stdout corrupts `scp`/`rsync`/`git`-over-ssh. The success path emits
   nothing on stdout/stderr — only the log file.

4. **Recovery has a hard limit (goal 6).** A child process cannot rewrite the
   environment of an already-running parent. "Fix mismatched env vars" only fixes
   the current shell and its descendants; already-open GUI apps are reached only
   via the fixed socket path (plus a dangling-socket symlink as a last resort).
   Same symlink mechanism adopts a healthy foreign agent (open decision 15).

5. **Give-up state & opt-out (goal 4). Decided** — see Phase 2 slice 4
   (`internal/giveup`, `SSHAKKU_GIVEUP_TTL`/`SSHAKKU_NO_GIVEUP`).

6. **Key-expiry semantics (goal 2). Decided** — expire the *key inside the agent*
   (`ssh-add -t`), keep the passphrase in the vault, and let a new shell re-add it
   silently. See Phase 2 slice 4.

7. **Secret backend abstraction (goal 11). Done for Linux (Phase 4.3).** KDE and
   GNOME are the *same* backend (Secret Service D-Bus). Backends: `secret-service`
   (KDE + GNOME + KeePassXC), 1Password (`op`), Bitwarden (`bw`), plus macOS
   Keychain and Windows Credential Manager still to come. `SecretBackend`
   interface defined early (also makes tests mockable) — all four Linux backends
   are implemented (Phase 4.2) and selectable at runtime via `config.toml`'s
   `secret_backend` key (Phase 4.3, see `docs/CONFIGURATION.md`).

8. **Platform port depth (goals 12, 13). Decided for macOS (2026-07-19): full
   port, not thin.** macOS gets sshakku's own agent management
   (adoption/self-healing/fixed-socket symlink) exactly as on Linux — the
   `internal/agent`/`internal/paths`/`internal/keyring` layers were already
   unix-portable — plus a real `SecretBackend` over Keychain (via
   Security.framework, see decision 23), not a reduction to a bare
   `ssh-add --apple-use-keychain` call. Rationale: consistent semantics and
   expiry behavior across platforms outweighs the extra code, and the extra
   code turned out to be mostly a new `SecretBackend` implementation plus a
   zsh install hook, not new agent-layer work. Shipped across Phase 5 Steps
   1–6. Windows is still deferred and its port depth undecided — remains the
   most divergent target (service + named pipe, no socket) and stays last.

9. **CI vs containers for non-Linux (goal 16).** Use GitHub Actions `macos-*`/
   `windows-*` runners for those platforms; keep Linux containers for the rest.

10. **Phasing (rules 1, 9). Decided:** bash/Go split — Phase 1 ships only the
    permanent shell plumbing; the branchy, stateful logic moves to a Go core grown
    incrementally (strangler), slice by slice, rather than one wholesale rewrite —
    see Phase 2. The diagnostic tool follows the core (Phase 3).

11. **CI least-privilege & lint coverage (rule 14, 12). Decided (Phase 0):**
    `make lint` runs `shellcheck`+`shfmt`+`markdownlint-cli2`+`checkmake`+
    `actionlint`(+`editorconfig-checker`, `golangci-lint`, `taplo`, `hadolint`,
    `zsh -n` as each file type entered the repo); CI declares `permissions:
    contents: read` and invokes the same `make lint`. See the per-file-type
    table under Phase 0.

12. **Install modes & path layout (goals 17–19). ✅ Done (step 1.1 for
    paths, Phase 1.2 for the bootstrap hook):** config **and** the session log live
    under `${XDG_CONFIG_HOME:-~/.config}/sshakku`; the agent socket resolves
    `$XDG_RUNTIME_DIR/sshakku` → `/run/user/$UID/sshakku` →
    `${XDG_CACHE_HOME:-~/.cache}/sshakku`, with an unpredictable per-login
    `@u`-keyring token as a path component (defense in depth above the `0700`
    boundary). Per-user bootstrap hook: `$HOME/.bash_profile.d/` if that
    directory already exists (just drop a file in it, existence is the only
    check — no attempt to confirm it's actually sourced), else a
    marker-delimited block appended to `$HOME/.bash_profile` (created if
    absent) — see Phase 1.2. **Extended (2026-07-19):** `WIRE_BASHRC=1`
    additionally wires the same hook into a non-login shell's startup
    files, opt-in and off by default, using the same drop-in-dir-or-
    delimited-file fallback shape uniformly in all four spots: `make
    install-user` targets `$HOME/.bashrc.d/` or `$HOME/.bashrc`; system-wide
    `make install` targets `/etc/bash/bashrc.d/` or `/etc/bash.bashrc`. The
    marker-block primitives (`strip_block`/`upsert_block`) and the drop-in
    ones live in the new `shell-hook-lib.sh`, shared by `install-user-hook.sh`
    and the Makefile (sourced by the former, invoked as a small standalone
    CLI by the latter) instead of being duplicated.

13. **Which keys to auto-load is configurable (goals 1, 2, 15). ✅ Done.**
    `config.toml`: `auto_load_mode` (`all`/`include`/`exclude`) +
    `auto_load_include`/`auto_load_exclude`, the same shape as decision 18's
    wallet-store keys (`config.Settings.AutoLoads`, `keys.Config.AutoLoad`,
    checked before the fingerprint lookup). An excluded key is simply not
    proactively loaded; the askpass broker still answers for it on demand.

14. **Project name (goal identity). Decided:** **SSHakku** (Akkadian *iššakku*, a
    steward who administers an estate on behalf of its owner). Replaces the
    original `ssh-profile-config` (mislabelled the tool as a `~/.ssh/config`
    manager) and the interim working name *sshepherd* (dropped over a trademark
    clash with FullArmor's SSHepherd®). CLI alias: `shak`.

15. **Agent lifecycle: self-healing & foreign-agent adoption (goals 5, 6, 8).** At
    shell-init the world is in one of five states, resolved in precedence order:

    - **A — clean** (nothing reachable): reap any dead socket at our path, start
      *our* agent on the fixed socket, load the keys.
    - **B — ours healthy** (agent on our fixed socket): attach, load only the
      missing keys (fingerprint dedup), stay silent.
    - **C — ours zombie** (our socket/process dead, including the legacy
      `~/.ssh/agent`): reap what is ours, restart on the fixed socket.
    - **D — foreign healthy** (a reachable agent we did not start): never spawn a
      competitor — adopt it by symlink (fixed socket → foreign socket, keep
      `SSH_AUTH_SOCK` on the fixed path) and **report the anomaly**, accepting the
      widened blast radius as exactly why it's reported, not the steady state.
    - **E — disaster** (mixed stale env, dead sockets, several agents): use any
      healthy agent (ours first), reap the dead, never leave the shell on a dead
      socket.

    "Ours" = the agent on our fixed socket (PID recorded when we start it);
    "legacy-ours" = `ssh-agent -a ~/.ssh/agent/…`; anything else is foreign. Dead
    foreign sockets/agents are reaped too (never a *healthy* one — that's case D),
    within the invoking user's own privileges; deeper cross-user cleanup is the
    diagnostic tool's job under `sudo`. Reporting/attribution of a foreign agent is
    the diagnostic tool's mandate (goal 8, Phase 3). ✅ Implemented as Go slice 2.

17. **Scoped, explicit-lock unlock window per collection (goals 2, 11; open
    decision 7; threat I6). ✅ Done.** sshakku uses its own Secret Service
    collection (label/alias `sshakku`), unlocked only for the seconds around each
    lookup/store rather than relying on the desktop's idle timeout —
    `internal/secretservice` (native D-Bus client, since `secret-tool` can't do
    this) behind `SecretServiceBackend`, falling back to `SecretToolBackend` if
    the session bus is unreachable. Does **not** close threat I6 (a same-UID
    process can still query the collection while unlocked); only shrinks the
    window. `SecretSession` (`Unlock`/`Lock`) lets `Loader.LoadKeys` batch one
    unlock per shell instead of one per key — every later multi-key secret backend
    (`BitwardenBackend`) reuses this same interface with zero changes to
    `load.go`.

18. **Which keys' passphrases are stored in the wallet is configurable (goals 2,
    7; open decision 13). ✅ Done.** `config.toml`: `wallet_store_mode`
    (`all`/`include`/`exclude`) + `wallet_store_include`/`wallet_store_exclude`
    (`config.Settings.StoresWallet`, `keys.Config.WalletStore`), consulted by both
    `Loader.storePassphrase` and `Broker.storePassphrase` before every
    `SecretBackend.Store`. Surfaced a real gap fixed at the same time: the askpass
    broker hadn't been loading `config.toml` at all; `cmd/sshakku` now shares one
    `loadSettings` helper between `load-keys` and the broker.

19. **Command to forget stored passphrases (goals 2, 15). ✅ Done.** `sshakku
    forget <keyname>...` / `--all`. `SecretBackend` gained `Delete`/`List`;
    `SecretServiceBackend.List` enumerates the dedicated collection directly.
    `SecretToolBackend.List` returns `ErrListUnsupported` (`secret-tool` has no
    enumeration verb) — `--all` needs the native backend. Field note:
    `secret-tool clear` was observed to fail silently against a real entry, which
    is why `SecretServiceBackend.Delete` goes through D-Bus `Item.Delete` directly.

20. **Three-tier container/VM test strategy (goal 16; open decision 9). Decided:**
    cost is not a blocker — thoroughness wins.
    1. **Tier 1** — headless, multi-distro containers, no desktop: the fake-backed
       unit/integration suite, run on every push (`test.yml`).
    2. **Tier 2** — a real desktop secret stack (or, for a self-hostable backend,
       the real backend daemon itself) headless via Xvfb/weston, exercising the
       real prompt/unlock flow. `workflow_dispatch`-only (heavier, more brittle).
    3. **Tier 3** — a full Vagrant Gentoo/OpenRC/KDE box doing a real login (SDDM,
       PAM). Deferred to Phase 6; a login/session check, not something a new
       backend needs.

21. **Distribution channel per Linux distro (goal 17; open).** Gentoo already
    works via the maintainer's personal `orbintsoft-ebuild` overlay; eventual
    submission to the community GURU overlay is the intended next step there,
    once the project is stable enough. Debian/Ubuntu/Fedora/openSUSE and
    friends have no channel decided yet — options include a self-hosted APT/RPM
    repo, a Launchpad PPA, Fedora COPR, openSUSE OBS, or Snap/Flatpak's own
    stores — and the project is explicitly **not** ready to submit to any
    distro's official repository yet. Settle when Phase 8 (packaging) is
    reached.

22. **macOS packaging & distribution (goal 12; open).** Raised 2026-07-19,
    settle when Phase 11 starts (after Phase 8's Linux release pipeline is
    solid — Linux is the primary target, goal 10). Open questions:
    - **Codesigning & notarization.** A Developer ID-signed, Apple-notarized
      binary/installer so Gatekeeper doesn't block it — needed for anything
      distributed as a prebuilt binary or `.pkg` (a Homebrew formula that
      builds from source may not need this; a bottled/precompiled one would).
    - **Installer format.** Whether to ship a `.pkg` installer alongside (or
      instead of) the Homebrew path.
    - **Architecture.** Apple Silicon (`arm64`) only, Intel (`amd64`) only, or
      a universal2 fat binary/package — cost/benefit not yet weighed.
    - **Installer configurability.** Whether a `.pkg` can/should expose the
      same install-time options `make install`/`install-user` already do
      (system-wide vs per-user, `WIRE_ZSHRC`, etc.) via installer choices, or
      whether the `.pkg` only ever applies fixed defaults and further
      customization stays CLI/`config.toml`-only, same as today.
    - **Homebrew.** A project-owned custom tap first, to validate the
      formula/cask in the wild; submission to homebrew-core (the public,
      unmaintained-by-us tap) only once proven stable — the same
      own-channel-first-then-upstream shape open decision 21 already uses for
      Gentoo's GURU overlay.

23. **macOS secret backend support (goals 11, 12; open).** Raised
    2026-07-19. Target backend set: Apple Keychain, 1Password, KeePassXC,
    Bitwarden.
    - **Keychain. ✅ Done** — Phase 5 step 2, `internal/keys/secret_keychain_darwin.go`.
    - **1Password / Bitwarden.** Both backends (`internal/keys/secret_onepassword.go`,
      `secret_bitwarden.go`) shell out to the `op`/`bw` CLIs and carry no
      build tag — already OS-portable code, same as `internal/agent` looked
      before Phase 5 step 3 found the real `/proc` gap underneath the
      "no work needed" assumption. Treat as **unverified, not proven** until
      exercised for real on macOS CI — do not assume they just work.
    - **KeePassXC.** On Linux this is *not* its own `SecretBackend` — it's
      reached generically through the `secret-service` backend, because
      KeePassXC implements the freedesktop Secret Service D-Bus API itself
      (open decision 7). macOS has no D-Bus/Secret Service, so that path
      doesn't carry over; KeePassXC support there needs its own design from
      scratch. Candidates to research, not yet decided between: KeePassXC's
      local native-messaging socket protocol (the same one its browser
      extension uses — a defined JSON-over-Unix-socket protocol, no D-Bus
      involved) versus shelling out to `keepassxc-cli` (need to check
      whether it supports a stdin-fed passphrase the way this project's
      argv rule, open decision 2, already requires of every other backend).

24. **Test coverage & reporting infrastructure (goal 16). Decided
    (2026-07-24):** every PR's CI posts/updates a single comment with
    per-OS coverage, test wall-clock time, the slowest tests, and any
    failures; once merged to master, CI commits a coverage badge and a
    markdown report to a dedicated branch (never master itself), linked from
    `README.md`. A maintained `docs/TEST-MATRIX.md` enumerates every
    user-facing case × OS/target/integration/environment/config/install-method
    combination, tracking integration-test coverage and a per-case
    last-main-run badge. See Phase 6.

---

## Phases

High-level roadmap, ordered so each phase leaves the repo committable (rule 9).
Only the *intent* of each phase is fixed here; the detailed sub-steps are written
into the phase when we reach it, and the open decisions above are resolved at the
phase that needs them (not up front).

The ordering follows open decision 10: harden the primary target first (possibly
still in bash), then introduce the Go core, then widen to other backends and OSes.

### Phase 0 — Foundations & repo hygiene ✅ Done

Lint and CI baseline with no behaviour change, the threat model, and contributor
licensing. → goals 16; open decisions 1, 11; rules 12, 14, 16.

- **0.1 — Repo hygiene.** `makefile` → `Makefile`, `.editorconfig`,
  `.gitattributes` (LF everywhere).
- **0.2 — Threat model.** `docs/THREAT-MODEL.md` (STRIDE: assets, trust
  boundaries, threats, derived invariants). Two-line summary in open decision 1.
- **0.3 — `make lint` target.** `lint-sh`/`lint-md`/`lint-make`/`lint-yaml`/
  `lint-editorconfig`, each with its own config file (rule 13); disabled Markdown
  rules MD013/MD029/MD060 (hand-wrapped prose, numbered goals, author-controlled
  table spacing). Lint tools are CI-only, never bundled — no EUPL-1.2 obligations
  (rule 16), a precedent every later lint tool follows.
- **0.4 — CI alignment & least-privilege.** `linting.yml` → `permissions:
  contents: read` + one job running `make lint`; Actions pinned by commit SHA;
  Dependabot enabled for `github-actions`.
- **0.5 — Contributor licensing & CLA.** `CONTRIBUTING.md`/`CLA.md`/`DCO.txt`:
  DCO 1.1 sign-off + acceptance-by-PR of an adapted Harmony HA-CLA-I (CC BY 3.0),
  contributors keep copyright, holder keeps a non-exclusive relicensing right.
  Governing law: EUPL Art. 15 (EU member state where the holder is established;
  Belgium as fallback).
- **0.6 — Contributor DX for sign-off.** `.githooks/prepare-commit-msg` (opt-in,
  `git interpret-trailers`), a rebase recovery recipe in `CONTRIBUTING.md`, a PR
  template. A custom "comment on DCO failure" bot action was rejected — the DCO
  app already links its own remediation, and it would need `pull-requests: write`
  against the least-privilege default.

Per-file-type lint decisions (rule 12), current as of the last file type added:

| File type | Decision |
|---|---|
| Shell — bash (`*.sh`) | `shellcheck` + `shfmt` |
| Shell — macOS (`*.zsh`) | `zsh -n` (syntax-only — no shellcheck/shfmt-equivalent linter exists for zsh) |
| Markdown (`*.md`) | `markdownlint-cli2` (config `.markdownlint-cli2.yaml`) |
| Makefile | `checkmake` (config `checkmake.ini`) |
| YAML / GitHub workflows | `actionlint`; other YAML/INI/JSON has no dedicated linter — `editorconfig-checker` covers charset/EOL/indent/final-newline |
| All committed files | `editorconfig-checker` (config excludes `LICENSE` verbatim, `*.zsh`, and `*.go` — gofmt owns Go formatting) |
| Shell — bats tests (`*.bats`) | Deferred until test files enter the repo |
| Go (`*.go`) | `gofmt -l` + `go vet` + `golangci-lint` (config `.golangci.yml`); `golang.org/x/sys` (BSD-3-Clause) recorded in `COPYRIGHT.md` |
| TOML (`*.toml`) | `taplo lint` + `taplo format --check`; runtime parser `github.com/BurntSushi/toml` (MIT) recorded in `COPYRIGHT.md` |
| Dockerfile (`test/containers/*.Dockerfile`) | `hadolint` (config ignores DL3008 — no viable apt-pin story against a rolling suite; the base image tag is the point-in-time anchor) |

### Phase 1 — Harden the primary target: shell plumbing (still bash) ✅ Done

Gentoo / OpenRC / KDE. Scope narrowed by the bash/Go split (open decision 10):
only the **permanent shell plumbing** stays in bash; the branchy, stateful logic
moved to the Go core in Phase 2 instead (1.3's seam and 1.4's lifecycle both ended
up as Go slices there — see Phase 2). → goals 3, 5, 6, 10, 17–19; open decisions
3, 4, 12.

- **1.1 — XDG path layout, out of `~/.ssh`. ✅ Done.** Scope is SSHakku's own
  files only (config, log, agent socket) — never the user's private keys,
  which stay exactly where OpenSSH creates them, under `~/.ssh`. Delivered as
  part of open decision 12: config and the session log under
  `${XDG_CONFIG_HOME:-~/.config}/sshakku`, the agent socket under
  `$XDG_RUNTIME_DIR/sshakku` (falling back to `/run/user/$UID/sshakku` or
  `${XDG_CACHE_HOME:-~/.cache}/sshakku`). → goal 19; open decision 12.
- **1.2 — Two install modes + bootstrap hook. ✅ Done.** System-wide
  (`make install`/`make uninstall`, `/usr/local/bin`, `/etc/profile.d`,
  needs root) and per-user (`make install-user`/`make uninstall-user`, no
  root): binary at `$HOME/.local/bin/sshakku`; a new `install-user-hook.sh`
  renders the same `nn-ssh-init-linux.sh` hook logic once to
  `$HOME/.local/share/sshakku/shell-hook.sh` (binary path substituted in,
  same `sed` mechanism the system-wide install already uses), so wiring it
  in is always a single `source` line — dropped into
  `$HOME/.bash_profile.d/` if that directory already exists (existence is
  the only check), else idempotently upserted as a marker-delimited block
  (`# >>> sshakku >>> … # <<< sshakku <<<`) into `$HOME/.bash_profile`
  (created if absent), verified byte-for-byte idempotent across repeated
  installs and fully reversible on uninstall. Kept in shell/Make rather
  than a new Go subcommand: this is a one-shot, human-invoked operation,
  not the always-running logic goal 14 targets, so the usual
  move-it-to-Go argument doesn't carry the same weight here. → goals 17,
  18; open decision 12.
- **1.3 — Silent-on-success & shell safety.** Superseded by the Go seam (`eval
  "$(sshakku shell-init)"`); the remaining bash-side work is `set -u` hardening.
- **1.4 — Agent lifecycle & recovery.** Moved into the Go core — see Phase 2 slice
  2.
- **1.5 — Shell test harness (rule 12). ✅ Done.** `bats` + tier-1
  containers (open decision 20): `test/bats/` runs against real
  `ssh-agent`/`ssh-add`, driven from `make test-bats` in the tier-1
  container job (`SSHAKKU_TEST_ALLOW_BATS=1`, an explicit opt-in — the
  suite must never run on a real machine, since it manipulates real
  ssh-agent processes and login-hook plumbing; a first local iteration
  learned this by actually triggering a real system-wide sshakku's
  `kdialog` prompt via `bash -i` sourcing real shell rc files, fixed by
  never using `-i` at all). A stub `secret-tool`
  (`test/bats/fixtures/secret-tool`) stands in for a real Secret Service so
  the vault is reachable without a desktop session. **Rule 12:** no new
  lint tool — `shellcheck` (0.7+) and `shfmt` both parse `.bats`/`.bash`
  natively, so `lint-sh`'s existing `SH_SCRIPTS` glob just grew to include
  them. Original checklist, adapted to what a container with no controlling
  terminal at all can actually drive (a live interactive prompt needs a
  pty this harness doesn't have — that is covered instead by Phase 4.5's
  Go-level headless integration tests):
  1. Fresh login, two terminals: both see the key in `ssh-add -l`, no second
     prompt. ✅ tested via the vault, not a live prompt.
  2. `SSH_AUTH_SOCK` is the fixed socket path everywhere. ✅ tested by
     sourcing the real hook non-interactively.
  3. Kill the agent → a new terminal restarts it at the **same** socket and
     reloads the key. ✅ tested — a direct regression test for Phase 4.5
     (the reload is now silent because the vault is always tried, GUI or
     not).
  4. First-ever run, empty vault: prompts once, silent thereafter. Split:
     empty vault → key not loaded, no hang/crash (✅ tested); "prompts
     once" needs a live tty this harness doesn't have, so it stays a
     Go-level concern.
  5. A reachable-but-empty agent (`ssh-add -l` exit 1) is **healthy**, never
     killed. ✅ tested — adopted, not killed and replaced.

### Phase 2 — Go logic core ✅ Done

Moved the branchy, stateful logic out of bash into a small Go core behind the
thin shell entrypoint, grown incrementally (strangler) rather than one wholesale
rewrite. → goals 1, 2, 4, 9, 14, 16; open decisions 2, 5, 6, 7, 9.

- **Slice 1 — path / token / dir / log core.** `cmd/sshakku` + `internal/paths` +
  `internal/sessionlog`: path resolution, the per-login `@u` keyring token via the
  `keyctl(2)` syscall (no `keyctl` binary), 0700/0600 dir+log creation with a
  symlinked-leaf guard, legacy `~/.ssh/agent` cleanup. `shell-init` prints the
  paths; the bash entrypoint evals them.
- **Slice 2 — agent lifecycle.** The five-state policy (open decision 15) in
  `internal/agent` (probe/inspect/manage/ensure, flock-serialised); `shell-init`
  is the sole owner of the lifecycle.
- **Slice 3 — key loading + `askpass`.** `internal/keys` + `internal/keyring`:
  enumerate `~/.ssh/id_*`, skip fingerprints already in the agent, add the rest
  via the secret store or a prompt, passphrase handed to `ssh-add` out of band via
  the `@u` keyring + an `SSH_ASKPASS` helper. GUI detection covers Wayland and
  X11.
- **Slice 4 — retries / give-up + key-expiry.** Resolves open decisions 5, 6.
  `ssh-add -t` expiry (default 8h, `SSHAKKU_KEY_LIFETIME`); the askpass broker
  refills an expired key silently from the wallet, falling back to `/dev/tty` only
  on a wallet miss. Bounded retries (`SSHAKKU_MAX_ATTEMPTS`); give-up is
  per-login/tmpfs-backed (`SSHAKKU_GIVEUP_TTL`, `SSHAKKU_NO_GIVEUP`).
  `internal/giveup` + `internal/keys`; knobs documented in
  `docs/CONFIGURATION.md`.

### Phase 3 — Diagnostic tool ✅ Done

`sshakku doctor` (`internal/diagnose`, reusing `internal/agent`'s inspection
primitives): a read-only report naming the agent-lifecycle state (A–E, open
decision 15), classifying every `ssh-agent` as ours/legacy/foreign, comparing
`SSH_AUTH_SOCK` against the fixed socket, tailing the session log, and listing
each `~/.ssh` key with its remaining agent TTL. `doctor --fix` re-runs the same
self-heal the login path uses. `doctor --user <name|uid>` diagnoses another
user's session under `sudo` via a kernel-mediated privilege drop
(`exec.Cmd.SysProcAttr.Credential`, never in-process `setuid`); `--fix` is
refused cross-user by design (read-only elevation only). Documented in
`docs/DIAGNOSTICS.md`. → goal 8; threat E1.

A handful of real bugs were found and fixed while building and using this, each
a one-line lesson: `EnsureAgent` mislabelled a dead-ours recovery as "clean" when
the dead process had already been reaped from `/proc`; agent attribution needed a
`/proc/<pid>/cgroup` fallback once Yama `ptrace_scope` was found to block
`/proc/<pid>/environ` even for a same-UID reader; a "expired, will refill" report
could be wrong once *something other than* sshakku refreshed a key, since the
loader's fingerprint dedup then silently skips it instead; and `internal/agent`/
`internal/diagnose` gained real (non-fake) `ssh-agent` integration tests,
which is what caught the `EnsureAgent` bug above in the first place — they need
an isolated PID namespace, so they run in the tier-1 container, not on a live
desktop session. → goal 16; open decisions 15, 20.

### Phase 4 — Configurability & pluggable secret backends ✅ Done

Make the secret store pluggable and the tool highly parametrizable via
`config.toml` under `$XDG_CONFIG_HOME/sshakku/`. Most of the config-file/env
migration landed in Phase 2/3 (open decisions 13, 17–19); what remained was the
pluggable-backend half: implementing every Linux backend (4.2) and making the
choice reachable at runtime (4.3). → goals 11, 15; open decisions 7, 8, 13, 17,
18, 19, 20.

- **4.1 — Container test infrastructure (open decision 20, tiers 1–2).**
  **Tier 1**: `test/containers/debian.Dockerfile`, running the existing suite in
  CI on every push (`test.yml`). Gentoo was evaluated and dropped from the
  matrix — no OpenRC service actually runs in a plain container, so it only added
  a different toolchain/libc, not primary-target coverage. **Tier 2**:
  `desktop-stack.yml` (`workflow_dispatch`-only), starting with a KDE row
  (`ksecretd`/`kwalletd6` via Fedora — Debian doesn't package `ksecretd` — driven
  non-interactively through `pamtester`/`pam_kwallet5.so` and a pre-seeded
  `kwalletrc`). **Tier 2/3 breadth matrix, decided so 4.1/4.2 wouldn't
  re-litigate it per backend:** cover secret backend/desktop session (not
  "desktop environment" — XFCE/LXQt pair with GNOME Keyring or nothing) ×
  display protocol (X11 now, Wayland only where shown to matter) × init system
  (OpenRC has nothing to exercise without a real login, so it stays tier 3 only).
  **Still open:** a tier-summary doc pulling the now-complete tier-1/tier-2 story
  (4 backend rows plus 1Password's service-account alternative) together in one
  place — noted here since it has never been picked up. → open decisions 7, 20;
  goals 15, 16.
- **4.2 — Secret backend survey. ✅ Done — all four Linux backends verified.**
  Candidates, most to least likely to need new code: GNOME Keyring, KeePassXC,
  1Password, Bitwarden.
  - **GNOME Keyring** — same Secret Service API as KDE, but its alias mechanism
    supports only `"default"` (unlike KDE), which a real `gnome-keyring-daemon`
    caught immediately as a hard D-Bus error, not a prompt; `Collection` now
    falls back to a label-based lookup, then an unaliased create. Its only
    non-interactive unlock is a **blank password**, itself unencrypted on disk —
    recorded as threat I11, not yet warned about at creation time. Tier 2 row:
    Debian trixie, one-time keyring-creation dialog driven via Xvfb + `xdotool`.
  - **KeePassXC** — accepts arbitrary D-Bus aliases (unlike GNOME), so the
    existing fast path just works; needed no product-code fix. Architecturally a
    "collection" is an open database tab in the full GUI app (no headless daemon
    mode, no non-interactive re-unlock at all). A Debian-trixie-specific
    segfault in backgrounded `--pw-stdin`/keyfile unlock (confirmed via `strace`,
    absent on Fedora's newer build) forced the tier-2 base image to Fedora.
    `keepassxc-create-collection.sh` runs a persistent watcher answering
    both the one-time creation wizard and every later unlock.
  - **1Password** — `OnePasswordBackend` shells out to `op`; no in-place item
    edit without argv/file exposure, so `Store` deletes and recreates from a
    stdin JSON template. Unlike the other three, a 1Password account is a cloud
    account, not a disposable local daemon, so it has no container tier: a
    dedicated service account ("SSHakku") authenticates in CI via
    `OP_SERVICE_ACCOUNT_TOKEN` (`op user get --me`, not `op whoami`/`op signin`,
    both unsupported for service accounts) — `onepassword-real-account.yml`,
    `workflow_dispatch`-only. A real packaging bug was found and fixed on the
    developer's own machine, unrelated to this repo: 1Password's Linux binaries
    reject a setgid IPC helper group id below 1000, which Gentoo's `acct-group`
    eclass auto-allocates into by default (`OrbintSoft/orbintsoft-ebuild#66`).
  - **Bitwarden** — `BitwardenBackend` shells out to `bw`; unlike `op`, `bw edit
    item <id>` supports a true in-place update via base64-encoded stdin JSON.
    Bitwarden **is** self-hostable (Vaultwarden, AGPL-3.0), so it gets a real
    tier-2 container despite needing no desktop/Xvfb. `bw` has no
    account-registration command (the master-password KDF + RSA keypair
    generation exist only in the web-vault UI, which itself refuses plain HTTP
    even on `localhost`) — the disposable test account was registered once via a
    self-signed-TLS Vaultwarden + headless Playwright, and the resulting
    empty-vault SQLite DB is shipped as a fixture
    (`test/containers/vaultwarden-fixture/`) rather than re-registered
    every run. Unlike the other three backends, `bw` has no official
    non-interactive unlock path at all (only an unofficial third-party wrapper,
    not adopted) — **decided**: `BitwardenBackend` prompts for the master
    password itself, every time, and never caches or stores it (a cached master
    password would unlock every credential the backend holds, well past this
    project's threat model for one SSH key passphrase); it implements
    `SecretSession` so `Loader.LoadKeys` still batches one prompt per shell, the
    same machinery decision 17 already built. Verified end to end against the
    real Vaultwarden container, `Unlock` driven for real via a fixed-answer
    `Prompter`; a real bug (`bw` refuses `config server` while already logged
    in) was found only by that run and fixed.
  → open decisions 7, 8.
- **4.3 — Runtime backend selection. ✅ Done.** `config.toml` gained
  `secret_backend` (`secret-service`/`1password`/`bitwarden`, default
  `secret-service`) plus the per-backend account fields (`onepassword_vault`,
  `bitwarden_email`, `bitwarden_server`) — all four config-file only, same
  reasoning as `wallet_store_mode`. `newSecretBackend` in `cmd/sshakku`
  switches on it instead of hardcoding `SecretServiceBackend`/
  `SecretToolBackend`; Bitwarden's master-password prompt reuses the same
  graphical/terminal split the SSH-key passphrase prompt already uses. Closes
  open decision 7 for every Linux backend. See `docs/CONFIGURATION.md`.
- **4.4 — Modular config: `config.d/`. ✅ Done.** Let settings be split across
  `$XDG_CONFIG_HOME/sshakku/config.d/*.toml` in addition to the single
  `config.toml`. **Decided:** `config.toml` (if present) loads first as the
  base; files under `config.d/` then apply in lexicographic filename order,
  each overriding a key it sets on top of what loaded before it (a `NN-`
  filename prefix, mirroring the existing `NN` convention in
  `nn-ssh-init-linux.sh`/`install-user-hook.sh`, controls the order). Merge is
  per-key, whole-value replacement — the same semantics `env > file > default`
  already uses — not a deep-merge of the include/exclude lists. A malformed
  file under `config.d/` is skipped and logged, without discarding the rest
  (`config.toml` or the other `config.d/` files); an absent `config.d/` is
  not an error, same as an absent `config.toml` today.
- **4.5 — Vault-backed proactive load without a GUI.** `Loader.addWithRetries`
  currently picks between `loadViaVaultThenPrompt` (consult the configured
  secret backend, prompt via `kdialog` on a miss) and `loadInteractive` (skip
  the backend entirely, let `ssh-add` prompt straight on the terminal) purely
  on `Config.GUI` (a reachable Wayland/X session plus `kdialog`) — so an
  interactive headless login never consults the backend at all, even one that
  needs no display or D-Bus (`op`, `bw`). The reactive askpass broker
  (`internal/keys/askpass.go`) already gets this right: it tries the backend
  unconditionally and only falls back to a terminal prompt on a miss, no GUI
  check anywhere. **Decided:**
  - Drop the GUI branch from the vault-usage decision: `addWithRetries`
    always tries the configured backend first. A D-Bus-only backend (Secret
    Service) simply misses when D-Bus is unreachable — recoverable, not
    fatal, exactly like today's handling of a lookup error.
  - **Having no GUI and having no controlling terminal are both perfectly
    normal, expected deployments — never surfaced to the user as an error.**
    A lookup miss because the backend can't be reached here (no D-Bus
    session, no GUI, a backend this environment isn't set up for) is logged
    at `INFO`, not `ERROR` — in both this loader path and the reactive
    askpass broker, which logs the same lookup failure at `ERROR` today
    (`internal/keys/askpass.go`'s `Broker.Answer`) and gets the same
    downgrade for consistency. Likewise, the new terminal prompter failing
    because there is no controlling terminal at all is logged at `INFO` and,
    critically, **never reaches `Notifier.Notify`** (the user-visible
    stderr line) — that channel stays reserved for what the user can
    actually act on: exhausted retries after a wrong passphrase, or
    `ssh-add` rejecting what was entered. A backend that can't be reached at
    all is already diagnosable on demand via `sshakku doctor --test-backend`
    (Phase 9) rather than by notifying on every load.
  - `Config.GUI` now only picks *how* to prompt on a miss: `KDialogPrompter`
    when available, otherwise a new terminal `Prompter` reading `/dev/tty`
    directly (factoring out the echo-disabling logic the reactive broker's
    `ttyPrompter` already has, so neither copy duplicates the raw termios
    calls) — so sshakku captures what was typed and stores it via the
    existing best-effort `storePassphrase`, instead of leaving `ssh-add` to
    own the whole prompt with no way to save it. `AddWithAskpass`
    (keyring-stashed, `SSH_ASKPASS`-driven, detached from any terminal)
    already works identically with or without a GUI, so this unifies onto it
    in both cases. `loadInteractive`/`ExecKeyAdder.AddInteractive` become
    unused once every path can prompt through the new terminal `Prompter`
    and are removed rather than kept as a second fallback.
  - **Must never block waiting for input that cannot come.** Opening
    `/dev/tty` with no controlling terminal fails immediately (`ENXIO`), not
    by hanging — the same guarantee the reactive broker's `ttyPrompter`
    already relies on — so a missing terminal fails the prompt attempt right
    away and the loader treats it as "key not loaded this round" (not
    exhausted, no user-visible notice, per above), never a hang. Verified
    with a test that calls the new `Prompter` with no controlling terminal at
    all (e.g. under `setsid`) and asserts it returns promptly rather than
    blocking.
  - **Verified with integration tests, not unit tests alone** — the same
    `requireRealSSHTools`-style approach `internal/keys/keyadd_ttl_test.go`
    already uses (a real `ssh-agent`/`ssh-add`/`ssh-keygen`, tier 1's
    container), covering: a headless `Config.GUI=false` load that still
    round-trips a passphrase through a real (fake-backed) `SecretBackend`;
    the no-controlling-terminal case actually returning promptly under
    `setsid` rather than merely asserting it in isolation; and a real
    `op`/`bw` CLI-backed round trip staying under `onepassword-real-account.yml`'s
    existing real-account gate rather than `make test`.

  → open decisions 7, 8.

### Phase 5 — Widen the OS targets

macOS as a wide port, never trust Apple; then Windows last as the most divergent target (service + named pipe, no socket, use win32 safe API). → goals 12, 13; open decision 8.

**macOS half ✅ Done** (PRs #77–#84). Full port per open decision 8: sshakku's
own agent runs on macOS exactly as on Linux, backed by a real Keychain
`SecretBackend` (Security.framework via cgo, open decision 23).

- `macos-latest` CI job (build + full test suite), surfacing and fixing two
  real portability gaps (`unix.TCGETS`/`TCSETS` termios constants,
  `t.TempDir()`-derived socket paths exceeding `sun_path`'s length limit on
  Darwin).
- Keychain `SecretBackend` (`internal/keys/secret_keychain_darwin.go`) over
  `SecItemAdd`/`CopyMatching`/`Update`/`Delete` — no shell-out, so the
  passphrase never touches argv (open decision 2).
- Agent management (`internal/agent`) verified on real Darwin CI; found and
  fixed a real gap — `Inspector.Agents()` hard-depended on `/proc`, absent on
  macOS — via a new sysctl-based `platformAgents()`
  (`golang.org/x/sys/unix`, no cgo).
- zsh install hook: `nn-ssh-init-linux.sh` generalized to `nn-ssh-init.sh`
  (shared by both platforms), a `Darwin` branch in the `Makefile`
  (render-to-`$(SHARE_DIR)` + marker-block upsert into `/etc/zprofile`,
  opt-in `/etc/zshrc` via `WIRE_ZSHRC`), the old dead-weight
  `ssh-init-macos.zsh` deleted.
- `doctor`/diagnose macOS specifics: `TPMPresent`/`TPMVersion` generalized to
  `SecureHardwarePresent`/`SecureHardwareKind` (TPM 2.0/1.2 or Secure
  Enclave); FileVault via `fdesetup status`; Secure Enclave via the
  `hw.optional.arm64` sysctl on Apple Silicon (revised after a real-hardware
  false negative — see below), `system_profiler` T1/T2 match on Intel.
- `README.md`/`docs/DEPENDENCIES.md`/`docs/CONFIGURATION.md`/
  `docs/DIAGNOSTICS.md` cover macOS install, `WIRE_ZSHRC`, and the
  `keychain` backend; `doctor --test-backend keychain` bug fixed
  (`validSecretBackendName` had never been updated for it).
- **Real-hardware pass (2026-07-19/20, Apple Silicon MacBook Pro):** caught
  two issues no CI VM could — a stale Secure Enclave detection (`ioreg -c
  AppleSEPManager` no longer matches on macOS 26; replaced with the sysctl
  approach above) and a login-shell gap (`SSH_AUTH_SOCK` unset / no agent
  when the shell under test wasn't a login shell — fixed with a shared,
  OS-specific `loginShellHint` appended to both the askpass and
  `SSH_AUTH_SOCK`-unset findings). Two findings from that same pass are still
  unexplained and need another real-hardware session, not further guessing:
  `doctor` appearing to hang right after `sudo make install` in the same
  shell, and one `ssh-agent` process listed `foreign`/`dead` whose origin
  wasn't confirmed. Real Secure Enclave hardware and a real launchd/
  Terminal.app login-shell chain are exactly what no CI runner in this
  project can provide (open decision 9) — expect macOS spot checks like this
  one to stay a manual step alongside CI, not something CI fully replaces.

→ goals 12, 13; open decisions 2, 8, 9, 23.

### Phase 6 — Test infrastructure, coverage & full test matrix

Extend CI to macOS and Windows runners, build real visibility into what is and
isn't tested, then use that visibility to close the gaps. Tier 2 of open
decision 20 (real-desktop-secret-stack containers) was brought forward to
Phase 4.1; this phase adds tier 3 (the full Vagrant Gentoo/OpenRC/KDE box) as a
manually-triggered CI workflow, once the steps below land. → goal 16; open
decisions 9, 20, 24.

1. **Coverage + test-health report on every PR.** CI computes per-OS
   unit-test coverage (Linux, macOS; Windows once it exists) and posts/updates
   a single PR comment: total coverage per OS, wall-clock test time, a ranked
   list of the slowest tests, and a failure report when something fails.
2. **Post-merge badge + report. ✅ Done.** Once merged to master, CI commits
   a shields.io endpoint badge (JSON, one per OS) and a markdown report to
   the `coverage-reports` branch (orphan history, never `master`);
   `README.md` on master links both. `tools/testreport badge` (PR #95),
   the `publish-coverage-report` CI job + script (PR #96), the README
   badges (PRs #97, #98), and a repository ruleset on `coverage-reports`
   blocking force-push/deletion.
3. **Test case matrix (`docs/TEST-MATRIX.md`).** Enumerates every
   user-facing case × OS/target/integration/environment/config/install-method
   combination; each row tracks whether it's covered by an integration test
   and shows a badge for that case's last main-branch run.
4. **Keep the matrix current** — every new OS/target, integration,
   environment, configuration, or install method gets a matrix row in the
   same change that introduces it (rule 19).
5. **Coverage push.** Once the reporting exists, drive coverage toward
   ~100%: mock what's mockable, optimize slow/redundant tests.
6. **Close every open cell** in the matrix with a real integration test.
7. **Race, goroutine-leak, and memory checks** alongside the existing
   `-race` suite.
8. **Performance/benchmark tests**, tracked over time alongside the coverage
   report.

### Phase 7 — CI review & dependency hardening ✅ Done

A final pass over the whole CI once it spanned every platform and language.

- **Least-privilege & structure.** Every workflow already declared top-level
  `permissions: contents: read` and pinned third-party actions by commit SHA
  (done in Phase 0) — confirmed still true, no gaps found. Added a
  `concurrency` group (cancel-in-progress) to all 4 workflows; pinned
  `go-version`/`node-version` to exact versions instead of `stable`/`lts/*`;
  added `actions/cache` for the native/Go lint tools, keyed on their pinned
  versions; deduplicated the repeated `setup-go` steps into a local composite
  action (`.github/actions/setup-go-env`).
- **Dependency automation.** `dependabot.yml` gained a `gomod` ecosystem entry
  for the 3 runtime deps. The 5 Go-installed lint tools stay hand-pinned by
  full commit hash in `linting.yml`, not moved into `go.mod`'s `tool` block —
  golangci-lint alone would have pulled ~200 transitive dependencies into the
  module's dependency graph, an unwanted licensing/audit surface for a
  dev-only tool never linked into the shipped binary (rule 16).
  markdownlint-cli2/taplo stay hand-pinned npm installs for the same reason —
  npm package versions are immutable, so no manifest is needed for
  reproducibility. shellcheck/hadolint stay hand-pinned native binaries — no
  ecosystem covers those.
- **Per-file-type lint coverage (rule 12).** Added `zsh -n` syntax checking
  (`lint-zsh`) for `ssh-init-macos.zsh`, the one previously-uncovered file
  type found; every other new extension since the table was last updated is
  either tool-owned config or plain/binary fixture data with nothing to lint.
- **Naming cleanup.** Renamed all 18 `test/containers/*-tier2-*` files to
  drop the `tier2` infix, which leaked the internal test-tier label (open
  decision 20) into filenames meant to describe what each file does, not
  which tier runs it.

→ goal 16; open decisions 9, 11, 20; rules 12, 14.

### Phase 8 — Release pipeline

Automate cutting a release once CI is solid across every target. Planned
flow (decided now, detailed steps written when this phase starts):

1. Merge to `master`.
2. Run the full unit test suite.
3. Run the fast integration tests (tier 1).
4. If those pass, run the slow integration tests (tier 2/3) — parallelized
   where it makes sense, since they're independent of each other.
5. If those pass too, tag with an incremented version and cut a release,
   building the various packages.

Two refinements to settle when this phase starts:

- **Change-gated releases.** Steps 1–5 should only actually cut a release when
  the diff since the last tag touches release-relevant files — Go source, the
  shell init scripts — not when only docs (`*.md`) or CI workflow files
  changed; a docs-only commit must not bump the version or publish a package.
- **Package formats.** Survey and build for the most common Linux package
  formats — `.deb` (Debian/Ubuntu), `.rpm` (Fedora/openSUSE), plus a
  distro-agnostic format (Snap or Flatpak, to be picked) — alongside the
  Gentoo ebuild this project already ships by hand. Open decision 21 covers
  *where* each gets published; this item is only about *building* them.

Until this phase starts, tier 2/3 stay manually-triggered
(`workflow_dispatch`) jobs (open decision 20) — not part of any automated
pipeline yet. → open decisions 9, 20, 21.

### Phase 9 — Diagnostics hardening ✅ Done

Extends `sshakku doctor` (Phase 3) with checks for conditions outside
sshakku's own control but that materially weaken its threat model, plus a way
to actually prove a configured secret backend works end to end instead of
only discovering it's broken the first time `ssh` needs it.

- **Environment checks (`diagnose.HostChecks`, `internal/diagnose/
  hostcheck.go`).** Best-effort, read-only, advisory only (doctor reports,
  never configures or refuses to run): disk encryption via `/proc/mounts` +
  `/sys/class/block/*/dm/uuid` LUKS detection (one level of
  LUKS-under-LVM resolved through `slaves/*`); whether `/tmp` is its own
  tmpfs mount and roughly how big; and **TPM presence/version**, detected
  from the bound kernel driver at `/sys/class/tpm/tpm<N>` (never nil — an
  absent device is a determination, not an unknown) rather than any
  `tpm2-tools` dependency. A nil/undetermined field is never guessed.
- `doctor --test-backend [name]` actively exercises the named (or, if
  omitted, the configured) `SecretBackend` end to end — unlock (when the
  backend implements `SecretSession`), store, look up, and delete a
  throwaway probe entry (`sshakku-doctor-probe`, a fresh random value per
  run) — surfacing a clear pass/fail per step instead of a silent
  misconfiguration that only shows up as a broken `ssh` prompt later.
  Refused cross-user (`--user`), same reasoning as `--fix`: it acts on the
  secret store, it doesn't just read. Documented in `docs/DIAGNOSTICS.md`.

→ goal 8; open decisions 1, 7.

### Phase 10 — Documentation pass & Linux hardening guide ✅ Done

A README and `docs/` overhaul aimed at an end user, not a contributor: explain
every feature, every `config.toml` key, and every secret backend in one place
a first-time reader can follow start to finish (today's docs grew
incrementally, phase by phase, and were never reviewed as a whole for a
newcomer). Everything under this phase is Linux-only as written and will need
a revisit once Phase 5 (macOS/Windows) lands.

- **10.1 — README + hardening guide. ✅ Done.** README overhaul (what
  sshakku is, requirements, installation, first run, a links table to every
  guide) and a new `docs/HARDENING.md`: a short key lifetime, not leaving
  the desktop wallet permanently unlocked, full-disk encryption, and a
  properly configured `/tmp` — cross-referencing `doctor`'s environment
  checks (Phase 9) for the ones it can detect itself, rather than
  duplicating the reasoning in `docs/CONFIGURATION.md`/`docs/DIAGNOSTICS.md`.
  Purely user-facing: no roadmap/phase language anywhere in end-user docs.
- **10.2 — CLI & configuration reference. ✅ Done.** New `docs/CLI.md`:
  every subcommand and flag with exit codes, which ones are wired in
  automatically versus meant to be run by hand, cross-referencing
  `docs/DIAGNOSTICS.md` for `doctor`'s full report detail and
  `docs/CONFIGURATION.md` for `forget`'s policy interactions rather than
  duplicating either.
- **10.3 — Dependencies documentation. ✅ Done.** New `docs/DEPENDENCIES.md`:
  what must be present to *run* sshakku (OpenSSH tools always; a D-Bus
  session bus + Secret Service, `secret-tool`, `kdialog`, `op`, `bw`
  conditionally, by backend/feature) versus what's needed only to *build* it
  (the Go toolchain) — plus a packaging-oriented summary of which
  dependencies are mandatory versus recommended-only.
- **10.4 — Developer/contributor documentation. ✅ Done.** New
  `docs/DEVELOPMENT.md`: the package architecture, the shell-init →
  ensure-agent → load-keys → askpass flow, building and running the unit
  tests, the exact commands to run the tier-1/tier-2 container test suite
  locally (no `make` target covered this before), the required lint tools,
  and a recommended dev environment (Docker, the lint tools, VS Code).
  Linked from `CONTRIBUTING.md`.

→ goals 2, 8, 11, 14, 15, 16; open decision 1.

### Phase 11 — macOS packaging & distribution

Starts after Phase 8's Linux release pipeline is solid — Linux stays the
primary target (goal 10), macOS the secondary one (goal 12). Covers
codesigning/notarization, installer format, architecture, installer
configurability, and the Homebrew tap-then-public-tap path (open decision
22), plus finishing out the secret backend set beyond Keychain — verifying
1Password/Bitwarden for real on macOS and designing KeePassXC support from
scratch, since it has no Secret-Service-equivalent path there (open decision
23). Detailed steps written when this phase starts.

→ goal 12; open decisions 22, 23.
