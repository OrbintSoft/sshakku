#!/bin/bash
# Makes the compartment the wallet keeps SSHakku's passphrases in, in a session
# with a Wayland compositor and no X server: gnome-keyring-make-compartment.sh
# runs the command that makes one, and this answers the "Choose password for new
# keyring" / "Store passwords unencrypted?" pair GNOME Keyring raises in reply,
# with a blank password.
#
# The dialog cannot be answered from the keyboard here: it grabs the seat, and
# every key sent to it is ignored while it holds focus. It is clicked instead,
# through the compositor's own cursor — which exists because the session was
# given a pointer device (see wayland-pointer.sh).
#
# Where each "Continue" sits is recorded relative to the dialog's own window, and
# the compositor is asked where it put that window rather than told, so a dialog
# placed somewhere else is still pressed where its button is.
#
# Must run from the module root (go.mod) with the compositor, D-Bus and
# gnome-keyring-daemon already up.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# shellcheck source=test/containers/gnome-keyring-make-compartment.sh
source "${SCRIPT_DIR}/gnome-keyring-make-compartment.sh"

# Where "Continue" sits in each of the two dialogs, relative to its own window.
readonly PASSWORD_CONTINUE_X=700
readonly PASSWORD_CONTINUE_Y=174
readonly UNENCRYPTED_CONTINUE_X=975
readonly UNENCRYPTED_CONTINUE_Y=96

wait_for() {
	local description="$1" tries="$2"
	shift 2
	until "$@" >/dev/null 2>&1; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "gnome-keyring-wayland-create-collection: timed out waiting for ${description}" >&2
			return 1
		fi
		sleep 0.3
	done
}

# dialog_rect echoes "x y width height" for the window on screen, or nothing
# when there is none. sway answers with the window the client actually has,
# which is how a second dialog is told from the first: they differ in size.
dialog_rect() {
	swaymsg -t get_tree |
		jq -r '.. | objects | select(.app_id != null) | "\(.rect.x) \(.rect.y) \(.rect.width) \(.rect.height)"' |
		head -1
}

dialog_on_screen() {
	[ -n "$(dialog_rect)" ]
}

# click_in presses where the given point falls inside the window on screen.
click_in() {
	local rx ry
	read -r rx ry _ _ <<<"$(dialog_rect)"
	[ -n "${rx}" ] || return 1
	swaymsg seat - cursor set "$((rx + $1))" "$((ry + $2))" >/dev/null
	swaymsg seat - cursor press button1 >/dev/null
	sleep 0.2
	swaymsg seat - cursor release button1 >/dev/null
}

start_making_the_compartment

# Each attempt is idempotent — pressing after the daemon already resolved the
# prompt is a no-op — so a press lost to render-timing jitter is retried rather
# than failing outright.
for _ in 1 2 3 4 5; do
	kill -0 "${compartment_pid}" 2>/dev/null || break
	if wait_for "the new-keyring password dialog" 40 dialog_on_screen; then
		sleep 0.5
		click_in "${PASSWORD_CONTINUE_X}" "${PASSWORD_CONTINUE_Y}" # blank password
		sleep 1
		if dialog_on_screen; then
			click_in "${UNENCRYPTED_CONTINUE_X}" "${UNENCRYPTED_CONTINUE_Y}"
		fi
	fi
	sleep 2
done

finish_making_the_compartment
