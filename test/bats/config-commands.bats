#!/usr/bin/env bats
# The configuration a user writes, driven through the real binary the way they
# would drive it: `load-keys` and `doctor` against a stated key directory and
# name rule, and `config` / `config --edit` against a configuration split
# across a file, a drop-in and an exported variable.
#
# Verifies features F34, F35 and F36 (docs/FEATURES.md); each round below is
# one line of their "how you can tell". Nothing in them reaches a facility of
# the operating system — a directory, a glob, a file and an environment — which
# is why both platforms are expected to answer identically here, and why
# running it on one of them said nothing about the other.
#
# What the F34 rounds judge is which files SSHakku treats as the user's keys,
# and the evidence for that is SSHakku naming the file — in the session log, as
# a key it went and asked the wallet about, and in the report. Whether the key
# then reaches the agent is a different promise (F4, F5, shell-plumbing.bats),
# and it turns on a passphrase handoff that not every environment has: asserting
# it here would leave the selection unexercised wherever the handoff is missing,
# which is the whole state this suite exists to end.
#
# agent_sock/log_file below come from `eval "$(sshakku shell-init)"`, which a
# static analyzer cannot trace.
# shellcheck disable=SC2154

load helpers

# use_config installs one of this suite's configuration fixtures as the user's
# own config.toml.
use_config() {
	mkdir -p "$XDG_CONFIG_HOME/sshakku"
	cp "$BATS_TEST_DIRNAME/fixtures/$1" "$XDG_CONFIG_HOME/sshakku/config.toml"
}

# use_dropin installs one under config.d/, where a file may be somebody else's
# to maintain.
use_dropin() {
	mkdir -p "$XDG_CONFIG_HOME/sshakku/config.d"
	cp "$BATS_TEST_DIRNAME/fixtures/$1" "$XDG_CONFIG_HOME/sshakku/config.d/50-work.toml"
}

# config_line prints what `sshakku config` says about one setting: the value in
# force and where that value came from.
config_line() {
	bounded 20 "$SSHAKKU_BIN" config | awk -v name="$1" '$1 == name'
}

# new_key_in generates a key in a directory of the caller's choosing, which
# new_test_key cannot: that one writes to ~/.ssh, and a key directory the user
# named is somewhere else by definition.
new_key_in() {
	local dir="$1" name="$2" passphrase="$3"
	mkdir -p "$dir"
	ssh-keygen -t ed25519 -N "$passphrase" -f "$dir/$name" -q
}

# start_agent drives the fixed socket to a healthy agent and points this shell
# at it, the two things the login hook does for a real login shell — so
# `load-keys` below runs against an agent, as it does at a real login, rather
# than against nothing.
start_agent() {
	eval "$("$SSHAKKU_BIN" shell-init)"
	export SSH_AUTH_SOCK="$agent_sock"
}

# use_editor points $EDITOR at this suite's stand-in and tells it what to leave
# behind in the file it is handed. $VISUAL is cleared so the editor under test
# is the one named here and not one the environment happened to carry.
use_editor() {
	export SSHAKKU_TEST_EDITOR_ARGV="$TEST_ROOT/editor-argv"
	export SSHAKKU_TEST_EDITOR_LINE="$1"
	export EDITOR="config-editor"
	unset VISUAL
}

@test "F34: a key the default rule does not name is neither loaded nor reported" {
	new_test_key work-github "test-passphrase"
	seed_vault work-github "test-passphrase"

	start_agent
	run bounded 30 "$SSHAKKU_BIN" load-keys
	[ "$status" -eq 0 ]

	# Never asked about, and so never loaded.
	run cat "$log_file"
	[[ "$output" != *"work-github"* ]]

	run bounded 30 "$SSHAKKU_BIN" doctor
	[[ "$output" == *"keys in $HOME/.ssh"* ]]
	[[ "$output" != *"work-github"* ]]
}

@test "F34: key_patterns names it, and then it is loaded and listed" {
	use_config keys-named-by-patterns.toml
	new_test_key work-github "test-passphrase"
	seed_vault work-github "test-passphrase"

	start_agent
	run bounded 30 "$SSHAKKU_BIN" load-keys
	[ "$status" -eq 0 ]

	# Asked about, which is what a file SSHakku takes for a key of the user's
	# gets, and the exact mirror of the round above.
	run cat "$log_file"
	[[ "$output" == *"work-github"* ]]

	run bounded 30 "$SSHAKKU_BIN" doctor
	[[ "$output" == *"work-github"* ]]
}

@test "F34: the keys loaded and reported are the ones in the directory the configuration names" {
	use_config keys-in-another-directory.toml
	new_test_key id_home "home-passphrase"
	new_key_in "$HOME/work-keys" deploy "test-passphrase"
	seed_vault id_home "home-passphrase"
	seed_vault deploy "test-passphrase"

	start_agent
	run bounded 30 "$SSHAKKU_BIN" load-keys
	[ "$status" -eq 0 ]

	run cat "$log_file"
	[[ "$output" == *"deploy"* ]]
	[[ "$output" != *"id_home"* ]]

	run bounded 30 "$SSHAKKU_BIN" doctor
	[[ "$output" == *"keys in $HOME/work-keys"* ]]
	[[ "$output" == *"deploy"* ]]
	[[ "$output" != *"id_home"* ]]
}

