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

8. **Thin platform ports (goals 12, 13).** macOS already does silent passphrase
   caching natively via launchd + `ssh-add --apple-use-keychain`; the macOS port
   may reduce to "add keys with keychain". Windows is the most divergent
   (service + named pipe, no socket) — keep it last.

9. **CI vs containers for non-Linux (goal 16).** Use GitHub Actions `macos-*`/
   `windows-*` runners for those platforms; keep Linux containers for the rest.

10. **Phasing (rules 1, 9). Decided:** bash/Go split — Phase 1 ships only the
    permanent shell plumbing; the branchy, stateful logic moves to a Go core grown
    incrementally (strangler), slice by slice, rather than one wholesale rewrite —
    see Phase 2. The diagnostic tool follows the core (Phase 3).

11. **CI least-privilege & lint coverage (rule 14, 12). Decided (Phase 0):**
    `make lint` runs `shellcheck`+`shfmt`+`markdownlint-cli2`+`checkmake`+
    `actionlint`(+`editorconfig-checker`, `golangci-lint`, `taplo`, `hadolint` as
    each file type entered the repo); CI declares `permissions: contents: read`
    and invokes the same `make lint`. See the per-file-type table under Phase 0.

12. **Install modes & path layout (goals 17–19). Decided (step 1.1):** config
    **and** the session log live under `${XDG_CONFIG_HOME:-~/.config}/sshakku`;
    the agent socket resolves `$XDG_RUNTIME_DIR/sshakku` →
    `/run/user/$UID/sshakku` → `${XDG_CACHE_HOME:-~/.cache}/sshakku`, with an
    unpredictable per-login `@u`-keyring token as a path component (defense in
    depth above the `0700` boundary). The per-user bootstrap hook (`~/.bashrc` vs
    a desktop-session env file) stays open.

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
| Shell — macOS (`*.zsh`) | Linting deferred to the macOS phase |
| Markdown (`*.md`) | `markdownlint-cli2` (config `.markdownlint-cli2.yaml`) |
| Makefile | `checkmake` (config `checkmake.ini`) |
| YAML / GitHub workflows | `actionlint`; other YAML/INI/JSON has no dedicated linter — `editorconfig-checker` covers charset/EOL/indent/final-newline |
| All committed files | `editorconfig-checker` (config excludes `LICENSE` verbatim, `*.zsh`, and `*.go` — gofmt owns Go formatting) |
| Shell — bats tests (`*.bats`) | Deferred until test files enter the repo |
| Go (`*.go`) | `gofmt -l` + `go vet` + `golangci-lint` (config `.golangci.yml`); `golang.org/x/sys` (BSD-3-Clause) recorded in `COPYRIGHT.md` |
| TOML (`*.toml`) | `taplo lint` + `taplo format --check`; runtime parser `github.com/BurntSushi/toml` (MIT) recorded in `COPYRIGHT.md` |
| Dockerfile (`test/containers/*.Dockerfile`) | `hadolint` (config ignores DL3008 — no viable apt-pin story against a rolling suite; the base image tag is the point-in-time anchor) |

### Phase 1 — Harden the primary target: shell plumbing (still bash)

Gentoo / OpenRC / KDE. Scope narrowed by the bash/Go split (open decision 10):
only the **permanent shell plumbing** stays in bash; the branchy, stateful logic
moved to the Go core in Phase 2 instead (1.3's seam and 1.4's lifecycle both ended
up as Go slices there — see Phase 2). → goals 3, 5, 6, 10, 17–19; open decisions
3, 4, 12.

- **1.1 — XDG path layout, out of `~/.ssh`.** → goal 19; open decision 12.
- **1.2 — Two install modes + bootstrap hook.** → goals 17, 18; open decision 12.
- **1.3 — Silent-on-success & shell safety.** Superseded by the Go seam (`eval
  "$(sshakku shell-init)"`); the remaining bash-side work is `set -u` hardening.
- **1.4 — Agent lifecycle & recovery.** Moved into the Go core — see Phase 2 slice
  2.
