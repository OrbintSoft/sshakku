#!/bin/bash
# Runs a command against a GNOME Keyring the user's own machine has no screen
# for, which takes two logins rather than one: the compartment SSHakku keeps its
# entries in is made through a dialog, so it is made in a login that has a
# screen, and the command then runs in a login that has none. What the two share
# is the account's home — the wallet on disk — and nothing else.
#
# Usage: run-headless-wallet.sh <image> <command...>
set -euo pipefail

if [ "$#" -lt 2 ]; then
	echo "usage: $0 <image> <command...>" >&2
	exit 2
fi

readonly IMAGE="$1"
shift

HOME_VOLUME="sshakku-headless-home-$$"
readonly HOME_VOLUME

cleanup() {
	docker volume rm -f "${HOME_VOLUME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker volume create "${HOME_VOLUME}" >/dev/null

# The login with a screen. It is asked for nothing here: bringing the session up
# is what makes the compartment, and the command is a no-op on purpose, so that
# nothing this step does can be mistaken for what the headless one below shows.
docker run --init --rm \
	-v "${PWD}":/src:ro \
	-v "${HOME_VOLUME}":/home \
	"${IMAGE}" true

# Not exec'd: the volume above is this script's to remove, and an exec would
# leave the trap with nothing to run in.
docker run --init --rm \
	-v "${PWD}":/src:ro \
	-v "${HOME_VOLUME}":/home \
	-e SSHAKKU_SESSION_SCRIPT=gnome-keyring-headless-session.sh \
	"${IMAGE}" "$@"
