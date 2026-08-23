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
| All committed files | `editorconfig-checker` (config excludes `LICENSE` verbatim, `*.zsh`, and `*.go` — gofumpt owns Go formatting) |
| Shell — bats tests (`*.bats`) | Deferred until test files enter the repo |
| Go (`*.go`) | `golangci-lint fmt --diff` (**gofumpt**) + `go vet` + `golangci-lint run` (config `.golangci.yml`), each run once per build — Linux, macOS, and the two failure-injection tags — see Phase 34 for why one run is not enough; `golang.org/x/sys` (BSD-3-Clause) recorded in `COPYRIGHT.md` |
| Go tests (`*_test.go`) | `testifylint`, with **`require-error` disabled** — see Phase 33 for why that one checker cannot decide `require` vs `assert` for us. Assertions go through `github.com/stretchr/testify` (MIT, recorded in `COPYRIGHT.md`) |
| TOML (`*.toml`) | `taplo lint` + `taplo format --check`; runtime parser `github.com/BurntSushi/toml` (MIT) recorded in `COPYRIGHT.md` |
| Dockerfile (`test/containers/*.Dockerfile`) | `hadolint` (config ignores DL3008 — no viable apt-pin story against a rolling suite; the base image tag is the point-in-time anchor) |
| XML (`internal/*/testdata/*.xml`) | `xmllint --noout` (`lint-xml`) — well-formedness only; the DTD the D-Bus bus configuration names is an `http://` URL and is deliberately not fetched, so `make lint` needs no network. Ubuntu's `libxml2-utils` is installed by the workflow rather than by the pinned tool cache, which a cache hit would skip |
| Windows batch (`cmd/*/testdata/*.cmd`) | `blinter` (`lint-bat`, config `blinter.ini`) — a static checker for `.bat`/`.cmd` with rules for syntax, security, performance and style. Python rather than a single binary, so CI pins it with `pip install Blinter==`; AGPL-3.0-or-later, which imposes nothing here on the same ground as `shellcheck` and `hadolint` (both GPL-3): CI-only, run as a separate process over the files, never bundled or distributed. Its first run reported twelve findings on the one batch file here and five of them shared a cause — a `goto` loop, which the file no longer has. Three are off in `blinter.ini` with a reason each: a stand-in editor is *told* what to do by the environment (E006) and *records the argv it was handed* (SEC013, SEC014), so both are its contract rather than defects. It is also the one file type `.gitattributes` and `.editorconfig` check out as CRLF — cmd.exe seeks through a script to find a label, and does not reliably find one when the lines end in LF alone |
| D-Bus service file (`*.service`) | **No linter.** `desktop-file-validate` validates desktop entries and rejects this format outright (`first group is not "Desktop Entry"`), and nothing else parses it; the only checker that applies is `editorconfig-checker`. The file is exercised instead: a test that reads it asserts the bus it is given to reports the name as activatable |

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
`SecretBackend` (Security.framework via cgo, open decision 23 — moved off cgo
in Phase 32).

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
   unit-test coverage (Linux, macOS and Windows) and posts/updates
   a single PR comment: total coverage per OS, wall-clock test time, a ranked
   list of the slowest tests, and a failure report when something fails.
   Windows joined last and needed no Go at all: `tools/testreport` never knew
   which system a report came from, so all of it is the job running the suite
   under `gotestsum`, three artifacts, and a third badge.
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
   from item 7) was dropped in Phase 32 for want of an object: there is no cgo
   left to instrument. The Linux
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
   the race detector already covers Go memory-safety; ASan/MSan added value only
   on the cgo path (the darwin keychain), and Phase 32 removed it — the tree is
   pure Go on both platforms, so there is nothing left for them to instrument.
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
   synchronous call into a system framework with no timeout, context or
   cancellation, so on macOS's default backend there was no deadline at all. **Decided 2026-08-01**: F21 is
   about anything SSHakku waits on, not only about programs it runs, so the gap
   was a stated violation of it rather than a case the promise never reached.

   `KeychainBackend` now takes a budget and applies it to each of the four
   operations — `forget --all` waits on `List` and `Delete` exactly as
   `load-keys` waits on `Find`. Which budget it takes was decided here and
   decided wrongly; Phase 27 has the correction. What
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

7. **The CLI backends were untested on macOS.** Both are supported on both
   platforms, but the real-account jobs ran on `ubuntu-latest` only, so nothing
   exercised them where the rest of the platform differs.

   **1Password: done, and settled by running it.** The real-account job is a
   matrix over both systems now, and the round trip — a throwaway vault
   created, stored into, read back, listed, deleted, and the vault removed —
   passed on `macos-latest` in 13s against a real service account. The answer
   was not knowable from here: the test cannot be run on a developer's machine
   without creating a vault in their own account, so the macOS job was the
   experiment, not a confirmation of one.

   Adding the job was the smaller half. That test skips itself when `op` is
   absent, when it is not authenticated, or when its opt-in variable is unset,
   and `go test -run` exits 0 on a skip — reproduced with the command the job
   ran: `--- SKIP`, then `PASS`, then `ok`, exit 0. A macOS leg added on top of
   that would have gone green having run nothing, which is worth less than the
   ❌ it replaced, and the Linux leg had the same hole already. Both go through
   `test/onepassword-real-account.sh`, which requires the test's own
   `--- PASS:` line in the output; made to fail by that same skipped run.

   **Bitwarden: done too, and the runner was not the obstacle it looked like.**
   The blocker had been recorded as "a hosted macOS runner has no Docker".
   Colima gives it one — emulated rather than virtualised, since Apple's
   hypervisor refuses on a runner that is itself a virtual machine and says so
   ("Virtualization is not available on this hardware"). Only the server goes in
   the container: `bw` and the test binary stay native, because a Mac talking to
   a Bitwarden is exactly what nothing had ever watched. The round trip passes
   on `macos-latest` in 114s.

   Two things were learned by running it that could not have been read off the
   code. The first bind mount carried nothing: Colima's machine cannot see
   macOS's temporary directory, and Docker substitutes an empty directory
   silently, so the server announced creating the private key it should have
   found. The fixture now travels as a build context instead, which depends on
   no mount at all.

   The second looked far worse than it was. The current `bw` threw from inside
   its own SDK on `create item` — on Linux too, and on its own item template, so
   neither the platform nor SSHakku's payload. Against Vaultwarden 1.37.1 the
   same CLI and the same fixture database create the item and exit 0: the client
   was fine and the fixture's server was too old for it. Pinning the CLI back
   would have made the job green on a pair no user runs, so the server moved
   instead — and the lesson is that a client-side exception is not evidence of a
   broken client.

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
covered by the formatting sweep `make lint-go` runs, and compiled on its own by the
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

### Phase 26 — The doctor that reassured you about a wallet that could hold nothing ✅ Done

Found while verifying Phase 25: on a machine with no screen, where no passphrase
can ever be saved, `sshakku doctor` reported `session bus: found` and nothing
else. Reproduced before any theory was built on it, and a second case came out of
the same run: a session bus with **nothing** owning `org.freedesktop.secrets`,
and nothing that could be started, produced the identical report. A wallet that
was not there at all was reported satisfied — the commoner of the two, since it
is any desktop whose keyring is not running.

**The analysis, before anything was changed.** F25 promises the doctor tells you
when something the wallet needs is not there, and constrains it to find out
*without touching a stored passphrase*. The implementation had turned that into a
larger rule of its own — never talk to the wallet at all — and only the session
bus survives it. Listing collections touches no passphrase, so the code was
under-delivering a promise that was written correctly: a defect to fix, not a
feature to rewrite. F25 stands unchanged.

**F41, added first** (rule 21). The three conditions the plain report must meet
were true only by accident while it talked to nobody, and become behaviour the
moment it does: the report always comes, even incomplete; it never waits without
end; it changes nothing. Nothing in the catalogue promised any of them, so
nothing could test them. F41 also fixes the three levels: the plain report looks,
`--test-backend` talks to prove, `--fix` is the one allowed to write.

Bus activation is an act, so the look asks the bus who owns the name and talks to
a wallet only if one already answers; a wallet that is merely activatable is left
alone and the compartment reported *undetermined* — a third state beside found
and missing, which produces no finding, because something nobody established is
not something wrong.

**Red before green** (rule 23). The scenario
(`gnome-keyring-doctor-scenario.sh`) went red on both cases against the previous
build. Two assertions could not fail in that session and were falsified where
they can: creating nothing was made to fail by resolving the compartment the
ordinary way (`looking created 1 collection(s)`), and the bound by dropping it
(30 seconds instead of 2). One assertion passed for the wrong reason first — the
findings were searched for the word "wallet", which another finding already
contains — and was tightened to the wallet's own name before being believed.

**Verified by running it** (rule 25): the real binary in a session with no
screen, reporting `compartment: missing — "sshakku" is not there and this session
has no screen to create it on`, and on a bus with no wallet naming the wallet
among the findings; the same collections after the report as before; and a
SIGSTOP-frozen daemon costing the report two seconds.

