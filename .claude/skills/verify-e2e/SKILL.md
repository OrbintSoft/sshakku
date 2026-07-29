---
name: verify-e2e
description: Verify that a SSHakku feature actually works by driving the real binary through a user's scenario in a disposable environment, instead of inferring it from unit tests. Use before claiming any user-visible behaviour is done or fixed, when investigating a reported symptom, and whenever a test matrix cell is about to be marked covered.
---

# Verify a feature end to end

Unit tests answer "does the code do what it says". This skill answers the only
question a user cares about: **does the product do what it promises**. Those are
different questions and the second one is not implied by the first — a feature
can be dead while every unit around it passes, because the units are stubbed
exactly where the feature lives.

Use this before saying a behaviour is done, fixed, or covered.

## 1. Start from the promise, never from the code

Open [docs/FEATURES.md](../../../docs/FEATURES.md) and find the feature id. Copy
its "How you can tell" column — that sentence is your acceptance criterion, and
it was written without reference to any implementation, which is the point.

If the behaviour has no feature id, stop and add it first (rule 21). Do not
proceed by reading the source to work out what the behaviour "should" be: a
criterion derived from the implementation cannot catch an implementation that is
wrong.

Do not open the implementation until after the criterion is written down.

## 2. Reproduce before diagnosing

When the trigger is a reported symptom, reproduce it before forming any theory.
A theory built from reading code will explain the code, not the symptom.

Write down, in the activity's scratch file: the exact commands, the environment
they ran in, and what was actually observed. If the symptom cannot be reproduced,
that is the finding — say so rather than proceeding on the assumption that it is
real.

## 3. Pick the environment that can actually show the failure

| Scenario | Where to run it |
| --- | --- |
| Login/non-login shell wiring, agent lifecycle, real secret daemon | the container images under `test/containers/` — see `test/bats/helpers.bash` for the opt-in gate |
| Anything involving a real Secret Service, or a desktop without `kdialog` | the `gnome-keyring` image (a real wallet, no KDE tooling) |
| macOS Keychain, `zsh` login shells, Apple's OpenSSH | the macOS CI runner; there is no macOS container |
| Prompting on a terminal | a pseudo-terminal allocated by the test itself — the container has no controlling terminal |
| Install/uninstall | a scratch `DESTDIR`/`USER_HOME`, never the developer's own system |

Two rules about the environment:

- It must be **disposable**. Never verify against the developer's live session,
  agent, wallet, or `~/.ssh`.
- It must be able to *fail*. If the scenario cannot break in the environment you
  chose, the run proves nothing. Ask what the failure would look like there
  before running it.

## 4. Force the precondition

Most real failures live in a state that a fresh setup does not reach on its own.
The state is usually the point:

- **Key expired from the agent** — `ssh-add -D`, or a short configured lifetime.
  Do not open a new shell afterwards: the login hook would repair the state and
  hide the bug.
- **Empty wallet** — a throwaway backend or a wiped collection, not a fake.
- **No graphical session** — unset `DISPLAY` and `WAYLAND_DISPLAY` *and* keep
  `kdialog` off `PATH`; either alone leaves a path open.
- **No controlling terminal** — `setsid`, so a terminal fallback cannot rescue a
  run that was supposed to be silent.
- **Wrong passphrase** — feed a deliberately wrong one and let the retries run
  out.

Where the shell's own configuration can mask the outcome, neutralise it and say
you did: `AddKeysToAgent yes` in `~/.ssh/config` makes `ssh` re-add a key by
itself, which turns a failing run into a passing-looking one.

## 5. Drive the real binary

Run the installed or freshly built `sshakku`, through the real entry point the
user goes through — the login hook, `ssh`, `ssh-add` — not a Go test harness
calling internal functions.

Two failure modes to guard against, both of which produce a green-looking run:

- **The wiring is skipped.** Check the environment the scenario depends on is
  really set (`echo $SSH_ASKPASS`), rather than assuming a command that exited 0
  did something. An early `return 0` and a successful run are indistinguishable
  from the exit code alone.
- **Something else satisfies the outcome.** A key can be in the agent because
  `ssh` added it, not because SSHakku did. Assert on the *evidence of the path*,
  not only on the end state — the session log line naming the wallet is worth
  more than `ssh-add -l`.

## 6. Capture evidence

A claim that the feature works is only as good as what is recorded with it:

- the commands, verbatim;
- the user-visible output — including the fact that there was none, when silence
  is the promise;
- the relevant session-log lines;
- the state afterwards (`ssh-add -l`, the wallet entry, the touched files).

Real paths, hostnames, and log excerpts go **only** in the gitignored scratch
file (rule 7). What reaches a commit message, a document, or a PR is the
conclusion, not the transcript.

## 7. Report honestly

Say what was exercised and what was not. These are all acceptable outcomes and
all better than a bare "verified":

- "Driven end to end on the gnome-keyring image; the silent refill happens and
  the log line confirms it came from the wallet."
- "Verified on Linux only. The macOS half is type-checked and its first real run
  is the CI job."
- "Unit tests pass; the feature was not run end to end because *reason*."

Never present linters, unit tests, or a coverage number as evidence that a
feature works (rule 25). If the end-to-end run did not happen, the sentence to
write is that it did not happen.
