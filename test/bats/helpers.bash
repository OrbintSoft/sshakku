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
# One thing genuinely cannot be redirected under that tree: on darwin the
# secret store is the OS keychain, which sshakku reaches in-process, so it
# searches whatever the *default* keychain is. Repointing that at a throwaway
# one before running this suite is the caller's job — the macOS CI job does it
# with test/macos-keychain-setup.sh. Everything else here, the install paths
# included, is confined to the per-test tmpdir.

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

setup() {
	local prefix="$BATS_TEST_TMPDIR/prefix"
	local home="$BATS_TEST_TMPDIR/home"
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
	export XDG_RUNTIME_DIR="$BATS_TEST_TMPDIR/runtime"
	unset XDG_CACHE_HOME
	mkdir -p "$XDG_RUNTIME_DIR"
	chmod 700 "$XDG_RUNTIME_DIR"

	export SSHAKKU_TEST_VAULT="$BATS_TEST_TMPDIR/vault"
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

	# Names of the keys a test creates or seeds, so teardown can remove what
	# outlives the per-test tmpdir (see forget_test_secrets).
	TEST_KEYNAMES=()
}

# teardown kills every ssh-agent this test started (sshakku's own, and any
# started directly by a test), identified by $BATS_TEST_TMPDIR appearing in
# its command line — every socket path this suite ever uses lives under
# there. sshakku deliberately keeps the agent running for the whole login
# session, so nothing here would stop it on its own; left alive past the
# test it would keep holding bats'/docker's own stdout open, hanging the
# whole run even after every test has already reported its result.
# The process list is read per platform: /proc on Linux, `ps` on darwin,
# which has no /proc at all. -ww keeps `ps` from truncating the command line
# to the terminal width, which would hide the very path being matched on.
teardown() {
	local pid cmdline
	case "$OSTYPE" in
	darwin*)
		while read -r pid cmdline; do
			case "$cmdline" in
			*"$BATS_TEST_TMPDIR"*) kill -9 "$pid" 2>/dev/null || true ;;
			esac
		done < <(ps -e -ww -o pid=,command=)
		;;
	*)
		for pid in /proc/[0-9]*; do
			[ -r "$pid/cmdline" ] || continue
			cmdline=$(tr '\0' ' ' <"$pid/cmdline" 2>/dev/null) || continue
			case "$cmdline" in
			*"$BATS_TEST_TMPDIR"*) kill -9 "${pid#/proc/}" 2>/dev/null || true ;;
			esac
		done
		;;
	esac
	forget_test_secrets
}

# forget_test_secrets removes stored passphrases that outlive the test. On
# Linux there are none to remove: the vault is a directory under the per-test
# tmpdir and goes with it. The darwin keychain is a real, shared store that
# survives the test, so an item left behind would still be there for the next
# test — including the ones whose whole point is an empty vault, which would
# then pass or fail for a reason that has nothing to do with what they assert.
forget_test_secrets() {
	case "$OSTYPE" in
	darwin*) ;;
	*) return 0 ;;
	esac
	local keyname
	for keyname in ${TEST_KEYNAMES+"${TEST_KEYNAMES[@]}"}; do
		security delete-generic-password -s "SSH-Key-${keyname}" -a "$USER" >/dev/null 2>&1 || true
	done
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
# The darwin write lands in the *default* keychain, the same one sshakku's
# lookup will search. Pointing that at a throwaway keychain is the caller's
# job (test/macos-keychain-setup.sh), because nothing in the search path can
# be overridden per-process.
seed_vault() {
	local keyname="$1" passphrase="$2"
	case "$OSTYPE" in
	darwin*)
		security add-generic-password -U -s "SSH-Key-${keyname}" -a "$USER" -w "$passphrase"
		TEST_KEYNAMES+=("$keyname")
		;;
	*)
		printf '%s' "$passphrase" >"$SSHAKKU_TEST_VAULT/SSH-Key-${keyname}-${USER}"
		;;
	esac
}

# new_test_key generates a throwaway ed25519 key named keyname under
# $HOME/.ssh, encrypted with passphrase.
new_test_key() {
	local keyname="$1" passphrase="$2"
	mkdir -p "$HOME/.ssh"
	ssh-keygen -t ed25519 -N "$passphrase" -f "$HOME/.ssh/$keyname" -q
	# Recorded even when nothing is seeded: a test can drive sshakku into
	# storing this key's passphrase itself, and that entry needs removing too.
	TEST_KEYNAMES+=("$keyname")
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
