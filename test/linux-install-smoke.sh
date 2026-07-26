#!/usr/bin/env bash
#
# Smoke-tests the Linux branch of the Makefile's install/install-user targets:
# builds and wires the hook into a staged PREFIX/DESTDIR/USER_HOME tree instead
# of the real /etc or $HOME, then confirms uninstall/uninstall-user cleanly
# reverse it. The counterpart to test/macos-install-smoke.sh; this one covers
# the Linux-only paths the macOS script never touches — the /etc/profile.d
# drop-in, the GNU `sed -i` (no backup-suffix arg), and the opt-in non-login
# bash wiring (WIRE_BASHRC) in both of its shapes: a drop-in into an existing
# bashrc.d directory, or a marker block in the single fallback file.
#
# Every path the install writes is redirected under <work_dir>, so it needs no
# root and leaves nothing behind on the host: DESTDIR stages the system-wide
# tree, USER_HOME stages the per-user one, and BASH_BASHRC_D/BASH_BASHRC_FILE
# stage the non-login wiring targets (BASH_BASHRC_D also decides which wiring
# shape the Makefile takes, since it probes that directory to choose).
#
# Usage: linux-install-smoke.sh <work_dir>
set -euxo pipefail

work_dir="${1:?usage: linux-install-smoke.sh <work_dir>}"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

# ---------------------------------------------------------------------------
# System-wide install / uninstall (make install / make uninstall).
# ---------------------------------------------------------------------------
sw="$work_dir/system"
sw_prefix="$sw/prefix"
sw_destdir="$sw/root"
mkdir -p "$sw_prefix" "$sw_destdir"
# BINDIR defaults to PREFIX/bin, so the runtime path the hook is rewritten to
# point at is PREFIX/bin/sshakku (with no DESTDIR — DESTDIR is a staging prefix
# for where files land, not part of the runtime path baked into the hook).
sw_runtime_bin="$sw_prefix/bin/sshakku"
sw_hook="$sw_destdir/etc/profile.d/001-ssh-init.sh"

make install PREFIX="$sw_prefix" DESTDIR="$sw_destdir"
test -x "$sw_destdir$sw_prefix/bin/sshakku"
test -x "$sw_hook"
grep -qF "$sw_runtime_bin" "$sw_hook"

make uninstall PREFIX="$sw_prefix" DESTDIR="$sw_destdir"
test ! -e "$sw_destdir$sw_prefix/bin/sshakku"
test ! -e "$sw_hook"

# ---------------------------------------------------------------------------
# System-wide non-login wiring, drop-in shape (WIRE_BASHRC=1, bashrc.d exists).
# The hook's runtime source line points at the real /etc/profile.d path (the
# rendered file's home), independent of DESTDIR.
# ---------------------------------------------------------------------------
di="$work_dir/wire-dropin"
di_destdir="$di/root"
di_bashrc_d="$di/etc/bash/bashrc.d"
mkdir -p "$di_destdir" "$di_bashrc_d"
di_dropin="$di_destdir$di_bashrc_d/001-ssh-init.sh"
# BASH_BASHRC_D is concatenated with the filename, so it needs the trailing
# slash the default value (/etc/bash/bashrc.d/) carries.
make install PREFIX="$di/prefix" DESTDIR="$di_destdir" WIRE_BASHRC=1 BASH_BASHRC_D="$di_bashrc_d/"
test -f "$di_dropin"
grep -qF '. "/etc/profile.d/001-ssh-init.sh"' "$di_dropin"

make uninstall PREFIX="$di/prefix" DESTDIR="$di_destdir" WIRE_BASHRC=1 BASH_BASHRC_D="$di_bashrc_d/"
test ! -e "$di_dropin"

# ---------------------------------------------------------------------------
# System-wide non-login wiring, fallback-file shape (WIRE_BASHRC=1, no
# bashrc.d, so a marker block is upserted into the single bash.bashrc file).
# ---------------------------------------------------------------------------
fb="$work_dir/wire-file"
fb_destdir="$fb/root"
fb_bashrc_file="$fb/etc/bash.bashrc"
mkdir -p "$fb_destdir"
fb_target="$fb_destdir$fb_bashrc_file"

make install PREFIX="$fb/prefix" DESTDIR="$fb_destdir" WIRE_BASHRC=1 \
	BASH_BASHRC_D="$fb/absent" BASH_BASHRC_FILE="$fb_bashrc_file"
test -f "$fb_target"
grep -qF '. "/etc/profile.d/001-ssh-init.sh"' "$fb_target"

make uninstall PREFIX="$fb/prefix" DESTDIR="$fb_destdir" WIRE_BASHRC=1 \
	BASH_BASHRC_D="$fb/absent" BASH_BASHRC_FILE="$fb_bashrc_file"
if grep -q sshakku "$fb_target"; then
	echo "bash.bashrc still wired after uninstall" >&2
	exit 1
fi

# ---------------------------------------------------------------------------
# Per-user install / uninstall (make install-user / make uninstall-user).
# On Linux USER_SHELL defaults to bash, so this wires ~/.bash_profile and,
# with WIRE_PATH=1 (the default), also puts ~/.local/bin on PATH.
# ---------------------------------------------------------------------------
home="$work_dir/home"
mkdir -p "$home"
profile="$home/.bash_profile"
user_hook="$home/.local/share/sshakku/shell-hook.sh"

make install-user USER_HOME="$home"
test -x "$home/.local/bin/sshakku"
test -x "$user_hook"
grep -qF "$user_hook" "$profile"
grep -qF "export PATH=\"$home/.local/bin:" "$profile"

make uninstall-user USER_HOME="$home"
test ! -e "$home/.local/bin/sshakku"
test ! -e "$user_hook"
if [ -f "$profile" ] && grep -q sshakku "$profile"; then
	echo "per-user .bash_profile still wired after uninstall" >&2
	exit 1
fi

# ---------------------------------------------------------------------------
# Per-user non-login wiring (make install-user WIRE_BASHRC=1 also wires the
# interactive ~/.bashrc, not just the login ~/.bash_profile).
# ---------------------------------------------------------------------------
home_wire="$work_dir/home-wire"
mkdir -p "$home_wire"
bashrc="$home_wire/.bashrc"

make install-user USER_HOME="$home_wire" WIRE_BASHRC=1
grep -qF sshakku "$home_wire/.bash_profile"
grep -qF sshakku "$bashrc"

make uninstall-user USER_HOME="$home_wire"
if [ -f "$bashrc" ] && grep -q sshakku "$bashrc"; then
	echo "per-user .bashrc still wired after uninstall" >&2
	exit 1
fi
