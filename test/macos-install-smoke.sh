#!/usr/bin/env bash
#
# Smoke-tests the Darwin branch of the Makefile's install/install-user
# targets: builds and wires the hook into a staged prefix/DESTDIR/USER_HOME
# tree instead of the real /etc or $HOME, then confirms uninstall/
# uninstall-user cleanly reverse it. Meant to run only on a real macOS
# runner (exercises the BSD `sed -i ''` syntax and /etc/zprofile marker-block
# wiring the Linux install path never touches).
#
# Both the default login-shell wiring and the opt-in non-login one
# (WIRE_ZSHRC=1) are covered, system-wide and per-user, in both shapes the
# per-user installer can take: a drop-in into an existing ~/.zshrc.d, or a
# marker block in ~/.zshrc when there is none.
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
# The askpass helper is a link to the binary beside it: -L that it is a link at
# all, -x that it resolves to something runnable, since a link made relative to
# the wrong directory would still be a link.
test -L "$destdir$prefix/bin/sshakku-askpass"
test -x "$destdir$prefix/bin/sshakku-askpass"
test -x "$rendered"
grep -qF ". \"$rendered\"" "$destdir/etc/zprofile"

make uninstall PREFIX="$prefix" DESTDIR="$destdir"
test ! -e "$destdir$prefix/bin/sshakku"
# -L rather than -e: -e follows the link, so one left pointing at the binary
# just removed would report itself absent while still sitting there.
test ! -L "$destdir$prefix/bin/sshakku-askpass"
test ! -e "$rendered"
if grep -q sshakku "$destdir/etc/zprofile"; then
	echo "zprofile still wired after uninstall" >&2
	exit 1
fi

make install-user USER_HOME="$home"
test -x "$home/.local/bin/sshakku"
test -L "$home/.local/bin/sshakku-askpass"
test -x "$home/.local/bin/sshakku-askpass"
grep -qF sshakku "$home/.zprofile"
# install-user also wires the per-user bindir onto PATH (default WIRE_PATH=1),
# since ~/.local/bin isn't on the macOS default PATH.
grep -qF "export PATH=\"$home/.local/bin:" "$home/.zprofile"

make uninstall-user USER_HOME="$home"
test ! -e "$home/.local/bin/sshakku"
test ! -L "$home/.local/bin/sshakku-askpass"
if [ -f "$home/.zprofile" ] && grep -q sshakku "$home/.zprofile"; then
	echo "per-user zprofile still wired after uninstall" >&2
	exit 1
fi

# ---------------------------------------------------------------------------
# System-wide opt-in non-login wiring (WIRE_ZSHRC=1). Verifies feature F20: the
# opt-in is additive, so a login shell and a plain new tab must BOTH end up
# wired — the zprofile hook is not replaced or disabled by asking for the
# zshrc one — and uninstall must take both back out again.
# ---------------------------------------------------------------------------
zs="$work_dir/wire-zshrc"
zs_destdir="$zs/root"
zs_zprofile="$zs/etc/zprofile"
zs_zshrc="$zs/etc/zshrc"
mkdir -p "$zs_destdir"

make install PREFIX="$zs/prefix" DESTDIR="$zs_destdir" WIRE_ZSHRC=1 \
	ETC_ZPROFILE="$zs_zprofile" ETC_ZSHRC="$zs_zshrc"
for f in "$zs_destdir$zs_zprofile" "$zs_destdir$zs_zshrc"; do
	grep -qF sshakku "$f"
	# The wiring has to point at a hook that is actually there: a marker block
	# sourcing a missing file would satisfy a grep and do nothing in a shell.
	sourced=$(sed -n 's/^\. "\(.*\)"$/\1/p' "$f")
	test -x "$sourced"
done

make uninstall PREFIX="$zs/prefix" DESTDIR="$zs_destdir" WIRE_ZSHRC=1 \
	ETC_ZPROFILE="$zs_zprofile" ETC_ZSHRC="$zs_zshrc"
for f in "$zs_destdir$zs_zprofile" "$zs_destdir$zs_zshrc"; do
	if [ -f "$f" ] && grep -q sshakku "$f"; then
		echo "$f still wired after uninstall" >&2
		exit 1
	fi
done

# ---------------------------------------------------------------------------
# Per-user opt-in non-login wiring, marker-block shape (no ~/.zshrc.d, so the
# block goes into the single ~/.zshrc). Same F20 promise, per-user path.
# ---------------------------------------------------------------------------
home_rc="$work_dir/home-wire-file"
mkdir -p "$home_rc"
make install-user USER_HOME="$home_rc" WIRE_ZSHRC=1
grep -qF sshakku "$home_rc/.zprofile"
grep -qF sshakku "$home_rc/.zshrc"

make uninstall-user USER_HOME="$home_rc" WIRE_ZSHRC=1
for f in "$home_rc/.zprofile" "$home_rc/.zshrc"; do
	if [ -f "$f" ] && grep -q sshakku "$f"; then
		echo "per-user $f still wired after uninstall" >&2
		exit 1
	fi
done

# ---------------------------------------------------------------------------
# Per-user opt-in non-login wiring, drop-in shape: an existing ~/.zshrc.d is
# preferred over touching ~/.zshrc at all.
# ---------------------------------------------------------------------------
home_rcd="$work_dir/home-wire-dropin"
mkdir -p "$home_rcd/.zshrc.d"
make install-user USER_HOME="$home_rcd" WIRE_ZSHRC=1
grep -qF sshakku "$home_rcd/.zprofile"
test -f "$home_rcd/.zshrc.d/001-sshakku-init.sh"
test ! -e "$home_rcd/.zshrc"

make uninstall-user USER_HOME="$home_rcd" WIRE_ZSHRC=1
test ! -e "$home_rcd/.zshrc.d/001-sshakku-init.sh"
