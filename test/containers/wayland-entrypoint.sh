#!/bin/bash
# Container entrypoint, run as root: creates the disposable test account and
# its runtime dir, then hands off to wayland-session.sh (as that account) to
# start the session and run the given command in it.
set -euo pipefail

readonly TEST_USER="sshakku-wayland-test"
readonly TEST_UID="1000"
readonly RUNTIME_DIR="/run/user/${TEST_UID}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# The D-Bus session bus refuses to start without a valid, non-empty machine
# ID.
dbus-uuidgen >/etc/machine-id
mkdir -p /var/lib/dbus
ln -sf /etc/machine-id /var/lib/dbus/machine-id

useradd -m -u "${TEST_UID}" -s /bin/bash "${TEST_USER}"

# A Wayland compositor refuses to start without a runtime directory of its
# own, and every socket in the session — the display, the IPC — is created
# inside it.
mkdir -p "${RUNTIME_DIR}"
chown "${TEST_USER}:${TEST_USER}" "${RUNTIME_DIR}"
chmod 700 "${RUNTIME_DIR}"

exec runuser -u "${TEST_USER}" -- env -i \
	HOME="/home/${TEST_USER}" \
	PATH="/usr/local/go/bin:${PATH}" \
	XDG_RUNTIME_DIR="${RUNTIME_DIR}" \
	DBUS_SESSION_BUS_ADDRESS="unix:path=${RUNTIME_DIR}/bus" \
	"${SCRIPT_DIR}/wayland-session.sh" "$@"
