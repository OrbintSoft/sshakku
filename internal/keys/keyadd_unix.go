//go:build unix

package keys

import (
	"os/exec"
	"syscall"
)

// platformChildEnv names the variables ssh-add and the askpass helper it starts
// must be given on this system, beyond the ones every system needs.
//
// USER is what a wallet's own tooling identifies the account by; the two
// display variables are how a graphical prompt finds a screen to appear on; the
// XDG pair is where this system keeps a login's runtime and configuration
// directories, which is where the helper looks for the handoff and for the
// settings; and the bus address is the wallet itself on a desktop that has one.
var platformChildEnv = []string{
	"USER",
	"DISPLAY",
	"WAYLAND_DISPLAY",
	"XDG_RUNTIME_DIR",
	"XDG_CONFIG_HOME",
	"DBUS_SESSION_BUS_ADDRESS",
}

// detachFromTerminal gives cmd a session of its own, so ssh-add has no
// controlling terminal to ask on and must go to the SSH_ASKPASS helper for the
// passphrase instead.
func detachFromTerminal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
