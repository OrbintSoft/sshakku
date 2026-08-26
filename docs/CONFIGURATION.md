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
| `SSHAKKU_KEY_LIFETIME` | `key_lifetime` | `8h` | How long an added key stays in the agent before it expires, as a Go duration (`30m`, `1h`, `8h`). Passed to `ssh-add -t`. A zero or negative value (`0`) disables expiry, so the key stays until the agent does. Windows has no expiry at all: the agent there refuses a key it is asked to hold for a time, so keys are added without one and stay until they are removed — the session log and `sshakku doctor` both say so, and this setting has no effect there. |
| `SSHAKKU_MAX_ATTEMPTS` | `max_attempts` | `3` | How many passphrase attempts to make per key before giving up. Values below `1` fall back to the default. |
| `SSHAKKU_GIVEUP_TTL` | `giveup_ttl` | `1h` | How long a key stays in the give-up state before it is retried, as a Go duration. A zero or negative value never expires (the state still clears at logout or reboot). |
| `SSHAKKU_NO_GIVEUP` | `no_giveup` | unset | When truthy, disables the give-up memory entirely: every shell retries every key. |
| `SSHAKKU_QUIET` | `quiet` | unset | When truthy, suppresses the user-facing failure notice on the terminal. |
| `SSHAKKU_COMMAND_TIMEOUT` | `command_timeout` | `10s` | How long to wait for something that should answer on its own — reading the wallet, checking the display — before giving up on it and falling back (usually to asking on the terminal). Lower it if you would rather be prompted quickly than wait on a wallet that may be locked. |
| `SSHAKKU_INTERACTIVE_TIMEOUT` | `interactive_timeout` | `2m` | How long to wait for something that is waiting for *you* — a password dialog, your desktop wallet's own unlock prompt, a CLI deferring to its desktop app for approval, or the macOS keychain asking you to allow an access. Raise it if you are ever still typing when SSHakku gives up; what follows a give-up is being asked for a passphrase that was already saved. |

Truthy means `1`, `true`, `yes`, or `on` (case-insensitive); in the config file a
boolean key (`no_giveup`, `quiet`) is a TOML `true` or `false`. A malformed
duration is ignored and the default is used; `sshakku config` shows the value
that was refused, and the session log records it as it happens.

