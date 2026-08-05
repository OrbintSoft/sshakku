#!/bin/bash
# Runs a wallet image's Wayland session, which needs more from the host than
# the other sessions do: a pointer device, and therefore the uinput module.
#
# Usage: run-wayland-wallet.sh <image> <session-script> <command...>
#
# What the container is granted, and what it deliberately is not: /dev/uinput to
# make its own device, the input major so it can open the node it made, and
# CAP_MKNOD to make it — but no privileged mode and no /dev/input mount, so the
# host's own keyboards and mice have no node inside the container and cannot be
# opened even though the udev database lists them. The device the container does
# make never sends an event; every click goes through the compositor.
set -euo pipefail

if [ "$#" -lt 3 ]; then
	echo "usage: $0 <image> <session-script> <command...>" >&2
	exit 2
fi

readonly IMAGE="$1" SESSION_SCRIPT="$2"
shift 2

# The device node cannot be made if the kernel cannot make devices: on a
# machine that has never used one, the module is not loaded.
if [ ! -e /dev/uinput ]; then
	sudo modprobe uinput
fi

exec docker run --init --rm \
	--device /dev/uinput \
	--device-cgroup-rule 'c 13:* rmw' \
	--cap-add MKNOD \
	-v /run/udev:/run/udev:ro \
	-v "${PWD}":/src:ro \
	-e SSHAKKU_TEST_UINPUT_POINTER=1 \
	-e "SSHAKKU_SESSION_SCRIPT=${SESSION_SCRIPT}" \
	"${IMAGE}" "$@"
