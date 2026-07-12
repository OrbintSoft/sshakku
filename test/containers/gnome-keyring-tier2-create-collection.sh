#!/bin/bash
# Drives the one-time "Choose password for new keyring" / "Store passwords
# unencrypted?" gcr-prompter dialog pair headlessly via xdotool, answering
# with a blank password. Unlike KDE's ksecretd, GNOME Keyring has no config
# file that lets an unattended container pre-seed a custom collection alias
# (see phase4.2-gnome-keyring-tier2-steps.md); a blank password is also what
# makes every later Unlock() call prompt-free, not just this one-time
# creation. Must run from the module root (go.mod) with DISPLAY/D-Bus/
# gnome-keyring-daemon already up.
set -euo pipefail

wait_for() {
	local description="$1" tries="$2"
	shift 2
	until "$@" >/dev/null 2>&1; do
		tries=$((tries - 1))
		if [ "${tries}" -le 0 ]; then
			echo "gnome-keyring-tier2-create-collection: timed out waiting for ${description}" >&2
			return 1
		fi
		sleep 0.3
	done
}

# Triggers CreateCollection for the "sshakku" collection exactly the way
# sshakku's own code does, so gcr-prompter renders the real dialog this
# script answers. A silent no-op if the collection already exists (e.g. this
# script runs again against a warm daemon).
SSHAKKU_TEST_ALLOW_REAL_SECRETSERVICE=1 go test ./internal/keys -run TestSecretServiceBackendRealDaemon -count=1 >/tmp/create-collection-trigger.log 2>&1 &
readonly TRIGGER_PID=$!

# The dialog pair only ever needs answering once, and each attempt is
# idempotent (clicking after the daemon already resolved the prompt is a
# no-op) — a missed click from render-timing jitter is retried rather than
# failing outright. This is a much smaller flakiness surface than driving
# every unlock, the approach already rejected for the KDE tier-2 row: here
# it is one bounded, retryable step, not one per operation.
for _ in 1 2 3 4 5; do
	if ! kill -0 "${TRIGGER_PID}" 2>/dev/null; then
		break
	fi
	if wait_for "the new-keyring password dialog" 15 xdotool search --name gcr-prompter; then
		sleep 0.5
		xdotool mousemove --sync 700 174 click 1 # "Continue" on "Choose password for new keyring", blank password
		sleep 1
		xdotool mousemove --sync 975 96 click 1 # "Continue" on "Store passwords unencrypted?"
	fi
	sleep 2
done

if ! wait "${TRIGGER_PID}"; then
	echo "gnome-keyring-tier2-create-collection: trigger test failed:" >&2
	cat /tmp/create-collection-trigger.log >&2
	exit 1
fi
