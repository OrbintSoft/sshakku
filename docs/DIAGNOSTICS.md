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

### Askpass wiring

When a graphical prompter is available (the same check `sshakku askpass-env`
uses) but this shell's `SSH_ASKPASS`/`SSH_ASKPASS_REQUIRE` aren't set, the
report flags it: ssh will fall back to prompting on the raw terminal instead of
routing passphrases through the wallet-aware broker. This typically means the
shell-init script never ran for this particular shell — e.g. it wasn't started
as a login shell, so `/etc/profile.d` was never sourced — re-source your shell
profile or open a new terminal. This check only applies to your own
session; `--user` reports never inspect it, since it describes the invoking
shell's environment, not the target's.

## `sshakku doctor --fix`

`sshakku doctor --fix` first prints the diagnosis, then applies the same
self-heal the login path runs: it reaps dead agents and their stale sockets,
starts a fresh agent on the fixed socket, or adopts a healthy agent started by
something else. A healthy agent is never killed. It then re-reports the result.

A running program cannot change the environment of the shell that started it, so
`--fix` cannot rewrite the current shell's `SSH_AUTH_SOCK`. When the shell still
points somewhere other than the healed socket, the command prints an
`export SSH_AUTH_SOCK=…` line to run — or you can simply open a new shell.

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
