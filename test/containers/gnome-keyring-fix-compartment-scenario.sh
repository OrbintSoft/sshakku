#!/bin/bash
# What `sshakku doctor --fix` does about the compartment the wallet keeps
# SSHakku's passphrases in. Derived from what SSHakku promises (docs/FEATURES.md,
# F42) and not from how the repair is built: whether the report offers the
# repair, whether --fix performs it and says what it made, whether the report
# that follows shows the result, and whether a session that cannot make one is
# told so and left alone.
#
# It runs in either kind of session and asserts the half that session is about,
# deciding from the screen it has rather than from an argument, so a run that
# lost its display cannot quietly assert the easier half.
#
# The compartment is made absent by configuring a name nothing has made, so
# everything around it — wallet, prompter, screen — is the session's own. The
# words asserted on below are the ones the promise uses.
set -uo pipefail

readonly COMPARTMENT="doctor-fix-scenario"

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
# SSHakku, so "the wallet was left as it was found" is judged from outside the
# thing being judged.
collections() {
	dbus-send --session --print-reply --dest=org.freedesktop.secrets \
		/org/freedesktop/secrets org.freedesktop.DBus.Properties.Get \
		string:org.freedesktop.Secret.Service string:Collections 2>/dev/null |
		grep 'object path' | sort
}

# fix_section is what --fix said about its own work, and after_report is the
# report it printed once it had finished — the two halves a user reads to learn
# what was done and what the machine looks like now.
fix_section() {
	awk '/applying self-heal/ { inside = 1; next } /^after:/ { inside = 0 } inside { print }'
}

after_report() {
	awk '/^after:/ { inside = 1; next } inside { print }'
}

# answer_the_wallets_dialog accepts the "Choose password for new keyring" and
# "Store passwords unencrypted?" pair with a blank password — which is also what
# makes every later unlock of that compartment prompt-free.
#
# Answered from the keyboard rather than by clicking a coordinate: the default
# button is wherever the dialog decides to draw it, and a click that lands
# somewhere else does not miss harmlessly, it dismisses the prompt and there is
# no second chance. Pressing Return on whichever of the pair is up is the same
# answer either way, so a press lost to render timing is simply repeated.
answer_the_wallets_dialog() {
	local pid="$1" window
	for _ in 1 2 3 4 5 6; do
		sleep 2
		kill -0 "${pid}" 2>/dev/null || return 0
		window="$(xdotool search --name gcr-prompter 2>/dev/null | head -n1)"
		if [ -n "${window}" ]; then
			xdotool windowactivate --sync "${window}" 2>/dev/null
			xdotool key --clearmodifiers Return
		fi
	done
}

go build -o /tmp/build/sshakku /src/cmd/sshakku || {
	echo "could not build sshakku" >&2
	exit 1
}
readonly SSHAKKU=/tmp/build/sshakku

mkdir -p "${HOME}/.config/sshakku"
echo "secret_container = \"${COMPARTMENT}\"" >"${HOME}/.config/sshakku/config.toml"

before="$(collections)"
report="$(timeout 60 "${SSHAKKU}" doctor 2>&1)"

if [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; then
	# ── F42, the session that can make one ─────────────────────────────
	if grep -q -- '--fix' <<<"${report}"; then
		ok "the report names --fix as what makes the compartment"
	else
		fail "the report never offers to make the compartment"
		echo "${report}" >&2
	fi

	timeout 180 "${SSHAKKU}" doctor --fix >/tmp/fix.out 2>&1 &
	fix_pid=$!
	answer_the_wallets_dialog "${fix_pid}"
	wait "${fix_pid}"
	fix_status=$?
	fixed="$(cat /tmp/fix.out)"

	if [ "${fix_status}" -eq 0 ]; then
		ok "--fix came back having done its work"
	else
		fail "--fix exited ${fix_status}"
		echo "${fixed}" >&2
	fi

	if fix_section <<<"${fixed}" | grep -q "${COMPARTMENT}"; then
		ok "--fix says what it made"
	else
		fail "--fix does not say what it made"
		echo "${fixed}" >&2
	fi

	# The report that follows is what F14 judges a repair by, so it has to
	# both exist and show the compartment as no longer missing.
	after="$(after_report <<<"${fixed}")"
	if grep -q 'compartment' <<<"${after}"; then
		ok "the report after --fix says something about the compartment"
	else
		fail "the report after --fix has nothing about the wallet in it"
		echo "${fixed}" >&2
	fi
	if grep -E "compartment: *found — ${COMPARTMENT}" <<<"${after}" >/dev/null; then
		ok "the report after --fix lists the compartment as present"
	else
		fail "the compartment is not reported as present after being made"
		echo "${after}" >&2
	fi

	# The wallet itself, not the report about it.
	if [ "$(collections)" = "${before}" ]; then
		fail "--fix reported success but the wallet holds what it held before"
	else
		ok "the wallet holds a compartment it did not hold before"
	fi
else
	# ── F42, the session that cannot ───────────────────────────────────
	fixed="$(timeout 180 "${SSHAKKU}" doctor --fix 2>&1)"
	fix_status=$?

	if [ "${fix_status}" -eq 124 ]; then
		fail "--fix never came back in a session with no screen to draw a dialog on"
	else
		ok "--fix came back rather than waiting on a dialog nobody can answer"
	fi

	fix_said="$(fix_section <<<"${fixed}")"
	if grep -q 'compartment' <<<"${fix_said}"; then
		ok "--fix says it did not make the compartment"
	else
		fail "--fix says nothing about the compartment it could not make"
		echo "${fixed}" >&2
	fi
	if grep -qiE 'screen|desktop session' <<<"${fix_said}"; then
		ok "--fix says what making the compartment would take"
	else
		fail "--fix does not say what it would take"
		echo "${fix_said}" >&2
	fi

	if after_report <<<"${fixed}" | grep -q 'compartment'; then
		ok "the report still comes back, wallet and all"
	else
		fail "no wallet section in the report after --fix"
		echo "${fixed}" >&2
	fi

	if [ "$(collections)" = "${before}" ]; then
		ok "the wallet holds exactly what it held before --fix"
	else
		fail "--fix could not make the compartment yet the wallet changed"
		printf 'before:\n%s\nafter:\n%s\n' "${before}" "$(collections)" >&2
	fi
fi

if [ "${failures}" -ne 0 ]; then
	echo "${failures} failure(s)" >&2
	exit 1
fi
echo "all good"
