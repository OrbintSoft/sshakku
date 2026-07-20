# Diagnostics

SSHakku includes a diagnostic command that reports the state of your SSH agent
and, on request, repairs it.

## `sshakku doctor`

Run `sshakku doctor` to print a read-only report of the current ssh-agent
situation. It changes nothing. The report covers:

- **State** — the agent lifecycle state, one of:
  - **A — clean**: no agent is running.
  - **B — ours healthy**: sshakku's agent is answering on the fixed socket.
  - **C — ours zombie**: only dead remnants of our agent remain.
  - **D — foreign agent serving**: a single agent sshakku did not start is
    answering.
  - **E — disaster**: several agents are answering at once.
- **Sockets** — the fixed socket sshakku uses, and this shell's `SSH_AUTH_SOCK`
  with whether it is reachable.
- **Processes** — every `ssh-agent` process found, each labelled *ours*,
  *legacy*, or *foreign*, with its reachability, owning user, socket, and the
  process chain that launched it.
- **Keys** — every key file under `~/.ssh`, whether it is currently loaded in
  the agent, and, for a loaded key, how much longer it has there.
- **Environment** — best-effort checks on conditions outside sshakku's own
  control that materially affect its threat model: disk encryption, `/tmp`,
  and TPM presence.
- **Findings** — plain-language observations (a stale environment, dead agents
  lingering, a foreign agent answering, and so on).
- **Recommendation** — what to do about the current state.
- **Recent log** — the tail of the session log.

### Attribution

For each agent the report shows the process chain up toward `init` and names the
launcher when it recognises one (systemd, the KDE Plasma or GNOME session, a
display manager, an SSH login, a login shell). Because `ssh-agent` daemonizes and
is then reparented to `init`, the original launcher is often no longer in the
process tree; when that happens, the report falls back to naming the systemd
unit (service or transient scope) the agent's own cgroup still belongs to, if
one is found — cgroup membership survives the reparent even though ancestry
does not — and otherwise says the launcher is unknown rather than guessing.

A foreign agent whose socket has sshakku's own naming shape
(`.../sshakku/<hex>/agent.sock`) but a token that doesn't match this session's
own is called out as likely a previous instance of sshakku's own agent —
orphaned by an old build or manual testing, say — rather than a truly
external tool, since another program reinventing that exact layout by
coincidence is far less likely.

When `SSH_AUTH_SOCK` is reachable but isn't the fixed socket, the report also
recognises the socket's shape for a few well-known ssh-agent-compatible
services — gpg-agent with ssh support enabled, gnome-keyring-daemon's ssh
emulation, a systemd-activated `ssh-agent.socket` unit — and names the service
instead of only saying "not our fixed socket". These services never run under
the `ssh-agent` binary name, so they cannot appear as an agent in the process
list above; the socket path is the only signal available for them.

### Login shell not picked up

Both `SSH_AUTH_SOCK` being unset and, separately, a graphical prompter being
available but this shell's `SSH_ASKPASS`/`SSH_ASKPASS_REQUIRE` not being set
(the same check `sshakku askpass-env` uses) most commonly trace back to the
same cause: the shell-init script never ran for this particular shell
because it wasn't started as a login shell, so the profile file that sources
it was never read — `/etc/profile.d` (or, for a per-user install,
`~/.bash_profile`) on Linux, `/etc/zprofile` (or `~/.zprofile`) on macOS.
Opening a plain new terminal tab does *not* reliably fix this — many
terminal emulators, multiplexers, and IDE-integrated terminals start a
non-login shell by default, which reads `~/.bashrc`/`~/.zshrc` instead, not
the login profile. Either re-source the profile directly in the affected
shell, or start a genuine login shell (e.g. `bash -l` on Linux, `zsh -l` on
macOS). The askpass half of this check only applies to your own session;
`--user` reports never inspect it, since it describes the invoking shell's
environment, not the target's.

### Keys and their remaining time

The ssh-agent protocol has no query for a key's remaining lifetime, so sshakku
tracks it itself: whenever `load-keys` adds a key, it records when and for how
long (the `-t` lifetime `ssh-add` was given) in a small per-key file under the
per-login runtime directory — the same tmpfs-backed location as the give-up
sentinels, wiped on logout or reboot, holding no secret. `doctor` reads those
records back to show, for each key under `~/.ssh`:

```text
~/.ssh keys (2):
  id_ed25519_github           loaded, expires in 7h12m30s
  id_rsa_old                  not loaded
```

A loaded key can also show:

- `loaded, no expiry` — added with `key_lifetime`/`SSHAKKU_KEY_LIFETIME` set to
  a non-positive value, so it never expires from the agent on its own.
- `loaded, TTL unknown (not added by sshakku, or added before a reboot)` — the
  key is in the agent but sshakku has no record for it: it was added by
  something other than `load-keys` (a manual `ssh-add`, forwarded from
  elsewhere), or the record was lost when the runtime directory was wiped
  while the agent itself survived.
