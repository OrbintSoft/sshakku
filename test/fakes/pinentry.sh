#!/bin/sh
# A stand-in for pinentry, for the graphical passphrase-prompt tests: a real
# program that speaks the same line protocol over stdin and stdout, so what the
# tests exercise is SSHakku's half of the conversation rather than a mock
# standing in for it.
#
# It is driven by the environment rather than by arguments, because the caller
# under test chooses the arguments:
#
#   SSHAKKU_TEST_PINENTRY_PIN     what GETPIN answers with; defaults to a fixed
#                                 passphrase
#   SSHAKKU_TEST_PINENTRY_CANCEL  non-empty: GETPIN answers the way a dismissed
#                                 dialog does, with an error and no passphrase
#   SSHAKKU_TEST_PINENTRY_NOISE   non-empty: precede the answer with a status
#                                 line and a comment, which a real pinentry may
#                                 send at any point and which answer nothing
#   SSHAKKU_TEST_PINENTRY_HANG    non-empty: never answer GETPIN, the way a
#                                 dialog nobody is sitting in front of does not
#   SSHAKKU_TEST_PINENTRY_FLAVOR  what GETINFO flavor answers with: the builds
#                                 this pinentry can draw with, most capable
#                                 first. Defaults to one that draws on a screen
set -eu

echo "OK Pleased to meet you"

while IFS= read -r line; do
	case "$line" in
	GETPIN*)
		if [ -n "${SSHAKKU_TEST_PINENTRY_NOISE:-}" ]; then
			echo "S PIN_REPEATED"
			echo "# a comment nobody asked for"
		fi
		if [ -n "${SSHAKKU_TEST_PINENTRY_HANG:-}" ]; then
			while :; do
				sleep 60
			done
		fi
		if [ -n "${SSHAKKU_TEST_PINENTRY_CANCEL:-}" ]; then
			echo "ERR 83886179 Operation cancelled"
		else
			printf 'D %s\n' "${SSHAKKU_TEST_PINENTRY_PIN:-a-real-passphrase}"
			echo "OK"
		fi
		;;
	"GETINFO flavor"*)
		printf 'D %s\n' "${SSHAKKU_TEST_PINENTRY_FLAVOR:-gtk2:curses}"
		echo "OK"
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
