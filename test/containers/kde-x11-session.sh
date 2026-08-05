#!/bin/bash
# Runs as the disposable test account (see kde-entrypoint.sh): puts an X11
# session around the ordinary KDE one, so ksecretd is reached from a login with
# an X server rather than from a Wayland login or from one with no display at
# all.
#
# The unlock itself is kde-session.sh's, unchanged: what is being varied here is
# the session, and a second copy of the PAM handshake would be a second thing to
# keep right.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly DISPLAY_NUM=":99"

# Qt is left to work out for itself what it is running under, exactly as it does
# on a real desktop: with an X server to talk to it takes the xcb platform, and
# forcing one would be this script asserting the very thing it exists to
# observe.
unset QT_QPA_PLATFORM

Xvfb "${DISPLAY_NUM}" -screen 0 1280x1024x24 &
export DISPLAY="${DISPLAY_NUM}"

tries=50
until [ -S "/tmp/.X11-unix/X${DISPLAY_NUM#:}" ]; do
	tries=$((tries - 1))
	if [ "${tries}" -le 0 ]; then
		echo "kde-x11-session: timed out waiting for the X server" >&2
		exit 1
	fi
	sleep 0.2
done

exec "${SCRIPT_DIR}/kde-session.sh" "$@"
