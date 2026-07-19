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
	usage="usage: shell-hook-lib.sh {drop-in <drop_file> <source_line>|remove-drop-in <drop_file>|upsert-block <file> <source_line>|strip-block <file>}"
	action="${1:?$usage}"
	case "$action" in
	drop-in) drop_in_hook "${2:?$usage}" "${3:?$usage}" ;;
	remove-drop-in) remove_drop_in_hook "${2:?$usage}" ;;
	upsert-block) upsert_block "${2:?$usage}" "${3:?$usage}" ;;
	strip-block) strip_block "${2:?$usage}" ;;
	*)
		echo "shell-hook-lib.sh: unknown action '$action' ($usage)" >&2
		exit 2
		;;
	esac
fi