@test "F34: the widest name rule still never treats what is not a key as one" {
	use_config keys-in-another-directory.toml
	new_key_in "$HOME/work-keys" deploy "test-passphrase"
	seed_vault deploy "test-passphrase"
	# The files OpenSSH keeps in a key directory for itself, and which
	# `key_patterns = ["*"]` matches by name exactly as it matches the key.
	: >"$HOME/work-keys/known_hosts"
	: >"$HOME/work-keys/authorized_keys"

	start_agent
	run bounded 30 "$SSHAKKU_BIN" load-keys
	[ "$status" -eq 0 ]

	run bounded 30 "$SSHAKKU_BIN" doctor
	[[ "$output" == *"keys in $HOME/work-keys (1)"* ]]
	[[ "$output" != *"known_hosts"* ]]
	[[ "$output" != *"authorized_keys"* ]]
	[[ "$output" != *"deploy.pub"* ]]

	run cat "$log_file"
	[[ "$output" != *"known_hosts"* ]]
	[[ "$output" != *"authorized_keys"* ]]
}

@test "F34: a key directory that is not there is said out loud, not read as an empty one" {
	use_config keys-in-a-directory-that-is-not-there.toml

	start_agent
	run bounded 30 "$SSHAKKU_BIN" load-keys
	[ "$status" -ne 0 ]
	[ "$status" -ne 124 ]
	[[ "$output" == *"no-such-directory"* ]]

	run cat "$log_file"
	[[ "$output" == *"no-such-directory"* ]]
}

@test "F35: the value in force is the drop-in's, beside the file that set it" {
	use_config key-lifetime-2h.toml
	use_dropin key-lifetime-3h-dropin.toml

	run config_line key_lifetime
	[[ "$output" == *"3h"* ]]
	[[ "$output" == *"config.d/50-work.toml"* ]]
	[[ "$output" != *"2h"* ]]

	# And the files it read, in the order it read them.
	run bounded 20 "$SSHAKKU_BIN" config
	[[ "$output" == *"config.toml"*"config.d/50-work.toml"* ]]
}

@test "F35: an exported variable takes the file's place, and is named in it" {
	use_config key-lifetime-2h.toml
	export SSHAKKU_KEY_LIFETIME=45m

	run config_line key_lifetime
	[[ "$output" == *"45m"* ]]
	[[ "$output" == *"SSHAKKU_KEY_LIFETIME"* ]]
	[[ "$output" != *"config.toml"* ]]
}

@test "F35: a value SSHakku refused is in the report, not only in the session log" {
	use_config key-lifetime-unreadable.toml

	run config_line key_lifetime
	[[ "$output" == *"half a day"* ]]
	# The default, standing in for what was refused.
	[[ "$output" == *"8h"* ]]
}

@test "F36: --edit opens that file and no other, and what is saved in it is what the report then says" {
	use_editor 'key_lifetime = "45m"'

	run bounded 20 "$SSHAKKU_BIN" config --edit
	[ "$status" -eq 0 ]

	run cat "$SSHAKKU_TEST_EDITOR_ARGV"
	[ "${#lines[@]}" -eq 1 ]
	[[ "$output" == *"/sshakku/config.toml" ]]

	# The file it was created from names every setting the report knows, so a
	# setting added without a line in the template is a template that has
	# stopped listing every setting.
	for setting in $(bounded 20 "$SSHAKKU_BIN" config | awk '/^  [a-z_]+ +/ {print $1}'); do
		grep -q "$setting" "$XDG_CONFIG_HOME/sshakku/config.toml" || {
			echo "the template SSHakku created names no $setting" >&2
			return 1
		}
	done

	run config_line key_lifetime
	[[ "$output" == *"45m"* ]]
	[[ "$output" == *"config.toml"* ]]
}

@test "F36: editing a key a drop-in decides says so before you walk away" {
	use_dropin key-lifetime-3h-dropin.toml
	use_editor 'key_lifetime = "2h"'

	run bounded 20 "$SSHAKKU_BIN" config --edit
	[ "$status" -eq 0 ]
	[[ "$output" == *"key_lifetime"* ]]
	[[ "$output" == *"50-work.toml"* ]]

	run config_line key_lifetime
	[[ "$output" == *"3h"* ]]
}

@test "F36: a value saved that SSHakku will not use is said on the way out" {
	use_editor 'key_lifetime = "half a day"'

	run bounded 20 "$SSHAKKU_BIN" config --edit
	[ "$status" -eq 0 ]
	[[ "$output" == *"half a day"* ]]
}

@test "F36: a file that no longer parses is said there and then, and named" {
	use_editor 'key_patterns = ["id_*"'

	run bounded 20 "$SSHAKKU_BIN" config --edit
	[ "$status" -eq 1 ]
	[[ "$output" == *"config.toml"* ]]
}