Not done here, and left with its own analysis to do: now that the doctor can say
the compartment is not there, `--fix` could create it — which works where there
is a screen and cannot where there is none. Phase 28 is that follow-up.

→ features F25, F41; open decision 24.

### Phase 27 — The wallet asked for a password, and was given thirty seconds ✅ Done

Reported from real use on both platforms, and the two turned out to be one
defect. On a Mac, after a reinstall, a lookup and a store each gave up after ten
seconds while macOS was showing its approval dialog and the user was still
typing. On Linux, after a reboot, the desktop asked for the wallet password, the
answer came too late, and the login went on to ask for a passphrase that had
been saved for months.

**The analysis, before anything was changed.** F21 already promises exactly what
was missing: the wait is bounded, and *how long to wait is configurable,
separately for something expected to answer on its own and something that is
waiting on you*. Both budgets existed and were right. What was wrong is which of
them these two waits were given. The keychain took `command_timeout` — and
`internal/config/config.go` said so on purpose, reasoning from *how* the wallet
is reached (in-process, not by running a command) rather than from *who* is
being waited for. The Secret Service prompt took neither: a package-level 30
seconds that nothing could configure, which is the promise's second half broken
outright. A defect in both places, and F21 stands unchanged.

**Red before green** (rule 23). The macOS half cannot be run from this project's
development machines at all, so its test was committed failing on its own and
watched go red in the macOS job — `Timeout = 3s, want 1m30s`. The Linux half was
made to fail on the numbers rather than on a missing symbol: the seams were
added carrying the old values first, which reported an unconfigured prompt wait
of 30s and a client handed no budget at all.

**What is unverified**: no test puts a real dialog in front of a real person on
either platform. The state was reached by hand — the two reports above are the
reproduction — and what the tests pin is the budget each wait is given. See the
two F21 dialog rows in `docs/TEST-MATRIX.md`.

Not done here, and noted rather than guessed at: `promptTimeout` bounded the
compartment-creation dialog too, which is the other place a person is asked to
choose a password. It now takes the same configurable budget as every other
prompt, but nothing exercises that path yet — Phase 28 does.

→ feature F21; PLAN Phase 17 item 2 (which chose the budget this corrects).

### Phase 28 — The compartment the report told you to go and make yourself ✅ Done

Phase 26's own follow-up. The doctor could say the compartment a wallet keeps
SSHakku's passphrases in was not there, and offered no way of making one: the
only way was to save a passphrase and let the first save create it, which is a
side effect of another operation rather than something a user can ask for. F39
tells the user to "make that compartment once from a desktop session" and the
project had no command for it — not even its own test harness, which ran a `go
test` to trigger the creation.

**The analysis, before anything was written.** Not a defect against F14: that
promise repairs "what it reports as fixable", nothing in the code classified a
finding as fixable, and so nothing was mechanically broken. The behaviour was
missing rather than wrong, so the promise was written first, as F42.

The reproduction found the shape of it exactly: `--fix` could act precisely
where the report said nothing was wrong (with a screen, "not there yet" and no
finding), and was helpless precisely where the report raised a problem (no
screen, where the dialog has nowhere to appear). And in both sessions the
`after:` report had no wallet section at all, so F14's "the same report comes
back clean" was unshowable for anything wallet-shaped — a second, smaller gap
found by running the command rather than by reading it.

**Red before green** (rule 23). The unit assertions were written from F42's "how
you can tell" and every one failed on behaviour: the report never named `--fix`,
the maker was asked for zero times, the report afterwards had nothing about the
wallet. The scenario (`gnome-keyring-fix-compartment-scenario.sh`) was run
against the previous build in both sessions — five failures on a screen, three
without — before being run against this one.

One assertion was corrected rather than satisfied: a repair that was attempted
and refused now exits non-zero. It had been written expecting zero, which would
have told a caller reading only the exit code that a repair it asked for had
succeeded.

**Verified by running it** (rule 25): the real binary in both sessions of the
`gnome-keyring` image, with the wallet's own dialog answered. On a screen the
bus itself — asked directly, not through SSHakku — holds a collection it did not
hold before; with no screen it holds exactly what it held before.

The scenario answers that dialog from the keyboard rather than by clicking a
coordinate, and the difference is not cosmetic: a click that lands where the
button is not does not miss harmlessly, it dismisses the prompt, and there is no
second chance to answer it.

Not done here: `gnome-keyring-create-collection.sh` still runs a `go test` to
make the compartment for the other scenarios. It could now drive the real
binary, which would have the harness set the state up the way a user does.
Done in Phase 30.

→ features F42, F14, F39, F41; PLAN Phase 26 (whose follow-up this is).

### Phase 29 — The configuration nobody had driven on a Mac ✅ Done

Three cells of `docs/TEST-MATRIX.md` (F34, F35, F36) carried the same ❌ with
the same excuse: nothing in them touches a facility of the operating system, so
the unit tests running in the macOS job were taken for coverage of the whole
promise. What that reasoning left out is that the promises were driven through
the real binary exactly once, by hand, on the `debian` image — and a run that
happened once on one platform says nothing about either.

`test/bats/config-commands.bats` drives all three, twelve rounds, and it needed
no new workflow: `make test-bats` already runs in the container job and on the
macOS runner, so the suite closes both platforms by existing.