- `loaded, TTL unknown (sshakku's record expired <duration> ago, but the agent
  still has it — likely refreshed outside sshakku)` — sshakku *does* have a
  record for this key, and by that record it should have expired, but the
  agent only ever drops a key exactly at its `ssh-add -t` deadline — so
  something re-added or extended it after sshakku's own load, without going
  through `load-keys` (a manual `ssh-add`, an IDE's own SSH integration).
  `doctor` no longer trusts the stale record once this happens: a new shell
  will **not** refill the key either, since the loader dedups on an
  already-loaded fingerprint and skips it.

### Environment hardening checks

sshakku's own threat model assumes the wallet database and any temporary
files are reasonably protected by the surrounding system; none of that is
sshakku's to configure, but a broken assumption there weakens everything it
does. `doctor` reports three best-effort, read-only checks:

```text
environment:
  disk encryption: no  |  /tmp: not tmpfs  |  secure hardware: present (TPM 2.0)
```

- **Disk encryption** — on Linux, whether the block device backing your home
  directory is LUKS-encrypted, detected via `/proc/mounts` and
  `/sys/class/block/*/dm/uuid`, including one level of LUKS-under-LVM; on
  macOS, FileVault's status via `fdesetup status`. An unencrypted disk means
  anyone with physical access to the drive can read the wallet database
  directly, bypassing sshakku entirely.
- **`/tmp`** — whether `/tmp` is its own tmpfs mount (memory-backed) rather
  than living on the root filesystem, and, when it is, whether it looks
  large enough (512 MiB) to be reliable under load. Always "not tmpfs" on
  macOS, which has no tmpfs-backed `/tmp`.
- **Secure hardware** — whether the machine has a hardware key store an
  OS-level encryption scheme could bind to: on Linux, a TPM device driver
  bound under `/sys/class/tpm/tpm<N>` (and its rough version, 1.2 or 2.0);
  on macOS, a Secure Enclave Processor. Every Apple Silicon Mac has one, so
  this is CPU architecture (the `hw.optional.arm64` sysctl), not a probe;
  on Intel Macs, where a Secure Enclave was optional (tied to a T1/T2
  Security Chip), `system_profiler SPiBridgeDataType` names it when
  present. Present hardware can back a stronger disk-encryption setup than
  a plain passphrase alone, where the platform supports it.

Every check that cannot be determined (a network filesystem, an unreadable
`/proc` or `/sys`, an `fdesetup`/`system_profiler` invocation that failed to run) is
reported as "undetermined" rather than guessed. A
concerning result also appears under **findings**, always phrased as
advisory: `doctor` reports these, it never configures anything or refuses to
run because of them. Fixing what these checks flag is outside sshakku's
scope — see [Hardening](HARDENING.md) for what to do about each one.

## `sshakku doctor --fix`

`sshakku doctor --fix` first prints the diagnosis, then applies the same
self-heal the login path runs: it reaps dead agents and their stale sockets,
starts a fresh agent on the fixed socket, or adopts a healthy agent started by
something else. A healthy agent is never killed. It then re-reports the result.

A running program cannot change the environment of the shell that started it, so
`--fix` cannot rewrite the current shell's `SSH_AUTH_SOCK`. When the shell still
points somewhere other than the healed socket, the command prints an
`export SSH_AUTH_SOCK=…` line to run — or you can simply open a new shell.

## `sshakku doctor --test-backend [name]`

A misconfigured secret backend otherwise only surfaces the first time `ssh`
actually needs a passphrase. `sshakku doctor --test-backend` proves the
configured backend works end to end instead: it stores a throwaway probe
entry, looks it up back, and deletes it, reporting a clear pass or fail for
each step:

```text
── testing secret backend ──
backend: secret-service
  unlock: ok
  store: ok
  lookup: ok
  delete: ok
backend test: PASS
```

With no name, it tests whichever backend `config.toml`'s `secret_backend`
resolves to (see [`CONFIGURATION.md`](CONFIGURATION.md)). Naming one of
`secret-service`, `keychain`, `1password`, or `bitwarden` explicitly tests
that backend instead, using the same account fields (`onepassword_vault`,
`bitwarden_email`, `bitwarden_server`) already in `config.toml` regardless of
which backend is actually configured as the default — useful for checking a
backend you're about to switch to before you switch. The probe entry is
always deleted, even after an earlier step failed, so no test data is left
behind in the wallet. Refused with `--user`: it acts on the secret store, so,
like `--fix`, it must run as the account it acts for.

## Scope

`doctor` inspects the invoking user's session, and `--fix` acts as that user: it
never escalates privileges, so it reaps only your own dead agents. Run it as the
user whose agent you are diagnosing.

### Diagnosing another user's session

`sshakku doctor --user <name|uid>` reports on a different user's session
instead — useful when you've `sudo`ed in to help diagnose someone else's
account. Invoked as root with no `--user`, the target is auto-detected from
`SUDO_UID` (the real user `sudo` ran as), so a plain `sudo sshakku doctor`
diagnoses *that* user rather than root's own, empty session.

This requires root (only root can assume another uid's identity), and it
never accepts `--fix`: elevation is for read-only inspection only, never for
writing as root into another user's files or sockets. To actually fix another
user's session, run as that user instead:

```sh
sudo -u <user> -H sshakku doctor --fix
```

Confirming the target's fixed socket needs their per-login token, which lives
in their own kernel keyring — invisible to root by simple file access, unlike
regular files. `doctor` reads it by re-executing itself as a short-lived child
process running under the target's own credentials (a kernel-mediated
privilege drop, not an in-process one), then discards that identity
immediately; nothing else runs under it.

`--user` reports omit the keys section: reading another user's `~/.ssh` and
key-state records under a privilege drop is not implemented.
