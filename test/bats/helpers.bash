# Shared setup for the shell-plumbing bats suite: builds sshakku once per
# file, installs it fresh into an isolated prefix for each test (exercising
# the real `make install` sed-templating, not a hand-edited copy of the
# hook), and points every XDG/HOME path at a throwaway tree so tests never
# touch the real user or system state.
#
# This suite must only ever run in a disposable environment (the tier-1
# container): a real machine can have its own system-wide sshakku install
# and secret store, and every test here manipulates real ssh-agent
# processes and real login-hook plumbing. SSHAKKU_TEST_ALLOW_BATS=1 is the
# same explicit-opt-in pattern internal/keys's real-daemon integration
# tests already use, not a default-on convenience.

REPO_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"

setup_file() {
	if [ "${SSHAKKU_TEST_ALLOW_BATS:-}" != "1" ]; then
		echo "skipping: set SSHAKKU_TEST_ALLOW_BATS=1 to run this suite — only safe in a disposable environment (the tier-1 container), never on a real machine with its own sshakku install" >&2
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

	make -C "$REPO_ROOT" install \
		GO_BIN="$BATS_FILE_TMPDIR/build/sshakku" \
		PREFIX="$prefix" \
		ETC_PROFILE_D="$prefix/profile.d/" >&2

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
	# developer's own box, none at all in the tier-1 container) must not
	# leak in and make sshakku try kdialog — that would pop a real dialog
	# and block on human input instead of running unattended. BASH_ENV is
	# cleared too: every bash invocation below is non-interactive, non-login
	# on purpose, specifically so it never sources any rc file — a real
	# system-wide sshakku install's own login hook must never run here.
	unset DISPLAY WAYLAND_DISPLAY BASH_ENV

	export PATH="$prefix/bin:$BATS_TEST_DIRNAME/fixtures:$PATH"
	export SSHAKKU_HOOK="$prefix/profile.d/001-ssh-init.sh"
	export SSHAKKU_BIN="$prefix/bin/sshakku"
}

# teardown kills every ssh-agent this test started (sshakku's own, and any
# started directly by a test), identified by $BATS_TEST_TMPDIR appearing in
# its command line — every socket path this suite ever uses lives under
# there. sshakku deliberately keeps the agent running for the whole login
# session, so nothing here would stop it on its own; left alive past the
# test it would keep holding bats'/docker's own stdout open, hanging the
# whole run even after every test has already reported its result.
teardown() {
	local pid cmdline
	for pid in /proc/[0-9]*; do
		[ -r "$pid/cmdline" ] || continue
		cmdline=$(tr '\0' ' ' <"$pid/cmdline" 2>/dev/null) || continue
		case "$cmdline" in
		*"$BATS_TEST_TMPDIR"*) kill -9 "${pid#/proc/}" 2>/dev/null || true ;;
		esac
	done
}

# require_keyring skips the calling test when the kernel user keyring (@u)
# isn't usable — the same probe internal/keyring.Available() does, so any
# test that expects a real AddWithAskpass round trip (a vault-stored
# passphrase actually reaching ssh-add) skips consistently with sshakku's
# own real-agent integration tests, instead of failing on an environment
# limitation (e.g. no PAM login session — common in sandboxed/nested
# containers) rather than a real bug.
require_keyring() {
	local id
	id=$(keyctl add user sshakku-bats-probe probe @u 2>/dev/null) || {
		skip "kernel user keyring unavailable in this environment (no PAM login session — common in sandboxed/nested containers)"
	}
	keyctl pipe "$id" >/dev/null 2>&1
	keyctl unlink "$id" @u >/dev/null 2>&1 || true
}

# seed_vault stores passphrase for keyname's default service, as if a prior
# session had already typed it once — the stub secret-tool's on-disk format
# directly, bypassing any prompt.
seed_vault() {
	local keyname="$1" passphrase="$2"
	printf '%s' "$passphrase" >"$SSHAKKU_TEST_VAULT/SSH-Key-${keyname}-${USER}"
}

# new_test_key generates a throwaway ed25519 key named keyname under
# $HOME/.ssh, encrypted with passphrase.
new_test_key() {
	local keyname="$1" passphrase="$2"
	mkdir -p "$HOME/.ssh"
	ssh-keygen -t ed25519 -N "$passphrase" -f "$HOME/.ssh/$keyname" -q
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
	stat -c %i "$1"
}
