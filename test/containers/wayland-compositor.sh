#!/bin/bash
# Sourced by a session script, not run on its own: brings up a sway compositor
# on wlroots' headless backend and exports the two variables a client needs to
# find it. Any image that wants a screen with no X server behind it needs this
# and differs only in what it starts inside the session.
#
# Both sockets are found rather than assumed: their names carry a pid, or the
# first free display number, neither of which this script picks.
# shellcheck shell=bash

readonly SWAY_CONFIG="${SWAY_CONFIG:-$(dirname "${BASH_SOURCE[0]}")/wayland-sway.config}"
readonly DISPLAY_SOCKET="${XDG_RUNTIME_DIR}/wayland-[0-9]"
readonly IPC_SOCKET="${XDG_RUNTIME_DIR}/sway-ipc.*.sock"

compositor_wait_for() {
	local description="$1" tries=50
	shift
	until "$@"; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "wayland-compositor: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.2
	done
}

# Whether the given glob names exactly one path. Two would mean a session left
# over from somewhere else, which is worth failing on rather than picking one.
compositor_exactly_one() {
	[ "$(compgen -G "$1" | wc -l)" -eq 1 ]
}

# Starts the compositor and exports WAYLAND_DISPLAY and SWAYSOCK. What a client
# then sees is a Wayland login without Xwayland: a screen, and no DISPLAY.
start_wayland_compositor() {
	# WLR_BACKENDS=headless is what makes this a session with no hardware behind
	# it: wlroots renders to memory instead of opening a DRM device, so no seat,
	# no /dev/dri and no privileges are needed. Without it sway would look for a
	# graphics card, find none, and exit.
	WLR_BACKENDS=headless sway --config "${SWAY_CONFIG}" &

	compositor_wait_for "the Wayland display socket" compositor_exactly_one "${DISPLAY_SOCKET}"
	WAYLAND_DISPLAY="$(basename "$(compgen -G "${DISPLAY_SOCKET}")")"
	export WAYLAND_DISPLAY

	compositor_wait_for "sway's IPC socket" compositor_exactly_one "${IPC_SOCKET}"
	SWAYSOCK="$(compgen -G "${IPC_SOCKET}")"
	export SWAYSOCK
}
