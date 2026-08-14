#!/bin/sh
# A stand-in for the user's editor, for the `sshakku config --edit` tests: a
# real program, run by SSHakku the way any editor is, that records what it was
# asked to open and then saves a prepared file over it.
#
# It is driven by the environment rather than by arguments so that a test can
# pass arguments of its own through $EDITOR and see them arrive here:
#
#   SSHAKKU_TEST_EDITOR_RECORD   file to append this invocation's arguments to
#   SSHAKKU_TEST_EDITOR_BODY     file to save over the last argument; unset or
#                                empty leaves the file exactly as it was found
set -eu

if [ -n "${SSHAKKU_TEST_EDITOR_RECORD:-}" ]; then
	echo "$*" >>"$SSHAKKU_TEST_EDITOR_RECORD"
fi

if [ -n "${SSHAKKU_TEST_EDITOR_BODY:-}" ]; then
	for target in "$@"; do :; done
	cat "$SSHAKKU_TEST_EDITOR_BODY" >"$target"
fi
