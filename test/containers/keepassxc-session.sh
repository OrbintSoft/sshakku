#!/bin/bash
# Runs as the disposable test account (see keepassxc-entrypoint.sh):
# starts a private D-Bus session bus and a headless X server, enables
# KeePassXC's Secret Service integration (the only piece of this that has a
# non-interactive config-file equivalent — see
# keepassxc-create-collection.sh for the part that doesn't), then runs the
# given command
# against it.
set -euo pipefail

readonly DISPLAY_NUM=":99"

wait_for() {
	local description="$1" tries=50
	shift
	until "$@"; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "keepassxc-session: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.2
	done
}

Xvfb "${DISPLAY_NUM}" -screen 0 1280x1024x24 &
export DISPLAY="${DISPLAY_NUM}"
wait_for "the X server" test -S "/tmp/.X11-unix/X${DISPLAY_NUM#:}"

dbus-daemon --session --fork --address="${DBUS_SESSION_BUS_ADDRESS}"
wait_for "the D-Bus session bus socket" test -S "${DBUS_SESSION_BUS_ADDRESS#unix:path=}"

# The "enable Secret Service integration" toggle is a plain app-config
# boolean, unlike the collection creation itself (see
# keepassxc-create-collection.sh).
mkdir -p "${HOME}/.config/keepassxc"
printf '[FdoSecrets]\nEnabled=true\n' >"${HOME}/.config/keepassxc/keepassxc.ini"

cd /src
"$(dirname "${BASH_SOURCE[0]}")/keepassxc-create-collection.sh"

exec "$@"
