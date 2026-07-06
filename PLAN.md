# SSHakku — PLAN

Roadmap for the rewrite. We fix the **goals** first; the **phases** come after the
goals are reviewed and agreed. See `CLAUDE.md` for the project rules and
`docs/THREAT-MODEL.md` for the threat model and the June 2026 incident that
motivated the rewrite.

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
honoured) before or during the phases. Each notes the related goal.

1. **Threat model (goal 2, 7).** State, in two lines, what the secret handling
   protects against and what it does not. The user keyring (`keyctl @u`) and the
   secret store do **not** protect against other processes of the same user — by
   design, since those processes must be able to use the key. Decide the target
   (other local users / root / swap & coredumps / logs) — it drives the design.
   **Decided (Phase 0) — see `docs/THREAT-MODEL.md` (source of truth).** In two
   lines: *protects* the passphrase from logs, shell history, `argv`
   (`ps` / `/proc/<pid>/cmdline`) and plaintext on disk — at rest only in the OS
   secret store, in transit only via a short-lived `keyctl` entry / stdin.
   Same-user processes, root, swap/coredumps and physical access are **enumerated as
   deferred decisions, not excluded by design**: each is settled per threat and
   confirmed at a final security evaluation.

2. **No passphrase in `argv` (goal 2).** Never pass the passphrase as a
   command-line argument (visible via `ps` / `/proc/<pid>/cmdline`). Feed it
   through stdin instead (e.g. `keyctl padd … <<<"$passphrase"`). Audit every tool
   invocation that touches the passphrase.

3. **"Silent" means zero stdout/stderr when non-interactive (goal 3).** Anything
   sourced from `profile.d` runs for non-interactive SSH sessions too; a single
   byte on stdout corrupts `scp` / `rsync` / `git`-over-ssh. The success path must
   emit nothing on stdout/stderr — only the log file.

4. **Recovery has a hard limit (goal 6).** A child process cannot rewrite the
   environment of an already-running parent (the session / GUI). "Fix mismatched
   env vars" can only fix the current shell and its descendants; already-open GUI
   apps are reachable only via the fixed socket path (plus a dangling-socket
   symlink as a last resort). Don't promise more. The same symlink is how a healthy
   foreign agent is adopted (open decision 15): the fixed path points at the
   foreign socket so the session's pinned `SSH_AUTH_SOCK` resolves to it.

5. **Give-up state & opt-out (goal 4).** Bounded retries need a persistent text
   sentinel ("gave up on key X") with a defined reset (next login? time-based?) and
   an opt-out switch (config flag / sentinel file). Define lifetime and reset rule.

6. **Key-expiry semantics (goal 2).** Confirm: expire the *key inside the agent*
   (`ssh-add -t <lifetime>`), keep the passphrase in the vault, and let a new shell
   re-add it silently — rather than expiring the stored passphrase itself.

7. **Secret backend abstraction (goal 11).** KDE and GNOME are the *same* backend:
   both implement the D-Bus Secret Service API (`secret-tool`/libsecret). The real
   backends are ~4: `secret-service` (KDE + GNOME), macOS Keychain, Windows
   Credential Manager, 1Password CLI (`op`). Define a `SecretBackend` interface
   early — it is also what makes integration tests mockable (goal 16).

8. **Thin platform ports (goals 12, 13).** macOS already does silent passphrase
   caching natively via launchd + `ssh-add --apple-use-keychain`; the macOS port
   may reduce to "add keys with keychain", so avoid over-engineering it. Windows is
   the most divergent (service + named pipe, no socket) — keep it last.

9. **CI vs containers for non-Linux (goal 16).** Use GitHub Actions `macos-*` /
   `windows-*` runners for those platforms (more realistic than containers); keep
   Linux containers for the rest, noting that `keyctl` / D-Bus need setup there —
   another reason for the mockable backend interface.

10. **Phasing (rules 1, 9).** Harden the primary target first (Gentoo / OpenRC /
    KDE), then the Go core, then widen to other backends and OSes; each step stays
    committable. **Decided:** the "possibly still in bash" hedge is settled as a
    **bash/Go split** — Phase 1 ships only the permanent shell plumbing in
    cleaned-up bash (paths, install modes, silence, agent lifecycle) as a stable
    baseline, and the branchy, stateful logic (retries / give-up, key-expiry,
    Wayland detection, secret-handling) moves to the Go core in Phase 2, written
    once rather than re-written from throwaway bash. The diagnostic tool follows
    the core (Phase 3) so it reuses Go primitives. **Brought forward:** rather than
    a wholesale Phase 2, the Go core is grown incrementally (strangler) starting
    with the path/token/dir/log slice, so no more throwaway bash accumulates — see
    the Phase 2 note for the slice breakdown.

11. **CI least-privilege & lint coverage (rule 14, 12).** The existing
    `.github/workflows/linting.yml` has no explicit `permissions:` block (it runs
    on the repository default). Add a least-privilege block — verifying which
    scopes `reviewdog`/`shfmt` actually need before tightening, so CI doesn't
    break. While there, decide the lint story: wire a `make lint` target
    (shellcheck + a Markdown linter) and align CI with it. Go and a Markdown linter
    will be new file types needing a lint decision.
    **Decided (Phase 0):** `make lint` runs `shellcheck` + `shfmt -d` +
    `markdownlint-cli2` + `checkmake` + `actionlint`; CI declares `permissions:
    contents: read` and invokes the same `make lint`, replacing the per-tool actions
    (which would need write scopes for inline annotations). Per-file-type lint
    decisions are recorded under Phase 0.

