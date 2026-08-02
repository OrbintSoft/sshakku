# Features

What SSHakku promises its users. Each entry is an outcome you can observe from
outside the program — something you could check on your own machine without
reading any source code. The ids are stable: tests, the test matrix, and bug
reports refer to features by id, so a promise and the thing meant to guarantee
it can always be lined up.

This is deliberately *not* a description of how SSHakku works. How it works is
in [DEVELOPMENT.md](DEVELOPMENT.md); what each command does is in
[CLI.md](CLI.md); what you can change is in [CONFIGURATION.md](CONFIGURATION.md).
This page is only the list of claims — the things that, if they stopped being
true, would mean SSHakku is broken regardless of what its code does.

Whether each promise is actually exercised by an automated test is a separate
question, tracked in [TEST-MATRIX.md](TEST-MATRIX.md).

## Agent lifecycle

| Id | Promise | How you can tell |
| --- | --- | --- |
| F1 | Every shell finds a working `ssh-agent` on the same fixed socket, so `SSH_AUTH_SOCK` stays valid even across an agent restart. | `echo $SSH_AUTH_SOCK` names the same path in every shell, and `ssh-add -l` answers in all of them. |
| F2 | A healthy agent someone else started is adopted, never killed. Only a dead agent or a stale socket is cleaned up. | Start your own `ssh-agent` before logging in: its keys are still there afterwards, and its process is still running. |
| F3 | With no keys to load, SSHakku exits cleanly and changes nothing. | An account with an empty `~/.ssh` opens a shell with no error, no delay, and no agent complaints. |

## Passphrases and the wallet

| Id | Promise | How you can tell |
| --- | --- | --- |
| F4 | The first time a key is used you are asked for its passphrase once, and it is saved in the OS secret store. | The prompt appears exactly once; afterwards the entry exists in your keychain/wallet under the key's name. |
| F5 | Every later login shell loads that key silently, with no prompt. | Log out and back in: `ssh-add -l` already lists the key and nothing was typed. |
| F6 | After a key expires from the agent, the next `ssh` **in a shell that is already open** gets the passphrase from the wallet, with no prompt. | Wait out the key lifetime (or run `ssh-add -D`) without opening a new terminal, then `ssh` to a host: it connects silently. |
| F7 | The passphrase never travels through an environment variable, a command line, or a file on disk. | `ps` and `/proc/<pid>/cmdline` never show it while a key is being added; no file under the runtime directory contains it. |
| F8 | A wrong passphrase is retried a bounded number of times, then that key is left alone until the retry window passes — no repeated prompting in every new shell. | Answer wrong until it gives up: you are asked the configured number of times, told once that the key could not be loaded, and new shells stay quiet. |
| F9 | `sshakku forget` removes stored passphrases, and the next use asks again. Where the wallet gives SSHakku no way to delete, it says so and names the entry for you to remove yourself — it never reports a passphrase as forgotten while it is still stored. | After `sshakku forget <key>`, the wallet entry is gone and the following load prompts; with a wallet that cannot be deleted from, you are told which entry to remove and nothing claims otherwise. |
| F29 | You are asked for a passphrase wherever you can actually answer: in a dialog when your session has a screen to put one on, and on the terminal when it has not — logged in over SSH, or booted into single-user mode. A window with nowhere to appear is never waited on. | Sitting at the machine, the prompt is a dialog. `ssh` into that same machine and load the same key: you are asked on the terminal, straight away, instead of the shell hanging on a dialog you could never have seen. |
| F24 | A wallet that can only be opened with a password of its own asks you for it, once, rather than failing the shell — and being asked is the exception: a wallet already open never asks. | With KeePassXC closed you are asked for the database password and the key still loads; with KeePassXC open and unlocked, the same key loads with nothing typed. |

## Behaviour in the shell

| Id | Promise | How you can tell |
| --- | --- | --- |
| F10 | On success SSHakku is completely silent: nothing on stdout, nothing on stderr. | Opening a shell prints nothing, and command substitution or a script's output is never polluted. |
| F11 | Something the user should act on — a key that could not be loaded — is said once, in plain language, on the terminal. | The give-up notice appears once in the shell that tried, not on every prompt. |
| F12 | Logs record what happened and never contain a secret. | The session log shows the load decisions; grepping it for your passphrase finds nothing. |
| F30 | A command SSHakku does not recognise is answered by naming it and showing what the commands are — never by asking for a secret. Having SSHakku wired into your login shell changes nothing about that: it asks for a passphrase when `ssh` asks it to, and at no other time, whatever you type. | In a shell where SSHakku is wired in, mistype a command — `sshakku --forget`, say. You get an unknown-command error and the usage, straight away; the terminal does not go quiet waiting for you to type a secret, and nothing you type is echoed back as though it were one. |
| F21 | Nothing SSHakku waits on can hold your shell up with no end — neither a program it runs nor a secret store it calls into directly: one that neither answers nor fails is given up on, and you are asked on the terminal instead. How long to wait is configurable, separately for something expected to answer on its own and something that is waiting on you. | With a wallet that hangs rather than fails (locked, with an unlock prompt nobody can answer; a keychain waiting for someone to approve access), the shell still comes back and `ssh` asks you for the passphrase instead of waiting forever. |

## Diagnostics

