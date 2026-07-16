#!/bin/bash
# Runs as the disposable test account (see vaultwarden-tier2-entrypoint.sh):
# starts a private Vaultwarden instance from the pre-registered test-account
# fixture (see vaultwarden-tier2-fixture/), logs the bw CLI into it, then
# runs the given command against it with a ready BW_SESSION.
set -euo pipefail

readonly VAULTWARDEN_PORT="8443"
readonly VAULTWARDEN_URL="https://localhost:${VAULTWARDEN_PORT}"
# This account's only purpose is to hold this disposable container's empty
# test vault; the password protects nothing of value and is fixed on
# purpose, so every run of this fixture unlocks the same way. Never reuse it
# for anything real.
readonly TEST_EMAIL="sshakku-test@example.invalid"
readonly TEST_MASTER_PASSWORD="sshakku-tier2-fixture-not-a-real-secret-1"

wait_for() {
	local description="$1" tries=50
	shift
	until "$@"; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "vaultwarden-tier2-session: timed out waiting for ${description}" >&2
			exit 1
		fi
		sleep 0.2
	done
}

readonly DATA_DIR="${HOME}/vaultwarden-data"
readonly SSL_DIR="${HOME}/vaultwarden-ssl"
mkdir -p "${DATA_DIR}" "${SSL_DIR}"
cp "$(dirname "${BASH_SOURCE[0]}")/vaultwarden-fixture/db.sqlite3" "${DATA_DIR}/db.sqlite3"
cp "$(dirname "${BASH_SOURCE[0]}")/vaultwarden-fixture/rsa_key.pem" "${DATA_DIR}/rsa_key.pem"

openssl req -x509 -newkey rsa:2048 -keyout "${SSL_DIR}/key.pem" -out "${SSL_DIR}/cert.pem" \
	-days 1 -nodes -subj "/CN=localhost" 2>/dev/null

DATA_FOLDER="${DATA_DIR}" \
	ROCKET_PORT="${VAULTWARDEN_PORT}" \
	ROCKET_TLS="{certs=\"${SSL_DIR}/cert.pem\",key=\"${SSL_DIR}/key.pem\"}" \
	DOMAIN="${VAULTWARDEN_URL}" \
	SIGNUPS_ALLOWED="false" \
	WEB_VAULT_ENABLED="false" \
	/usr/local/bin/vaultwarden &
wait_for "Vaultwarden" wget -q --no-check-certificate -O /dev/null "${VAULTWARDEN_URL}/alive"

export NODE_EXTRA_CA_CERTS="${SSL_DIR}/cert.pem"
bw config server "${VAULTWARDEN_URL}" >/dev/null
bw login "${TEST_EMAIL}" "${TEST_MASTER_PASSWORD}" >/dev/null
SESSION=$(bw unlock "${TEST_MASTER_PASSWORD}" --raw)

exec env SSHAKKU_TEST_ALLOW_REAL_BITWARDEN="1" BW_SESSION="${SESSION}" "$@"
