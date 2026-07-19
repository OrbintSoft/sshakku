#!/bin/bash
# Runs as the disposable test account (see gnome-keyring-entrypoint.sh):
# starts a private D-Bus session bus, a headless X server, and
# gnome-keyring-daemon, drives the one-time "create the sshakku collection
# with a blank password" dialog via xdotool (gnome-keyring has no
# non-interactive equivalent of KDE's kwalletrc pre-seed), then runs the
# given command against the now prompt-free collection.
set -euo pipefail

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

secrets_registered() {
	dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus \
		org.freedesktop.DBus.ListNames | grep -q org.freedesktop.secrets
}

Xvfb "${DISPLAY_NUM}" -screen 0 1280x1024x24 &
export DISPLAY="${DISPLAY_NUM}"
wait_for "the X server" test -S "/tmp/.X11-unix/X${DISPLAY_NUM#:}"

# DISPLAY must already be in dbus-daemon's own environment before it starts:
# bus-activated services (gcr-prompter, for the one-time collection-creation
# dialog) inherit the environment of the dbus-daemon process that activates
# them, not the shell that happened to launch it.
dbus-daemon --session --fork --address="${DBUS_SESSION_BUS_ADDRESS}"
wait_for "the D-Bus session bus socket" test -S "${DBUS_SESSION_BUS_ADDRESS#unix:path=}"

eval "$(gnome-keyring-daemon --start --components=secrets)"
export GNOME_KEYRING_CONTROL
wait_for "gnome-keyring to register org.freedesktop.secrets" secrets_registered

cd /src
"$(dirname "${BASH_SOURCE[0]}")/gnome-keyring-create-collection.sh"

exec "$@"
