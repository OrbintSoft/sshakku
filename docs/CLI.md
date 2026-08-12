# Command reference

`sshakku` is a single binary with subcommands. Most of them are wired in
automatically by the login hook (see [README.md](../README.md#how-it-works))
and are not meant to be typed by hand — day to day, the ones you'll actually
run yourself are `config`, `doctor` and `forget`. This page documents every subcommand
and flag for reference; `sshakku help` prints a short version of the same
list.

Every subcommand follows the same exit-code convention: `0` on success, `1`
on a runtime failure, `2` on a usage error (unknown command, missing or
malformed argument).

Three of them — `shell-init`, `ensure-agent` and `askpass-env` — print lines
for a shell to run, and take `--shell <posix|powershell>` (also
`--shell=<name>`) to say which shell is asking. Without it they print the
Bourne form, which is what `eval` in `sh`, `bash` and `zsh` reads — including
from Git Bash on Windows. `--shell=powershell` prints the same values as
PowerShell assignments, for `Invoke-Expression` to run. Nothing is inferred
from the operating system: the binary cannot see which shell started it, and a
name it does not have is a usage error rather than a guess.

| Command | Run by hand? | Effect |
| --- | --- | --- |
| [`shell-init`](#sshakku-shell-init) | No | Keeps the agent healthy, prints the shell assignments the login hook evals. |
| [`ensure-agent`](#sshakku-ensure-agent) | Rarely | Same agent lifecycle step alone, without the other assignments. |
| [`load-keys`](#sshakku-load-keys) | Rarely | Adds every key in your key directory to the agent. |
| [`askpass-env`](#sshakku-askpass-env) | No | Prints the exports that route ssh's passphrase prompts through the wallet-aware broker. |
| [`config`](#sshakku-config) | Yes | Prints the configuration in force and where each value came from; with `--edit`, opens your `config.toml`. |
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

or, with `--shell=powershell`:

```powershell
$agent_sock = '…'
$agent_lock = '…'
$log_file = '…'
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

Adds every key file in your key directory — `~/.ssh` unless you say otherwise,
see [Choosing which files are your keys](CONFIGURATION.md#choosing-which-files-are-your-keys)
— to the agent, skipping any key already loaded. Each passphrase comes from the configured secret backend; on a miss,
it prompts (graphically when available, otherwise on the terminal) and
stores the result for next time, subject to `wallet_store_mode` — see
[Choosing which keys' passphrases are stored](CONFIGURATION.md#choosing-which-keys-passphrases-are-stored)
and [Choosing which keys are auto-loaded](CONFIGURATION.md#choosing-which-keys-are-auto-loaded).

The login hook runs this only in interactive shells, since it may prompt and
write to the terminal — never in a non-interactive one (a script, `scp`,
`rsync`). Run it by hand to force a re-check of that directory without opening
a new shell.

## `sshakku askpass-env`

Prints the `export` lines that route this shell's `ssh` passphrase prompts
through sshakku's wallet-aware broker:

```sh
export SSH_ASKPASS='…/sshakku-askpass'
export SSH_ASKPASS_REQUIRE='force'
```

or, with `--shell=powershell`:

```powershell
$env:SSH_ASKPASS = '…\sshakku-askpass'
$env:SSH_ASKPASS_REQUIRE = 'force'
```

`sshakku-askpass` is installed alongside `sshakku` and is the program `ssh`
runs when it needs a passphrase; you never run it yourself.

Every session gets them, with or without a desktop: reading the wallet needs
no graphical prompter, and when the wallet has no entry the broker asks on the
terminal just as `ssh` would have. The login hook evals this in every login
shell, interactive or not, since it only ever prints these lines.

Once these are exported, `ssh` itself execs the same `sshakku` binary as its
`SSH_ASKPASS` helper whenever it needs a passphrase or confirmation — that
invocation is ssh's doing, not a subcommand you run yourself, and it answers
only the one prompt ssh passes it as an argument.

## `sshakku config`

```sh
sshakku config
sshakku config --edit
```

Plain `config` prints the configuration in force: every setting, its current
value, and where that value came from — the built-in default, an environment
variable, `config.toml`, or the file under `config.d/` that overruled the ones
before it. It also names the files it read, in the order it read them, and
repeats any value that was refused, which otherwise reaches only the session
log. It reads and changes nothing.

Values are printed as they stand. No setting holds a secret — a passphrase is
never one of them — but some of them name you or your machine (an account
email, a database path under your home), so read the output before pasting it
into a bug report.

- `--edit` — opens `config.toml` in `$EDITOR` (then `$VISUAL`, then `vi`),
  creating it from a commented template if you have none. `$EDITOR` may carry
  arguments (`code -w`), and they are passed on. That file only: `config.d/` is
  never opened for you. When the editor exits, SSHakku re-reads the file and
  tells you what you would otherwise meet at your next login — that it can no
  longer be parsed, that a value in it was refused, or that a key set in it is
  decided by a drop-in or by an exported variable instead.

Exits `0` when the configuration was printed or the file was edited, `1` when
the editor could not be run, the file could not be written, or what was saved
can no longer be parsed, `2` on a usage error.

## `sshakku doctor`

```sh
sshakku doctor [--fix] [--user <name|uid>] [--test-backend [name]]
```

Reports the ssh-agent situation: lifecycle state, sockets, processes, keys
and their remaining time, the wallet it would use and whether what that
wallet needs is present, environment hardening checks, findings, and a
recommendation. Plain `doctor` only inspects and changes nothing — the wallet
section looks for the pieces (a command-line tool, a database file, a running
KeePassXC, a session bus) without reading or storing a single passphrase.

- `--fix` — applies the same self-heal the login path runs, then re-reports.
- `--user <name|uid>` — reports on a different user's session (root only,
  read-only).
- `--test-backend [name]` — actively exercises a secret backend end to end
  (store, look up, delete a throwaway probe entry). This is the deliberate
  version of the wallet section above: that one says whether the pieces are
  there, this one says whether they work.

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
