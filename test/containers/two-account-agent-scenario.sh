#!/bin/bash
# Whose agent a session is pointed at, when another account on the machine is
# already running one this session can reach. Derived from what SSHakku promises
# (docs/FEATURES.md, F64 and F2) and not from how the decision is made.
#
# The two accounts here are an ordinary user and root, and that pairing is the
# point rather than a convenience. An ssh-agent refuses a client whose uid is not
# its own, so one ordinary account cannot reach another's however open the socket
# is left -- but it makes an exception for uid 0, which is what puts a root login
# in reach of every agent on the machine. A root session pointed at an ordinary
# account's agent is that account being handed whatever root loads afterwards.
#
# The precondition is asserted positively, by listing a key that is really there:
# `ssh-add -l` exits 1 both for an agent holding nothing and for one that refused
# to talk, so an exit code alone cannot tell "reachable" from "refused" -- and a
# scenario that mistook the second for the first would pass against the very
# build it exists to catch.
#
# The binary is built and placed where an install would put it, together with the
# hook a login shell reads, because what is under test is the login path of the
# real program. Whether `make install` puts them there is the install smoke
# test's question, not this one's.
set -uo pipefail

export GOCACHE=/tmp/gc
export GOPATH=/tmp/gp

readonly SSHAKKU=/usr/local/bin/sshakku
readonly HOOK=/etc/profile.d/001-ssh-init.sh
readonly ALICE_SOCK=/home/alice/.cache/sshakku/agent.sock

failures=0

fail() {
	echo "FAIL: $*" >&2
	failures=$((failures + 1))
}

ok() {
	echo "ok: $*"
}

# as_alice runs one login shell as the ordinary account, which is what makes the
# hook run at all: the shell reads /etc/profile, which reads the hook.
as_alice() {
	su - alice -c "$*"
}

# as_root does the same for root, whose login is the one under test.
as_root() {
	bash -l -c "$*"
}

echo "== building and installing the program under test"
go build -o /tmp/build/sshakku /src/cmd/sshakku || {
	echo "FAIL: could not build sshakku" >&2
	exit 1
}
install -Dm755 /tmp/build/sshakku "${SSHAKKU}"
ln -sf sshakku /usr/local/bin/sshakku-askpass
install -Dm755 /src/internal/install/nn-ssh-init.sh "${HOOK}"

echo "== alice logs in and loads a key of her own"
useradd -m -s /bin/bash alice
as_alice 'ssh-keygen -q -t ed25519 -N "" -C alice-key -f ~/.ssh/id_ed25519 </dev/null'
as_alice 'ssh-add ~/.ssh/id_ed25519' >/dev/null 2>&1

if ! as_alice 'ssh-add -l' 2>/dev/null | grep -q alice-key; then
	echo "FAIL: setup — alice's own key is not in her own agent, so nothing below means anything" >&2
	exit 1
fi
ok "setup: alice has an agent of her own holding her own key"

# The precondition, and not a formality: if root could not reach her agent,
# every assertion below would hold for a reason that has nothing to do with
# what is being tested. Asserted on the key she loaded, not on an exit code.
if SSH_AUTH_SOCK="${ALICE_SOCK}" ssh-add -l 2>/dev/null | grep -q alice-key; then
	ok "precondition: root can reach alice's agent, so adopting it is possible here"
else
	echo "FAIL: precondition — root cannot reach alice's agent, so this scenario proves nothing" >&2
	exit 1
fi

echo "== root opens a login shell"
# shellcheck disable=SC2016  # the variable is root's login shell's to expand, not this one's -- it is what is being asked about
root_sock="$(as_root 'printf "%s" "$SSH_AUTH_SOCK"' 2>/dev/null)"

if [ -z "${root_sock}" ]; then
	fail "root's login shell was given no SSH_AUTH_SOCK at all"
elif [ "${root_sock}" = "${ALICE_SOCK}" ]; then
	fail "root's session was pointed straight at alice's socket"
else
	ok "root's session is not pointed at alice's socket"
fi

# A symlink at the fixed path is how an adoption is carried out, so a plain
# socket there is the same fact said from the other side.
if [ -L "${root_sock}" ]; then
	fail "root's endpoint is a symlink to $(readlink "${root_sock}")"
else
	ok "root's endpoint is not a symlink to somebody else's socket"
fi

# Who owns the socket is who is serving it: ssh-agent creates it as itself.
#
# Dereferenced on purpose. An adoption leaves a symlink of root's own making at
# this path, and stat reports a symlink's own owner unless told to follow it --
# so the undereferenced question answers "root" for an endpoint that is serving
# somebody else entirely, which is the one answer this must never give.
owner="$(stat -Lc %U "${root_sock}" 2>/dev/null)"
if [ "${owner}" = "root" ]; then
	ok "the agent serving root runs as root"
else
	fail "the agent serving root runs as '${owner}', not root"
fi

# The other direction of the same promise: root's session is not reading the
# keys alice happens to have loaded either.
if as_root 'ssh-add -l' 2>/dev/null | grep -q alice-key; then
	fail "root's session is looking at alice's loaded keys"
else
	ok "root's session does not see alice's keys"
fi

echo "== root loads a key of root's own"
ssh-keygen -q -t ed25519 -N "" -C root-key -f /root/.ssh/id_ed25519 </dev/null
as_root 'ssh-add /root/.ssh/id_ed25519' >/dev/null 2>&1

if as_root 'ssh-add -l' 2>/dev/null | grep -q root-key; then
	ok "root's key is in the agent root's own session was pointed at"
else
	fail "root's key did not reach the agent root's session was pointed at"
fi

# The promise this scenario exists for. Asked of alice's agent by alice, so what
# is judged is what she can actually get at, not what the program reports.
if as_alice "SSH_AUTH_SOCK=${ALICE_SOCK} ssh-add -l" 2>/dev/null | grep -q root-key; then
	fail "root's key was handed to alice's agent: she can authenticate with it"
else
	ok "no key of root's reached alice's agent"
fi

# F2's half: root's session passed her agent over, it did not kill it. Asked of
# the agent rather than of the process table -- still answering is the promise.
if as_alice "SSH_AUTH_SOCK=${ALICE_SOCK} ssh-add -l" 2>/dev/null | grep -q alice-key; then
	ok "alice's agent still answers and still holds her key: passed over, not killed"
else
	fail "alice's agent stopped serving her: it was not left alone"
fi

echo "== what doctor tells root"
report="$(as_root "${SSHAKKU} doctor" 2>&1)"
if grep -q "different user account" <<<"${report}"; then
	ok "doctor names alice's agent as another account's"
else
	fail "doctor said nothing about the other account's agent"
	echo "${report}" >&2
fi

if [ "${failures}" -gt 0 ]; then
	echo "${failures} failure(s)" >&2
	exit 1
fi
echo "all assertions held"
