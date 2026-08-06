#!/bin/bash
# Container entrypoint, run as root: creates the disposable test account and
# its runtime dir, then hands off to a session script (as that account) to
# actually drive the test command.
#
# Which session script decides what the wallet is reached from: an X server by
# default, or a Wayland compositor or no screen at all when
# SSHAKKU_SESSION_SCRIPT names one of those instead. The two that have a screen
# answer the collection-creation dialog the way that screen allows; the one
# without has nothing to answer it with.
set -euo pipefail

readonly TEST_USER="sshakku-gnome-test"
readonly TEST_UID="1000"
readonly RUNTIME_DIR="/run/user/${TEST_UID}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly SESSION_SCRIPT="${SSHAKKU_SESSION_SCRIPT:-gnome-keyring-session.sh}"

# The D-Bus session bus refuses to start without a valid, non-empty machine
# ID.
dbus-uuidgen >/etc/machine-id
mkdir -p /var/lib/dbus
ln -sf /etc/machine-id /var/lib/dbus/machine-id

# X11's socket directory convention: normally created at boot by the system
# itself (mode 1777, like /tmp), which a container skips — Xvfb running as
# the unprivileged test account below cannot create it itself.
mkdir -p /tmp/.X11-unix
chmod 1777 /tmp/.X11-unix

# A mid-session-failure test kills the daemon to watch the backend cope; the
# bus would otherwise respawn it from this activation file on the next call,
# so the test opts into removing it. The prompter services stay so the
# one-time collection-creation dialog still works.
if [ -n "${SSHAKKU_DISABLE_SECRETS_ACTIVATION:-}" ]; then
	rm -f /usr/share/dbus-1/services/org.freedesktop.secrets.service
fi

useradd -m -u "${TEST_UID}" -s /bin/bash "${TEST_USER}"

# A session whose dialogs are answered by clicking needs a pointer, and making
# one is root's work: the device node, and the seat daemon the compositor opens
# it through. Only the run that asked for it gets one.
if [ -n "${SSHAKKU_TEST_UINPUT_POINTER:-}" ]; then
	# shellcheck source=test/containers/wayland-pointer.sh
	source "${SCRIPT_DIR}/wayland-pointer.sh"
	start_uinput_pointer "${TEST_USER}"
fi

mkdir -p "${RUNTIME_DIR}"
chown "${TEST_USER}:${TEST_USER}" "${RUNTIME_DIR}"
chmod 700 "${RUNTIME_DIR}"

exec runuser -u "${TEST_USER}" -- env -i \
	HOME="/home/${TEST_USER}" \
	PATH="/usr/local/go/bin:${PATH}" \
	XDG_RUNTIME_DIR="${RUNTIME_DIR}" \
	DBUS_SESSION_BUS_ADDRESS="unix:path=${RUNTIME_DIR}/bus" \
	SSHAKKU_TEST_ALLOW_REAL_SECRETSERVICE="1" \
	SSHAKKU_DISABLE_SECRETS_ACTIVATION="${SSHAKKU_DISABLE_SECRETS_ACTIVATION:-}" \
	"${SCRIPT_DIR}/${SESSION_SCRIPT}" "$@"
