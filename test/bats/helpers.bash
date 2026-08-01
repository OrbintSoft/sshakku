# Shared setup for the shell-plumbing bats suite: builds sshakku once per
# file, installs it fresh into an isolated prefix for each test (exercising
# the real `make install` sed-templating, not a hand-edited copy of the
# hook), and points every XDG/HOME path at a throwaway tree so tests never
# touch the real user or system state.
#
# This suite must only ever run in a disposable environment (the container
# test suite, a CI runner): a real machine can have its own system-wide
# sshakku install and secret store, and every test here manipulates real
# ssh-agent processes and real login-hook plumbing. SSHAKKU_TEST_ALLOW_BATS=1
# is the same explicit-opt-in pattern internal/keys's real-daemon integration
# tests already use, not a default-on convenience.
#
# The install paths, the secret store and the agent all live under one
# per-test root ($TEST_ROOT), which on darwin is a short path of its own — see
# setup() for why the length matters there.

REPO_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"

setup_file() {
	if [ "${SSHAKKU_TEST_ALLOW_BATS:-}" != "1" ]; then
		echo "skipping: set SSHAKKU_TEST_ALLOW_BATS=1 to run this suite — only safe in a disposable environment (the container test suite), never on a real machine with its own sshakku install" >&2
		exit 1
	fi

	SSHAKKU_BUILD_DIR="$BATS_FILE_TMPDIR/build"
	mkdir -p "$SSHAKKU_BUILD_DIR"
	make -C "$REPO_ROOT" build GO_BIN="$SSHAKKU_BUILD_DIR/sshakku" >&2
}

# trace appends a progress line to $SSHAKKU_BATS_TRACE when the caller set one,
# and does nothing otherwise. bats reports a test only once it has ended, so a
# run killed from outside for taking too long says nothing about how far it
# got; a trace written as the suite goes is the only record that survives.
trace() {
	[ -n "${SSHAKKU_BATS_TRACE:-}" ] || return 0
	printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*" >>"$SSHAKKU_BATS_TRACE"
}

# bounded runs a command with a hard time limit and without coreutils'
# `timeout`, which macOS does not ship. A tool that stops to ask a person
# something — a keychain wanting a password, with nowhere to display the
# request — would otherwise stall the whole suite until CI kills the job, and
# the only thing the report would say is that the job ran out of time. Exits
# 124 on expiry, like `timeout` does, so a caller can tell the two apart.
bounded() {
	local secs="$1" pid ticks=0 rc=0
	shift
	"$@" &
	pid=$!
	while kill -0 "$pid" 2>/dev/null; do
		if [ "$ticks" -ge $((secs * 10)) ]; then
			kill -9 "$pid" 2>/dev/null || true
			wait "$pid" 2>/dev/null || true
			trace "bounded: '$*' did not finish within ${secs}s"
			echo "bounded: '$*' did not finish within ${secs}s" >&2
			return 124
		fi
		sleep 0.1
		ticks=$((ticks + 1))
	done
	wait "$pid" || rc=$?
	return "$rc"
}

# trace_environment records what this machine actually provides. It writes to
# the trace rather than to stderr because bats keeps a passing setup's output
# to itself, and the case worth diagnosing is a run that gets killed rather
# than one that fails. Called once $HOME has been redirected, since on darwin
# that is exactly what decides which keychain `security` resolves.
trace_environment() {
	local tool
	for tool in timeout setsid perl security ssh-add ssh-keygen; do
		trace "tool $tool: $(command -v "$tool" || echo MISSING)"
	done
	case "$OSTYPE" in
	darwin*) ;;
	*) return 0 ;;
	esac
	trace "HOME=$HOME"
	trace "default-keychain: $(bounded 10 security default-keychain 2>&1 | tr -d '\n')"
	trace "list-keychains: $(bounded 10 security list-keychains 2>&1 | tr '\n' ' ')"
}

# no_tty_bounded runs a command with no controlling terminal and a hard time
# limit, so a program that would rather ask a person cannot rescue itself by
# prompting, and cannot stall the suite either.
#
# setsid is util-linux and does not exist on macOS, where there is then no way
# to drop a controlling terminal — only a way to establish that there is none
# to drop, which opening /dev/tty answers exactly. A run under CI has none, so
# this costs nothing there; a developer running the suite from a terminal is
# told why it stopped rather than quietly getting a weaker test than the name
# promises.
no_tty_bounded() {
	local secs="$1"
	shift
	if command -v setsid >/dev/null 2>&1; then
		bounded "$secs" setsid "$@"
		return
	fi
	if (: </dev/tty) 2>/dev/null; then
		skip "this shell has a controlling terminal and no setsid to drop it, so the command under test could fall back to prompting on it"
	fi
	bounded "$secs" "$@"
}