12. **Install modes & path layout (goals 17–19).** Realise the two install modes
    and the XDG/FHS path layout in Phase 1 (steps 1.1–1.2) — config in `/etc` or
    `$XDG_CONFIG_HOME`, state/logs in `$XDG_STATE_HOME`, agent socket in
    `$XDG_RUNTIME_DIR`, nothing under `~/.ssh`. Open within: the per-user mode can't
    write `/etc/profile.d`, so its bootstrap hook moves to `~/.bashrc` /
    `~/.config/plasma-workspace/env/` — pick the per-user hook in step 1.2.
    **Decided (step 1.1):** config **and** the session log live together under
    `${XDG_CONFIG_HOME:-~/.config}/sshakku` (one discoverable tree, not the
    `$XDG_STATE_HOME` split sketched above). The agent socket goes in the per-user
    tmpfs, resolved independently of the desktop/display server:
    `$XDG_RUNTIME_DIR/sshakku` → `/run/user/$UID/sshakku` (probed, owned by us)
    → `${XDG_CACHE_HOME:-~/.cache}/sshakku` when no logind exists. An
    unpredictable per-login token from the `@u` user keyring is inserted as a path
    component (`<runtime_dir>/<token>/agent.sock`) so the path is not reproducible
    across logins/reboots — defense-in-depth above the ownership+`0700` boundary;
    it degrades to the plain runtime dir when `keyctl` is absent. Deferred to the
    Go core (which owns path computation behind the entrypoint seam): keyring via
    syscalls (no `keyctl` binary), a `/dev/shm/sshakku/$UID/<token>/` tmpfs
    fallback with parent-validation (`lstat`/owner/no-symlink) + a `tmpfiles.d`
    entry for the system install, optional `boot_id` rotation for the `~/.cache`
    fallback, and optional per-login agent isolation as a config flag. The
    per-user bootstrap hook stays open for step 1.2.

13. **Which keys to auto-load is configurable (goals 1, 2, 15). ✅ Done.**
    `config.toml` gains three keys, config-file only — no `SSHAKKU_*` twin, same
    reasoning as decision 18's wallet-store keys: `auto_load_mode` (`"all"`
    default, `"include"`, or `"exclude"`), `auto_load_include`, and
    `auto_load_exclude`. The mode is authoritative, so the two lists never
    conflict, mirroring `StoresWallet` exactly
    (`config.Settings.AutoLoads(keyname) bool`). `keys.Config` gained an
    `AutoLoad func(keyname string) bool` predicate (nil loads every key,
    preserving prior behaviour), checked at the top of `Loader.loadOne` — before
    the fingerprint lookup, so an excluded key never runs `ssh-keygen` or
    touches the agent at all. A security-conscious user narrows the auto-load
    set to shrink the agent's blast radius (A2): fewer keys sitting in the agent
    means fewer credentials exposed to same-user processes and to agent
    forwarding. Independent of decision 18: an auto-load-excluded key is not
    proactively added at shell-init, but the askpass broker still answers for it
    normally on demand (e.g. `ssh -i`), since the broker never calls
    `Loader.LoadKeys`.

