#!/usr/bin/env bats
# Shell-level regression checklist for the login hook and agent lifecycle
# (nn-ssh-init.sh + the sshakku binary it drives), run against real
# ssh-agent/ssh-add processes. A stub secret-tool (test/bats/fixtures)
# stands in for a real Secret Service so the vault is reachable without a
# desktop session.
#
# Every bash invocation below is deliberately non-interactive
# (`$- != *i*`): an interactive shell would source system rc files this
# suite has no control over — on a real machine that can mean a real
# system-wide sshakku install and its real login hook, not this suite's
# isolated one (see helpers.bash's SSHAKKU_TEST_ALLOW_BATS gate). Loading
# keys is driven by calling `sshakku load-keys` directly instead, the same
# call the hook's own interactive branch makes.
#
# The container this runs in has no controlling terminal at all, so a live
# interactive passphrase prompt cannot be driven from here either — that
# case is covered by the Go suite, which allocates a pseudo-terminal of
# its own rather than depending on the one it was started from.
#
# agent_sock/agent_lock/log_file below come from `eval "$(sshakku
# shell-init)"`, the same mechanism nn-ssh-init.sh itself uses — a
# static analyzer cannot trace an eval'd assignment, hence the file-wide
# disable below.
# shellcheck disable=SC2154

load helpers

@test "sourcing the hook exports the fixed socket and does not load keys outside an interactive shell" {
	new_test_key id_test "test-passphrase"
	seed_vault id_test "test-passphrase"

	eval "$("$SSHAKKU_BIN" shell-init)"
	want_sock="$agent_sock"

	run bash -c 'source "$SSHAKKU_HOOK"; echo "SOCK=$SSH_AUTH_SOCK"'
	[ "$status" -eq 0 ]
	[[ "$output" == *"SOCK=$want_sock"* ]]

	fingerprint=$(key_fingerprint id_test)
	run ssh-add -l
	[[ "$output" != *"$fingerprint"* ]]
}

@test "a second shell sees the already-loaded key and does not add it again" {
	require_keyring
	new_test_key id_test "test-passphrase"
	seed_vault id_test "test-passphrase"
	fingerprint=$(key_fingerprint id_test)

	eval "$("$SSHAKKU_BIN" shell-init)"
	first_sock="$agent_sock"
	"$SSHAKKU_BIN" load-keys
	run ssh-add -l
	[[ "$output" == *"$fingerprint"* ]]

	eval "$("$SSHAKKU_BIN" shell-init)"
	[ "$agent_sock" = "$first_sock" ]
	"$SSHAKKU_BIN" load-keys

	run ssh-add -l
	[[ "$output" == *"$fingerprint"* ]]
	# Exactly one line for this key in ssh-add -l: the second shell must not
	# have added a duplicate.
	key_lines=$(printf '%s\n' "$output" | grep -c "$fingerprint" || true)
	[ "$key_lines" -eq 1 ]
}

@test "killing the agent lets a new shell restart it at the same socket and reload the key from the vault" {
	require_keyring
	new_test_key id_test "test-passphrase"
	seed_vault id_test "test-passphrase"
	fingerprint=$(key_fingerprint id_test)

	eval "$("$SSHAKKU_BIN" shell-init)"
	"$SSHAKKU_BIN" load-keys
	run ssh-add -l
	[[ "$output" == *"$fingerprint"* ]]

	before_sock="$agent_sock"
	before_inode=$(socket_inode "$before_sock")
	before_pid=$(doctor_recorded_pid)
	[ -n "$before_pid" ]

	kill -9 "$before_pid"
	for _ in $(seq 1 50); do
		kill -0 "$before_pid" 2>/dev/null || break
		sleep 0.1
	done
	run ! kill -0 "$before_pid"

	eval "$("$SSHAKKU_BIN" shell-init)"
	"$SSHAKKU_BIN" load-keys

	[ "$agent_sock" = "$before_sock" ]
	after_inode=$(socket_inode "$agent_sock")
	[ "$after_inode" != "$before_inode" ]
	after_pid=$(doctor_recorded_pid)
	[ "$after_pid" != "$before_pid" ]

	run ssh-add -l
	[[ "$output" == *"$fingerprint"* ]]
}

@test "an empty vault does not load the key and never hangs or crashes with no terminal" {
	new_test_key id_test "test-passphrase"
	fingerprint=$(key_fingerprint id_test)

	# With nothing stored and no terminal, what is left to ask on is a dialog —
	# and on a machine whose session says it has a screen, one is raised. This
	# runner's does, and there is nobody in front of it, which is F21's case
	# exactly: sshakku has to give up on its own rather than be waited out. The
	# budget is named so that giving up is what is measured; left at its two
	# minute default, a run that never gave up would look the same.
	# shellcheck disable=SC2030
	export SSHAKKU_INTERACTIVE_TIMEOUT=5s

	eval "$("$SSHAKKU_BIN" shell-init)"
	run no_tty_bounded 20 "$SSHAKKU_BIN" load-keys
	[ "$status" -ne 124 ]
	[ "$status" -eq 0 ]

	run ssh-add -l
	[[ "$output" != *"$fingerprint"* ]]
}

