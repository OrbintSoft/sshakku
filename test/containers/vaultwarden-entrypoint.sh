#!/bin/bash
# Container entrypoint, run as root: creates the disposable test account and
# its runtime dir, then hands off to vaultwarden-session.sh (as that
# account) to actually drive the test command.
set -euo pipefail

readonly TEST_USER="sshakku-bitwarden-test"
readonly TEST_UID="1000"
readonly RUNTIME_DIR="/run/user/${TEST_UID}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

useradd -m -u "${TEST_UID}" -s /bin/bash "${TEST_USER}"

mkdir -p "${RUNTIME_DIR}"
chown "${TEST_USER}:${TEST_USER}" "${RUNTIME_DIR}"
chmod 700 "${RUNTIME_DIR}"

exec runuser -u "${TEST_USER}" -- env -i \
	HOME="/home/${TEST_USER}" \
	PATH="/usr/local/go/bin:${PATH}" \
	XDG_RUNTIME_DIR="${RUNTIME_DIR}" \
	"${SCRIPT_DIR}/vaultwarden-session.sh" "$@"
