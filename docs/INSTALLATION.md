# Installation

What installing SSHakku actually does, where each piece goes, and how the shell
it wires itself into is chosen. [README.md](../README.md) has the commands to
type; this page is the model behind them, and the place to look when your
machine is not the ordinary case — several shells, several PowerShell editions,
a home directory somewhere unusual, or a policy that decides what your shell may
run.

## The three things an install does

1. **Puts the binary somewhere**, with `sshakku-askpass` beside it — a second
   name for the same binary, which `ssh` runs when it needs a passphrase and
   which you never run yourself. This is `make`'s work on every platform. The
   second name is a link to the same file wherever the system makes links
   cheaply, and it carries whatever that system puts at the end of a program's
   name, so on Windows it is `sshakku-askpass.exe` beside `sshakku.exe`.
2. **Wires a login hook into one shell startup file**, so every new session has
   a working agent without your doing anything.
3. **Makes `sshakku` runnable by name**, which on some systems the first step
   does not achieve on its own.

The first is where the program lives, so it decides the other two: the hook a
shell runs names that copy, and it is that directory — never the tree you built
in — that is made findable.

Uninstalling undoes exactly those three, and nothing else: the surrounding lines
of your startup file, and the rest of your `PATH`, come back byte for byte as
they were.

## Scopes

| Scope | Who it affects | What it needs |
| --- | --- | --- |
| user | your account only | nothing — no `sudo`, no administrator |
| machine | every account on the machine | `sudo` on Unix, an elevated prompt on Windows |

The same wiring logic serves both; only the files differ.

## Which shell gets wired

A shell is wired one file at a time, and one file per install: wiring the same
hook into several startup files of the same shell means running it several times
per session, which buys nothing and doubles what you have to remove later.

On Windows the wiring is done by SSHakku itself:

```text
sshakku install [--shell=auto|bash|zsh|powershellcore|windowspowershell]
                [--shell-exe=<interpreter>]   # ask that interpreter where its files are
                [--profile=<file>]            # or name the file yourself
                [--scope=user|machine]        # default: user
                [--hosts=all|current]         # PowerShell only, default: all
                [--no-path]                   # skip the PATH step
sshakku uninstall [the same selectors]
```

Two selectors described elsewhere on this page are not there yet, and asking
for one is a usage error rather than something quietly ignored: `--rc`, for the
opt-in non-login wiring, and `--dry-run`. On Linux and macOS the non-login
wiring is `make install-user`'s `WIRE_BASHRC=1`/`WIRE_ZSHRC=1`, which is
unaffected by any of this.

`sshakku install` with nothing else wires **the shell you ran it from**: from a
PowerShell window it wires that PowerShell, from Git Bash it wires that bash.
That is what makes the flags above the exception rather than the rule.

### How the shell is worked out

In order, first answer wins:

1. `--profile` — you named a file, so that file is wired. If `--shell-exe` was
   given too and the file is not one of that interpreter's own profiles, you are
   told so and it is wired anyway.
2. `--shell-exe` — the interpreter is asked where *its* startup files are, which
   is also how the surrounding environment is found (see below).
3. `--shell` — the named shell is looked for on `PATH`.
4. Nothing — the process that started SSHakku is looked at, and its ancestors,
   until a shell is recognised.

If none of those answers, the install stops and asks for `--shell` rather than
guessing. A wiring written into the wrong file is worse than one not written.

### Which shells can be wired where

| `--shell` | Linux | macOS | Windows |
| --- | --- | --- | --- |
| `bash` | yes | yes | yes — Git Bash |
| `zsh` | yes | yes | no such shell here |
| `powershellcore` | possible, not supported | possible, not supported | yes — PowerShell 6 and later |
| `windowspowershell` | does not exist there | does not exist there | yes — Windows PowerShell 5.x |

"Possible, not supported" is a deliberate line, and it is drawn by the tests
rather than by the code: PowerShell runs perfectly well on Linux, and SSHakku
would have no trouble writing a profile there, but every combination that is
offered is a combination that has to be exercised, and this one is nobody's
daily driver. The same line runs through the Unix column: `zsh` on Linux and
`bash` on macOS are wired on request and work, but each platform's own login
shell is what the test suite holds. Naming an unsupported combination tells you
so, by name, instead of half-doing it.

## Where the hook goes, per shell

### bash and zsh

The login file is the primary one and is always wired: `~/.bash_profile` or
`~/.zprofile` for a user install; for a machine install, a file of SSHakku's own
in `/etc/profile.d/` on Linux and under Git Bash, and a block in `/etc/zprofile`
(zsh) or `/etc/profile` (bash) on macOS. A machine-wide install for zsh on Linux
is refused rather than guessed at: which file that shell reads system-wide is
the distribution's own choice, so name it with `--profile`. Where a drop-in
directory already exists beside the file
(`~/.bash_profile.d/`, `~/.zshrc.d/`, `/etc/bash/bashrc.d/`), a small file is
dropped into it instead of a block being added to the profile: the directory
existing is taken as saying that it is read.

A login shell does not fire for every new terminal — a plain tab, or a
multiplexer pane, usually starts a non-login shell that reads the rc file
instead. `--rc` will wire that one too, additively and never as a replacement —
it is the selector named above as not yet there, so today the login file is the
only one `sshakku install` writes.

Under Git Bash and other MSYS-derived environments, the same files are wired,
with two differences that are invisible once they work: the environment's own
root (and so its `etc/profile.d`, for a machine install) is discovered from the
interpreter rather than assumed, and paths are translated between the Windows
spelling SSHakku uses and the POSIX spelling bash reads, using that
environment's own translator.

