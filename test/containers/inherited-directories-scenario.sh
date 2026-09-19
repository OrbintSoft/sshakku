#!/bin/bash
# What a session does with the directories its environment names, when they
# belong to somebody else. Derived from what SSHakku promises (docs/FEATURES.md,
# F67 and F65) and not from how the decision is made.
#
# The pairing is an ordinary account and root, and it is the point rather than a
# convenience. These variables are inherited like any other, so a shell that
# becomes another user without opening a session of its own carries the account
# it came from -- su without a login shell, sudo without -i -- and the account
# worth carrying into is root, whose session has something to lose.
#
# Nothing here is an attack. Both sessions resolve directories; the whole
# question is whose they end up being, and whether a session that was sent
# somewhere else says so. Two of the checks below must hold on any build worth
# having: a directory that really is this account's own is still obeyed, and one
# that is merely absent is still this session's to create. A "fix" that simply
# stopped reading the environment would pass everything but those two.
set -uo pipefail

export GOCACHE=/tmp/gc
export GOPATH=/tmp/gp

readonly SSHAKKU=/usr/local/bin/sshakku
readonly HOOK=/etc/profile.d/001-ssh-init.sh
readonly ALICE_CONFIG=/home/alice/.config
readonly ALICE_STATE=/home/alice/.local/state
readonly ALICE_CACHE=/home/alice/.cache
readonly HERS=AlicePrefixNobodyElseWouldPick
readonly MARKER=/home/alice/the-editor-alice-named-ran

failures=0

fail() {
	echo "FAIL: $*" >&2
	failures=$((failures + 1))
}

ok() {
	echo "ok: $*"
}

# as_root_carrying runs one root login shell with the given assignment in its
# environment, standing for a root shell started from somebody else's session.
as_root_carrying() {
	local assignment="$1"
	shift
	env "${assignment}" bash -l -c "$*"
}

echo "== building and installing the program under test"
go build -o /tmp/build/sshakku /src/cmd/sshakku || {
	echo "FAIL: could not build sshakku" >&2
	exit 1
}
install -Dm755 /tmp/build/sshakku "${SSHAKKU}"
ln -sf sshakku /usr/local/bin/sshakku-askpass
install -Dm644 /src/internal/install/nn-ssh-init.sh "${HOOK}"

echo "== the two accounts, and a configuration alice wrote"
id -u alice >/dev/null 2>&1 || useradd -m -u 1000 alice
su alice -s /bin/bash -c "mkdir -p ${ALICE_CONFIG}/sshakku ${ALICE_STATE} ${ALICE_CACHE}"
su alice -s /bin/bash -c "cat > ${ALICE_CONFIG}/sshakku/config.toml" <<TOML
service_prefix = "${HERS}"
editor = "/home/alice/editor"
TOML
su alice -s /bin/bash -c 'cat > /home/alice/editor' <<SH
#!/bin/bash
id -un > ${MARKER}
SH
su alice -s /bin/bash -c 'chmod +x /home/alice/editor'
install -d -m 700 /root/.ssh
ssh-keygen -q -t ed25519 -N "" -C root-key -f /root/.ssh/id_ed25519 <<<y >/dev/null 2>&1

# The precondition is asserted positively: a directory alice owns, which root
# can reach and must decline to use. A scenario that mistook an unreadable path
# for a refusal would pass against the very build it exists to catch.
if [ -r "${ALICE_CONFIG}/sshakku/config.toml" ] && grep -q "${HERS}" "${ALICE_CONFIG}/sshakku/config.toml"; then
	ok "alice's configuration is there and root can read it, so declining it is a decision"
else
	fail "alice's configuration was never written: the rest of this would prove nothing"
	exit 1
fi

# ── F67: the settings obeyed are this account's own ────────────────────────
echo "== root opens a shell carrying alice's configuration directory"
report="$(as_root_carrying "XDG_CONFIG_HOME=${ALICE_CONFIG}" "${SSHAKKU} config" 2>&1)"

if grep -q "config directory: ${ALICE_CONFIG}" <<<"${report}"; then
	fail "root's configuration directory is alice's"
else
	ok "root's configuration directory is not alice's"
fi

if grep -q "${HERS}" <<<"${report}"; then
	fail "a setting alice wrote is in force in root's session"
	echo "${report}" >&2
else
	ok "no setting of alice's is in force in root's session"
fi

echo "== and asks to edit its configuration"
as_root_carrying "XDG_CONFIG_HOME=${ALICE_CONFIG}" "${SSHAKKU} config --edit" </dev/null >/dev/null 2>&1

