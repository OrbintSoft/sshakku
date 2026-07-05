# Configuration

SSHakku reads its settings from environment variables and an optional TOML config
file. For each setting the precedence is **environment variable > config file >
built-in default**: an environment variable always wins, a value in the config
file applies when the variable is unset, and otherwise the built-in default is
used.

Set the environment variables before the login hook runs (for example in
`/etc/profile.d` or your shell profile) so `sshakku load-keys` and the askpass
broker see them.

## Settings

| Variable | Config-file key | Default | Effect |
| --- | --- | --- | --- |
| `SSHAKKU_KEY_LIFETIME` | `key_lifetime` | `8h` | How long an added key stays in the agent before it expires, as a Go duration (`30m`, `1h`, `8h`). Passed to `ssh-add -t`. A zero or negative value (`0`) disables expiry, so the key stays until the agent does. |
| `SSHAKKU_MAX_ATTEMPTS` | `max_attempts` | `3` | How many passphrase attempts to make per key before giving up. Values below `1` fall back to the default. |
| `SSHAKKU_GIVEUP_TTL` | `giveup_ttl` | `1h` | How long a key stays in the give-up state before it is retried, as a Go duration. A zero or negative value never expires (the state still clears at logout or reboot). |
| `SSHAKKU_NO_GIVEUP` | `no_giveup` | unset | When truthy, disables the give-up memory entirely: every shell retries every key. |
| `SSHAKKU_QUIET` | `quiet` | unset | When truthy, suppresses the user-facing failure notice on the terminal. |

Truthy means `1`, `true`, `yes`, or `on` (case-insensitive); in the config file a
boolean key (`no_giveup`, `quiet`) is a TOML `true` or `false`. A malformed
duration is ignored, logged to the session log, and the default is used.

## Config file

SSHakku also reads `~/.config/sshakku/config.toml` (more precisely
`$XDG_CONFIG_HOME/sshakku/config.toml`). The file is optional and TOML-formatted;
every key is optional and maps to one setting in the table above:

```toml
# ~/.config/sshakku/config.toml
key_lifetime = "8h"
max_attempts = 3
giveup_ttl = "1h"
no_giveup = false
quiet = false
```

Durations (`key_lifetime`, `giveup_ttl`) are strings holding a Go duration,
`max_attempts` is an integer, and `no_giveup` and `quiet` are booleans. A missing
file is fine — SSHakku falls back to the environment and the defaults. A syntax
error discards the whole file; an unrecognised key is ignored while the keys
SSHakku understood stay in effect; either is logged to the session log. Because
the environment takes precedence, an exported variable overrides the file in
either direction — for example `SSHAKKU_QUIET=0` re-enables the notice even when
`quiet = true` in the file.

## Where passphrases are stored

Passphrases live in their own Secret Service collection, labelled and aliased
`sshakku`, separate from the desktop's default wallet (`kdewallet` on KDE, the
login keyring on GNOME). SSHakku talks to the Secret Service D-Bus API
(`org.freedesktop.secrets`) directly — the same API KDE Wallet and GNOME
Keyring both implement — rather than shelling out to `secret-tool`, so it can
unlock its collection only for the seconds a lookup or store takes and lock it
again immediately after, instead of relying on the desktop's fixed idle
timeout to bound how long an unlocked entry is queryable by another process of
the same user.

