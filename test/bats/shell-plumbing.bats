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
# interactive passphrase prompt cannot be driven here either — that is
# covered at the Go level instead (internal/keys's headless integration
# tests).
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

	run ssh-add -l
	[[ "$output" != *"id_test"* ]]
}

@test "a second shell sees the already-loaded key and does not add it again" {
	require_keyring
	new_test_key id_test "test-passphrase"
	seed_vault id_test "test-passphrase"

	eval "$("$SSHAKKU_BIN" shell-init)"
	first_sock="$agent_sock"
	"$SSHAKKU_BIN" load-keys
	run ssh-add -l
	[[ "$output" == *"id_test"* ]]

	eval "$("$SSHAKKU_BIN" shell-init)"
	[ "$agent_sock" = "$first_sock" ]
	"$SSHAKKU_BIN" load-keys

	run ssh-add -l
	[[ "$output" == *"id_test"* ]]
	# Exactly one key line in ssh-add -l: the second shell must not have
	# added a duplicate.
	key_lines=$(printf '%s\n' "$output" | grep -c "id_test")
	[ "$key_lines" -eq 1 ]
}

@test "killing the agent lets a new shell restart it at the same socket and reload the key from the vault" {
	require_keyring
	new_test_key id_test "test-passphrase"
	seed_vault id_test "test-passphrase"

	eval "$("$SSHAKKU_BIN" shell-init)"
	"$SSHAKKU_BIN" load-keys
	run ssh-add -l
	[[ "$output" == *"id_test"* ]]

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
	[[ "$output" == *"id_test"* ]]
}

@test "an empty vault does not load the key and never hangs or crashes with no terminal" {
	new_test_key id_test "test-passphrase"

	eval "$("$SSHAKKU_BIN" shell-init)"
	run timeout --signal=KILL 5 setsid "$SSHAKKU_BIN" load-keys
	[ "$status" -eq 0 ]

	run ssh-add -l
	[[ "$output" != *"id_test"* ]]
}

@test "a vault-seeded passphrase loads silently even with no terminal at all" {
	require_keyring
	new_test_key id_test "test-passphrase"
	seed_vault id_test "test-passphrase"

	eval "$("$SSHAKKU_BIN" shell-init)"
	run timeout --signal=KILL 5 setsid "$SSHAKKU_BIN" load-keys
	[ "$status" -eq 0 ]

	run ssh-add -l
	[[ "$output" == *"id_test"* ]]
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
