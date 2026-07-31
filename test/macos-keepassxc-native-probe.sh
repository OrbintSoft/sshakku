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
# Everything it makes lives in a throwaway directory and is removed with it.
set -uo pipefail

readonly DB_PASSWORD="sshakku-macos-probe-password"
readonly SOCKET_NAME="org.keepassxc.KeePassXC.BrowserServer"
readonly APP="/Applications/KeePassXC.app/Contents/MacOS/KeePassXC"

WORK="$(mktemp -d)"
readonly WORK
trap 'rm -rf "${WORK}"' EXIT

say() { printf '\n=== %s\n' "$*"; }

say "what is installed"
keepassxc-cli --version || echo "keepassxc-cli is not on PATH"
[ -x "${APP}" ] && echo "app: ${APP}" || echo "app not found at ${APP}"

say "where the socket would live, against Darwin's 104-byte cap"
socket_path="${TMPDIR:-/tmp}${SOCKET_NAME}"
printf 'path: %s\nlength: %d\n' "${socket_path}" "${#socket_path}"

say "browser integration on, then a database made without a GUI"
mkdir -p "${HOME}/Library/Application Support/org.keepassxc.KeePassXC"
cp "$(dirname "$0")/containers/keepassxc-browser.ini" \
	"${HOME}/Library/Application Support/org.keepassxc.KeePassXC/keepassxc.ini"
printf '%s\n%s\n' "${DB_PASSWORD}" "${DB_PASSWORD}" |
	keepassxc-cli db-create -p "${WORK}/probe.kdbx" || exit 0

say "opening it with --pw-stdin, which nothing has to type into"
printf '%s\n' "${DB_PASSWORD}" | "${APP}" --pw-stdin "${WORK}/probe.kdbx" >"${WORK}/app.log" 2>&1 &
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
if [ -S "${socket_path}" ]; then
	echo "socket present at ${socket_path}"
else
	echo "no socket at ${socket_path}; looking for one anywhere under TMPDIR:"
	find "${TMPDIR:-/tmp}" -maxdepth 2 -name "${SOCKET_NAME}" 2>/dev/null || true
fi

# The decisive question, asked through the product rather than around it:
# doctor --test-backend stores, reads back and removes a throwaway entry. Its
# answer says which wall we are at — a locked database means --pw-stdin did not
# open one, while an unapproved association means it did and only the approval
# is missing.
say "asking the product to use it"
mkdir -p "${WORK}/config/sshakku"
printf 'secret_backend = "keepassxc"\nkeepassxc_route = "native"\n' \
	>"${WORK}/config/sshakku/config.toml"
go build -o "${WORK}/sshakku" ./cmd/sshakku || exit 0
XDG_CONFIG_HOME="${WORK}/config" "${WORK}/sshakku" doctor --test-backend keepassxc 2>&1 |
	tail -30

kill "${app_pid}" 2>/dev/null
say "probe done — nothing above is an assertion"
