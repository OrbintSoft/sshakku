#!/usr/bin/env bash
#
# Shared primitives for wiring a `source` line into a shell startup file:
# either a small executable wrapper dropped into an existing *.d/ drop-in
# directory, or a marker-delimited block idempotently upserted into a
# single file (created if absent) when no such directory exists. Used by
# both install-user-hook.sh (per-user profile/bashrc) and this repo's
# Makefile (system-wide /etc/profile.d and, optionally, a non-login bash
# rc drop-in or file).
#
# Sourced normally by another script. Also directly runnable, for callers
# (like a Makefile recipe) that have no other shell context to source it
# from:
#   shell-hook-lib.sh drop-in <drop_file> <source_line>
#   shell-hook-lib.sh remove-drop-in <drop_file>
#   shell-hook-lib.sh upsert-block <file> <source_line>
#   shell-hook-lib.sh strip-block <file>
#   shell-hook-lib.sh strip-block-file <file>
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
#
# Every other line is printed back exactly as it was read, carriage return
# included: a startup file mostly belongs to somebody else, and its line
# endings are not ours to convert. That is why the file is read here in the
# shell and not handed to awk, which on the systems that have a text mode opens
# it in one — taking the carriage return off a CRLF file's lines before the
# program can see it, and writing the file back with the other system's
# endings.
#
# A marker is recognised without that carriage return, so a profile saved with
# CRLF endings — or normalised to them by an editor after the install — is
# still unwired by the uninstall rather than reported as having no block.
strip_block() {
	local file="$1" line kept=() n=0 skip=0 cr=$'\r'
	[ -f "$file" ] || return 0
	while IFS= read -r line || [ -n "$line" ]; do
		case "${line%"$cr"}" in
		"$marker_start")
			skip=1
			continue
			;;
		"$marker_end")
			skip=0
			continue
			;;
		esac
		if [ "$skip" -eq 0 ]; then
			kept[n]="$line"
			n=$((n + 1))
		fi
	done <"$file"
	# A blank line of a CRLF file holds the carriage return and nothing else.
	while [ "$n" -gt 0 ] && [ -z "${kept[$((n - 1))]%"$cr"}" ]; do
		n=$((n - 1))
	done
	if [ "$n" -gt 0 ]; then
		printf '%s\n' "${kept[@]:0:n}"
	fi
}

# file_mode prints file's permission bits as octal digits, or nothing when
# there is no file there to have any. The flag that asks for them is not the
# same on the two systems this runs on and neither accepts the other's, so both
# are tried.
file_mode() {
	local file="$1"
	[ -e "$file" ] || return 0
	stat -c '%a' "$file" 2>/dev/null || stat -f '%Lp' "$file"
}

# upsert_block replaces any existing sshakku marker block in file with one
# wrapping source_line, appending it if none existed. Writes via a temp file
# in the same directory (so the final mv is an atomic same-filesystem
# rename) rather than editing file in place.
#
# The permissions are set explicitly, on the temp file, because that file is the
# one that ends up in place: mktemp creates it readable only by its owner and the
# rename carries that with it. A startup file keeps what it had, and one this
# created gets what a startup file normally has — a machine-wide file
# (/etc/bash.bashrc, /etc/zprofile) that came back owner-only would silently stop
# being read at every other account's login, taking its own contents with it.
upsert_block() {
	local file="$1" source_line="$2" tmp mode
	mode="$(file_mode "$file")"
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
	chmod "${mode:-644}" "$tmp"
	mv "$tmp" "$file"
}

# strip_block_file removes the sshakku block from file in place, leaving every
# other line of it — and the permissions it was found with — alone. A file that
# isn't there is not an error: nothing was wired into it, so there is nothing to
# unwire, and an uninstall has to run on a machine that was never installed.
#
# It writes through a temp file in the same directory for the reason upsert_block
# does, and sets the mode for the same reason: an uninstall that hands back a
# machine-wide startup file nobody but its owner can read has broken the file it
# was asked to repair.
strip_block_file() {
	local file="$1" tmp mode
	[ -f "$file" ] || return 0
	mode="$(file_mode "$file")"
	tmp="$(mktemp "${file}.XXXXXX")"
	strip_block "$file" >"$tmp"
	chmod "${mode:-644}" "$tmp"
	mv "$tmp" "$file"
}

# drop_in_hook writes source_line as a small executable wrapper into
# drop_file. drop_file's parent directory must already exist.
drop_in_hook() {
	local drop_file="$1" source_line="$2"
	{
		echo "#!/bin/bash"
		echo "# sshakku shell hook. Regenerate by re-running the sshakku install."
		echo "$source_line"
	} >"$drop_file"
	chmod 755 "$drop_file"
}

# remove_drop_in_hook removes drop_file; a no-op if it doesn't exist.
remove_drop_in_hook() {
	rm -f "$1"
}

# Dispatch only when executed directly, not when sourced.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	usage="usage: shell-hook-lib.sh {drop-in <drop_file> <source_line>|remove-drop-in <drop_file>|upsert-block <file> <source_line>|strip-block <file>|strip-block-file <file>}"
	action="${1:?$usage}"
	case "$action" in
	drop-in) drop_in_hook "${2:?$usage}" "${3:?$usage}" ;;
	remove-drop-in) remove_drop_in_hook "${2:?$usage}" ;;
	upsert-block) upsert_block "${2:?$usage}" "${3:?$usage}" ;;
	strip-block) strip_block "${2:?$usage}" ;;
	strip-block-file) strip_block_file "${2:?$usage}" ;;
	*)
		echo "shell-hook-lib.sh: unknown action '$action' ($usage)" >&2
		exit 2
		;;
	esac
fi
