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

23. **macOS secret backend support (goals 11, 12; KeePassXC route decided
    2026-07-31, the rest open).** Raised 2026-07-19. Target backend set: Apple
    Keychain, 1Password, KeePassXC, Bitwarden.
    - **Keychain. ✅ Done** — Phase 5 step 2, `internal/keys/secret_keychain_darwin.go`.
    - **1Password / Bitwarden.** Both backends (`internal/keys/secret_onepassword.go`,
      `secret_bitwarden.go`) shell out to the `op`/`bw` CLIs and carry no
      build tag — already OS-portable code, same as `internal/agent` looked
      before Phase 5 step 3 found the real `/proc` gap underneath the
      "no work needed" assumption. Treat as **unverified, not proven** until
      exercised for real on macOS CI — do not assume they just work.
    - **KeePassXC. ✅ Design decided 2026-07-31.** On Linux this is *not* its
      own `SecretBackend` — it's reached generically through the
      `secret-service` backend, because KeePassXC implements the freedesktop
      Secret Service D-Bus API itself (open decision 7). macOS has no
      D-Bus/Secret Service, so that path doesn't carry over.

      **The user names the wallet, not the mechanism:** `secret_backend =
      "keepassxc"` is valid on every platform, and the route is resolved per-OS
      (Secret Service on Linux, native messaging on macOS). The mechanism stays
      **configurable** — a separate route setting takes `secret-service`,
      `native` or `cli`, and a pinned route is used *and no other*: configuring
      `cli` means `keepassxc-cli` is used directly, not as a fallback behind an
      attempted socket connection, and an unavailable pinned route reports which
      one failed rather than silently switching. Only the unpinned default falls
      back. Features F22 (name the wallet) and F23 (pin the route).

      **The routes are not tied to an OS — only the defaults are.** A Linux
      user who does not want the Secret Service can pin `native` or `cli` and
      bypass it entirely; both work there (the socket lives under
      `XDG_RUNTIME_DIR`, and the CLI is the same program). The single
      exception is `secret-service`, which cannot exist on macOS because the
      API does not. So route availability is a property of the route, not of
      the platform, and each one is implemented and tested once rather than
      per-OS.

      **Primary route: the native-messaging socket protocol** — the one the
      browser extension uses; JSON over a unix socket, encrypted with NaCl box
      (X25519 + XSalsa20-Poly1305), reaching the running, already-unlocked
      instance. Chosen because it has the *same preconditions as the Linux
      route* (app running and unlocked), so F5 and F6 keep their silent refill
      on both platforms. Non-browser precedent: `git-credential-keepassxc`.
      Costs, accepted knowingly: the protocol is URL-keyed and has no
      arbitrary named-secret verb, so entries need a synthetic URL; the user
      must enable "Browser Integration" and approve a one-time association; and
      the association key sits in plaintext, so a same-user process could
      replay it — no new threat class, since `docs/THREAT-MODEL.md` already
      lists same-user processes as *currently trusted*, but it is documented
      rather than left implicit.

      **Fallback route: `keepassxc-cli`,** which works on the database file
      with no app running. Rejected as the *primary* route because it needs the
      master password on every call, which would cost F5/F6 on macOS only, and
      because it has **no documented** non-interactive password input (no
      `--pw-stdin`; upstream #1297 and #11068 are open) — so the argv rule
      (open decision 2) would rest on unspecified behaviour, to be probed at
      run time rather than assumed. Its non-silent behaviour is its own
      promise, F24.

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
   blocking force-push/deletion. Follow-up: a richer, Mochawesome-style HTML
   test report per OS via `gotestsum`+`gopogh` (PR #101), published to
   `coverage-reports` alongside the badges (PR #102), a per-package coverage
   breakdown, report link, and CI status badge in the PR comment/README
   (PR #103), and both HTML reports linked from `report.md`/the PR comment
   via the already-live GitHub Pages site for that branch (PR #104).
3. **Test case matrix (`docs/TEST-MATRIX.md`).** Enumerates every
   user-facing case × OS/target/integration/environment/config/install-method
   combination; each row tracks whether it's covered by an integration test
   and shows a badge for that case's last main-branch run.
4. **Keep the matrix current** — every new OS/target, integration,
   environment, configuration, or install method gets a matrix row in the
   same change that introduces it (rule 19).
5. **Coverage push.** Once the reporting exists, drive coverage toward
   ~100%: mock what's mockable, optimize slow/redundant tests.
   Condition/branch coverage via `gobco` (BSD-2-Clause, CI-only, JSON
   `-stats` output) was evaluated and accepted in principle, but deferred to
   *after* this line-coverage push: a stricter metric only pays off once line
   coverage is already high, gobco ignores `select` statements (a real blind
   spot in this concurrency-heavy code), and it runs per-package (needs a
   sweep driver + a second instrumented test run). Revisit once line coverage
   is near the target.
   Genuinely untestable code (the process entry point's lone `os.Exit`,
   platform stubs) is excluded from the reported percentage via
   `go-ignore-cov` (MIT, CI-only, pinned by commit), wired into `make
   test-json`: it marks `//coverage:ignore`-tagged blocks as covered so the
   number reflects real testable coverage. See rule 20 for when the tag is
   allowed.
6. **Close every open cell** in the matrix with a real integration test.
   **In progress.** The macOS Keychain cell is closed:
   `TestDarwinKeychainClientRealRoundTrip` drives a live
   `Add`/`Find`/`Update`/`Delete`/`List` round trip (plus the duplicate-add
   and update-missing error paths) through `DarwinKeychainClient` in the
   `test-macos` CI job, against a throwaway default keychain the job stands up
   first (`test/macos-keychain-setup.sh`) so the runner's login keychain is
   never touched. The ASan/MSan pass over the cgo keychain path (deferred here
   from item 7) rides on top of this test as a follow-up. The Linux
   install/uninstall cells are closed too: `test/linux-install-smoke.sh` stages
   `make install`/`uninstall`, `install-user`/`uninstall-user`, the opt-in
   non-login `WIRE_BASHRC` wiring (both the `bashrc.d` drop-in and the
   fallback-file shape), and the per-user `PATH` wiring under a scratch
   `DESTDIR`/`USER_HOME`. The graceful-stop agent cell is closed by
   `TestEnsureAgentRealGracefulStopRemovesSocket` (a SIGTERM'd agent unlinks its
   own socket, so the next `EnsureAgent` is a clean start, not a zombie reap),
   running in the isolated container/macOS suites alongside the other
   `TestEnsureAgentReal*` states. The two live-terminal prompt cells are
   closed on both platforms by `TestLoadKeysFirstTimePromptRealTerminal` and
   `TestLoadKeysWrongPassphraseRealTerminal`: the loader runs in a child
   process holding a real pseudo-terminal as its controlling terminal — the
   only way a process can obtain one, and what the unit tests substitute a
   socketpair plus stubbed termios ioctls for — while the test plays the user
   on the other end. They also settled the open question about the stale
   `test/bats/shell-plumbing.bats` comment that claimed the case was already
   covered at the Go level. Other open cells in `docs/TEST-MATRIX.md` remain.

   Integration tests are heavier than the unit suite, so each lives in its own
   workflow that runs on `workflow_dispatch` or on a push whose head commit
   opts in with `[integration]` in its message, never on every PR. The
   convention spans every integration workflow — `install-methods.yml` (this
   smoke), `desktop-stack.yml`, and `onepassword-real-account.yml` — so one
   marked push exercises the whole integration suite. The push-marker trigger
   is also what lets a brand-new such workflow be exercised before it reaches
   `master` (a `workflow_dispatch`-only workflow isn't dispatchable until it's
   on the default branch). When the release pipeline lands (Phase 8) these
   become a release gate; that will likely add `workflow_call` so release can
   depend on them.
7. **Race, goroutine-leak, and memory checks** alongside the existing
   `-race` suite. **Partly done.** Race: the whole suite already runs under
   `-race` (`make test` / `make test-json`). Goroutine leaks: every package
   with goroutines runs under `go.uber.org/goleak` (MIT) in a `TestMain`, so
   one left running past a package's tests fails the build; a
   `*GoroutineLeak*` test additionally drives Go 1.26's experimental
   goroutine-leak profiler (`GOEXPERIMENT=goroutineleakprofile`,
   `make test-leakprofile`, wired into the Linux CI job) to catch a goroutine
   blocked *forever* — a class the running-count check can't see. It skips when
   the profiler isn't compiled in, so it's inert in the normal suite. Memory:
   the race detector already covers Go memory-safety; ASan/MSan add value only
   on the cgo path (the darwin keychain), so they ride with the live-keychain
   integration test (item 6) rather than the pure-Go suite.
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

- **Configurable Keychain access policy (ACL).** The `KeychainBackend` sets an
  explicit access control on every item it writes, chosen by a `config.toml`
  key (e.g. `macos_keychain_access`): `touchid-always` (**default** — maximum
  security: each passphrase release requires Touch ID or the login password,
  and stays deterministic even for an ad-hoc-signed binary), `touchid-once`
  (authorize once, then silent for sshakku), or `silent` (no prompt when
  sshakku reads its own items). Today `Add()` sets no access control, so the
  prompt behaviour is left to the default ACL and drifts with the binary's
  code-signing identity — which is why the same tool sometimes prompts for
  Touch ID and sometimes doesn't. `silent`/`touchid-once` are only
  deterministic with a stable Developer ID signature, tying this to the
  codesigning work above. macOS-only (the key is inert off Darwin); the chosen
  policy is documented in `docs/CONFIGURATION.md`.

→ goal 12; open decisions 22, 23.

### Phase 12 — Configuration visibility & editing ✅ Done

Make the effective configuration and the active secret backend legible from the
CLI, so a user can see what sshakku is actually doing without reading source or
guessing which `config.toml` key won.

- **`doctor` reports the active secret backend. ✅ Done** with the wallet
  section (F25) and the environment section (F31): the report names the wallet
  in use and how it would be reached, not only whether `--test-backend` can
  exercise it.
- **`sshakku config` prints the effective configuration. ✅ Done (F35).** Every
  setting with the value in force and *what decided it* — the built-in default,
  an environment variable, `config.toml`, or the `config.d` drop-in that
  overruled the rest — plus the files read in the order they were applied, and
  any value SSHakku refused, which until then reached only the session log.
- **`sshakku config --edit` ✅ Done (F36).** Opens `config.toml` in `$EDITOR`
  (then `$VISUAL`, then `vi`), creating it from an embedded commented template
  if absent, and on exit names what would otherwise have surfaced at the next
  login: a file that no longer parses, a value that was refused, a key a
  drop-in or a variable decides instead.

**Decided (2026-08-03), closing the two open points above.** Flags, not verbs:
plain `config` reports and `--edit` acts, the shape `doctor`/`doctor --fix`
already has, and nothing else in this CLI has a verb pair. Provenance is
reported per key and was the reason to build it at all — with `config.d` merged
in filename order, "which value won" is not something a person can work out by
reading their own files.

Two decisions worth keeping visible: show merges everything a login shell
merges while edit opens `config.toml` alone, so an edit can be a no-op and the
overrule notice exists to say so; and `loadSettings` was rebuilt on the same
ordered source list the report prints from, since a report that reads the
configuration its own way describes something no other command acts on.

→ goals 8, 11, 15; open decisions 7, 12.

### Phase 13 — Cross-platform GUI passphrase prompt ✅ Done

Today the only graphical passphrase prompter is `KDialogPrompter` (kdialog,
KDE). On GNOME without kdialog, and on macOS always, the key-passphrase prompt
falls back to the terminal, so the graphical path is effectively KDE-only. Make
the graphical prompt work across desktops without hard-depending on any single
tool, always keeping the terminal as the floor. Detailed steps written when the
phase starts.

- **Prompter chain.** Linux: `pinentry` (Assuan) → `TTYPrompter`. macOS:
  `osascript` → `TTYPrompter` (a Darwin-only file, not compiled elsewhere).
  `pinentry`/`osascript` auto-select the desktop-appropriate frontend, so no
  single toolkit is presumed.
- **Config override.** `config.toml` `gui_prompter = auto | pinentry | kdialog |
  zenity | none`: `auto` (default) = pinentry on Linux, osascript on macOS;
  `none` = never use a GUI; `kdialog`/`zenity` = explicit override (still gated
  on a graphical session).
- **Fallback invariants.** No graphical session → **always** `TTYPrompter`,
  whatever the config says. Graphical session present but the GUI prompter
  cannot run (binary absent, D-Bus/Assuan error) → **TTY fallback**. A user who
  *cancels* the dialog is not a fallback: the cancel propagates
  (`ErrPromptCanceled`), and the loader gives up on that key without a terminal
  retry.
- **No `$SSH_ASKPASS` delegation.** sshakku *is* the askpass broker (it exports
  `SSH_ASKPASS=self`), so a prompter that consulted `$SSH_ASKPASS` to find "the
  system helper" would point back at sshakku. That approach is deliberately
  rejected.
- **Note.** The broker gate this phase used to also cover was separated out and
  fixed on its own (see "Un-gate the askpass broker" below): wiring the wallet
  broker never depended on a graphical prompter, so it did not belong here.
- **Licence (rule 16).** pinentry (GPL), kdialog (GPL), zenity, osascript are
  invoked as separate processes at runtime, never linked or embedded, so their
  licences do not affect EUPL compatibility or relicensing; recorded as
  runtime-invoked tools.

**Done (2026-08-04).** The chain is pinentry → kdialog → zenity → terminal, not
pinentry → terminal: dropping straight to the terminal where kdialog is
installed and pinentry is not would have taken the dialog away from the one
desktop that already had it. `gui_prompter` accepts what each platform can
actually draw with, per-platform tables read by a platform-neutral rule, and a
name that cannot mean anything there is refused like any other bad value.
Verified with the real binary in a disposable image with a screen and no KDE
tooling, against the previous build for comparison: no dialog appeared there at
any point, a dialog titled SSHakku appears now.

→ feature F29, F37; goals 11, 15; open decision 7.

### Phase 14 — Un-gate the askpass broker ✅ Done

Split out of Phase 13, which is about graphical dialogs: this is not. Reading
the wallet needs no display, so gating the `SSH_ASKPASS` wiring on a graphical
prompter left the reactive refill (F6) dead on macOS and on every kdialog-less
desktop — the promise held only where a GUI happened to exist.

- **Drop the gate** from `askpassEnv`, and with it the same gate on doctor's
  askpass finding, which kept the report silent about the failure precisely
  where it happened.
- **`SSH_ASKPASS_REQUIRE=force`,** not `prefer`: OpenSSH ignores `prefer` when
  `DISPLAY` is unset, so the exports alone changed nothing in a terminal session
  or on a Mac without an X server.
- **Bound every external command** (F21). Removing the gate routes every ssh
  passphrase prompt through the broker, which turned an unbounded wallet call
  from one slow login into every ssh in the session. Only 3 of 22 invocations
  had a deadline. Two configurable budgets now cover all of them —
  `command_timeout` and `interactive_timeout` — and the deadline kills the
  command's whole process group, since a surviving grandchild holds the output
  pipe that `ssh` itself is reading.

→ features F6, F21; goals 1, 11.

### Phase 15 — Sign the macOS builds

A keychain item's ACL trusts the code identity of the binary that created it. An
unsigned Go binary changes identity on every build, so after each rebuild macOS
re-asks *"sshakku wants to use your confidential information"* even after a
previous "Always Allow", and a non-interactive lookup fails with `-25308` or
`-128` instead. During development this is noise; for distribution it is every
user re-prompted on every release.

- Decide the signing identity and where it lives (a developer ID in CI secrets,
  or ad-hoc signing with a stable identifier).
- Until it is settled, the rebuild-then-look-up case is expected to fail; record
  it that way rather than leaving it undocumented.

→ feature F4; open decision: how releases are signed and notarised.

### Phase 16 — The home too long for a socket ✅ Done

A socket address is capped at 104 bytes on Darwin and 108 on Linux — a limit on
the address, not on the file system, so it surfaces as `bind` answering
`invalid argument` and naming no length at all. Two of SSHakku's sockets were
built from the user's home, which has no such limit and contributes its whole
length. Both were measured, reproduced and closed here; the second was not
known to exist when the phase was written.

- **The passphrase handoff** (Darwin only — Linux hands the passphrase over
  through the kernel keyring, which has no address). Under the cache
  directory, the path was the home plus 47 bytes, so a home past 56 characters
  had no room. The key did not load, and what the user was told was the
  kernel's `invalid argument`.
- **The agent socket** (both platforms). The runtime layout falls back to
  `$HOME/.cache/sshakku` when there is neither `XDG_RUNTIME_DIR` nor an owned
  `/run/user/$UID` — which is every macOS login, and any Linux session without
  logind. That path is the home plus 26 bytes, so a home past 81 characters
  left the session with **no agent at all**: F1 rather than F5, and the shell
  was told only `start ssh-agent: exit status 1`.

Both now prefer the private per-user temporary directory the session already
has, which is short, is what that system's own `ssh-agent` uses when nobody
tells it otherwise, and is only taken when it really is private: it must exist,
not be a symlink, belong to this user and grant nothing to anyone else, so a
shared directory named through the environment — `/tmp`, which is `/private/tmp`
under another name on macOS — is never written in (threat model T1, invariant
3). Anything else falls back to the cache directory as before. The handoff
directory is additionally forced to `0700` rather than left to the umask.

Where an address still does not fit, the error now names the length and the
limit; and an agent that refuses to start reports what `ssh-agent` itself said
rather than an exit status alone.

**Left open, deliberately:** a session with neither a runtime directory nor a
private temporary one — a container with `TMPDIR` unset, say — and a home past
81 characters still cannot bind an agent socket. The only shorter place left is
`/tmp`, which T1 forbids outright; changing that is a threat-model decision, not
an implementation one. Such a session is now told why.

Which directory to use and how long an address may be are passed in as values
rather than decided behind a build tag, so the choice, the length guard and the
privacy check are all exercised on Linux even where Darwin is what runs them.

→ features F1, F5, F6; open decision 24.

### Phase 17 — Finish macOS support

The shell suite runs on macOS now, and running it surfaced what is still
missing there. These are separate pieces of work; the list exists so they are
not rediscovered one at a time.

1. **KeePassXC has no route on macOS.** On Linux it is reached generically
   through the `secret-service` backend because KeePassXC implements that D-Bus
   API itself. macOS has neither, so a wallet the project offers on one
   platform is simply unavailable on the other. **Design decided 2026-07-31**
   (open decision 23): the native-messaging socket protocol is the primary
   route, `keepassxc-cli` the fallback, the wallet is named `keepassxc` on
   every platform, and the route stays pinnable — a pinned one is used and no
   other. Features F22, F23, F24.

   **Done, and verified by running it.** Both routes have been driven against a
   real KeePassXC on both platforms, as the whole user scenario rather than a
   backend round trip: `TestKeePassXCNativeFullRound` has the real binary ask
   once on a real terminal, save the passphrase over the local protocol, and
   reload the key into a dedicated `ssh-agent` with nothing typed — again after
   the key expires (F4, F5, F6, F9, F23). It runs in the container under
   Xvfb/xdotool, and on `macos-latest`, where the test starts KeePassXC itself
   because nothing there provides a running one.

   Driving the real thing is also what found the defect that made the route
   non-functional against any real KeePassXC: acceptance was read off the
   envelope, where a real KeePassXC names only failures, and the whole unit
   suite had endorsed it because the fake had been built from the
   implementation. Two steps a person normally takes are staged on macOS — the
   database is opened with `--pw-stdin` holding the stream open, and the
   association is written into the database rather than approved in a dialog —
   both preconditions of the route, not the promises under test.

   The `cli` route runs against a real `keepassxc-cli` on both platforms
   (`TestKeePassXCCLIRealDatabase`), which is also what established that it
   still takes the database password on standard input, something it offers no
   documented flag for.
2. **Nothing bounded the keychain — done.** `SecItemCopyMatching` is a
   synchronous cgo call with no timeout, context or cancellation, so on macOS's
   default backend there was no deadline at all. **Decided 2026-08-01**: F21 is
   about anything SSHakku waits on, not only about programs it runs, so the gap
   was a stated violation of it rather than a case the promise never reached.

   `KeychainBackend` now takes `command_timeout`, the same budget every other
   wallet gets, and applies it to each of the four operations — `forget --all`
   waits on `List` and `Delete` exactly as `load-keys` waits on `Find`. What
   elapses ends the waiting and not the call: nothing in Go can interrupt one
   already inside the framework, so the goroutine holding it stays blocked for
   as long as the framework does, deliberately and where the code says so.

   **What is unverified, and cannot be verified here**: no test makes a *real*
   keychain hang. The one that gives up is driven by a client that neither
   answers nor fails, which is what the framework does while waiting on an
   authorization nobody grants — but producing that state for real needs an
   interactive GUI session for SecurityAgent to display in, and a hosted runner
   has none: it fails such a call instead of blocking. See the two F21 keychain
   rows in `docs/TEST-MATRIX.md`.

   The gap was read off the code; no observed symptom motivated it. An earlier
   note here guessed that a report of `sshakku forget` not returning on a Mac
   was this same defect from the other side, with the keychain waiting on an
   authorization nobody grants. **That guess was wrong**, and running the
   product is what disproved it: on a real Mac `sshakku doctor --test-backend`
   stores, reads back and deletes its own keychain entry without asking anyone
   anything. Items SSHakku creates need no approval, and the report turned out
   to be two unrelated defects (items 4 and 5 below).

4. **A mistyped command asked for a secret instead of failing — done.**
   `SSHAKKU_ASKPASS` was exported into the whole login shell, and any argument
   that was not a known subcommand was taken for an ssh prompt. So
   `sshakku --forget` in a wired shell printed `--forget` as a prompt, read a
   line from the terminal with echo off, and wrote what was typed back to
   stdout — where a user expected "unknown command". Not macOS-specific. The
   promise is F30.

   **Decided 2026-08-01**: the guess goes away rather than getting better. ssh
   is pointed at `sshakku-askpass`, a link to the same binary installed beside
   it, and the binary reads the name it was run under. `SSHAKKU_ASKPASS` is
   deleted; nothing reads it.

   Why a name and not a better guess, established by experiment rather than
   from the manual: OpenSSH execs the whole `SSH_ASKPASS` value as one
   filename — `SSH_ASKPASS="<path> --askpass"` fails with `No such file or
   directory` — and always passes exactly one argument, the prompt. So no flag
   can be handed over, "no arguments" can never mean askpass, and "exactly one
   argument" would not have helped either: `sshakku --forget` is exactly one.
   The name is left, and it is not the same kind of signal as the marker was:
   the marker was identical in both situations and could only be resolved by
   guessing, while the name differs by construction — ssh execs the path it was
   given, a person types the binary's own name.

   A second binary was considered and rejected: Go links its runtime into each
   one, so it costs ~2.2 MB and an exec per prompt, and the link needs the same
   install and uninstall work either way. Backwards compatibility was declared
   out of scope: a login shell open across the upgrade keeps the old
   `SSH_ASKPASS` until re-login and falls back to prompting on the terminal.

   **What was run.** The real binary, installed, in a shell carrying the real
   exports: `sshakku --forget` answers `unknown command "--forget"` with the
   usage and exit 2, no prompt and no echo. `test/bats/askpass-broker.bats`
   drives the installed binary the same way, and its wallet-refill case proves
   the exports still reach a real `ssh-add` through the link. Two integration
   tests failed on the way and were right to: they built the binary and handed
   its own path as the askpass program, a layout no install produces.

   **What is unverified**: the macOS half of all of it runs only in the macOS
   job — nothing here can drive it.
5. **A wallet the platform has not got could be named and chosen.** **Done.**
   `resolveSecretBackend` defaulted an unset `secret_backend` to
   `secret-service` on every platform, so on macOS the report named a wallet
   that cannot exist there and demanded a D-Bus session bus, while the
   passphrases went to the Keychain. Each platform now declares what it has
   (F26), the freedesktop probe is compiled on Linux alone, and one list —
   `config.SecretBackends` — answers every caller that offers or rejects a name.

   The review this opened found the same shape in three more places, all fixed
   here: the graphical prompt (X11/Wayland and kdialog, compiled on macOS where
   none of the three exist), the doctor's process ancestry and cgroup reads
   (procfs, wired on every platform, so on a Mac every agent was attributed to
   nobody), and the two macOS output parsers in `hostcheck.go`. Rule 26 records
   the rule; no negated build tag is left in the tree.

6. **macOS graphical passphrase prompt — done, one half unverified.**
   `newGraphicalPrompter` now returns an osascript dialog, but only where
   `launchctl managername` answers `Aqua`. Being on a Mac was never the
   condition: a single-user-mode boot and an SSH login from another machine
   are sessions with no window server, and a dialog sent to one of those is a
   login shell waiting on something that can never appear (F21). The promise
   is F29; the AppleScript is a file of its own with a `lint-applescript`
   target (Rule 12), run by the macOS job since `osacompile` ships only there.

   What remains: **the window has never been drawn for anyone to see.** The
   hosted runner's session does report `Aqua`, so CI raises a real dialog and
   shows sshakku giving up on it inside its budget — but nobody is at that
   screen, so whether the prompt is readable, correctly titled and actually
   answerable is unverified until someone runs it on a Mac they are sitting
   at. See the ⚠️ in `docs/TEST-MATRIX.md`.

7. **The CLI backends are untested on macOS.** 1Password and Bitwarden are
   supported on both platforms, but the real-account jobs run on
   `ubuntu-latest` only, so nothing exercises them where the rest of the
   platform differs.

→ features F5, F6, F13, F17, F21, F25, F26; open decision 23.

### Phase 18 — An empty answer is a wrong answer ✅ Done

Pressing Enter at the passphrase prompt handed an empty passphrase to the key
adder. On Linux the out-of-band handoff is the kernel keyring, which refuses an
empty payload (`add_key` → `EINVAL`), and the loader treats an adder error as
fatal: the key was abandoned after one attempt, with a message naming an errno,
and no give-up recorded — so the next shell asked all over again. F8 promises
the opposite, and macOS already kept it, because a socket accepts an empty
payload and so the failure lands on `ssh-add`, where it is counted.

**Decided (2026-08-03).** The loader decides it, once, rather than either
handoff: an empty answer opens no key — a key that has no passphrase is never
asked about — so it costs an attempt and the user is asked again. Nothing about
that keystroke is transport-specific, and leaving it to the transport is what
made one keystroke mean two different things on the two platforms. Only the
exactly-empty string is refused; a passphrase of spaces is the user's business.
The stored-passphrase path needed no change: it already drops an empty or
whitespace-only value.

Found by reading a symptom nobody had explained — a report of
`stash passphrase: invalid argument` — rather than from the code, and
reproduced before it was diagnosed: the kernel refuses an empty `user` key, and
`TestAddRefusesAnEmptyPayload` now pins that so the reasoning above cannot rot.
The promise is covered on a real terminal on both platforms
(`TestLoadKeysEmptyAnswerRealTerminal`), which is the only place the question
can be asked: anywhere else an empty answer and no answer are the same bytes.

→ feature F8; goals 1, 11.

### Phase 19 — A pinentry that cannot draw is not a dialog ✅ Done

The prompter chain asked whether `pinentry` was on PATH, not what it was. GnuPG
builds several and the distribution picks which one `pinentry` runs; the curses
and tty builds draw on a terminal. Since pinentry is tried first, a graphical
session that had one of those lost the dialog to it — and lost it to a prompt
that appeared nowhere, because a login shell has no terminal for it either. A
box with kdialog or zenity installed was worse off than before Phase 13.

**Decided (2026-08-04).** pinentry is asked what it can draw with (`GETINFO
flavor`) rather than guessed at from its name. The answer is a chain, most
capable first — a GTK build says `gtk2:curses`, meaning it draws with GTK and
falls back to the console where there is no display — so the first element
settles it. Only `curses` and `tty` disqualify a build: an answer nobody
recognises, or none at all, still counts as a dialog, since passing over one
that works is the worse mistake and one that fails when asked already reaches
the terminal with its name in the log. Asking waits on no person, so it takes
`command_timeout` rather than the interactive budget.

Naming such a pinentry is still that one or the terminal, never another dialog
(F37) — but the log said it was "not installed" on a machine where it was.
Which reasons a prompter can be unavailable for is now the prompter's own answer:
kdialog and zenity are either there or not and keep the sentence they had.

Reproduced before it was diagnosed, in an image with a screen, zenity, and
`pinentry-curses` as the only pinentry: no dialog at any point, and the key
never loaded. The same image now shows zenity's window and takes what is typed
into it.

→ features F29, F37; goals 11, 15.

### Phase 20 — The wait budgets nobody could set ✅ Done

`command_timeout` and `interactive_timeout` never survived being read from a
file. `Merge` had no clause for either, and `Merged` folds every source onto an
empty `File`, so a value written in `config.toml` was dropped exactly like one
written in a drop-in. Both are config-file only, by the same reasoning as the
wallet settings — which left no way at all to change how long SSHakku waits,
against what F21 promises.

`sshakku config` made it worse rather than catching it: the report reads which
file *states* a setting from the sources and the value in force from the
resolved settings, so it printed the built-in default beside the name of the
user's own file, with no refusal. The one command meant to end that doubt
confirmed a setting that was not in force.

**Found (2026-08-04)** while closing coverage gaps, not from a report. Every
test of these two built the `File` in memory and called `Resolve`, skipping the
merge; the "every field" merge test listed its fields by hand and had stopped
naming the ones added after it was written. Rewritten to fill the struct by
reflection, it failed on its first run — and a field it cannot fill now fails
the suite, so the next setting added without a merge clause cannot disappear
the same way.

→ features F21, F35; open decision 24.

### Phase 21 — Closing the prompt is an answer ✅ Done

Closing a passphrase dialog without answering was user-visible behaviour with no
feature id: it existed as an error value in the code and a sentence in a commit
message, so no test could be derived from it and nothing had to keep agreeing
with it. F38 states it now, and two things it says were not true.

A dismissed dialog abandoned that key and went on to the next, so a login with
three keys still meant three windows after the first was shut — the gesture that
means "not now" everywhere else bought nothing. And it was logged at ERROR, which
`doctor` tails back to the user, so a deliberate choice was shown to them as
something the product got wrong.

**Decided (2026-08-04)**: configurable, because both readings are defensible and
neither is discoverable from the other. `on_dismiss` is config-file only like the
dialog it answers: `"stop"` (the default) asks about no further key that login,
`"skip"` turns down that key alone, `"retry"` treats the dismissal as a wrong
answer and ends at the ordinary give-up. Whichever applies, nothing is stored and
no key is given up — the next login shell asks again from the first key.

Ctrl-D at a terminal prompt is the same gesture, decided at the same time. It
reached the loader as an ordinary read failure, so a person who pressed a key on
purpose was told `could not load key id_test: EOF`. `ReadTTYLine` now answers the
refusal, so every question SSHakku asks on a terminal inherits it — but only for
input that ended, never for a read that failed, since a terminal letting the user
down is not the user declining.

**Verified by running it** (rule 25): the real binary in a disposable image with
a display server and three keys, five rounds, plus the live-terminal half on a
real pseudo-terminal. See the two F38 rows in `docs/TEST-MATRIX.md`. What no run
here covers is a person closing a real dialog on macOS, which is the same gap the
graphical-prompt rows already record.

→ features F8, F21, F29, F35, F37, F38; open decision 24.

### Phase 22 — The session nobody had tested in ✅ Done

Every claim SSHakku makes about a graphical session had only ever been observed
in an X11 one: every image in `test/containers/` starts `Xvfb`, and the Wayland
column of the session table in `docs/TEST-MATRIX.md` was three ❌. Wayland is
what most Linux desktops log into.

`wayland.Dockerfile` is that session — sway on wlroots' headless backend, so no
DRM device, no seat and no privileges, with no Xwayland and no X server of any
kind. Windows are read back with `swaymsg`, which answers with the client's own
name, and typed into with `wtype`, which speaks Wayland's virtual-keyboard
protocol; `xdotool` needs an X server and `ydotool` needs `/dev/uinput` and a
privileged container. The seat carries no keyboard until wtype makes one, so the
first keystroke of each invocation is dropped and a leading `Shift_L` absorbs it.

**The defect it found.** `pinentry-gnome3` answers `GETINFO flavor` with
`gnome3:curses`, so SSHakku takes it for a dialog, and then fails `GETPIN` with
`Inappropriate ioctl for device` wherever the Gcr prompter it wants is not
running. The chosen dialog was paired with the terminal alone, so one that could
not draw carried the question past every dialog that could — on a desktop with
zenity installed and working, and with no controlling terminal to fall back to,
the user was asked **nowhere at all**. F37 now says what happens when the dialog
nobody named cannot draw, and the dialogs a desktop has are asked in turn with
the terminal last. A dialog the user did name is unchanged: that one or the
terminal, never a substitute.

The fix uncovered a second, smaller one: the log said `asking on the terminal
instead` when the question had gone to another dialog. It names where the
question actually went now, which needed the terminal and a fallback pair to be
able to say what they are called.

**Verified by running it** (rule 25): `test/containers/wayland-prompt-scenario.sh`
drives the real binary in that session and asserts what a user meets. It was
observed failing against the tree before the fix — 0 windows with nothing
configured — and is wired into `desktop-stack.yml`.

**Rule 12, new file type.** The image carries a sway configuration
(`wayland-sway.config`). No linter or validator for that format exists — sway
checks its own configuration with `sway --validate`, which needs a compositor
build rather than a lint tool, and the file is exercised on every run of the
image anyway, since a session that will not start fails the job. **Decision: no
`lint-sway` target.**

**What this does not close.** The three wallet cells in the session table stay
❌: each of those daemons has a one-time dialog of its own that the existing
images answer with `xdotool`, and answering them with `wtype` is separate work.
The container's kernel keyring also refuses the passphrase handoff, so the run
shows where the user is asked and that the answer reaches the product, not the
key arriving in the agent.

→ features F12, F29, F37; open decisions 20, 24.

### Phase 23 — The wallets nobody had opened on Wayland ✅ Done

The three wallet cells Phase 22 left ❌. Each wallet is now reached from a
Wayland login as well as the one it already had, and the obstacle was different
in every case — none of them the `xdotool`-to-`wtype` port the previous note
predicted.

- **KDE** needed nothing but the session: PAM opens the wallet at login, so
  ksecretd draws no dialog at all. The same image starts a compositor or not,
  chosen by `SSHAKKU_SESSION_SCRIPT`, and the round trip is the one that was
  already there.
- **GNOME Keyring** cannot be answered from the keyboard in such a session:
  `libgcr-ui` grabs the seat before it accepts input, and a seat with no input
  device has nothing to grant, so the collection dialog draws, holds focus and
  ignores every key — while zenity, the same toolkit in the same session, takes
  what is typed into it. The session is therefore given a **pointer device that
  never sends an event**, purely so the seat has one, and the dialog is clicked
  through the compositor's own cursor.
- **KeePassXC** would not start: it links Qt5 and the image carried the Qt6
  wayland package, so Qt fell back to X11, found no display, and quit before
  drawing. With `qt5-qtwayland` and the `XDG_SESSION_TYPE` a real Wayland login
  declares, its wizard is answered by **what each window says it is** — the
  compositor names them — rather than by whether a click landed inside an
  allotted sleep. That is the one place where driving a GUI on Wayland is
  steadier than on X11.

**What the container is not given**, and why it matters: no privileged mode and
no `/dev/input` mount. It is granted `/dev/uinput`, the input major and
`CAP_MKNOD`, so it makes the one node it uses and can open nothing else. An
earlier attempt did mount `/dev/input`, and the compositor inside the container
opened every real device on the host — the developer's keyboard included.

**Found on the way, and fixed here:** `TestKeePassXCNativeFullRound` had been
failing on master on both platforms. The test built the binary into a temporary
directory and put no `sshakku-askpass` link beside it — a layout no install
produces — so `ssh-add` was pointed at a path that does not exist and the key
could never open, however right the passphrase was. Nothing had said so because
the integration suite runs only on demand.

**A matrix defect, also fixed here.** `kde.Dockerfile` starts no display server
and says so in its own header, yet its ✅ sat in the X11 column and its
no-display cell was a `—` justified by "these daemons all need some display" —
false for KDE, and exactly the kind of `—` rule 19 forbids.

**Rule 12, new file types.** The KeePassXC session carries a second sway
configuration that includes the shared one; the same "no validator that is not a
compositor" decision as Phase 22 applies, and the file is exercised on every run.
**Decision: still no `lint-sway` target.** The uinput pointer tool is Go behind a
`//go:build ignore` tag: outside the module and the coverage it reports, still
covered by the `gofmt` sweep `make lint-go` runs, and compiled on its own by the
images that need it — so no new linter and no new language.

**Verified by running it** (rule 25): each wallet's round trip driven against its
real daemon in its Wayland session, from rebuilt images, through the same runner
script CI uses. The GNOME and KDE steps have since passed on a hosted runner,
which is also what settled whether one grants `/dev/uinput`.

→ features F4, F5, F6, F9; open decisions 20, 24.

### Phase 24 — The X11 session, and the screen the daemon never saw ✅ Done

The last ❌ in the KDE row of `docs/TEST-MATRIX.md`: the session most KDE users
are actually sitting in had never been built. `kde-x11-session.sh` puts an Xvfb
display around the same PAM unlock the other two sessions use, and the round
trip is the one already there — same image, same test, so the session stays the
only variable.

**Building it is what showed the other two were not what they claimed.** The
X11 round trip passed on the first run, and the daemon's own
`/proc/<pid>/maps` said why it should not have: ksecretd had mapped
`libqoffscreen.so`, with no `DISPLAY` in its environment. The Wayland session,
already ✅, was the same — `libqoffscreen.so`, no `WAYLAND_DISPLAY`. `kde.env`
is installed as `/etc/environment` and `kde-pam.conf` has PAM read it into the
session, so `QT_QPA_PLATFORM=offscreen` reached the daemon in every login; each
session script's `unset` had only ever reached the client side. Both "a login
with a screen" cells were varying SSHakku's view of the session while the
daemon stayed exactly where the no-display cell has it.

Dropping that one line is the fix: the platform each session needs is already
in the process environment the daemon inherits — the entrypoint's `offscreen`
where there is no display, and nothing at all where there is one, so Qt chooses
as it does on a real desktop. The daemon now maps `libqoffscreen.so`,
`libqwayland.so` and `libqxcb.so` in the three sessions respectively.

**The pairing that proves it** (rule 23), both halves run: with the X server
removed from the session script, the X11 login fails — ksecretd never registers
`org.freedesktop.secrets` and the session times out — and with the old
`QT_QPA_PLATFORM` pin restored, that same displayless session passes. The X
server was decorative before the fix and load-bearing after it.

**Verified by running it** (rule 25): all three sessions driven against the real
ksecretd from rebuilt images, each one's platform read out of the daemon's own
address space rather than inferred from the session it was started in.

→ features F4, F5, F9; open decisions 20, 24.

### Phase 25 — The machine with no screen, and the promise nobody had made ✅ Done

The last ❌ in the session/display table of `docs/TEST-MATRIX.md`: GNOME Keyring
daemonized with no session and no display at all — the machine its user reaches
over SSH. `gnome-keyring-headless-session.sh` starts the same daemon with the
login keyring unlocked from standard input, which is the one unlock
gnome-keyring offers with nothing to draw on, and checks the daemon's own
`/proc/<pid>/environ` for a display rather than trusting what the script
exported.

**The run found a line the catalogue had never drawn.** Creating the
compartment is the one Secret Service operation gnome-keyring always answers
with a prompt, and the prompter needs a screen: without one the round trip
fails at `prompt … dismissed`. Using a compartment that already exists needs no
screen at all — established with two logins sharing one home, the compartment
made in a login that had a screen and then used in one that had none.

**The analysis before touching the catalogue.** The session/display dimension,
and this very ❌, had been in the matrix since 2026-07-24, five days before the
feature catalogue was written — so the gap was not a promise written badly but
one never made. The catalogue did address the display, though only where a user
meets it, the prompt (F29 names the SSH login outright); that a *wallet* might
need a screen was never stated, because an installed, answering wallet was taken
for a usable one. That held for everything then tested: KDE needs no screen
because PAM opens its wallet, GNOME and KeePassXC were only ever driven in
sessions that had one. Of the three ways out — make it work headless (closed
from outside: `CreateCollection` always prompts, and the only keyring
gnome-keyring opens without one is `login`, which F33 refuses for good reason),
declare it unsupported (throws away behaviour that works), or state what
happens — the third. It is stated for the class rather than this cell: any
wallet that needs a screen is subject to it.

**F39 and F40, added first** (rule 21). F39: a wallet that can only be opened
where there is a screen is used wherever there is one, and where there is none
the keys still load — asked for each time, saved never, so the asking comes back
at every login and every expiry; what such a wallet needs a screen for is being
set up, not being used. F40 covers the second gap the same analysis found, which
F29 and F21 do not: a session with nobody to ask at all — no screen and no
terminal either, which is what an `scp` or a scheduled job runs in — is never
held up.

**Red before green** (rule 23), both pairings run. The scenario
(`gnome-keyring-headless-scenario.sh`, driving the real binary through a real
login: `make install-user` wires the account's own profile, `bash -li` on a
pseudo-terminal, `keyring-session.sh` for the handoff) was made to fail by
running it in the X11 session of the same image, where the wallet *can* be set
up: the two assertions belonging to F39 and F40 go red there, because the key is
reloaded from the wallet in silence and nothing is ever left alone for want of
somebody to ask. The round trip was made to fail by taking away the compartment:
the same headless run without it stops at `prompt … dismissed`.

**Verified by running it** (rule 25): six assertions green in the headless
session, the round trip green against a compartment made elsewhere, and the
non-interactive half — a login shell, a `setsid ssh-add` and a `setsid sshakku
load-keys`, none with a terminal — coming back in under a second rather than
waiting.

→ features F4, F5, F9, F39, F40; open decisions 20, 24.
