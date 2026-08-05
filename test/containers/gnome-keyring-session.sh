#!/bin/bash
# Runs as the disposable test account (see gnome-keyring-entrypoint.sh):
# starts a headless X server, a private D-Bus session bus and
# gnome-keyring-daemon, drives the one-time "create the sshakku collection
# with a blank password" dialog via xdotool (gnome-keyring has no
# non-interactive equivalent of KDE's kwalletrc pre-seed), then runs the
# given command against the now prompt-free collection.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly DISPLAY_NUM=":99"

wait_for() {
	local description="$1" tries=50
	shift
	until "$@"; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "gnome-keyring-session: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.2
	done
}

Xvfb "${DISPLAY_NUM}" -screen 0 1280x1024x24 &
export DISPLAY="${DISPLAY_NUM}"
wait_for "the X server" test -S "/tmp/.X11-unix/X${DISPLAY_NUM#:}"

# shellcheck source=test/containers/gnome-keyring-start.sh
source "${SCRIPT_DIR}/gnome-keyring-start.sh"
start_gnome_keyring

cd /src
"${SCRIPT_DIR}/gnome-keyring-create-collection.sh"

exec "$@"
