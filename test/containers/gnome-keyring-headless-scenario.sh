#!/bin/bash
# What SSHakku does for a user whose wallet can only be opened where there is a
# screen, on a machine that has none — the one they reach over SSH. Derived from
# what SSHakku promises (docs/FEATURES.md, F39 and F40) and not from how it is
# built: whether the keys still load when the wallet cannot be set up, whether
# the asking comes back every time because nothing could be saved, and what a
# session with nobody to ask does instead of waiting.
#
# It runs in the session gnome-keyring-headless-session.sh starts, which has no
# display of any kind, and it installs SSHakku the way a user without root does,
# so the login shell below is a login shell in the ordinary sense rather than a
# stand-in for one. The passphrase handoff needs a session keyring that possesses
# the keys it adds, so the container command is expected to be wrapped in
# keyring-session.sh.
set -uo pipefail

readonly PASSPHRASE="the-passphrase"
readonly KEY="id_ed25519"
readonly LOG="${HOME}/.local/state/sshakku/sessions.log"
readonly ELAPSED_BUDGET=30

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

# login runs an interactive login shell on a pseudo-terminal and feeds it the
# given lines. Each is taken by whatever reads next — a passphrase prompt, or
# the shell itself — which is what a person at a keyboard does.
login() {
	printf '%s\n' "$@" | timeout 120 script -qec 'bash -li' /dev/null
}

# between prints the part of the transcript a marker introduces, so an assertion
# about what `ssh-add -l` answered cannot be satisfied by an answer it gave
# earlier or later in the same login.
between() {
	local from="$1" to="$2"
	awk -v from="${from}" -v to="${to}" '
		$0 ~ from { inside = 1; next }
		$0 ~ to { inside = 0 }
		inside { print }
	'
}

# after prints everything a marker introduces, to the end of the login. Answering
# a passphrase prompt is the last thing this login can be told to do: the reader
# takes what is waiting on the terminal, so a command queued behind the answer
# never reaches the shell.
after() {
	awk -v from="$1" '$0 ~ from { inside = 1; next } inside { print }'
}

mkdir -p "${HOME}/.ssh"
chmod 700 "${HOME}/.ssh"
ssh-keygen -t ed25519 -N "${PASSPHRASE}" -C sshakku-headless -f "${HOME}/.ssh/${KEY}" -q

make -C /src install-user GO_BIN=/tmp/build/sshakku >/dev/null || {
	echo "could not install sshakku for this account" >&2
	exit 1
}

# ── F39: the keys load, and the asking comes back ──────────────────────────
#
# One login: the key is asked for, added, then dropped from the agent and asked
# for again in the same shell. Nothing could have been saved in between, so the
# second `ssh-add` must ask rather than reload in silence.
transcript="$(login \
	"${PASSPHRASE}" \
	'echo MARK-loaded' 'ssh-add -l' \
	'ssh-add -D' 'echo MARK-dropped' \
	"ssh-add ${HOME}/.ssh/${KEY}" \
	"${PASSPHRASE}")"
echo "${transcript}"

prompts="$(grep -c 'Enter passphrase' <<<"${transcript}")"
if [ "${prompts}" -eq 2 ]; then
	ok "asked twice: once at the login, once again after the key was dropped"
else
	fail "expected to be asked twice, was asked ${prompts} times"
fi

if between MARK-loaded MARK-dropped <<<"${transcript}" | grep -q sshakku-headless; then
	ok "the key is in the agent after the login"
else
	fail "the login asked for the passphrase but the key never reached the agent"
fi

if after MARK-dropped <<<"${transcript}" | grep -q 'Identity added'; then
	ok "answering again puts the key back"
else
	fail "the key did not come back after the second answer"
fi

# ── F40: a session with nobody to ask is not held up ───────────────────────
#
# No terminal of its own and no screen: what an scp or a scheduled job runs in.
# A key already in the agent is still usable there.
if setsid bash -lc 'ssh-add -l' </dev/null 2>&1 | grep -q sshakku-headless; then
	ok "a key already in the agent is usable with no terminal"
else
	fail "a key in the agent was not usable from a session with no terminal"
fi

# The same session, with the key gone, is where the asking would have to happen
# and cannot: it has to come back rather than wait for an answer nobody can give.
setsid bash -lc 'ssh-add -D' </dev/null >/dev/null 2>&1
started="$(date +%s)"
timeout 120 setsid bash -lc "ssh-add ${HOME}/.ssh/${KEY}" </dev/null >/tmp/no-terminal.out 2>&1
status=$?
elapsed=$(($(date +%s) - started))

if [ "${status}" -eq 124 ]; then
	fail "a session with nobody to ask waited instead of coming back"
elif [ "${elapsed}" -le "${ELAPSED_BUDGET}" ]; then
	ok "came back in ${elapsed}s with nobody to ask"
else
	fail "came back only after ${elapsed}s, past the ${ELAPSED_BUDGET}s a user would wait"
fi

if grep -q 'no terminal' "${LOG}"; then
	ok "the session log says there was nobody to ask"
else
	fail "nothing in the session log says why the key was left alone"
	tail -20 "${LOG}" >&2
fi

if [ "${failures}" -ne 0 ]; then
	echo "${failures} failure(s)" >&2
	exit 1
fi
echo "all good"
