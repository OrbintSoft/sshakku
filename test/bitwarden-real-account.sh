#!/usr/bin/env bash
#
# Runs the Bitwarden real-account test and requires that it really ran.
#
# The test skips itself when the bw CLI is absent, when its opt-in variable is
# unset, and when the account's email or password is empty — and `go test -run`
# exits 0 on a skip, as it does when the pattern matches no test at all. By exit
# status alone, a run that reached a real vault and a run that reached nothing
# look the same, so this asks the output for the test's own pass line instead. A
# run that ends here successfully is therefore one where the store, look up,
# list, update and delete round trip really happened.
#
# The server it talks to is somebody else's job: vaultwarden-server.sh stands
# one up and runs this against it.
#
# Usage: bitwarden-real-account.sh
set -euo pipefail

test_name=TestBitwardenBackendRealAccount

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

# An explicit template, because a bare `mktemp` is a GNU extension: the macOS
# one this also runs on wants to be told the name.
log="$(mktemp "${TMPDIR:-/tmp}/sshakku-bitwarden.XXXXXX")"
trap 'rm -f "$log"' EXIT

status=0
go test -count=1 -run "^${test_name}\$" -v ./internal/keys/... 2>&1 | tee "$log" || status=$?
if [ "$status" -ne 0 ]; then
	exit "$status"
fi

if ! grep -q -- "--- PASS: ${test_name}" "$log"; then
	echo "${test_name} did not run: the test suite reported success without reaching a Bitwarden vault." >&2
	echo "The output above says which of its preconditions was not met." >&2
	exit 1
fi
