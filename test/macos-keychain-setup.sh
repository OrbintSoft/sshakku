#!/usr/bin/env bash
#
# Points the process's default keychain at a throwaway, unlocked keychain, so a
# test that stores generic-password items through Security.framework lands them
# there and never in the runner's real login keychain. Meant to run on a real
# macOS runner right before the live-keychain integration test.
#
# The keychain is disposable and its password is a fixed, non-secret constant:
# it guards nothing real, it only lets `security` create and unlock the file
# non-interactively. The login keychain is kept on the search list so anything
# else that reads it still works; only the default (where unqualified writes
# go) is repointed.
#
# Usage: macos-keychain-setup.sh
set -euo pipefail

keychain="${RUNNER_TEMP:-/tmp}/sshakku-test.keychain-db"
password="sshakku-test"

# The current default keychain, unquoted and untrimmed by xargs, so it can be
# kept on the search list alongside the throwaway one.
login_keychain="$(security default-keychain -d user | xargs)"

# Recreate from scratch so a rerun on a reused runner starts clean.
security delete-keychain "$keychain" 2>/dev/null || true
security create-keychain -p "$password" "$keychain"

# No auto-lock: neither an idle timeout nor a sleep re-locks it mid-run.
security set-keychain-settings "$keychain"
security unlock-keychain -p "$password" "$keychain"

# Put the throwaway keychain first on the user search list (login stays on it),
# then make it the default so SecItemAdd without an explicit keychain writes
# here.
security list-keychains -d user -s "$keychain" "$login_keychain"
security default-keychain -d user -s "$keychain"