Because the collection is separate from the desktop's default, it will not
appear in wallet GUIs that only browse the default collection (e.g.
KWalletManager on KDE, where `ksecretd` — the Secret Service backend — and
`kwalletd6` — KWalletManager's own backend — are different daemons entirely).
Inspect it with `secret-tool` if needed, e.g.
`secret-tool search --unlock service SSH-Key-id_rsa`.

Upgrading from a version that stored passphrases in the default collection: an
already-stored key is not found in the new `sshakku` collection, so it
re-prompts once on the first load after upgrading and is then stored under
`sshakku` — no migration, and every load after that behaves as before.

## Choosing which keys' passphrases are stored

By default every passphrase you type is stored in the wallet, so every key
refills silently after it expires from the agent. `wallet_store_mode` in
`config.toml` narrows that with an include or exclude list. Unlike every other
setting, these three keys are config-file only — there is no `SSHAKKU_*`
environment override, since a list of key names does not fit a single
environment variable cleanly:

```toml
wallet_store_mode = "exclude"       # "all" (default), "include", or "exclude"
wallet_store_include = ["id_rsa"]   # consulted only when mode = "include"
wallet_store_exclude = ["id_work"]  # consulted only when mode = "exclude"
```

- `"all"` (the default) stores every key's passphrase.
- `"include"` stores only the keys named in `wallet_store_include`; every
  other key is still used normally in the session, but its passphrase is
  never persisted, so it prompts again on the next expiry or login.
- `"exclude"` stores every key except those named in `wallet_store_exclude`.

The mode is authoritative: with `wallet_store_mode = "include"`,
`wallet_store_exclude` is never read even if present in the file, and vice
versa — the two lists never conflict. An unrecognised mode falls back to
`"all"` and is logged. The policy applies wherever a passphrase is written to
the wallet — the load-keys prompt-then-store path and the askpass broker's
miss-then-store fallback — so an excluded key is never stored from either
path.

## Choosing which keys are auto-loaded

By default every key found in `~/.ssh` is proactively added to the agent at
shell-init. `auto_load_mode` in `config.toml` narrows that with an include or
exclude list, in the same shape as `wallet_store_mode` above and, like it,
config-file only:

```toml
auto_load_mode = "exclude"       # "all" (default), "include", or "exclude"
auto_load_include = ["id_rsa"]   # consulted only when mode = "include"
auto_load_exclude = ["id_work"]  # consulted only when mode = "exclude"
```

- `"all"` (the default) auto-loads every key.
- `"include"` auto-loads only the keys named in `auto_load_include`.
- `"exclude"` auto-loads every key except those named in `auto_load_exclude`.

The mode is authoritative, exactly as for `wallet_store_mode`: the two lists
never conflict, and an unrecognised mode falls back to `"all"` and is logged.
This policy is independent from `wallet_store_mode` — it only controls
whether a key is *proactively* added at shell-init. A key excluded from
auto-load is not added to the agent automatically, but if you use it directly
(e.g. `ssh -i ~/.ssh/id_work`), the askpass broker still fetches or prompts
for its passphrase normally; narrowing auto-load shrinks the agent's blast
radius (fewer keys sitting in the agent for other same-user processes or
agent forwarding to reach), without affecting whether that key's passphrase
is stored.

## Forgetting stored passphrases

`sshakku forget <keyname>...` deletes the stored passphrase for one or more
keys (matched by file name, e.g. `id_rsa`), and `sshakku forget --all` deletes
every entry sshakku manages. Useful for testing, for revoking a passphrase
after suspecting it was exposed, or for removing an already-stored passphrase
so the key goes back to being prompted for and kept in memory only.

`--all` enumerates the dedicated `sshakku` collection directly, so it needs
the native Secret Service backend; if sshakku fell back to `secret-tool` (no
D-Bus session bus reachable), `--all` fails with an explanatory error and the
named form must be used instead.

## Key expiry and the wallet

Keys are added to the agent with a lifetime (`SSHAKKU_KEY_LIFETIME`, default 8h).
When that elapses the agent drops the key; the passphrase stays in the OS wallet,
so re-adding the key never asks you to retype it.

- **Opening a new terminal** re-adds any expired key automatically: SSHakku sees
  the fingerprint is no longer in the agent and re-adds it from the wallet,
  silently. Because every shell shares one agent on a fixed socket, this refills
  the key for all terminals at once.
- **In a still-open terminal** where a key just expired, the next `ssh` (or
  `git`, `rsync`, or any program that uses ssh) is routed through SSHakku's
  askpass broker. The broker fetches the passphrase from the wallet and hands it
  to ssh without prompting on the terminal. Only if the wallet entry is missing,
  the wallet does not exist, or wallet access fails does it fall back to prompting
  on the terminal — and a passphrase typed at that fallback is then stored in the
  wallet for next time.

The askpass routing is enabled only when a graphical secret prompter is available.
A headless session keeps ssh's own terminal prompting, and non-interactive
sessions (such as `scp`, `rsync`, or `git` in scripts) are never touched.

A short lifetime keeps the window in which a key sits in the agent small. Because
the wallet refills the key silently, you can keep that window short without ever
retyping a passphrase from memory — which also makes rotating keys cheaper.

## Retries and giving up

A wrong passphrase is retried up to `SSHAKKU_MAX_ATTEMPTS` times. On the graphical
path a stored passphrase that ssh-add rejects is treated as stale: SSHakku prompts
once and, on success, replaces it in the wallet.

When the attempts are exhausted, SSHakku gives up on that key and notifies you on
the terminal (unless `SSHAKKU_QUIET`). It then skips the key in every new shell for
`SSHAKKU_GIVEUP_TTL`, so a misconfigured key does not re-prompt on every terminal
you open. A later successful load clears the give-up state. The state is per-login
and lives in tmpfs, so logging out or rebooting clears it; `SSHAKKU_NO_GIVEUP`
disables it entirely.
