#!/bin/bash
# Container entrypoint, run as root: creates the disposable test account and
# PAM service ksecretd's non-interactive unlock needs, then hands off to
# kde-tier2-session.sh (as that account) to actually drive the test command.
set -euo pipefail

readonly TEST_USER="sshakku-kde-test"
readonly TEST_UID="1000"
readonly TEST_PASSWORD="sshakku-kde-tier2-test-password"
readonly PAM_SERVICE="sshakku-kde-tier2"
readonly RUNTIME_DIR="/run/user/${TEST_UID}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# ksecretd (a Qt app) and the D-Bus session bus both refuse to start without
# a valid, non-empty machine ID.
dbus-uuidgen >/etc/machine-id
mkdir -p /var/lib/dbus
ln -sf /etc/machine-id /var/lib/dbus/machine-id

useradd -m -u "${TEST_UID}" -s /bin/bash "${TEST_USER}"
echo "${TEST_USER}:${TEST_PASSWORD}" | chpasswd

mkdir -p "${RUNTIME_DIR}"
chown "${TEST_USER}:${TEST_USER}" "${RUNTIME_DIR}"
chmod 700 "${RUNTIME_DIR}"

install -m 644 "${SCRIPT_DIR}/kde-tier2.env" /etc/environment
install -m 644 "${SCRIPT_DIR}/kde-tier2-pam.conf" "/etc/pam.d/${PAM_SERVICE}"

# Makes "sshakku" (not "kdewallet") the wallet PAM's hash-based unlock opens,
# and pre-registers its Secret Service alias — this, not an interactive
# create dialog, is what makes the real round-trip test deterministic.
install -m 644 -o "${TEST_USER}" -g "${TEST_USER}" -D \
	"${SCRIPT_DIR}/kde-tier2-kwalletrc" "/home/${TEST_USER}/.config/kwalletrc"

exec runuser -u "${TEST_USER}" -- env -i \
	HOME="/home/${TEST_USER}" \
	PATH="/usr/local/go/bin:${PATH}" \
	XDG_RUNTIME_DIR="${RUNTIME_DIR}" \
	DBUS_SESSION_BUS_ADDRESS="unix:path=${RUNTIME_DIR}/bus" \
	QT_QPA_PLATFORM="offscreen" \
	SSHAKKU_TEST_ALLOW_REAL_SECRETSERVICE="1" \
	SSHAKKU_TEST_KDE_PASSWORD="${TEST_PASSWORD}" \
	SSHAKKU_TEST_KDE_PAM_SERVICE="${PAM_SERVICE}" \
	"${SCRIPT_DIR}/kde-tier2-session.sh" "$@"
