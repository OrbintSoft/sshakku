#!/bin/bash
# Runs as the disposable test account (see gnome-keyring-entrypoint.sh): starts a
# private D-Bus session bus and gnome-keyring-daemon with no screen of any kind,
# the way the daemon runs on a machine its user reaches over ssh, and runs the
# given command against that wallet.
#
# There is no collection-creation dialog answered here, and that is what this
# session is for rather than something it forgot: the login keyring is unlocked
# with a password read from standard input, and whatever else the run needs has
# to be reachable without anything being drawn.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# The account's own password: it unlocks a keyring that is thrown away with the
# container it was made in.
readonly LOGIN_PASSWORD="sshakku-gnome-headless-test-password"

# Whether the daemon has a screen is read from the daemon itself once it is up,
# not from what this script exported: a variable installed for the machine as a
# whole reaches it without passing through here, and a session that quietly had a
# display would be one of the other two sessions run a second time.
assert_daemon_has_no_display() {
	local daemon_path pid="" dir
	daemon_path="$(command -v gnome-keyring-daemon)"

	for dir in /proc/[0-9]*; do
		if [ "$(readlink "${dir}/exe" 2>/dev/null)" = "${daemon_path}" ]; then
			pid="${dir##*/}"
			break
		fi
	done

	if [ -z "${pid}" ]; then
		echo "gnome-keyring-headless-session: the daemon owns the bus name but no process runs ${daemon_path}" >&2
		exit 1
	fi

	if tr '\0' '\n' <"/proc/${pid}/environ" | grep -qE '^(DISPLAY|WAYLAND_DISPLAY)='; then
		echo "gnome-keyring-headless-session: the daemon has a display in its environment" >&2
		exit 1
	fi
}

# shellcheck source=test/containers/gnome-keyring-start.sh
source "${SCRIPT_DIR}/gnome-keyring-start.sh"
start_gnome_keyring_unlocked "${LOGIN_PASSWORD}"
assert_daemon_has_no_display

cd /src
exec "$@"
