#!/bin/bash
# Answers whatever KeePassXC's GUI is asking for, headlessly on Wayland, for the
# whole lifetime of the test command that follows — not just once before it. The
# X11 script does the same job with xdotool. Must run from the module root
# (go.mod) with the compositor and D-Bus already up and KeePassXC's Secret
# Service integration enabled.
#
# Why a watcher rather than a one-time answer: KeePassXC has no non-interactive
# re-unlock path, and sshakku's own design relocks the collection between uses,
# so every later unlock needs the same interactive prompt as the first.
#
# What this session can read that an X11 one cannot: the compositor names every
# window, so each thing the app asks is recognised by what it says it is —
# "Create a new KeePassXC database…", "Save database as", "No password set" —
# instead of by how long the last click took to land. Buttons are still pressed
# by position, but relative to the window that owns them, anchored to its
# bottom-right corner where this app keeps them on every page.
set -euo pipefail

readonly DB_PASSWORD="sshakku-keepassxc-desktop-stack-test-password"
readonly APP_ID="org.keepassxc.KeePassXC"
readonly WIZARD_TITLE="Create a new KeePassXC database…"
readonly SAVE_TITLE="Save database as"
readonly UNLOCKED_TITLE="sshakku - KeePassXC"

# Where the primary button sits, measured from the bottom-right corner of the
# window that carries it: the wizard keeps Continue and Done there on every
# page, whatever the page's height.
readonly BUTTON_RIGHT=169
readonly BUTTON_BOTTOM=33

# The first page of the wizard is shorter than the ones after it, which is how
# "the name page" is told from "the page after it" without reading any text.
readonly FIRST_PAGE_HEIGHT=461

focused_field() {
	swaymsg -t get_tree |
		jq -r --arg app "${APP_ID}" \
			'.. | objects | select(.focused == true and .app_id == $app) | "\(.name)\t\(.rect.x)\t\(.rect.y)\t\(.rect.width)\t\(.rect.height)"' |
		head -1 | cut -f"$1"
}

focused_title() { focused_field 1; }
focused_height() { focused_field 5; }

any_window() {
	[ -n "$(swaymsg -t get_tree | jq -r --arg app "${APP_ID}" '.. | objects | select(.app_id == $app) | .name' | head -1)" ]
}

unlocked() {
	swaymsg -t get_tree | jq -r '.. | objects | select(.app_id != null) | .name // ""' | grep -qx "${UNLOCKED_TITLE}"
}

app_running() { pgrep -x -u "$(id -u)" keepassxc >/dev/null; }

# wait_for_title returns once the focused window says it is the given thing.
wait_for_title() {
	local want="$1" tries="$2"
	until [ "$(focused_title)" = "${want}" ]; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			return 1
		fi
		sleep 0.5
	done
}

click_at() {
	local x="$1" y="$2" rx ry
	rx="$(focused_field 2)"
	ry="$(focused_field 3)"
	[ -n "${rx}" ] || return 1
	swaymsg seat - cursor set "$((rx + x))" "$((ry + y))" >/dev/null
	swaymsg seat - cursor press button1 >/dev/null
	sleep 0.2
	swaymsg seat - cursor release button1 >/dev/null
}

# click_primary presses the button in the bottom-right corner of the focused
# window — Continue, Done, whichever this page calls it.
click_primary() {
	click_at "$(($(focused_field 4) - BUTTON_RIGHT))" "$(($(focused_field 5) - BUTTON_BOTTOM))"
}

# type_text types into whatever holds focus. The leading Shift_L is a keystroke
# with no effect that absorbs the one this session drops: the keyboard exists
# only while wtype runs, and the first event arrives while the client is still
# learning it has one.
type_text() {
	wtype -d 30 -k Shift_L "$@" 2>/dev/null || true
}

start_keepassxc() {
	pkill -x -u "$(id -u)" keepassxc 2>/dev/null || true
	sleep 0.5
	keepassxc >/tmp/keepassxc.log 2>&1 &
	local tries=40
	until any_window; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "keepassxc-wayland-create-collection: KeePassXC drew no window" >&2
			return 1
		fi
		sleep 0.5
	done
}

# The first-ever run: no database exists, so CreateCollection opens the real
# "New Database" wizard. The default name and location are kept — the D-Bus
# Label property, set by sshakku's own code, is what names the collection.
drive_create_wizard() {
	wait_for_title "${WIZARD_TITLE}" 40 || return 1

	# The name page, then the encryption page: both are answered by their own
	# primary button, and the second is known to have arrived because the wizard
	# grows when it does.
	click_primary
	local tries=20
	while [ "$(focused_height)" = "${FIRST_PAGE_HEIGHT}" ] && [ "${tries}" -gt 0 ]; do
		tries=$((tries - 1))
		sleep 0.5
	done
	click_primary
	sleep 2.5

	# The credentials page puts the cursor in the password field itself, so the
	# password is typed where the page is already asking for it rather than at a
	# coordinate that would land on a different widget one page earlier.
	type_text "${DB_PASSWORD}" -k Tab "${DB_PASSWORD}"
	sleep 1
	click_primary

	# The file dialog arrives with the name filled in and Save as its default.
	wait_for_title "${SAVE_TITLE}" 30 || return 1
	type_text -k Return
	sleep 2
}

# Every later run: the database exists and is locked, so it only needs its
# password typed into the unlock screen, which focuses its own field.
drive_unlock() {
	type_text "${DB_PASSWORD}" -k Return
	sleep 1.5
}

watch_loop() {
	while true; do
		if ! app_running; then
			start_keepassxc || true
		elif ! unlocked; then
			if [ "$(focused_title)" = "${WIZARD_TITLE}" ]; then
				drive_create_wizard || true
			else
				drive_unlock
			fi
		fi
		sleep 0.5
	done
}

start_keepassxc
watch_loop &
disown

# Blocks until the "sshakku" collection exists and is usable, driving whatever
# dialog that needs through the watcher above — the same round trip sshakku's
# own code performs in production.
if ! SSHAKKU_TEST_ALLOW_REAL_SECRETSERVICE=1 go test ./internal/keys -run TestSecretServiceBackendRealDaemon -count=1 >/tmp/create-collection-trigger.log 2>&1; then
	echo "keepassxc-wayland-create-collection: trigger test failed:" >&2
	cat /tmp/create-collection-trigger.log >&2
	exit 1
fi
