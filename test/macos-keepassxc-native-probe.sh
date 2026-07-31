#!/bin/bash
# Reports what KeePassXC's local protocol can be reached with on a macOS CI
# runner. Not a test: it asserts nothing and is expected to end with something
# still out of reach. It exists so that decision is made from measurements
# rather than from guesses about what a runner allows.
#
# One thing is left to find out, and it is the one a runner cannot do by hand:
# opening a database with nobody at the keyboard. Two ways are tried, because
# the app is known to differ by build — on Linux's 2.7.12 both crash it outright,
# while macOS's build of the same version survives them:
#
#   a database whose only credential is a key file, opened with --keyfile, so
#   there is no password to deliver at all
#
#   a password on stdin with the stream held open afterwards, in case closing it
#   is what the earlier attempt died of
#
# What decides it is the product's own answer, not a window nobody can see: a
# locked database is reported as such, while any complaint about the client not
# being recognised means the database is open and only the association is
# missing.
#
# Browser integration is turned on with --config, which takes the settings file
# to use as an argument. Nothing here writes to the settings KeePassXC would
# otherwise read, and no assumption is made about where those live.
#
# Everything it makes lives in a throwaway directory and is removed with it.
set -uo pipefail

readonly DB_PASSWORD="sshakku-macos-probe-password"
readonly SOCKET_NAME="org.keepassxc.KeePassXC.BrowserServer"
readonly APP="/Applications/KeePassXC.app/Contents/MacOS/KeePassXC"

WORK="$(mktemp -d)"
readonly WORK
trap 'rm -rf "${WORK}"' EXIT

say() { printf '\n=== %s\n' "$*"; }

# look_for_socket prints every path the endpoint was found at, searching the
# directories KeePassXC is known to use on any platform rather than only the one
# this build is expected to pick.
look_for_socket() {
	find "${TMPDIR:-/tmp}" /tmp -maxdepth 3 -name "${SOCKET_NAME}" 2>/dev/null
}

say "what is installed"
keepassxc-cli --version || echo "keepassxc-cli is not on PATH"
[ -x "${APP}" ] && echo "app: ${APP}" || echo "app not found at ${APP}"

say "where the socket would live, against Darwin's 104-byte cap"
socket_path="${TMPDIR:-/tmp}${SOCKET_NAME}"
printf 'path: %s\nlength: %d\n' "${socket_path}" "${#socket_path}"

say "two databases made without a GUI: one with a password, one with a key file"
cp "$(dirname "$0")/containers/keepassxc-browser.ini" "${WORK}/keepassxc.ini"
printf '%s\n%s\n' "${DB_PASSWORD}" "${DB_PASSWORD}" |
	keepassxc-cli db-create -p "${WORK}/password.kdbx" || exit 0
keepassxc-cli db-create --set-key-file "${WORK}/db.keyx" "${WORK}/keyfile.kdbx" || exit 0
echo "the key file alone opens it:"
keepassxc-cli ls --no-password -k "${WORK}/db.keyx" "${WORK}/keyfile.kdbx" || echo "…it does not"

# The product is built once and asked the same question after each attempt. It
# reads no wallet of its own here: the configuration points it at KeePassXC over
# the local protocol and nothing else.
mkdir -p "${WORK}/config/sshakku"
printf 'secret_backend = "keepassxc"\nkeepassxc_route = "native"\n' \
	>"${WORK}/config/sshakku/config.toml"
go build -o "${WORK}/sshakku" ./cmd/sshakku || exit 0
export XDG_CONFIG_HOME="${WORK}/config"

# ask_the_product runs the backend test and prints the lines that say which wall
# we are at.
ask_the_product() {
	"${WORK}/sshakku" doctor --test-backend keepassxc 2>&1 |
		sed -n '/testing secret backend/,$p'
}

# KeePassXC allows one instance at a time, so each attempt gets the field to
# itself.
stop_the_app() {
	pkill -f "KeePassXC.app/Contents/MacOS/KeePassXC" 2>/dev/null
	sleep 3
}

say "attempt 1: a key-file-only database, nothing to type"
"${APP}" --config "${WORK}/keepassxc.ini" --keyfile "${WORK}/db.keyx" \
	"${WORK}/keyfile.kdbx" >"${WORK}/keyfile.log" 2>&1 &
sleep 15
if pgrep -f "KeePassXC.app/Contents/MacOS/KeePassXC" >/dev/null; then
	echo "the app is still running"
else
	echo "the app is gone; --keyfile did not survive here either"
fi
echo "socket: $(look_for_socket | head -1)"
echo "--- what it said:"
cat "${WORK}/keyfile.log"
echo "--- what the product makes of it:"
ask_the_product
stop_the_app

say "attempt 2: a password on stdin, with the stream held open"
{
	printf '%s\n' "${DB_PASSWORD}"
	sleep 60
} | "${APP}" --config "${WORK}/keepassxc.ini" --pw-stdin \
	"${WORK}/password.kdbx" >"${WORK}/pwstdin.log" 2>&1 &
sleep 15
if pgrep -f "KeePassXC.app/Contents/MacOS/KeePassXC" >/dev/null; then
	echo "the app is still running"
else
	echo "the app is gone"
fi
echo "socket: $(look_for_socket | head -1)"
echo "--- what it said:"
cat "${WORK}/pwstdin.log"
echo "--- what the product makes of it:"
ask_the_product

# KeePassXC saves its settings back into the file it was given, so keys it added
# to the one we wrote are the evidence that --config was taken rather than
# ignored in favour of wherever this build keeps its own.
say "the settings file we handed over, as KeePassXC left it"
cat "${WORK}/keepassxc.ini"

stop_the_app
say "probe done — nothing above is an assertion"
