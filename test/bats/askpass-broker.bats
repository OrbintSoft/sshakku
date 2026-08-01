#!/usr/bin/env bats
# The reactive wallet refill, driven through a real OpenSSH binary.
#
# Verifies feature F6 (docs/FEATURES.md): after a key expires from the agent,
# the next ssh in a shell that is already open gets the passphrase from the
# wallet, with no prompt. The scenario is the one a user actually reaches — the
# login hook's own wiring, then an ssh tool asking for a passphrase — rather
# than a hand-built environment.
#
# `ssh-add <encrypted-key>` is the driver: it emits the same passphrase prompt
# ssh does, it honours SSH_ASKPASS/SSH_ASKPASS_REQUIRE for real, and it needs no
# sshd. It is invoked with no SSHAKKU_HANDOFF_TOKEN in the environment, which is
# what routes it to the wallet broker instead of the proactive loader's one-shot
# stash — and what a user's own ssh-add does.
#
# Every run goes through no_tty_bounded, so it has no controlling terminal and
# a hard time limit: the terminal fallback cannot rescue a run that was supposed
# to be silent — either the wallet answers through the broker or the run fails —
# and a broker that hangs waiting for input fails the test instead of stalling
# the suite.
#
# A stub secret-tool (test/bats/fixtures) stands in for a real Secret Service,
# and helpers.bash unsets DISPLAY/WAYLAND_DISPLAY with no kdialog on PATH: this
# suite is a headless session by construction, which is the condition under
# which the refill must still work.
#
# log_file below comes from sourcing the installed hook, which a static analyzer
# cannot trace.
# shellcheck disable=SC2154

load helpers

@test "the askpass broker is wired with no graphical prompter" {
	run "$SSHAKKU_BIN" askpass-env
	[ "$status" -eq 0 ]
	[[ "$output" == *"export SSH_ASKPASS='$SSHAKKU_BIN'"* ]]
	[[ "$output" == *"export SSH_ASKPASS_REQUIRE=force"* ]]
	[[ "$output" == *"export SSHAKKU_ASKPASS=1"* ]]
}

@test "a key missing from the agent is refilled from the wallet with no terminal" {
	new_test_key id_test "test-passphrase"
	seed_vault id_test "test-passphrase"

	# The hook itself, sourced non-interactively: it pins SSH_AUTH_SOCK and wires
	# the broker but does not load keys, which is exactly the state F6 describes
	# — a shell that is already open, with the key no longer in the agent.
	trace "sourcing the hook"
	# shellcheck source=/dev/null  # installed at a path only known at run time
	source "$SSHAKKU_HOOK"
	trace "hook sourced"

	# Prove the run tests what it claims to: the broker, reached through the
	# exports, and not the proactive stash.
	[ -n "${SSH_ASKPASS:-}" ]
	[ "${SSH_ASKPASS_REQUIRE:-}" = "force" ]
	[ -z "${SSHAKKU_HANDOFF_TOKEN:-}" ]

	trace "ssh-add start"
	run no_tty_bounded 10 ssh-add "$HOME/.ssh/id_test"
	trace "ssh-add exited with $status"
	[ "$status" -eq 0 ]

	# Matched by fingerprint: `ssh-add -l` identifies a key by its comment
	# (user@host), which says nothing about which file it came from.
	fingerprint=$(ssh-keygen -lf "$HOME/.ssh/id_test.pub" | awk '{print $2}')
	run ssh-add -l
	[[ "$output" == *"$fingerprint"* ]]

	# The load-bearing assertion: the passphrase came from the wallet, not from
	# a stray terminal, a cached agent entry, or an unencrypted key.
	grep -q "askpass: provided passphrase for id_test from the wallet" "$log_file"
}

