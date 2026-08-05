#!/bin/bash
# Runs as the disposable test account (see keepassxc-entrypoint.sh): starts a
# sway compositor on wlroots' headless backend and a private D-Bus session bus,
# enables KeePassXC's Secret Service integration, opens the database through the
# app's own wizard, then runs the given command against it.
#
# What differs from the X11 session is only where the app is drawn and how its
# dialogs are answered: the database, the collection and the tests are the same.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# The opt-in the Secret Service round trip refuses to run without. It belongs
# here rather than in the entrypoint: this script is what makes it true, by
# standing up the bus and the collection the tests then use.
export SSHAKKU_TEST_ALLOW_REAL_SECRETSERVICE=1

# The pointer the entrypoint arranged is reached through the seat daemon, and
# the libinput backend is what makes the compositor look for it at all.
export WLR_BACKENDS=libinput,headless
export LIBSEAT_BACKEND=seatd

# A KeePassXC window that fills the screen would put every button somewhere
# that depends on the screen; floating keeps the size the app itself chose, so
# what a test clicks is decided by the app and not by the compositor's output.
export SWAY_CONFIG="${SCRIPT_DIR}/keepassxc-sway.config"

# What a Wayland login declares about itself, and what this app reads to decide
# where to draw: without it Qt takes the X11 platform, finds no display, and
# quits before showing anything.
export XDG_SESSION_TYPE=wayland

wait_for() {
	local description="$1" tries=50
	shift
	until "$@"; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "keepassxc-wayland-session: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.2
	done
}

# shellcheck source=test/containers/wayland-compositor.sh
source "${SCRIPT_DIR}/wayland-compositor.sh"
start_wayland_compositor

dbus-daemon --session --fork --address="${DBUS_SESSION_BUS_ADDRESS}"
wait_for "the D-Bus session bus socket" test -S "${DBUS_SESSION_BUS_ADDRESS#unix:path=}"

# The "enable Secret Service integration" toggle is a plain app-config boolean,
# unlike the collection creation itself.
mkdir -p "${HOME}/.config/keepassxc"
printf '[FdoSecrets]\nEnabled=true\n' >"${HOME}/.config/keepassxc/keepassxc.ini"

cd /src
"${SCRIPT_DIR}/keepassxc-wayland-create-collection.sh"

exec "$@"
