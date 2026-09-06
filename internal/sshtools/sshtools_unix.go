//go:build unix

package sshtools

// ThisSystem is what this system has: one OpenSSH, and an agent listening on a
// socket that any build of it can open. Nothing here emulates another system's
// kernel, so there is nothing to choose between and no directory to prefer —
// the program the session finds is the program that runs, including a build the
// user put on their PATH themselves.
func ThisSystem() System { return System{} }