Unlike the other durations, the two timeouts have no "wait forever" value: zero
and negative are refused and the default is used instead. A program with no
limit can hold up the shell that is waiting on it, which is exactly what they
exist to prevent.

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
command_timeout = "10s"
interactive_timeout = "2m"
```

Durations (`key_lifetime`, `giveup_ttl`, `command_timeout`,
`interactive_timeout`) are strings holding a Go duration,
`max_attempts` is an integer, and `no_giveup` and `quiet` are booleans. A missing
file is fine — SSHakku falls back to the environment and the defaults. A syntax
error discards the whole file; an unrecognised key is ignored while the keys
SSHakku understood stay in effect; `sshakku config` reports either, and the
session log records them as they happen. Because
the environment takes precedence, an exported variable overrides the file in
either direction — for example `SSHAKKU_QUIET=0` re-enables the notice even when
`quiet = true` in the file.

## Splitting the config across `config.d/`

Settings can also be split across several files instead of one
`config.toml`. Every `*.toml` file directly under
`~/.config/sshakku/config.d/` is read in filename order, each overriding any
key it sets on top of `config.toml` and the files before it — prefix the
names with numbers (`00-defaults.toml`, `50-work.toml`) to control that
order. A key a file doesn't mention is left untouched by that file; setting a
list key (e.g. `wallet_store_include`) to `[]` explicitly clears it, rather
than being treated as "not mentioned".

```toml
# ~/.config/sshakku/config.d/50-work.toml
key_lifetime = "2h"
```

Both `config.toml` and `config.d/` are optional and can be used together or
alone. A syntax error in one `config.d/` file discards only that file — every
other file, and `config.toml`, still apply — reported the same way a problem in
`config.toml` itself is.

## Seeing what took effect

With settings arriving from three places — the environment, `config.toml`, and
each file under `config.d/` — what is in force is not necessarily what any one
file says. `sshakku config` prints it: every setting, the value actually being
used, and which of the three put it there, along with the files that were read
in the order they were read. A value SSHakku refused — a malformed duration, a
name it will not use — is shown there too, so a mistake can be found without
going through the session log.

`sshakku config --edit` opens `config.toml` in your editor, creating it from a
commented template if you have none, and checks it when you close the editor:
whether it still parses, whether a value in it was refused, and whether a
`config.d/` file or an exported variable decides a key it sets. It edits that
one file — a drop-in is never opened for you.

See [CLI.md](CLI.md#sshakku-config) for both.

## Choosing the secret backend

Left alone, SSHakku uses the wallet your operating system provides itself, and
you need configure nothing: a dedicated Secret Service collection on Linux (KDE
Wallet, GNOME Keyring, or KeePassXC via its Secret Service integration — see the
next section), the Keychain on macOS, and the Credential Manager on Windows.

Which wallets you can name depends on the system, because two of them *are* the
system:

| Value | Linux | macOS | Windows |
| --- | --- | --- | --- |
| `"secret-service"` | ✅ the default | ❌ no such API exists there | ❌ no such API exists there |
| `"keychain"` | ❌ no such API exists there | ✅ the default | ❌ no such API exists there |
| `"credential-manager"` | ❌ no such API exists there | ❌ no such API exists there | ✅ the default |
| `"keepassxc"` | ✅ | ✅ | ✅ by the `cli` route, which needs `keepassxc_database` |
| `"1password"` | ✅ | ✅ | ❌ not yet driven there |
| `"bitwarden"` | ✅ | ✅ | ❌ not yet driven there |

Naming one your system has not got is a mistake in the configuration, not a
wallet waiting for you to install something: SSHakku logs it and carries on with
your platform's default, so a stale config file cannot take your login shell
down with it.

Two of those cells say *not yet driven there* rather than *no such thing
exists*, and the difference is deliberate. Those wallets are programs that could
perfectly well be installed on Windows; what is missing is not the program but
anyone having run SSHakku against it there and a test holding it to that. A
wallet is offered on a platform once it has been — which is how KeePassXC came
to be offered on Windows, and what the other two are still waiting for.

Like `wallet_store_mode`, these four keys are config-file only — an account
identity (an email address, a vault name) doesn't fit a single environment
variable, and there is no benefit to leaving it sitting in the process
environment instead of the file:

```toml
secret_backend = "bitwarden"                  # see the table above for what your system accepts
onepassword_vault = "sshakku"                  # consulted only when secret_backend = "1password"
bitwarden_email = "you@example.com"            # consulted only when secret_backend = "bitwarden"
bitwarden_server = "https://vault.example.com" # optional; a self-hosted Vaultwarden instance
```

- `"secret-service"` (the Linux default) is described in the next section.
- `"keychain"` stores passphrases as generic-password items in the macOS
  Keychain, one per key, scoped to your account. SSHakku talks to
  Security.framework directly, never shelling out to the `security`
  CLI — a plain status read has no secret material to protect from `ps`/argv
  exposure, but a passphrase does, and `security add-generic-password -w`
  has no way to take it other than on the command line. It is the default
  there, so a macOS install needs no configuration to use it.
- `"credential-manager"` stores each passphrase as a generic credential in the
  Windows Credential Manager, one per key, under your own account. SSHakku
  calls the credential API directly rather than shelling out — there is no
  command-line that would give a stored secret back in any case. It is the
  default there, so a Windows install needs no configuration to use it. Note
  what guards it, because it differs from the other two: your Windows sign-in,
  and nothing further. You are never asked to unlock this wallet, there is no
  separate password, and no permission is asked per program — so anything
  running as you can read what is stored in it. `sshakku doctor` says so beside
  the wallet's name.
- `"1password"` shells out to the `op` CLI. `onepassword_vault` names the vault
  to keep the entries in; SSHakku tags every item it creates there and only
  ever reads or deletes its own, so the vault does not have to be one you keep
  for nothing else. `op` must already be signed in (the desktop app's own integration, or, for a Business/
  Teams account, `OP_SERVICE_ACCOUNT_TOKEN` in the environment) — SSHakku
  never drives an interactive `op signin` itself.
- `"bitwarden"` shells out to the `bw` CLI, against bitwarden.com or a
  self-hosted Vaultwarden instance named by `bitwarden_server`. Unlike the
  other backends, `bw` has no non-interactive unlock: SSHakku prompts for the
  Bitwarden master password itself, fresh, every time it needs the vault —
  graphically when a display is available, otherwise on the terminal — and
  never caches or stores that password anywhere. This is a second, independent
  password from any SSH key passphrase; the point of storing passphrases in
  Bitwarden at all is that you still never retype an SSH key's own passphrase,
  only the one Bitwarden master password, once per shell that actually touches
  the vault.
- `"keepassxc"` names KeePassXC as your wallet on every platform. How SSHakku
  reaches it is chosen for you and can be overridden — see the next section.
- A `secret_backend` value this system does not offer — a misspelling, or a
  wallet belonging to the other platform — falls back to your platform's
  default and is logged.

## Choosing how KeePassXC is reached

You name the wallet, not the mechanism: `secret_backend = "keepassxc"` works on
every OS SSHakku supports. SSHakku then picks a way to reach it — the Secret
Service on Linux, which KeePassXC implements itself; its local socket protocol
on macOS; and `keepassxc-cli` against the database file on Windows, where that
protocol is served over a named pipe SSHakku cannot yet speak. Only that last
one needs anything more from you, and it is one line: `keepassxc_database`,
naming the file, since a file on disk cannot be discovered the way a running
KeePassXC can be asked what it has open.

If you would rather decide, `keepassxc_route` says so, and then that route is
used **and no other**: an unavailable one is reported by name instead of being
quietly swapped for a different one.

```toml
secret_backend = "keepassxc"
keepassxc_route = "native"   # "auto" (default), "secret-service", "native", or "cli"
keepassxc_database = "~/secrets.kdbx"  # only the "cli" route needs to be told where the database is
keepassxc_key_file = "~/secrets.key"   # optional, for a database that also uses a key file
keepassxc_no_password = true           # only if the key file above is the database's *only* key
```

- `"auto"` (the default) is the only value that chooses, and the only one that
  falls back.
- `"secret-service"` reaches KeePassXC through the freedesktop Secret Service,
  exactly as KDE Wallet and GNOME Keyring are reached. Linux only — macOS has
  no such API to implement, and pinning it there tells you so.
- `"native"` speaks KeePassXC's local socket protocol to a running instance
  with its database unlocked, the same one its browser extension uses. It needs
  **Browser Integration** switched on in KeePassXC's settings, and the first
  time it connects KeePassXC asks you to approve SSHakku once; the approval is
  remembered afterwards. Because it talks to a KeePassXC you have already
  unlocked, it never asks you for anything again.
- `"cli"` runs `keepassxc-cli` against the database file, so it works with no
  KeePassXC running — but the database has to be opened each time, so it asks
  for its password rather than being silent. It asks once per session, not once
  per key.

  Unless there is no password to ask for. A database whose **only** key is a key
  file is opened with that alone, and `keepassxc_no_password = true` says so:
  nothing is asked, at that login or any other, and keys load in a session with
  nobody at a screen to answer. SSHakku never works this out from
  `keepassxc_key_file` being set, because a database can carry a key file *and* a
  password — saying nothing means there is one. Set it wrongly and every
  operation on the database is refused; set it rightly and remember that the
  wallet's lock is then worth exactly what the key file's own permissions are
  worth.

Routes are not tied to an operating system; only the default is. On Linux you
can pin `"native"` or `"cli"` and bypass the Secret Service entirely.

An unrecognised `keepassxc_route` falls back to `"auto"` and is logged, so a
typo never silently pins a route you did not name.

### What KeePassXC can and cannot do for you

SSHakku's entries live in an **SSHakku** group in your database (renameable —
see [Naming the compartment](#naming-the-compartment-they-are-kept-in)), one per key,
under a URL built from the same per-key name the other backends use
(`sshakku://SSHakku-Key-id_ed25519`), which is also the entry's username.

