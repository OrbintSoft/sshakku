#!/usr/bin/env bash
#
# Runs the 1Password real-account test and requires that it really ran.
#
# The test skips itself when the op CLI is absent, when op is not
# authenticated, or when its opt-in variable is unset — and `go test -run`
# exits 0 on a skip, as it does when the pattern matches no test at all. By
# exit status alone, a run that reached a real 1Password account and a run that
# reached nothing look the same, so this asks the output for the test's own
# pass line instead. A run that ends here successfully is therefore one where
# the store, look up, list and delete round trip really happened.
#
# op must already be authenticated: OP_SERVICE_ACCOUNT_TOKEN for a service
# account, or a session signed in by hand. This never signs in itself, and
# never takes a credential as an argument.
#
# Usage: onepassword-real-account.sh
set -euo pipefail

test_name=TestOnePasswordBackendRealAccount

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

# An explicit template, because a bare `mktemp` is a GNU extension: the macOS
# one this also runs on wants to be told the name.
log="$(mktemp "${TMPDIR:-/tmp}/sshakku-onepassword.XXXXXX")"
trap 'rm -f "$log"' EXIT

status=0
go test -count=1 -run "^${test_name}\$" -v ./internal/keys/... 2>&1 | tee "$log" || status=$?
if [ "$status" -ne 0 ]; then
	exit "$status"
fi

if ! grep -q -- "--- PASS: ${test_name}" "$log"; then
	echo "${test_name} did not run: the test suite reported success without reaching a 1Password account." >&2
	echo "The output above says which of op's preconditions was not met." >&2
	exit 1
fi
