#!/bin/bash
# Container entrypoint, run as root: creates the disposable test account and
# its runtime dir, then hands off to gnome-keyring-tier2-session.sh (as that
# account) to actually drive the test command.
set -euo pipefail

readonly TEST_USER="sshakku-gnome-test"
readonly TEST_UID="1000"
readonly RUNTIME_DIR="/run/user/${TEST_UID}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

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

useradd -m -u "${TEST_UID}" -s /bin/bash "${TEST_USER}"

mkdir -p "${RUNTIME_DIR}"
chown "${TEST_USER}:${TEST_USER}" "${RUNTIME_DIR}"
chmod 700 "${RUNTIME_DIR}"

exec runuser -u "${TEST_USER}" -- env -i \
	HOME="/home/${TEST_USER}" \
	PATH="/usr/local/go/bin:${PATH}" \
	XDG_RUNTIME_DIR="${RUNTIME_DIR}" \
	DBUS_SESSION_BUS_ADDRESS="unix:path=${RUNTIME_DIR}/bus" \
	SSHAKKU_TEST_ALLOW_REAL_SECRETSERVICE="1" \
	"${SCRIPT_DIR}/gnome-keyring-tier2-session.sh" "$@"