# blocking_wallet selects the named secret backend and puts a stand-in for its
# CLI ahead of the real one on PATH — a wallet that is present but never
# answers, waiting on something nobody is there to provide.
#
# The backend is named rather than inherited from the platform. F21 is a promise
# about any program SSHakku runs, so it has to hold for every backend that runs
# one; driving whichever implementation the host happens to default to would
# make the coverage an accident of the machine rather than a statement about the
# product. `secret-tool` is not "the wallet", it is one backend of several.
#
# The stand-in goes under $TEST_ROOT so teardown's sweep reaches whatever it
# leaves running: it outlives the caller that gave up on it.
blocking_wallet() {
	local backend="$1" binary="$2"
	mkdir -p "$TEST_ROOT/blocked" "$XDG_CONFIG_HOME/sshakku"
	cp "$BATS_TEST_DIRNAME/fixtures/blocking-$binary" "$TEST_ROOT/blocked/$binary"
	chmod +x "$TEST_ROOT/blocked/$binary"
	export PATH="$TEST_ROOT/blocked:$PATH"
	cp "$BATS_TEST_DIRNAME/fixtures/${backend}-backend.toml" "$XDG_CONFIG_HOME/sshakku/config.toml"
}

# require_secret_service skips where that backend cannot exist: the Secret
# Service is a freedesktop D-Bus API and has no macOS implementation, so there
# is no such thing to drive there. The subject is absent, not inconvenient.
require_secret_service() {
	case "$OSTYPE" in
	darwin*) skip "the freedesktop Secret Service has no macOS implementation, so this backend does not exist there" ;;
	esac
}

# The four cases below verify feature F21 — nothing SSHakku waits on can hold the
# shell up with no end — for the half of it reached by running a program: once per
# backend that reaches its wallet that way, and once per entry point. A wallet
# SSHakku calls into directly is the other half, covered elsewhere because no
# shell test can block it. Both entry points matter because routing
# every ssh prompt through the broker turns an unbounded wallet call from one
# slow login into every ssh in the session.
#
# Each names the budget that governs it, and they differ: secret-tool is a
# lookup expected to answer on its own, so command_timeout bounds it, while `op`
# may defer to the 1Password desktop app for approval and is bounded by
# interactive_timeout. Setting the wrong one would leave the call unbounded and
# the test passing for the wrong reason.

@test "F21: a secret-service wallet that never answers does not hold up an ssh" {
	require_secret_service
	new_test_key id_test "test-passphrase"
	blocking_wallet secret-service secret-tool
	# Each bats test is its own subshell and wants its own value, which is what
	# SC2030/SC2031 warn about here.
	# shellcheck disable=SC2030,SC2031
	export SSHAKKU_COMMAND_TIMEOUT=1s

	# shellcheck source=/dev/null  # installed at a path only known at run time
	source "$SSHAKKU_HOOK"

	run no_tty_bounded 30 ssh-add "$HOME/.ssh/id_test"
	# 124 is the limit expiring, i.e. it was still waiting on the wallet. Any
	# other status means it gave up on its own and ssh could move on.
	[ "$status" -ne 124 ]
}

@test "F21: a secret-service wallet that never answers does not hold up a login shell" {
	require_secret_service
	new_test_key id_test "test-passphrase"
	blocking_wallet secret-service secret-tool
	# shellcheck disable=SC2030,SC2031
	export SSHAKKU_COMMAND_TIMEOUT=1s

	run no_tty_bounded 30 "$SSHAKKU_BIN" load-keys
	[ "$status" -ne 124 ]
}

@test "F21: a 1Password wallet that never answers does not hold up an ssh" {
	new_test_key id_test "test-passphrase"
	blocking_wallet onepassword op
	# shellcheck disable=SC2030,SC2031
	export SSHAKKU_INTERACTIVE_TIMEOUT=1s

	# shellcheck source=/dev/null  # installed at a path only known at run time
	source "$SSHAKKU_HOOK"

	run no_tty_bounded 30 ssh-add "$HOME/.ssh/id_test"
	[ "$status" -ne 124 ]
}

@test "F21: a 1Password wallet that never answers does not hold up a login shell" {
	new_test_key id_test "test-passphrase"
	blocking_wallet onepassword op
	# shellcheck disable=SC2030,SC2031
	export SSHAKKU_INTERACTIVE_TIMEOUT=1s

	run no_tty_bounded 30 "$SSHAKKU_BIN" load-keys
	[ "$status" -ne 124 ]
}
