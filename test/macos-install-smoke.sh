#!/usr/bin/env bash
#
# Smoke-tests the Darwin branch of the Makefile's install/install-user
# targets: builds and wires the hook into a staged prefix/DESTDIR/USER_HOME
# tree instead of the real /etc or $HOME, then confirms uninstall/
# uninstall-user cleanly reverse it. Meant to run only on a real macOS
# runner (exercises the BSD `sed -i ''` syntax and /etc/zprofile marker-block
# wiring the Linux install path never touches).
#
# Usage: macos-install-smoke.sh <work_dir>
set -euxo pipefail

work_dir="${1:?usage: macos-install-smoke.sh <work_dir>}"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"

prefix="$work_dir/prefix"
destdir="$work_dir/root"
home="$work_dir/home"
rendered="$destdir$prefix/share/sshakku/001-sshakku-init.sh"
mkdir -p "$prefix" "$destdir" "$home"

cd "$repo_root"

make install PREFIX="$prefix" DESTDIR="$destdir"
test -x "$destdir$prefix/bin/sshakku"
test -x "$rendered"
grep -qF ". \"$rendered\"" "$destdir/etc/zprofile"

make uninstall PREFIX="$prefix" DESTDIR="$destdir"
test ! -e "$destdir$prefix/bin/sshakku"
test ! -e "$rendered"
if grep -q sshakku "$destdir/etc/zprofile"; then
	echo "zprofile still wired after uninstall" >&2
	exit 1
fi

make install-user USER_HOME="$home"
test -x "$home/.local/bin/sshakku"
grep -qF sshakku "$home/.zprofile"

make uninstall-user USER_HOME="$home"
test ! -e "$home/.local/bin/sshakku"
if [ -f "$home/.zprofile" ] && grep -q sshakku "$home/.zprofile"; then
	echo "per-user zprofile still wired after uninstall" >&2
	exit 1
fi