# setup_test_keychain gives the running test a keychain of its own, created
# inside its tmpdir and registered through the redirected $HOME.
#
# It is what makes the darwin secret store reachable at all here. Both
# `security` and sshakku resolve the user's default keychain from preferences
# under $HOME, and a throwaway $HOME holds none: the search list it yields
# contains only /Library/Keychains/System.keychain, which is not writable, and
# asking for the default reports that there isn't one. Registering a keychain
# here fixes that for every process the test starts, sshakku included, because
# they all inherit the same $HOME.
#
# Being inside the tmpdir, it also cannot leak: what one test stores is
# unreachable from the next, and nothing survives the run.
setup_test_keychain() {
	local keychain="$1"
	# Guards nothing real — it exists so `security` can create and unlock the
	# file without asking anyone.
	local password="sshakku-bats"
	mkdir -p "$HOME/Library/Preferences"
	bounded 20 security create-keychain -p "$password" "$keychain"
	# No idle or sleep timeout: nothing may re-lock it mid-test and turn a
	# lookup into a password request with nowhere to display it.
	bounded 20 security set-keychain-settings "$keychain"
	bounded 20 security unlock-keychain -p "$password" "$keychain"
	bounded 20 security list-keychains -d user -s "$keychain"
	bounded 20 security default-keychain -d user -s "$keychain"
}

setup() {
	trace "setup start: ${BATS_TEST_DESCRIPTION:-?}"

	# Everything this test owns lives under one root. On darwin that root is a
	# short path of its own rather than the per-test bats tmpdir: bats nests
	# suite, file and test under /var/folders/<random>/T/, and a unix socket
	# bound below that overruns the 104-byte sun_path limit BSD imposes. It is a
	# limit on the address, not on the file system, so it surfaces as bind()
	# failing with "invalid argument" and nothing pointing at the length.
	case "$OSTYPE" in
	darwin*) TEST_ROOT=$(mktemp -d /tmp/sshakku-bats.XXXXXXXX) ;;
	*) TEST_ROOT="$BATS_TEST_TMPDIR" ;;
	esac

	local prefix="$TEST_ROOT/prefix"
	local home="$TEST_ROOT/home"
	mkdir -p "$prefix" "$home"

	# Every system path `make install` is able to write to is redirected under
	# $prefix, not only the ones today's tests happen to reach: which file the
	# login hook is wired into differs per platform (a /etc/profile.d drop-in
	# on Linux, the /etc/zprofile marker block on darwin), and the opt-in
	# non-login wiring targets a third set again. A knob left at its default
	# here writes to the real system, so they are all named explicitly rather
	# than relying on no test ever enabling one.
	make -C "$REPO_ROOT" install \
		GO_BIN="$BATS_FILE_TMPDIR/build/sshakku" \
		PREFIX="$prefix" \
		ETC_PROFILE_D="$prefix/profile.d/" \
		BASH_BASHRC_D="$prefix/bashrc.d/" \
		BASH_BASHRC_FILE="$prefix/bash.bashrc" \
		ETC_ZPROFILE="$prefix/zprofile" \
		ETC_ZSHRC="$prefix/zshrc" >&2

	# sshakku's currentUser() prefers $USER; force it set so it and this
	# file's own use of $USER (e.g. seed_vault's filename) always agree,
	# regardless of whether the invoking environment happens to set it.
	export USER="${USER:-$(id -un)}"

	export HOME="$home"
	export XDG_CONFIG_HOME="$home/.config"
	export XDG_STATE_HOME="$home/.local/state"
	export XDG_RUNTIME_DIR="$TEST_ROOT/runtime"
	unset XDG_CACHE_HOME
	mkdir -p "$XDG_RUNTIME_DIR"
	chmod 700 "$XDG_RUNTIME_DIR"

	export SSHAKKU_TEST_VAULT="$TEST_ROOT/vault"
	mkdir -p "$SSHAKKU_TEST_VAULT"

	# This suite exercises the headless path deliberately: whatever graphical
	# session the machine actually running bats has (a real desktop on a
	# developer's own box, none at all in the container test suite) must not
	# leak in and make sshakku try kdialog — that would pop a real dialog
	# and block on human input instead of running unattended. BASH_ENV is
	# cleared too: every bash invocation below is non-interactive, non-login
	# on purpose, specifically so it never sources any rc file — a real
	# system-wide sshakku install's own login hook must never run here.
	unset DISPLAY WAYLAND_DISPLAY BASH_ENV

	export PATH="$prefix/bin:$BATS_TEST_DIRNAME/fixtures:$PATH"
	export SSHAKKU_BIN="$prefix/bin/sshakku"

	# Where a login shell ends up sourcing the hook from. On Linux that is the
	# drop-in itself; on darwin the install renders the hook under SHARE_DIR
	# and only sources it from the zprofile marker block, so the rendered file
	# is what a test sources to stand in for a login shell. The two also carry
	# different names.
	case "$OSTYPE" in
	darwin*) export SSHAKKU_HOOK="$prefix/share/sshakku/001-sshakku-init.sh" ;;
	*) export SSHAKKU_HOOK="$prefix/profile.d/001-ssh-init.sh" ;;
	esac

	case "$OSTYPE" in
	darwin*)
		TEST_KEYCHAIN="$TEST_ROOT/sshakku-test.keychain-db"
		setup_test_keychain "$TEST_KEYCHAIN"
		;;
	esac

	trace_environment
	trace "setup done"
}

