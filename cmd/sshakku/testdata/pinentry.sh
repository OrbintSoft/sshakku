#!/bin/sh
# A stand-in for pinentry, for the graphical passphrase-prompt tests: a real
# program found on PATH that speaks the same line protocol over stdin and
# stdout, so what the test exercises is SSHakku's half of the conversation
# rather than a mock standing in for it.
#
# It is driven by the environment rather than by arguments, because the caller
# under test chooses the arguments:
#
#   SSHAKKU_TEST_PINENTRY_PIN     what GETPIN answers with; defaults to a fixed
#                                 passphrase
#   SSHAKKU_TEST_PINENTRY_CANCEL  non-empty: GETPIN answers the way a dismissed
#                                 dialog does, with an error and no passphrase
set -eu

echo "OK Pleased to meet you"

while IFS= read -r line; do
	case "$line" in
	GETPIN*)
		if [ -n "${SSHAKKU_TEST_PINENTRY_CANCEL:-}" ]; then
			echo "ERR 83886179 Operation cancelled"
		else
			printf 'D %s\n' "${SSHAKKU_TEST_PINENTRY_PIN:-a-real-passphrase}"
			echo "OK"
		fi
		;;
	BYE*)
		echo "OK closing connection"
		exit 0
		;;
	*)
		echo "OK"
		;;
	esac
done
