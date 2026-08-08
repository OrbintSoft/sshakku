#!/bin/bash
# Makes the compartment the wallet keeps SSHakku's passphrases in, in a session
# with an X server: gnome-keyring-make-compartment.sh runs the command that
# makes one, and this answers the "Choose password for new keyring" / "Store
# passwords unencrypted?" pair GNOME Keyring raises in reply.
#
# Answered from the keyboard rather than by clicking a coordinate: the default
# button is wherever the dialog decides to draw it, and a click that lands
# somewhere else does not miss harmlessly, it dismisses the prompt and there is
# no second chance. Pressing Return on whichever of the pair is up is the same
# answer either way, so a press lost to render timing is simply repeated.
#
# Must run from the module root (go.mod) with DISPLAY/D-Bus/gnome-keyring-daemon
# already up.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

# shellcheck source=test/containers/gnome-keyring-make-compartment.sh
source "${SCRIPT_DIR}/gnome-keyring-make-compartment.sh"

start_making_the_compartment

for _ in 1 2 3 4 5 6; do
	sleep 2
	kill -0 "${compartment_pid}" 2>/dev/null || break
	window="$(xdotool search --name gcr-prompter 2>/dev/null | head -n1 || true)"
	if [ -n "${window}" ]; then
		xdotool windowactivate --sync "${window}" 2>/dev/null || true
		xdotool key --clearmodifiers Return || true
	fi
done

finish_making_the_compartment