# teardown kills every ssh-agent this test started (sshakku's own, and any
# started directly by a test), identified by $TEST_ROOT appearing in
# its command line — every socket path this suite ever uses lives under
# there. sshakku deliberately keeps the agent running for the whole login
# session, so nothing here would stop it on its own; left alive past the
# test it would keep holding bats'/docker's own stdout open, hanging the
# whole run even after every test has already reported its result.
# The process list is read per platform: /proc on Linux, `ps` on darwin,
# which has no /proc at all. -ww keeps `ps` from truncating the command line
# to the terminal width, which would hide the very path being matched on.
teardown() {
	trace "teardown start"
	local pid cmdline
	case "$OSTYPE" in
	darwin*)
		while read -r pid cmdline; do
			case "$cmdline" in
			*"$TEST_ROOT"*) kill -9 "$pid" 2>/dev/null || true ;;
			esac
		done < <(ps -e -ww -o pid=,command=)
		;;
	*)
		for pid in /proc/[0-9]*; do
			[ -r "$pid/cmdline" ] || continue
			cmdline=$(tr '\0' ' ' <"$pid/cmdline" 2>/dev/null) || continue
			case "$cmdline" in
			*"$TEST_ROOT"*) kill -9 "${pid#/proc/}" 2>/dev/null || true ;;
			esac
		done
		;;
	esac
	# Only where setup() made a root of its own; elsewhere it is bats' own
	# per-test tmpdir, which bats removes itself.
	case "$OSTYPE" in
	darwin*) rm -rf "$TEST_ROOT" ;;
	esac
	trace "teardown done"
}

# require_keyring skips the calling test when the kernel user keyring (@u)
# isn't usable — the same probe internal/keyring.Available() does, so any
# test that expects a real AddWithAskpass round trip (a vault-stored
# passphrase actually reaching ssh-add) skips consistently with sshakku's
# own real-agent integration tests, instead of failing on an environment
# limitation (e.g. no PAM login session — common in sandboxed/nested
# containers) rather than a real bug.
#
# On darwin there is nothing to require: the out-of-band handoff there is a
# unix socket, not the kernel keyring, so every test this gates can run. It
# returns rather than skips deliberately — a suite that skipped its way to
# green on a platform would say nothing about that platform, which is the
# state this whole job exists to end.
require_keyring() {
	case "$OSTYPE" in
	darwin*) return 0 ;;
	esac
	local id
	id=$(keyctl add user sshakku-bats-probe probe @u 2>/dev/null) || {
		skip "kernel user keyring unavailable in this environment (no PAM login session — common in sandboxed/nested containers)"
	}
	keyctl pipe "$id" >/dev/null 2>&1
	keyctl unlink "$id" @u >/dev/null 2>&1 || true
}

