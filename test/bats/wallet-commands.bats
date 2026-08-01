#!/usr/bin/env bats
# The two commands that reach the wallet on their own, driven the way a user
# runs them: `sshakku doctor --test-backend` and `sshakku forget`.
#
# Verifies features F15, F9 and F21 (docs/FEATURES.md). Neither command had ever
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
# suite. Status 124 is that limit expiring, and it is the failure these tests
# exist to catch — a command still waiting is exactly what F21 forbids.
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

@test "F21: forget comes back on a wallet entry that needs someone to approve access" {
	budget_5s
	seed_vault_needing_approval id_test "test-passphrase"

	# Whether the entry is deleted is not the promise under test: nobody is here
	# to authorise it, so it may well survive. The promise is that SSHakku stops
	# waiting and hands the terminal back, rather than holding it with no end.
	run no_tty_bounded 30 "$SSHAKKU_BIN" forget id_test
	[ "$status" -ne 124 ]
}
