#!/bin/bash
# What a wallet is allowed to end up holding for a key. Derived from what
# SSHakku promises (docs/FEATURES.md, F66) and not from how it is built: a
# passphrase that opened the key is kept, one that opened nothing is not, and an
# entry that has stopped fitting is not answered with for ever.
#
# The key is excluded from automatic loading (F18), which leaves the login shell
# wired and silent and makes the reactive path — ssh asking, sshakku-askpass
# answering — the only thing that can write to the wallet. Without that, the
# proactive loader would save the passphrase at login and there would be nothing
# for this scenario to be about.
#
# It runs in the session gnome-keyring-session.sh starts, where the wallet is a
# real Secret Service with SSHakku's compartment already made.
set -uo pipefail

readonly RIGHT="the-right-one"
readonly WRONG="the-typo"
readonly LATER="the-changed-one"
readonly KEY="id_ed25519"
readonly SERVICE="SSHakku-Key-${KEY}"
readonly KEYFILE="${HOME}/.ssh/${KEY}"

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

# held reports whether the wallet has an entry for the key, and helditis
# whether that entry is a given passphrase. Neither prints what it found: the
# transcript of this run is not a place for a passphrase, test-made or not.
held() {
	secret-tool lookup service "${SERVICE}" username "$(id -un)" >/dev/null 2>&1
}

helditis() {
	secret-tool lookup service "${SERVICE}" username "$(id -un)" 2>/dev/null | grep -qxF "$1"
}

# asked counts the passphrase prompts a transcript put on the terminal. ssh
# prints its own words ("Enter passphrase", "Bad passphrase, try again"), and
# what they have in common is the key they name.
asked() {
	grep -c "passphrase.*${KEY}" <<<"$1"
}

mkdir -p "${HOME}/.ssh"
chmod 700 "${HOME}/.ssh"
ssh-keygen -t ed25519 -N "${RIGHT}" -C sshakku-stored-passphrase -f "${KEYFILE}" -q
mkdir -p "${HOME}/.config/sshakku"
cp /src/test/containers/stored-passphrase-config.toml "${HOME}/.config/sshakku/config.toml"

make -C /src install-user GO_BIN=/tmp/build/sshakku >/dev/null || {
	echo "could not install sshakku for this account" >&2
	exit 1
}

if held; then
	echo "the wallet already holds an entry for ${KEY}; this scenario needs one that does not" >&2
	exit 1
fi

# ── The wiring this scenario rests on ──────────────────────────────────────
#
# An early return and a login that did nothing look the same from the outside,
# so what the shell was actually given is checked before anything is concluded
# from what it did.
# shellcheck disable=SC2016 # the variables are the login shell's to expand, not this one's
wiring="$(login 'echo MARK ASKPASS=${SSH_ASKPASS} REQUIRE=${SSH_ASKPASS_REQUIRE}')"
if grep -q 'MARK ASKPASS=.*sshakku-askpass REQUIRE=force' <<<"${wiring}"; then
	ok "the login shell is wired to answer ssh's passphrase prompts through sshakku"
else
	echo "the login shell was not wired; nothing below would mean anything" >&2
	echo "${wiring}" >&2
	exit 1
fi

# ── F66: a passphrase that opened nothing is not kept ──────────────────────
#
# Three wrong answers, because ssh asks up to three times and each one has to
# meet a wallet that is still empty.
mistyped="$(login "ssh-add ${KEYFILE}" "${WRONG}" "${WRONG}" "${WRONG}")"
echo "${mistyped}"

if held; then
	if helditis "${WRONG}"; then
		fail "the wallet kept what was mistyped; every later use of ${KEY} is answered with it, asking nobody"
	else
		fail "the wallet kept something after a wrong answer"
	fi
else
	ok "nothing was kept for ${KEY} after a wrong answer"
fi

if [ "$(asked "${mistyped}")" -gt 1 ]; then
	ok "the wrong answer was met with another prompt rather than with silence"
else
	fail "answering wrong was not followed by being asked again"
fi

# ── F66: and the right one is ──────────────────────────────────────────────
#
# The half that says this is not simply a program that stores nothing.
typed="$(login "ssh-add ${KEYFILE}" "${RIGHT}")"
echo "${typed}"

if helditis "${RIGHT}"; then
	ok "the passphrase that opened ${KEY} is in the wallet"
else
	fail "the passphrase that opened ${KEY} was not kept, so every later use asks again"
fi

# ── F6, which F66 must not cost: a fitting entry is still used in silence ──
#
# This is what a fix that simply refused to answer from the wallet would fail,
# and it must hold on any build worth having.
silent="$(login 'ssh-add -D' "ssh-add ${KEYFILE}" 'echo MARK-loaded' 'ssh-add -l')"
if [ "$(asked "${silent}")" -eq 0 ]; then
	ok "a stored passphrase that opens the key is used with nobody asked"
else
	fail "the user was asked for a passphrase the wallet was holding"
	echo "${silent}" >&2
fi

if grep -q sshakku-stored-passphrase <<<"${silent}"; then
	ok "and the key is in the agent"
else
	fail "the key never reached the agent"
	echo "${silent}" >&2
fi

# ── F66: an entry that has stopped fitting ─────────────────────────────────
#
# The user changes the key's passphrase, which nothing tells the wallet about.
ssh-keygen -p -P "${RIGHT}" -N "${LATER}" -f "${KEYFILE}" -q

changed="$(login 'ssh-add -D' "ssh-add ${KEYFILE}" "${LATER}")"
echo "${changed}"

if [ "$(asked "${changed}")" -gt 0 ]; then
	ok "a stored passphrase that no longer opens the key is answered by asking"
else
	fail "the key's passphrase changed and nobody was asked; the key simply stops working"
fi

if grep -q 'Identity added' <<<"${changed}"; then
	ok "and answering loads the key"
else
	fail "answering did not load the key"
fi

if helditis "${LATER}"; then
	ok "the wallet now holds the passphrase that opens it, so this is asked once and not for ever"
else
	fail "the new passphrase was not kept"
fi

if [ "${failures}" -ne 0 ]; then
	echo "${failures} failure(s)" >&2
	exit 1
fi
echo "all assertions held"