`sshakku forget` behaves differently by route, because the two can do different
things:

| | `"native"` / `"secret-service"` | `"cli"` |
| --- | --- | --- |
| read and write a passphrase | yes | yes |
| `sshakku forget <key>` | **no** | yes |
| `sshakku forget --all` | no | yes |
| asks you for anything | no | the database password, once per shell |

KeePassXC's local protocol can read and write an entry but has no way to delete
one, so on the `"native"` route `sshakku forget` fails and names the entry for
you to remove in KeePassXC. It will not tell you a passphrase is gone while it
is still in your database. The `"cli"` route can delete, so `forget` works
there.

## Where passphrases are stored

The default (`secret_backend = "secret-service"`) stores passphrases in their
own Secret Service collection, labelled and aliased
`sshakku` (renameable — see below), separate from the desktop's default wallet (`kdewallet` on KDE, the
login keyring on GNOME). SSHakku talks to the Secret Service D-Bus API
(`org.freedesktop.secrets`) directly — the same API KDE Wallet and GNOME
Keyring both implement — rather than shelling out to `secret-tool`, so it can
unlock its collection only for the seconds a lookup or store takes and lock it
again immediately after, instead of relying on the desktop's fixed idle
timeout to bound how long an unlocked entry is queryable by another process of
the same user.

