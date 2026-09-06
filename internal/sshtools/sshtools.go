// Package sshtools decides which of OpenSSH's programs SSHakku runs.
//
// On most systems there is nothing to decide: one OpenSSH is installed, the
// agent is a socket any build of it can open, and the program a session finds
// on its PATH is the program to run. A system whose agent is a named pipe is
// different. More than one build of OpenSSH can be installed there, and they
// are not interchangeable: a build made for a POSIX emulation layer talks to
// that layer's own socket and has no way to open a pipe of the system
// underneath it. A shell that brings such a build puts it first on its PATH,
// so a program started from that shell reaches for it without anything having
// gone visibly wrong.
//
// What that costs is not a failure anyone can read. Handed a key, the wrong
// build exits the way it exits for a wrong passphrase — so a correct passphrase
// is asked for again, and again, until the attempts run out and the key is
// given up on.
package sshtools

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/OrbintSoft/sshakku/internal/paths"
)

// The two OpenSSH programs this package is asked about. The names are
// OpenSSH's, not choices made here.
const (
	// SSHAddName adds a key to the agent, lists what the agent holds, and takes
	// a key back out. It is what SSHakku itself runs.
	SSHAddName = "ssh-add"
	// SSHName is what the session runs for itself — every `ssh` the user types,
	// and every one `git` starts on their behalf.
	SSHName = "ssh"
)

// lookPath resolves a program the way the session that started this one would.
// It is a variable so a test can put a system's answer in front of the
// deciding without having to be running on that system.
var lookPath = exec.LookPath

// System is what one system has to say about the OpenSSH builds on it.
//
// It is data rather than code so that the deciding below stays one piece of
// logic, checkable from either machine against either machine's answer — the
// one this project's developers are sitting at, and the one where the question
// arises at all.
type System struct {
	// EmulationRuntimes are the libraries a build made for a POSIX emulation
	// layer carries beside it. A program in a directory holding one of these
	// runs on that layer rather than on this system, and reaches this system's
	// agent through nothing at all.
	EmulationRuntimes []string
	// NativeDirs are where this system keeps the build that speaks to its own
	// agent, in the order they are to be preferred. Empty entries and
	// directories that are not there are passed over, since a system names
	// where it keeps such a build and not that every machine has one.
	NativeDirs []string
}

// SSHAdd returns the ssh-add this system's agent can be reached with.
func SSHAdd() (string, error) { return ThisSystem().Tool(SSHAddName) }

// Tool returns the program to run for one of OpenSSH's agent-facing tools.
//
// Where the session's own answer can reach the agent, the name comes back
// unchanged and the session's lookup resolves it: the user goes on running the
// program their PATH says they run, including a build of their own they put
// there deliberately. Only where that answer cannot reach the agent is another
// program named, and where there is none, that is said rather than worked
// around — running the one that cannot reach it is what this exists to stop.
func (s System) Tool(name string) (string, error) {
	if len(s.EmulationRuntimes) == 0 {
		// Nothing here emulates another system, so every build of OpenSSH on
		// this machine reaches the same agent and there is nothing to choose
		// between. Looking anything up would only be a slower way to say so.
		return name, nil
	}

	found, err := lookPath(name)
	if err != nil {
		// The session was handed a PATH with no OpenSSH on it. The system may
		// still have its own, somewhere this session was never told about.
		if native, ok := s.nativeBuild(name); ok {
			return native, nil
		}
		return "", err
	}

	runtime := s.emulationRuntimeBeside(found)
	if runtime == "" {
		return name, nil
	}
	if native, ok := s.nativeBuild(name); ok {
		return native, nil
	}
	return "", emulatedOnlyError{name: name, found: found, runtime: runtime}
}

// SessionTools says what the session that started this process runs for one of
// OpenSSH's programs, and what it would take for that session to reach this
// system's agent with it.
//
// It is about the session's own tools rather than the ones SSHakku drives.
// Pointing a shell at an agent its own ssh cannot open is pointing it at
// nothing: every `ssh` typed there, and every one `git` starts on the user's
// behalf, asks for a passphrase however full the agent is.
type SessionTools struct {
	// Emulated is the build the session finds first, where that build cannot
	// reach this system's agent. Empty where what the session finds can — and
	// it is worth having even when nothing can be done about it, since it names
	// which environment the session belongs to.
	Emulated string
	// Native is the directory holding a build that can reach the agent, to be
	// searched ahead of the session's own. Empty where nothing has to change,
	// and empty too where this machine has nothing better to offer: a directory
	// with no ssh in it on somebody's PATH for the rest of their login would
	// change nothing except their PATH.
	Native string
}

// SessionSSHTools reports what the session that started this process would run
// for name, and what it would take for that session to reach the agent.
func (s System) SessionSSHTools(name string) SessionTools {
	if len(s.EmulationRuntimes) == 0 {
		return SessionTools{}
	}
	found, err := lookPath(name)
	if err != nil {
		// A PATH with no ssh on it is not a session running the wrong one, and
		// what the user runs ssh from is their arrangement, not this program's.
		return SessionTools{}
	}
	if s.emulationRuntimeBeside(found) == "" {
		return SessionTools{}
	}
	native, ok := s.nativeBuild(name)
	if !ok {
		return SessionTools{Emulated: found}
	}
	return SessionTools{Emulated: found, Native: filepath.Dir(native)}
}

// nativeBuild finds name in the directories this system keeps its own OpenSSH
// in, and reports whether one of them had it.
func (s System) nativeBuild(name string) (string, bool) {
	for _, dir := range s.NativeDirs {
		if dir == "" {
			continue
		}
		prog, err := lookPath(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		return prog, true
	}
	return "", false
}

// emulationRuntimeBeside names the emulation runtime sitting in the same
// directory as prog, or "" where none does.
//
// Which layer a program was built for is asked of the directory it is in
// rather than of the program itself: a build for an emulation layer is shipped
// with that layer's runtime beside it, because it cannot start without one.
func (s System) emulationRuntimeBeside(prog string) string {
	dir := filepath.Dir(prog)
	for _, runtime := range s.EmulationRuntimes {
		if !paths.Absent(filepath.Join(dir, runtime)) {
			return runtime
		}
	}
	return ""
}

// emulatedOnlyError is a session whose only OpenSSH is one that cannot reach
// this system's agent. It names the program and the runtime beside it, because
// a user told only that something is wrong has nowhere to go and nothing to
// look at.
type emulatedOnlyError struct {
	name    string
	found   string
	runtime string
}

func (e emulatedOnlyError) Error() string {
	return fmt.Sprintf("the %s on this session's PATH is %s, which is built for a POSIX emulation"+
		" layer (%s sits beside it) and cannot reach this system's agent; no build that can was found",
		e.name, e.found, e.runtime)
}
