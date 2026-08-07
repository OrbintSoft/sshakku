#!/bin/bash
# What `sshakku doctor` tells a user about the wallet it would use. Derived from
# what SSHakku promises (docs/FEATURES.md, F25 and F41) and not from how the
# report is built: whether a wallet that cannot hold anything here is said to be
# unusable, whether a wallet that is not there at all is noticed, and whether the
# report is the harmless, prompt look it is promised to be.
#
# It runs in the session gnome-keyring-headless-session.sh starts, which has no
# display of any kind, so the compartment SSHakku would store in cannot be
# created here. The container is expected to be started with
# SSHAKKU_DISABLE_SECRETS_ACTIVATION set, so that a bus of this test's own has no
# wallet on it and none that could be started.
#
# The words asserted on below — compartment, screen, desktop session — are the
# ones the promises themselves use, not ones read off the implementation.
set -uo pipefail

readonly REPORT_BUDGET=15

export GOCACHE=/tmp/gc
export GOPATH=/tmp/gp

failures=0

fail() {
	echo "FAIL: $*" >&2
	failures=$((failures + 1))
}

ok() {
	echo "ok: $*"
}

# collections is what the wallet holds, asked of it directly rather than through
# SSHakku, so "the report changed nothing" is judged from outside the thing being
# judged.
collections() {
	dbus-send --session --print-reply --dest=org.freedesktop.secrets \
		/org/freedesktop/secrets org.freedesktop.DBus.Properties.Get \
		string:org.freedesktop.Secret.Service string:Collections 2>/dev/null |
		grep 'object path' | sort
}

# findings is the part of the report a user reads to learn what is wrong.
findings() {
	awk '/^findings:/ { inside = 1; next } inside { print }'
}

keyring_pid() {
	local daemon_path dir
	daemon_path="$(command -v gnome-keyring-daemon)"
	for dir in /proc/[0-9]*; do
		if [ "$(readlink "${dir}/exe" 2>/dev/null)" = "${daemon_path}" ]; then
			echo "${dir##*/}"
			return 0
		fi
	done
	return 1
}

go build -o /tmp/build/sshakku /src/cmd/sshakku || {
	echo "could not build sshakku" >&2
	exit 1
}
readonly SSHAKKU=/tmp/build/sshakku

# ── F25: a wallet that cannot hold anything here says so ───────────────────
#
# The session has no screen, so the compartment cannot be created: no passphrase
# can ever be saved. The report has to say that, rather than leave it to be
# discovered the next time ssh asks.
before="$(collections)"
report="$(timeout 60 "${SSHAKKU}" doctor 2>&1)"
echo "${report}"

if grep -q 'secret-service' <<<"${report}"; then
	ok "the report names the wallet it would use"
else
	fail "the report does not name the wallet"
fi

if grep -qi 'compartment' <<<"${report}"; then
	ok "the report says something about the compartment"
else
	fail "the report says nothing about the compartment SSHakku would store in"
fi

if findings <<<"${report}" | grep -qi 'compartment'; then
	ok "the missing piece is stated where a user looks for problems"
else
	fail "nothing among the findings says the wallet cannot hold anything here"
fi

if findings <<<"${report}" | grep -qiE 'screen|desktop session'; then
	ok "the report says what it would take to have a compartment"
else
	fail "the report does not say the compartment needs a screen it has not got"
fi

# ── F41: the report changed nothing ────────────────────────────────────────
after="$(collections)"
if [ "${before}" = "${after}" ]; then
	ok "the wallet holds exactly what it held before the report"
else
	fail "the report changed what the wallet holds"
	printf 'before:\n%s\nafter:\n%s\n' "${before}" "${after}" >&2
fi

# ── F41: a bus with no wallet on it, and none that could be started ────────
bus_output="$(dbus-daemon --session --fork --print-address=1 --print-pid=1)"
second_bus="$(head -n1 <<<"${bus_output}")"
second_bus_pid="$(sed -n 2p <<<"${bus_output}")"

empty_report="$(DBUS_SESSION_BUS_ADDRESS="${second_bus}" timeout 60 "${SSHAKKU}" doctor 2>&1)"
empty_status=$?

if [ "${empty_status}" -eq 0 ] && grep -q '^findings:' <<<"${empty_report}"; then
	ok "a report still comes when there is no wallet to look at"
else
	fail "no report came back from a session whose bus has no wallet on it"
fi

# Named, not merely mentioned: the word "wallet" appears among the findings for
# reasons that have nothing to do with this one, so the wallet's own name is what
# says a finding is about it.
if findings <<<"${empty_report}" | grep -q 'secret-service'; then
	ok "the report says there is no wallet answering"
else
	fail "a bus with no wallet on it is reported as though the wallet were fine"
	echo "${empty_report}" >&2
fi

kill "${second_bus_pid}" 2>/dev/null

# ── F41: a wallet that stopped answering does not hold the report up ───────
#
# Stopped, not killed: the daemon still owns its name on the bus and still
# accepts calls, it just never replies — which is the case that waits forever
# when nothing bounds it.
if pid="$(keyring_pid)"; then
	kill -STOP "${pid}"
	# shellcheck disable=SC2064  # the pid is wanted now, not when the trap runs
	trap "kill -CONT ${pid} 2>/dev/null" EXIT

	started="$(date +%s)"
	frozen_report="$(timeout 120 "${SSHAKKU}" doctor 2>&1)"
	frozen_status=$?
	elapsed=$(($(date +%s) - started))

	kill -CONT "${pid}"
	trap - EXIT

	if [ "${frozen_status}" -eq 124 ]; then
		fail "the report never came back from a wallet that stopped answering"
	elif [ "${elapsed}" -gt "${REPORT_BUDGET}" ]; then
		fail "the report took ${elapsed}s against a wallet that stopped answering, past the ${REPORT_BUDGET}s a look may take"
	else
		ok "the report came back in ${elapsed}s from a wallet that stopped answering"
	fi

	if grep -q '^findings:' <<<"${frozen_report}"; then
		ok "the report is whole even when the wallet could not be asked"
	else
		fail "the report was cut short by a wallet that stopped answering"
		echo "${frozen_report}" >&2
	fi
else
	fail "no gnome-keyring-daemon to stop; the frozen-wallet case was not exercised"
fi

if [ "${failures}" -ne 0 ]; then
	echo "${failures} failure(s)" >&2
	exit 1
fi
echo "all good"
