#!/bin/bash
# Runs as the disposable test account (see wayland-entrypoint.sh): starts a
# private D-Bus session bus and a sway compositor on wlroots' headless
# backend, then runs the given command inside that session.
#
# The command is given WAYLAND_DISPLAY and SWAYSOCK and no DISPLAY, which is
# what a Wayland login without Xwayland looks like from a program's side.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

wait_for() {
	local description="$1" tries=50
	shift
	until "$@"; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "wayland-session: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.2
	done
}

dbus-daemon --session --fork --address="${DBUS_SESSION_BUS_ADDRESS}"
wait_for "the D-Bus session bus socket" test -S "${DBUS_SESSION_BUS_ADDRESS#unix:path=}"

# shellcheck source=test/containers/wayland-compositor.sh
source "${SCRIPT_DIR}/wayland-compositor.sh"
start_wayland_compositor

cd /src
exec "$@"