| Id | Promise | How you can tell |
| --- | --- | --- |
| F13 | `sshakku doctor` explains the current agent and key situation, including problems SSHakku cannot fix by itself. | Break something (kill the agent, leave a stale socket) and the report names it. |
| F14 | `sshakku doctor --fix` repairs what it reports as fixable, and says what it did. | After `--fix` the same report comes back clean. |
| F15 | `sshakku doctor --test-backend` proves the configured secret backend can store, look up, and delete for real. | It performs a real round trip and reports pass or fail per operation. |
| F25 | `sshakku doctor` names the wallet it would use, and how it would reach it, and tells you when something that wallet needs is not there — without touching a single stored passphrase to find out. | Configure a wallet whose tool is not installed: the report names the wallet and says which piece is missing, instead of leaving you to discover it the next time `ssh` asks for a passphrase. |
| F31 | `sshakku doctor` shows the environment it is actually running in — every variable SSHakku reads, each with its value, or `unset` where you have set none. A shell that is wired wrongly can be read off the report instead of guessed at, and a setting you exported and forgot is visible next to the one it is overriding. Secrets are the exception, and the report says only whether they are set: what you paste into a bug report must never be a way of publishing one. | Run `sshakku doctor` in a shell SSHakku is wired into: `SSH_ASKPASS` and `SSH_ASKPASS_REQUIRE` are listed with the values that shell has, alongside any `SSHAKKU_*` setting you exported. Now export `SSHAKKU_HANDOFF_TOKEN` to a value you would recognise and run it again: the report tells you the variable is set, and that value appears nowhere in it. |

## Configuration

| Id | Promise | How you can tell |
| --- | --- | --- |
| F16 | The key lifetime in the agent is configurable, and a key really is dropped when it elapses. | Set a short lifetime; `ssh-add -l` stops listing the key once it passes. |
| F17 | Which secret backend is used can be chosen, and an unavailable one degrades to prompting rather than failing the shell. | Point the configuration at a backend that isn't running: the shell still opens and you are prompted. |
| F18 | Per-key policy is honoured: a key can be excluded from automatic loading, or from being stored in the wallet, without losing the other behaviour. | An excluded key is never auto-loaded but can still be added on demand; a non-stored key works in-session and leaves no wallet entry. |
| F22 | KeePassXC can be chosen as your wallet by name, on every OS SSHakku supports. How SSHakku reaches it is not something you have to state: the same setting works on Linux and on macOS. | Put `secret_backend = "keepassxc"` in the configuration on either OS: your passphrases are saved into, and read back from, your open KeePassXC database. |
| F26 | Only wallets your operating system actually has can be chosen. Naming one that cannot exist there is treated as a mistake in your configuration — SSHakku says so and carries on with your platform's own default — rather than as a wallet that is merely missing a piece you could go and install. | Put `secret_backend = "secret-service"` in the configuration on macOS: the session log says the value is not valid there, `sshakku doctor` names the keychain, which is the wallet actually in use, and never asks you to supply a D-Bus session bus that no Mac has. |
| F27 | SSHakku only ever touches entries it put there itself. Where your wallet also holds other applications' secrets, forgetting everything forgets everything *of SSHakku's*: nothing else is read, listed or deleted, whatever the wallet would have let SSHakku do. | Keep an unrelated secret in the same store — anything another program saved there — then run `sshakku forget --all`: your keys' passphrases are gone and that entry is still exactly where it was. |
| F32 | The name SSHakku's entries carry in your wallet is yours to choose, and every operation moves with it: what a first prompt saves, what a later shell reads back, and what `sshakku forget` removes always agree on one name. No configuration you can write leaves a passphrase stored under one name and looked for under another. | Set `service_prefix` to a name of your own and load a key: the wallet entry carries that name. `sshakku forget <key>` then removes that entry and the next load asks for the passphrase again; `sshakku forget --all` removes everything stored under the name you chose, and nothing stored under any other. |
| F33 | The compartment SSHakku keeps its entries in is yours to name — the collection it makes in your Linux wallet, the group it makes in your KeePassXC database — and it creates that one and uses no other. What it will not do is move into a compartment your desktop already keeps for itself: a name belonging to your desktop's own wallet is refused. Everything inside SSHakku's compartment is SSHakku's to delete, and that is a promise it can only keep about a compartment it made itself. | Set `secret_container` to a name of your own and load a key: the passphrase is in a collection by that name, and there is none named `sshakku`. Now set it to `default` and load again: the session log says the name is refused, your keys go to SSHakku's own collection, and every secret your desktop keeps in its login keyring is still there. |
| F23 | If you would rather decide how KeePassXC is reached, you can say so, and SSHakku uses that way and no other. Every way of reaching it is available wherever it can work, not only on the OS where SSHakku would have picked it. | Pin the route in the configuration: the chosen one is used even where SSHakku would have picked differently — on Linux you can bypass the Secret Service entirely — and if it is unavailable you are told which one failed instead of being silently moved to another. |

## Installation

| Id | Promise | How you can tell |
| --- | --- | --- |
| F19 | Installing wires SSHakku into login shells; uninstalling removes every trace of that wiring and leaves the surrounding files untouched. | Diff the touched shell files before and after: only SSHakku's own delimited block appears and disappears. |
| F20 | The opt-in non-login wiring is additive — it never replaces or disables the login hook. | With both wired, a login shell and a plain new tab each end up with a working agent, and removing the opt-in leaves the login path intact. |

## Keeping this current

A behaviour that users can observe belongs here before it is implemented, not
after — see rule 21 in `CLAUDE.md`. Add the feature, give it the next free id,
then write the test that verifies it and reference the id from
[TEST-MATRIX.md](TEST-MATRIX.md). Ids are never reused: a promise that is
withdrawn is removed from the table and its id retired, so an old test or bug
report can never silently come to mean something else.