# seed_vault stores passphrase for keyname's default service, as if a prior
# session had already typed it once, bypassing any prompt. It writes whatever
# store sshakku itself will read on this platform: the stub secret-tool's
# on-disk format on Linux, and on darwin the OS keychain, which sshakku
# reaches in-process through Security.framework — no stand-in placed on PATH
# would ever be consulted there.
#
# The darwin write names the test's own keychain (setup_test_keychain), which
# is also the default sshakku's lookup resolves, since both run under the same
# redirected $HOME.
seed_vault() {
	local keyname="$1" passphrase="$2"
	case "$OSTYPE" in
	darwin*)
		trace "seed_vault $keyname start"
		# -A: the item is written here by `security` and read back by sshakku,
		# a different binary, and an item whose ACL names only its writer makes
		# the reader ask the user for permission.
		bounded 20 security add-generic-password -U -A \
			-s "SSH-Key-${keyname}" -a "$USER" -w "$passphrase" \
			"$TEST_KEYCHAIN"
		trace "seed_vault $keyname done"
		;;
	*)
		printf '%s' "$passphrase" >"$SSHAKKU_TEST_VAULT/SSH-Key-${keyname}-${USER}"
		;;
	esac
}

# seed_vault_needing_approval stores passphrase the way an item ends up when
# nothing has said which programs may read it: on darwin, `security
# add-generic-password` without -A leaves the item's access list naming only
# the program that wrote it, so any later access by another one has to be
# authorised by a person.
#
# Where that authorisation can be asked for, the access waits for it. Where it
# cannot — a machine with no interactive GUI session for the dialog to appear
# in, which is every hosted runner — the access fails instead. So this seeds an
# entry the caller is not entitled to, not an entry the caller will wait on.
#
# It is the counterpart of seed_vault, which passes -A precisely to avoid this:
# a fixture that has to be readable is seeded that way on purpose. A test that
# wants the waiting to happen asks for it explicitly, here.
seed_vault_needing_approval() {
	local keyname="$1" passphrase="$2"
	case "$OSTYPE" in
	darwin*)
		bounded 20 security add-generic-password -U \
			-s "SSH-Key-${keyname}" -a "$USER" -w "$passphrase" \
			"$TEST_KEYCHAIN"
		;;
	*)
		skip "the store this suite uses off darwin is a file the fixture writes, which has no authorisation step for an access to wait on"
		;;
	esac
}

# refute_vault_entry fails unless keyname's entry is absent from the store
# sshakku actually writes to on this platform.
#
# The darwin check reads attributes only, never the secret: `find-generic-password`
# without -w answers whether the item exists without opening it, so this can tell
# a deleted entry from a surviving one even when reading the surviving one would
# need someone's permission.
refute_vault_entry() {
	local keyname="$1"
	case "$OSTYPE" in
	darwin*)
		if bounded 20 security find-generic-password \
			-s "SSH-Key-${keyname}" -a "$USER" "$TEST_KEYCHAIN" >/dev/null 2>&1; then
			echo "the keychain still holds SSH-Key-${keyname}" >&2
			return 1
		fi
		;;
	*)
		if [ -e "$SSHAKKU_TEST_VAULT/SSH-Key-${keyname}-${USER}" ]; then
			echo "the vault still holds SSH-Key-${keyname}" >&2
			return 1
		fi
		;;
	esac
}

# new_test_key generates a throwaway ed25519 key named keyname under
# $HOME/.ssh, encrypted with passphrase.
new_test_key() {
	local keyname="$1" passphrase="$2"
	mkdir -p "$HOME/.ssh"
	ssh-keygen -t ed25519 -N "$passphrase" -f "$HOME/.ssh/$keyname" -q
}

# key_fingerprint prints keyname's SHA256 fingerprint, which is the only part
# of `ssh-add -l`'s output that says which file a loaded key came from: that
# listing identifies a key by its comment (user@host), the same for every key
# generated on one machine, and never names the file at all. Matching on the
# key's own name there can therefore neither succeed nor ever fail.
key_fingerprint() {
	ssh-keygen -lf "$HOME/.ssh/$1.pub" | awk '{print $2}'
}

# doctor_recorded_pid prints the pid `sshakku doctor` reports as the one it
# started the agent under (agent.state), via sshakku's own diagnostics
# rather than an external process-inspection tool.
doctor_recorded_pid() {
	"$SSHAKKU_BIN" doctor 2>/dev/null | sed -n 's/^recorded pid:  *\([0-9]*\).*/\1/p'
}

# socket_inode prints the inode number backing path, so a test can tell a
# socket was kept (same inode) from torn down and recreated (a new one) even
# when both happen to bind the same path.
socket_inode() {
	case "$OSTYPE" in
	darwin*) stat -f %i "$1" ;;
	*) stat -c %i "$1" ;;
	esac
}
