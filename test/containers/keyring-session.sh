#!/bin/bash
# Links the user keyring into the session keyring, then runs the given command.
# Meant to be invoked as `keyctl session - keyring-session.sh <command>`, which
# is what gives it a session keyring of its own to link into.
#
# Without the link a process can add a key to the user keyring but not read it
# back: read permission on a new key is granted to whoever possesses it, and
# possession comes from the keyring tree. A PAM login arranges that tree; a bare
# container has no login and so has no link, which is why anything that puts a
# secret in the keyring and reads it out again cannot work there untouched.
#
# The container also has to be started with seccomp unconfined, or keyctl is
# refused outright before any of this runs.
set -euo pipefail

keyctl link @u @s
exec "$@"
