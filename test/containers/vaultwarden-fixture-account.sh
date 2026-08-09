# shellcheck shell=bash
# shellcheck disable=SC2034  # read by the scripts that source this file
#
# The identity of the disposable account baked into vaultwarden-fixture/.
# Sourced by whatever stands that fixture up, so the two callers cannot drift
# apart: the account's own database is a committed binary, and a password that
# no longer matches it is indistinguishable from a broken server.
#
# This account's only purpose is to hold a disposable, empty test vault; the
# password protects nothing of value and is fixed on purpose, so every run of
# this fixture unlocks the same way. Never reuse it for anything real.
#
# This exact string is baked into vaultwarden-fixture/db.sqlite3: both the
# stored login hash and the wrapping of the vault key are derived from it, so it
# is an opaque token bound to that binary, not free-form prose. Changing it
# without regenerating the fixture makes login fail with "Username or password
# is incorrect" — do not "tidy" it.
VAULTWARDEN_FIXTURE_EMAIL="sshakku-test@example.invalid"
VAULTWARDEN_FIXTURE_PASSWORD="sshakku-tier2-fixture-not-a-real-secret-1"
