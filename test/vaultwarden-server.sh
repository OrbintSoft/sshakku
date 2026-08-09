#!/usr/bin/env bash
#
# Stands the committed Vaultwarden fixture up in a container and runs the given
# command against it, with the fixture account's identity in the environment.
#
# The server runs in a Linux container and the client — the bw CLI and the test
# binary — runs here, on the host. That split is the point on a Mac, which has
# no Bitwarden server of its own and cannot be one: what needs exercising is a
# native bw talking to a real server, not a Linux box talking to itself.
#
# Login and unlock are BitwardenBackend's own job, not this script's; it only
# hands over where the server is and who to log in as. Needs a working docker.
#
# Usage: vaultwarden-server.sh <command> [args...]
set -euo pipefail

readonly PORT="8443"
readonly URL="https://localhost:${PORT}"
readonly CONTAINER="sshakku-vaultwarden-fixture"
readonly IMAGE="sshakku-vaultwarden-fixture"

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"
# shellcheck source=test/containers/vaultwarden-fixture-account.sh
. test/containers/vaultwarden-fixture-account.sh

# An explicit template, because a bare `mktemp` is a GNU extension: the macOS
# one this also runs on wants to be told the name.
work="$(mktemp -d "${TMPDIR:-/tmp}/sshakku-vaultwarden.XXXXXX")"
cleanup() {
	docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
	docker rmi -f "${IMAGE}" >/dev/null 2>&1 || true
	rm -rf "${work}"
}
trap cleanup EXIT

# The work directory is a build context, not a mount: what the container needs
# is sent to the daemon rather than shared with it, so nothing here depends on
# which of the host's directories the daemon can see — see
# test/containers/vaultwarden-server.Dockerfile. The certificate is generated
# per run and belongs to that one build.
#
# The server refuses to start on storage it cannot keep, which is exactly the
# storage wanted here, so it is told so: everything this vault holds is thrown
# away with the container, deliberately.
cp test/containers/vaultwarden-fixture/db.sqlite3 \
	test/containers/vaultwarden-fixture/rsa_key.pem "${work}/"
chmod u+w "${work}/db.sqlite3"

openssl req -x509 -newkey rsa:2048 -keyout "${work}/key.pem" -out "${work}/cert.pem" \
	-days 1 -nodes -subj "/CN=localhost" 2>/dev/null

docker build -q -f test/containers/vaultwarden-server.Dockerfile -t "${IMAGE}" "${work}" >/dev/null

docker run -d --name "${CONTAINER}" -p "${PORT}:${PORT}" \
	-e DATA_FOLDER=/data \
	-e I_REALLY_WANT_VOLATILE_STORAGE=true \
	-e ROCKET_PORT="${PORT}" \
	-e ROCKET_TLS='{certs="/ssl/cert.pem",key="/ssl/key.pem"}' \
	-e DOMAIN="${URL}" \
	-e SIGNUPS_ALLOWED=false \
	-e WEB_VAULT_ENABLED=false \
	"${IMAGE}" >/dev/null

# The published port accepts a connection before the server behind it will
# answer one, so this waits for the server's own answer. --insecure belongs to
# this wait and to nothing else: the certificate was generated a moment ago and
# no one has ever seen it, whereas the client under test is handed it properly
# below and is expected to verify it.
tries=120
until curl -fs --insecure -o /dev/null "${URL}/alive"; do
	tries=$((tries - 1))
	if [ "${tries}" -le 0 ]; then
		echo "vaultwarden-server: timed out waiting for ${URL}/alive" >&2
		docker logs "${CONTAINER}" >&2 || true
		exit 1
	fi
	sleep 1
done

# Not exec'd: the container has to be removed afterwards, and the certificate
# the command is pointed at has to outlive the command. Its status is the
# script's, so a failing command is not swallowed by a successful cleanup.
env \
	SSHAKKU_TEST_ALLOW_REAL_BITWARDEN="1" \
	SSHAKKU_TEST_BW_EMAIL="${VAULTWARDEN_FIXTURE_EMAIL}" \
	SSHAKKU_TEST_BW_PASSWORD="${VAULTWARDEN_FIXTURE_PASSWORD}" \
	SSHAKKU_TEST_BW_SERVER="${URL}" \
	NODE_EXTRA_CA_CERTS="${work}/cert.pem" \
	"$@"
