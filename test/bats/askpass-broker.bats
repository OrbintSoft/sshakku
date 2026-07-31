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
# Every run is wrapped in `setsid` so there is no controlling terminal: the
# terminal fallback cannot rescue a run that was supposed to be silent, so
# either the wallet answers through the broker or the run fails. `timeout`
# bounds it because a broker that hangs waiting for input would otherwise stall
# the suite instead of failing it.
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
	run timeout --signal=KILL 10 setsid ssh-add "$HOME/.ssh/id_test"
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

# blocking_wallet puts a secret-tool that never answers ahead of the stub on
# PATH, standing in for a wallet that is present but locked behind an unlock
# prompt nobody is there to answer.
blocking_wallet() {
	mkdir -p "$BATS_TEST_TMPDIR/blocked"
	cp "$BATS_TEST_DIRNAME/fixtures/blocking-secret-tool" "$BATS_TEST_TMPDIR/blocked/secret-tool"
	chmod +x "$BATS_TEST_TMPDIR/blocked/secret-tool"
	export PATH="$BATS_TEST_TMPDIR/blocked:$PATH"
}

# Verifies feature F17: a backend that cannot answer degrades to prompting
# rather than failing the shell — and, before that can even be tried, must not
# hold the shell up. Routing every ssh prompt through the broker means an
# unbounded wallet call is no longer one slow login but every ssh in the
# session, so both entry points are checked.
@test "a wallet that never answers does not hold up an ssh" {
	new_test_key id_test "test-passphrase"
	blocking_wallet
	# Also exercises the setting itself. ssh asks the helper once per passphrase
	# attempt, so the wait the user actually sees is a small multiple of this.
	# Each bats test is its own subshell and wants its own value, which is what
	# SC2030/SC2031 warn about here.
	# shellcheck disable=SC2030,SC2031
	export SSHAKKU_COMMAND_TIMEOUT=1s

	trace "sourcing the hook"
	# shellcheck source=/dev/null  # installed at a path only known at run time
	source "$SSHAKKU_HOOK"
	trace "hook sourced"

	run timeout --signal=KILL 30 setsid ssh-add "$HOME/.ssh/id_test"
	# 137 is the KILL from timeout, i.e. it was still waiting on the wallet.
	# Any other status means it gave up on its own and ssh could move on.
	[ "$status" -ne 137 ]
}

@test "a wallet that never answers does not hold up a login shell" {
	new_test_key id_test "test-passphrase"
	blocking_wallet
	# shellcheck disable=SC2030,SC2031
	export SSHAKKU_COMMAND_TIMEOUT=1s

	run timeout --signal=KILL 30 setsid "$SSHAKKU_BIN" load-keys
	[ "$status" -ne 137 ]
}
