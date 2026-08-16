#!/usr/bin/env bash
# Reports what this shell knows about itself, one key=value to a line, so that
# an install wires the files this shell will actually read.
#
# The answers are the shell's own. A home directory is not always where the
# operating system would say it is: under a POSIX-emulating environment on
# Windows it is taken from HOME, or from HOMEDRIVE and HOMEPATH, or from
# USERPROFILE, whichever the environment was set up to prefer — and a startup
# file written anywhere else is a file nothing ever reads.
#
# Unknown keys are ignored by the reader, so a line may be added here without
# breaking an older one. A value may contain spaces; it cannot contain a line
# ending, which is what makes one answer to a line safe.

set -eu

printf 'home=%s\n' "$HOME"
printf 'shell=%s\n' "${BASH:-}"
printf 'version=%s\n' "${BASH_VERSION:-}"
