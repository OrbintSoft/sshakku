#!/bin/bash
# Sourced by a container entrypoint while it is still root, not run on its own:
# gives the session a pointer device, which some dialogs will not take input
# without.
#
# A password prompt grabs the seat before it accepts anything, and a seat with
# no input device of its own has nothing to grant: GNOME Keyring's
# collection-creation dialog draws, keeps focus, and ignores every key sent to
# it — a virtual keyboard included. A pointer device makes the grab possible,
# after which the compositor's own cursor can answer the dialog.
#
# The device never sends an event. Everything a test does goes through the
# compositor (`swaymsg seat - cursor ...`), which is confined to this container;
# a device that emits nothing cannot reach a session outside it. That is also
# why the container needs no /dev/input mount and no privileged mode: it makes
# the one node it uses and can open nothing else.
# shellcheck shell=bash

pointer_wait_for() {
	local description="$1" tries=50
	shift
	until "$@"; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "wayland-pointer: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.2
	done
}

# start_uinput_pointer <group> creates the device, makes its node — a container
# has no udev to make it — and starts the seat daemon the compositor opens it
# through, reachable by the given group.
start_uinput_pointer() {
	local group="$1" log=/tmp/uinput-pointer.log node devnum

	uinput-pointer >"${log}" 2>&1 &
	pointer_wait_for "the pointer device" grep -q "^node:" "${log}"

	node="$(awk '{print $2}' "${log}")"
	devnum="$(awk '{print $4}' "${log}")"
	mkdir -p /dev/input
	mknod "/dev/input/${node}" c "${devnum%%:*}" "${devnum##*:}"
	chgrp "${group}" "/dev/input/${node}"

	# SEATD_VTBOUND=0: seatd otherwise wants a virtual terminal to switch
	# between sessions, and a container has none.
	SEATD_VTBOUND=0 seatd -g "${group}" >/tmp/seatd.log 2>&1 &
	pointer_wait_for "the seat daemon's socket" test -S /run/seatd.sock
}
