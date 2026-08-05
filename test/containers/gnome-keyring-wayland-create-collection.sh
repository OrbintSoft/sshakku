#!/bin/bash
# Drives the one-time "Choose password for new keyring" / "Store passwords
# unencrypted?" gcr-prompter dialog pair headlessly on Wayland, answering with
# a blank password. The X11 script does the same job with xdotool; the two
# differ only in how a button is pressed. Must run from the module root (go.mod)
# with the compositor, D-Bus and gnome-keyring-daemon already up.
#
# The dialog cannot be answered from the keyboard here: it grabs the seat, and
# every key sent to it is ignored while it holds focus. It is clicked instead,
# through the compositor's own cursor — which exists because the session was
# given a pointer device (see wayland-pointer.sh).
#
# The two points clicked are the ones the X11 script clicks, and they are the
# same points because they are read the same way: on X11 the dialog is placed at
# the origin, so the coordinates there are already relative to the window. Here
# they are added to wherever the compositor put it, which it is asked for rather
# than told.
set -euo pipefail

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

# Triggers CreateCollection for the "sshakku" collection exactly the way
# sshakku's own code does, so gcr-prompter renders the real dialog this script
# answers. A silent no-op if the collection already exists.
SSHAKKU_TEST_ALLOW_REAL_SECRETSERVICE=1 go test ./internal/keys -run TestSecretServiceBackendRealDaemon -count=1 >/tmp/create-collection-trigger.log 2>&1 &
readonly TRIGGER_PID=$!

# Each attempt is idempotent — clicking after the daemon already resolved the
# prompt is a no-op — so a missed press from render-timing jitter is retried
# rather than failing outright.
for _ in 1 2 3 4 5; do
	if ! kill -0 "${TRIGGER_PID}" 2>/dev/null; then
		break
	fi
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

if ! wait "${TRIGGER_PID}"; then
	echo "gnome-keyring-wayland-create-collection: trigger test failed:" >&2
	cat /tmp/create-collection-trigger.log >&2
	exit 1
fi
