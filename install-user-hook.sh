#!/usr/bin/env bash
#
# Wires or unwires sshakku's per-user login hook into a login shell's own
# profile, mirroring nn-ssh-init-linux.sh's system-wide role but never
# touching /etc. The hook logic itself is rendered once to
# <home>/.local/share/sshakku/shell-hook.sh (from nn-ssh-init-linux.sh, with
# the per-user binary path substituted in), so both wiring mechanisms below
# reduce to a single `source` line instead of duplicating the hook logic.
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

marker_start="# >>> sshakku >>>"
marker_end="# <<< sshakku <<<"

# strip_block prints file with any existing sshakku marker block, and any
# trailing blank lines left behind by an earlier upsert_block's own
# separator, removed. A missing file prints nothing — the caller decides
# whether that's fine. Trimming trailing blanks here (rather than leaving
# them for upsert_block to reason about) is what makes re-running install
# byte-for-byte idempotent: without it, each re-run would leave one more
# blank line than the last.
strip_block() {
	local file="$1"
	[ -f "$file" ] || return 0
	awk -v start="$marker_start" -v end="$marker_end" '
		$0 == start { skip = 1; next }
		$0 == end   { skip = 0; next }
		!skip       { lines[++n] = $0 }
		END {
			while (n > 0 && lines[n] == "") n--
			for (i = 1; i <= n; i++) print lines[i]
		}
	' "$file"
}

# upsert_block replaces any existing sshakku marker block in file with one
# wrapping source_line, appending it if none existed. Writes via a temp file
# in the same directory (so the final mv is an atomic same-filesystem
# rename) rather than editing file in place.
upsert_block() {
	local file="$1" source_line="$2" tmp
	tmp="$(mktemp "${file}.XXXXXX")"
	strip_block "$file" >"$tmp"
	# A blank separator line only when something preceded the block — a
	# brand-new file starts straight with the marker. Checked as its own
	# statement, after the write above has already completed, so nothing
	# reads and writes tmp within the same pipeline.
	if [ -s "$tmp" ]; then
		printf '\n' >>"$tmp"
	fi
	{
		echo "$marker_start"
		echo "$source_line"
		echo "$marker_end"
	} >>"$tmp"
	mv "$tmp" "$file"
}

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
# otherwise upserts it as a marker block into file. Shared by the profile
# and (optional) bashrc wiring below — they only differ in which dir/file
# pair they target.
wire_hook() {
	local dir="$1" file="$2" nn="$3" source_line="$4"
	local drop_file="$dir/${nn}-sshakku-init.sh"
	if [ -d "$dir" ]; then
		{
			echo "#!/bin/bash"
			echo "# sshakku per-user login hook. Regenerate with: make install-user"
			echo "$source_line"
		} >"$drop_file"
		chmod 755 "$drop_file"
	else
		upsert_block "$file" "$source_line"
	fi
}

# unwire_hook removes dir/<nn>-sshakku-init.sh and strips the marker block
# from file, whichever was actually used — safe to call even if wire_hook
# was never run for this dir/file pair.
unwire_hook() {
	local dir="$1" file="$2" nn="$3"
	rm -f "$dir/${nn}-sshakku-init.sh"
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
