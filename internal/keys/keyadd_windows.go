//go:build windows

package keys

import "os/exec"

// platformChildEnv names the variables ssh-add and the askpass helper it starts
// must be given on this system, beyond the ones every system needs.
//
// Most of this list is not about SSHakku at all. A process here starts with
// almost nothing working if SystemRoot and its two relatives are absent — the
// socket library the handoff crosses is among what fails — and the account's
// own directories, which is where the helper goes looking for that handoff, are
// named by USERPROFILE and LOCALAPPDATA and by nothing else. TEMP, ComSpec and
// PATHEXT are what any child expects to have on this system, and a program
// started without them behaves in ways nobody would connect back to a
// passphrase.
//
// ProgramData is the one that had to be measured rather than reasoned about.
// This system's ssh-add resolves the machine-wide ssh configuration under it,
// so a child that does not get it is reading a different configuration from the
// session that started it. What that costs depends on the machine: where that
// directory exists, ssh-add exits 255 having printed nothing whatsoever — no
// message, no clue, and an exit code that says only that something went wrong
// very early; where it was never created, nothing happens at all, which is what
// makes the first kind of machine so hard to recognise from the second.
//
// The cost of getting any of this wrong is that it does not look wrong. What a
// user sees is a key that will not load and a session log saying their stored
// passphrase has gone stale, when the passphrase was perfect and the program
// that was handed it never got as far as reading it.
var platformChildEnv = []string{
	"SystemRoot",
	"SystemDrive",
	"windir",
	"ProgramData",
	"USERPROFILE",
	"LOCALAPPDATA",
	"APPDATA",
	"USERNAME",
	"TEMP",
	"TMP",
	"ComSpec",
	"PATHEXT",
}

// detachFromTerminal leaves cmd attached to whatever console it inherited:
// Windows has no session to leave, and detaching from a console is a different
// mechanism from Unix's setsid. What forces ssh-add to the SSH_ASKPASS helper
// here is the caller giving it no stdin, not the absence of a console.
func detachFromTerminal(*exec.Cmd) {}
