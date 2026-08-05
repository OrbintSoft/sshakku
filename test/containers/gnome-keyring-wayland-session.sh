#!/bin/bash
# Runs as the disposable test account (see gnome-keyring-entrypoint.sh): starts
# a sway compositor on wlroots' headless backend, a private D-Bus session bus
# and gnome-keyring-daemon, answers the one-time collection-creation dialog
# from the keyboard, then runs the given command against the now prompt-free
# collection.
#
# What differs from the X11 session is only where the dialog is drawn and how it
# is answered: the wallet, the daemon and the collection are the same, and so is
# the test that follows.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# The pointer the entrypoint arranged is reached through the seat daemon, and
# the libinput backend is what makes the compositor look for it at all.
export WLR_BACKENDS=libinput,headless
export LIBSEAT_BACKEND=seatd

# shellcheck source=test/containers/wayland-compositor.sh
source "${SCRIPT_DIR}/wayland-compositor.sh"
start_wayland_compositor

# shellcheck source=test/containers/gnome-keyring-start.sh
source "${SCRIPT_DIR}/gnome-keyring-start.sh"
start_gnome_keyring

cd /src
"${SCRIPT_DIR}/gnome-keyring-wayland-create-collection.sh"

exec "$@"
