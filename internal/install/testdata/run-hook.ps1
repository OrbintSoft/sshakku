# Runs one rendered SSHakku hook the way a login does — dot-sourced into the
# session that was opened — and then ends that session.
#
# Test scaffolding. The hook is named in the environment rather than on the
# command line, because the command line is what the session under test is
# being asked about and nothing of this script's own belongs in it.
#
# The session is ended here because a session told to stay would otherwise sit
# at a prompt nothing in a test can type into.
. $env:SSHAKKU_TEST_HOOK
[System.Environment]::Exit(0)
