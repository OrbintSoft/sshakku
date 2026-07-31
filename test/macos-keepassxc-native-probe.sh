#!/bin/bash
# Reports what KeePassXC's local protocol can be reached with on a macOS CI
# runner. Not a test: it asserts nothing and is expected to end with something
# still out of reach. It exists so that decision is made from measurements
# rather than from guesses about what a runner allows.
#
# Three things have to hold before the native route can be driven with nobody
# at the keyboard, and each is unknown here for its own reason:
#
#   the app must open a database without anyone typing the password
#   (--pw-stdin; on Linux's build that option crashes the app outright)
#
#   the socket address must fit: a unix socket path is capped at 104 bytes on
#   Darwin, checked at bind, and macOS puts TMPDIR under /var/folders
#
#   the association must be approved without anyone clicking, which a runner
#   grants no accessibility permission for
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

say "a database made without a GUI"
printf '%s\n%s\n' "${DB_PASSWORD}" "${DB_PASSWORD}" |
	keepassxc-cli db-create -p "${WORK}/probe.kdbx" || exit 0

say "opening it with --pw-stdin, browser integration on, which nothing has to type into"
cp "$(dirname "$0")/containers/keepassxc-browser.ini" "${WORK}/keepassxc.ini"
printf '%s\n' "${DB_PASSWORD}" |
	"${APP}" --config "${WORK}/keepassxc.ini" --pw-stdin "${WORK}/probe.kdbx" \
		>"${WORK}/app.log" 2>&1 &
app_pid=$!
sleep 15

if kill -0 "${app_pid}" 2>/dev/null; then
	echo "the app is still running after 15s"
else
	echo "the app is gone; --pw-stdin did not survive here either"
fi
echo "--- what it said:"
cat "${WORK}/app.log"

say "is anything listening?"
found="$(look_for_socket)"
if [ -n "${found}" ]; then
	echo "${found}"
else
	echo "nothing is listening anywhere under ${TMPDIR:-/tmp} or /tmp"
fi

# KeePassXC saves its settings back into the file it was given, so keys it added
# to the one we wrote are the evidence that --config was taken rather than
# ignored in favour of wherever this build keeps its own.
say "the settings file we handed over, as KeePassXC left it"
cat "${WORK}/keepassxc.ini"

# Two questions, asked through the product rather than around it. The plain
# report only looks, and says which wallet would be used and whether what it
# needs is here; --test-backend then stores, reads back and removes a throwaway
# entry, and its answer says which wall we are at — a locked database means
# --pw-stdin did not open one, while an unapproved association means it did and
# only the approval is missing.
say "asking the product what it sees"
mkdir -p "${WORK}/config/sshakku"
printf 'secret_backend = "keepassxc"\nkeepassxc_route = "native"\n' \
	>"${WORK}/config/sshakku/config.toml"
go build -o "${WORK}/sshakku" ./cmd/sshakku || exit 0
export XDG_CONFIG_HOME="${WORK}/config"
"${WORK}/sshakku" doctor 2>&1 | tail -30

say "asking the product to use it"
"${WORK}/sshakku" doctor --test-backend keepassxc 2>&1 | tail -30

kill "${app_pid}" 2>/dev/null
say "probe done — nothing above is an assertion"