### PowerShell

PowerShell keeps four profiles, and `$PROFILE` on its own is the fourth of them:

| Scope | `--hosts=all` (default) | `--hosts=current` |
| --- | --- | --- |
| user | `$PROFILE.CurrentUserAllHosts` | `$PROFILE.CurrentUserCurrentHost` |
| machine | `$PROFILE.AllUsersAllHosts` | `$PROFILE.AllUsersCurrentHost` |

All hosts is the default because a working `ssh` is not a property of the window
you are typing in: the console, Windows Terminal, the editor's integrated
terminal and a script host are all places you may run `ssh` from, and only the
all-hosts profile reaches every one of them.

The paths are **asked of the interpreter**, never assembled from a template. Two
common setups make an assembled path wrong: a `Documents` folder redirected into
OneDrive, and PowerShell installed somewhere other than the usual place — from
the Store, portable, or side by side with another version. Asking costs one
short process and is right in all of them.

Windows PowerShell 5.x and PowerShell 6+ are **separate targets**: different
executables, different profile directories, and — measurably — different
execution policies for the same account. Wiring one does not wire the other, and
an install that notices the other edition on the machine says so and gives you
the command that would wire it. Note that PowerShell 6 and 7 share their
per-user profile directory, so wiring one covers the other, and keeps covering
it across an upgrade.

Where a `Profile.d` directory sits beside the profile it is used as a drop-in
directory, on one condition that does not apply to bash: PowerShell reads no
such directory by itself — only your own profile can — so the profile is checked
for the code that reads it. Where nothing reads it, the block goes into the
profile instead, and you are told why.

## Where the files go on Windows

| | user | machine |
| --- | --- | --- |
| binary | `%LOCALAPPDATA%\Programs\sshakku\` | `%ProgramFiles%\sshakku\` |
| rendered hook | `%LOCALAPPDATA%\sshakku\` | `%ProgramData%\sshakku\` |
| `PATH` | the account's own environment | the machine's environment |

Git Bash uses the same locations: it is a shell on this machine, not a machine
of its own.

`make install` puts the binary in the first of those and then runs **the copy it
just placed** to do the wiring, which is what makes the recorded directory the
installed one rather than a second answer written down somewhere and free to
disagree; `make install-user` does the same under your account. The defaults are
the table above and are yours to change:

```sh
make install       BINDIR="/c/tools/sshakku"
make install-user  USER_BINDIR="/d/apps/sshakku"
```

Give the directory in the spelling the shell running `make` reads (`/c/…`),
since that is the shell that creates it. `DESTDIR`, which on Unix stages an
install under a second root, is refused here rather than obeyed halfway: a path
on this system names the drive it is on, so there is no second root to put it
under.

## PATH

On Unix nothing is recorded anywhere, because there is nowhere to record it: a
session searches what its startup files built, and no stored environment
outlives it. A machine install puts the binary somewhere every session already
searches; a per-user `make install-user` puts `~/.local/bin` on `PATH` from the
block it wires, since that directory is not on the default `PATH` everywhere.
`sshakku install` does neither — it wires a shell and reports that this system
had nothing to record — so the binary is put in place by `make`, which is also
what makes the askpass helper reachable beside it.

Windows has no such directory, so there the install **records the change in your
environment**, for the account or for the machine depending on the scope. What
is recorded is the directory the program is in, so it is the copy `make` placed
that records it — run any other copy and it records where that one lives, which
is what you want when you are running it by hand from wherever you unpacked it.
This is the one thing an install does that outlives the shell, so it is done
carefully: the stored value is read and written in its raw form, so a `PATH`
that refers to other variables keeps referring to them rather than being
flattened into whatever they meant at that moment; the entry is added once,
however many times you install; the previous value is kept so it can be put
back; and uninstalling removes SSHakku's entry and leaves every other one
exactly as it was. `--no-path` skips the whole step.

## When your shell will never read the hook

An install that writes a perfectly good file into a shell that will not read it
has failed, and you would find out at your next login. So the install checks,
and tells you at the time:

- **An execution policy that forbids scripts.** Under `Restricted` — and, for an
  unsigned profile, `AllSigned` — PowerShell does not read your profile at all,
  and says nothing about it. The install reports the policy in force, for that
  edition, and the command that changes it. It does not change it for you: what
  your machine may run is your decision, or your administrator's.
- **A `Profile.d` nothing reads**, as above.
- **A location that cannot be written**, notably the all-users profile of a
  Store-installed PowerShell, which lives under a directory Windows protects
  from every account including administrators. You are told to install for your
  account instead.
- **Constrained language mode**, where a dot-sourced hook may not run.
- Sessions started with `-NoProfile` read no profile by design; nothing can wire
  those, and nothing pretends to.

## What is in place today

On Linux and macOS, everything above is what `make install` and
`make install-user` already do, through the shell installer they have always
used; `sshakku install` is being brought in to take that over, and the
Makefile keeps being the way the binary is put in place on every platform.

On Windows, the binary builds, its test suite runs, `make install` and
`make install-user` put it in the directories above, and `sshakku install` wires
the shells listed above. Three things stay out of it deliberately, and are not
oversights: the ssh-agent itself (Windows serves it as a service on a named
pipe, which is a different mechanism from the socket the Unix builds keep
healthy), the askpass helper, and the Credential Manager as a wallet. A wired
Windows session therefore gets the hook and the `PATH` and nothing more — it
opens silently, exactly as one on any other system does, and the session log
records what this platform cannot yet do rather than the shell reporting it as
a failure. `sshakku doctor` says the same in the report, and does not recommend
opening a new login shell to get an agent that nothing here can start.