On GNOME Keyring that collection is made through a dialog, which a machine with
no graphical session cannot show: log in there before it exists and your keys
still load, but you are asked for each passphrase every time, because nothing
can be saved. Make the collection once from a desktop session on the same
account and it goes on serving logins that have no screen at all. KDE Wallet is
not subject to this — PAM opens it as part of the login — and neither is a
backend that reaches its wallet by running a program (`keepassxc`, `1password`,
`bitwarden`).

You do not have to work this out from the passphrase prompts coming back:
`sshakku doctor` says which of the pieces are there, and where the collection
cannot be created it says so and why. The report only looks — it never creates
the collection to find out, and it comes back even when the wallet does not
answer at all. To have it prove the wallet really works, ask for that: `sshakku
doctor --test-backend` stores, reads back and deletes an entry for real.

Because the collection is separate from the desktop's default, it will not
appear in wallet GUIs that only browse the default collection (e.g.
KWalletManager on KDE, where `ksecretd` — the Secret Service backend — and
`kwalletd6` — KWalletManager's own backend — are different daemons entirely).
Inspect it with `secret-tool` if needed, e.g.
`secret-tool search --unlock service SSHakku-Key-id_rsa`.

Upgrading from a version that stored passphrases in the default collection: an
already-stored key is not found in the new `sshakku` collection, so it
re-prompts once on the first load after upgrading and is then stored under
`sshakku` — no migration, and every load after that behaves as before.

With `secret_backend = "keychain"` (macOS), each key's passphrase is a
generic-password item in your default (login) keychain, labelled `SSH
Passphrase for <keyname>` and scoped to your account. Unlike the Secret
Service default, these items are ordinary entries in the same keychain
everything else uses, so they're visible in Keychain Access alongside your
other passwords, not tucked away in a separate collection.

With `secret_backend = "credential-manager"` (Windows), each key's passphrase is
a generic credential named after the key — `SSHakku-Key-id_rsa` — described as
`SSH Passphrase for <keyname>` and carrying the prefix as its user name, so the
column beside the target says where the entry came from. `cmdkey /list` shows
them, as does the Credential Manager in Control Panel; neither will show you the
secret, which no supported command-line will give back either. There is no
separate collection here any more than there is on macOS: the entries sit in the
same store as everything else the account has saved, and SSHakku reads, lists
and deletes only the ones carrying its own prefix.

### Naming SSHakku's entries

Whatever the backend, an entry is named after the key whose passphrase it holds,
behind a prefix that says who put it there: `SSHakku-Key-id_rsa` holds the
passphrase for `~/.ssh/id_rsa`. `service_prefix` changes that prefix.

```toml
# ~/.config/sshakku/config.toml
service_prefix = "SSHakku-Key"
```

Unlike most settings this one is config-file only, with no `SSHAKKU_*`
environment override. The prefix decides where your passphrases live, and a
variable exported in one shell but not in the next would have SSHakku save under
one name and look under another — which, from where you are sitting, is
indistinguishable from a wallet that has quietly emptied itself.

An absent or empty value uses the default. A value containing whitespace or a
`/` is refused, since some wallets read a `/` in an entry name as a folder
separator; the refusal is shown by `sshakku config`, recorded in the session
log, and SSHakku carries on with the default.

Choose the name with some care where the wallet is shared with the rest of the
system — the macOS login keychain, a Bitwarden vault. There the prefix is the
only thing separating SSHakku's entries from every other program's, and it is
what `sshakku forget --all` goes by when deciding what it may delete. A prefix
generic enough that another program might have picked it too (`ssh`, `key`,
`SSH-Key`) puts that program's secrets within reach of a command meant to touch
nothing but SSHakku's own. The default names SSHakku itself, which is the point
of it.

Changing the setting renames nothing that is already stored. The old entries stay
in the wallet, outside SSHakku's view: each key prompts once more and is saved
under the new name, and `sshakku forget --all` will not remove the old ones — it
deletes what SSHakku manages, and under the configuration you have just written
those are no longer it. Remove them with your wallet's own tools if you want them
gone.

### Naming the compartment they are kept in

Where the wallet has room for one, SSHakku makes itself a compartment and keeps
its entries there and nowhere else: a Secret Service collection on Linux, a
group in your KeePassXC database. Left alone, the collection is `sshakku` and
the group is `SSHakku` — each keeps the name it has always had, so nothing you
have already stored moves. `secret_container` names both at once.

```toml
# ~/.config/sshakku/config.toml
secret_container = "my-own-compartment"
```

| `secret_backend` | what the name applies to |
| --- | --- |
| `"secret-service"` | the collection SSHakku creates, both its label and its alias |
| `"keepassxc"`, `"cli"` route | the group SSHakku creates in the database |
| `"keepassxc"`, `"native"` route | nothing — see below |
| `"1password"` | nothing; the vault is named by `onepassword_vault` |
| `"keychain"`, `"bitwarden"` | nothing; there is no compartment to name |

On the KeePassXC `"native"` route the setting has no effect, and SSHakku will
not pretend otherwise. KeePassXC files an entry into a group of its own
choosing — the one it keeps for entries saved over that protocol — and the group
name it is handed is not what it goes by. Use the `"cli"` route if the group
matters to you, or move the entries in KeePassXC afterwards.

On the macOS keychain and in a Bitwarden vault there is no compartment at all:
the entries sit among everything else, and `service_prefix` above is what tells
them apart. A 1Password vault keeps its own key, because a vault is not
something SSHakku makes for you — it exists first, with your team's access rules
on it.

Like `service_prefix`, this is config-file only, for the same reason: it decides
where your passphrases live, and a variable exported in one shell and not the
next would put them somewhere the next shell does not look.

An absent or empty value uses the default, and a value containing whitespace or
a `/` is refused — a KeePassXC group name is a path, and a `/` in it would nest
the group somewhere you did not ask for. So are the names your desktop uses for
its own wallets: `default`, `login`, `session`, `kdewallet`, in any casing. Each
refusal is shown by `sshakku config`, recorded in the session log, and SSHakku
carries on with its own default.

Those four are refused because SSHakku would not merely write into such a wallet
— it would adopt it. It treats its compartment as entirely its own, since it is
the one that made it; `sshakku forget --all` empties it without reading whose
entry is whose. Pointed at your login keyring, that is every password your
desktop keeps.

Which is also the one thing to be careful about with a name of your own:
**choose one nothing else has already taken.** If a collection or group by that
name is already there, SSHakku will use it rather than make a second, and from
then on it considers the contents its own. The list above is the names SSHakku
knows to refuse, not every name a wallet somewhere might be using.

Changing the setting moves nothing that is already stored. The old compartment
stays where it is with its entries in it, outside SSHakku's view: each key
prompts once more and is saved into the new one, and `sshakku forget --all` will
not empty the old one. Move or remove it with your wallet's own tools.

## Choosing which files are your keys

SSHakku looks in `~/.ssh` for files whose names begin with `id_`, which is
OpenSSH's own convention and covers the keys `ssh-keygen` creates when you do
not tell it otherwise. A key you named yourself — `work-github`, `deploy` — is
not found by that rule, and nothing goes wrong visibly: it is simply never
loaded and never listed. `key_dir` and `key_patterns` in `config.toml` state
the two halves of the rule. Like the include/exclude lists below they are
config-file only, since a list of patterns does not fit one environment
variable cleanly:

```toml
key_dir = ".ssh"                       # relative to your home directory
key_patterns = ["id_*", "work-*"]      # file names, matched as shell globs
```

- `key_dir` is where SSHakku looks. A relative path is relative to your home
  directory, a leading `~/` means the same thing, and an absolute path is taken
  as it is. It is not searched recursively: keys in a subdirectory are not
  found, and neither is anything that is not a regular file.
- `key_patterns` are matched against the **file name** alone, not the path,
  with the usual shell wildcards (`*`, `?`, `[a-z]`). A file matching any one
  pattern in the list is a key. An empty list, or a pattern that is not valid,
  is refused and logged, and the default rule stays in force — so a typo does
  not silently leave you with no keys at all.

Both settings apply everywhere SSHakku decides what your keys are: what
`sshakku load-keys` adds at login, and what `sshakku doctor` reports. The
report names the directory it read, so if it lists nothing you can see which
directory it was told to look in.

Widening the patterns does not widen what SSHakku is willing to load. These are
never treated as keys, whatever you write:

- `*.pub` — the public halves;
- the files OpenSSH keeps in that directory for its own use: `config`,
  `known_hosts` (and `known_hosts.old`), `authorized_keys` (and
  `authorized_keys2`), `environment`, `rc`.

So `key_patterns = ["*"]` means "every key in there", not "every file in
there". As with the wallet names above, that list is what SSHakku knows to
leave alone, not proof that a file matching your patterns is a key — point
`key_dir` at a directory full of other things and SSHakku will try them.

### When the directory is not there

An account with no `~/.ssh` at all is normal: there are no keys, the shell
opens, and nothing is said. A directory you asked for by name is different — a
mistyped `key_dir` looks exactly like a directory with no keys in it, so
SSHakku says so — on the terminal and in the session log — instead of loading
nothing quietly. It does
not fall back to `~/.ssh`: you asked for a directory, and loading keys you did
not ask for is the worse of the two answers.

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

By default every key SSHakku finds — see
[Choosing which files are your keys](#choosing-which-files-are-your-keys) — is
proactively added to the agent at
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

`--all` enumerates every stored entry directly, which every backend supports
except the `secret-tool` fallback (used only when the default Secret Service
backend can't reach a D-Bus session bus at all): `secret-tool` has no verb to
list entries without already knowing their exact attributes, so in that
situation `--all` fails with an explanatory error and the named form must be
used instead.

## Key expiry and the wallet

Keys are added to the agent with a lifetime (`SSHAKKU_KEY_LIFETIME`, default 8h).
When that elapses the agent drops the key; the passphrase stays in the OS wallet,
so re-adding the key never asks you to retype it.

- **Opening a new login shell** re-adds any expired key automatically: SSHakku
  sees the fingerprint is no longer in the agent and re-adds it from the
  wallet, silently. Because every shell shares one agent on a fixed socket,
  this refills the key for all terminals at once — but only a login shell
  runs the hook that triggers it (see
  [Requirements](../README.md#requirements)); a plain new terminal tab that
  doesn't start one won't, unless the optional `.bashrc`/`.bashrc.d` wiring
  (`make install-user WIRE_BASHRC=1`) is also in place.
- **In a still-open terminal** where a key just expired, the next `ssh` (or
  `git`, `rsync`, or any program that uses ssh) is routed through SSHakku's
  askpass broker. The broker fetches the passphrase from the wallet and hands it
  to ssh without prompting on the terminal. Only if the wallet entry is missing,
  the wallet does not exist, or wallet access fails does it fall back to prompting
  on the terminal — and a passphrase typed at that fallback is then stored in the
  wallet for next time.

The askpass routing is wired into every login shell — interactive or not —
and always tries the wallet first, whether or not a graphical secret prompter
is available: a GUI only changes *how* a wallet miss is then prompted for
(a dialog versus the terminal), never whether the wallet is consulted at all.
It is passive plumbing (it only matters if ssh later asks for a passphrase),
so a `scp`/`rsync`/`git` script run from a login shell can still be refilled
from the wallet silently instead of failing outright for lack of a tty;
proactive key loading (re-adding expired keys from the wallet on its own)
remains interactive-only, since a wallet-miss prompt may write to the
terminal.

See [Hardening](HARDENING.md#a-short-key-lifetime) for why a short lifetime is
worth keeping.

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

## Choosing the dialog you are asked in

Where your session has a screen, SSHakku asks for a passphrase in a dialog
rather than on a terminal you may not be looking at. Which dialog is normally
not something you have to say: it uses the first one your desktop has, trying
`pinentry` (which comes with GnuPG and is drawn with whichever toolkit your
distribution built it for), then KDE's `kdialog`, then GNOME's `zenity`.

GnuPG has builds of `pinentry` that draw on a terminal rather than on a screen —
`pinentry-curses` and `pinentry-tty` — and on some systems one of those is what
`pinentry` runs. Where you have a screen, SSHakku passes over such a build and
uses a dialog you do have instead, so having the console one installed does not
cost you the window.

`gui_prompter` decides instead:

```toml
gui_prompter = "zenity"   # "auto" (default), "pinentry", "kdialog", "zenity", or "none"
```

`"none"` means you are always asked on the terminal, even sitting at the screen.
Naming one uses that one and no other — if it cannot ask you are asked on the
terminal, and the session log names it and says why, rather than being handed a
dialog you did not choose. Naming `pinentry` where the installed build draws on
a terminal is one of those cases: it is the one you named, so nothing else is
substituted for it.

On macOS the choice is between `"auto"` (the system's own dialog, drawn through
`osascript`), `"osascript"`, and `"none"`. A name belonging to another operating
system is refused, `sshakku config` reports the refusal, and `"auto"` applies.

Where there is no screen at all — logged in over SSH, or booted into single-user
mode — you are asked on the terminal whatever this setting says, since a dialog
would have nowhere to appear. A dialog you close without answering is not the
same as one that could not be shown: closing it is an answer, and you are not
then asked the same thing somewhere else.

This is a config-file key only, like the wallet settings: it decides how you are
spoken to, and a variable exported in one shell but not the next would ask two
different ways for no reason you could see.

## Closing the prompt without answering

Closing the dialog costs you nothing: no passphrase is stored, and no key is
given up. The next login shell asks again from the first key, and `ssh` asks the
moment you use one of those keys. Pressing Ctrl-D at a terminal prompt is the
same gesture, and neither is reported to you as a failure.

What it means for the keys that come after it is yours to choose:

```toml
on_dismiss = "skip"   # "stop" (default), "skip", or "retry"
```

`"stop"` asks about no further key for the rest of that login, so shutting one
window you did not ask for does not leave you with one more of them per key.
`"skip"` turns down that key alone and goes on asking about the others, which is
what you want if you unlock some of your keys and not others. `"retry"` treats
closing the prompt as a wrong answer: the same key is asked about again until
`max_attempts` runs out, and it then ends the way any key that never opened
ends — you are told once, and it is left alone until the retry window passes.

This is a config-file key only, for the same reason as the dialog it answers.