14. **Project name (goal identity).** **Decided:** the project is named
    **SSHakku** (from Akkadian *iššakku*, a steward who administers an estate on
    behalf of its owner — here it tends the SSH agent and guards the keys, pulling
    each passphrase from the OS secret store; the name also surfaces `ssh`). It
    replaces the original `ssh-profile-config`, which mislabelled the tool as an
    `~/.ssh/config` manager (it manages neither SSH connection profiles nor
    `~/.ssh/config`) and described the bootstrap mechanism (`profile.d`) rather than
    the purpose. An earlier working name, *sshepherd*, was dropped to avoid a clash
    with an active registered trademark (FullArmor's **SSHepherd®**) in the
    SSH-security space. The `<name>` placeholder in the path layout (goal 19, open
    decision 12) resolves to `sshakku`. A short command alias `shak` is to be
    provided by the CLI when it lands. The GitHub repository and the Gentoo package
    are renamed to match.

15. **Agent lifecycle: self-healing & foreign-agent adoption (goals 5, 6, 8).** At
    shell-init the world is in one of five states; sshakku resolves them in
    precedence order rather than only "never kill a healthy agent":

    - **A — clean** (nothing reachable): reap any dead socket at our path, start
      *our* agent on the fixed socket, load the keys.
    - **B — ours healthy** (agent on our fixed socket): attach, load only the
      missing keys (fingerprint dedup), stay silent.
    - **C — ours zombie** (our socket/process dead, including the legacy
      `~/.ssh/agent`): reap what is ours, restart on the fixed socket.
    - **D — foreign healthy** (a reachable agent we did not start, env points
      elsewhere): never spawn a competitor — adopt it and **report the anomaly**.
    - **E — disaster** (mixed stale env, dead sockets, several agents): be
      maximally resilient — use any healthy agent (ours first, then a foreign one
      with a report), reap the dead, never leave the shell on a dead socket.

    Identity: "ours" = the agent listening on our fixed socket (PID recorded in a
    state file when we start it); "legacy-ours" = `ssh-agent -a ~/.ssh/agent/…`;
    anything else is foreign. The hard limit of open decision 4 still holds — we fix
    only the current shell and its descendants; already-open GUI apps are reached
    only via the fixed-socket symlink.

    **Decided (this discussion):**
    - **Adopt a foreign healthy agent by symlink** (case D): point the fixed socket
      at the foreign socket (`fixed → foreign`), keep `SSH_AUTH_SOCK` on the fixed
      path, and load our keys into the foreign agent — accepting that this widens
      the keys' blast radius, which is exactly why it is reported as an anomaly, not
      the steady state. If the foreign agent dies the fixed path is left dangling
      and handled like any other dead socket.
    - **Reap dead foreign sockets/agents too**, not only ours — but *only the dead*
      (no listener / unreachable process); a healthy foreign agent is never killed
      (that is case D). Automatic reaping stays within the invoking user's
      privileges (rule 18); deeper cleanup across users is the diagnostic tool's job
      under `sudo`.

    The reporting/attribution side — who started the foreign agent, and how to
    return to the clean state where only we run the agent — is the diagnostic tool's
    mandate (goal 8, Phase 3).

17. **Scoped, explicit-lock unlock window per collection (goals 2, 11; open
    decision 7; threat I6). ✅ Done.** Gave sshakku its own Secret Service
    collection (label and alias `sshakku`), separate from the desktop's default
    (`kdewallet` / login keyring). Rather than relying on the desktop's fixed idle
    timeout to bound the unlocked window, sshakku unlocks only its own collection
    right before a lookup or store and locks it again immediately after —
    collapsing the exposure window from minutes to the seconds the operation
    actually takes. `secret-tool` only ever targets the default collection and has
    no lock/unlock verbs, so this needed a native D-Bus Secret Service client
    (`internal/secretservice`, using `github.com/godbus/dbus/v5`) rather than
    shelling out; `SecretServiceBackend` (`internal/keys/secret.go`) wraps it
    behind the existing `SecretBackend` interface and is now the default in
    `cmd/sshakku`, falling back to `SecretToolBackend` if the D-Bus session bus is
    unreachable (the same underlying dependency secret-tool always had, so no new
    failure mode). Verified live against the user's `ksecretd`: `CreateCollection`
    with a custom alias works, and `Unlock` can complete either synchronously or
    via the async `Prompt`/`Completed` flow depending on session state, so the
    client handles both — confirmed by a white-box test suite
    (`internal/secretservice`) exporting a fake Secret Service over a private
    `dbus-daemon` session bus, reproducing every completion mode observed live.
    Does **not** close threat I6 (`docs/THREAT-MODEL.md`: any process of the same
    UID can still query the collection while it happens to be unlocked, and a
    process killed between unlock and the deferred lock leaves it open until the
    desktop's idle timeout) — it only shrinks the window during which that
    exposure exists. **No migration** from the old default-collection storage: an
    already-stored key re-prompts once on the first load after upgrading, then
    lands in `sshakku` and behaves normally — simpler than a dual-collection read
    fallback or a copy/delete migration, for a single-user deployment today.
    **Follow-up (rule 2, noticed in passing):** the Go test suite had no CI
    entrypoint (`linting.yml` only ran `make lint`); added `make test` (`go test
    -race ./...`) and a `test.yml` workflow so it actually runs on push.
    **Follow-up (bug report): batch the unlock across `load-keys`.** Per-key
    unlock/lock meant a shell with N keys prompted the wallet password up to N
    times. `SecretServiceBackend` gained an explicit `Unlock`/`Lock` pair
    (`SecretSession` interface) that a caller can hold across several
    `Lookup`/`Store` calls; `Loader.LoadKeys` uses it to unlock lazily on the
    first key that actually needs the wallet and lock once after the whole
    batch, rather than once per key or waiting out the wallet's own idle
    timeout — see `docs/THREAT-MODEL.md` I6 for the resulting exposure window.
    The reactive askpass-broker path (a single expired key re-added outside
    `load-keys`) is unaffected: it still opens and closes for that one lookup.

18. **Which keys' passphrases are stored in the wallet is configurable (goals 2,
    7; open decision 13). ✅ Done.** `config.toml` gains three keys, config-file
    only — no `SSHAKKU_*` twin, since the include/exclude lists don't fit a
    single environment variable cleanly: `wallet_store_mode` (`"all"` default,
    `"include"`, or `"exclude"`), `wallet_store_include`, and
    `wallet_store_exclude`. The mode is authoritative, so the two lists never
    conflict — `"include"` consults only `wallet_store_include` and
    `"exclude"` consults only `wallet_store_exclude`; the other list, if
    present, is simply not read. An unrecognised mode falls back to `"all"`
    and is logged (`config.Settings.StoresWallet(keyname) bool`). The policy
    is consulted at every wallet write: `keys.Config` gained a `WalletStore
    func(keyname string) bool` predicate (nil stores everything, preserving
    prior behaviour), checked by both `Loader.storePassphrase` (the load-keys
    prompt-then-store path) and `Broker.storePassphrase` (the askpass
    broker's miss-then-store fallback) before every `SecretBackend.Store`
    call. An excluded key is still used normally in the session — only the
    persistent store is skipped. Scoping this surfaced a real gap: the
    askpass broker never loaded `config.toml` at all, so none of the
    file-based settings — not just this one — reached it; `cmd/sshakku` now
    shares one `loadSettings` helper between `load-keys` and the askpass
    broker so both read the same config.

19. **Command to forget stored passphrases (goals 2, 15). ✅ Done.** `sshakku
    forget <keyname>...` deletes one or more stored passphrases; `sshakku forget
    --all` clears every sshakku-managed entry. Useful for testing, for revoking
    a passphrase after suspecting exposure, or for opting a key out of
    persistent storage after it was already saved (decision 18 only stops
    *future* stores). `SecretBackend` (`internal/keys/secret.go`) gained
    `Delete(service string) error` (a miss is success, not an error) and
    `List() ([]string, error)`. `SecretServiceBackend.List` enumerates the
    dedicated `sshakku` collection's items directly (`secretservice.Client`
    gained `Items`/`ItemAttributes`/`DeleteItem`) — since the collection is
    sshakku's own (decision 17), every item in it is sshakku-managed, so no
    separate prefix filter is needed. `SecretToolBackend.List` returns
    `ErrListUnsupported`: `secret-tool` has no generic enumeration verb, so
    `--all` only works with the native Secret Service backend; the fallback
    path still supports deleting named keys via `secret-tool clear`. Field note
    from manual testing during scoping: `secret-tool clear` failed silently
    (exit 1, no stderr) against a real stored entry, while a direct D-Bus
    `Item.Delete` call succeeded — this is why `SecretServiceBackend.Delete`
    goes through `DeleteItem`/D-Bus rather than shelling out, and why the
    fallback's `secret-tool clear` path is documented as not fully trustworthy.

20. **Three-tier container/VM test strategy (goal 16; open decision 9).**
    Prompted by a real incident: the kernel user keyring behaves differently
    without a PAM-established session (Add succeeds, a same-process Read is
    denied — see the `internal/keyring`/`internal/paths` fix that shipped
    alongside open decision 17), which a plain headless container missed
    until it was hit in CI. Bare container matrices are necessary but not
    sufficient; real desktop-session behaviour (wallet prompts, PAM-linked
    keyrings, a real login) needs a heavier tier. Three tiers, increasing in
    fidelity and cost:
    1. **Headless, multi-distro containers** (Gentoo/OpenRC, systemd
       distros, …), no desktop: the agent five-state lifecycle (Phase 1.5),
       Go unit/integration tests (already how `internal/secretservice`'s
       fake-D-Bus-peer suite and `internal/agent` run). Cheap and fast —
       every push, in the existing `test.yml` workflow or one like it.
    2. **Containers with a real desktop secret stack** (`ksecretd` +
       `kwalletd6` or GNOME Keyring, `kdialog`, a headless display via Xvfb
       or weston) exercising the actual wallet prompt/unlock flow end to
       end, not the fake server. Heavier and more brittle.
    3. **Vagrant, a full Gentoo/OpenRC/KDE box** doing a real login (SDDM,
       PAM, an actual user session) for the truest end-to-end check —
       reproducing, automatically and repeatably, the same kind of live
       install today only done by hand on the user's own PC — the slowest
       and most maintenance-heavy tier.
    **Decided:** cost is not a blocker — thoroughness wins. Tiers 2 and 3 are
    too heavy/flaky for every push, so they run as **manually-triggered CI
    workflows** (`workflow_dispatch`), not on every push like tier 1's
    `test.yml`; run on demand or before a release rather than gating every
    commit. Detailed sub-steps (which distros, which container images, the
    Vagrant box definition) are written when the phase that needs them is
    reached — this decision fixes the shape, not the implementation.

---

## Phases

High-level roadmap, ordered so each phase leaves the repo committable (rule 9).
Only the *intent* of each phase is fixed here; the detailed sub-steps are written
into the phase when we reach it, and the open decisions above are resolved at the
phase that needs them (not up front).

The ordering follows open decision 10: harden the primary target first (possibly
still in bash), then introduce the Go core, then widen to other backends and OSes.

### Phase 0 — Foundations & repo hygiene

Lint and CI baseline with no behaviour change: a `make lint` target (shellcheck +
a Markdown linter) aligned with CI, and an explicit least-privilege `permissions:`
block in every workflow. Write the threat model down in two lines, since it drives
the later design. Settle contributor licensing (a CLA preserving the holder's
freedom to relicense) while the project has no external contributors yet.
→ goals 16; open decisions 1, 11; rules 12, 14, 16.

Sub-phases (detailed steps written when we start each one):

- **0.1 — Repo hygiene. ✅ Done.** Renamed `makefile` → `Makefile`; added an
  `.editorconfig` (UTF-8, LF line endings, final newline, trim trailing whitespace,
  per-file-type indentation) and a `.gitattributes` (`* text=auto eol=lf`, explicit
  handling for shell scripts) to fix one formatting/line-ending standard across the
  repo. `.gitignore` already covers scratch/step files.
- **0.2 — Threat model. ✅ Done.** `docs/THREAT-MODEL.md` — a formal STRIDE model
  (assets, trust boundaries, threats tagged present/presumed/future, and the derived
  security invariants) to anchor the rewrite and the platform ports. The two-line
  summary stays in open decision 1 above.
- **0.3 — `make lint` target (rule 12). ✅ Done.** `make lint` runs `lint-sh`
  (`shellcheck` + `shfmt -d`), `lint-md` (`markdownlint-cli2`), `lint-make`
  (`checkmake`), `lint-yaml` (`actionlint`) and `lint-editorconfig`
  (`editorconfig-checker`). Renamed `ssh-init-macos.sh` → `ssh-init-macos.zsh`
  (zsh linting deferred to the macOS phase). `editorconfig-checker` **adopted**
  (whole tree; it honours `.gitignore`, so scratch files are skipped). Each tool
  reads its own config file (rule 13): `.markdownlint-cli2.yaml` (disables
  MD013/MD029/MD060 — see below), `checkmake.ini` (relaxes
  `minphony`/`maxbodylength`),
  `.editorconfig-checker.json` (excludes `LICENSE` verbatim and the deferred
  `*.zsh`). To satisfy the new linters with no behaviour change: shell scripts
  reformatted with `shfmt -w`, `.vscode/settings.json` reindented to 2 spaces, and
  `.editorconfig` marks Markdown indentation `unset` (content-driven). The lint
  tools are external dev/CI tools (separate processes, not bundled or
  distributed), so they carry no EUPL-1.2 obligations and don't obstruct
  relicensing (rule 16). `linting.yml`'s `ignore_names` was updated to the `.zsh`
  name (the shellcheck action scans `*.zsh`); the full CI rework (permissions
  block + running `make lint`) stays in 0.4.
  - Disabled Markdown rules: `MD013` (line-length — prose is hand-wrapped, tables
    and URLs legitimately exceed 80), `MD029` (ol-prefix — goals are numbered
    continuously across sub-sections and referenced by number), `MD060`
    (table-column-style — pipe spacing left to the author).
- **0.4 — CI alignment & least-privilege (open decision 11, rule 14). ✅ Done.**
  `linting.yml` now declares top-level `permissions: contents: read` and runs a
  single `lint` job that installs the six tools and invokes `make lint`,
  replacing the per-tool actions (`ludeeus/action-shellcheck`,
  `reviewdog/action-shfmt`) and dropping the `ignore_names` workaround. GitHub
  Actions are pinned by full commit SHA with a `# vX.Y.Z` comment (minor+patch),
  and a new `.github/dependabot.yml` enables the `github-actions` ecosystem to
  keep them current. The lint tools are pinned to explicit versions in the
  install step (shellcheck via release tarball; shfmt, checkmake, actionlint and
  editorconfig-checker via `go install`; markdownlint-cli2 via `npm`); auto-bump
  of those waits for the `go.mod`/`package.json` that arrive with the Go core
  (Phase 3). `dependabot.yml` is non-workflow YAML, already covered by the 0.3
  lint decision (editorconfig-checker for formatting; GitHub validates the schema
  server-side), so it needs no new per-file-type decision.
- **0.5 — Contributor licensing & CLA (rule 16). ✅ Done.** Added `CONTRIBUTING.md`,
  `CLA.md` and `DCO.txt` so contributors **keep the copyright** in their work while
  granting the copyright holder a **non-exclusive** licence to also distribute the
  project under other licences (e.g. proprietary/OEM) alongside the permanent public
  EUPL-1.2 release — no copyright assignment, no commit reverts. Mechanism: **DCO 1.1
  sign-off** (`Signed-off-by`) **+ acceptance-by-action** of the CLA (no signing
  bot); opening a PR with a sign-off certifies the DCO and accepts the CLA. The CLA
  adapts the **Harmony HA-CLA-I** (individual; HA-CLA-E noted for entities). The
  Harmony text is **CC BY 3.0 Unported**, adapted with attribution — a contract
  document, not runtime code or a dependency, so it imposes no terms on the software
  and does not obstruct relicensing (rule 16). `COPYRIGHT.md`, `AUTHORS.md` and
  `README.md` were updated to match. The new files are Markdown / plain text, already
  covered by `markdownlint-cli2` and `editorconfig-checker`, so no new per-file-type
  linter (rule 12). Governing law follows EUPL Art. 15 (law of the EU Member State
  where the holder is established, with Belgian law as the fallback), interpreted
  consistently with Union law and the EUPL — not a hard-coded national choice. A
  final IP-lawyer review is advisable before the first non-EUPL (OEM) sale. **Follow-up (rule 2):** propose a Rule 17 —
  "every contribution requires a DCO sign-off and CLA acceptance before merge" — to
  be formalised when the contribution flow is enforced.
- **0.6 — Contributor DX for the sign-off flow. ✅ Done.** Lower the friction a
  contributor meets with the DCO/CLA sign-off requirement. `CONTRIBUTING.md` gains
  a recovery recipe (`git rebase --signoff origin/master` + `git push
  --force-with-lease`) for when the DCO check fails on an earlier commit, plus an
  opt-in `prepare-commit-msg` hook under `.githooks/` (enabled with `git config
  core.hooksPath .githooks`) that adds the trailer automatically via `git
  interpret-trailers`, never duplicating one and skipping merge/squash messages. A
  `.github/pull_request_template.md` checklist nudges sign-off, `make lint`, scope
  and English before a PR is opened. The hook is an extensionless shell script, so
  it is wired into `make lint`'s `lint-sh` (`SH_SCRIPTS` now also globs
  `.githooks/*`) and given a tab-indent `.editorconfig` rule (`[.githooks/*]`) so
  shellcheck, shfmt and editorconfig-checker all cover it consistently (rule 12). A
  custom "comment on DCO failure" action was **rejected**: the DCO app already
  links its own remediation, and the action would widen the workflow token to
  `pull-requests: write` against the least-privilege default (rule 14).

