#!/usr/bin/env bash
#
# Wires or unwires sshakku's per-user login hook into a login shell's own
# profile, mirroring nn-ssh-init-linux.sh's system-wide role but never
# touching /etc. The hook logic itself is rendered once to
# <home>/.local/share/sshakku/shell-hook.sh (from nn-ssh-init-linux.sh, with
# the per-user binary path substituted in), so both wiring mechanisms below
# reduce to a single `source` line instead of duplicating the hook logic.
# The marker-block/drop-in file primitives themselves live in
# shell-hook-lib.sh, shared with this repo's Makefile for the equivalent
# system-wide wiring.
#
# Usage:
#   install-user-hook.sh install <home> <sshakku_bin_path> [nn] [wire_bashrc]
#   install-user-hook.sh uninstall <home> [nn]
#
# <home>/.bash_profile.d/, if it already exists, gets a small file dropped
# into it. Otherwise a marker-delimited block is idempotently upserted into
# <home>/.bash_profile (created if absent) — re-running install regenerates
# identical content, never duplicating the block; uninstall removes whichever
# of the two was actually used.
#
# The login-shell profile is the primary mechanism and is always wired. A
# login shell doesn't fire for a plain new terminal tab or a multiplexer
# pane started without re-logging in — most of those start a non-login
# shell instead, which reads .bashrc, not the profile. Passing a non-empty
# wire_bashrc additionally wires the same hook into <home>/.bashrc.d/ (if it
# exists) or <home>/.bashrc, the same fallback shape as the profile case, so
# those shells self-heal too. The hook itself is idempotent (a healthy fixed
# ssh-agent socket, and load-keys skips keys already present), so sourcing
# it twice in the same shell — e.g. a .bash_profile that itself sources
# .bashrc — is harmless.
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=shell-hook-lib.sh
. "$script_dir/shell-hook-lib.sh"

usage="usage: install-user-hook.sh {install <home> <sshakku_bin_path> [nn] [wire_bashrc]|uninstall <home> [nn]}"
action="${1:?$usage}"
home="${2:?$usage}"

hook_dir="$home/.local/share/sshakku"
hook_file="$hook_dir/shell-hook.sh"
profile_d_dir="$home/.bash_profile.d"
profile_file="$home/.bash_profile"
bashrc_d_dir="$home/.bashrc.d"
bashrc_file="$home/.bashrc"

# wire_hook drops source_line into dir/<nn>-sshakku-init.sh if dir exists,
# otherwise upserts it as a marker block into file (both via shell-hook-lib.sh
# primitives). Shared by the profile and (optional) bashrc wiring below —
# they only differ in which dir/file pair they target.
wire_hook() {
	local dir="$1" file="$2" nn="$3" source_line="$4"
	if [ -d "$dir" ]; then
		drop_in_hook "$dir/${nn}-sshakku-init.sh" "$source_line"
	else
		upsert_block "$file" "$source_line"
	fi
}

# unwire_hook removes dir/<nn>-sshakku-init.sh and strips the marker block
# from file, whichever was actually used — safe to call even if wire_hook
# was never run for this dir/file pair.
unwire_hook() {
	local dir="$1" file="$2" nn="$3"
	remove_drop_in_hook "$dir/${nn}-sshakku-init.sh"
	if [ -f "$file" ]; then
		local tmp
		tmp="$(mktemp "${file}.XXXXXX")"
		strip_block "$file" >"$tmp"
		mv "$tmp" "$file"
	fi
}

case "$action" in
install)
	sshakku_bin="${3:?$usage}"
	nn="${4:-001}"
	wire_bashrc="${5:-}"

	mkdir -p "$hook_dir"
	template_dir="$(cd "$(dirname "$0")" && pwd)"
	sed 's|/usr/local/bin/sshakku|'"$sshakku_bin"'|g' "$template_dir/nn-ssh-init-linux.sh" >"$hook_file"
	chmod 755 "$hook_file"

	source_line=". \"$hook_file\""
	wire_hook "$profile_d_dir" "$profile_file" "$nn" "$source_line"
	if [ -n "$wire_bashrc" ]; then
		wire_hook "$bashrc_d_dir" "$bashrc_file" "$nn" "$source_line"
	fi
	;;
uninstall)
	nn="${3:-001}"

	unwire_hook "$profile_d_dir" "$profile_file" "$nn"
	unwire_hook "$bashrc_d_dir" "$bashrc_file" "$nn"
	rm -f "$hook_file"
	rmdir "$hook_dir" 2>/dev/null || true
	;;
*)
	echo "install-user-hook.sh: unknown action '$action' ($usage)" >&2
	exit 2
	;;
esac
