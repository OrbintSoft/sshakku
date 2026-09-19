#!/bin/bash
# Where the endpoint a session is pointed at is put, when the environment names
# a runtime directory belonging to somebody else. Derived from what SSHakku
# promises (docs/FEATURES.md, F65) and not from how the decision is made.
#
# The pairing is an ordinary account and root, and it is the point rather than a
# convenience. $XDG_RUNTIME_DIR is inherited like any other variable, so a shell
# that becomes another user without opening a session of its own carries the
# account it came from -- and the account worth carrying into is root, whose
# session is the one that has something to lose. Nothing here is an attack: both
# sessions resolve a path, and the whole question is whether they resolve the
# same one.
#
# Three cases, because only the middle one is a refusal. A directory that is not
# there was not turned down, and a report that called it somebody else's would
# send a person after a stale variable as though it were an intruder; a
# directory that is the session's own must go on being used, or the promise
# would be kept by breaking every ordinary login.
#
# The binary is built and placed where an install would put it, together with
# the hook a login shell reads, because what is under test is the login path of
# the real program.
set -uo pipefail

export GOCACHE=/tmp/gc
export GOPATH=/tmp/gp

readonly SSHAKKU=/usr/local/bin/sshakku
readonly HOOK=/etc/profile.d/001-ssh-init.sh
readonly ALICE_RUNTIME=/run/user/1000
readonly ALICE_SOCK="${ALICE_RUNTIME}/sshakku/agent.sock"

failures=0

fail() {
	echo "FAIL: $*" >&2
	failures=$((failures + 1))
}

ok() {
	echo "ok: $*"
}

# as_alice runs one login shell as the ordinary account, with her own runtime
# directory named -- which is what a session of hers is given.
as_alice() {
	su alice -s /bin/bash -c "export XDG_RUNTIME_DIR=${ALICE_RUNTIME}; bash -l -c '$*'"
}

# as_root_carrying runs one root login shell with $XDG_RUNTIME_DIR set to what
# it is given, standing for a root shell that inherited it from the session it
# was started from.
as_root_carrying() {
	local runtime="$1"
	shift
	XDG_RUNTIME_DIR="${runtime}" bash -l -c "$*"
}

echo "== building and installing the program under test"
go build -o /tmp/build/sshakku /src/cmd/sshakku || {
	echo "FAIL: could not build sshakku" >&2
	exit 1
}
install -Dm755 /tmp/build/sshakku "${SSHAKKU}"
ln -sf sshakku /usr/local/bin/sshakku-askpass
install -Dm644 /src/internal/install/nn-ssh-init.sh "${HOOK}"

echo "== the two accounts, and the runtime directory logind would give each"
id -u alice >/dev/null 2>&1 || useradd -m -u 1000 alice
install -d -o alice -g alice -m 700 "${ALICE_RUNTIME}"
install -d -o root -g root -m 700 /run/user/0
su alice -s /bin/bash -c 'ssh-keygen -q -t ed25519 -N "" -C alice-key -f /home/alice/.ssh/id_ed25519 <<<y' >/dev/null 2>&1
install -d -m 700 /root/.ssh
ssh-keygen -q -t ed25519 -N "" -C root-key -f /root/.ssh/id_ed25519 <<<y >/dev/null 2>&1

echo "== alice logs in, and her key goes into her own agent"
as_alice "ssh-add /home/alice/.ssh/id_ed25519" >/dev/null 2>&1

# The precondition is asserted positively, by listing a key that is really
# there: an exit code alone cannot tell an agent holding nothing from one that
# would not talk, and a scenario that mistook the second for the first would
# pass against the very build it exists to catch.
if as_alice "SSH_AUTH_SOCK=${ALICE_SOCK} ssh-add -l" 2>/dev/null | grep -q alice-key; then
	ok "alice's agent is up in her own runtime directory and holds her key"
else
	fail "alice's agent never came up: the rest of this scenario would prove nothing"
	echo "${failures} failure(s)" >&2
	exit 1
fi

echo "== root logs in carrying alice's runtime directory"
# shellcheck disable=SC2016 # $SSH_AUTH_SOCK is the login shell's to expand, not this one's: what it was set to there is the answer being read.
root_sock="$(as_root_carrying "${ALICE_RUNTIME}" 'printf "%s" "$SSH_AUTH_SOCK"' 2>/dev/null)"

case "${root_sock}" in
"${ALICE_RUNTIME}"/*)
	fail "root's endpoint is inside alice's runtime directory: ${root_sock}"
	;;
"")
	fail "root's session was given no endpoint at all"
	;;
*)
	ok "root's endpoint is outside alice's runtime directory: ${root_sock}"
	;;
esac

# The promise is about whose agent answers, not only about where the path is.
if as_root_carrying "${ALICE_RUNTIME}" 'ssh-add -l' 2>&1 | grep -q alice-key; then
	fail "root's session is being served by alice's agent: it lists her key"
else
	ok "root's session does not see alice's keys"
fi

echo "== root loads a key of root's own"
as_root_carrying "${ALICE_RUNTIME}" "ssh-add /root/.ssh/id_ed25519" >/dev/null 2>&1

if su alice -s /bin/bash -c "SSH_AUTH_SOCK=${ALICE_SOCK} ssh-add -l" 2>/dev/null | grep -q root-key; then
	fail "root's key was loaded into alice's agent, where she can use it"
else
	ok "root's key did not reach alice's agent"
fi

# F2's half: alice's agent was left alone. Asked of the agent rather than of the
# process table -- still answering is the promise.
if su alice -s /bin/bash -c "SSH_AUTH_SOCK=${ALICE_SOCK} ssh-add -l" 2>/dev/null | grep -q alice-key; then
	ok "alice's agent still answers and still holds her key"
else
	fail "alice's agent stopped serving her: it was not left alone"
fi

echo "== what root is told about the move"
if grep -q 'XDG_RUNTIME_DIR names' /root/.local/state/sshakku/sessions.log 2>/dev/null; then
	ok "the session log names the directory that was asked for"
else
	fail "root's endpoint moved and nothing in the session log accounts for it"
fi

report="$(as_root_carrying "${ALICE_RUNTIME}" "${SSHAKKU} doctor" 2>&1)"
if grep -q 'XDG_RUNTIME_DIR names' <<<"${report}"; then
	ok "doctor names the directory the environment asked for"
else
	fail "doctor said nothing about the runtime directory"
	echo "${report}" >&2
fi

echo "== a runtime directory that is the session's own is still used"
# shellcheck disable=SC2016 # as above: the login shell expands it, this one only reads what came back.
own_sock="$(as_root_carrying /run/user/0 'printf "%s" "$SSH_AUTH_SOCK"' 2>/dev/null)"
case "${own_sock}" in
/run/user/0/*)
	ok "a session whose runtime directory is its own is served from it: ${own_sock}"
	;;
*)
	fail "a runtime directory of root's own was not used: ${own_sock}"
	;;
esac

echo "== a runtime directory that is not there is not somebody else's"
: >/root/.local/state/sshakku/sessions.log
as_root_carrying /run/user/4242 'true' >/dev/null 2>&1
if grep -q 'XDG_RUNTIME_DIR names' /root/.local/state/sshakku/sessions.log 2>/dev/null; then
	fail "a runtime directory that is merely absent was reported as refused"
else
	ok "a stale variable naming nothing is not reported as another account's directory"
fi

if [ "${failures}" -gt 0 ]; then
	echo "${failures} failure(s)" >&2
	exit 1
fi
echo "all assertions held"
