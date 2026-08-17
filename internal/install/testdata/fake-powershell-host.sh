#!/bin/sh
#
# Stands in for a PowerShell host, for a test on a system that has none to ask.
# The subject of such a test is what an install makes of a host's answer, not how
# the answer was obtained, so this ignores the arguments a real host would read
# and prints the answer the test left in the file named below — which is a real
# host's answer, captured.
#
# Copied into a directory of the test's own under the name a PowerShell answers
# to, since that name is what an install recognises an interpreter by.
set -eu

exec cat "$SSHAKKU_TEST_HOST_ANSWER"
