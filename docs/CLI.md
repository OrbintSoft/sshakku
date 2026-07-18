# Command reference

`sshakku` is a single binary with subcommands. Most of them are wired in
automatically by the login hook (see [README.md](../README.md#how-it-works))
and are not meant to be typed by hand — day to day, the two you'll actually
run yourself are `doctor` and `forget`. This page documents every subcommand
and flag for reference; `sshakku help` prints a short version of the same
list.

Every subcommand follows the same exit-code convention: `0` on success, `1`
on a runtime failure, `2` on a usage error (unknown command, missing or
malformed argument).

| Command | Run by hand? | Effect |
| --- | --- | --- |
| [`shell-init`](#sshakku-shell-init) | No | Keeps the agent healthy, prints the shell assignments the login hook evals. |
| [`ensure-agent`](#sshakku-ensure-agent) | Rarely | Same agent lifecycle step alone, without the other assignments. |
| [`load-keys`](#sshakku-load-keys) | Rarely | Adds every key under `~/.ssh` to the agent. |
| [`askpass-env`](#sshakku-askpass-env) | No | Prints the exports that route ssh's passphrase prompts through the wallet-aware broker. |
| [`doctor`](#sshakku-doctor) | Yes | Reports (and, with `--fix`, repairs) the ssh-agent situation. |
| [`forget`](#sshakku-forget) | Yes | Deletes stored passphrases. |
| [`help`](#sshakku-help--h---help) | Yes | Prints the command list. |

## `sshakku shell-init`

Resolves the per-user runtime layout, drives the fixed socket to a healthy
`ssh-agent` (starting one, reaping a dead one, or adopting a healthy foreign
one), and prints the result as shell assignments to `eval`:

```sh
agent_sock='…'
agent_lock='…'
log_file='…'
```

This is the command the login hook evals to pin the shell to the fixed
socket; it is not meant to be run interactively for its own output; use
`sshakku doctor` to inspect the same state in a human-readable form instead.

## `sshakku ensure-agent`

The agent lifecycle step alone, without the log file or lock path — prints
just:

```sh
agent_sock='…'
```

`shell-init` calls the same logic internally and adds the other two
assignments; `ensure-agent` exists as a standalone entry point for exercising
the lifecycle (e.g. from a script that only needs the socket path) without
the rest of `shell-init`'s output.

## `sshakku load-keys`

Adds every key file under `~/.ssh` to the agent, skipping any key already
loaded. Each passphrase comes from the configured secret backend; on a miss,
it prompts (graphically when available, otherwise on the terminal) and
stores the result for next time, subject to `wallet_store_mode` — see
[Choosing which keys' passphrases are stored](CONFIGURATION.md#choosing-which-keys-passphrases-are-stored)
and [Choosing which keys are auto-loaded](CONFIGURATION.md#choosing-which-keys-are-auto-loaded).

The login hook runs this only in interactive shells, since it may prompt and
write to the terminal — never in a non-interactive one (a script, `scp`,
`rsync`). Run it by hand to force a re-check of `~/.ssh` without opening a
new shell.

## `sshakku askpass-env`

Prints the `export` lines that route this shell's `ssh` passphrase prompts
through sshakku's wallet-aware broker:

```sh
export SSH_ASKPASS='…'
export SSH_ASKPASS_REQUIRE=prefer
export SSHAKKU_ASKPASS=1
```

Prints nothing (and exits `0`) when no graphical prompter is available, since
a headless session keeps ssh's own terminal prompting instead. The login hook
evals this in every login shell, interactive or not — it is cheap even as a
no-op, so gating it on interactivity isn't needed.

Once these are exported, `ssh` itself execs the same `sshakku` binary as its
`SSH_ASKPASS` helper whenever it needs a passphrase or confirmation — that
invocation is ssh's doing, not a subcommand you run yourself, and it answers
only the one prompt ssh passes it as an argument.

## `sshakku doctor`

```sh
sshakku doctor [--fix] [--user <name|uid>] [--test-backend [name]]
```

Reports the ssh-agent situation: lifecycle state, sockets, processes, keys
and their remaining time, environment hardening checks, findings, and a
recommendation. Plain `doctor` only inspects and changes nothing.

- `--fix` — applies the same self-heal the login path runs, then re-reports.
- `--user <name|uid>` — reports on a different user's session (root only,
  read-only).
- `--test-backend [name]` — actively exercises a secret backend end to end
  (store, look up, delete a throwaway probe entry).

Full details on the report, each flag, and cross-user diagnosis are in
[docs/DIAGNOSTICS.md](DIAGNOSTICS.md).

## `sshakku forget`

```sh
sshakku forget <keyname>...
sshakku forget --all
```

Deletes the stored passphrase for one or more keys (matched by file name,
e.g. `id_rsa`), or, with `--all`, every entry sshakku manages. `--all` cannot
be combined with key names. See
[Forgetting stored passphrases](CONFIGURATION.md#forgetting-stored-passphrases)
for when to use it and the native-backend requirement `--all` has.

Prints `forgot <service>` on stdout for each key actually deleted; exits `1`
if any deletion fails (after attempting the rest), `2` on a usage error.

## `sshakku help`, `-h`, `--help`

Prints the command list shown at the top of this page and exits `0`. Running
`sshakku` with no arguments or an unrecognised command also prints it, on
stderr, exiting `2`.
