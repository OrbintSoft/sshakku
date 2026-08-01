#!/usr/bin/env bats
# The two commands that reach the wallet on their own, driven the way a user
# runs them: `sshakku doctor --test-backend` and `sshakku forget`.
#
# Verifies features F15 and F9 (docs/FEATURES.md). Neither command had ever
# been run against a real wallet by any test: the rest of the suite reaches the
# store through the loader, and seeds it first. On darwin that seeding passes
# `security add-generic-password -A`, which marks the item readable by any
# program — a convenience for a fixture, and the reason nothing here had ever
# met a keychain item in the state a real one is in, where an access has to be
# authorised by a person.
#
# Every run goes through no_tty_bounded: no controlling terminal, so nothing can
# rescue itself by asking whoever started it, and a hard limit, so a command
# waiting for an answer nobody will give fails this test instead of stalling the
# suite. Status 124 is that limit expiring: a command still waiting when it
# expires is what F21 forbids, so no case here may end that way.
# shellcheck disable=SC2154

load helpers

# The budgets are named rather than left at their defaults: these tests assert
# that something gives up, and one that waited out a two-minute default would be
# indistinguishable from one that never gives up at all.
budget_5s() {
	# Each bats test is its own subshell and wants its own value, which is what
	# SC2030/SC2031 warn about here.
	# shellcheck disable=SC2030,SC2031
	export SSHAKKU_COMMAND_TIMEOUT=5s
	# shellcheck disable=SC2030,SC2031
	export SSHAKKU_INTERACTIVE_TIMEOUT=5s
}

@test "F15: doctor --test-backend round-trips the real wallet and comes back" {
	budget_5s

	run no_tty_bounded 30 "$SSHAKKU_BIN" doctor --test-backend
	[ "$status" -ne 124 ]

	# The probe writes an entry, reads it back and deletes it, all through the
	# backend the configuration selects. Every step has to pass: a store a
	# passphrase cannot be read back out of is not one to put a passphrase in.
	[ "$status" -eq 0 ]
	[[ "$output" == *"store: ok"* ]]
	[[ "$output" == *"lookup: ok"* ]]
	[[ "$output" == *"delete: ok"* ]]
}

@test "F9: forget removes a stored passphrase, and says so only once it is gone" {
	budget_5s
	seed_vault id_test "test-passphrase"

	run no_tty_bounded 30 "$SSHAKKU_BIN" forget id_test
	[ "$status" -ne 124 ]

	[ "$status" -eq 0 ]
	[[ "$output" == *"forgot SSH-Key-id_test"* ]]

	# F9's second half: the entry is really gone, not merely reported gone.
	refute_vault_entry id_test
}

# What this does NOT cover: a wallet that actually makes SSHakku wait. Getting
# Security.framework to wait needs an interactive GUI session for SecurityAgent
# to put its dialog in, and a hosted macOS runner has none — it fails such a
# call rather than blocking on it. So the entry below is one SSHakku is not
# entitled to touch, and what is asserted is that it is handled, not that a wait
# was survived. F21's keychain half cannot be reached from CI at all.
@test "F9: forget handles an entry written by another program rather than hanging on it" {
	budget_5s
	seed_vault_needing_approval id_test "test-passphrase"

	# Deleting it may or may not be permitted, and which one is not the promise:
	# F9 says SSHakku must never report a passphrase as forgotten while it is
	# still stored. So either outcome is fine as long as the two agree.
	run no_tty_bounded 30 "$SSHAKKU_BIN" forget id_test
	[ "$status" -ne 124 ]

	if [[ "$output" == *"forgot SSH-Key-id_test"* ]]; then
		refute_vault_entry id_test
	fi
}

@test "F27: forget --all forgets SSHakku's own entries and leaves other programs' alone" {
	budget_5s
	seed_vault id_test "test-passphrase"
	seed_foreign_secret "Another App-credentials" "not-ours"

	run no_tty_bounded 30 "$SSHAKKU_BIN" forget --all
	[ "$status" -ne 124 ]

	# What F27 promises is about what is left standing, so it is asserted
	# whatever the command managed to do: a wallet with no way to enumerate its
	# entries answers that it cannot forget everything, and that answer has to
	# leave the other program's entry alone just the same.
	assert_foreign_secret "Another App-credentials"

	# And it is not merely left in place: an entry sshakku did not store is not
	# sshakku's to enumerate either, so it has no business being named on the way
	# past — as forgotten, as failed, or at all.
	[[ "$output" != *"Another App-credentials"* ]]

	if [[ "$output" == *"forgot SSH-Key-id_test"* ]]; then
		refute_vault_entry id_test
	fi
}
