#!/bin/bash
# Runs as the disposable test account (see kde-entrypoint.sh): puts a Wayland
# session around the ordinary KDE one, so ksecretd is reached from a login with
# a screen and no X server rather than from a login with no display at all.
#
# The unlock itself is kde-session.sh's, unchanged: what is being varied here is
# the session, and a second copy of the PAM handshake would be a second thing to
# keep right.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# Qt is left to work out for itself what it is running under, exactly as it does
# on a real desktop: with a compositor to talk to it takes the wayland platform,
# and forcing one would be this script asserting the very thing it exists to
# observe.
unset QT_QPA_PLATFORM

# shellcheck source=test/containers/wayland-compositor.sh
source "${SCRIPT_DIR}/wayland-compositor.sh"
start_wayland_compositor

exec "${SCRIPT_DIR}/kde-session.sh" "$@"