if [ -e "${MARKER}" ]; then
	fail "the command alice named ran as $(cat "${MARKER}")"
else
	ok "the command alice named was not run"
fi

# ── F67: and the record of the session is written into this account's own ──
echo "== root logs in carrying alice's state directory"
as_root_carrying "XDG_STATE_HOME=${ALICE_STATE}" "true" >/dev/null 2>&1

if find "${ALICE_STATE}" -name 'sessions.log' | grep -q .; then
	fail "root's session log was written into alice's state directory"
else
	ok "nothing of root's session was written into alice's state directory"
fi

# ── F65: the endpoint, when the cache is where it would land ───────────────
#
# The cache is reached when there is no runtime directory to use, which is the
# ordinary case for a root shell on a system that makes none.
echo "== alice leaves an agent of her own where root's endpoint would land"
# She owns the parent, so the directory at that path is hers to make, and what
# answers there is hers to choose. This is what the mode on the leaf cannot
# help with: a session pointed here is pointed at her agent.
rm -rf /run/user/0
su alice -s /bin/bash -c "mkdir -p ${ALICE_CACHE}/sshakku &&
	eval \"\$(ssh-agent -a ${ALICE_CACHE}/sshakku/agent.sock)\" >/dev/null &&
	chmod 777 ${ALICE_CACHE}/sshakku ${ALICE_CACHE}/sshakku/agent.sock" >/dev/null 2>&1

echo "== root logs in carrying alice's cache directory"
# shellcheck disable=SC2016 # $SSH_AUTH_SOCK is the login shell's to expand, not this one's: what it was set to there is the answer being read.
root_sock="$(as_root_carrying "XDG_CACHE_HOME=${ALICE_CACHE}" 'printf "%s" "${SSH_AUTH_SOCK:-}"' 2>/dev/null)"

case "${root_sock}" in
"${ALICE_CACHE}"/*)
	fail "root's endpoint is inside alice's cache directory: ${root_sock}"
	;;
"")
	fail "root's session was given no endpoint at all"
	;;
*)
	ok "root's endpoint is outside alice's cache directory: ${root_sock}"
	;;
esac

echo "== root loads a key of root's own"
as_root_carrying "XDG_CACHE_HOME=${ALICE_CACHE}" "ssh-add /root/.ssh/id_ed25519" >/dev/null 2>&1

if su alice -s /bin/bash -c "SSH_AUTH_SOCK=${ALICE_CACHE}/sshakku/agent.sock ssh-add -l" 2>/dev/null | grep -q root-key; then
	fail "root's key is in alice's agent: she can authenticate as root with it"
else
	ok "root's key did not reach the agent alice left waiting"
fi

# ── The report: a session whose directories moved must say so ──────────────
echo "== what root is told about it"
findings="$(as_root_carrying "XDG_CONFIG_HOME=${ALICE_CONFIG}" "${SSHAKKU} doctor" 2>&1)"

if grep -q "XDG_CONFIG_HOME" <<<"${findings}" && grep -q "${ALICE_CONFIG}" <<<"${findings}"; then
	ok "the report names the directory that was asked for and the variable that named it"
else
	fail "root was sent elsewhere with nothing said about it"
	echo "${findings}" >&2
fi

# ── What must hold on any build: this is not a program that ignores the ────
#    environment, it is one that asks whose the directory is.
echo "== a configuration directory that really is root's own"
install -d -o root -g root -m 700 /root/own-config/sshakku
readonly OURS=RootPrefixOfItsOwn
cat >/root/own-config/sshakku/config.toml <<TOML
service_prefix = "${OURS}"
TOML

if as_root_carrying "XDG_CONFIG_HOME=/root/own-config" "${SSHAKKU} config" 2>&1 | grep -q "${OURS}"; then
	ok "a configuration directory this account owns is still obeyed"
else
	fail "root's own configuration was refused: the environment is being ignored rather than questioned"
fi

echo "== a configuration directory that is merely absent"
if as_root_carrying "XDG_CONFIG_HOME=/root/not-there-at-all" "${SSHAKKU} config" 2>&1 |
	grep -q "config directory: /root/not-there-at-all/sshakku"; then
	ok "a directory that is not there yet is still this session's to create"
else
	fail "a path nobody turned down was treated as though somebody had"
fi

if [ "${failures}" -ne 0 ]; then
	echo "${failures} failure(s)" >&2
	exit 1
fi
echo "all assertions held"
