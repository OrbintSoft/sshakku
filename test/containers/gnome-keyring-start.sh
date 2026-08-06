#!/bin/bash
# Sourced by a session script, not run on its own: starts the D-Bus session bus
# and gnome-keyring-daemon, and returns once the daemon owns
# org.freedesktop.secrets. What kind of screen the session has is the caller's
# business — this is only the wallet.
#
# A caller that has a display must bring it up before calling this: bus-activated
# services (gcr-prompter, for the one-time collection-creation dialog) inherit
# the environment of the dbus-daemon process that activates them, not the shell
# that happened to launch it, so a display exported afterwards would never reach
# the dialog.
# shellcheck shell=bash

keyring_wait_for() {
	local description="$1" tries=50
	shift
	until "$@"; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "gnome-keyring-start: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.2
	done
}

keyring_secrets_registered() {
	dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus \
		org.freedesktop.DBus.ListNames | grep -q org.freedesktop.secrets
}

keyring_start_session_bus() {
	dbus-daemon --session --fork --address="${DBUS_SESSION_BUS_ADDRESS}"
	keyring_wait_for "the D-Bus session bus socket" test -S "${DBUS_SESSION_BUS_ADDRESS#unix:path=}"
}

start_gnome_keyring() {
	keyring_start_session_bus

	eval "$(gnome-keyring-daemon --start --components=secrets)"
	export GNOME_KEYRING_CONTROL
	keyring_wait_for "gnome-keyring to register org.freedesktop.secrets" keyring_secrets_registered
}

# Starts the same daemon the way a machine with no screen has to: --unlock reads
# the login keyring's password from standard input, creating that keyring with it
# when it is not there yet. It is the one unlock gnome-keyring offers with
# nothing to draw a dialog on, and it applies to the login keyring alone — any
# other collection is created through a dialog.
start_gnome_keyring_unlocked() {
	local password="$1"

	keyring_start_session_bus

	eval "$(printf '%s' "${password}" | gnome-keyring-daemon --daemonize --unlock --components=secrets)"
	export GNOME_KEYRING_CONTROL
	keyring_wait_for "gnome-keyring to register org.freedesktop.secrets" keyring_secrets_registered
}
