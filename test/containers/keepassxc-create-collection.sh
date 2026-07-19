#!/bin/bash
# Answers whatever KeePassXC's GUI is asking for, headlessly via xdotool, for
# the whole lifetime of the test command that follows — not just once before
# it. Unlike ksecretd (PAM-hash auto-unlock) and GNOME Keyring (blank-
# password auto-unlock), KeePassXC has no non-interactive re-unlock path at
# all: relocking the "sshakku" collection between uses (sshakku's own
# design) means every later unlock needs the same interactive password
# prompt as the very first one, so a one-shot dialog answerer isn't enough
# here. Unlike ksecretd/gnome-keyring-daemon, KeePassXC also has no
# standalone daemon: a Secret Service "collection" is an open database tab
# inside the full GUI app, and creating one over D-Bus (exactly what
# sshakku's own code does) opens the real multi-page "Create New Database"
# wizard the first time — name, master password, encryption settings, then
# a save-file dialog — rather than KDE/GNOME's single dialog. KeePassXC also
# has no non-interactive way to set a database's exposed group, so this
# wizard can't be replaced by a pre-seeded config file either. Must run from
# the module root (go.mod) with DISPLAY/D-Bus already up and KeePassXC's
# Secret Service integration enabled.
set -euo pipefail

readonly DB_PASSWORD="sshakku-keepassxc-desktop-stack-test-password"
readonly DB_FILE="${HOME}/Passwords.kdbx"

wait_for() {
	local description="$1" tries="$2"
	shift 2
	until "$@" >/dev/null 2>&1; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "keepassxc-create-collection: timed out waiting for ${description}" >&2
			return 1
		fi
		sleep 0.3
	done
}

# A prompt left unanswered past sshakku's own 30s D-Bus prompt timeout makes
# KeePassXC quit outright (no crash, no log output — just the last top-level
# window closing with nothing else open to keep the app alive), so a stuck
# attempt needs the app itself relaunched, not just the dialog re-driven.
#
# pkill/pgrep -x (exact match): this script's own /proc/*/comm, when it runs
# as a non-PID-1 child, is "keepassxc-creat" — the shebang-exec'd basename of
# this very file, truncated to 15 bytes by the kernel — which a substring
# match against "keepassxc" also matches, killing this script itself.
start_keepassxc() {
	pkill -x -u "$(id -u)" keepassxc 2>/dev/null || true
	sleep 0.3
	keepassxc >/tmp/keepassxc.log 2>&1 &
	wait_for "the KeePassXC window" 30 xdotool search --name KeePassXC
}

# First-ever run: no database exists yet, so CreateCollection opens the real
# "New Database" wizard. The default name/location are kept — the D-Bus
# Label property (set by sshakku's own code) is what actually names the
# collection, not this dialog's own name field.
drive_create_wizard() {
	xdotool mousemove --sync 619 458 click 1 # "Continue" on General Database Information
	sleep 0.6
	xdotool mousemove --sync 619 512 click 1 # "Continue" on Encryption Settings (defaults kept)
	sleep 0.6
	xdotool mousemove --sync 538 168 click 1 # "Enter password" field
	xdotool type --delay 20 "${DB_PASSWORD}"
	sleep 0.3
	xdotool mousemove --sync 538 206 click 1 # "Confirm password" field
	xdotool type --delay 20 "${DB_PASSWORD}"
	sleep 0.3
	xdotool mousemove --sync 631 512 click 1 # "Done"
	sleep 0.6
	xdotool mousemove --sync 412 275 click 1 # "Continue with weak password"
	sleep 0.6
	xdotool mousemove --sync 647 412 click 1 # "Save" on the file-save dialog (filename pre-filled)
	sleep 1
}

# Every later run: the database already exists (KeePassXC reopens it locked
# on its own at startup, and sshakku's own code locks it again after every
# use), so it only needs its password typed into the plain unlock screen.
drive_unlock() {
	xdotool mousemove --sync 400 298 click 1 # "Enter Password" field
	xdotool key ctrl+a                       # replace, don't append to, a previous failed attempt's leftover text
	xdotool type --delay 20 "${DB_PASSWORD}"
	xdotool mousemove --sync 558 399 click 1 # "Unlock"
	sleep 1
}

# The window title is "KeePassXC" while nothing is unlocked (welcome
# screen, mid-wizard, or waiting for the unlock password) and becomes
# "sshakku - KeePassXC" (the D-Bus Label sshakku's own code sets) once the
# collection is actually open — the one reliable, content-free signal this
# loop needs to tell "needs an answer" from "already answered".
unlocked() {
	xdotool search --name '^sshakku - KeePassXC$' >/dev/null 2>&1
}

# Runs for as long as the container does, answering every dialog KeePassXC
# raises — not just the first one — since sshakku itself may lock and
# re-unlock the collection many times across a single test run.
watch_loop() {
	while true; do
		if ! pgrep -x -u "$(id -u)" keepassxc >/dev/null; then
			start_keepassxc
		fi
		if ! unlocked; then
			if [ -f "${DB_FILE}" ]; then
				drive_unlock
			else
				drive_create_wizard
			fi
		fi
		sleep 0.5
	done
}

start_keepassxc
watch_loop &
disown

# Blocks until the "sshakku" collection exists and is usable, driving
# whatever dialog that needs via the watcher above — exactly the same
# round trip sshakku's own code performs in production. A silent no-op if
# the collection already exists and is unlocked (e.g. this script runs
# again against a warm app).
if ! SSHAKKU_TEST_ALLOW_REAL_SECRETSERVICE=1 go test ./internal/keys -run TestSecretServiceBackendRealDaemon -count=1 >/tmp/create-collection-trigger.log 2>&1; then
	echo "keepassxc-create-collection: trigger test failed:" >&2
	cat /tmp/create-collection-trigger.log >&2
	exit 1
fi
