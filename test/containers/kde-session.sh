#!/bin/bash
# Runs as the disposable test account (see kde-entrypoint.sh): starts a
# private D-Bus session bus, opens a real PAM session to unlock ksecretd the
# same way a real login does, then forwards this session's environment over
# ksecretd's own handshake socket — the step a desktop's pam_kwallet_init
# autostart entry normally performs — before finally running the given
# command with a genuinely unlocked Secret Service on the bus.
set -euo pipefail

readonly KWALLET_SOCKET="${XDG_RUNTIME_DIR}/kwallet5.socket"

wait_for() {
	local description="$1" tries=50
	shift
	until "$@"; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "kde-session: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.2
	done
}

secrets_registered() {
	dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus \
		org.freedesktop.DBus.ListNames | grep -q org.freedesktop.secrets
}

dbus-daemon --session --fork --address="${DBUS_SESSION_BUS_ADDRESS}"
wait_for "the D-Bus session bus socket" test -S "${DBUS_SESSION_BUS_ADDRESS#unix:path=}"

echo "${SSHAKKU_TEST_KDE_PASSWORD}" |
	pamtester -v "${SSHAKKU_TEST_KDE_PAM_SERVICE}" "$(id -un)" authenticate open_session

wait_for "ksecretd's handshake socket" test -S "${KWALLET_SOCKET}"
env | socat - "UNIX-CONNECT:${KWALLET_SOCKET}"

wait_for "ksecretd to register org.freedesktop.secrets" secrets_registered

cd /src
exec "$@"
