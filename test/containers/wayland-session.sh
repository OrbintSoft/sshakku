#!/bin/bash
# Runs as the disposable test account (see wayland-entrypoint.sh): starts a
# private D-Bus session bus and a sway compositor on wlroots' headless
# backend, then runs the given command inside that session.
#
# The command is given WAYLAND_DISPLAY and SWAYSOCK and no DISPLAY, which is
# what a Wayland login without Xwayland looks like from a program's side. Both
# sockets are found rather than assumed: their names carry a pid, or the first
# free display number, neither of which this script picks.
set -euo pipefail

readonly SWAY_CONFIG="/opt/sshakku-wayland/wayland-sway.config"
readonly DISPLAY_SOCKET="${XDG_RUNTIME_DIR}/wayland-[0-9]"
readonly IPC_SOCKET="${XDG_RUNTIME_DIR}/sway-ipc.*.sock"

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

# Whether the given glob names exactly one path. Two would mean a session left
# over from somewhere else, which is worth failing on rather than picking one.
exactly_one() {
	[ "$(compgen -G "$1" | wc -l)" -eq 1 ]
}

dbus-daemon --session --fork --address="${DBUS_SESSION_BUS_ADDRESS}"
wait_for "the D-Bus session bus socket" test -S "${DBUS_SESSION_BUS_ADDRESS#unix:path=}"

# WLR_BACKENDS=headless is what makes this a session with no hardware behind
# it: wlroots renders to memory instead of opening a DRM device, so no seat,
# no /dev/dri and no privileges are needed. Without it sway would look for a
# graphics card, find none, and exit.
WLR_BACKENDS=headless sway --config "${SWAY_CONFIG}" &

wait_for "the Wayland display socket" exactly_one "${DISPLAY_SOCKET}"
WAYLAND_DISPLAY="$(basename "$(compgen -G "${DISPLAY_SOCKET}")")"
export WAYLAND_DISPLAY

wait_for "sway's IPC socket" exactly_one "${IPC_SOCKET}"
SWAYSOCK="$(compgen -G "${IPC_SOCKET}")"
export SWAYSOCK

cd /src
exec "$@"