**What running it found.** `doctor` printed the keys section only when it had a
key to list, so a key directory that held nothing SSHakku recognised was never
named — the case `docs/CONFIGURATION.md` describes as the reason the name is
printed at all ("if it lists nothing you can see which directory it was told to
look in"), and the one where a name rule matching nothing and a directory the
user did not mean look identical from the outside. `KeysDir` is set only where
the keys were actually looked for, which is what separates "looked and found
nothing" from "never looked" — a report about another user, which must stay
silent. F34 gained the sentence the fix satisfies.

**Red before green** (rule 23), one round at a time rather than all at once: the
default name rule widened to `["*"]`, `key_patterns` dropped from the mapping,
`key_dir` ignored, and the `config` case removed from the command dispatch. The
unit test for the empty section was red against the condition as it stood.

The F34 rounds judge the selection on what SSHakku says it did — the session log
naming the file it went and asked the wallet about, and the report listing it —
not on the key reaching the agent. That last step turns on a passphrase handoff
which a plain container has no kernel keyring for, and asserting it would have
left the selection itself unexercised in the very job meant to cover it.

→ features F34, F35, F36; PLAN Phase 6 item 6 (closing open matrix cells),
Phase 17 (what was still unrun on macOS).

### Phase 30 — The compartment the harness made for itself ✅ Done

Phase 28's leftover. The gnome-keyring image built the state every run in it
starts from — the compartment the wallet keeps SSHakku's passphrases in — by
running `go test ./internal/keys -run TestSecretServiceBackendRealDaemon`. So
the precondition of those runs was reached by a road no user has, while the one
command a user does have for it (F42's `sshakku doctor --fix`) did not exist
when that script was written.

Both create-collection scripts now run the real binary, and the half they share
moved into `gnome-keyring-make-compartment.sh` — leaving in each script only how
a button is pressed, which is what their comments had always claimed was the
only difference between them while both in fact also carried the trigger. X11
answers from the keyboard, Phase 28's finding applied where it came from;
Wayland still clicks and must, since the dialog grabs the seat there and ignores
every key sent to it.

The setup now also asks the wallet — over the bus, not through SSHakku — whether
the compartment is there before letting the session continue. It had no such
check: a press lost to render timing left the compartment unmade and the setup
silent, and the run that followed failed somewhere far from the cause.

**Red before green** (rule 23). Two deliberate breakages, both observed in the
image: with the answering loop removed, `--fix` is killed at its budget and the
session refuses to start; with the dialog answered but the check pointed at a
name nothing makes, the session fails on the check alone, while `--fix` itself
reports having made the compartment.

**What running it settled** (rule 25). `doctor --fix` does more than make a
compartment — it heals agent state too, so the setup now leaves an ssh-agent
running before everything else in that image, the whole Go suite included.
Whether that mattered was measured rather than argued: `make test`, the Wayland
round trip, the compartment-repair scenario on a screen and the frozen-daemon
scenario all pass. The headless round trip is the one that settles it, since it
is the two logins together: a compartment `--fix` made in the login with a
screen is there in the login with none.

**Found on the way, not fixed here.** `doctor` prints a compartment that is not
there as `found`: `compartmentRequirement`'s fixable branch sets `Present: true`,
which is how "not a finding where there is a screen to make one on" was
obtained, since `WalletFindings` reads `Missing()`. Suppressing the finding and
reporting the piece as present are two different things, and the state column
now contradicts the sentence printed beside it. Fixed in Phase 31.

→ feature F42; PLAN Phase 28 (whose leftover this is).

### Phase 31 — A compartment that is not there is not found ✅ Done

Phase 30's leftover, and a plain breach of F42: on a session that can make one,
`sshakku doctor` printed `compartment: found — "sshakku" is not there yet; …`.
The promise says the report tells you it is not there and names `--fix`; only
the second half was kept, and the first was contradicted on the same line.

`compartmentRequirement`'s fixable branch set `Present: true`. That bought one
thing worth having — a compartment that would appear at the first passphrase
saved is not a fault, and does not belong among the findings — but bought it by
claiming the piece was there, because `Present` is read both by the printer
choosing the word and by `Missing()` deciding findings. The two are separate
claims, as `Fixable`'s own comment already said, so the suppression moved to
`Missing()`, which now passes over what this session can provide itself, and
`Present` went back to meaning what it says.

The same conflation was in the tests: a table field named `wantPresent` and
commented "whether the compartment counts as a problem". Split in two, so a
change to either answer is caught rather than absorbed by the other.

**Red before green** (rule 23). Three, all observed failing first. The subtest
whose name already promised this — "the report says it is not there, and names
--fix as what makes it" — asserted only the second clause; given the first, it
failed printing the self-contradicting line. A new case pinned the requirement
itself, `Present:true` on a piece whose own Detail says it is not there. And the
scenario, driven through the real binary in the image's X11 session, failed on
the new assertion alone while its other six stayed green.

**What running it settled** (rule 25). The fix-compartment scenario on a screen
(7 assertions) and with none (5), and the doctor scenario (9) — the last of
which holds the other side down: with no screen the compartment is still stated
where a user looks for problems, because there it really cannot be made.

→ feature F42; PLAN Phase 30 (whose leftover this is).

### Phase 32 — The build that needed a Mac to make a Mac binary ✅ Done

One file in the tree opened `import "C"`: the Keychain client, linked against
Security.framework. Everything downstream of that followed from it — a macOS
binary could only be produced on a machine carrying Apple's SDK and a C
compiler, and that file alone could not be type-checked from anywhere else,
while every other darwin file could.

`DarwinKeychainClient` now reaches Security.framework and CoreFoundation
through `purego` (Apache-2.0, see `COPYRIGHT.md`), which loads them at run time.
The type, the `KeychainClient` interface, its five methods and their errors are
unchanged; no user-visible behaviour moved, so `docs/FEATURES.md` gained
nothing.

**Red first, and the red said more than the change did.** `make build-cross`
builds both targets with `CGO_ENABLED=0`. On the tree before the rewrite it
failed with `undefined: keys.DarwinKeychainClient` — not "C source files not
allowed": with cgo off, Go drops the file that opens `import "C"` *silently*.
A cgo-free macOS binary built then would not have failed; it would have shipped
with no Keychain at all.

**Race and cgo stopped being one decision.** The race detector is built on cgo
and a distributed binary should need neither, so `SSHAKKU_RACE` selects: set,
the suite runs under `-race` with cgo; unset, it runs as the shipped binary is
built. CI sets it. `build` never consults it, and neither does `test-keychain` —
the one test that reaches the framework has to reach it the way the shipped
binary will.

Three things were established by running them rather than by reasoning, and are
worth keeping:

- `Dlsym` yields the *address of* a data symbol, so a framework constant is one
  dereference away — checked against libc's `environ` on Linux, the mechanism
  being the same everywhere. The two `kCFTypeDictionary*CallBacks` are structs,
  so there the address itself is the value wanted.
- `go vet` rejects `unsafe.Pointer(uintptr)` and has no per-line suppression,
  so that dereference goes through `memcpy`, keeping the address on the C side.
- purego panics at *registration* on a signature it cannot marshal, and
  `RegisterFunc` takes a bare address it never calls — so all nineteen
  declarations were put to it on Linux, with no Apple symbol present. That is a
  type check, not a verification: accepting a signature is not agreeing with
  Apple's API.

**What running it settled** (rule 25). The live round trip
(`TestDarwinKeychainClientRealRoundTrip`) passed on `macos-latest` against a
real keychain with `CGO_ENABLED=0`, which is the only thing that could say the
signatures match the framework. On the Linux job, `CGO_ENABLED=0 GOOS=darwin go
build ./...` passed on a machine with no Mac and no Apple SDK.

Two consequences elsewhere: the ASan/MSan follow-up recorded in Phase 6 items 6
and 7 no longer has an object, there being no cgo path left to instrument; and
`CGO_ENABLED=0 GOOS=darwin go vet ./...` now covers the whole macOS tree, tests
included, from either platform.

→ features F4, F5, F6, F9 (unchanged, and the round trip is what says so);
Phase 6 items 6, 7; Phase 8, which no longer needs a Mac to build for one.

### Phase 33 — The tests that agreed with the code ✅ Done

Every check the suite makes now goes through `github.com/stretchr/testify`:
2,907 `assert`/`require` calls across 164 test files, and no `t.Errorf` or
`t.Fatalf` left anywhere as an assertion. Rule 27 records which of the two to
reach for — `require` where continuing would only panic or report a
precondition twice, `assert` where several independent checks each want to be
named, so one run tells you all of them.

**Rule 12 decision.** `testifylint` is enabled in `.golangci.yml` with
**`require-error` disabled**. That checker demands `require` for *every* error
assertion, which contradicts the `assert`-for-a-richer-report half of rule 27:
which of the two a given assertion wants is a judgement about that test, not
something a checker can read off the call. The rest of `testifylint` is kept
precisely because it catches assertions that compile, pass, and check nothing —
`go-require` (a `require` on a goroutine other than the test's calls `FailNow`
off it, hanging the run instead of failing it), `float-compare`, `formatter`.
Licences, all permissive and none obstructing relicensing: testify MIT, go-spew
ISC, go-difflib BSD-3, yaml.v3 MIT + Apache-2.0 — recorded in `COPYRIGHT.md`.

**The conversion was not the point; what it exposed was.** Rewriting an
assertion means reading what it actually pins, and each package was then put to
a differential mutation check: the promise is broken in the source, the old
suite and the new one are both run, and a mutation no test on either side
notices is a claim nothing was holding. **Forty-five defects were found this
way, every one of them in a test that was passing.** They fall into a few
shapes, and the shapes are the reusable part:

- **The subject can be deleted and the test still passes.** A fixture that ran
  out of input before the check under test was reached; `Ensure` failing on an
  empty `LogFile` no matter what the symlink guard did; `SUDO_UID` set to the
  caller's own uid, so honouring it and ignoring it gave the same answer.
- **An assertion satisfied by the wrong thing.** "An error came back" where the
  error's identity is the whole point — a prompt that could not be written
  reported as the user declining, a generic wallet failure taken for a refused
  alias. `indexOf(a) > indexOf(b)` is false when `a` is missing entirely, so the
  test passed when the row it was about was not there.
- **The evidence read off the wrong place.** A summary cell stuck on "n/a"
  passed because the same number appears again lower down and a bare `Contains`
  found it there.
- **Nobody asserted it at all.** `Look.Activatable`; `environmentNames`, the
  report about *another user's* session; whether the terminal's echo is ever
  turned off — the tests stub the very ioctl that does it, so a passphrase typed
  in the clear passed all of them.

Several of those are user-visible on a working machine: a doctor reporting that
passphrases cannot be saved on an ordinary Wayland desktop, or calling an
activatable wallet a missing piece; `gui_prompter = "auto"` — the documented way
to write "choose for me" — read as a dialog's name, matching none, and sending
every prompt to the terminal; a blank wallet entry handed to `ssh` as a
passphrase, which opens no key and asks for nothing.

**One change to production code**, and it is a seam, not a fix:
`ProcfsCgroup.root()` makes the empty-`Root` default to `/proc` a decision that
can be asserted, rather than one only reachable by reading the real procfs — and
so only checkable on a machine whose own processes happen to belong to a systemd
unit. The single production caller constructs `ProcfsCgroup{}`, so that default
is the one that ships. Nothing else in the tree changed: no behaviour moved, and
`docs/FEATURES.md` gained nothing.

**What the harness cannot judge**, recorded so it is not mistaken for agreement:
a mutation that stops the package compiling reports no failing tests, which
reads exactly like a mutation nothing caught — it was hit some two dozen times
and every `SURVIVED` has to be diagnosed before it is believed. A mutation
caught only by `goleak` fails the *package*, with no test name for a
differential to compare. And one that makes the code hang is reported by the
run's own deadline, not by a test.

**Verified** (rule 25): `SSHAKKU_RACE=1 make test` green across all thirteen
packages, `make build-cross`, `CGO_ENABLED=0 GOOS=darwin go vet ./...`, the two
build-tagged real-daemon files vetted with their tags, and `make lint` clean.
Per-package coverage is identical to `master`, package by package — this is a
test-suite change, and a coverage number is exactly the thing it must not move.
No end-to-end run belongs to this phase: no user-visible behaviour changed, and
the defects above are described by what a mutation showed, not by a claim that
each was reproduced against the shipped binary.

→ rules 22, 23, 24, 27; features F5, F6, F25, F29, F30, F37, F42 — all
unchanged, and that is the point: each was being reported on by at least one
test that could not have failed.

### Phase 34 — The linters, and the builds nobody linted ✅ Done

`gofumpt` plus ten analysers, added in the order they were argued for rather
than alphabetically, and each one made to fire before it was trusted.

**Rule 12 decision.** `.golangci.yml` gains a `formatters:` section enabling
**gofumpt**, and ten linters on top of the standard set and `testifylint`:
`gocheckcompilerdirectives`, `nilerr`, `durationcheck`, `forcetypeassert`,
`errname`, `bidichk`, `nolintlint` (with `require-explanation` and
`require-specific`), `errorlint`, `usetesting` (with the off-by-default
`os-setenv` and `os-temp-dir` turned on), and `thelper`. No new module
dependency: every one of them ships inside the golangci-lint version the lint
workflow already pins, so `go.mod` is untouched and there is no new licence to
record.

**gofumpt subsumes the old gate rather than competing with it.** `gofmt -l .` is
gone from `lint-go`, replaced by `golangci-lint fmt --diff`. The rules gofmt
leaves to taste are exactly the ones that had drifted: two files carried a
project import inside the stdlib group. The replacement also reaches further —
`fmt` reads files, not builds, so it sees the `_darwin.go` files and the
`//go:build ignore` tool under `test/containers/` that `run` never compiles.

**A linter that reports nothing is indistinguishable from one that is not
running**, so each was made to fire in a throwaway package that was then
deleted: a `// go:build linux` with a space in it, a `return nil` after
`err != nil`, an `i.(string)`, a sentinel without the `Err` prefix, a literal
U+202E inside a comment, a bare `//nolint`, a `d * time.Second`.

That discipline removed one from the list. **`nilnesserr` was proposed and is
not here**: it loads and activates, and fires on nothing — six shapes tried,
including its own documented one, with the callee made opaque so the nilness
analyzer could not fold the branch away. `nilerr` reports on those same shapes.
`durationcheck` took the slot: key lifetimes, agent timeouts and prompt budgets
are most of this program's arithmetic, and multiplying two durations gives
seconds squared.

**Six defects, all in code that compiled and passed.** Two imports buried in the
stdlib group. Two errors flattened into text with `%v`, so the cause was
readable and not reachable — `filepath.Match`'s verdict on a bad `key_patterns`
glob, and the open failure under `ErrNoTerminal`, which is what separates a
session that has no terminal from a terminal that would not open. One helper
without `t.Helper()`, reporting its callers' failures at its own line. And one
test helper that was dead code on macOS.

**The blind spot that made the last one invisible.** golangci-lint analyses one
build: the host's GOOS, no build tags. A file behind another platform's tag is
not skipped with a note — it is never looked at, so the macOS half of this tree
had never been through a Go linter at all, and neither had the two
failure-injection files. `lint-go` now names each build: linux, darwin,
`backend_unresponsive`, `midsession_failure`. Fourteen seconds warm, and no
macOS host needed for the darwin pass. Both new passes were confirmed able to
fail — an unused func appended to a `_darwin_test.go` and to a tag-gated test is
`0 issues` to a plain `run` and exits 1 under `make lint-go`.

**Six suppressions, each an argument rather than a silencing**, which is what
`nolintlint`'s `require-explanation` is for. Four are the `shortDir` helpers,
where `t.TempDir()`'s macOS layout overruns the 104-byte `sun_path` limit a unix
socket is bound under; two are `lockRealAgentTests`, where a temp directory
shared between test binaries is the entire mechanism and a per-test one would be
no lock at all.

**Deliberate "no"s, so they are not re-proposed as oversights.** `noctx` (10
findings, all `net.Dial`/`net.Listen` on local unix sockets with explicit
timeouts, where a context adds nothing). `gosec` (33: G115×8, G204×6, G30x×8,
G101×3, G703×3, G602×2) — worth doing and **deferred to its own activity**,
because almost every finding is inherent to what the program does (exec
`ssh-add`, read `~/.ssh`, chmod 0600) and each needs a justification, not a
blanket exclusion. `modernize` (27) is a production refactor wearing a linter's
hat. `exhaustive` (5) reports only missing zero-cases on switches that already
have the unreachable `default` rule 27 names. `perfsprint`, `tparallel`, `wsl`,
`nlreturn`, `varnamelen`, `lll`: churn. `testpackage` would break the internal
tests. Left on the table with real findings and no decision yet: `misspell`,
`dupword`, `predeclared`, `godoclint`, `unconvert`, `intrange`, and `recvcheck`
as the reserve if a slot opens.

**Verified**: `make lint-go` clean on all four builds, `make test` green across
all thirteen packages, `make build-cross`, `checkmake`, `markdownlint-cli2`.
No end-to-end run belongs to this phase — one user-visible surface changed and
it changed in text only: two error messages read exactly as before, and what
moved is what `errors.Is` can find underneath them.

### Phase 35 — Windows, and the shell that cannot eval

Windows is the last and most divergent target (goal 13). This phase does not
settle open decision 8's port depth: the agent endpoint (a named pipe served by
a service, not a socket) and the Credential Manager backend stay open. It
settles what has to be settled before any of that can be attempted — a binary
that compiles for `GOOS=windows`, a way to answer a shell that cannot `eval` a
Bourne assignment, and a login hook in the four places PowerShell keeps one.

**The dialect is a parameter, never inferred from the platform.** `shell-init`
and `askpass-env` take `--shell=posix|powershell`; no flag means posix, so
Linux and macOS are untouched. The per-dialect quoting stays in Go beside
`shellSingleQuote`, where a test can reach it, rather than in the hook, which
stays a single line. Inference would have been the cheaper-looking choice and
is the wrong one: a Windows binary invoked from Git Bash or MSYS2 wants posix,
and what shell is asking is not something `GOOS` knows. (The other half of
that story — `C:\…` against MSYS2's `/c/…` when a path is handed to `ssh` — is
not answered here.)

**Both PowerShells, both scopes — but one wiring point at a time.** Windows
PowerShell 5.x and PowerShell 6+ keep their profiles in different directories,
and all-users and per-user are different files again; each of those is then a
pair, one profile for every host and one for the host in front of you. All five
of the `$PROFILE` variants are addressable, and exactly **one** of them is
written per install: the same hook wired into several startup files of one shell
runs several times per session, which buys nothing and doubles what an uninstall
has to find. The default is the all-hosts profile of the scope being installed —
a working `ssh` is not a property of the window you are typing in — and
`--hosts=current` selects the other. The hook body is rendered once and the
chosen point holds one dot-source line, exactly as `install-user-hook.sh`
already does for bash and zsh.

**No PowerShell module.** It would replace four dot-source lines with four
`Import-Module` lines, and would itself have to be installed under a
`$env:PSModulePath` that differs per scope and per version — the multiplicity
moves rather than goes, and a second artefact has to be installed, versioned
and removed. A module earns its place when SSHakku has cmdlets to export, not
as a way to share ten lines.

**A `profile.d` is honoured where the user made one**, but not on quite the same
terms as bash. There the directory's existence is the only check, and having
made one is taken as saying it gets sourced — a shell or a distribution often
sources it. PowerShell sources no such directory by itself: only the user's own
profile can, so the profile is read for the code that does it, and where nothing
does, the block goes into the profile and says why. Not a priority either way —
the marker block in the profile is the mechanism that has to work.

- **W1 — a binary that compiles for `GOOS=windows`. ✅ Done.** `_windows.go` files
  (rule 26 — named for the platform, never a negation of another) for what the
  unix build supplies and Windows does not: `keyring`'s `Add`/`Read`/`Unlink`
  (no kernel keyring — the `_darwin.go` shape already says this), `paths`'
  environment and directory-ownership probes and its socket token, `agent`'s
  `platformAgents`, `keys`' `ErrNoTerminal`, `fetchPassphrase` and
  `boundToProcessGroup`. That list is the *first* build's answer and will grow:
  a package whose dependency failed to compile was never type-checked, so
  `paths`, `diagnose`, `config` and `cmd/sshakku` have not yet been heard from.
  Nothing here promises new behaviour — where Windows has no equivalent the
  stub says so and fails explicitly. Closes with `GOOS=windows` in
  `build-cross`, in `lint-go`'s per-build list, and in CI, so it cannot
  regress silently.
- **W2 — `--shell=powershell`. ✅ Done.** F43: `$agent_sock = '…'` and
  `$env:SSH_ASKPASS` in place of `export`, on the three commands that print for
  a shell — `ensure-agent` as well as the two named here, since a command that
  prints for a shell and cannot be told which shell is a hole rather than a
  smaller change. The quoting is not the doubled apostrophe this bullet
  predicted: PowerShell ends a single-quoted literal at any of **five**
  characters — `'` and the four curly quotes U+2018, U+2019, U+201A, U+201B —
  and each doubled stands for itself, identically on 5.1 and 7. Escaping only
  the apostrophe makes a home under `C:\Users\O’Brien` a parse error, which is
  an account name, not a curiosity. `SSH_ASKPASS_REQUIRE=force` becomes
  `'force'` on the posix side too: every value goes through its dialect's
  quoting, since PowerShell reads a bare word after `=` as a command to run and
  there is nothing to gain from deciding per value what may be left bare.
- **W3 — the hook and its wiring, as a command of this program.** The wiring
  becomes `sshakku install` / `sshakku uninstall` rather than a second
  `install-user-hook.ps1` beside the first. The design in full is
  `docs/INSTALLATION.md`, which is where a user reads it; what follows is why it
  is that and not something else, and what it costs.

  **Why Go and not a `.ps1` twin.** Three reasons, none of them taste. A
  Windows machine that has PowerShell has neither `make` nor `sh`, so a
  Makefile-driven install is unusable by exactly the person this step is for.
  Shell wiring logic is the thing goal 14 exists to move out of shell, and a
  second copy of it — in a third language, with its own test framework to bring
  in — is the opposite of that move. And the wiring is where the machine's
  variety lives (which editions, which scopes, which drop-ins, which
  translation), which is to say it is where the tests are needed, and Go is
  where this project can write them and run them on the Windows job that already
  exists. The Makefile stays the entry point for putting the **binary** in
  place, on every platform, and calls the command for the wiring; the unix
  wiring keeps its shell scripts until a later step retires them, so this step
  changes nothing on Linux or macOS.

  **The vocabulary is `--shell=auto|bash|zsh|powershellcore|windowspowershell`,
  and it is not the same word as `--shell` on the printing commands.** There it
  names a *dialect* to print in (W2); here it names a *shell to wire*, and each
  target renders in the dialect it reads. Each platform's table says which
  targets it has: no zsh on Windows, no `windowspowershell` anywhere else. Two
  more are possible and are deliberately not offered — PowerShell Core on
  Linux and macOS — and two are offered but are not what the suite holds: zsh
  on Linux, bash on macOS, which have always been wireable and are what
  `USER_SHELL` already selects. The line is drawn by the test matrix rather
  than by the code: every combination offered is a combination that has to be
  exercised, and the matrix is the thing that would explode.

  **Selection has two forms because the two are answers to different
  questions.** `--shell-exe=<interpreter>` says *ask that one*; `--profile=<file>`
  says *I know which file*. The first is what makes the design immune to the
  variety above — the interpreter is asked for its own `$PROFILE` set, so a
  `Documents` redirected into OneDrive, a Store or portable PowerShell, and a
  version installed beside another are all right without a single path being
  assembled. It is also what finds a Git Bash installation's own root, and so
  its `etc/profile.d`, without a fixed `C:\Program Files\Git` in the source.
  With neither flag the process's own ancestry is read until a shell is
  recognised, which is what makes `sshakku install`, typed in a window, wire
  that window's shell. Nothing is guessed: where the ancestry answers nothing,
  the command asks for `--shell`.

  **A POSIX-emulating shell does not see the paths this program writes**, and
  the mapping between the two spellings is that environment's own, so its own
  translator is asked rather than the mapping reproduced. `cygpath` is found
  beside the interpreter being wired — its neighbour, or one level up and back
  down through `usr\bin`, which is where Git for Windows keeps it — so no
  installation path is written into the source and a second environment
  installed alongside the first is not mistaken for it. MSYS2 falls under the
  first of those two places should it become a target; Cygwin is hypothetical
  and chased no further than costing one `Stat`.

  **The path is handed to the translator on its standard input, never as an
  argument**, and this was measured rather than reasoned. A program built for
  that environment re-parses the command line it is given under that
  environment's quoting rules, where an apostrophe opens a quoted string: a path
  through an account named `O'Brien` comes back with the apostrophe *silently
  gone*, naming a directory that does not exist — a hook that would then fail at
  every login with nothing to say why. `-f -` reads the path as bytes and it
  arrives whole, apostrophes, spaces and ampersands together. A path cannot
  contain a line ending on this platform, which is what makes one path to a line
  safe. Characters the platform forbids in a name (`" * : < > ? |`) are mapped
  by that environment into a private area of Unicode; no path arriving from this
  platform can contain one, so that mapping is never met and is not guarded
  against.

  **A PowerShell host is asked with `-File`, in the clear.** The alternatives
  were measured. `-Command -` on standard input silently swallows all but the
  first line of a multi-line script. `-Command` with the script inline keeps it
  readable but hands it to two quoting dialects in a row, Go's and PowerShell's.
  `-EncodedCommand` (base64 of UTF-16LE) works on both editions and touches no
  disk, and was used here first for exactly that reason — but it is unreadable
  to every party entitled to read it: the person running the install, an
  administrator looking at a process list, an audit log, and the endpoint
  protection deciding whether to allow it. It is also the only one of these
  forms that can never carry an Authenticode signature, so it leads away from
  the machines where scripts must be signed rather than towards them. The query
  stays a real `.ps1` in the tree, so rule 13 holds and PSScriptAnalyzer reads
  it; it is embedded, laid out in a private temporary directory for the call at
  0600, and taken back afterwards.

  Two measured traps ride along, one fewer than before. Windows PowerShell
  writes a CLIXML progress payload to stderr, so only stdout may be read; and
  its stdout is the OEM code page unless the script sets it, which mangles a
  non-ASCII path. The byte-order mark stopped being a trap when the encoding
  did: every `.ps1` here carries one because 5.1 reads a mark-less **file** in
  the machine's ANSI code page, which is exactly the case `-File` puts us in.
  Under `-EncodedCommand` the same mark made 5.1 print nothing and exit 0, a
  failure indistinguishable from a host with nothing to say; that hazard is now
  gone rather than guarded.

  **That `-File` is governed by the execution policy is the point, not the
  price.** It was the argument for encoding — the query would be blocked by the
  very thing it is diagnosing. But a policy that refuses this script file
  refuses the profile for the same reason, so the hook the install is about to
  write there could not run either. Failing at the query is therefore the
  earliest truthful report available, where bypassing the policy to read it
  would wire a hook that silently never starts. The failure is told apart by the
  error identifier PowerShell prints, `UnauthorizedAccess`, which is not
  translated as the message beside it is, and is turned into the sentence the
  user actually needs — the profile will not run either. This is pinned by
  driving a real host into a real refusal with `-ExecutionPolicy Restricted`,
  which outranks the account's own setting and so needs nothing changed on the
  machine running the tests.

  **What a host is started with is as much a decision as what it is asked.**
  Two variables describe the PowerShell that started *us* and must not reach
  the one we start. `PSModulePath` names where one edition keeps its modules:
  handed PowerShell 7's list, 5.x cannot resolve a module-qualified cmdlet at
  all — it reports that the module could not be auto-loaded and exits having
  printed nothing, which is the same shape as every other failure of this
  mechanism. `PSExecutionPolicyPreference` is the process-scope policy we were
  given, and passing it on makes the host report the asking session's policy as
  its own, when the question is what governs the sessions the user will start
  later. Both were caught by asking a real 5.1 rather than by reading.

  **Endpoint protection was the first evidence that the encoding was wrong.**
  Norton's behavioural heuristics flag `powershell.exe -EncodedCommand` started
  by a process that is not a shell, which is exactly what this used to do; on
  the machine this was written on it terminated the host mid-query, making
  "blocked" and "broken" indistinguishable from inside. Handing the script over
  as a named file removes the signature rather than arguing with it, and is the
  same move that makes the script signable. The e2e run (10) still belongs on a
  machine with such a product running, not only on a clean one — but it now has
  a shape to defend rather than a shape to excuse.

  **The two editions disagree about the policy on the same account**, which is
  the measurement that settles whether one host may be asked and the other
  assumed: on the machine this was written on, one reports `CurrentUser` as
  `RemoteSigned` and the other as `Bypass`, from separate registry keys. Each
  host is asked for itself, always.

  **`PATH` on Windows is made persistent, and that is the one thing here that
  outlives the shell.** Unix has a directory already on everyone's `PATH` for
  the machine scope and a hook line for the user one; Windows has neither, so
  the account's or the machine's environment is written. Through the registry
  directly, not through the .NET setter: that one expands `REG_EXPAND_SZ`, so a
  `PATH` written through it stops referring to the variables it referred to —
  the classic way an installer damages someone else's `PATH`. The raw value and
  its type are preserved, the entry is added once however many times an install
  runs, the previous value is kept so it can be put back, and uninstall removes
  our entry and nothing else. `--no-path` skips it. The registry key is a
  parameter, so the whole of that is testable against a throwaway key rather
  than believed.

  **An install that will never be read reports itself as such.** Execution
  policy (per edition, in force, with the command that changes it — never
  changed for the user), a `Profile.d` that no profile actually sources
  (PowerShell reads no such directory by itself, unlike the bash convention this
  borrows), an all-users profile under a Store install's protected directory,
  and constrained language mode. This is the promise with the most value in it
  and the one that could not be made without the measurements above.

  Rule 12: `.ps1` is a new file type here, and `PSScriptAnalyzer` (MIT, so rule
  16 is satisfied) becomes `lint-ps1` in `make lint` and CI, with the honest
  skip `lint-applescript` already models for a machine without `pwsh`. `.ps1`
  joins `.gitattributes`, and `docs/TEST-MATRIX.md` gains the rows (rule 19).

  Three promises are reserved for this work and are written into
  `docs/FEATURES.md` by the change that implements each, not before (rule 21):
  **F44**, installing wires the shell you ran it from, in one place, and says
  which file it touched, and uninstalling leaves no trace; **F45**, after
  installing, `sshakku` runs by name in a new session, and uninstalling takes
  that back too; **F46**, a shell that will never read the hook is reported at
  install time, with what it would take, rather than discovered at the next
  login.

  **Restating the shell library in Go found a defect in the shell library**,
  and the Go primitives deliberately do not reproduce it. `upsert_block` writes
  through `mktemp` and renames the result into place, and a rename carries the
  temporary file's mode with it: measured on Debian, an existing `0644` startup
  file comes back `0600`, and one the library creates is `0600` from the start.
  Per-user that is harmless; machine-wide it is not, since `WIRE_BASHRC=1 make
  install` upserts into `/etc/bash.bashrc`, and `make install` on macOS upserts
  into `/etc/zprofile` — both read at every account's login. The Go side sets
  the mode explicitly — the file keeps what it had, a new one gets `0644`, a
  drop-in gets `0755` — and three tests fail with `0600` when that is removed.
  It is the one place where "byte-identical" means the file's *contents* and not
  its mode.

  `shell-hook-lib.sh` now does the same, after the two install smoke scripts
  were made to fail on the mode first: the permissions are settled before the
  write and applied to the temporary file, since that file is the one the rename
  puts in place. The unwiring had the same defect and three copies of it — the
  `mktemp`/`strip-block`/`mv` dance was written out in the Makefile twice and in
  `install-user-hook.sh` once — so it became one primitive, `strip-block-file`,
  and the callers ask for that instead.

  Deferred here on purpose, so they are not read as oversights: the agent
  endpoint (see W4's findings), the askpass helper — wiring `askpass-env` on
  Windows today would trade a working console prompt for a helper that always
  fails, since the handoff is a stub there — the Credential Manager, MSYS2 and
  Cygwin as verified environments rather than as a design the discovery already
  fits, and retiring the unix shell installers in favour of this command. The
  interactivity test that gates `load-keys` — PowerShell has no `$-`, and it
  runs its profile for `-Command` and `-File` sessions too, so open decision 3
  binds harder here than anywhere — moves with `load-keys` to the step that
  wires it, since a gate on a command that is not called is untestable.

  **Where the binary goes on Windows, and who records it (step 11).** Until this
  step `make install` there wired a binary that stayed in the build tree: delete
  or move the tree and every session it had wired broke, and the `PATH` entry
  recorded named a repository. The split settled with the user is that the
  Makefile chooses the place — `%ProgramFiles%\sshakku` for the machine,
  `%LOCALAPPDATA%\Programs\sshakku` for the account, `BINDIR`/`USER_BINDIR` to
  say otherwise — and the wiring is then run from the copy it placed, so the
  directory `sshakku install` records is the installed one by construction rather
  than a second answer written down elsewhere and free to disagree. Rejected: a
  `sshakku path add|remove` command taking the directory as an argument (a second
  command and an F47 rewrite for nothing observable today), and the Makefile
  writing the registry itself (a second implementation of the one thing an
  install does that outlives the shell). `--no-path` stays, which is what an MSI
  will use when it manages that entry itself.

  Two traps found by writing it, both recorded in the Makefile itself.
  `$(PROGRAMFILES)` read inside a Makefile is the **x86** directory whenever make
  is itself a 32-bit program, which GnuWin32 make is — Windows redirects that
  variable per process — so the directory is asked of the shell through
  `cygpath`, which is not redirected. And `DESTDIR` cannot stage anything here: a
  path names its own drive, so prefixing one yields `…/stageC:/…` rather than
  either a staged tree or an error, and the Windows branch refuses it instead.
  F49 was added for the placement itself, which is user-visible on every platform
  and had lived only in a table in `docs/INSTALLATION.md`. The askpass helper is
  still not installed there, deliberately, and is the next step's subject.

  Sub-steps, each committable: (1) this design, in `docs/INSTALLATION.md`,
  `PLAN.md` and the matrix; (2) the hook and query `.ps1` files with `lint-ps1`;
  (3) the marker-block and drop-in primitives in Go, byte-identical to
  `shell-hook-lib.sh` and pinned by a test that says so; (4) asking a host,
  reading an ancestry, translating a path; (5) the Windows targets; (6) the
  persistent `PATH`; (7) the commands themselves, with `docs/CLI.md` and F44–F46;
  (8) `shell-init` reporting an unsupported platform through the log instead of
  failing every shell; (9) the Makefile's `MINGW*`/`MSYS*` branch; (10) the run
  on a real desktop (rule 25) across both editions and Git Bash, and the
  uninstall that leaves the files as they were; (11) `make install` putting the
  binary where this system keeps programs, and the `PATH` entry that then follows
  from where it was put.
- **W4 — run it** on `windows-*` runners (open decision 9) and on a real
  desktop session, driving the binary through a user's scenario (rule 25).
  **The suite half is done.** `go test (windows)` on `windows-latest` builds,
  vets and now runs every package under `-race`; `go test` rather than `make
  test` because that runner has no GNU Make, so the flags are kept in step by
  hand.

  What the suite failed on was three things, and only the first was what this
  entry originally said. Most of it was the platform's own vocabulary being
  written into tests as if it were universal: `/`-joined path literals against
  paths the product builds with `filepath`, `0600` asserted where mode bits are
  synthesised, `/tmp` in fixtures, a `.sh` as a stand-in editor, `/srv/keys`
  taken for an absolute path where a volume is what makes one. Second, and
  genuinely absent here: no wallet, so the promises that need one are held to
  the systems that have one, and no numeric uid, so `--user` is asked where
  there are uids. Third, two defects the port surfaced in shared code — a
  directory that is not there and one that is not a directory were told apart
  by the errno, which Windows spells differently, in both `Enumerator.Keys`
  and `dropInSources`.

  **The hang this entry blamed on a re-executed test helper was not that.**
  The three files that re-execute are all `//go:build unix` and never ran on
  Windows at all, so that explanation cannot have been the cause. On the
  machine where it was observed it was the antivirus: Go builds and runs test
  binaries under `GOTMPDIR`, which defaults to `%TEMP%` on `C:`, and a
  real-time scanner there left them started but never executing — unkillable,
  `CPU 0.00`, hours old. Pointed at a directory the scanner does not watch, the
  whole suite runs in three seconds, and forty-one under `-race`. Nothing in
  the product was involved, and nothing was changed for it.

  Still to do here: the real desktop session (rule 25). Windows joining the
  coverage and test-health reporting the other two platforms get — Phase 6
  item 1's "Windows once it exists" — is done, and cost less than this entry
  guessed: no column in `tools/testreport`, which never needed one. What it did
  cost is that the job runs under bash, since PowerShell splits
  `-coverprofile=coverage.out` at the dot and leaves `go test` writing a
  profile named `coverage` before failing on a package that does not exist.

- **W5 — the agent, the environment and the keys on Windows.** The endpoint is
  the system's own: a named pipe served by a machine-wide service, which
  `sshakku` probes with the agent's own handshake and starts when it is not
  running. The pipe has two writings and each shell is handed the one it can
  carry — measured, `\\.\pipe\…` does not survive a POSIX-emulating shell's
  environment while `//./pipe/…` reaches the same pipe from both. F50, F51 and
  F52 state what that promises; the askpass helper is installed and named the
  way a program is named here, the console is where a passphrase is asked for,
  and the handoff crosses on a socket under the account's own profile.

  **Key expiry is the one promise this platform cannot keep, and it needs a
  step of its own.** The agent here refuses `ssh-add -t` outright — the key is
  not added at all — and it keeps what it is given in `HKCU`, so a key added
  survives the session, the agent and the reboot. Today the key loads with no
  expiry and both the session log and `sshakku doctor` say the configured
  lifetime is not in force (F52), which is honest but is not the promise the
  other platforms keep. Three ways out, none of them free: **(a)** SSHakku
  expires the key itself, removing it with `ssh-add -d` when its time is up,
  which needs something alive to do it — a scheduled task, or the next session
  noticing on the way in, and the keystate record is already what would say
  when; **(b)** the login removes every key it added at logout, which is
  narrower than an expiry and needs a hook this project does not have here;
  **(c)** an agent of our own on an endpoint of our own, which the ssh-agent
  that ships with the system cannot be asked for and would mean keeping one.
  (a) is the only one that keeps the shape of the promise, and its cost is a
  piece of the product that runs when nobody is logged in. Phase 40 takes it up,
  and takes the half of it that costs nothing of the kind first.

**Out of scope here, named so they are not mistaken for oversights**: Credential
Manager and 1Password as Windows backends, WSL2 (Linux with an agent story of
its own), Cygwin and MSYS2 as environments this project has run in and checked,
MSI packaging (Phase 8's business), and translating the paths handed to `ssh`
between `C:\…` and MSYS2's `/c/…`. W3 translates paths for one narrower purpose
— reaching the startup files of a bash that spells them the other way — and does
it with that environment's own translator rather than with a rule of our own.

→ goals 13, 16, 17; open decisions 3, 8, 9 — 3 is settled for this shell in W5,
which asks the two things that can be known instead of the one flag it has not
got, and 8's agent half is settled by W5 as well: the port is the system's own
endpoint rather than one of ours.

→ rules 12, 15, 19, 23, 26; `docs/FEATURES.md` gained F50 (one endpoint, written
the way each shell reads it), F51 (a service that is not running is started, or
explained) and F52 (a key loads where the agent holds no lifetimes, and you are
told), and F48 kept its promise while its illustration moved to the wallet,
which is what this platform now has none of

### Phase 36 — The context that every call started over ✅ Done

This program waits on other people's software — a D-Bus daemon that may not
answer, `ssh-add`, a wallet CLI, a socket nobody is listening on — and it did so
with no way to say "stop, the caller has gone". Twelve `context.Background()`
calls, each one a root of its own, born where the wait was and reaching nothing
above it; below `main` there was no context at all, so a deadline set at the top
had nothing to travel down. Rule 28 states the shape that was missing: **one
root, created in `main`, and everything below inherits it.** Tests take theirs
from `t.Context()`, which the testing package cancels when the test ends, so a
call that hangs fails the run rather than outliving it.

**The root is a plain `context.Background()`, not `signal.NotifyContext`.**
Making Ctrl-C cancel the D-Bus call in flight is a promise to a user, and a
promise belongs in `docs/FEATURES.md` with a test that watches it fail first
(rules 21–23). This phase changes no behaviour anyone can observe: it is the
plumbing that such a promise would need, and it is deliberately left un-wired
so the refactor and the feature can be judged separately.

**Rule 12 decision.** `.golangci.yml` gains four linters, all of them already
inside the pinned golangci-lint, so `go.mod` is untouched and there is no new
licence to record (rule 16): **`contextcheck`** (a function that receives a
context and creates a new one instead of inheriting), **`noctx`**
(`exec.Command`, `net.Dial` and friends where a `*Context` form exists),
**`fatcontext`** (a context re-wrapped inside a loop), **`containedctx`** (a
context kept in a struct field, where it outlives the call it belonged to).

**This reverses Phase 34's deliberate "no" on `noctx`**, and the reversal is the
point rather than a change of taste. That "no" read: ten findings, all local
unix sockets with explicit timeouts, *where a context adds nothing*. It was true
only because there was no context to pass — with nothing above the dial, the
timeout was genuinely all there was. Once the root flows from `main`, the same
ten calls are the places where the caller's cancellation stops. A timeout ends
the wait; a context ends the work.

- **C1 — the rule and the record.** Rule 28 in `CLAUDE.md`, this phase in
  `PLAN.md`.
- **C2 — tests take `t.Context()`.** The test-side roots, and `usetesting`'s
  `context-background`/`context-todo` stated explicitly in `.golangci.yml`
  rather than left to the default.
- **C3 — the root and the dispatch.** `main` creates the one
  `context.Background()` and hands it to `dispatch`; `run`, `askpass` and the
  command bodies take it as their first argument. Plumbing only — nothing
  consumes it yet.
- **C4 — the secret backends.** `ctx` on `Lookup`/`Store`/`Delete`/`List`,
  across all eight implementations and every fake, which is where three of the
  production roots lived (the Secret Service calls) and where the KeePassXC
  dial is.
- **C5 — the prompts.** `Prompter`, `TTY`, the askpass broker, the passphrase
  handoff socket, pinentry, and the platform dialogs.
- **C6 — loading keys.** The `ssh-add` runner and the key-add path: the last
  production roots on Linux.
- **C7 — the agent and the doctor.** `EnsureAgent` and its runner,
  `net.DialTimeout` → `(&net.Dialer{Timeout: …}).DialContext`, the report
  gatherer with the two darwin roots (ancestry, host checks), the wallet view,
  the compartment maker, the cross-user token source, and the `$EDITOR` the
  `config --edit` path spawns.
- **C8 — enable the linters and close.** Each made to fire once before it is
  trusted (rule 23 applied to a linter: a check that has never reported is
  indistinguishable from one that is not running), then `make lint` on every
  build, `make test`, `make build-cross`.

**Two of them reported on this tree rather than on a throwaway.** `noctx`
named twenty-five calls, all of them real: the agent's dial, the wallet's,
the handoff's, the `$EDITOR`, the cross-user re-exec, and the processes the
integration tests start and used to leave behind. `contextcheck` caught a
defect in code written for this phase — a stubbed `execOutput` that was handed
a context and ran its command on the test's own instead, which is exactly the
shape rule 28 exists to stop, three days old and already there. `fatcontext`
and `containedctx` had nothing to say about this tree and were made to fire in
a throwaway package (a context re-wrapped in a loop, a context kept in a struct
field) before being trusted, then it was deleted.

**One behaviour changed underneath, and it is not a promise.** The Secret
Service prompt wait — the longest wait in the program, a person in front of a
dialog — watched only its own budget; the context reached it and was not read.
It now ends with the caller and dismisses the dialog on the way out, so a
prompt nobody is waiting for is taken off the user's screen. No user can
observe that yet, because nothing cancels the root: it is what makes a future
`signal.NotifyContext` a promise rather than a rewrite.

**Verified**: `make lint` clean on every build, `make test` green across all
thirteen packages, `make build-cross`. And, since a linter is not evidence that
a program works (rule 25), the real binary driven through a login's worth of
work in a disposable HOME: `ensure-agent` and `shell-init` adopted a healthy
agent and printed the assignments, `doctor` reported the agent, the keys and
the wallet — reached over the live session bus, compartment found — and
`load-keys` looked a passphrase up in the wallet, missed, raised the dialog,
and gave up on a dismissal without hanging the shell.

→ goals 14, 16.

→ rules 12, 16, 28; no feature in `docs/FEATURES.md` gained or lost a promise,
and `docs/TEST-MATRIX.md` is untouched: nothing here is visible to a user.

### Phase 37 — The machine nobody has touched ✅ Done

A Windows install could not be tested the way a person meets it. What SSHakku
writes there lives in a user's profile, in that account's stored environment and
in a service the whole machine shares, so a throwaway directory does not isolate
it and a second account does not either — and the state that matters most, a
machine on which nobody has installed anything, is the one state a machine that
has been developed on cannot be put back into.

**A native Windows container, and nothing Linux in it.** `containerd` with
`nerdctl` on the host, `mcr.microsoft.com/windows/servercore` as the machine: no
Docker Desktop, no WSL, no Linux container anywhere. The base image already
carries the OpenSSH this project drives — `ssh.exe`, `ssh-add.exe` and the
`ssh-agent` service, present and disabled, as on a machine nobody has set up — so
nothing is installed into it, and a scenario drives the same programs a user has
rather than substitutes chosen to make it pass.

**Isolation is an argument, because it is a property of the host.** Process
isolation needs the host's build to be exactly the base image's; a machine whose
build has moved past the newest image cannot use it and gets
`hcs::CreateComputeSystem: The request is not supported` for trying. Hyper-V
isolation gives the container a kernel of its own and does not care. The runner
defaults to the one that works anywhere and takes `-Isolation process` where the
builds match — a CI runner, whose build is the image's. The container command is
an argument for the same reason: `nerdctl` and `docker` are two drivers of the
same containerd underneath, so a machine with either can run this.

**No new linter to choose.** Neither file kind is new to the tree: the Dockerfile
is picked up by `lint-docker`'s existing wildcard and the scenarios by
`lint-ps1`. But `PS1_FILES` stopped at `tools/`, so a PowerShell file under
`test/` would have gone unlinted without anything saying so; that glob now
reaches `test/`.

**What it found on its first real run.** `sshakku install` failed on an account
that had never had a PowerShell profile: a startup file is replaced through a
temporary file beside it, and `Documents\WindowsPowerShell` was not there to hold
one. `Documents` was. The fix is `UpsertBlockFile` making the directory it is
about to write into, which is what its own doc comment already promised.

Three unit tests changed with it, and they are why this was never caught: each
used "a directory that is not there" as a convenient way to make a write fail, so
all three agreed with the code about a case the product gets wrong. They now use
a directory that *cannot be made* — a file sitting where it would go — which is a
genuine failure and still names the file it could not wire.

**Verified**: the real binary driven through the scenario in a throwaway
container. Red first — exit 1, nothing written — then green, the install
reporting the interpreter, the profile, the hook and the recorded `PATH`, and the
profile holding one block dot-sourcing a hook that is there. `go test ./...`
green and `golangci-lint` clean. `make lint` was not run whole: it wants a POSIX
shell and tools this Windows host has not got, so `lint-ps1` and `lint-docker`
were run directly instead.

→ features F44; rules 12, 19, 22, 23.

### Phase 38 — The agent that is not running ✅ Done

F51 promises two things about an agent that is a service: one that is stopped is
started, and one that cannot be started names the command that puts it right.
Neither state could be reached on a machine that works on this project, because
the agent there is running — which is what makes the promise invisible. Both
were held by unit tests against a scripted service manager and by runs somebody
did once, by hand, and remembered.

**The base image ships the state.** `servercore` carries the `ssh-agent`
service present, stopped and disabled, so the harder of the two states costs
nothing to build: it is what the machine is when it starts.
`windows-agent-service-scenario.ps1` drives both, in the order a person meets
them — the refusal first, then the service it can start once the refusal has
been acted on.

**The command it names is run, not matched.** "Named in full rather than
described" is a promise about a string being *usable*, and a comparison cannot
tell those apart: a sentence explaining what an administrator ought to do
matches a pattern just as well as a command does, and only one of the two
survives being executed. The scenario takes `Set-Service ssh-agent -StartupType
Automatic` out of the message, runs it, and requires the machine to have
changed. That run is also what carries the machine into the second state, so the
two halves are one journey rather than two fixtures.

**A native program's standard error must be read from a file.** Reaching this
shell directly, it arrives as an error record wrapped to the width of a console,
which split the one-line message into `Set-Service ssh-agent` and `-StartupType
Automatic` — two lines, neither of them a command. The scenario runs everything
through `Start-Process` with both streams redirected to files, so what it judges
is what was written rather than how a host chose to render it.

**A promise with nothing behind it.** F51 also says the person whose shell it is
gets told. They do not: the hook runs `shell-init` with `2>$null`, so the
sentence reaches the session log and `sshakku doctor` and never the terminal. A
user whose agent service is disabled is asked for every passphrase, every time,
with nothing on screen ever saying why. The unit tests do not see it because
they call `shell-init` directly, where the standard error is right there — the
hook, which is the thing that discards it, is not in their path.

The fix has an argument against it that is worth keeping: a session being driven
by a script — a build step, a scheduled task — should not find diagnostics in
its standard error. The hook already works out whether somebody is sitting at
the session, but only after the call that would print. Moving that decision
above the call is the shape that keeps both.

**Two things are still to do, and the second is a question rather than a task**:
the fix itself, and whether a suite can watch a session's screen at all — open,
not settled, and not to be called manual by nature before it has been
established. Both are Phase 39.

**Integration coverage on this platform is early on purpose.** The port itself is
young, and scenarios are not where the next effort goes. What is not negotiable
is that every gap is written down where it can be seen: a promise nobody has
tested belongs in the matrix as an uncovered cell, not in somebody's memory, so
that the work of covering it can be picked up rather than rediscovered.

→ features F51; rules 19, 21, 22, 23, 25.

### Phase 39 — The line that never reached the screen ✅ Done

F51 says the person whose shell it is gets told what to run. On this platform
they were not: the hook ran `shell-init` with `2>$null`, so the one sentence
naming the command reached the session log and never the terminal. A user whose
agent service is disabled paid a passphrase for every connection with nothing on
screen saying why.

**The answer was already in the file, one call too late.** The hook works out
whether somebody is sitting at the session — nothing on its command line saying
it was handed work, and standard input not being fed by something else — but it
did so below the call that would have printed. Moved to the top, that one answer
now decides both of the things that turn on it: whose keys are loaded, and who is
told. A session a person opened sees the sentence; one handed work still sees
nothing, because its standard error is a script's own output, and a script that
starts failing because SSHakku had something to say has been given a new problem
in place of the one it was being told about. Both sides went into F51, which had
promised only the first.

**Which failures are worth saying out loud is not the hook's to decide.** The
binary already judges that: what nobody can act on it keeps to the session log —
a platform with no agent mechanism at all says so there and nowhere else — and
what somebody can act on it puts on standard error. A hook that filtered that a
second time would be a second opinion in a second place, free to drift from the
first.

**Passed through, never captured.** The stream is left alone rather than read
into the shell and written out again. Standard error read back arrives as an
error record, and that is two hazards at once: it is rendered wrapped to the
width of the console, which breaks a long message across lines and leaves a
command named in one no longer a command anybody can paste; and in a session that
has asked for errors to stop it, `2>&1` on a native command throws. Both editions
were measured: pwsh carries on, Windows PowerShell does not. This file is
dot-sourced into a session somebody else arranged, so it does not get to end that
session's startup.

**A suite can watch what a session printed — the question Phase 38 left open.**
It took three container runs to answer, and the first two answered something
else. A session started with any of its streams captured is handed the standard
handles the scenario itself was given, and in a container those are pipes:
`IsInputRedirected` comes back true and the product concludes, correctly, that
nobody is sitting there. So the session is started through `cmd`, asked for the
one thing this shell cannot do — give the session a console of its own for input
while pointing its error stream at a file. Then the product sees a person, and
the sentence is in the file where the scenario can read it. Ending such a session
takes ending the process: `exit` in a profile ends the profile, and the session
goes on to its prompt to wait for input that cannot come.

**The check passed while the screen was empty.** Written first and run against
the unfixed hook, it went green: the property holding what the session said is
empty when it said nothing, and an emptiness compared with `-notmatch` answers
with an empty collection rather than with true. Read as a string it fails, which
is what it then did. A test that has never been red proves only that it agrees
with today's code — this one agreed with code that was wrong, and the only reason
that was ever noticed is that it was made to run before there was anything to
find.

**A comment describing a file the file had stopped being.** The header said that
adding the user's keys was deliberately absent here, and had gone on saying it
after the hook started adding them. It now says where the one answer both of them
turn on is worked out.

→ features F51; rules 15, 19, 21, 22, 23, 24, 25.

### Phase 40 — The key that outlived every session

W5 named this as the one promise this platform cannot keep, and left it needing a
step of its own. This is that step. The agent here refuses `ssh-add -t` outright,
so a key is added with no expiry at all, and it is kept in `HKCU` — which means
the key outlives the session that loaded it, the agent, and the reboot. A user
who wrote `key_lifetime` in their configuration has it enforced by nobody: on the
other two platforms the agent drops the key, and here nothing ever does.

**SSHakku expires the key itself, at the start of every session.** Of W5's three
ways out, this is (a), and the cheaper half of it: the session on its way in
notices what has run out of time and takes it out of the agent, rather than
something staying alive to do it at the deadline. What that buys is the whole of
the reboot — a key can no longer survive one — and what it does not buy is the
stretch while nobody logs in, where a machine keeps the key until somebody opens
a shell. The other half, a scheduled task that runs when nobody is logged in, is
a piece of the product that runs with nobody in front of it: it is deliberately
**deferred until the secret backends for this platform exist**, since a key that
must be re-typed by hand at every expiry is a worse bargain than a slightly late
expiry, and the backends are what make the exchange fair.

**In every session, not only the ones somebody is sitting at.** `load-keys` runs
in interactive sessions alone, which is right for asking questions and wrong for
this: a key past its time is past it whoever opens the next shell, and a machine
that mostly runs build steps would keep it for weeks. So the removal goes with
`shell-init`, which every session runs, and says nothing on the stream that
session is evaluating — the session log is where it is written down.

**The record has to hold what was configured, not what the agent was told.**
`effectiveKeyLifetime` zeroes the lifetime where the agent holds none, and the
keystate record was written from that, so every record on this platform says "no
expiry" — there was nothing to expire against. The record now keeps the
configured lifetime and means what it always read as: this key is meant to live
until then, whoever turns out to enforce it. `doctor` on this platform therefore
stops saying a key has no expiry and gives it a remaining time, which becomes
true in the same change.

**It also has to say where the key is.** `ssh-add -d` names a file; the record
was one file per key basename holding an instant and a duration. It gains the
key's path, which makes expiry a directory read that needs neither the
configuration nor the enumerator, and keeps `shell-init` as cheap as it was.
Records already written, with no path in them, still parse and are simply not
acted on.

**Only what SSHakku added, and only where the agent holds nothing.** A key the
user added themselves has no record and is never touched, whatever its age. And
the whole mechanism is gated on the platform answer that already exists — the
agent that keeps lifetimes keeps them, and nothing here runs on Linux or macOS.
The answer arrives as an argument rather than as a build tag around the logic, so
both outcomes stay checkable from either machine (rule 26).

Sub-steps, each committable: (1) the promise, in `docs/FEATURES.md` — F52
rewritten, F53 added — and this phase; (2) the record keeping the configured
lifetime and the key's path; (3) the expiry mechanism itself; (4) its wiring into
`shell-init`; (5) the matrix row and the real binary driven through a key that
runs out of time, in the Windows container.

→ features F52, F53; PLAN Phase 35 W5; rules 19, 21, 22, 23, 25, 26.
