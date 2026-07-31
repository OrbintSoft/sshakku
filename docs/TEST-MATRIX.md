# Test matrix

Every combination of OS, secret-backend integration, session/display
environment, and failure scenario that matters to a real SSHakku user, and
whether an **integration** test exercises it -- a real `ssh-agent`/
`ssh-add`, a real secret-store daemon, a real shell, not a mock. A case
covered only by a unit test with a faked/mocked dependency counts as not
covered here: that's a different, already-answered question (does the logic
work in isolation), not this document's question (does the real thing work
end to end). Cases where an integration test wouldn't add anything over the
existing unit coverage are left out of this document entirely. Whether a
covered case runs on every merge or only on demand is a separate concern,
tracked by that case's own badge, not by this document.

Use this to see what's actually exercised versus assumed to work, and to
plan new integration tests where a cell shows a gap.

## Legend

| Mark | Meaning |
| --- | --- |
| ✅ | Covered by an integration test. |
| ❌ | Not integration-tested. |
| — | Not applicable (the combination doesn't exist in practice). |

## Secret store × OS

Which wallet a user can actually choose, on which platform, and what covers it
there. This is the first table on purpose: a user picks *KeePassXC*, not "the
secret-service backend", and how SSHakku reaches it is an implementation
detail that does not always carry across platforms. Organised the other way
round — by the mechanism — a wallet that is offered on one OS and absent on
the other has no cell to be missing from, and the gap becomes invisible.

| Wallet | Linux | macOS |
| --- | --- | --- |
| KDE Wallet (`ksecretd`/`kwalletd6`) | ✅ via `secret-service`, `kde.Dockerfile` | — (no D-Bus session bus or Secret Service on macOS) |
| GNOME Keyring | ✅ via `secret-service`, `gnome-keyring.Dockerfile` | — (same) |
| KeePassXC | ✅ via `secret-service`, `keepassxc.Dockerfile` | ❌ **not supported yet** — the Secret Service route does not carry over. The replacement is designed but not built: the native-messaging socket protocol as the primary route, `keepassxc-cli` as the fallback (features F22–F24, PLAN.md Phase 17 item 1) |
| 1Password (`op` CLI) | ✅ `TestOnePasswordBackendRealAccount`, `onepassword-real-account.yml` | ❌ supported, untested — that workflow runs on `ubuntu-latest` only |
| Bitwarden (`bw` CLI) | ✅ real backend against Vaultwarden, `desktop-stack.yml` | ❌ supported, untested — that job runs on `ubuntu-latest` only |
| macOS Keychain | — (Security.framework is macOS-only) | ✅ `TestDarwinKeychainClientRealRoundTrip` — a live `Add`/`Find`/`Update`/`Delete`/`List` round trip against a throwaway default keychain, plus the whole shell suite (`make test-bats`) driving it as the default backend |

A ❌ here means the promise is not covered on that platform, whether because
nothing tests it or because nothing implements it; the cell says which. A `—`
means the wallet cannot exist there at all.

## Secret-service backend × session/display environment

Linux only, and a second dimension rather than a second list of wallets:
which desktop session the daemon runs under. Whether each of these wallets is
available on a given OS at all is the table above.

`ksecretd`/`kwalletd6`, `gnome-keyring-daemon`, and KeePassXC all implement
the same `org.freedesktop.secrets` D-Bus API and are driven through the one
`SecretServiceBackend`/`TestSecretServiceBackendRealDaemon` test; only the
container session around it differs.

| Session | X11 | Wayland | No display |
| --- | --- | --- | --- |
| KDE (`ksecretd`/`kwalletd6`) | ✅ `kde.Dockerfile` | ❌ | — |
| GNOME (`gnome-keyring-daemon`, desktop session) | ✅ `gnome-keyring.Dockerfile` | ❌ | — |
| GNOME Keyring, headless-daemonized (no session/display at all) | — | — | ❌ |
| KeePassXC (standalone, any desktop) | ✅ `keepassxc.Dockerfile` | ❌ | — |

KDE/GNOME/KeePassXC's own daemons all need some display (real or virtual)
to run, hence the `—`s. XFCE has no native provider of its own -- it would
only ever host GNOME Keyring or KeePassXC, both already rows above -- so it
isn't listed separately unless a KeePassXC-under-XFCE test is actually
worth adding later.

## Backend round trips

What each backend's own store/lookup/delete cycle is exercised by, once the
table above has established where that backend is available at all.

| Backend | Covered by |
| --- | --- |
| Keychain | `TestDarwinKeychainClientRealRoundTrip` — a live round trip through `DarwinKeychainClient` (`Add`/`Find`/`Update`/`Delete`/`List`, including the duplicate-add and update-missing error paths) against a throwaway default keychain the `test-macos` job stands up first (`test/macos-keychain-setup.sh`), so the runner's login keychain is never touched |
| Secret Service | `TestSecretServiceBackendRealDaemon`, against each of the three daemons in its own container |
| 1Password (`op` CLI) | `TestOnePasswordBackendRealAccount`, `onepassword-real-account.yml` |
| Bitwarden (`bw` CLI) | real backend against Vaultwarden, `desktop-stack.yml` |

## Install and uninstall methods

| Case | Linux | macOS |
| --- | --- | --- |
| System-wide (`make install`/`uninstall`) | ✅ `test/linux-install-smoke.sh` | ✅ `test/macos-install-smoke.sh` |
| Per-user (`make install-user`/`uninstall-user`) | ✅ `test/linux-install-smoke.sh` | ✅ same script |
| Non-login shell wiring, opt-in (`WIRE_BASHRC=1`/`WIRE_ZSHRC=1`) (F20) | ✅ `test/linux-install-smoke.sh` (both the `bashrc.d` drop-in and the fallback-file shape) | ✅ `test/macos-install-smoke.sh` — the system-wide `/etc/zshrc` marker block and both per-user shapes (a drop-in into an existing `~/.zshrc.d`, a marker block in `~/.zshrc` when there is none), each checked to leave the login-shell wiring in place and to be removed again on uninstall |
| User bindir put on `PATH` from the hook (`install-user`, default; `WIRE_PATH=0` opts out) | ✅ `test/linux-install-smoke.sh` | ✅ `test/macos-install-smoke.sh` |

## Agent lifecycle and recovery scenarios

| Scenario | Linux | macOS |
| --- | --- | --- |
| Fresh start, no agent running yet | ✅ `TestEnsureAgentRealClean` | ✅ same |
| Agent already running and healthy, reused rather than restarted | ✅ `TestEnsureAgentRealHealthyReuse` | ✅ same |
| Agent reachable with zero keys loaded, still treated as healthy | ✅ `TestEnsureAgentRealReachableButEmptyIsHealthy` | ✅ same |
| Agent process killed (SIGKILL), stale socket left behind, recovered on next run | ✅ `TestEnsureAgentRealZombie` | ✅ same |
| Agent stopped gracefully (SIGTERM) after being registered, socket removed rather than left stale | ✅ `TestEnsureAgentRealGracefulStopRemovesSocket` | ✅ same |
| A foreign `ssh-agent` started before SSHakku ever runs, adopted rather than killed | ✅ `TestEnsureAgentRealForeignAdopted` | ✅ same |
| One dead agent of ours plus multiple healthy foreign agents | ✅ `TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID` | ✅ same |
| `sshakku doctor --fix` recovers a dead own-agent | ✅ `TestDoctorDetectsAndFixesDeadOursAgent` | ✅ same |
| Headless (no TTY at all), vault already has the passphrase, silent load (F5) | ✅ `TestLoadKeysHeadlessVaultHit` | ✅ `test/bats/shell-plumbing.bats` — the shell suite runs on macOS, where the vault is the real keychain rather than the Linux kernel keyring that test's stand-in uses |
| Live interactive TTY, vault empty, first-time passphrase prompt | ✅ `TestLoadKeysFirstTimePromptRealTerminal` — the loader runs in a child process holding a real pseudo-terminal as its controlling terminal, with an empty vault and no graphical prompter, while the test plays the user on the other end: the prompt reaches the terminal, the typed passphrase is never echoed back onto it, it is stored for next time, and the key lands in a real `ssh-agent` through the real out-of-band handoff | ✅ same |
| Wrong passphrase entered at the prompt | ✅ `TestLoadKeysWrongPassphraseRealTerminal` — the same live terminal, answered wrong every time: the user is asked exactly `MaxAttempts` times and no more, is told on the terminal that the key could not be loaded, and the key is marked as given up so later shells stop re-prompting; the rejected passphrase is neither echoed nor written to the vault, and the key stays out of the agent | ✅ same |
| Secret backend unresponsive or hangs | ✅ `TestSecretServiceUnresponsiveDaemon` — a live `SecretServiceBackend` round-trips, then `gnome-keyring-daemon` is SIGSTOP-frozen (still alive and still owning the bus name, so the bus cannot respawn it) and the next `Lookup`, bounded by the client's `CallTimeout`, must return an error promptly instead of blocking forever; `desktop-stack.yml` runs it in a throwaway container | ❌ the Keychain is reached through a synchronous Security.framework call, which SSHakku puts no deadline on and cannot cancel. There is no daemon to freeze from the outside the way this test does, but that is a reason the *scenario* has to be built differently — not a reason the case cannot exist. See PLAN.md Phase 17 |
| Secret-service daemon stopped/crashed mid-session (clean disconnect, not just slow) | ✅ `TestSecretServiceMidSessionFailure` (daemon-stopped) — a live `SecretServiceBackend` round-trips, then `gnome-keyring-daemon` is SIGKILLed (with its D-Bus activation file removed so the bus cannot respawn it) and the next `Lookup` must return promptly with an error, never a stale hit or a hang; `desktop-stack.yml` runs it in a throwaway container | — (Keychain has no comparable daemon to stop) |
| D-Bus session bus itself unreachable mid-session (lower-level than the daemon above) | ✅ `TestSecretServiceMidSessionFailure` (bus-unreachable) — same live round-trip, then the `dbus-daemon` session bus itself is SIGKILLed, severing the transport under the client, and the next `Lookup` must surface the broken connection promptly rather than hang | — |
| Real environment variables tampered (`SSH_AUTH_SOCK`, `SSH_ASKPASS`, `SSH_ASKPASS_REQUIRE`, `SSHAKKU_ASKPASS`, `SSHAKKU_HANDOFF_TOKEN`) | ✅ `TestTamperedEnvVarsHandledSafely` — real `os.Getenv` reads feeding the real `gatherReport`/`dispatch`/`askpass`: a hijacked or cleared `SSH_AUTH_SOCK` is flagged unreachable, a leftover `SSHAKKU_ASKPASS` marker never hijacks a real subcommand, and a malformed `SSHAKKU_HANDOFF_TOKEN` redeems nothing from the real store. Clearing `SSH_ASKPASS`/`SSH_ASKPASS_REQUIRE` is what the rows below exercise end to end, since without them the wallet is never consulted. | ✅ same |
| A key gone from the agent is refilled from the wallet, in a shell already open, with no terminal and no graphical prompter (F6) | ✅ `test/bats/askpass-broker.bats` — the installed login hook is sourced, then a real `ssh-add` runs with no controlling terminal, so nothing can fall back to a prompt, and with no handoff token, so only the wallet can answer; the session log line naming the wallet is asserted, not merely the key reaching the agent | ✅ same, against the real keychain |
| The askpass broker is wired in a session with no graphical prompter (F6) | ✅ `test/bats/askpass-broker.bats` and `TestAskpassEnvHeadless` — with no `DISPLAY`/`WAYLAND_DISPLAY` and no `kdialog`, `askpass-env` still emits all three exports | ✅ same |
| A wallet that never answers — present but never returning — does not hold up an `ssh` or a login shell (F21), per backend that reaches its wallet by running a program | ✅ `test/bats/askpass-broker.bats`, once per backend × entry point: `secret-service` (blocked `secret-tool`, bounded by `command_timeout`) and `1password` (blocked `op`, bounded by `interactive_timeout`). `TestNoCommandBlocksIndefinitely` covers every external program this code runs, `bw` included — reached there by answering the master-password prompt Bitwarden asks for before it will run anything | ✅ `1password`; `secret-service` — (that backend does not exist on macOS). Bitwarden as on Linux |
| A wallet that never answers, where the wallet is **not** a program (F21) | — (every Linux backend reaches its wallet by running one, or over D-Bus with its own `CallTimeout`) | ❌ the keychain is a synchronous Security.framework call with no timeout, context or cancellation, so nothing bounds it; F21 is worded about programs and does not reach it either. See PLAN.md Phase 17 |
| The shell suite itself runs on this OS, against the real login hook and real ssh binaries | ✅ `make test-bats` in `debian.Dockerfile` | ✅ `make test-bats` in the `test-macos` job, after the throwaway keychain is set up |

The two live-terminal rows above are covered by the Go suite rather than by
`test/bats/shell-plumbing.bats`: that suite runs in a container with no
controlling terminal at all, while the Go tests allocate a pseudo-terminal
of their own. They need no daemon and no container, so they run on both
platforms in the ordinary test run.

## Init system (systemd/OpenRC/...)

Not a real axis: SSHakku has no runtime dependency on any init system or
service manager. `internal/diagnose` reads `/proc/<pid>/cgroup` to *label*
the launching systemd unit when one exists, purely for `doctor`'s
diagnostic output -- it changes no behavior and has nothing to fall back to
on a non-systemd host (OpenRC included).

## Keeping this current

Whenever a new OS/target, integration, session/display environment, or
recovery scenario is implemented (or a new gap is found), add or update its
row here in the same change.
