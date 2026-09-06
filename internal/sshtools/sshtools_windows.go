//go:build windows

package sshtools

import (
	"os"
	"path/filepath"
)

// The two POSIX emulation layers a build of OpenSSH here may have been made
// for. Git for Windows ships a build against the first, and a Cygwin
// installation one against the second; either of them is what a shell of that
// kind puts first on its PATH. Such a build answers "Error connecting to agent"
// however healthy the agent is, and in both writings of the pipe's name,
// because a pipe is not a thing it can open at all.
const (
	msys2Runtime  = "msys-2.0.dll"
	cygwinRuntime = "cygwin1.dll"
)

// ThisSystem is what this system has: two ways of emulating POSIX on it, and
// one directory where it keeps the OpenSSH that speaks to its own agent.
//
// Where that directory is comes from the environment rather than being written
// out here, because an installation is not always on C: — a path with the drive
// letter in it is a lookup that quietly finds nothing on every machine it is
// wrong about, and quietly finding nothing is the failure this package exists
// to remove.
func ThisSystem() System {
	return System{
		EmulationRuntimes: []string{msys2Runtime, cygwinRuntime},
		NativeDirs:        systemOpenSSHDirs(),
	}
}

// systemOpenSSHDirs is where this system installs the OpenSSH it ships with. A
// system that will not say where it is installed names no directory at all,
// rather than one resolved against wherever this process happens to be running
// from.
func systemOpenSSHDirs() []string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		return nil
	}
	return []string{filepath.Join(root, "System32", "OpenSSH")}
}
