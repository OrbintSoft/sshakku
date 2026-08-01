#!/bin/bash
# Runs as the disposable test account (see keepassxc-entrypoint.sh): brings up a
# headless X server and a real KeePassXC with its local browser protocol
# listening and its database unlocked, then runs the given command against it.
#
# Unlike the Secret Service route (keepassxc-session.sh), nothing here needs the
# GUI's "create database" wizard: keepassxc-cli makes the database
# non-interactively and KeePassXC opens it straight on the unlock screen. Only
# two things still need a person, and both are answered by keystroke rather than
# by clicking a coordinate — there is no window manager here, so typing reaches
# whatever holds focus:
#
#   the unlock screen, once at startup
#   "New key association request", the one dialog the protocol raises, which
#   appears the first time a client asks to be remembered
#
# The access-confirmation dialog is answered by configuration instead; see
# keepassxc-browser.ini.
set -euo pipefail

readonly DISPLAY_NUM=":99"
readonly DB_PASSWORD="sshakku-keepassxc-native-test-password"
readonly ASSOCIATION_NAME="sshakku"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly DB_FILE="${HOME}/Passwords.kdbx"

# The test refuses to run without this: a running KeePassXC is somebody's real
# wallet, and the round trip writes to it. Here it is this container's own.
export SSHAKKU_TEST_ALLOW_REAL_KEEPASSXC_NATIVE=1

wait_for() {
	local description="$1" tries=100
	shift
	until "$@" >/dev/null 2>&1; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "keepassxc-native-session: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.3
	done
}

Xvfb "${DISPLAY_NUM}" -screen 0 1280x1024x24 &
export DISPLAY="${DISPLAY_NUM}"
wait_for "the X server" test -S "/tmp/.X11-unix/X${DISPLAY_NUM#:}"

mkdir -p "${HOME}/.config/keepassxc"
cp "${SCRIPT_DIR}/keepassxc-browser.ini" "${HOME}/.config/keepassxc/keepassxc.ini"

printf '%s\n%s\n' "${DB_PASSWORD}" "${DB_PASSWORD}" |
	keepassxc-cli db-create -p "${DB_FILE}" >/dev/null

keepassxc "${DB_FILE}" >/tmp/keepassxc.log 2>&1 &
wait_for "the KeePassXC window" xdotool search --name KeePassXC
# The unlock screen puts the cursor in the password field on its own, so the
# password can be typed without aiming at anything.
sleep 2
xdotool type --delay 30 "${DB_PASSWORD}"
xdotool key Return

# The socket is not the signal: KeePassXC listens on it from startup, database
# open or not, and answers "database not opened" until one is. The window title
# is the signal — it carries "[Locked]" until the unlock takes, and nothing else
# here distinguishes the two.
wait_for "the database to be unlocked" \
	xdotool search --name '^Passwords\.kdbx - KeePassXC$'
wait_for "the browser protocol socket" \
	test -S "${XDG_RUNTIME_DIR}/app/org.keepassxc.KeePassXC/org.keepassxc.KeePassXC.BrowserServer"

# Answers the association request for as long as the test command runs: a client
# asks to be remembered the first time it stores anything, and the request waits
# on a person until it is answered.
(
	while true; do
		if xdotool search --name 'New key association request' >/dev/null 2>&1; then
			xdotool type --delay 30 "${ASSOCIATION_NAME}"
			sleep 0.5
			xdotool key Return
			sleep 1
		fi
		sleep 0.5
	done
) &
disown

cd /src
exec "$@"
