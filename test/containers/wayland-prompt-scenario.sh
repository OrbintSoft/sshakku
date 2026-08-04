#!/bin/bash
# The passphrase prompt on a Wayland desktop, driven through the real binary in
# the session wayland-session.sh has already started. Derived from what SSHakku
# promises a user (docs/FEATURES.md, F29 and F37) and not from how it is built:
# where you are asked, in a window or on a terminal you may not be looking at,
# and what you are told when the first dialog cannot draw.
#
# The session it runs in has a screen and no controlling terminal, so a terminal
# prompt cannot stand in for a dialog that was supposed to appear, and a dialog
# that was supposed not to appear cannot hide behind one.
set -euo pipefail

readonly PASSPHRASE="the-passphrase"
readonly KEY="id_a"
readonly LOG="${HOME}/.local/state/sshakku/sessions.log"
readonly CONFIG="${HOME}/.config/sshakku/config.toml"

export GOCACHE=/tmp/gc
export GOPATH=/tmp/gp

failures=0

fail() {
	echo "FAIL: $*" >&2
	failures=$((failures + 1))
}

# windows lists what is on screen as "app_id :: title" — what a person sitting
# there would see. sway answers with the client's own name rather than an id it
# recycles between windows.
windows() {
	swaymsg -t get_tree | jq -r '.. | objects | select(.app_id != null) | .app_id + " :: " + (.name // "")'
}

# answer_dialogs types the passphrase into whatever window appears, the way a
# person answers the prompt they are shown, and records each one. The leading
# Shift_L is a keystroke with no effect that absorbs the one this session drops:
# there is no keyboard on the seat until wtype makes one, and the first event
# after that arrives while the client is still learning it has one.
answer_dialogs() {
	local seen_file="$1"
	: >"${seen_file}"
	while :; do
		if [ -n "$(windows)" ]; then
			windows >>"${seen_file}"
			wtype -d 40 -k Shift_L "${PASSPHRASE}" -k Return 2>/dev/null || true
			sleep 1.5
		fi
		sleep 0.3
	done
}

watcher=""

watch_windows() {
	answer_dialogs "$1" &
	watcher=$!
}

# stop_watching gives a late window time to appear before the counting stops: a
# dialog raised after load-keys returns is still one the user was shown.
stop_watching() {
	sleep 4
	kill "${watcher}" 2>/dev/null || true
	wait "${watcher}" 2>/dev/null || true
}

reset_state() {
	ssh-add -D >/dev/null 2>&1 || true
	rm -rf "${XDG_RUNTIME_DIR}/sshakku/giveup"
	rm -f "${LOG}" "${CONFIG}"
	pkill zenity 2>/dev/null || true
	pkill pinentry 2>/dev/null || true
	sleep 1
}

# shown echoes the distinct windows recorded in the given file.
shown() {
	sort -u "$1" | grep . || true
}

mkdir -p "${HOME}/.ssh" "${HOME}/.config/sshakku"
go build -o /tmp/bin/sshakku ./cmd/sshakku
ln -sf /tmp/bin/sshakku /tmp/bin/sshakku-askpass
export PATH="/tmp/bin:${PATH}"

ssh-keygen -t ed25519 -N "${PASSPHRASE}" -f "${HOME}/.ssh/${KEY}" -q

# agent_sock is one of the assignments `ensure-agent` prints for a shell to
# evaluate, which is how a login shell finds the agent too.
eval "$(sshakku ensure-agent)"
# shellcheck disable=SC2154
export SSH_AUTH_SOCK="${agent_sock}"

echo "session: WAYLAND_DISPLAY=${WAYLAND_DISPLAY} DISPLAY=${DISPLAY:-unset}"
echo "pinentry is really: $(readlink -f "$(command -v pinentry)")"

# 1. Nothing configured. This desktop's first dialog is a pinentry that
# announces a toolkit and then cannot draw; the promise is a window all the
# same, from the next dialog the desktop provides.
reset_state
watch_windows /tmp/seen-default
timeout 90 sshakku load-keys >/tmp/out-default 2>&1 || true
stop_watching

echo "--- nothing configured ---"
shown /tmp/seen-default
if [ "$(shown /tmp/seen-default | wc -l)" -ne 1 ]; then
	fail "the user was shown $(shown /tmp/seen-default | wc -l) window(s) with nothing configured, want exactly one"
fi
if ! shown /tmp/seen-default | grep -q "^zenity :: Enter passphrase for ${KEY}$"; then
	fail "no window asked for ${KEY}: a dialog that cannot draw took the question past one that can"
fi
if ! grep -q "pinentry could not ask for ${KEY}" "${LOG}"; then
	fail "the log does not name the dialog that could not ask"
fi
if ! grep -q "asking zenity instead" "${LOG}"; then
	fail "the log does not say where the question went instead"
fi

# 2. The dialog this desktop can draw, named. The control for the round above:
# it establishes that a window was always available here, so being asked
# anywhere else was a loss rather than an environment that could not be asked
# in.
reset_state
printf 'gui_prompter = "zenity"\n' >"${CONFIG}"
watch_windows /tmp/seen-zenity
timeout 90 sshakku load-keys >/tmp/out-zenity 2>&1 || true
stop_watching

echo "--- gui_prompter = zenity ---"
shown /tmp/seen-zenity
if ! shown /tmp/seen-zenity | grep -q "^zenity :: Enter passphrase for ${KEY}$"; then
	fail "the named dialog did not ask for ${KEY}"
fi

# 3. No dialog at all, on the same screen that has just shown one. With no
# terminal to fall back to, the promise is silence rather than a window.
reset_state
printf 'gui_prompter = "none"\n' >"${CONFIG}"
watch_windows /tmp/seen-none
timeout 90 sshakku load-keys >/tmp/out-none 2>&1 || true
stop_watching

echo "--- gui_prompter = none ---"
shown /tmp/seen-none
if [ -n "$(shown /tmp/seen-none)" ]; then
	fail "a window appeared with gui_prompter = none: $(shown /tmp/seen-none)"
fi
if ! grep -q "no terminal available to prompt for ${KEY}" "${LOG}"; then
	fail "nothing in the log says the question had nowhere to go"
fi

if [ "${failures}" -ne 0 ]; then
	echo "${failures} check(s) failed" >&2
	exit 1
fi
echo "every check passed"