@test "a vault-seeded passphrase loads silently even with no terminal at all" {
	require_keyring
	new_test_key id_test "test-passphrase"
	seed_vault id_test "test-passphrase"
	fingerprint=$(key_fingerprint id_test)

	eval "$("$SSHAKKU_BIN" shell-init)"
	run no_tty_bounded 5 "$SSHAKKU_BIN" load-keys
	[ "$status" -eq 0 ]

	run ssh-add -l
	[[ "$output" == *"$fingerprint"* ]]
}

@test "F5: a vault-seeded passphrase loads silently from a long home directory too" {
	# 90 characters: long for a home directory, and nothing any file system
	# objects to — a single name may be 255 on both platforms this runs on.
	# Nothing in what SSHakku promises is conditional on how a user's home is
	# named, so this is the same scenario as the test above it, moved.
	#
	# Moved before the environment is checked, so that a machine which cannot
	# run the rest of this test still runs the move: a home relocated wrongly
	# would otherwise only ever be found on the platform where the scenario
	# does complete.
	long_home 90
	require_keyring

	new_test_key id_test "test-passphrase"
	seed_vault id_test "test-passphrase"
	fingerprint=$(key_fingerprint id_test)

	eval "$("$SSHAKKU_BIN" shell-init)"
	run no_tty_bounded 10 "$SSHAKKU_BIN" load-keys
	[ "$status" -eq 0 ]
	load_said="$output"

	run ssh-add -l
	if [[ "$output" != *"$fingerprint"* ]]; then
		# A key that did not load has more than one way of not loading, and the
		# listing says only that it isn't there. What the run itself said, and
		# what the session log recorded, name the step that gave up.
		echo "load-keys said: ${load_said:-(nothing)}" >&2
		echo "ssh-add -l said: $output" >&2
		echo "session log:" >&2
		cat "$log_file" >&2 2>/dev/null || echo "(no session log at $log_file)" >&2
		return 1
	fi
}

@test "a reachable but empty agent is adopted, not killed and replaced" {
	eval "$("$SSHAKKU_BIN" shell-init)"
	bootstrap_pid=$(doctor_recorded_pid)
	[ -n "$bootstrap_pid" ]
	sock="$agent_sock"

	kill -9 "$bootstrap_pid"
	for _ in $(seq 1 50); do
		kill -0 "$bootstrap_pid" 2>/dev/null || break
		sleep 0.1
	done
	rm -f "$sock"

	eval "$(ssh-agent -a "$sock")"
	own_pid="$SSH_AGENT_PID"
	run ssh-add -l
	[ "$status" -eq 1 ]
	[[ "$output" == *"no identities"* ]]

	eval "$("$SSHAKKU_BIN" shell-init)"
	[ "$agent_sock" = "$sock" ]
	# Adopting a healthy pre-existing agent never writes agent.state (that
	# only happens when sshakku starts one itself), so the proof of
	# "adopted, not killed and replaced" is the original process simply
	# still being alive under the same socket, not a doctor-recorded pid.
	kill -0 "$own_pid"
}

@test "install-user-hook.sh leaves .bashrc/.bashrc.d untouched without the opt-in flag" {
	"$REPO_ROOT/install-user-hook.sh" install "$HOME" "$SSHAKKU_BIN" 001
	[ -f "$HOME/.bash_profile" ]
	[ ! -e "$HOME/.bashrc" ]
	[ ! -e "$HOME/.bashrc.d" ]
}

@test "install-user-hook.sh with the opt-in flag wires .bashrc when .bashrc.d is absent" {
	"$REPO_ROOT/install-user-hook.sh" install "$HOME" "$SSHAKKU_BIN" 001 1
	grep -q '# >>> sshakku >>>' "$HOME/.bashrc"
	grep -q 'shell-hook.sh' "$HOME/.bashrc"
}

@test "install-user-hook.sh with the opt-in flag drops a file into an existing .bashrc.d instead" {
	mkdir -p "$HOME/.bashrc.d"
	"$REPO_ROOT/install-user-hook.sh" install "$HOME" "$SSHAKKU_BIN" 001 1
	[ -f "$HOME/.bashrc.d/001-sshakku-init.sh" ]
	[ ! -e "$HOME/.bashrc" ]
}

@test "install-user-hook.sh uninstall removes whichever bashrc artifact was wired" {
	mkdir -p "$HOME/.bashrc.d"
	"$REPO_ROOT/install-user-hook.sh" install "$HOME" "$SSHAKKU_BIN" 001 1
	[ -f "$HOME/.bashrc.d/001-sshakku-init.sh" ]

	"$REPO_ROOT/install-user-hook.sh" uninstall "$HOME" 001
	[ ! -e "$HOME/.bashrc.d/001-sshakku-init.sh" ]
	run ! grep -q 'sshakku' "$HOME/.bash_profile"
}