- **1.5 — Shell test harness (rule 12).** `bats` + tier-1 containers (open
  decision 20). Plumbing regression checklist, still the right manual check after
  any lifecycle change:
  1. Fresh login, two terminals: both see the key in `ssh-add -l`, no second
     prompt.
  2. `SSH_AUTH_SOCK` is the fixed socket path everywhere, including a GUI app.
  3. Kill the agent → a new terminal restarts it at the **same** socket and
     reloads the key.
  4. First-ever run, empty vault: prompts once, silent thereafter.
  5. A reachable-but-empty agent (`ssh-add -l` exit 1) is **healthy**, never
     killed.

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
  `tier2-desktop.yml` (`workflow_dispatch`-only), starting with a KDE row
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
    `keepassxc-tier2-create-collection.sh` runs a persistent watcher answering
    both the one-time creation wizard and every later unlock.
  - **1Password** — `OnePasswordBackend` shells out to `op`; no in-place item
    edit without argv/file exposure, so `Store` deletes and recreates from a
    stdin JSON template. Unlike the other three, a 1Password account is a cloud
    account, not a disposable local daemon, so it has no container tier: a
    dedicated service account ("SSHakku") authenticates in CI via
    `OP_SERVICE_ACCOUNT_TOKEN` (`op user get --me`, not `op whoami`/`op signin`,
    both unsupported for service accounts) — `tier2-onepassword.yml`,
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
    (`test/containers/vaultwarden-tier2-fixture/`) rather than re-registered
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

### Phase 5 — Widen the OS targets

macOS as a wide port, never trust Apple; then Windows last as the most divergent target (service + named pipe, no socket, use win32 safe API). → goals 12, 13; open decision 8.

### Phase 6 — Full test matrix

Extend CI to macOS and Windows runners and complete the cross-platform test
matrix. Tier 2 of open decision 20 (real-desktop-secret-stack containers) was
brought forward to Phase 4.1; this phase adds tier 3 (the full Vagrant
Gentoo/OpenRC/KDE box) as a manually-triggered CI workflow. → goal 16; open
decisions 9, 20.

### Phase 7 — CI review & dependency hardening

A final pass over the whole CI once it spans every platform and language. Audit
each workflow for least-privilege `permissions:` (rule 14), de-duplicate the
lint/test jobs, add dependency caching and sensible `concurrency`, and confirm
`make lint` and the test suites stay the single entrypoints CI invokes. Settle
dependency automation: choose Dependabot vs Renovate (open) and extend it to
*every* ecosystem — `github-actions`, `gomod`, `npm` — so the lint-tool versions
pinned by hand in Phase 0.4 become auto-managed once the `go.mod`/`package.json`
manifests exist. Pin all third-party actions by full commit SHA with version
comments, and pin tool/runtime versions (Go, Node, the linters) for reproducible
builds. Re-evaluate per-file-type lint coverage (rule 12) against whatever file
types the repo has grown by then. Also rename `test/containers/*-tier2-*`
files to drop the `tier2` infix — it leaked an internal test-strategy label
(open decision 20) into filenames meant to describe *what* each script does,
not *which test tier* runs it; update the workflows and any doc references at
the same time. → goal 16; open decisions 9, 11, 20; rules 12, 14.

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

### Phase 10 — Documentation pass & Linux hardening guide

A README and `docs/` overhaul aimed at an end user, not a contributor: explain
every feature, every `config.toml` key, and every secret backend in one place
a first-time reader can follow start to finish (today's docs grew
incrementally, phase by phase, and were never reviewed as a whole for a
newcomer). Alongside it, a best-practices guide for things outside sshakku's
own control that materially strengthen its threat model: a short key TTL, not
leaving the desktop wallet permanently unlocked, full-disk encryption, and a
properly configured `/tmp` — cross-referencing Phase 9's new `doctor` checks
for the ones doctor can detect itself. → goals 2, 11, 15; open decision 1.
