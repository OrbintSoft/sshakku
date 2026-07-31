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
| F9 | `sshakku forget` removes stored passphrases, and the next use asks again. | After `sshakku forget <key>`, the wallet entry is gone and the following load prompts. |

## Behaviour in the shell

| Id | Promise | How you can tell |
| --- | --- | --- |
| F10 | On success SSHakku is completely silent: nothing on stdout, nothing on stderr. | Opening a shell prints nothing, and command substitution or a script's output is never polluted. |
| F11 | Something the user should act on — a key that could not be loaded — is said once, in plain language, on the terminal. | The give-up notice appears once in the shell that tried, not on every prompt. |
| F12 | Logs record what happened and never contain a secret. | The session log shows the load decisions; grepping it for your passphrase finds nothing. |
| F21 | No program SSHakku runs can hold your shell up with no end: one that neither answers nor fails is given up on, and you are asked on the terminal instead. How long to wait is configurable, separately for a program that should answer on its own and one that is waiting on you. | With a wallet that hangs rather than fails (locked, with an unlock prompt that cannot be shown), the shell still comes back and `ssh` asks you for the passphrase instead of waiting forever. |

## Diagnostics

| Id | Promise | How you can tell |
| --- | --- | --- |
| F13 | `sshakku doctor` explains the current agent and key situation, including problems SSHakku cannot fix by itself. | Break something (kill the agent, leave a stale socket) and the report names it. |
| F14 | `sshakku doctor --fix` repairs what it reports as fixable, and says what it did. | After `--fix` the same report comes back clean. |
| F15 | `sshakku doctor --test-backend` proves the configured secret backend can store, look up, and delete for real. | It performs a real round trip and reports pass or fail per operation. |

## Configuration

| Id | Promise | How you can tell |
| --- | --- | --- |
| F16 | The key lifetime in the agent is configurable, and a key really is dropped when it elapses. | Set a short lifetime; `ssh-add -l` stops listing the key once it passes. |
| F17 | Which secret backend is used can be chosen, and an unavailable one degrades to prompting rather than failing the shell. | Point the configuration at a backend that isn't running: the shell still opens and you are prompted. |
| F18 | Per-key policy is honoured: a key can be excluded from automatic loading, or from being stored in the wallet, without losing the other behaviour. | An excluded key is never auto-loaded but can still be added on demand; a non-stored key works in-session and leaves no wallet entry. |

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