Per-file-type lint decisions (rule 12):

| File type | Decision |
|---|---|
| Shell — bash (`*.sh`) | `shellcheck` + `shfmt` |
| Shell — macOS (`*.zsh`) | Renamed in 0.3; linting deferred to the macOS phase (also removes the shellcheck by-name exclusion) |
| Markdown (`*.md`) | `markdownlint-cli2` (config `.markdownlint-cli2.yaml`) |
| Makefile | `checkmake` (config `checkmake.ini`) |
| YAML / GitHub workflows | `actionlint`; non-workflow YAML/INI/JSON configs have no dedicated linter — `editorconfig-checker` enforces their charset/EOL/indent/final-newline |
| All committed files | `editorconfig-checker` **adopted in 0.3** (config `.editorconfig-checker.json` excludes `LICENSE` verbatim, the deferred `*.zsh`, and `*.go` — gofmt owns Go formatting and legitimately allows spaces inside string literals; `.gitignore` is honoured) |
| Shell — bats tests (`*.bats`) | Deferred to Phase 1.5 when test files enter the repo |
| Go (`*.go`) | `gofmt -l` + `go vet ./...` + `golangci-lint` (config `.golangci.yml`, standard set); compiled by `make build`. Wired into `make lint` as `lint-go` and installed in CI (pinned). License (rule 16): the Go toolchain, its standard library (BSD-3-Clause) and `golangci-lint` are EUPL-1.2 compatible and don't obstruct relicensing — build/dev tools follow the 0.3 precedent (no bundled obligations); the third-party module list (`golang.org/x/sys`, BSD-3-Clause) is recorded in `COPYRIGHT.md`. |
| TOML (`*.toml`) | `taplo lint` + `taplo format --check`, wired into `make lint` as `lint-toml` and installed in CI (pinned `@taplo/cli`); config `.taplo.toml` excludes only the deliberately malformed test fixture (the parser's error path, covered by Go tests). License (rule 16): the runtime parser `github.com/BurntSushi/toml` (MIT) is EUPL-1.2 compatible and doesn't obstruct relicensing, recorded in `COPYRIGHT.md`; `taplo` is a CI-only dev tool (0.3 precedent). |

### Phase 1 — Harden the primary target: shell plumbing (still bash)

Gentoo / OpenRC / KDE. Scope narrowed by the bash/Go split (open decision 10):
Phase 1 ships only the **permanent shell plumbing** in cleaned-up bash — a stable,
committable baseline on the primary box — while the branchy, stateful logic moves
to the Go core in Phase 2 (written once, not twice). Fixed agent socket and
never-kill-a-healthy-agent (already shipped), clean exit with no keys, best-effort
recovery, silent-on-success output discipline, and the standard path/install
layout that gets our files out of `~/.ssh`. The login entrypoint is shaped so the
Go core slots in behind it. → goals 3, 5, 6, 10, 17–19; open decisions 3, 4, 12.

Sub-phases (detailed steps written when we start each one):

- **1.1 — XDG path layout, out of `~/.ssh`.** Move our files to standard paths:
  socket + lock to `$XDG_RUNTIME_DIR/<name>/` (0700, with a fallback for when it
  is unset — possible under OpenRC/elogind), log/state to `$XDG_STATE_HOME/<name>/`
  (0600 files), config under `$XDG_CONFIG_HOME/<name>/` or `/etc/<name>/`. The keys
  stay in `~/.ssh` (OpenSSH's domain; we only read them). Align the askpass log to
  the same state dir. → goal 19; open decision 12; threats I7, I10, D2; invariant 3.
- **1.2 — Two install modes + bootstrap hook.** System-wide (`sudo`,
  `/etc/profile.d`, `$BINDIR`) vs per-user (no root, everything under `$HOME`); the
  same logic, only the paths and the bootstrap hook differ. Resolves the per-user
  hook left open in open decision 12 (`~/.bashrc` vs
  `~/.config/plasma-workspace/env/`). → goals 17, 18; open decision 12; threat E3.
- **1.3 — Silent-on-success & shell safety, with the Go seam.** Zero stdout/stderr
  on the success path; `set -u`-clean; degrade gracefully when `keyctl` / `flock`
  are absent. The seam is now real: the entrypoint evals the Go core
  (`sshakku shell-init`, Phase 2 slice 1), so the remaining 1.3 work is the
  silence / `set -u` hardening layered on top. → goal 3; open decision 3; threat
  I4; invariant 2.
- **1.4 — Agent lifecycle & recovery.** The five-state self-healing policy (open
  decision 15): never kill a healthy agent (`ssh-add -l` exit 0 and 1 both healthy),
  clean exit with no keys, reap dead sockets/agents (ours and dead foreign ones),
  adopt-by-symlink a healthy foreign agent with an anomaly report, and a last-resort
  dangling-socket symlink for already-open GUI apps. **Now Go slice 2** (see the
  Phase 2 note): this lifecycle logic moves into the Go core rather than staying in
  bash. → goals 5, 6, 8; threats D1, D5.
- **1.5 — Shell test harness (rule 12).** `bats` unit tests + container integration
  tests covering the plumbing regression scenarios below — tier 1 of open
  decision 20 (headless multi-distro containers). `bats` is a new file
  type — evaluate a linter and record the decision (including a deliberate "no
  linter") here. → goal 16.

  Plumbing regression checklist (post-change):

  1. Fresh login → two terminals → both see the key in `ssh-add -l`, with **no**
     passphrase prompt the second time.
  2. `SSH_AUTH_SOCK` is the fixed socket path in every terminal and in a GUI app
     (e.g. inspect `/proc/<plasmashell-pid>/environ`).
  3. Kill the agent (`ssh-agent -k` / `pkill ssh-agent`) → a new terminal restarts
     it at the **same** socket path and reloads the key.
  4. First-ever run with an empty vault → prompts once, then `secret-tool lookup`
     returns the passphrase on later logins (no prompt).
  5. Reachable-but-empty agent (`ssh-add -l` exit 1) must be treated as **healthy**
     and never killed (the D1 regression).

### Phase 2 — Go logic core

Move the branchy, stateful logic out of bash into a small Go core behind the thin
shell entrypoint, minimizing duplication: bounded retries with a persistent
give-up sentinel and an opt-out, key-expiry semantics (`ssh-add -t`, silent re-add
from the vault), GUI / secret-prompt detection that works under Wayland and
headless, and secret-handling hardening (no passphrase in env or argv, absolute
`SSH_ASKPASS` + `SSH_ASKPASS_REQUIRE=force`, clean child env). Define the
`SecretBackend` interface (it also makes the tests mockable) and stand up unit
tests plus container integration tests on Linux. Go entered the repo early (slice
1 below), where the Go lint decision (`gofmt` / `go vet` / `golangci-lint`,
`make build`) and `go.mod` were already made (rule 12). → goals 1, 2, 4, 9, 14,
16; open decisions 2, 5, 6, 7, 9.

**Brought forward — Go core grown incrementally (strangler).** Instead of one
wholesale rewrite, the Go core is added slice by slice behind the entrypoint seam
while the bash shrinks toward a thin `eval "$(sshakku shell-init)"`. Each slice
is committable and the bash keeps working until each piece moves.

- **Slice 1 — path / token / dir / log core. ✅ Done.** `cmd/sshakku` +
  `internal/paths` + `internal/sessionlog`: path resolution (config dir; runtime
  dir XDG_RUNTIME_DIR → /run/user/$UID owned → ~/.cache), the per-login `@u`
  keyring token via the `keyctl(2)` syscall (`golang.org/x/sys`, no `keyctl`
  binary), 0700/0600 dir+log creation with a symlinked-leaf guard, legacy
  `~/.ssh/agent` cleanup, and a bounded session log. `shell-init` prints
  `agent_sock`/`agent_lock`/`log_file`; the entrypoint evals them. The Go lint and
  `golang.org/x/sys` (BSD-3-Clause) licence decisions are recorded (rules 12, 16).
  Follow-up: the session log had shipped under `ConfigDir` instead of a
  dedicated state dir, missing the `$XDG_STATE_HOME` split called for in 1.1.
  Closed by adding a `StateDir` field, defaulting to `~/.local/state`, and moving
  `LogFile` under it. `internal/config` no longer resolves its own config dir; it
  now reuses the `ConfigDir` already computed by `paths.Resolve`.
- **Slice 2 — agent lifecycle. ✅ Done.** (the Phase 1.4 work, in Go): reachability
  plus the five-state self-healing policy of open decision 15 — start on the fixed
  socket when clean, attach when ours is healthy, reap dead sockets/agents (ours and
  dead foreign ones), and adopt-by-symlink a healthy foreign agent while reporting
  the anomaly. Never kill a healthy agent. `internal/agent` (probe/inspect/manage/
  ensure, flock-serialised); `shell-init` is the sole owner of the lifecycle.
- **Slice 3 — key loading + `askpass`. ✅ Done.** `internal/keys` +
  `internal/keyring`: enumerate `~/.ssh/id_*`, skip fingerprints already in the
  agent (`ssh-keygen`/`ssh-add -l`), and add the rest via the secret store
  (`secret-tool`) or a prompt (`kdialog`), handing each passphrase to ssh-add out of
  band through the `@u` keyring (payload never in argv) + an SSH_ASKPASS helper
  marked by `SSHAKKU_ASKPASS`. `sshakku load-keys` is driven from the login hook in
  interactive shells; the bash askpass script is retired. GUI detection covers
  Wayland and X11.
- **Slice 4 — retries / give-up + key-expiry. ✅ Done.** Resolves open decisions
  5 and 6. Keys expire in the agent via `ssh-add -t` (default 8h, configurable
  with `SSHAKKU_KEY_LIFETIME`; `0` disables); a new terminal silently re-adds an
  expired key from the wallet, and in a still-open shell the wallet-aware
  SSH_ASKPASS broker refills it without a terminal prompt, falling back to
  `/dev/tty` only on a wallet miss (and storing what is typed there). A wrong
  passphrase retries up to `SSHAKKU_MAX_ATTEMPTS`, a stale stored passphrase being
  re-prompted and replaced; after exhaustion the key is given up — notified on the
  terminal unless `SSHAKKU_QUIET` — and skipped in new shells for
  `SSHAKKU_GIVEUP_TTL` (per-login, tmpfs-backed; `SSHAKKU_NO_GIVEUP` opts out).
  `internal/giveup` + `internal/keys`; the env knobs are documented in
  `docs/CONFIGURATION.md` and the bash is now just the thin hook.

### Phase 3 — Diagnostic tool

The currently-missing diagnostic that reports who started the agent, why it isn't
working, and which processes are involved — runnable under `sudo` for the full
picture. It attributes a foreign agent (open decision 15, case D) to the process or
tool that started it, and guides — or applies — the fix back to the clean state in
which only sshakku runs the agent. Now lands after the Go core, so it is built in Go
reusing the core's inspection primitives rather than as throwaway bash. → goal 8;
threat E1.

**✅ Done.** `sshakku doctor` (`internal/diagnose`, reusing the `agent` package's
inspection primitives): a read-only report that names the agent-lifecycle state
(the five states of open decision 15, A–E), classifies every `ssh-agent` process
as ours/legacy/foreign, probes reachability, compares `SSH_AUTH_SOCK` against the
fixed socket, tails the session log, and derives plain findings with a
recommendation. It attributes each agent to its launcher by walking the `/proc`
PPid ancestry (systemd, KDE Plasma, GNOME/GDM/SDDM/LightDM, sshd, login shells);
because `ssh-agent` daemonizes and reparents to `init`, ancestry frequently
dead-ends at pid 1, and the report says so rather than crediting init.
`doctor --fix` applies the same self-heal the login path runs (`EnsureAgent`:
reap dead, start on the fixed socket, or adopt a healthy foreign agent — never
killing a healthy one) and re-reports; it warns that it cannot rewrite the calling
shell's `SSH_AUTH_SOCK`. Documented in `docs/DIAGNOSTICS.md` (linked from the
README). No new file type — the docs are Markdown, already covered by
`markdownlint-cli2` + `editorconfig-checker` (rule 12).

**✅ Done — cross-user inspection under `sudo`.** `sshakku doctor --user
<name|uid>` diagnoses another user's session instead of the invoking one
(auto-detected from `SUDO_UID` when invoked as root with no `--user`), closing
the gap noted below. It confirms the target's own fixed socket by reading
their per-login token from their own kernel keyring — reached by re-executing
the binary as a child process under their credentials (a kernel-mediated
privilege drop via `exec.Cmd.SysProcAttr.Credential`, never in-process
`setuid`/`seteuid`), so the confirmation is real, not a filesystem-shape
guess. Requires root; `--fix` is refused cross-user (threat model E1:
elevation is for read-only inspection, never for writing as root) — fixing
another user's session is `sudo -u <user> -H sshakku doctor --fix` instead,
which needs none of this machinery. Root also bypasses unix-socket
permissions, which was inflating "reachable" for an agent the target could
never actually reach themselves; `agent.UIDGatedProber` closes that, and
`classifyState`/`findings` no longer let a different real user's agent drive
this account's own state or foreign-agent wording — which is what actually
fixes a plain `sudo sshakku doctor` (no `--user` at all) misreporting the
invoking-as-root case as state D. Documented in `docs/DIAGNOSTICS.md`.

*Deferred refinements (not blocking):* deeper foreign-agent attribution via
socket-path heuristics (gnome-keyring `keyring/ssh`, gpg-agent, systemd
`ssh-agent.socket`) and environment probing to recover a launcher lost to the
daemonize/reparent — including the specific case (found while validating the
cross-user work above) of an agent whose socket has sshakku's own naming shape
but an unrecognised per-login token, most likely one of our own orphaned by a
keyring that didn't survive across logins/reboots rather than a truly external
tool; tracked separately (see `orphaned-agent-token-steps.md` during
development).

### Phase 4 — Configurability & pluggable secret backends

Make the secret store pluggable (secret-service first, then 1Password) and the
tool highly parametrizable via a config file under `$XDG_CONFIG_HOME/sshakku/`
(default `~/.config/sshakku/`), into which the current `SSHAKKU_*` environment
knobs migrate. → goals 11, 15; open decisions 7, 13, 17, 18, 19.

### Phase 5 — Widen the OS targets

macOS as a wide port, never trust Apple; then Windows last as the most divergent target (service + named pipe, no socket, use win32 safe API). → goals 12, 13; open decision 8.

### Phase 6 — Full test matrix

Extend CI to macOS and Windows runners and complete the cross-platform test
matrix. Also where tiers 2 and 3 of open decision 20 (real-desktop-secret-stack
containers; the Vagrant Gentoo/OpenRC/KDE box) get their manually-triggered
CI workflows. → goal 16; open decisions 9, 20.

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
types the repo has grown by then. → goal 16; open decisions 9, 11; rules 12, 14.
